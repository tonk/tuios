package vt

import (
	"github.com/charmbracelet/x/ansi"
	"reflect"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// These tests pin the print path, wide runes, grapheme clusters, insert mode,
// resize and scrollback against xterm and ghostty behaviour. Rows are compared
// cell by cell: a wide rune's continuation column reads as "".

// rowCells returns the content of every column of row y of the active screen.
func rowCells(e *Emulator, y int) []string {
	out := make([]string, e.Width())
	for x := range out {
		if c := e.CellAt(x, y); c != nil {
			out[x] = c.Content
		}
	}
	return out
}

// lineCells is rowCells for a scrollback line.
func lineCells(l uv.Line) []string {
	out := make([]string, len(l))
	for x := range l {
		out[x] = l[x].Content
	}
	return out
}

func checkRow(t *testing.T, e *Emulator, y int, want []string) {
	t.Helper()
	if got := rowCells(e, y); !reflect.DeepEqual(got, want) {
		t.Errorf("row %d = %q, want %q", y, got, want)
	}
}

type printCase struct {
	name   string
	w, h   int
	writes []string
	rows   map[int][]string
	cx, cy int
}

func runPrintCases(t *testing.T, cases []printCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEmulator(tc.w, tc.h)
			for _, s := range tc.writes {
				_, _ = e.WriteString(s)
			}
			for y, want := range tc.rows {
				checkRow(t, e, y, want)
			}
			checkCursor(t, e, tc.cx, tc.cy)
		})
	}
}

// Bug 1: a wide rune that ends exactly on the right edge sets the pending wrap.
func TestPrintWideRuneEndingOnTheEdgeWraps(t *testing.T) {
	runPrintCases(t, []printCase{
		{
			name: "wide in second-to-last column then narrow", w: 5, h: 3,
			writes: []string{"abc中X"},
			rows: map[int][]string{
				0: {"a", "b", "c", "中", ""},
				1: {"X", " ", " ", " ", " "},
			},
			cx: 1, cy: 1,
		},
		{
			name: "wide in second-to-last column waits on the last column", w: 5, h: 3,
			writes: []string{"abc中"},
			rows:   map[int][]string{0: {"a", "b", "c", "中", ""}},
			cx:     4, cy: 0,
		},
		{
			name: "wide rune pair filling a 4-column line", w: 4, h: 2,
			writes: []string{"中中中"},
			rows: map[int][]string{
				0: {"中", "", "中", ""},
				1: {"中", "", " ", " "},
			},
			cx: 2, cy: 1,
		},
	})
}

// Bug 2: a wide rune that does not fit in the last column wraps whole, or is
// not printed at all when auto-wrap is off (xterm, ghostty).
func TestPrintWideRuneInTheLastColumn(t *testing.T) {
	runPrintCases(t, []printCase{
		{
			name: "wraps whole to the next line", w: 5, h: 3,
			writes: []string{"abcd中"},
			rows: map[int][]string{
				0: {"a", "b", "c", "d", " "},
				1: {"中", "", " ", " ", " "},
			},
			cx: 2, cy: 1,
		},
		{
			name: "emoji with VS16 in one write", w: 5, h: 3,
			writes: []string{"abcd❤️Z"},
			rows: map[int][]string{
				0: {"a", "b", "c", "d", " "},
				1: {"❤️", "", "Z", " ", " "},
			},
			cx: 3, cy: 1,
		},
		{
			name: "open cluster widened at the edge by the next write", w: 5, h: 3,
			writes: []string{"abcd❤", "️Z"},
			rows: map[int][]string{
				0: {"a", "b", "c", "d", " "},
				1: {"❤️", "", "Z", " ", " "},
			},
			cx: 3, cy: 1,
		},
		{
			name: "no auto-wrap: wide rune not printed, cursor stays", w: 5, h: 3,
			writes: []string{"\x1b[?7labcd中"},
			rows:   map[int][]string{0: {"a", "b", "c", "d", " "}},
			cx:     4, cy: 0,
		},
		{
			name: "no auto-wrap: narrow rune still overwrites the last column", w: 5, h: 3,
			writes: []string{"\x1b[?7labcd中Z"},
			rows:   map[int][]string{0: {"a", "b", "c", "d", "Z"}},
			cx:     4, cy: 0,
		},
		{
			name: "wide rune on a one-column screen is dropped", w: 1, h: 2,
			writes: []string{"中"},
			rows:   map[int][]string{0: {" "}},
			cx:     0, cy: 0,
		},
	})
}

// Bug 3: a combining mark after an ASCII base joins that base's cell.
func TestPrintCombiningMarkJoinsASCIIBase(t *testing.T) {
	runPrintCases(t, []printCase{
		{
			name: "e + U+0301", w: 6, h: 2,
			writes: []string{"café"},
			rows:   map[int][]string{0: {"c", "a", "f", "é", " ", " "}},
			cx:     4, cy: 0,
		},
		{
			name: "mark split across writes", w: 6, h: 2,
			writes: []string{"cafe", "́!"},
			rows:   map[int][]string{0: {"c", "a", "f", "é", "!", " "}},
			cx:     5, cy: 0,
		},
		{
			name: "two marks", w: 6, h: 2,
			writes: []string{"á̂b"},
			rows:   map[int][]string{0: {"á̂", "b", " ", " ", " ", " "}},
			cx:     2, cy: 0,
		},
		{
			name: "keycap", w: 6, h: 2,
			writes: []string{"1️⃣x"},
			rows:   map[int][]string{0: {"1️⃣", "", "x", " ", " ", " "}},
			cx:     3, cy: 0,
		},
		{
			name: "mark in the last column keeps the pending wrap", w: 5, h: 3,
			writes: []string{"abcdé"},
			rows:   map[int][]string{0: {"a", "b", "c", "d", "é"}},
			cx:     4, cy: 0,
		},
		{
			name: "mark in the last column then a print wraps", w: 5, h: 3,
			writes: []string{"abcdéX"},
			rows: map[int][]string{
				0: {"a", "b", "c", "d", "é"},
				1: {"X", " ", " ", " ", " "},
			},
			cx: 1, cy: 1,
		},
		{
			name: "mark after the cursor moved away does not attach", w: 5, h: 2,
			writes: []string{"ab\ŕ"},
			rows:   map[int][]string{0: {"a", "b", " ", " ", " "}},
			cx:     0, cy: 0,
		},
	})

	t.Run("joined cell is one column wide", func(t *testing.T) {
		e := NewEmulator(6, 2)
		_, _ = e.WriteString("café")
		if c := e.CellAt(3, 0); c == nil || c.Width != 1 {
			t.Fatalf("cell = %+v, want width 1", c)
		}
	})
}

// Bug 4: a wide rune written over the lead of another leaves no orphaned
// continuation; the broken half becomes an erase cell in the current pen's
// background, not the old rune's style.
func TestPrintWideOverWideRepairsTheCutRune(t *testing.T) {
	runPrintCases(t, []printCase{
		{
			name: "wide over the second half of one and the lead of the next", w: 6, h: 2,
			writes: []string{"中中中\ra世"},
			rows:   map[int][]string{0: {"a", "世", "", " ", "中", ""}},
			cx:     3, cy: 0,
		},
		{
			name: "narrow over a continuation", w: 6, h: 2,
			writes: []string{"中中中\x1b[1;2HX"},
			rows:   map[int][]string{0: {" ", "X", "中", "", "中", ""}},
			cx:     2, cy: 0,
		},
	})

	t.Run("broken half takes the erase background", func(t *testing.T) {
		e := NewEmulator(6, 2)
		_, _ = e.WriteString("\x1b[42m中中中\r\x1b[41ma世")
		c := e.CellAt(3, 0)
		if c == nil || c.Content != " " || c.Width != 1 {
			t.Fatalf("cell 3 = %+v, want a blank", c)
		}
		red := e.CellAt(0, 0).Style.Bg
		if red == nil || !reflect.DeepEqual(c.Style.Bg, red) {
			t.Errorf("cell 3 background = %v, want the current pen's %v", c.Style.Bg, red)
		}
	})
}

// Bug 5: REP repeats the last printed cluster, whatever its size or width.
func TestRepeatRepeatsTheLastCluster(t *testing.T) {
	runPrintCases(t, []printCase{
		{
			name: "box drawing", w: 6, h: 2,
			writes: []string{"a─\x1b[3b"},
			rows:   map[int][]string{0: {"a", "─", "─", "─", "─", " "}},
			cx:     5, cy: 0,
		},
		{
			name: "CJK", w: 8, h: 2,
			writes: []string{"中\x1b[2b"},
			rows:   map[int][]string{0: {"中", "", "中", "", "中", "", " ", " "}},
			cx:     6, cy: 0,
		},
		{
			name: "combined cluster", w: 6, h: 2,
			writes: []string{"é\x1b[2b"},
			rows:   map[int][]string{0: {"é", "é", "é", " ", " ", " "}},
			cx:     3, cy: 0,
		},
		{
			name: "ASCII", w: 6, h: 2,
			writes: []string{"x\x1b[3b"},
			rows:   map[int][]string{0: {"x", "x", "x", "x", " ", " "}},
			cx:     4, cy: 0,
		},
	})
}

// Bug 6: IRM (CSI 4 h) shifts the rest of the line right before printing.
func TestPrintInsertMode(t *testing.T) {
	runPrintCases(t, []printCase{
		{
			name: "wide rune inserted at the start", w: 5, h: 2,
			writes: []string{"\x1b[4habcde\r中"},
			rows:   map[int][]string{0: {"中", "", "a", "b", "c"}},
			cx:     2, cy: 0,
		},
		{
			name: "narrow insert in the middle", w: 6, h: 2,
			writes: []string{"abcd\x1b[1;2H\x1b[4hXY"},
			rows:   map[int][]string{0: {"a", "X", "Y", "b", "c", "d"}},
			cx:     3, cy: 0,
		},
		{
			name: "reset returns to replace", w: 5, h: 2,
			writes: []string{"\x1b[4h\x1b[4labc\rX"},
			rows:   map[int][]string{0: {"X", "b", "c", " ", " "}},
			cx:     1, cy: 0,
		},
	})

	t.Run("DECRQM reports it", func(t *testing.T) {
		e := NewEmulator(5, 2)
		_, _ = e.WriteString("\x1b[4h")
		if !e.isModeSet(ansi.ModeInsertReplace) {
			t.Fatal("IRM not recorded as set")
		}
	})
}

// Bugs 7, 8 and 9: a resize scrolls and clamps both screens, including saved
// cursors, and its auto-scroll ignores the guest's DECSTBM region.
func TestResizeKeepsBothScreensConsistent(t *testing.T) {
	t.Run("height shrink while on the alt screen keeps the main prompt", func(t *testing.T) {
		e := NewEmulator(10, 10)
		_, _ = e.WriteString("top\x1b[10;1H$ ")
		_, _ = e.WriteString("\x1b[?1049h")
		e.Resize(10, 5)
		_, _ = e.WriteString("\x1b[?1049lX")
		checkRow(t, e, 4, []string{"$", " ", "X", " ", " ", " ", " ", " ", " ", " "})
		checkCursor(t, e, 3, 4)
		if n := e.ScrollbackLen(); n != 5 {
			t.Errorf("scrollback holds %d lines, want 5", n)
		}
		if got := lineCells(e.ScrollbackLine(0)); len(got) < 3 || !reflect.DeepEqual(got[:3], []string{"t", "o", "p"}) {
			t.Errorf("oldest scrollback line starts %q, want top", got)
		}
	})

	t.Run("width shrink while on the alt screen clamps the main cursor", func(t *testing.T) {
		e := NewEmulator(20, 10)
		_, _ = e.WriteString("\x1b[1;20H\x1b[?1049h")
		e.Resize(8, 10)
		_, _ = e.WriteString("\x1b[?1049l")
		checkCursor(t, e, 7, 0)
		_, _ = e.WriteString("X")
		if c := e.CellAt(7, 0); c == nil || c.Content != "X" {
			t.Errorf("cell (7,0) = %+v, want X", c)
		}
	})

	t.Run("saved cursor is clamped", func(t *testing.T) {
		e := NewEmulator(20, 10)
		_, _ = e.WriteString("\x1b[9;18H\x1b7")
		e.Resize(10, 5)
		_, _ = e.WriteString("\x1b8")
		checkCursor(t, e, 9, 4)
		_, _ = e.WriteString("XY")
		if c := e.CellAt(9, 3); c == nil || c.Content != "X" {
			t.Errorf("cell (9,3) = %+v, want X", c)
		}
		if c := e.CellAt(0, 4); c == nil || c.Content != "Y" {
			t.Errorf("cell (0,4) = %+v, want Y", c)
		}
	})

	t.Run("saved cursor is clamped without a scroll", func(t *testing.T) {
		e := NewEmulator(20, 10)
		_, _ = e.WriteString("\x1b[2;18H\x1b7")
		e.Resize(10, 10)
		_, _ = e.WriteString("\x1b8")
		checkCursor(t, e, 9, 1)
	})

	t.Run("auto-scroll ignores DECSTBM and resets the margins", func(t *testing.T) {
		e := NewEmulator(10, 6)
		_, _ = e.WriteString("top\x1b[2;5r\x1b[6;1H$")
		e.Resize(10, 3)
		checkRow(t, e, 2, []string{"$", " ", " ", " ", " ", " ", " ", " ", " ", " "})
		checkCursor(t, e, 1, 2)
		if n := e.ScrollbackLen(); n != 3 {
			t.Errorf("scrollback holds %d lines, want 3", n)
		}
		if r := e.ScrollRegion(); r != e.Bounds() {
			t.Errorf("scroll region = %v, want the full screen %v", r, e.Bounds())
		}
	})
}

// Bug 10: pending wrap across a resize follows ghostty's Screen.resize.
func TestResizePendingWrap(t *testing.T) {
	t.Run("grow steps past the last character", func(t *testing.T) {
		e := NewEmulator(5, 3)
		_, _ = e.WriteString("abcde")
		e.Resize(8, 3)
		_, _ = e.WriteString("X")
		checkRow(t, e, 0, []string{"a", "b", "c", "d", "e", "X", " ", " "})
		checkCursor(t, e, 6, 0)
	})
	t.Run("height-only resize keeps the pending wrap", func(t *testing.T) {
		e := NewEmulator(5, 3)
		_, _ = e.WriteString("abcde")
		e.Resize(5, 4)
		_, _ = e.WriteString("X")
		checkRow(t, e, 0, []string{"a", "b", "c", "d", "e"})
		checkRow(t, e, 1, []string{"X", " ", " ", " ", " "})
	})
}

// Bug 11: a narrowing resize blanks wide runes it cuts in the scrollback too.
func TestResizeBlanksWideRunesCutInScrollback(t *testing.T) {
	e := NewEmulator(6, 2)
	_, _ = e.WriteString("abcd中\r\n\r\n\r\n")
	if e.ScrollbackLen() == 0 {
		t.Fatal("nothing reached the scrollback")
	}
	e.Resize(5, 2)
	line := e.ScrollbackLine(0)
	width := 0
	for x := 0; x < 5 && x < len(line); x++ {
		if line[x].Width > 0 {
			width += line[x].Width
		}
	}
	if width > 5 {
		t.Errorf("first five columns of the scrollback line span %d cells: %q", width, lineCells(line))
	}
	if got := lineCells(line)[:5]; !reflect.DeepEqual(got, []string{"a", "b", "c", "d", " "}) {
		t.Errorf("scrollback line = %q", got)
	}
}
