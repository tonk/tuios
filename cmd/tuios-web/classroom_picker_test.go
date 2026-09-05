package main

import (
	"regexp"
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

	m := newClassroomPickerModel(login, "guru[0-9", 80, 24, nil, false)
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

	m := newClassroomPickerModel(login, "", 80, 24, nil, false)
	if m.patternErr == nil {
		t.Fatal("expected a pattern error for an empty trainee_pattern")
	}
}

func TestClassroomPickerCursorNavigation(t *testing.T) {
	fakeSocket := runFakePAMHelperForAuth(t)
	login := dialFakeLogin(t, fakeSocket, "ton")

	m := newClassroomPickerModel(login, "^guru[0-9]{2}$", 80, 24, nil, false)
	m.sessions = []session.SessionInfo{{Name: "guru01"}, {Name: "guru02"}, {Name: "guru03"}}

	model, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = model.(*classroomPickerModel)
	if m.cursor != 1 {
		t.Fatalf("cursor after one down = %d, want 1", m.cursor)
	}

	model, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = model.(*classroomPickerModel)
	model, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = model.(*classroomPickerModel)
	if m.cursor != 2 {
		t.Fatalf("cursor should not move past the last session: got %d, want 2", m.cursor)
	}

	model, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = model.(*classroomPickerModel)
	if m.cursor != 1 {
		t.Fatalf("cursor after one up = %d, want 1", m.cursor)
	}
}

func TestClassroomPickerQuits(t *testing.T) {
	fakeSocket := runFakePAMHelperForAuth(t)
	login := dialFakeLogin(t, fakeSocket, "ton")
	m := newClassroomPickerModel(login, "^guru[0-9]{2}$", 80, 24, nil, false)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("expected a quit command from 'q'")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("expected tea.Quit's message, got %#v", msg)
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
