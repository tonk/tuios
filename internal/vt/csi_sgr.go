package vt

import (
	"image/color"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// parseStyleColor parses an extended (38/48/58) color from SGR params.
// Returns the color and the number of extra params consumed (to add to loop
// index). A palette entry stays a palette entry (ansi.IndexedColor): it is
// resolved through the theme when the cell is drawn, not here.
func parseStyleColor(params ansi.Params, i int) (color.Color, int) {
	var c color.Color
	n := ansi.ReadStyleColor(params[i:], &c)
	if n > 0 {
		return c, n - 1
	}
	return nil, 0
}

// handleSgr handles Select Graphic Rendition (SGR) escape sequences.
//
// Cells keep palette colors as palette colors (ansi.BasicColor for 0-15,
// ansi.IndexedColor for 38;5;n) with or without a theme; the renderer
// resolves them through ResolveColor every frame, so a theme switch also
// recolors output already on screen. With no theme step left at write time,
// the same reader serves both cases, and it knows codes uv.ReadStyle does not
// (SGR 21, double underline).
func (e *Emulator) handleSgr(params ansi.Params) {
	readStyle(params, &e.scr.cur.Pen)
}

// readStyle reads SGR sequences into pen. It is based on uv.ReadStyle and
// stores the same colour values it does; the theme is applied at render time
// (see ResolveColor).
func readStyle(params ansi.Params, pen *uv.Style) {
	if len(params) == 0 {
		*pen = uv.Style{}
		return
	}

	for i := 0; i < len(params); i++ {
		param, hasMore, _ := params.Param(i, 0)
		switch param {
		case 0: // Reset
			*pen = uv.Style{}
		case 1: // Bold
			pen.Attrs |= uv.AttrBold
		case 2: // Dim/Faint
			pen.Attrs |= uv.AttrFaint
		case 3: // Italic
			pen.Attrs |= uv.AttrItalic
		case 4: // Underline
			nextParam, _, ok := params.Param(i+1, 0)
			if hasMore && ok {
				// A colon subparameter follows (e.g. 4:3). Always consume it,
				// even when the style value is out of range, so a stray value
				// like 4:7 is not reinterpreted as a separate SGR (7 reverse).
				i++
				switch nextParam {
				case 0:
					pen.Underline = ansi.UnderlineNone
				case 1:
					pen.Underline = ansi.UnderlineSingle
				case 2:
					pen.Underline = ansi.UnderlineDouble
				case 3:
					pen.Underline = ansi.UnderlineCurly
				case 4:
					pen.Underline = ansi.UnderlineDotted
				case 5:
					pen.Underline = ansi.UnderlineDashed
				default:
					// Unknown underline style: no-op, but still consumed above.
				}
			} else {
				pen.Underline = ansi.UnderlineSingle
			}
		case 5: // Slow Blink
			pen.Attrs |= uv.AttrBlink
		case 6: // Rapid Blink
			pen.Attrs |= uv.AttrRapidBlink
		case 7: // Reverse
			pen.Attrs |= uv.AttrReverse
		case 8: // Conceal
			pen.Attrs |= uv.AttrConceal
		case 9: // Crossed-out/Strikethrough
			pen.Attrs |= uv.AttrStrikethrough
		case 21: // Doubly underlined (xterm, ECMA-48)
			pen.Underline = ansi.UnderlineDouble
		case 22: // Normal Intensity
			pen.Attrs &^= uv.AttrBold | uv.AttrFaint
		case 23: // Not italic
			pen.Attrs &^= uv.AttrItalic
		case 24: // Not underlined
			pen.Underline = ansi.UnderlineNone
		case 25: // Blink off
			pen.Attrs &^= uv.AttrBlink | uv.AttrRapidBlink
		case 27: // Positive (not reverse)
			pen.Attrs &^= uv.AttrReverse
		case 28: // Reveal
			pen.Attrs &^= uv.AttrConceal
		case 29: // Not crossed out
			pen.Attrs &^= uv.AttrStrikethrough
		case 30, 31, 32, 33, 34, 35, 36, 37: // Set foreground (palette 0-7)
			pen.Fg = ansi.BasicColor(param - 30) //nolint:gosec
		case 38: // Set foreground 256 or truecolor
			if c, skip := parseStyleColor(params, i); c != nil {
				pen.Fg = c
				i += skip
			}
		case 39: // Default foreground
			pen.Fg = nil
		case 40, 41, 42, 43, 44, 45, 46, 47: // Set background (palette 0-7)
			pen.Bg = ansi.BasicColor(param - 40) //nolint:gosec
		case 48: // Set background 256 or truecolor
			if c, skip := parseStyleColor(params, i); c != nil {
				pen.Bg = c
				i += skip
			}
		case 49: // Default Background
			pen.Bg = nil
		case 58: // Set underline color
			if c, skip := parseStyleColor(params, i); c != nil {
				pen.UnderlineColor = c
				i += skip
			}
		case 59: // Default underline color
			pen.UnderlineColor = nil
		case 90, 91, 92, 93, 94, 95, 96, 97: // Set bright foreground (palette 8-15)
			pen.Fg = ansi.BasicColor(param - 90 + 8) //nolint:gosec
		case 100, 101, 102, 103, 104, 105, 106, 107: // Set bright background (palette 8-15)
			pen.Bg = ansi.BasicColor(param - 100 + 8) //nolint:gosec
		default:
			// Delegate any scalar attribute code this switch does not
			// special-case to the canonical uv reader, so the themed path
			// stays attribute-complete with the non-themed path. Color codes
			// (38/48/58) and their subparameters are handled above, so this
			// only sees single scalar codes.
			uv.ReadStyle(params[i:i+1], pen)
		}
	}
}
