package main

import (
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

// classroomPickerModel is the trainer console's landing screen: an
// authorized trainer connecting with no "attach" query parameter (see
// pamAuthMiddleware/classroomShowPickerFromContext) lands here instead of
// their own session, and picks a live trainee session to attach to instead
// of having to know/type a username. Once one is chosen, Update/View
// delegate to the attached daemon-backed OS instance built around it - see
// the "m.attached != nil" branches below - for the rest of the connection's
// life.
//
// login is only ever used to authenticate this connection; a trainer never
// spawns anything themselves (see createTrainerAttachInstance's own
// reasoning), so it is closed as soon as the picker is built.
type classroomPickerModel struct {
	self          string
	pattern       *regexp.Regexp
	patternErr    error
	width, height int
	graphicsOut   *os.File
	touch         bool

	sessions []session.SessionInfo
	cursor   int
	loadErr  error

	attached tea.Model
}

type classroomPickerTickMsg struct{}

type classroomPickerRefreshMsg struct {
	sessions []session.SessionInfo
	err      error
}

// newClassroomPickerModel builds the picker for an authorized trainer.
// traineePattern is the raw [classroom] trainee_pattern config string,
// compiled here rather than by the caller so a picker is always buildable -
// an invalid pattern surfaces as an on-screen error instead of a startup
// failure, the same "fails closed" treatment ClassroomConfig.MatchesTrainee
// already gives it server-side.
func newClassroomPickerModel(login *pamauth.Login, traineePattern string, width, height int, graphicsOut *os.File, touch bool) *classroomPickerModel {
	self := login.Username()
	_ = login.Close()

	m := &classroomPickerModel{self: self, width: width, height: height, graphicsOut: graphicsOut, touch: touch}
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
// "guru00" against "^guru[0-9]{2}$": watching their own session through the
// picker makes no sense), sorted by name for a stable, predictable list.
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
	if m.attached != nil {
		updated, cmd := m.attached.Update(msg)
		m.attached = updated
		return m, cmd
	}

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
			if m.cursor >= len(m.sessions) {
				m.cursor = max(len(m.sessions)-1, 0)
			}
		}
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}
			return m, nil
		case "enter":
			if len(m.sessions) == 0 {
				return m, nil
			}
			return m.attach(m.sessions[m.cursor].Name)
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

// attach builds a daemon-backed instance around an already-live session
// from the picker's own list - never a login handoff, since a session only
// ever appears here because some trainee is already logged into it under
// their own account.
func (m *classroomPickerModel) attach(name string) (tea.Model, tea.Cmd) {
	model, _, err := attachDaemonSession(name, false, m.width, m.height, m.graphicsOut, m.touch)
	if err != nil {
		m.loadErr = fmt.Errorf("attaching to %q: %w", name, err)
		return m, nil
	}
	m.attached = model
	return m, model.Init()
}

func (m *classroomPickerModel) View() tea.View {
	if m.attached != nil {
		return m.attached.View()
	}

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
			if i == m.cursor {
				b.WriteString(selectedStyle.Render("▸ " + line))
			} else {
				b.WriteString(normalStyle.Render("  " + line))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render("↑/k up  ↓/j down  enter: attach  q/esc: disconnect"))

	var v tea.View
	v.SetContent(b.String())
	v.AltScreen = true
	return v
}
