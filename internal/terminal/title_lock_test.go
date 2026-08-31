package terminal

import "testing"

// TestTitleLock verifies that OSC 0/2 title changes from the guest are
// applied normally, then dropped once SetTitleLocked(true) is set, and
// resume once unlocked again.
func TestTitleLock(t *testing.T) {
	exitChan := make(chan string, 1)
	window := NewWindow("test-titlelock01", "Test", 0, 0, 80, 24, 0, exitChan, nil)
	if window == nil {
		t.Skip("Failed to create window with PTY")
	}
	defer window.Close()

	osc := func(title string) []byte {
		return []byte("\x1b]2;" + title + "\a")
	}

	window.WriteOutput(osc("first-title"))
	if got := window.Title(); got != "first-title" {
		t.Fatalf("before lock: Title() = %q, want %q", got, "first-title")
	}

	if window.TitleLocked() {
		t.Fatalf("new window should not start title-locked")
	}
	window.SetTitleLocked(true)
	if !window.TitleLocked() {
		t.Fatalf("SetTitleLocked(true) did not stick")
	}

	window.WriteOutput(osc("second-title"))
	if got := window.Title(); got != "first-title" {
		t.Fatalf("while locked: Title() = %q, want unchanged %q", got, "first-title")
	}

	window.SetTitleLocked(false)
	window.WriteOutput(osc("third-title"))
	if got := window.Title(); got != "third-title" {
		t.Fatalf("after unlock: Title() = %q, want %q", got, "third-title")
	}
}
