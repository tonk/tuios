package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tonk/tuios/internal/pamauth"
	"github.com/tonk/tuios/internal/session"
)

// classroomPickerRefreshInterval is how often the picker re-polls the daemon
// for its live, trainee-pattern-filtered session list.
const classroomPickerRefreshInterval = 3 * time.Second

// classroomPickerTitleIdle is the window title while the trainer console list
// is showing. The front-door page script (frontDoorWebsocketHead) treats this
// as "Enter opens a new browser tab", so Enter can window.open during the
// keydown gesture - required to beat popup blockers - and later navigate that
// tab when a tuios-open-tab:* title arrives.
const classroomPickerTitleIdle = "tuios-trainer-picker"

// classroomPickerModel is the trainer console's landing screen: an
// authorized trainer connecting with no "attach" query parameter (see
// pamAuthMiddleware/classroomShowPickerFromContext) lands here instead of
// their own session, and picks a live trainee session - or their own,
// ordinary session - to open in a new browser tab (via ?attach=<name>),
// keeping this picker tab as the console. Previously Enter attached
// in-process in the same tab, which made returning to the list require a
// full reload without ?attach=.
//
// The list always has one extra, fixed entry at the top - "My own session"
// - ahead of the live trainee list; cursor 0 is that entry, cursor N is
// m.sessions[N-1].
//
// login is kept alive for as long as the picker is showing and closed only
// on quit: opening a session happens in a new tab that authenticates on its
// own (Basic Auth / PAM stash), so this connection never needs a login
// handoff.
type classroomPickerModel struct {
	ctx           context.Context
	login         *pamauth.Login
	self          string
	pattern       *regexp.Regexp
	patternErr    error
	width, height int
	graphicsOut   *os.File
	touch         bool

	sessions []session.SessionInfo
	cursor   int
	loadErr  error

	// openTabSeq / openTabUser arm a one-shot WindowTitle the injected page
	// script turns into window.open(...?attach=<user>). Cleared shortly after
	// so the next Enter for the same user still produces a distinct title.
	openTabSeq  uint64
	openTabUser string
}

type classroomPickerTickMsg struct{}

type classroomPickerRefreshMsg struct {
	sessions []session.SessionInfo
	err      error
}

type classroomPickerClearOpenTabMsg struct{}

// newClassroomPickerModel builds the picker for an authorized trainer.
// traineePattern is the raw [classroom] trainee_pattern config string,
// compiled here rather than by the caller so a picker is always buildable -
// an invalid pattern surfaces as an on-screen error instead of a startup
// failure, the same "fails closed" treatment ClassroomConfig.MatchesTrainee
// already gives it server-side.
func newClassroomPickerModel(ctx context.Context, login *pamauth.Login, traineePattern string, width, height int, graphicsOut *os.File, touch bool) *classroomPickerModel {
	m := &classroomPickerModel{
		ctx: ctx, login: login, self: login.Username(),
		width: width, height: height, graphicsOut: graphicsOut, touch: touch,
	}
	if traineePattern == "" {
		m.patternErr = fmt.Errorf("classroom.trainee_pattern is empty; no session can ever match")
	} else if re, err := regexp.Compile(traineePattern); err != nil {
		m.patternErr = fmt.Errorf("classroom.trainee_pattern %q does not compile: %w", traineePattern, err)
	} else {
		m.pattern = re
	}
	return m
}

func (m *classroomPickerModel) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), classroomPickerTick())
}

func classroomPickerTick() tea.Cmd {
	return tea.Tick(classroomPickerRefreshInterval, func(time.Time) tea.Msg { return classroomPickerTickMsg{} })
}

// refreshCmd polls the daemon for its current session list. A fresh,
// one-shot session.Client is used rather than keeping a persistent
// connection open: the picker's own connection to the daemon needs nothing
// but this occasional read, and a local Unix-socket round trip is cheap
// enough every few seconds for as long as a trainer's picker screen is open.
func (m *classroomPickerModel) refreshCmd() tea.Cmd {
	if m.patternErr != nil {
		return nil
	}
	pattern := m.pattern
	self := m.self
	return func() tea.Msg {
		client := session.NewClient(&session.ClientConfig{Version: "trainer-console"})
		if err := client.Connect(); err != nil {
			return classroomPickerRefreshMsg{err: err}
		}
		defer func() { _ = client.Close() }()

		all, err := client.ListSessions()
		if err != nil {
			return classroomPickerRefreshMsg{err: err}
		}
		return classroomPickerRefreshMsg{sessions: filterClassroomSessions(all, self, pattern)}
	}
}

// filterClassroomSessions keeps only sessions matching pattern, excluding
// self even if its own name would otherwise match (e.g. a trainer named
// "guru00" against "^guru[0-9]{2}$": it appears as the picker's fixed "My
// own session" entry instead, never twice), sorted by name for a stable,
// predictable list.
func filterClassroomSessions(all []session.SessionInfo, self string, pattern *regexp.Regexp) []session.SessionInfo {
	filtered := make([]session.SessionInfo, 0, len(all))
	for _, s := range all {
		if s.Name == self {
			continue
		}
		if pattern.MatchString(s.Name) {
			filtered = append(filtered, s)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name < filtered[j].Name })
	return filtered
}

func (m *classroomPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case classroomPickerTickMsg:
		return m, tea.Batch(m.refreshCmd(), classroomPickerTick())

	case classroomPickerRefreshMsg:
		m.loadErr = msg.err
		if msg.err == nil {
			m.sessions = msg.sessions
			if m.cursor > len(m.sessions) {
				m.cursor = len(m.sessions)
			}
		}
		return m, nil

	case classroomPickerClearOpenTabMsg:
		m.openTabUser = ""
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.sessions) {
				m.cursor++
			}
			return m, nil
		case "enter":
			return m.armOpenTab()
		case "q", "esc", "ctrl+c":
			_ = m.login.Close()
			return m, tea.Quit
		}
	}
	return m, nil
}

// armOpenTab sets a one-shot window title the front-door page script turns
// into a new browser tab at ?attach=<user>, then clears the title so a later
// Enter for the same user still emits a distinct signal.
func (m *classroomPickerModel) armOpenTab() (tea.Model, tea.Cmd) {
	name := m.self
	if m.cursor > 0 {
		name = m.sessions[m.cursor-1].Name
	}
	m.openTabSeq++
	m.openTabUser = name
	m.loadErr = nil
	// Long enough for the title to reach the browser over the websocket
	// before we flip back to the idle picker title; too short and the
	// open-tab signal is overwritten before the page script sees it.
	return m, tea.Tick(time.Second, func(time.Time) tea.Msg {
		return classroomPickerClearOpenTabMsg{}
	})
}

func (m *classroomPickerModel) View() tea.View {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).MarginBottom(1)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(true).Padding(0, 1)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Padding(0, 1)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)

	var b strings.Builder
	b.WriteString(titleStyle.Render("TUIOS Trainer Console"))
	b.WriteString("\n\n")

	if err := m.patternErr; err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Misconfigured: %v", err)))
		b.WriteString("\n\n")
	} else if m.loadErr != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.loadErr)))
		b.WriteString("\n\n")
	}

	ownLine := fmt.Sprintf("My own session (%s)", m.self)
	if m.cursor == 0 {
		b.WriteString(selectedStyle.Render("▸ " + ownLine))
	} else {
		b.WriteString(normalStyle.Render("  " + ownLine))
	}
	b.WriteString("\n\n")

	if len(m.sessions) == 0 {
		b.WriteString(dimStyle.Render("No live trainee sessions right now."))
		b.WriteString("\n\n")
	} else {
		for i, s := range m.sessions {
			status := "detached"
			if s.Attached {
				status = "attached"
			}
			line := fmt.Sprintf("%s (%d windows, %s)", s.Name, s.WindowCount, status)
			if i+1 == m.cursor {
				b.WriteString(selectedStyle.Render("▸ " + line))
			} else {
				b.WriteString(normalStyle.Render("  " + line))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render("↑/k up  ↓/j down  enter: open in new tab  q/esc: disconnect"))

	// Centered in the full terminal size, matching tuios's own empty-
	// workspace welcome screen (see render_overlays.go's identical
	// lipgloss.Place(..., lipgloss.Center, lipgloss.Center, ...)): tea.View
	// has no "fill the screen" default the way an ordinary tuios
	// window/dock layout does, and left un-placed the picker rendered as a
	// cramped block of text pinned to the top-left corner of an otherwise
	// blank viewport - the reported "screen size is incorrect".
	content := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, b.String())

	var v tea.View
	v.SetContent(content)
	v.AltScreen = true
	if m.openTabUser != "" {
		// seq makes each Enter distinct so the page script re-fires for the
		// same username; the username is the only untrusted piece and is
		// validated again client-side before window.open.
		v.WindowTitle = fmt.Sprintf("tuios-open-tab:%d:%s", m.openTabSeq, m.openTabUser)
	} else {
		v.WindowTitle = classroomPickerTitleIdle
	}
	return v
}
