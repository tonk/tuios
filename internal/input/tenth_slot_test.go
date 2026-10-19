package input

import "testing"

// TestTenthSlotHasHandlers checks that the actions the default config binds to
// 0 do something: a binding whose action has no handler is a key that silently
// does nothing.
func TestTenthSlotHasHandlers(t *testing.T) {
	d := NewActionDispatcher()
	for _, action := range []string{"select_window_10", "restore_minimized_10", "minimize_prefix_restore_10"} {
		if !d.HasAction(action) {
			t.Errorf("no handler for %s", action)
		}
	}
}
