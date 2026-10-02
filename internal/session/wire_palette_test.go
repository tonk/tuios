package session

import (
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/tonk/tuios/internal/vt"
)

// A palette entry and the RGB it happens to resolve to are different colors:
// the first follows the theme, the second does not. These tests pin that the
// snapshot wire keeps them apart in both directions.

// sameColorKind reports whether a and b are the same kind of color with the
// same value. Comparing through RGBA() would call BasicColor(1) and #800000
// equal, which is exactly the confusion under test.
func sameColorKind(a, b color.Color) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case ansi.BasicColor:
		bv, ok := b.(ansi.BasicColor)
		return ok && av == bv
	case ansi.IndexedColor:
		bv, ok := b.(ansi.IndexedColor)
		return ok && av == bv
	}
	switch b.(type) {
	case nil, ansi.BasicColor, ansi.IndexedColor:
		return false
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func TestColorWireRoundTripKeepsTheKind(t *testing.T) {
	maroon := color.RGBA{R: 0x80, A: 0xff}
	tests := []struct {
		name string
		in   color.Color
		wire string
	}{
		{name: "default", in: nil, wire: ""},
		{name: "basic-red", in: ansi.BasicColor(1), wire: "a1"},
		{name: "basic-bright-white", in: ansi.BasicColor(15), wire: "a15"},
		{name: "indexed-low", in: ansi.IndexedColor(1), wire: "i1"},
		{name: "indexed-cube", in: ansi.IndexedColor(200), wire: "i200"},
		{name: "rgb-same-shade-as-red", in: maroon, wire: "#800000"},
		{name: "ansi-rgb", in: ansi.RGBColor{R: 12, G: 34, B: 56}, wire: "#0c2238"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire := colorToWire(tt.in)
			if wire != tt.wire {
				t.Fatalf("colorToWire(%#v) = %q, want %q", tt.in, wire, tt.wire)
			}
			got := colorFromWire(wire)
			if !sameColorKind(tt.in, got) {
				t.Fatalf("colorFromWire(%q) = %#v (%T), want %#v (%T)", wire, got, got, tt.in, tt.in)
			}
		})
	}

	t.Run("basic-red-distinct-from-maroon", func(t *testing.T) {
		basic := colorFromWire(colorToWire(ansi.BasicColor(1)))
		rgb := colorFromWire(colorToWire(maroon))
		if sameColorKind(basic, rgb) {
			t.Fatalf("BasicColor(1) and #800000 came back as the same color: %#v, %#v", basic, rgb)
		}
	})
}

func TestColorFromWireRejectsMalformed(t *testing.T) {
	for _, s := range []string{"a16", "a-1", "i256", "i-1", "ax", "x1", "#zzzzzz"} {
		t.Run(s, func(t *testing.T) {
			if got := colorFromWire(s); got != nil {
				t.Fatalf("colorFromWire(%q) = %#v, want nil", s, got)
			}
		})
	}
}

// testPalette is a 16-color palette whose every slot is base plus its index, so
// two palettes built from different bases never share a shade.
func testPalette(base uint8) [16]color.Color {
	var p [16]color.Color
	for i := range p {
		p[i] = color.RGBA{R: base + uint8(i), G: base, B: base, A: 0xff} // #nosec G115 - i < 16
	}
	return p
}

// TestRestoredPaletteFollowsAThemeSwitch is the shape of the bug: a pane is
// rebuilt from a daemon snapshot into a client emulator that already holds a
// theme, and the theme is then switched. Palette text has to repaint in the new
// theme; only truecolor keeps its shade.
func TestRestoredPaletteFollowsAThemeSwitch(t *testing.T) {
	const cols, rows = 20, 2
	maroon := color.RGBA{R: 0x80, A: 0xff}
	oldTheme, newTheme := testPalette(0x10), testPalette(0x40)

	tests := []struct {
		name     string
		out      string
		wantKind color.Color
		// wantAfter is what the cell must draw as once newTheme is active.
		wantAfter color.Color
	}{
		{name: "sgr-31", out: "\x1b[31mX", wantKind: ansi.BasicColor(1), wantAfter: newTheme[1]},
		{name: "sgr-97", out: "\x1b[97mX", wantKind: ansi.BasicColor(15), wantAfter: newTheme[15]},
		{name: "38-5-1", out: "\x1b[38;5;1mX", wantKind: ansi.IndexedColor(1), wantAfter: newTheme[1]},
		{name: "38-5-200", out: "\x1b[38;5;200mX", wantKind: ansi.IndexedColor(200), wantAfter: ansi.IndexedColor(200)},
		{name: "truecolor", out: "\x1b[38;2;128;0;0mX", wantKind: maroon, wantAfter: maroon},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			daemon := vt.NewEmulator(cols, rows)
			defer func() { _ = daemon.Close() }()
			if _, err := daemon.Write([]byte(tt.out)); err != nil {
				t.Fatalf("feed the daemon emulator: %v", err)
			}

			client := vt.NewEmulator(cols, rows)
			defer func() { _ = client.Close() }()
			client.SetThemeColors(color.White, color.Black, color.White, oldTheme)
			ApplyTerminalState(client, TerminalStateOf(daemon, cols, rows, 0))

			cell := client.CellAt(0, 0)
			if cell == nil || cell.Content != "X" {
				t.Fatalf("restored cell = %#v, want the X the guest wrote", cell)
			}
			if !sameColorKind(cell.Style.Fg, tt.wantKind) {
				t.Fatalf("restored fg = %#v (%T), want %#v (%T)", cell.Style.Fg, cell.Style.Fg, tt.wantKind, tt.wantKind)
			}

			client.SetThemeColors(color.White, color.Black, color.White, newTheme)
			if got := client.ResolveColor(cell.Style.Fg); !sameColorKind(got, tt.wantAfter) {
				t.Fatalf("after the theme switch the cell draws as %#v, want %#v", got, tt.wantAfter)
			}
		})
	}
}

// TestCaptureANSIKeepsPaletteCodes pins what capture-pane --ansi emits. It is
// answered from the daemon's emulator, which holds no theme (the theme belongs
// to each client), so a palette color is captured as the palette code the
// guest wrote and is painted by whatever terminal the capture is shown in.
// Freezing it to an RGB shade would bake in a palette the daemon does not have.
func TestCaptureANSIKeepsPaletteCodes(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		want    string
		notWant string
	}{
		{name: "sgr-31", out: "\x1b[31mred", want: "\x1b[31m", notWant: "38;2;"},
		{name: "38-5-1", out: "\x1b[38;5;1mred", want: "38;5;1m", notWant: "38;2;"},
		{name: "truecolor", out: "\x1b[38;2;128;0;0mrgb", want: "38;2;128;0;0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PTY{terminal: vt.NewEmulator(20, 2)}
			defer func() { _ = p.terminal.Close() }()
			if _, err := p.terminal.Write([]byte(tt.out)); err != nil {
				t.Fatalf("feed the emulator: %v", err)
			}
			got := p.CaptureContent(false, true)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("capture %q does not contain %q", got, tt.want)
			}
			if tt.notWant != "" && strings.Contains(got, tt.notWant) {
				t.Fatalf("capture %q contains %q: a palette color was frozen to RGB", got, tt.notWant)
			}
		})
	}
}
