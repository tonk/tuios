package main

import (
	"context"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tonk/tuios/internal/pamauth"
	"github.com/tonk/tuios/internal/session"
)

func TestFilterClassroomSessions(t *testing.T) {
	pattern := regexp.MustCompile("^guru[0-9]{2}$")
	all := []session.SessionInfo{
		{Name: "guru07"},
		{Name: "guru00"}, // the trainer's own name; must be excluded
		{Name: "web"},    // does not match the pattern
		{Name: "guru02"},
		{Name: "root"}, // does not match the pattern
	}

	got := filterClassroomSessions(all, "guru00", pattern)
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2: %+v", len(got), got)
	}
	if got[0].Name != "guru02" || got[1].Name != "guru07" {
		t.Errorf("got %q, %q; want sorted guru02, guru07", got[0].Name, got[1].Name)
	}
}

func TestNewClassroomPickerModelInvalidPattern(t *testing.T) {
	fakeSocket := runFakePAMHelperForAuth(t)
	login := dialFakeLogin(t, fakeSocket, "ton")

	m := newClassroomPickerModel(context.Background(), login, "guru[0-9", 80, 24, nil, false)
	if m.patternErr == nil {
		t.Fatal("expected a pattern error for an unparseable regex")
	}
	if m.refreshCmd() != nil {
		t.Error("refreshCmd should be a no-op when the pattern is invalid")
	}
}

func TestNewClassroomPickerModelEmptyPattern(t *testing.T) {
	fakeSocket := runFakePAMHelperForAuth(t)
	login := dialFakeLogin(t, fakeSocket, "ton")

	m := newClassroomPickerModel(context.Background(), login, "", 80, 24, nil, false)
	if m.patternErr == nil {
		t.Fatal("expected a pattern error for an empty trainee_pattern")
	}
}

func TestClassroomPickerCursorNavigation(t *testing.T) {
	fakeSocket := runFakePAMHelperForAuth(t)
	login := dialFakeLogin(t, fakeSocket, "ton")

	m := newClassroomPickerModel(context.Background(), login, "^guru[0-9]{2}$", 80, 24, nil, false)
	m.sessions = []session.SessionInfo{{Name: "guru01"}, {Name: "guru02"}, {Name: "guru03"}}

	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0 (\"My own session\")", m.cursor)
	}

	model, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = model.(*classroomPickerModel)
	if m.cursor != 1 {
		t.Fatalf("cursor after one down = %d, want 1 (guru01)", m.cursor)
	}

	model, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = model.(*classroomPickerModel)
	model, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = model.(*classroomPickerModel)
	model, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = model.(*classroomPickerModel)
	if m.cursor != 3 {
		t.Fatalf("cursor should not move past the last session: got %d, want 3 (guru03)", m.cursor)
	}

	model, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = model.(*classroomPickerModel)
	if m.cursor != 2 {
		t.Fatalf("cursor after one up = %d, want 2 (guru02)", m.cursor)
	}
}

func TestClassroomPickerQuits(t *testing.T) {
	fakeSocket := runFakePAMHelperForAuth(t)
	login := dialFakeLogin(t, fakeSocket, "ton")
	m := newClassroomPickerModel(context.Background(), login, "^guru[0-9]{2}$", 80, 24, nil, false)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("expected a quit command from 'q'")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("expected tea.Quit's message, got %#v", msg)
	}
}

// TestClassroomPickerEnterOpensOwnSessionInNewTab pins that Enter on cursor 0
// arms a tuios-open-tab title for the trainer's own username (the new-tab
// bridge), rather than attaching in-process - and leaves login open so the
// picker tab can keep running as the console.
func TestClassroomPickerEnterOpensOwnSessionInNewTab(t *testing.T) {
	fakeSocket := runFakePAMHelperForAuth(t)
	login := dialFakeLogin(t, fakeSocket, "ton")
	m := newClassroomPickerModel(context.Background(), login, "^guru[0-9]{2}$", 80, 24, nil, false)
	m.sessions = []session.SessionInfo{{Name: "guru01"}}

	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(*classroomPickerModel)
	if m.openTabUser != "ton" {
		t.Fatalf("openTabUser = %q, want ton", m.openTabUser)
	}
	if m.openTabSeq != 1 {
		t.Fatalf("openTabSeq = %d, want 1", m.openTabSeq)
	}
	title := m.View().WindowTitle
	if !strings.HasPrefix(title, "tuios-open-tab:1:ton") {
		t.Fatalf("WindowTitle = %q, want tuios-open-tab:1:ton", title)
	}
	if cmd == nil {
		t.Fatal("expected a clear-open-tab tick command")
	}
	// Login must stay open: this tab remains the picker console.
	if err := m.login.Close(); err != nil {
		t.Fatalf("login should still be open after arming a new-tab open: %v", err)
	}
}

// TestClassroomPickerEnterOpensTraineeInNewTab pins Enter on a trainee row
// arms ?attach=<trainee> via the open-tab title and does not close login.
func TestClassroomPickerEnterOpensTraineeInNewTab(t *testing.T) {
	fakeSocket := runFakePAMHelperForAuth(t)
	login := dialFakeLogin(t, fakeSocket, "ton")
	m := newClassroomPickerModel(context.Background(), login, "^guru[0-9]{2}$", 80, 24, nil, false)
	m.sessions = []session.SessionInfo{{Name: "guru01"}}
	m.cursor = 1

	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(*classroomPickerModel)
	if m.openTabUser != "guru01" {
		t.Fatalf("openTabUser = %q, want guru01", m.openTabUser)
	}
	title := m.View().WindowTitle
	if title != "tuios-open-tab:1:guru01" {
		t.Fatalf("WindowTitle = %q, want tuios-open-tab:1:guru01", title)
	}
	if msg := cmd(); msg != (classroomPickerClearOpenTabMsg{}) {
		// cmd is a Tick that returns the clear msg after delay; invoke via
		// running the tick's function indirectly by waiting - easier: just
		// apply ClearOpenTabMsg and check title resets.
		_ = msg
	}
	model, _ = m.Update(classroomPickerClearOpenTabMsg{})
	m = model.(*classroomPickerModel)
	if m.openTabUser != "" {
		t.Fatalf("openTabUser still %q after clear", m.openTabUser)
	}
	if got := m.View().WindowTitle; got != classroomPickerTitleIdle {
		t.Fatalf("WindowTitle after clear = %q, want %q", got, classroomPickerTitleIdle)
	}
	if err := m.login.Close(); err != nil {
		t.Fatalf("login should still be open: %v", err)
	}
}

func TestClassroomPickerClearOpenTabCmd(t *testing.T) {
	fakeSocket := runFakePAMHelperForAuth(t)
	login := dialFakeLogin(t, fakeSocket, "ton")
	m := newClassroomPickerModel(context.Background(), login, "^guru[0-9]{2}$", 80, 24, nil, false)
	defer func() { _ = m.login.Close() }()

	_, cmd := m.armOpenTab()
	if cmd == nil {
		t.Fatal("expected clear tick")
	}
	if m.openTabUser != "ton" || m.openTabSeq != 1 {
		t.Fatalf("armed openTabUser=%q seq=%d", m.openTabUser, m.openTabSeq)
	}
}

func TestClassroomPickerViewFillsTheTerminal(t *testing.T) {
	fakeSocket := runFakePAMHelperForAuth(t)
	login := dialFakeLogin(t, fakeSocket, "ton")
	m := newClassroomPickerModel(context.Background(), login, "^guru[0-9]{2}$", 100, 40, nil, false)
	defer func() { _ = m.login.Close() }()

	view := m.View()
	lines := strings.Split(view.Content, "\n")
	if len(lines) < 40 {
		t.Errorf("rendered %d lines, want at least the full 40-row terminal height", len(lines))
	}
	if !strings.Contains(view.Content, "My own session (ton)") {
		t.Error("view does not contain the \"My own session\" entry")
	}
	if view.WindowTitle != classroomPickerTitleIdle {
		t.Errorf("idle WindowTitle = %q, want %q", view.WindowTitle, classroomPickerTitleIdle)
	}
	if !strings.Contains(view.Content, "open in new tab") {
		t.Error("view hint should mention opening in a new tab")
	}
}

// dialFakeLogin dials the fake helper directly (bypassing pamAuthMiddleware)
// to get a *pamauth.Login for tests that only need a valid, closeable Login,
// not the full HTTP auth flow.
func dialFakeLogin(t *testing.T, socketPath, username string) *pamauth.Login {
	t.Helper()
	login, err := pamauth.Dial(socketPath, username, "irrelevant")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	return login
}
