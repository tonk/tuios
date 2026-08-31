package app

import (
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/config"
)

// TestAddWindowAppliesInitialTitleFormat pins appearance.initial_title_format
// reaching an actual created window's title, not just the config global -
// FormatInitialTitle is only honest if AddWindow's own "Terminal <id>"
// literal defers to it.
func TestAddWindowAppliesInitialTitleFormat(t *testing.T) {
	prev := config.InitialTitleFormat
	config.InitialTitleFormat = "{user}'s pane"
	t.Cleanup(func() { config.InitialTitleFormat = prev })

	m := newStartupOS(t, false, false)
	defer closeWindows(m)

	m.AddWindow("")
	if len(m.Windows) != 1 {
		t.Fatalf("setup: expected 1 window, got %d", len(m.Windows))
	}

	want := config.FormatInitialTitle()
	if got := m.Windows[0].Title(); got != want {
		t.Errorf("Windows[0].Title() = %q, want %q (from initial_title_format)", got, want)
	}
}

// TestAddWindowAppliesLockTitles pins appearance.lock_titles reaching an
// actual created window's lock state, not just the config global.
func TestAddWindowAppliesLockTitles(t *testing.T) {
	prev := config.LockTitles
	config.LockTitles = true
	t.Cleanup(func() { config.LockTitles = prev })

	m := newStartupOS(t, false, false)
	defer closeWindows(m)

	m.AddWindow("")
	if len(m.Windows) != 1 {
		t.Fatalf("setup: expected 1 window, got %d", len(m.Windows))
	}
	if !m.Windows[0].TitleLocked() {
		t.Error("a window created with lock_titles=true was not title-locked")
	}
}

// TestAddWindowLeavesTitleUnlockedByDefault is the counterpart: lock_titles
// off (the default) must not lock windows behind the user's back.
func TestAddWindowLeavesTitleUnlockedByDefault(t *testing.T) {
	prev := config.LockTitles
	config.LockTitles = false
	t.Cleanup(func() { config.LockTitles = prev })

	m := newStartupOS(t, false, false)
	defer closeWindows(m)

	m.AddWindow("")
	if len(m.Windows) != 1 {
		t.Fatalf("setup: expected 1 window, got %d", len(m.Windows))
	}
	if m.Windows[0].TitleLocked() {
		t.Error("a window was title-locked with lock_titles=false")
	}
}
