package app

import (
	"strings"
	"testing"

	"github.com/tonk/tuios/internal/terminal"
)

// TestSelectionHighlightSkipsBlankRows pins a bug where a blank row inside a
// selection was highlighted across the pane's full width: the renderer clamps
// each selected row to its last non-blank cell, but started that search from
// the right edge and kept it when the row held nothing.
func TestSelectionHighlightSkipsBlankRows(t *testing.T) {
	win := newTestWindow(t, "sel-blank-0001", 40, 6)
	m := newTestOS(win)

	win.LockIO()
	_, _ = win.Terminal.Write([]byte("first\r\n\r\nthird"))
	win.UnlockIO()

	win.EnterCopyMode()
	win.CopyMode.State = terminal.CopyModeVisualChar
	win.CopyMode.VisualStart = terminal.Position{X: 0, Y: 0}
	win.CopyMode.VisualEnd = terminal.Position{X: 30, Y: 2}
	win.CopyMode.CursorX, win.CopyMode.CursorY = 4, 2
	win.MarkContentDirty()

	visualPref, _ := styleToANSI(visualSelectionStyle())
	rows := strings.Split(m.renderTerminal(win, true, false), "\n")
	if len(rows) < 3 {
		t.Fatalf("render produced %d rows, want at least 3", len(rows))
	}

	for i, wantHighlight := range []bool{true, false, true} {
		if got := strings.Contains(rows[i], visualPref); got != wantHighlight {
			t.Errorf("row %d highlighted = %v, want %v: %q", i, got, wantHighlight, rows[i])
		}
	}
}
