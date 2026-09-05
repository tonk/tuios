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

// TestClassroomPickerOwnSessionIsAlwaysCursorZero pins the fixed "My own
// session" entry ahead of the live trainee list: cursor 0 always means "my
// own session", regardless of how many trainee sessions are currently
// listed, and enter at cursor 0 must route to attachOwn (which needs
// login), not attach (which would try to treat "" as a trainee's session
// name).
func TestClassroomPickerOwnSessionIsAlwaysCursorZero(t *testing.T) {
	fakeSocket := runFakePAMHelperForAuth(t)
	login := dialFakeLogin(t, fakeSocket, "ton")
	m := newClassroomPickerModel(context.Background(), login, "^guru[0-9]{2}$", 80, 24, nil, false)
	m.sessions = []session.SessionInfo{{Name: "guru01"}}

	// attachOwn will fail here (no real daemon reachable), but that failure
	// itself proves cursor 0 routed through attachOwn and not attach: the
	// error message names attachOwn's own wording, and login (consumed by
	// createClassroomTUIOSInstance regardless of outcome) ends up closed.
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(*classroomPickerModel)
	if m.loadErr == nil {
		t.Fatal("expected attachOwn to fail with no daemon reachable")
	}
	if !strings.Contains(m.loadErr.Error(), "attaching to your own session") {
		t.Errorf("loadErr = %v, want it to come from attachOwn, not attach", m.loadErr)
	}
}

// TestClassroomPickerCrossAttachClosesLogin pins that attaching to another
// trainee's session closes login: that path never needs it (see attach's
// own doc comment), and leaving it open would leak a live connection to
// tuios-pam-helper - and, in production, an open PAM session - for as long
// as the resulting attached OS instance keeps running.
func TestClassroomPickerCrossAttachClosesLogin(t *testing.T) {
	fakeSocket := runFakePAMHelperForAuth(t)
	login := dialFakeLogin(t, fakeSocket, "ton")
	m := newClassroomPickerModel(context.Background(), login, "^guru[0-9]{2}$", 80, 24, nil, false)
	m.sessions = []session.SessionInfo{{Name: "guru01"}}
	m.cursor = 1

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(*classroomPickerModel)
	if m.loadErr == nil {
		t.Fatal("expected attach to fail with no daemon reachable")
	}
	if err := m.login.Close(); err == nil {
		t.Error("login.Close() succeeded on a second call; attach should have already closed it")
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
