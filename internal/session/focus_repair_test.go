package session

import (
	"testing"

	"github.com/tonk/tuios/internal/config"
)

// The focus-repair rule is what decides where focus lands after the focused
// window goes away. Two implementations answer that question today: the daemon's
// (CloseDaemonWindow) and the TUI's (OS.FocusNextVisibleWindow, which takes the
// first window in slice order on the current workspace that is neither minimized
// nor minimizing, and gives up with no focus when there is none).
//
// Converging close onto the daemon makes the daemon's rule the only rule, so it
// has to be the TUI's rule. This test states that rule case by case so a later
// change to it is a deliberate, visible edit rather than a silent shift in where
// the cursor ends up.
func TestDaemonFocusRepairAfterClose(t *testing.T) {
	type window struct {
		id        string
		workspace int
		minimized bool
	}

	cases := []struct {
		name string
		// windows in slice order, before the close.
		windows []window
		focused string
		// close is the ID of the window to remove.
		close string
		// wantFocus is the ID expected to hold focus afterwards; "" means no
		// window should be focused.
		wantFocus string
		// wantWorkspaceFocus is the expected WorkspaceFocus entry for the closed
		// window's workspace; "" means the entry must be absent.
		wantWorkspaceFocus string
		workspace          int
	}{
		{
			name:      "closing an unfocused window leaves focus alone",
			windows:   []window{{id: "a", workspace: 1}, {id: "b", workspace: 1}},
			focused:   "a",
			close:     "b",
			wantFocus: "a", wantWorkspaceFocus: "a", workspace: 1,
		},
		{
			name:      "closing the focused window falls to the next in slice order",
			windows:   []window{{id: "a", workspace: 1}, {id: "b", workspace: 1}},
			focused:   "a",
			close:     "a",
			wantFocus: "b", wantWorkspaceFocus: "b", workspace: 1,
		},
		{
			name:      "the only window on a workspace leaves nothing focused",
			windows:   []window{{id: "a", workspace: 1}},
			focused:   "a",
			close:     "a",
			wantFocus: "", wantWorkspaceFocus: "", workspace: 1,
		},
		{
			name:      "a window on another workspace is not a focus candidate",
			windows:   []window{{id: "a", workspace: 1}, {id: "b", workspace: 2}},
			focused:   "a",
			close:     "a",
			wantFocus: "", wantWorkspaceFocus: "", workspace: 1,
		},
		{
			// This is the case where the daemon used to disagree with the TUI: it
			// took the first remaining window on the workspace whether or not it
			// was minimized, so closing a window could focus one sitting in the
			// dock while a visible window went unfocused.
			name:      "a minimized window is skipped in favour of a visible one",
			windows:   []window{{id: "a", workspace: 1}, {id: "b", workspace: 1, minimized: true}, {id: "c", workspace: 1}},
			focused:   "a",
			close:     "a",
			wantFocus: "c", wantWorkspaceFocus: "c", workspace: 1,
		},
		{
			name:      "with only minimized windows left nothing is focused",
			windows:   []window{{id: "a", workspace: 1}, {id: "b", workspace: 1, minimized: true}},
			focused:   "a",
			close:     "a",
			wantFocus: "", wantWorkspaceFocus: "", workspace: 1,
		},
		{
			name:      "closing on a workspace that is not current repairs that workspace",
			windows:   []window{{id: "a", workspace: 1}, {id: "b", workspace: 2}, {id: "c", workspace: 2}},
			focused:   "b",
			close:     "b",
			wantFocus: "c", wantWorkspaceFocus: "c", workspace: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := newTestSession(t)

			// Build the window set directly: these cases are about the repair
			// rule, not about PTY lifecycle, so no PTYs are spawned.
			if err := sess.mutateState(func(state *SessionState) error {
				state.Windows = nil
				state.WorkspaceFocus = make(map[int]string)
				for _, w := range tc.windows {
					state.Windows = append(state.Windows, WindowState{
						ID:        w.id,
						Workspace: w.workspace,
						Minimized: w.minimized,
					})
					if w.id == tc.focused {
						state.CurrentWorkspace = w.workspace
					}
				}
				state.FocusedWindowID = tc.focused
				state.WorkspaceFocus[tc.workspace] = tc.focused
				return nil
			}); err != nil {
				t.Fatalf("seeding state failed: %v", err)
			}

			if _, err := sess.CloseDaemonWindow(tc.close); err != nil {
				t.Fatalf("CloseDaemonWindow(%q) failed: %v", tc.close, err)
			}

			state := sess.GetState()
			if state.FocusedWindowID != tc.wantFocus {
				t.Errorf("FocusedWindowID = %q, want %q", state.FocusedWindowID, tc.wantFocus)
			}
			got := state.WorkspaceFocus[tc.workspace]
			if got != tc.wantWorkspaceFocus {
				t.Errorf("WorkspaceFocus[%d] = %q, want %q", tc.workspace, got, tc.wantWorkspaceFocus)
			}
		})
	}
}

// TestCloseDaemonWindowFocusAfterClosePrevious confirms CloseDaemonWindow
// honours appearance.focus_after_close = "previous" - this is the daemon's
// own copy of the client's FocusPreviousVisibleWindow rule, and is the one
// that actually applies for a tuios attach session: DeleteWindow never picks
// focus itself there, it just asks the daemon to close the window.
func TestCloseDaemonWindowFocusAfterClosePrevious(t *testing.T) {
	prev := config.FocusAfterClose
	config.FocusAfterClose = "previous"
	t.Cleanup(func() { config.FocusAfterClose = prev })

	sess := newTestSession(t)
	if err := sess.mutateState(func(state *SessionState) error {
		state.Windows = []WindowState{
			{ID: "w1", Workspace: 1}, {ID: "w2", Workspace: 1}, {ID: "w3", Workspace: 1},
			{ID: "w4", Workspace: 1}, {ID: "w5", Workspace: 1}, {ID: "w6", Workspace: 1},
			{ID: "w7", Workspace: 1},
		}
		state.CurrentWorkspace = 1
		state.WorkspaceFocus = map[int]string{1: "w7"}
		state.FocusedWindowID = "w7"
		return nil
	}); err != nil {
		t.Fatalf("seeding state failed: %v", err)
	}

	if _, err := sess.CloseDaemonWindow("w7"); err != nil {
		t.Fatalf("CloseDaemonWindow: %v", err)
	}

	if got := sess.GetState().FocusedWindowID; got != "w6" {
		t.Errorf("FocusedWindowID after closing w7 = %q, want w6", got)
	}
}

// TestCloseDaemonWindowFocusAfterClosePreviousFallsBackToFirst confirms
// closing the first window under "previous" falls back to the new first
// window, since nothing precedes position 1.
func TestCloseDaemonWindowFocusAfterClosePreviousFallsBackToFirst(t *testing.T) {
	prev := config.FocusAfterClose
	config.FocusAfterClose = "previous"
	t.Cleanup(func() { config.FocusAfterClose = prev })

	sess := newTestSession(t)
	if err := sess.mutateState(func(state *SessionState) error {
		state.Windows = []WindowState{{ID: "w1", Workspace: 1}, {ID: "w2", Workspace: 1}, {ID: "w3", Workspace: 1}}
		state.CurrentWorkspace = 1
		state.WorkspaceFocus = map[int]string{1: "w1"}
		state.FocusedWindowID = "w1"
		return nil
	}); err != nil {
		t.Fatalf("seeding state failed: %v", err)
	}

	if _, err := sess.CloseDaemonWindow("w1"); err != nil {
		t.Fatalf("CloseDaemonWindow: %v", err)
	}

	if got := sess.GetState().FocusedWindowID; got != "w2" {
		t.Errorf("FocusedWindowID after closing w1 = %q, want w2", got)
	}
}
