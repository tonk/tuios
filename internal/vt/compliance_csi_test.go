package vt

import (
	"image/color"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Regression tests for VT compliance fixes in the CSI, ESC and control
// handlers. Expected values follow xterm.

// feedEmulator returns a w x h emulator that has been fed every chunk in turn,
// one Write per chunk, so a test can split a sequence across writes.
func feedEmulator(t *testing.T, w, h int, chunks ...string) *Emulator {
	t.Helper()
	e := NewEmulator(w, h)
	for _, c := range chunks {
		if _, err := e.WriteString(c); err != nil {
			t.Fatalf("WriteString(%q): %v", c, err)
		}
	}
	return e
}

// complianceRow returns row y of the active screen as text, a blank cell as a
// space and the continuation half of a wide rune as nothing.
func complianceRow(e *Emulator, y int) string {
	var b strings.Builder
	for x := range e.Width() {
		c := e.CellAt(x, y)
		switch {
		case c == nil || c.Content == "":
			if c == nil || c.Width != 0 {
				b.WriteByte(' ')
			}
		default:
			b.WriteString(c.Content)
		}
	}
	return b.String()
}

// readResponse drains what the emulator answered. The response pipe buffers,
// so a reply to a query written beforehand is already there.
func readResponse(t *testing.T, e *Emulator) string {
	t.Helper()
	buf := make([]byte, 256)
	n, err := e.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return string(buf[:n])
}

func checkCursor(t *testing.T, e *Emulator, wantX, wantY int) {
	t.Helper()
	if pos := e.CursorPosition(); pos.X != wantX || pos.Y != wantY {
		t.Errorf("cursor = (%d,%d), want (%d,%d)", pos.X, pos.Y, wantX, wantY)
	}
}

func checkRows(t *testing.T, e *Emulator, want []string) {
	t.Helper()
	for y, w := range want {
		if got := complianceRow(e, y); got != w {
			t.Errorf("row %d = %q, want %q", y, got, w)
		}
	}
}

func TestComplianceMarginClamp(t *testing.T) {
	tests := []struct {
		name       string
		seq        string
		wantRegion uv.Rectangle
	}{
		{"DECSTBM bottom past screen then IL", "\x1b[1;100r\x1b[L", uv.Rect(0, 0, 10, 5)},
		{"DECSTBM bottom past screen then DL", "\x1b[1;100r\x1b[M", uv.Rect(0, 0, 10, 5)},
		{"DECSTBM bottom past screen then SU", "\x1b[1;100r\x1b[S", uv.Rect(0, 0, 10, 5)},
		{"DECSTBM bottom past screen then SD", "\x1b[1;100r\x1b[T", uv.Rect(0, 0, 10, 5)},
		{"DECSTBM top inside, bottom past", "\x1b[2;100r\x1b[2;1H\x1b[L\x1b[M", uv.Rect(0, 1, 10, 4)},
		{"DECSLRM right past screen then ICH", "\x1b[?69h\x1b[1;100s\x1b[@", uv.Rect(0, 0, 10, 5)},
		{"DECSLRM right past screen then DCH", "\x1b[?69h\x1b[1;100s\x1b[P", uv.Rect(0, 0, 10, 5)},
		{"DECSLRM right past screen then IL", "\x1b[?69h\x1b[1;100s\x1b[L", uv.Rect(0, 0, 10, 5)},
		{"DECSLRM left inside, right past", "\x1b[?69h\x1b[3;100s\x1b[1;5H\x1b[@\x1b[P", uv.Rect(2, 0, 8, 5)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := feedEmulator(t, 10, 5, "abcdefghij", tt.seq)
			if got := e.ScrollRegion(); got != tt.wantRegion {
				t.Errorf("ScrollRegion() = %v, want %v", got, tt.wantRegion)
			}
		})
	}

	t.Run("region assigned past the screen never panics", func(t *testing.T) {
		e := feedEmulator(t, 10, 5, "abcdefghij")
		e.scr.scroll = uv.Rect(0, 0, 100, 100)
		_, _ = e.WriteString("\x1b[L\x1b[M\x1b[S\x1b[T\x1b[@\x1b[P\x1b[3;3H\x1b[L\x1b[M\x1b[@\x1b[P")
	})
}

func TestComplianceZeroCountParam(t *testing.T) {
	// Cursor starts at (4,4) of a 20x10 screen.
	moves := []struct {
		name         string
		seq          string
		wantX, wantY int
	}{
		{"CUU", "\x1b[0A", 4, 3},
		{"CUD", "\x1b[0B", 4, 5},
		{"CUF", "\x1b[0C", 5, 4},
		{"CUB", "\x1b[0D", 3, 4},
		{"CNL", "\x1b[0E", 0, 5},
		{"CPL", "\x1b[0F", 0, 3},
		{"HPR", "\x1b[0a", 5, 4},
		{"VPR", "\x1b[0e", 4, 5},
		{"CHT", "\x1b[0I", 8, 4},
		{"CBT", "\x1b[1;11H\x1b[0Z", 8, 0},
	}
	for _, tt := range moves {
		t.Run(tt.name, func(t *testing.T) {
			e := feedEmulator(t, 20, 10, "\x1b[5;5H", tt.seq)
			checkCursor(t, e, tt.wantX, tt.wantY)
		})
	}

	edits := []struct {
		name string
		seq  string
		want []string
	}{
		{"ICH", "\x1b[1;1H\x1b[0@", []string{" abcd", "efghi", "jklmn"}},
		{"DCH", "\x1b[1;1H\x1b[0P", []string{"bcd  ", "efghi", "jklmn"}},
		{"IL", "\x1b[1;1H\x1b[0L", []string{"     ", "abcd ", "efghi"}},
		{"DL", "\x1b[1;1H\x1b[0M", []string{"efghi", "jklmn", "     "}},
		{"SU", "\x1b[0S", []string{"efghi", "jklmn", "     "}},
		{"SD", "\x1b[0T", []string{"     ", "abcd ", "efghi"}},
		{"REP", "\x1b[3;4Hz\x1b[0b", []string{"abcd ", "efghi", "jklzz"}},
	}
	for _, tt := range edits {
		t.Run(tt.name, func(t *testing.T) {
			e := feedEmulator(t, 5, 3, "abcd\r\nefghi\r\njklmn", tt.seq)
			checkRows(t, e, tt.want)
		})
	}

	t.Run("REP 0 repeats once", func(t *testing.T) {
		e := feedEmulator(t, 10, 2, "a\x1b[0b")
		checkRows(t, e, []string{"aa        "})
	})
}

func TestComplianceShiftOutShiftIn(t *testing.T) {
	tests := []struct {
		name string
		seq  string
		want string
	}{
		{"SO invokes G1", "\x1b)0\x0eq", "─    "},
		{"SI invokes G0 again", "\x1b)0\x0eq\x0fq", "─q   "},
		{"SO with G1 ASCII", "\x0eq", "q    "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := feedEmulator(t, 5, 1, tt.seq)
			checkRows(t, e, []string{tt.want})
		})
	}
}

func TestCompliancePendingWrapCleared(t *testing.T) {
	// Each case leaves a wrap pending by filling a line of a 10x5 screen, sends
	// the sequence under test, then prints X. A wrap still pending would put X
	// at the start of the next line.
	const fill = "0123456789"
	tests := []struct {
		name         string
		setup        string
		seq          string
		wantX, wantY int
	}{
		{"RI", "\x1b[3;1H" + fill, "\x1bM", 9, 1},
		{"EL 1", "\x1b[3;1H" + fill, "\x1b[1K", 9, 2},
		{"EL 2", "\x1b[3;1H" + fill, "\x1b[2K", 9, 2},
		{"ED 0", "\x1b[3;1H" + fill, "\x1b[0J", 9, 2},
		{"ED 1", "\x1b[3;1H" + fill, "\x1b[1J", 9, 2},
		{"ED 2", "\x1b[3;1H" + fill, "\x1b[2J", 9, 2},
		{"ED 3", "\x1b[3;1H" + fill, "\x1b[3J", 9, 2},
		{"ICH", "\x1b[3;1H" + fill, "\x1b[@", 9, 2},
		{"DCH", "\x1b[3;1H" + fill, "\x1b[P", 9, 2},
		{"IL", "\x1b[3;1H" + fill, "\x1b[L", 0, 2},
		{"DL", "\x1b[3;1H" + fill, "\x1b[M", 0, 2},
		{"DECRC of a save without wrap", "\x1b[1;10H\x1b7\x1b[3;1H" + fill, "\x1b8", 9, 0},
		{"SCORC of a save without wrap", "\x1b[1;10H\x1b[s\x1b[3;1H" + fill, "\x1b[u", 9, 0},
		{"DECRC restores a pending wrap", "\x1b[3;1H" + fill + "\x1b7\x1b[1;1H", "\x1b8", 0, 3},
		{"SCORC restores a pending wrap", "\x1b[3;1H" + fill + "\x1b[s\x1b[1;1H", "\x1b[u", 0, 3},
		{"leaving 47 drops the alt screen's wrap", "\x1b[1;1H\x1b[?47h\x1b[3;1H" + fill, "\x1b[?47l", 0, 0},
		{"leaving 1047 drops the alt screen's wrap", "\x1b[1;1H\x1b[?1047h\x1b[3;1H" + fill, "\x1b[?1047l", 0, 0},
		{"leaving 1049 drops the alt screen's wrap", "\x1b[1;1H\x1b[?1049h\x1b[3;1H" + fill, "\x1b[?1049l", 0, 0},
		{"leaving 1049 restores the saved wrap", "\x1b[3;1H" + fill + "\x1b[?1049h\x1b[1;1H", "\x1b[?1049l", 0, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := feedEmulator(t, 10, 5, tt.setup, tt.seq, "X")
			if c := e.CellAt(tt.wantX, tt.wantY); c == nil || c.Content != "X" {
				t.Errorf("X not at (%d,%d); rows:\n%s", tt.wantX, tt.wantY, strings.Join([]string{
					complianceRow(e, 0), complianceRow(e, 1), complianceRow(e, 2),
					complianceRow(e, 3), complianceRow(e, 4),
				}, "\n"))
			}
		})
	}
}

func TestComplianceSaveRestoreCursorState(t *testing.T) {
	t.Run("charset designation", func(t *testing.T) {
		e := feedEmulator(t, 5, 1, "\x1b(0\x1b7\x1b(B\x1b8q")
		checkRows(t, e, []string{"─    "})
	})
	t.Run("GL invocation", func(t *testing.T) {
		e := feedEmulator(t, 5, 1, "\x1b)0\x0e\x1b7\x0f\x1b8q")
		checkRows(t, e, []string{"─    "})
	})
	t.Run("single shift", func(t *testing.T) {
		e := feedEmulator(t, 5, 1, "\x1b*0\x1bN\x1b7\x1b8qq")
		checkRows(t, e, []string{"─q   "})
	})
	t.Run("restore without save resets charsets", func(t *testing.T) {
		e := feedEmulator(t, 5, 1, "\x1b(0\x1b8q")
		checkRows(t, e, []string{"q    "})
	})
	t.Run("origin mode", func(t *testing.T) {
		e := feedEmulator(t, 10, 10, "\x1b[5;8r\x1b[?6h\x1b7\x1b[?6l\x1b8\x1b[H")
		if !e.isModeSet(ansi.ModeOrigin) {
			t.Error("DECRC did not restore DECOM")
		}
		checkCursor(t, e, 0, 4)
	})
	t.Run("origin mode reset by restore", func(t *testing.T) {
		e := feedEmulator(t, 10, 10, "\x1b[5;8r\x1b7\x1b[?6h\x1b8\x1b[H")
		if e.isModeSet(ansi.ModeOrigin) {
			t.Error("DECRC did not restore DECOM off")
		}
		checkCursor(t, e, 0, 0)
	})
	t.Run("cursor visibility is not restored", func(t *testing.T) {
		e := feedEmulator(t, 10, 2, "\x1b7\x1b[?25l\x1b8")
		if !e.IsCursorHidden() {
			t.Error("DECRC showed a cursor hidden after DECSC")
		}
		e = feedEmulator(t, 10, 2, "\x1b[?25l\x1b7\x1b[?25h\x1b8")
		if e.IsCursorHidden() {
			t.Error("DECRC hid a cursor shown after DECSC")
		}
	})
	t.Run("cursor style is not restored", func(t *testing.T) {
		e := feedEmulator(t, 10, 2, "\x1b7\x1b[4 q\x1b8")
		if cur := e.scr.Cursor(); cur.Style != CursorUnderline || !cur.Steady {
			t.Errorf("cursor style = %v steady=%v, want steady underline", cur.Style, cur.Steady)
		}
	})
	t.Run("position and pen", func(t *testing.T) {
		e := feedEmulator(t, 10, 5, "\x1b[3;4H\x1b[31m\x1b7\x1b[H\x1b[0m\x1b8X")
		checkCursor(t, e, 4, 2)
		if c := e.CellAt(3, 2); c == nil || c.Content != "X" || c.Style.Fg == nil {
			t.Errorf("cell (3,2) = %+v, want red X", c)
		}
	})
}

func TestComplianceOriginRelativePositioning(t *testing.T) {
	// Margins rows 5..8 (1-based), so origin row 0 is screen row 4.
	const origin = "\x1b[5;8r\x1b[?6h"
	tests := []struct {
		name         string
		seq          string
		wantX, wantY int
	}{
		{"VPA", "\x1b[3d", 0, 6},
		{"VPA keeps column", "\x1b[1;4H\x1b[3d", 3, 6},
		{"VPA clamps to bottom margin", "\x1b[20d", 0, 7},
		{"HPA keeps row", "\x1b[2;1H\x1b[5`", 4, 5},
		{"HPR keeps row", "\x1b[2;1H\x1b[3a", 3, 5},
		{"VPR", "\x1b[2;3H\x1b[1e", 2, 6},
		{"VPR clamps to bottom margin", "\x1b[9e", 0, 7},
		{"HVP is CUP", "\x1b[2;3f", 2, 5},
		{"HVP clamps like CUP", "\x1b[99;99f", 9, 7},
		{"HVP zero is home", "\x1b[0;0f", 0, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := feedEmulator(t, 10, 10, origin, tt.seq)
			checkCursor(t, e, tt.wantX, tt.wantY)
		})
	}

	t.Run("HVP without origin mode", func(t *testing.T) {
		e := feedEmulator(t, 10, 10, "\x1b[5;8r\x1b[2;3f")
		checkCursor(t, e, 2, 1)
	})
}

func TestComplianceCursorVerticalMargins(t *testing.T) {
	// 10x10 screen with margins on rows 3..5 (1-based), screen rows 2..4.
	tests := []struct {
		name  string
		seq   string
		wantY int
	}{
		{"CUD from above stops at bottom margin", "\x1b[1;1H\x1b[20B", 4},
		{"CUU from below stops at top margin", "\x1b[7;1H\x1b[20A", 2},
		{"CUD inside stops at bottom margin", "\x1b[4;1H\x1b[20B", 4},
		{"CUU inside stops at top margin", "\x1b[4;1H\x1b[20A", 2},
		{"CUD below stops at screen bottom", "\x1b[7;1H\x1b[20B", 9},
		{"CUU above stops at screen top", "\x1b[2;1H\x1b[20A", 0},
		{"CNL from above stops at bottom margin", "\x1b[1;5H\x1b[20E", 4},
		{"CPL from below stops at top margin", "\x1b[8;5H\x1b[20F", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := feedEmulator(t, 10, 10, "\x1b[3;5r", tt.seq)
			if y := e.CursorPosition().Y; y != tt.wantY {
				t.Errorf("row = %d, want %d", y, tt.wantY)
			}
		})
	}
}

func TestComplianceCursorPositionReport(t *testing.T) {
	tests := []struct {
		name string
		seq  string
		want string
	}{
		{"absolute", "\x1b[5;8r\x1b[6;3H\x1b[6n", "\x1b[6;3R"},
		{"origin relative", "\x1b[5;8r\x1b[?6h\x1b[2;3H\x1b[6n", "\x1b[2;3R"},
		{"DECXCPR origin relative", "\x1b[5;8r\x1b[?6h\x1b[2;3H\x1b[?6n", "\x1b[?2;3R"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := feedEmulator(t, 10, 10, tt.seq)
			if got := readResponse(t, e); got != tt.want {
				t.Errorf("response = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComplianceEraseInDisplay(t *testing.T) {
	const content = "aaaaa\r\nbbbbb\r\nccccc"
	tests := []struct {
		name  string
		seq   string
		want  []string
		wantX int
	}{
		{"ED 0", "\x1b[2;3H\x1b[0J", []string{"aaaaa", "bb   ", "     "}, 2},
		{"ED 1 stops at the cursor", "\x1b[2;3H\x1b[1J", []string{"     ", "   bb", "ccccc"}, 2},
		{"ED 1 at the last column", "\x1b[2;5H\x1b[1J", []string{"     ", "     ", "ccccc"}, 4},
		{"ED 2", "\x1b[2;3H\x1b[2J", []string{"     ", "     ", "     "}, 2},
		{"ED 3 keeps the screen", "\x1b[2;3H\x1b[3J", []string{"aaaaa", "bbbbb", "ccccc"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := feedEmulator(t, 5, 3, content, tt.seq)
			checkRows(t, e, tt.want)
			checkCursor(t, e, tt.wantX, 1)
		})
	}

	t.Run("ED 3 clears the scrollback", func(t *testing.T) {
		e := feedEmulator(t, 5, 3, "1\r\n2\r\n3\r\n4\r\n5")
		if e.ScrollbackLen() == 0 {
			t.Fatal("setup did not scroll anything off")
		}
		_, _ = e.WriteString("\x1b[3J")
		if n := e.ScrollbackLen(); n != 0 {
			t.Errorf("ScrollbackLen() = %d, want 0", n)
		}
		checkRows(t, e, []string{"3    ", "4    ", "5    "})
	})

	bce := []struct {
		name string
		seq  string
	}{
		{"ED 2", "\x1b[41m\x1b[2J"},
		{"ED 0", "\x1b[41m\x1b[H\x1b[0J"},
		{"ED 1", "\x1b[41m\x1b[3;5H\x1b[1J"},
	}
	for _, tt := range bce {
		t.Run(tt.name+" fills with the pen background", func(t *testing.T) {
			e := feedEmulator(t, 5, 3, content, tt.seq)
			for _, p := range []uv.Position{uv.Pos(0, 0), uv.Pos(4, 2)} {
				if c := e.CellAt(p.X, p.Y); c == nil || c.Style.Bg == nil {
					t.Errorf("cell %v has no background: %+v", p, c)
				}
			}
		})
	}
}

func TestComplianceAltScreenModes(t *testing.T) {
	t.Run("47 switches without clearing or moving the cursor", func(t *testing.T) {
		e := feedEmulator(t, 6, 2, "main", "\x1b[?47h")
		checkCursor(t, e, 4, 0)
		_, _ = e.WriteString("\x1b[Halt")
		_, _ = e.WriteString("\x1b[?47l")
		checkRows(t, e, []string{"main  "})
		_, _ = e.WriteString("\x1b[?47h")
		checkRows(t, e, []string{"alt   "})
		if !e.IsAltScreen() {
			t.Error("IsAltScreen() = false under mode 47")
		}
	})
	t.Run("47 is reported by DECRQM", func(t *testing.T) {
		e := feedEmulator(t, 6, 2, "\x1b[?47h\x1b[?47$p")
		if got, want := readResponse(t, e), "\x1b[?47;1$y"; got != want {
			t.Errorf("DECRQM = %q, want %q", got, want)
		}
	})
	t.Run("1047 clears the alt screen on exit", func(t *testing.T) {
		e := feedEmulator(t, 6, 2, "\x1b[?1047h\x1b[Halt\x1b[?1047l")
		checkRows(t, e, []string{"      "})
		_, _ = e.WriteString("\x1b[?47h")
		checkRows(t, e, []string{"      "})
	})
	t.Run("1047 does not clear on entry", func(t *testing.T) {
		e := feedEmulator(t, 6, 2, "\x1b[?47h\x1b[Halt\x1b[?47l\x1b[?1047h")
		checkRows(t, e, []string{"alt   "})
	})
	t.Run("1049 clears on entry and restores the cursor", func(t *testing.T) {
		e := feedEmulator(t, 6, 3, "\x1b[?47h\x1b[Halt\x1b[?47l", "\x1b[2;3H\x1b[?1049h")
		checkRows(t, e, []string{"      "})
		_, _ = e.WriteString("\x1b[3;5H\x1b[?1049l")
		checkCursor(t, e, 2, 1)
	})
}

func TestComplianceSoftReset(t *testing.T) {
	const setup = "\x1b[?25l\x1b[4h\x1b[?7l\x1b=\x1b[?1h\x1b(0\x1b[31m" +
		"\x1b[3;4H\x1b7\x1b[2;4r\x1b[?6h\x1b[2;2Hab"
	e := feedEmulator(t, 10, 5, "hello", setup, "\x1b[!p")

	checkCursor(t, e, 3, 2)
	if e.IsCursorHidden() {
		t.Error("cursor still hidden")
	}
	modes := []struct {
		mode ansi.Mode
		want bool
	}{
		{ansi.ModeInsertReplace, false},
		{ansi.ModeOrigin, false},
		{ansi.ModeAutoWrap, true},
		{ansi.ModeNumericKeypad, false},
		{ansi.ModeCursorKeys, false},
		{ansi.ModeTextCursorEnable, true},
	}
	for _, m := range modes {
		if got := e.isModeSet(m.mode); got != m.want {
			t.Errorf("mode %v set = %v, want %v", m.mode, got, m.want)
		}
	}
	if got, want := e.ScrollRegion(), uv.Rect(0, 0, 10, 5); got != want {
		t.Errorf("ScrollRegion() = %v, want %v", got, want)
	}
	if pen, _ := e.CursorPen(); !pen.IsZero() {
		t.Errorf("pen = %+v, want default", pen)
	}
	if got := complianceRow(e, 0); got != "hello     " {
		t.Errorf("row 0 = %q, screen was cleared", got)
	}

	_, _ = e.WriteString("q\x1b8")
	if c := e.CellAt(3, 2); c == nil || c.Content != "q" {
		t.Errorf("cell (3,2) = %+v, want plain q", c)
	}
	checkCursor(t, e, 0, 0)
}

func TestComplianceScreenAlignment(t *testing.T) {
	e := feedEmulator(t, 4, 3, "\x1b[31mxy\x1b[2;3r\x1b[2;2H\x1b#8")
	checkRows(t, e, []string{"EEEE", "EEEE", "EEEE"})
	checkCursor(t, e, 0, 0)
	if got, want := e.ScrollRegion(), uv.Rect(0, 0, 4, 3); got != want {
		t.Errorf("ScrollRegion() = %v, want %v", got, want)
	}
	if c := e.CellAt(1, 1); c.Style.Fg != nil {
		t.Errorf("DECALN cell carries the pen: %+v", c.Style)
	}
}

func TestComplianceSingleShift7Bit(t *testing.T) {
	tests := []struct {
		name string
		seq  string
		want string
	}{
		{"SS2", "\x1b*0\x1bNqq", "─q   "},
		{"SS3", "\x1b+0\x1bOqq", "─q   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := feedEmulator(t, 5, 1, tt.seq)
			checkRows(t, e, []string{tt.want})
		})
	}
}

func TestComplianceTabStopsSurviveResize(t *testing.T) {
	tests := []struct {
		name     string
		newWidth int
		wantTabs []int
	}{
		{"grow", 30, []int{3, 24, 29}},
		{"shrink", 15, []int{3, 14}},
		// Columns 15 and up are new after the shrink, so they get defaults.
		{"shrink then grow", 0, []int{3, 16, 24, 29}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Only a stop at column 3 inside the original 20 columns.
			e := feedEmulator(t, 20, 2, "\x1b[3g\x1b[1;4H\x1bH")
			if tt.newWidth == 0 {
				e.Resize(15, 2)
				e.Resize(30, 2)
			} else {
				e.Resize(tt.newWidth, 2)
			}
			_, _ = e.WriteString("\x1b[1;1H")
			for i, want := range tt.wantTabs {
				_, _ = e.WriteString("\t")
				if x := e.CursorPosition().X; x != want {
					t.Errorf("tab %d: column %d, want %d", i+1, x, want)
				}
			}
		})
	}
}

func TestComplianceSGR21DoubleUnderline(t *testing.T) {
	tests := []struct {
		name string
		seq  string
		want ansi.Underline
	}{
		{"21 is double underline", "\x1b[21m", ansi.UnderlineDouble},
		{"24 clears it", "\x1b[21;24m", ansi.UnderlineNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// SGR 21 is handled on the themed path; uv.ReadStyle, used when
			// no theme is set, does not know it.
			e := NewEmulator(5, 1)
			e.SetIndexedColor(0, color.Black)
			_, _ = e.WriteString(tt.seq)
			if pen, _ := e.CursorPen(); pen.Underline != tt.want {
				t.Errorf("underline = %v, want %v", pen.Underline, tt.want)
			}
		})
	}
}

func TestComplianceControlInsideSequence(t *testing.T) {
	tests := []struct {
		name         string
		chunks       []string
		wantX, wantY int
		wantRow      string
	}{
		{"CR inside CUF", []string{"abcde\x1b[2\rC"}, 2, 0, "abcde     "},
		{"BS inside CUF does not erase", []string{"abcde\x1b[\bC"}, 5, 0, "abcde     "},
		{"CR inside CUF split across writes", []string{"abcde\x1b[", "2", "\r", "C"}, 2, 0, "abcde     "},
		{"BS inside EL keeps EL", []string{"abcde\x1b[\b1K"}, 4, 0, "          "},
		{"LF inside CUP", []string{"\x1b[3\n;4H"}, 3, 2, "          "},
		{"CR inside ESC 7", []string{"\x1b[3;3H\x1b\r7\x1b[H\x1b8"}, 0, 2, "          "},
		{"CR inside ESC intermediate", []string{"\x1b(\r0q"}, 1, 0, "─         "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := feedEmulator(t, 10, 3, tt.chunks...)
			checkCursor(t, e, tt.wantX, tt.wantY)
			checkRows(t, e, []string{tt.wantRow})
		})
	}

	t.Run("CR inside a private-mode sequence", func(t *testing.T) {
		e := feedEmulator(t, 10, 3, "abc\x1b[?2\r5l")
		if !e.IsCursorHidden() {
			t.Error("CSI ?25l with an embedded CR did not hide the cursor")
		}
		checkCursor(t, e, 0, 0)
	})
}
