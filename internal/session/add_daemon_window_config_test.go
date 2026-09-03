package session

import (
	"testing"

	"github.com/tonk/tuios/internal/config"
)

// TestAddDaemonWindowAppliesInitialTitleFormat pins
// appearance.initial_title_format reaching the daemon-headless window
// creation path (AddDaemonWindow) too, not just the TUI's own AddWindow -
// this is the only creation point a daemon session's "new window" actually
// goes through (see os_window.go's AddWindow, which sends an empty title
// over the wire for a daemon session and lets the daemon decide).
func TestAddDaemonWindowAppliesInitialTitleFormat(t *testing.T) {
	prev := config.InitialTitleFormat
	config.InitialTitleFormat = "{user}'s shell"
	t.Cleanup(func() { config.InitialTitleFormat = prev })

	sess := newTestSession(t)
	win, err := sess.AddDaemonWindow("", nil)
	if err != nil {
		t.Fatalf("AddDaemonWindow failed: %v", err)
	}

	want := config.FormatInitialTitle()
	if win.Title != want {
		t.Errorf("Title = %q, want %q (from initial_title_format)", win.Title, want)
	}
}

// TestAddDaemonWindowAppliesLockTitles pins appearance.lock_titles reaching
// the daemon-headless window creation path's WindowState.TitleLocked, so a
// trainee's window starts locked even when created via the daemon rather
// than a local session.
func TestAddDaemonWindowAppliesLockTitles(t *testing.T) {
	prev := config.LockTitles
	config.LockTitles = true
	t.Cleanup(func() { config.LockTitles = prev })

	sess := newTestSession(t)
	win, err := sess.AddDaemonWindow("", nil)
	if err != nil {
		t.Fatalf("AddDaemonWindow failed: %v", err)
	}
	if !win.TitleLocked {
		t.Error("a window created with lock_titles=true was not title-locked")
	}
}

// TestAddDaemonWindowExplicitTitleWinsOverFormat: a caller that already
// supplied a title (e.g. the NewWindow tape/verb command giving one
// explicitly) is not overridden by initial_title_format - the format only
// fills in when the caller left it blank, the same precedence the plain
// "Terminal <id>" fallback it replaces already had.
func TestAddDaemonWindowExplicitTitleWinsOverFormat(t *testing.T) {
	prev := config.InitialTitleFormat
	config.InitialTitleFormat = "{user}'s shell"
	t.Cleanup(func() { config.InitialTitleFormat = prev })

	sess := newTestSession(t)
	win, err := sess.AddDaemonWindow("explicit-title", nil)
	if err != nil {
		t.Fatalf("AddDaemonWindow failed: %v", err)
	}
	if win.Title != "explicit-title" {
		t.Errorf("Title = %q, want the explicitly supplied title unchanged", win.Title)
	}
}
