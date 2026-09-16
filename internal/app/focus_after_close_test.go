package app

import (
	"fmt"
	"testing"

	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/terminal"
)

// floatingOSWithWindows builds n floating (non-tiled) windows on workspace 1,
// numbered window-0001..window-000n in the same order FocusWindowAtWorkspacePosition
// would report them (1-based, in m.Windows order).
func floatingOSWithWindows(n int) *OS {
	windows := make([]*terminal.Window, n)
	for i := range n {
		windows[i] = &terminal.Window{
			ID:        fmt.Sprintf("window-%04d", i+1),
			Workspace: 1,
			Width:     80,
			Height:    24,
		}
	}
	return &OS{
		NumWorkspaces:    9,
		CurrentWorkspace: 1,
		WorkspaceFocus:   make(map[int]int),
		Width:            200,
		Height:           60,
		Windows:          windows,
		FocusedWindow:    n - 1,
	}
}

// TestDeleteWindowFocusAfterCloseFirst confirms the default ("first") behavior
// is unchanged: closing the focused window jumps focus to the first
// (lowest-position) remaining visible window, regardless of which one closed.
func TestDeleteWindowFocusAfterCloseFirst(t *testing.T) {
	prev := config.FocusAfterClose
	config.FocusAfterClose = "first"
	t.Cleanup(func() { config.FocusAfterClose = prev })

	m := floatingOSWithWindows(7)
	m.DeleteWindow(6) // close window 7, which is focused

	if got := m.GetFocusedWindow(); got == nil || got.ID != "window-0001" {
		t.Fatalf("focus after closing window 7 = %v, want window-0001", got)
	}
}

// TestDeleteWindowFocusAfterClosePrevious confirms appearance.focus_after_close
// = "previous" moves focus to the window that sat one position before the
// one that closed, matching the user-requested "close 7, focus 6" behavior.
func TestDeleteWindowFocusAfterClosePrevious(t *testing.T) {
	prev := config.FocusAfterClose
	config.FocusAfterClose = "previous"
	t.Cleanup(func() { config.FocusAfterClose = prev })

	m := floatingOSWithWindows(7)
	m.DeleteWindow(6) // close window 7, which is focused

	if got := m.GetFocusedWindow(); got == nil || got.ID != "window-0006" {
		t.Fatalf("focus after closing window 7 = %v, want window-0006", got)
	}
}

// TestDeleteWindowFocusAfterClosePreviousFallsBackToFirst confirms that
// closing the first window under "previous" falls back to focusing the new
// first window, since nothing precedes position 1.
func TestDeleteWindowFocusAfterClosePreviousFallsBackToFirst(t *testing.T) {
	prev := config.FocusAfterClose
	config.FocusAfterClose = "previous"
	t.Cleanup(func() { config.FocusAfterClose = prev })

	m := floatingOSWithWindows(3)
	m.FocusedWindow = 0
	m.DeleteWindow(0) // close window 1, which is focused

	if got := m.GetFocusedWindow(); got == nil || got.ID != "window-0002" {
		t.Fatalf("focus after closing window 1 = %v, want window-0002", got)
	}
}
