package vt

import (
	"image/color"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestThemedPaletteResolvesAtRenderTime pins where the theme is applied.
// Cells keep a palette colour as a palette colour, and ResolveColor maps it
// through the theme in force when the cell is drawn. Converting it to the
// theme's RGB at write time left text in the palette it was written under
// after a theme switch, and switching theming off left the old palette in the
// emulator's colour table, so new SGR 31 text kept the theme's red too.
func TestThemedPaletteResolvesAtRenderTime(t *testing.T) {
	var themeA, themeB [16]color.Color
	for i := range 16 {
		themeA[i] = color.RGBA{R: 0xa0, G: uint8(i), A: 0xff}
		themeB[i] = color.RGBA{R: 0xb0, G: uint8(i), A: 0xff}
	}

	e := NewEmulator(20, 3)
	defer func() { _ = e.Close() }()
	e.SetThemeColors(color.White, color.Black, color.White, themeA)
	_, _ = e.Write([]byte("\x1b[31ma\x1b[44mb\x1b[0m\x1b[38;5;9mc\x1b[0m\x1b[38;5;200md\x1b[0m\x1b[38;2;1;2;3me\x1b[0m"))

	cells := []struct {
		x       int
		fg, bg  color.Color
		themedA color.Color // what fg (or bg for column 1) resolves to under theme A
	}{
		{0, ansi.BasicColor(1), nil, themeA[1]},
		{1, ansi.BasicColor(1), ansi.BasicColor(4), themeA[4]},
		{2, ansi.IndexedColor(9), nil, themeA[9]},
		{3, ansi.IndexedColor(200), nil, ansi.IndexedColor(200)},
		{4, color.RGBA{R: 1, G: 2, B: 3, A: 0xff}, nil, color.RGBA{R: 1, G: 2, B: 3, A: 0xff}},
	}
	for _, c := range cells {
		cell := e.CellAt(c.x, 0)
		if cell.Style.Fg != c.fg {
			t.Errorf("column %d fg = %T(%v), want %T(%v) kept unresolved", c.x, cell.Style.Fg, cell.Style.Fg, c.fg, c.fg)
		}
		if cell.Style.Bg != c.bg {
			t.Errorf("column %d bg = %T(%v), want %T(%v)", c.x, cell.Style.Bg, cell.Style.Bg, c.bg, c.bg)
		}
		probe := cell.Style.Fg
		if c.bg != nil {
			probe = cell.Style.Bg
		}
		if got := e.ResolveColor(probe); got != c.themedA {
			t.Errorf("column %d resolves to %v under theme A, want %v", c.x, got, c.themedA)
		}
	}

	e.SetThemeColors(color.White, color.Black, color.White, themeB)
	if got := e.ResolveColor(e.CellAt(0, 0).Style.Fg); got != themeB[1] {
		t.Errorf("after switching to theme B the old red resolves to %v, want %v", got, themeB[1])
	}

	e.SetThemeColors(nil, nil, nil, [16]color.Color{})
	if got := e.ResolveColor(e.CellAt(0, 0).Style.Fg); got != ansi.BasicColor(1) {
		t.Errorf("with theming off the old red resolves to %v, want the host palette's red", got)
	}
	_, _ = e.Write([]byte("\r\n\x1b[31mn"))
	if got := e.CellAt(0, 1).Style.Fg; got != ansi.BasicColor(1) {
		t.Errorf("new red after disabling the theme = %T(%v), want BasicColor(1)", got, got)
	}
	if got := e.PaletteColor(1); got != ansi.BasicColor(1) {
		t.Errorf("PaletteColor(1) = %v, want BasicColor(1)", got)
	}
}
