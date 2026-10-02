package app

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/tonk/tuios/internal/terminal"
	"github.com/tonk/tuios/internal/vt"
)

// attributeSample is guest output exercising every attribute a cell records,
// one feature per row so a failure names the feature. The last row puts a
// palette red next to the truecolor shade it happens to resolve to, which a
// colour comparison by RGBA alone cannot tell apart.
var attributeSample = []struct {
	name, input string
}{
	{"bold italic faint strike", "\x1b[1mB\x1b[0m \x1b[3mI\x1b[0m \x1b[2mF\x1b[0m \x1b[9mS\x1b[0m"},
	{"underline styles", "\x1b[4mu\x1b[0m \x1b[4:2md\x1b[0m \x1b[4:3mc\x1b[0m \x1b[4:4mo\x1b[0m \x1b[4:5ma\x1b[0m"},
	{"underline colour", "\x1b[4:3;58;2;255;0;0mred-curly\x1b[0m"},
	{"blink", "\x1b[5mslow\x1b[0m \x1b[6mrapid\x1b[0m"},
	{"reverse", "\x1b[7mrev\x1b[0m"},
	{"conceal", "\x1b[8msecret\x1b[0m"},
	{"hyperlink", "\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\ plain"},
	{"palette vs rgb", "\x1b[31mA\x1b[38;2;128;0;0mB\x1b[0m\x1b[41m \x1b[48;2;128;0;0m \x1b[0m"},
	{"indexed", "\x1b[38;5;1mi\x1b[38;5;200mj\x1b[0m"},
}

func writeAttributeSample(t *testing.T, win *terminal.Window) {
	t.Helper()
	var b strings.Builder
	for i, row := range attributeSample {
		fmt.Fprintf(&b, "\x1b[%d;1H%s", i+2, row.input)
	}
	// Park the cursor on row 1 so the fake-cursor overlay, if any, is off
	// the rows under test.
	b.WriteString("\x1b[1;1H")
	win.WriteOutput([]byte(b.String()))
}

// reparse feeds a rendered pane body into a fresh emulator, so two renders can
// be compared by the cells a terminal would make of them rather than by their
// bytes.
func reparse(t *testing.T, rendered string, w, h int) *vt.Emulator {
	t.Helper()
	e := vt.NewEmulator(w, h)
	t.Cleanup(func() { _ = e.Close() })
	_, _ = e.Write([]byte(strings.ReplaceAll(rendered, "\n", "\r\n")))
	return e
}

func cellDescription(c *uv.Cell) string {
	if c == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%q w=%d fg=%T(%v) bg=%T(%v) ul=%d ulc=%T(%v) attrs=%08b link=%q",
		c.Content, c.Width, c.Style.Fg, c.Style.Fg, c.Style.Bg, c.Style.Bg,
		c.Style.Underline, c.Style.UnderlineColor, c.Style.UnderlineColor, c.Style.Attrs, c.Link.URL)
}

func sameCell(a, b *uv.Cell) bool {
	if a == nil || b == nil {
		return a == b
	}
	ac, bc := a.Content, b.Content
	if ac == "" {
		ac = " "
	}
	if bc == "" {
		bc = " "
	}
	return ac == bc && a.Width == b.Width &&
		cellStylesIdentical(&a.Style, &b.Style) && a.Link.URL == b.Link.URL
}

// TestPaneRenderPathsCarryEveryAttribute renders the same emulator through
// each pane path and requires every one to reproduce the guest's cells
// exactly: the unfocused fast path, the focused path, and the unfocused copy
// mode path. The focused path used to drop underline styles and colour,
// blink, conceal and hyperlinks, and the copy-mode path kept nothing but the
// two colours, so a pane changed appearance as focus moved.
func TestPaneRenderPathsCarryEveryAttribute(t *testing.T) {
	const width, height = 60, 12
	paths := []struct {
		name  string
		setup func(m *OS, win *terminal.Window)
		focus bool
	}{
		{"unfocused", func(*OS, *terminal.Window) {}, false},
		{"focused terminal mode", func(m *OS, _ *terminal.Window) { m.Mode = TerminalMode }, true},
		{"focused window mode", func(m *OS, _ *terminal.Window) { m.Mode = WindowManagementMode }, true},
		{"unfocused copy mode", func(_ *OS, win *terminal.Window) {
			win.CopyMode = &terminal.CopyMode{Active: true, Implicit: true}
		}, false},
	}
	for _, path := range paths {
		t.Run(path.name, func(t *testing.T) {
			win := newTestWindow(t, "attrs", width, height)
			writeAttributeSample(t, win)
			m := newTestOS(win)
			path.setup(m, win)
			win.ContentDirty = true
			win.CachedContent = ""
			out := m.renderTerminal(win, path.focus, m.Mode == TerminalMode)

			got := reparse(t, out, win.Terminal.Width(), win.Terminal.Height())
			for i, row := range attributeSample {
				y := i + 1
				for x := range win.Terminal.Width() {
					want := win.Terminal.CellAt(x, y)
					have := got.CellAt(x, y)
					if !sameCell(want, have) {
						t.Errorf("%s: column %d\n got %s\nwant %s", row.name, x, cellDescription(have), cellDescription(want))
						break
					}
				}
			}
		})
	}
}

// TestStyleCacheSeparatesColorKindsAndUnderlines pins the cache key: entries
// that would emit different escapes must not share a slot.
func TestStyleCacheSeparatesColorKindsAndUnderlines(t *testing.T) {
	maroon := color.RGBA{R: 0x80, A: 0xff}
	tests := []struct {
		name string
		a, b uv.Style
	}{
		{"basic red vs #800000", uv.Style{Fg: ansi.BasicColor(1)}, uv.Style{Fg: maroon}},
		{"indexed 1 vs basic 1", uv.Style{Fg: ansi.IndexedColor(1)}, uv.Style{Fg: ansi.BasicColor(1)}},
		{"background basic vs rgb", uv.Style{Bg: ansi.BasicColor(1)}, uv.Style{Bg: maroon}},
		{"single vs curly underline", uv.Style{Underline: uv.UnderlineSingle}, uv.Style{Underline: uv.UnderlineCurly}},
		{"underline colour", uv.Style{Underline: uv.UnderlineSingle, UnderlineColor: maroon}, uv.Style{Underline: uv.UnderlineSingle}},
		{"conceal", uv.Style{Attrs: uv.AttrConceal}, uv.Style{}},
		{"rapid vs slow blink", uv.Style{Attrs: uv.AttrRapidBlink}, uv.Style{Attrs: uv.AttrBlink}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := NewStyleCache(64)
			_, pa, _ := cache.GetWithANSI(&uv.Cell{Content: "x", Width: 1, Style: tc.a}, false, false)
			_, pb, _ := cache.GetWithANSI(&uv.Cell{Content: "x", Width: 1, Style: tc.b}, false, false)
			if pa == pb {
				t.Fatalf("both styles emit %q", pa)
			}
			if want := cellSGR(&tc.a); pa != want {
				t.Errorf("escape = %q, want %q", pa, want)
			}
			if size := cache.GetStats().Size; size != 2 {
				t.Errorf("cache holds %d entries, want 2", size)
			}
		})
	}
}

// TestColorsIdentical is the batching equality: identical RGBA is not enough.
func TestColorsIdentical(t *testing.T) {
	maroon := color.RGBA{R: 0x80, A: 0xff}
	tests := []struct {
		name string
		a, b color.Color
		want bool
	}{
		{"nil nil", nil, nil, true},
		{"nil basic", nil, ansi.BasicColor(0), false},
		{"basic same", ansi.BasicColor(1), ansi.BasicColor(1), true},
		{"basic vs rgb", ansi.BasicColor(1), maroon, false},
		{"indexed vs basic", ansi.IndexedColor(1), ansi.BasicColor(1), false},
		{"rgb same value different type", maroon, color.NRGBA{R: 0x80, A: 0xff}, true},
		{"rgb differ", maroon, color.RGBA{R: 0x81, A: 0xff}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := colorsIdentical(tc.a, tc.b); got != tc.want {
				t.Errorf("colorsIdentical(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestThemeSwitchRecolorsExistingText writes palette text under one theme and
// renders it under another, then with theming off. Resolving the palette at
// write time froze each line in the theme it was written under.
func TestThemeSwitchRecolorsExistingText(t *testing.T) {
	var themeA, themeB [16]color.Color
	for i := range 16 {
		themeA[i] = color.RGBA{R: 0x10, G: uint8(i), B: 0x01, A: 0xff}
		themeB[i] = color.RGBA{R: 0x20, G: uint8(i), B: 0x02, A: 0xff}
	}
	sgrOf := func(c color.Color) string {
		return ansi.Style{}.ForegroundColor(c).String()
	}
	for _, focused := range []bool{false, true} {
		t.Run(fmt.Sprintf("focused=%v", focused), func(t *testing.T) {
			win := newTestWindow(t, "theme", 20, 8)
			m := newTestOS(win)
			m.Mode = TerminalMode
			render := func() string {
				win.ContentDirty = true
				win.CachedContent = ""
				return m.renderTerminal(win, focused, true)
			}

			win.Terminal.SetThemeColors(color.White, color.Black, color.White, themeA)
			win.WriteOutput([]byte("\x1b[31mold\x1b[0m"))
			if out := render(); !strings.Contains(out, sgrOf(themeA[1])) {
				t.Fatalf("theme A red missing: %q", out)
			}

			win.Terminal.SetThemeColors(color.White, color.Black, color.White, themeB)
			out := render()
			if !strings.Contains(out, sgrOf(themeB[1])) || strings.Contains(out, sgrOf(themeA[1])) {
				t.Fatalf("text written under theme A did not follow the switch to B: %q", out)
			}

			win.Terminal.SetThemeColors(nil, nil, nil, [16]color.Color{})
			win.WriteOutput([]byte("\r\n\x1b[31mnew\x1b[0m"))
			out = render()
			if strings.Contains(out, sgrOf(themeB[1])) {
				t.Fatalf("theme B red survived disabling the theme: %q", out)
			}
			if strings.Count(out, sgrOf(ansi.BasicColor(1))) != 2 {
				t.Fatalf("with theming off both lines should use the host's palette red: %q", out)
			}
		})
	}
}

// TestSyncHeldFrameKeepsItsCursor holds a frame for a DEC 2026 synchronized
// update and checks the host cursor stays with it. The held layer shows the
// screen from before the update, so a cursor read live from the emulator sat
// wherever the guest was halfway through drawing.
func TestSyncHeldFrameKeepsItsCursor(t *testing.T) {
	win := newTestWindow(t, "sync-cursor", 40, 10)
	m := newTestOS(win)
	m.Mode = TerminalMode
	m.Width, m.Height = 80, 24

	win.WriteOutput([]byte("prompt$ "))
	m.GetCanvas(true)
	before := m.getRealCursor()
	if before == nil {
		t.Fatal("no cursor before the update; the fixture proves nothing")
	}

	win.WriteOutput([]byte("\x1b[?2026h\x1b[5;20Hhalfway"))
	if !win.Terminal.IsSyncActive() {
		t.Fatal("synchronized update did not start")
	}
	m.GetCanvas(true)
	held := m.getRealCursor()
	if held == nil || held.X != before.X || held.Y != before.Y {
		t.Fatalf("cursor during the held frame = %+v, want it where the held frame put it (%d,%d)", held, before.X, before.Y)
	}

	win.WriteOutput([]byte("\x1b[?2026l"))
	m.GetCanvas(true)
	after := m.getRealCursor()
	if after == nil || (after.X == before.X && after.Y == before.Y) {
		t.Fatalf("cursor did not follow the finished frame: %+v", after)
	}
}

// TestBusyPaneCacheKeepsItsCursor covers the other held frame: a pane whose
// output lock is busy serves its previous frame, and the cursor goes with it.
func TestBusyPaneCacheKeepsItsCursor(t *testing.T) {
	win := newTestWindow(t, "busy-cursor", 40, 10)
	m := newTestOS(win)
	m.Mode = TerminalMode

	win.WriteOutput([]byte("prompt$ "))
	win.ContentDirty = true
	m.renderTerminal(win, true, true)
	before := m.getRealCursor()
	if before == nil {
		t.Fatal("no cursor; the fixture proves nothing")
	}

	// The guest moves on, but the content is only flagged dirty, not
	// dropped, the way WriteToPTY does it, and a writer holds the lock.
	win.Terminal.Write([]byte("\x1b[6;30H"))
	win.ContentDirty = true
	release := holdIOLock(win)
	out := m.renderTerminal(win, true, true)
	held := m.getRealCursor()
	release()
	if out != win.CachedContent {
		t.Fatal("busy pane was not served from cache; the fixture proves nothing")
	}
	if held == nil || held.X != before.X || held.Y != before.Y {
		t.Fatalf("cursor over the cached frame = %+v, want (%d,%d)", held, before.X, before.Y)
	}
	if !win.ContentDirty {
		t.Error("a busy pane served from cache must stay dirty")
	}
}

// holdIOLock takes the window's I/O lock exclusively from another goroutine,
// the way the PTY writer does, and returns once it is held.
func holdIOLock(win *terminal.Window) (release func()) {
	held := make(chan struct{})
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		win.LockIO()
		close(held)
		<-done
		win.UnlockIO()
		close(finished)
	}()
	<-held
	return func() {
		close(done)
		<-finished
	}
}

// TestCanvasKeepsBlankFrameDirty pins the blank-frame protection end to end.
// cacheRender leaves a blank frame dirty so the next frame reads the emulator
// again, but GetCanvas cleared every dirty flag after drawing the box, so a
// pane caught between an application clearing the screen and painting it
// stayed blank.
func TestCanvasKeepsBlankFrameDirty(t *testing.T) {
	win := newTestWindow(t, "blank-canvas", 40, 10)
	m := newTestOS(win)
	m.Width, m.Height = 80, 24
	m.FocusedWindow = -1

	win.WriteOutput([]byte("\x1b[?1049h\x1b[2J"))
	m.GetCanvas(true)
	if !win.ContentDirty {
		t.Fatal("blank frame cleared ContentDirty; the pane would freeze blank")
	}

	win.WriteOutput([]byte("painted"))
	m.GetCanvas(true)
	if win.ContentDirty {
		t.Error("a painted frame should clear ContentDirty")
	}
}

// TestIsBlankRenderSeesPaintedSpaces: a block of spaces with a background
// colour or in reverse video is on screen, so it is content.
func TestIsBlankRenderSeesPaintedSpaces(t *testing.T) {
	tests := []struct {
		in    string
		blank bool
	}{
		{"\x1b[41m   \x1b[m", false},
		{"\x1b[48;2;10;20;30m  \x1b[0m", false},
		{"\x1b[48;5;7m \x1b[0m", false},
		{"\x1b[7m \x1b[27m", false},
		{"\x1b[101m \x1b[m", false},
		{"\x1b[41m\x1b[49m   ", true},
		{"\x1b[41m\x1b[0m   ", true},
		{"\x1b[41m\x1b[m   ", true},
		{"\x1b[38;2;7;7;7m   \x1b[m", true}, // 7 is a colour component, not reverse
		{"\x1b[38;5;41m   \x1b[m", true},   // 41 is a palette index, not a background
		{"\x1b[41m\n\n", true},              // a background with no cell to show it on
	}
	for _, tc := range tests {
		if got := isBlankRender(tc.in); got != tc.blank {
			t.Errorf("isBlankRender(%q) = %v, want %v", tc.in, got, tc.blank)
		}
	}
}
