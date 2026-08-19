package vt

import "testing"

// TestCSI_u_RestoresSavedCursor verifies that a bare CSI u (SCORC) restores
// the cursor position previously saved with CSI s (SCOSC). Ink's
// ansi-escapes package emits this exact pair when redrawing TUI output on
// non-Apple-Terminal platforms; a dropped CSI u leaves the cursor stranded
// wherever the following writes happened to move it.
func TestCSI_u_RestoresSavedCursor(t *testing.T) {
	e := NewEmulator(80, 24)

	e.WriteString("\x1b[5;10H") // move cursor to row 5, col 10 (1-indexed)
	e.WriteString("\x1b[s")     // save cursor position (SCOSC)
	e.WriteString("\x1b[20;1H") // move cursor elsewhere
	e.WriteString("some redrawn output")
	e.WriteString("\x1b[u") // restore cursor position (SCORC)

	pos := e.CursorPosition()
	wantX, wantY := 9, 4 // 0-indexed equivalent of row 5, col 10
	if pos.X != wantX || pos.Y != wantY {
		t.Fatalf("CursorPosition() after CSI u = (%d,%d), want (%d,%d)", pos.X, pos.Y, wantX, wantY)
	}
}
