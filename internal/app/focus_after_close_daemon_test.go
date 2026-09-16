package app

import (
	"testing"

	"github.com/tonk/tuios/internal/config"
)

// TestDaemonDeleteWindowMovesFocusImmediately guards the fix for a reported
// bug: closing the focused window in a daemon session (tuios attach, the only
// way this app is actually run) used to leave focus - and so keyboard input -
// sitting on the closing pane until the daemon's state sync came back and
// removed it. The pane's PTY was already gone by then, so it looked frozen
// and unresponsive until some unrelated action forced a render. DeleteWindow
// must move focus off the closing window synchronously, before any round
// trip to the daemon completes.
func TestDaemonDeleteWindowMovesFocusImmediately(t *testing.T) {
	prev := config.FocusAfterClose
	config.FocusAfterClose = "previous"
	t.Cleanup(func() { config.FocusAfterClose = prev })

	r := newRig(t, 7)
	r.attach()

	closing := r.win(6) // window 7
	want := r.win(5)    // window 6
	r.m.FocusedWindow = 6

	r.m.DeleteWindow(6)

	// No settle/converge wait: this must already be true the instant
	// DeleteWindow returns, before the daemon's CloseWindow round trip has
	// any chance to complete.
	got := r.m.GetFocusedWindow()
	if got == nil || got.ID != want.ID {
		t.Fatalf("focus immediately after DeleteWindow = %v, want window %s (window 7's neighbor, not %s)",
			got, want.ID, closing.ID)
	}
	if r.m.FocusedWindow == 6 || (r.m.FocusedWindow >= 0 && r.m.Windows[r.m.FocusedWindow].ID == closing.ID) {
		t.Fatalf("focus is still on the closing window %s", closing.ID)
	}
}
