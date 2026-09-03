package app

import (
	"testing"

	"github.com/tonk/tuios/internal/config"
)

func withCursorBlink(t *testing.T, v bool) {
	t.Helper()
	prev := config.CursorBlink
	config.CursorBlink = v
	t.Cleanup(func() { config.CursorBlink = prev })
}

func focusedCursor(t *testing.T, output string) *OS {
	t.Helper()
	win := newTestWindow(t, "cursor-blink", 80, 24)
	if output != "" {
		win.WriteOutput([]byte(output))
	}
	m := newTestOS(win)
	m.Mode = TerminalMode
	return m
}

// TestCursorBlinkDefaultsOn pins the host cursor blinking when no guest has
// sent DECSCUSR. The window's atomic blink flag defaults off, so without
// falling back to appearance.cursor_blink the cursor would sit steady.
func TestCursorBlinkDefaultsOn(t *testing.T) {
	withCursorBlink(t, true)
	m := focusedCursor(t, "prompt$ ")
	c := m.getRealCursor()
	if c == nil {
		t.Fatal("expected a cursor")
	}
	if !c.Blink {
		t.Error("cursor did not blink by default")
	}
}

// TestCursorBlinkFollowsConfigUntilDECSCUSR is the config off side: a pane
// whose guest never set a style must pick up appearance.cursor_blink live.
func TestCursorBlinkFollowsConfigUntilDECSCUSR(t *testing.T) {
	withCursorBlink(t, false)
	m := focusedCursor(t, "prompt$ ")
	c := m.getRealCursor()
	if c == nil {
		t.Fatal("expected a cursor")
	}
	if c.Blink {
		t.Error("cursor blinked despite appearance.cursor_blink = false")
	}
}

// TestCursorBlinkGuestDECSCUSROverridesConfig is why the default is only a
// default: CSI 1 q (blinking block) must win over cursor_blink = false, and
// CSI 2 q (steady block) must win over cursor_blink = true.
func TestCursorBlinkGuestDECSCUSROverridesConfig(t *testing.T) {
	withCursorBlink(t, false)
	m := focusedCursor(t, "prompt$ ")
	m.Windows[0].WriteOutput([]byte("\x1b[1 q"))
	c := m.getRealCursor()
	if c == nil {
		t.Fatal("expected a cursor after DECSCUSR 1")
	}
	if !c.Blink {
		t.Error("guest DECSCUSR 1 (blinking block) did not override cursor_blink = false")
	}

	config.CursorBlink = true
	m.Windows[0].WriteOutput([]byte("\x1b[2 q"))
	c = m.getRealCursor()
	if c == nil {
		t.Fatal("expected a cursor after DECSCUSR 2")
	}
	if c.Blink {
		t.Error("guest DECSCUSR 2 (steady block) left the cursor blinking")
	}
}
