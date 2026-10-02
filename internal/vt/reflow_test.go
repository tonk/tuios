package vt

import (
	"fmt"
	"image/color"
	"reflect"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// rowText is a row as a reader sees it: the clusters in order, a wide rune's
// continuation skipped, trailing blanks trimmed, and "~" appended when the row
// is soft-wrapped into the next one.
func rowText(l uv.Line, w LineWrap) string {
	var b strings.Builder
	for x := range l {
		b.WriteString(l[x].Content)
	}
	s := strings.TrimRight(b.String(), " ")
	if w.Wrapped() {
		s += "~"
	}
	return s
}

// dumpScrollback returns every scrollback line through rowText.
func dumpScrollback(e *Emulator) []string {
	out := []string{}
	for i := range e.ScrollbackLen() {
		out = append(out, rowText(e.ScrollbackLine(i), e.ScrollbackLineWrap(i)))
	}
	return out
}

// dumpScreen returns the active screen's rows through rowText, without the
// blank rows at the bottom.
func dumpScreen(e *Emulator) []string {
	out := []string{}
	for y := range e.Height() {
		var l uv.Line
		for x := range e.Width() {
			l = append(l, *e.CellAt(x, y))
		}
		out = append(out, rowText(l, e.LineWrap(y)))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// checkGrid fails unless every row of the screen is exactly the width and no
// wide rune is cut, the invariant every reader depends on.
func checkGrid(t *testing.T, e *Emulator) {
	t.Helper()
	for y := range e.Height() {
		for x := 0; x < e.Width(); {
			c := e.CellAt(x, y)
			if c == nil {
				t.Fatalf("no cell at (%d,%d) on a %dx%d screen", x, y, e.Width(), e.Height())
			}
			if c.Width == 0 {
				t.Fatalf("orphaned continuation at (%d,%d)", x, y)
			}
			if x+c.Width > e.Width() {
				t.Fatalf("%q at (%d,%d) runs past the edge", c.Content, x, y)
			}
			x += c.Width
		}
	}
}

func TestSoftWrapTracking(t *testing.T) {
	for _, tc := range []struct {
		name     string
		w, h     int
		input    string
		screen   []string
		sb       []string
		spacerAt int // row whose spacer count is checked, -1 for none
		spacer   int
	}{
		{"autowrap marks the row", 5, 3, "abcdefgh", []string{"abcde~", "fgh"}, nil, -1, 0},
		{"exact fill then CRLF is a hard break", 5, 3, "abcde\r\nfg", []string{"abcde", "fg"}, nil, -1, 0},
		{"pending wrap alone does not wrap", 5, 3, "abcde", []string{"abcde"}, nil, -1, 0},
		{"wide rune wrap leaves a spacer", 5, 3, "abcd中e", []string{"abcd~", "中e"}, nil, 0, 1},
		{"EL 2 clears the wrap", 5, 3, "abcdefgh\x1b[1;1H\x1b[2K", []string{"", "fgh"}, nil, -1, 0},
		{"EL 0 clears the wrap", 5, 3, "abcdefgh\x1b[1;3H\x1b[K", []string{"ab", "fgh"}, nil, -1, 0},
		{"EL 1 short of the edge keeps it", 5, 3, "abcdefgh\x1b[1;2H\x1b[1K", []string{"  cde~", "fgh"}, nil, -1, 0},
		{"ECH short of the edge keeps it", 5, 3, "abcdefgh\x1b[1;1H\x1b[2X", []string{"  cde~", "fgh"}, nil, -1, 0},
		{"ED 2 clears every wrap", 5, 3, "abcdefgh\x1b[2J", []string{}, nil, -1, 0},
		{"ED 0 clears from the cursor", 5, 3, "abcdefghijklm\x1b[2;1H\x1b[J", []string{"abcde~"}, nil, -1, 0},
		{"IL moves the flag down", 5, 4, "abcdefgh\x1b[1;1H\x1b[L", []string{"", "abcde~", "fgh"}, nil, -1, 0},
		{"DL moves the flag up", 5, 4, "x\r\nabcdefgh\x1b[1;1H\x1b[M", []string{"abcde~", "fgh"}, nil, -1, 0},
		{"scrolling carries the flag into the scrollback", 5, 2, "abcdefgh\r\nij\r\nkl",
			[]string{"ij", "kl"}, []string{"abcde~", "fgh"}, -1, 0},
		{"a scroll region carries the flag with the row", 5, 4, "\x1b[2;4r\x1b[2;1Hx\r\nabcdefgh\r\n",
			[]string{"", "abcde~", "fgh"}, nil, -1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEmulator(tc.w, tc.h)
			_, _ = e.WriteString(tc.input)
			if got := dumpScreen(e); !reflect.DeepEqual(got, tc.screen) {
				t.Errorf("screen = %q, want %q", got, tc.screen)
			}
			if tc.sb != nil {
				if got := dumpScrollback(e); !reflect.DeepEqual(got, tc.sb) {
					t.Errorf("scrollback = %q, want %q", got, tc.sb)
				}
			}
			if tc.spacerAt >= 0 {
				if got := e.LineWrap(tc.spacerAt).Spacer(); got != tc.spacer {
					t.Errorf("row %d spacer = %d, want %d", tc.spacerAt, got, tc.spacer)
				}
			}
		})
	}

	t.Run("the alternate screen keeps its own", func(t *testing.T) {
		e := NewEmulator(5, 3)
		_, _ = e.WriteString("abcdefgh\x1b[?1049h\x1b[Hxy")
		if e.LineWrap(0).Wrapped() {
			t.Error("alternate screen row 0 is wrapped")
		}
		if !e.MainLineWrap(0).Wrapped() {
			t.Error("normal screen lost its wrap on entering the alternate screen")
		}
		_, _ = e.WriteString("\x1b[?1049l")
		if !e.LineWrap(0).Wrapped() {
			t.Error("normal screen lost its wrap across the alternate screen")
		}
	})

	t.Run("PushLine is a hard break", func(t *testing.T) {
		sb := NewScrollback(4)
		sb.PushLine(uv.Line{uv.EmptyCell})
		sb.PushLineWithWrap(uv.Line{uv.EmptyCell}, true)
		sb.PushLineWrap(uv.Line{uv.EmptyCell}, SoftWrap(2))
		if sb.IsSoftWrapped(0) || !sb.IsSoftWrapped(1) || sb.LineWrap(2).Spacer() != 2 {
			t.Errorf("wraps = %v %v %v", sb.LineWrap(0), sb.LineWrap(1), sb.LineWrap(2))
		}
	})
}

func TestReflowOnResize(t *testing.T) {
	for _, tc := range []struct {
		name   string
		w, h   int
		input  string
		nw, nh int
		sb     []string
		screen []string
		cx, cy int
	}{
		{
			name: "narrowing splits a line", w: 10, h: 3,
			input: "abcdefghij\r\n$ ", nw: 5, nh: 3,
			sb: []string{}, screen: []string{"abcde~", "fghij", "$"}, cx: 2, cy: 2,
		},
		{
			name: "widening joins soft-wrapped rows", w: 4, h: 4,
			input: "abcdefghij\r\n$ ", nw: 10, nh: 4,
			sb: []string{}, screen: []string{"abcdefghij", "$"}, cx: 2, cy: 1,
		},
		{
			name: "hard breaks are not joined", w: 4, h: 4,
			input: "abcd\r\nefgh\r\n$ ", nw: 10, nh: 4,
			sb: []string{}, screen: []string{"abcd", "efgh", "$"}, cx: 2, cy: 2,
		},
		{
			name: "scrollback lines join with the screen", w: 4, h: 2,
			input: "abcdefghij", nw: 12, nh: 2,
			sb: []string{}, screen: []string{"abcdefghij"}, cx: 10, cy: 0,
		},
		{
			name: "scrollback reflows on its own lines", w: 4, h: 2,
			input: "abcdefgh\r\nxy\r\nz\r\n$ ", nw: 8, nh: 2,
			sb: []string{"abcdefgh", "xy"}, screen: []string{"z", "$"}, cx: 2, cy: 1,
		},
		{
			name: "narrowing pushes the top into the scrollback", w: 6, h: 3,
			input: "abcdef\r\nghijkl\r\n$ ", nw: 3, nh: 3,
			sb: []string{"abc~", "def"}, screen: []string{"ghi~", "jkl", "$"}, cx: 2, cy: 2,
		},
		{
			name: "rows under the cursor make room", w: 6, h: 4,
			input: "abcdef\r\n$ ", nw: 3, nh: 4,
			sb: []string{}, screen: []string{"abc~", "def", "$"}, cx: 2, cy: 2,
		},
		{
			name: "a wide rune is never split", w: 6, h: 4,
			input: "ab中cd", nw: 3, nh: 4,
			sb: []string{}, screen: []string{"ab~", "中c~", "d"}, cx: 1, cy: 2,
		},
		{
			name: "spacer from a wide wrap is not content", w: 5, h: 3,
			input: "abcd中ef", nw: 10, nh: 3,
			sb: []string{}, screen: []string{"abcd中ef"}, cx: 8, cy: 0,
		},
		{
			name: "a real space before the wrap is content", w: 5, h: 3,
			input: "abcd 中ef", nw: 10, nh: 3,
			sb: []string{}, screen: []string{"abcd 中ef"}, cx: 9, cy: 0,
		},
		{
			name: "grapheme clusters stay whole", w: 4, h: 3,
			input: "ab👨‍👩‍👧éfg", nw: 2, nh: 4,
			sb: []string{}, screen: []string{"ab~", "👨‍👩‍👧~", "éf~", "g"}, cx: 1, cy: 3,
		},
		{
			name: "the cursor stays on its character", w: 20, h: 4,
			input: "hello world\x1b[1;3H", nw: 5, nh: 4,
			sb: []string{}, screen: []string{"hello~", " worl~", "d"}, cx: 2, cy: 0,
		},
		{
			name: "the cursor after a full row has the wrap pending", w: 10, h: 3,
			input: "$ ", nw: 2, nh: 3,
			sb: []string{}, screen: []string{"$"}, cx: 1, cy: 0,
		},
		{
			name: "a cursor on a blank row keeps its row", w: 20, h: 3,
			input: "\x1b[2;19H", nw: 8, nh: 3,
			sb: []string{}, screen: []string{}, cx: 7, cy: 1,
		},
		{
			name: "height and width at once", w: 10, h: 4,
			input: "0123456789\r\nabcdefghij\r\n$ ", nw: 5, nh: 3,
			sb: []string{"01234~", "56789"}, screen: []string{"abcde~", "fghij", "$"}, cx: 2, cy: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEmulator(tc.w, tc.h)
			_, _ = e.WriteString(tc.input)
			e.Resize(tc.nw, tc.nh)
			checkGrid(t, e)
			if got := dumpScrollback(e); !reflect.DeepEqual(got, tc.sb) {
				t.Errorf("scrollback = %q, want %q", got, tc.sb)
			}
			if got := dumpScreen(e); !reflect.DeepEqual(got, tc.screen) {
				t.Errorf("screen = %q, want %q", got, tc.screen)
			}
			checkCursor(t, e, tc.cx, tc.cy)
		})
	}
}

func TestReflowCursorAndPrinting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		w, h   int
		input  string
		nw     int
		after  string
		screen []string
	}{
		{"typing continues after the text", 20, 4, "hello world", 5, "!", []string{"hello~", " worl~", "d!"}},
		{"a prompt that now fills the row wraps the next character", 10, 3, "$ ", 2, "x", []string{"$~", "x"}},
		{"a pending wrap steps forward when there is room again", 5, 3, "abcde", 8, "X", []string{"abcdeX"}},
		{"a pending wrap survives a narrowing that keeps it at the edge", 6, 3, "abcdef", 3, "X", []string{"abc~", "def~", "X"}},
		{"a pending wrap after a narrowing that moves it", 5, 3, "abcde", 3, "X", []string{"abc~", "deX"}},
		{"saved cursor follows its character", 20, 6, "hello world\x1b[1;8H\x1b7\x1b[4;1H", 5, "\x1b8X", []string{"hello~", " wXrl~", "d"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEmulator(tc.w, tc.h)
			_, _ = e.WriteString(tc.input)
			e.Resize(tc.nw, tc.h)
			_, _ = e.WriteString(tc.after)
			if got := dumpScreen(e); !reflect.DeepEqual(got, tc.screen) {
				t.Errorf("screen = %q, want %q", got, tc.screen)
			}
		})
	}
}

func TestReflowKeepsCellAttributes(t *testing.T) {
	e := NewEmulator(6, 3)
	_, _ = e.WriteString("ab\x1b[31m\x1b]8;;https://example.com\x1b\\cdefgh\x1b]8;;\x1b\\\x1b[m")
	e.Resize(3, 3)
	// "abc" / "def" / "gh": c through h are red and linked.
	for _, p := range []struct{ x, y int }{{2, 0}, {0, 1}, {1, 2}} {
		c := e.CellAt(p.x, p.y)
		if c.Link.URL != "https://example.com" {
			t.Errorf("cell (%d,%d) %q link = %q", p.x, p.y, c.Content, c.Link.URL)
		}
		if c.Style.Fg == nil || !reflect.DeepEqual(c.Style.Fg, ansi.IndexedColor(1)) && !reflect.DeepEqual(c.Style.Fg, ansi.BasicColor(1)) {
			t.Errorf("cell (%d,%d) %q fg = %v", p.x, p.y, c.Content, c.Style.Fg)
		}
	}
	if c := e.CellAt(0, 0); c.Style.Fg != nil || c.Link.URL != "" {
		t.Errorf("cell (0,0) picked up attributes: %+v", c)
	}
}

func TestReflowBackgroundRowsDoNotGrow(t *testing.T) {
	// A prompt that erases to the end of the line in its own colour: the row
	// is all background, which is not text to wrap.
	e := NewEmulator(10, 4)
	_, _ = e.WriteString("\x1b[44m$ \x1b[K\x1b[m\r\nnext")
	e.Resize(6, 4)
	if got := dumpScreen(e); !reflect.DeepEqual(got, []string{"$", "next"}) {
		t.Errorf("screen = %q", got)
	}
	if bg := e.CellAt(5, 0).Style.Bg; bg == nil {
		t.Error("the background of the first row was lost where it still fits")
	}
	var want color.Color = ansi.BasicColor(4)
	if bg := e.CellAt(3, 0).Style.Bg; !reflect.DeepEqual(bg, want) && !reflect.DeepEqual(bg, ansi.IndexedColor(4)) {
		t.Errorf("background = %v", bg)
	}
}

func TestReflowRoundTrip(t *testing.T) {
	e := NewEmulator(12, 5)
	var in strings.Builder
	for i := range 20 {
		fmt.Fprintf(&in, "line %d %s\r\n", i, strings.Repeat("中x", i%5))
	}
	in.WriteString("$ ")
	_, _ = e.WriteString(in.String())
	sb, screen := dumpScrollback(e), dumpScreen(e)
	cur := e.CursorPosition()

	// Width 1 is left out: a wide rune does not fit in it at all and is lost.
	for _, w := range []int{7, 3, 2, 30, 5} {
		e.Resize(w, 5)
		checkGrid(t, e)
	}
	e.Resize(12, 5)
	if got := dumpScrollback(e); !reflect.DeepEqual(got, sb) {
		t.Errorf("scrollback after the round trip:\n%q\nwant\n%q", got, sb)
	}
	if got := dumpScreen(e); !reflect.DeepEqual(got, screen) {
		t.Errorf("screen after the round trip = %q, want %q", got, screen)
	}
	if got := e.CursorPosition(); got != cur {
		t.Errorf("cursor after the round trip = %v, want %v", got, cur)
	}
}

func TestReflowWideRuneWiderThanTheScreen(t *testing.T) {
	e := NewEmulator(4, 3)
	_, _ = e.WriteString("a中b")
	e.Resize(1, 6)
	checkGrid(t, e)
	if got := dumpScreen(e); !reflect.DeepEqual(got, []string{"a~", "~", "b"}) {
		t.Errorf("screen = %q", got)
	}
}

func TestReflowNeverTouchesTheAlternateScreen(t *testing.T) {
	e := NewEmulator(8, 3)
	_, _ = e.WriteString("abcdefghij\x1b[?1049h\x1b[Habcdefgh")
	e.Resize(4, 3)
	if got := dumpScreen(e); !reflect.DeepEqual(got, []string{"abcd"}) {
		t.Errorf("alternate screen = %q, want it cut, not reflowed", got)
	}
	_, _ = e.WriteString("\x1b[?1049l")
	if got := dumpScreen(e); !reflect.DeepEqual(got, []string{"abcd~", "efgh~", "ij"}) {
		t.Errorf("normal screen = %q, want it reflowed underneath", got)
	}
}

func TestReflowRespectsTheScrollbackLimit(t *testing.T) {
	e := NewEmulator(20, 2)
	e.SetScrollbackMaxLines(10)
	for i := range 30 {
		_, _ = e.WriteString(fmt.Sprintf("%02d-%s\r\n", i, strings.Repeat("x", 15)))
	}
	e.Resize(5, 2)
	if n := e.ScrollbackLen(); n != 10 {
		t.Fatalf("scrollback holds %d lines, want the limit of 10", n)
	}
	// The newest history survives: line 29 ends on the screen, the cursor on
	// the blank row under it.
	if got := dumpScrollback(e); got[9] != "xxxxx~" || got[7] != "29-xx~" || got[6] != "xxx" {
		t.Errorf("scrollback tail = %q", got[6:])
	}
	if got := dumpScreen(e); !reflect.DeepEqual(got, []string{"xxx"}) {
		t.Errorf("screen = %q", got)
	}
}

func TestReflowMovesSemanticMarkers(t *testing.T) {
	e := NewEmulator(10, 5)
	_, _ = e.WriteString("0123456789abc\r\n\x1b]133;A\x07$ ")
	before := e.SemanticMarkers().Markers()
	if len(before) != 1 || before[0].AbsLine != 2 {
		t.Fatalf("markers before = %+v", before)
	}
	e.Resize(5, 5)
	after := e.SemanticMarkers().Markers()
	if len(after) != 1 || after[0].AbsLine != 3 || after[0].Col != 0 {
		t.Errorf("markers after = %+v, want the prompt on line 3", after)
	}
}

func TestScrollbackReflowStandalone(t *testing.T) {
	sb := NewScrollback(100)
	push := func(s string, w LineWrap) {
		l := make(uv.Line, 4)
		for x := range l {
			l[x] = uv.EmptyCell
		}
		for x, r := range s {
			l[x] = uv.Cell{Content: string(r), Width: 1}
		}
		sb.PushLineWrap(l, w)
	}
	push("abcd", SoftWrap(0))
	push("ef", HardBreak)
	push("ghij", SoftWrap(0))
	push("kl", SoftWrap(2)) // continues on a screen this buffer does not have

	sb.Reflow(3)
	var got []string
	for i := range sb.Len() {
		got = append(got, rowText(sb.Line(i), sb.LineWrap(i)))
	}
	want := []string{"abc~", "def", "ghi~", "jkl~"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reflowed = %q, want %q", got, want)
	}
}

// BenchmarkReflow resizes a pane holding a full 10,000 line scrollback back
// and forth, by one column (the cost a drag pays per step) and by 80. Every
// fifth line is long enough to wrap, the rest are short.
func BenchmarkReflow(b *testing.B) {
	for _, step := range []int{1, 80} {
		b.Run(fmt.Sprintf("%d columns", step), func(b *testing.B) {
			e := NewEmulator(200, 50)
			var in strings.Builder
			for i := range DefaultScrollbackSize + 50 {
				if i%5 == 0 {
					fmt.Fprintf(&in, "%05d %s\r\n", i, strings.Repeat("wrapped text ", 40))
				} else {
					fmt.Fprintf(&in, "%05d short line\r\n", i)
				}
			}
			_, _ = e.WriteString(in.String())
			if e.ScrollbackLen() != DefaultScrollbackSize {
				b.Fatalf("scrollback holds %d lines", e.ScrollbackLen())
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				e.Resize(200-(i+1)%2*step, 50)
			}
		})
	}
}
