package app

import (
	"image/color"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Cell styling shared by every pane render path.
//
// A cell's style is emitted with uv.Style.String(), which covers every
// attribute the emulator records: bold, faint, italic, slow and rapid blink,
// reverse, conceal, strikethrough, the underline and its style (single,
// double, curly, dotted, dashed) and colour. Hyperlinks travel separately on
// the cell (OSC 8). The focused pane used to rebuild cells through a lipgloss
// style that knew only some of these, so a focused pane silently lost curly
// underlines, blink and, worst, conceal: SGR 8 text became readable.
//
// Colours are compared by kind as well as value. ansi.BasicColor(1) and
// #800000 resolve to the same RGBA, but the first is the host's (or the
// theme's) red and the second a fixed shade, and they render differently.

// colorResolver maps a cell colour to the colour to draw it in. The emulator's
// ResolveColor is the one in use: it applies the active theme to palette
// entries at render time.
type colorResolver func(color.Color) color.Color

// colorKind tags a colour by how a terminal is told about it.
const (
	colorKindNone byte = iota
	colorKindBasic
	colorKindIndexed
	colorKindRGB
)

// classifyColor returns the kind of c and, for palette kinds, its index.
func classifyColor(c color.Color) (kind byte, index uint8) {
	switch v := c.(type) {
	case nil:
		return colorKindNone, 0
	case ansi.BasicColor:
		return colorKindBasic, uint8(v)
	case ansi.IndexedColor:
		return colorKindIndexed, uint8(v)
	}
	return colorKindRGB, 0
}

// colorsIdentical reports whether a and b render identically: same kind and,
// for palette kinds, same index; for anything else, same RGBA.
func colorsIdentical(a, b color.Color) bool {
	// Adjacent cells almost always share the same interface value.
	if a == b {
		return true
	}
	ka, ia := classifyColor(a)
	kb, ib := classifyColor(b)
	if ka != kb {
		return false
	}
	switch ka {
	case colorKindNone:
		return true
	case colorKindBasic, colorKindIndexed:
		return ia == ib
	}
	if !isColorSafe(a) || !isColorSafe(b) {
		return false
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// cellStylesIdentical is uv.Style.Equal with kind-aware colour comparison.
func cellStylesIdentical(a, b *uv.Style) bool {
	return a.Attrs == b.Attrs &&
		a.Underline == b.Underline &&
		colorsIdentical(a.Fg, b.Fg) &&
		colorsIdentical(a.Bg, b.Bg) &&
		colorsIdentical(a.UnderlineColor, b.UnderlineColor)
}

// safeCellColor drops a colour whose RGBA() panics, so a malformed colour
// value degrades to the default colour instead of taking the frame down.
func safeCellColor(c color.Color) color.Color {
	if c == nil || isColorSafe(c) {
		return c
	}
	return nil
}

// resolveCellStyle returns s with its colours resolved for display.
func resolveCellStyle(s uv.Style, resolve colorResolver) uv.Style {
	if resolve != nil {
		s.Fg = resolve(s.Fg)
		s.Bg = resolve(s.Bg)
		s.UnderlineColor = resolve(s.UnderlineColor)
	}
	s.Fg = safeCellColor(s.Fg)
	s.Bg = safeCellColor(s.Bg)
	s.UnderlineColor = safeCellColor(s.UnderlineColor)
	return s
}

// styleMemo remembers the last style resolveCellStyle produced. Runs of cells
// share one raw style, so resolving (a theme lookup per colour plus the
// RGBA() safety checks) once per run instead of once per cell keeps the theme
// at render time from costing a full pass over every cell.
type styleMemo struct {
	raw, resolved uv.Style
	ok, same      bool
	// gen counts misses, so two cells resolved under the same gen are known
	// to share one style without comparing them.
	gen uint64
}

// resolve returns resolveCellStyle(*s, resolver), reusing the previous result
// when s is the same raw style. Colours are compared as interface values, the
// way colorsIdentical's fast path does: the same dynamic type and value.
// same reports that resolving changed nothing (no theme, no unsafe colour), so
// the caller can keep drawing the cell it has instead of a resolved copy.
func (m *styleMemo) resolve(s *uv.Style, resolver colorResolver) (resolved *uv.Style, same bool) {
	if !m.ok || s.Attrs != m.raw.Attrs || s.Underline != m.raw.Underline ||
		s.Fg != m.raw.Fg || s.Bg != m.raw.Bg || s.UnderlineColor != m.raw.UnderlineColor {
		m.raw, m.resolved, m.ok = *s, resolveCellStyle(*s, resolver), true
		m.same = m.resolved.Fg == s.Fg && m.resolved.Bg == s.Bg &&
			m.resolved.UnderlineColor == s.UnderlineColor
		m.gen++
	}
	return &m.resolved, m.same
}

// cursorCellStyle is the style of the fake cursor block drawn over a cell: the
// cell's colours swapped, white on black where a colour is the default. The
// other attributes are dropped so the block reads as a cursor whatever the
// cell carries; conceal in particular would hide the character under it.
func cursorCellStyle(s uv.Style) uv.Style {
	fg := s.Fg
	if fg == nil {
		fg = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	}
	bg := s.Bg
	if bg == nil {
		bg = color.RGBA{A: 0xff}
	}
	return uv.Style{Fg: bg, Bg: fg}
}

// cellSGR returns the escape that switches a fresh pen to s, or "" for the
// default style.
func cellSGR(s *uv.Style) string {
	if s.IsZero() {
		return ""
	}
	return s.String()
}

// cellLineWriter writes a row of emulator cells as styled text, tracking the
// pen and the hyperlink so an escape is emitted only where either changes. It
// is uv's own line renderer (Buffer.Render) with the theme applied to palette
// colours and colours compared by kind, so BasicColor red next to #800000
// still switches pen.
type cellLineWriter struct {
	out     *strings.Builder
	resolve colorResolver
	pen     uv.Style
	link    uv.Link
	pending int // blank default cells not yet written; dropped at end of line
	memo    styleMemo
	penGen  uint64 // memo gen the pen was set from; 0 once the pen is reset
}

func (w *cellLineWriter) begin(out *strings.Builder, resolve colorResolver) {
	w.out = out
	w.resolve = resolve
	w.pen = uv.Style{}
	w.link = uv.Link{}
	w.pending = 0
	w.memo = styleMemo{}
	w.penGen = 0
}

// cell writes c. A nil or zero cell (the continuation column of a wide
// character) writes nothing; the wide character before it already covers it.
func (w *cellLineWriter) cell(c *uv.Cell) {
	if c == nil {
		w.blank()
		return
	}
	if c.IsZero() {
		return
	}
	if c.Style.IsZero() && c.Link.URL == "" && (c.Content == " " || c.Content == "") && c.Width <= 1 {
		w.blank()
		return
	}
	w.flushPending()
	style, _ := w.memo.resolve(&c.Style, w.resolve)
	if w.penGen != w.memo.gen {
		w.setPen(style)
		w.penGen = w.memo.gen
	}
	w.setLink(c.Link)
	if c.Content == "" {
		w.out.WriteByte(' ')
	} else {
		w.out.WriteString(c.Content)
	}
}

// blank records a default-styled space. Trailing ones are trimmed like the
// emulator's own Render does.
func (w *cellLineWriter) blank() {
	if !w.pen.IsZero() {
		w.out.WriteString(ansi.ResetStyle)
		w.pen = uv.Style{}
	}
	w.penGen = 0
	if w.link.URL != "" {
		w.out.WriteString(ansi.ResetHyperlink())
		w.link = uv.Link{}
	}
	w.pending++
}

func (w *cellLineWriter) flushPending() {
	for ; w.pending > 0; w.pending-- {
		w.out.WriteByte(' ')
	}
}

func (w *cellLineWriter) setPen(style *uv.Style) {
	if cellStylesIdentical(style, &w.pen) {
		return
	}
	if !w.pen.IsZero() {
		w.out.WriteString(ansi.ResetStyle)
	}
	w.out.WriteString(cellSGR(style))
	w.pen = *style
}

func (w *cellLineWriter) setLink(link uv.Link) {
	if link == w.link {
		return
	}
	if w.link.URL != "" {
		w.out.WriteString(ansi.ResetHyperlink())
	}
	if link.URL != "" {
		w.out.WriteString(ansi.SetHyperlink(link.URL, link.Params))
	}
	w.link = link
}

// end closes the line: any open hyperlink and pen are reset, trailing blanks
// dropped.
func (w *cellLineWriter) end() {
	if w.link.URL != "" {
		w.out.WriteString(ansi.ResetHyperlink())
		w.link = uv.Link{}
	}
	if !w.pen.IsZero() {
		w.out.WriteString(ansi.ResetStyle)
		w.pen = uv.Style{}
	}
	w.penGen = 0
	w.pending = 0
}

// cellScreen is the part of the emulator a pane render reads.
type cellScreen interface {
	Width() int
	Height() int
	CellAt(x, y int) *uv.Cell
}

// renderCellScreen renders every row of screen, the way the emulator's own
// Render does but with colours resolved through resolve. It is the unfocused
// pane path, so its output must carry exactly the attributes the focused path
// draws.
func renderCellScreen(screen cellScreen, resolve colorResolver) string {
	width, height := screen.Width(), screen.Height()
	var b strings.Builder
	b.Grow(width * height)
	var line cellLineWriter
	for y := range height {
		if y > 0 {
			b.WriteByte('\n')
		}
		line.begin(&b, resolve)
		for x := range width {
			line.cell(screen.CellAt(x, y))
		}
		line.end()
	}
	return b.String()
}
