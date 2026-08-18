package vt

import (
	"testing"
)

// TestOSC66_WritesTextIntoCells is the OpenTUI/Cursor path: OSC 66 carries the
// glyphs, so consuming the sequence without painting them leaves a blank hole.
func TestOSC66_WritesTextIntoCells(t *testing.T) {
	e := NewEmulator(20, 5)
	defer e.Close()

	e.Write([]byte("\x1b]66;;Hello\a"))

	if got := rowRunes(e, 0); got != "Hello" {
		t.Errorf("row = %q, want %q", got, "Hello")
	}
	cur := e.CursorPosition()
	if cur.X != 5 || cur.Y != 0 {
		t.Errorf("cursor = (%d,%d), want (5,0)", cur.X, cur.Y)
	}
}

// TestOSC66_DoesNotWipeTheRestOfTheRow pins the defect that made Cursor's
// agent transcript look like Swiss cheese: each OSC 66 used to SetCell(nil)
// across the entire row, so only the last character on a line survived.
func TestOSC66_DoesNotWipeTheRestOfTheRow(t *testing.T) {
	e := NewEmulator(20, 5)
	defer e.Close()

	e.WriteString("AAAAAAAA")
	e.Write([]byte("\x1b[H\x1b]66;w=1;H\a\x1b]66;w=1;i\a"))

	if got := rowRunes(e, 0); got != "HiAAAAAA" {
		t.Errorf("row = %q, want %q", got, "HiAAAAAA")
	}
}

// TestOSC66_ExplicitWidthProbe is kitty's CPR detection sequence: a space
// drawn at w=2 must advance the cursor two cells, or apps conclude the
// terminal does not support explicit width and then also, if the sequence was
// consumed, that it is safe to keep emitting OSC 66.
func TestOSC66_ExplicitWidthProbe(t *testing.T) {
	e := NewEmulator(20, 5)
	defer e.Close()

	e.Write([]byte("\x1b]66;w=2; \a"))

	cur := e.CursorPosition()
	if cur.X != 2 || cur.Y != 0 {
		t.Errorf("cursor = (%d,%d), want (2,0)", cur.X, cur.Y)
	}
}

// TestOSC66_ScaleProbe is the matching s=2 probe: a space occupies a 2x2
// block, so the cursor moves two cells on the same row.
func TestOSC66_ScaleProbe(t *testing.T) {
	e := NewEmulator(20, 5)
	defer e.Close()

	e.Write([]byte("\x1b]66;s=2; \a"))

	cur := e.CursorPosition()
	if cur.X != 2 || cur.Y != 0 {
		t.Errorf("cursor = (%d,%d), want (2,0)", cur.X, cur.Y)
	}
}

// TestOSC66_PerCellCUP is how OpenTUI lays out a line: absolute position then
// one OSC 66 cell. Neighbours must remain, which is exactly what wiping the
// row destroyed.
func TestOSC66_PerCellCUP(t *testing.T) {
	e := NewEmulator(20, 5)
	defer e.Close()

	e.Write([]byte("\x1b[1;1H\x1b]66;w=1;N\a"))
	e.Write([]byte("\x1b[1;2H\x1b]66;w=1;o\a"))
	e.Write([]byte("\x1b[1;3H\x1b]66;w=1;.\a"))

	if got := rowRunes(e, 0); got != "No." {
		t.Errorf("row = %q, want %q", got, "No.")
	}
}

func TestParseTextSizingMeta(t *testing.T) {
	scale, width := parseTextSizingMeta([]byte("s=2:w=1"))
	if scale != 2 || width != 1 {
		t.Errorf("s=2:w=1 → (%d,%d), want (2,1)", scale, width)
	}
	scale, width = parseTextSizingMeta([]byte(""))
	if scale != 1 || width != 0 {
		t.Errorf("empty → (%d,%d), want (1,0)", scale, width)
	}
	scale, width = parseTextSizingMeta([]byte("n=1:d=2:w=1"))
	if scale != 1 || width != 1 {
		t.Errorf("n=1:d=2:w=1 → (%d,%d), want (1,1)", scale, width)
	}
}
