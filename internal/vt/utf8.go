package vt

import (
	"unicode"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// kittyPlaceholderChar is the base character used by kitty's unicode
// placeholder image protocol (U=1). Apps like yazi emit this character
// with combining diacritical marks to encode image-id/row/column.
// tuios handles kitty graphics via a separate overlay layer, so these
// placeholder characters should be invisible in the text buffer.
const kittyPlaceholderChar = 0x10EEEE

// asciiStr holds the 128 single-byte ASCII strings so the printable-ASCII fast
// path in handlePrint can pass a package-lifetime string to handleGrapheme
// instead of allocating string(r) (which escapes to the heap) for every char.
var asciiStr [128]string

func init() {
	for i := range asciiStr {
		asciiStr[i] = string(rune(i))
	}
}

// openGrapheme records a cluster that was drawn at the end of a Write while
// more of it may still be in flight, along with the cell it landed in.
type openGrapheme struct {
	active bool
	x, y   int
	width  int
}

// handlePrint handles printable characters.
func (e *Emulator) handlePrint(r rune) {
	// Suppress kitty unicode placeholder characters. They would show as
	// garbled text because tuios renders images via its own passthrough
	// layer, not by interpreting placeholder cells.
	if r == kittyPlaceholderChar {
		return
	}
	if r >= ansi.SP && r < ansi.DEL {
		if len(e.grapheme) > 0 {
			// If we have a grapheme buffer, flush it before handling the ASCII character.
			e.flushGrapheme()
		}
		e.handleGrapheme(asciiStr[r], 1)
	} else {
		if len(e.grapheme) == 0 && e.attachToLastCell(r) {
			return
		}
		e.grapheme = append(e.grapheme, r)
		if e.openGrapheme.active {
			e.extendOpenGrapheme()
		}
	}
}

// flushGrapheme flushes the current grapheme buffer, if any, and handles the
// grapheme as a single unit.
func (e *Emulator) flushGrapheme() {
	if len(e.grapheme) == 0 {
		return
	}
	// An open cluster is already on screen; the arriving sequence closes it,
	// so retire the buffer instead of drawing it a second time.
	if e.openGrapheme.active {
		e.openGrapheme.active = false
		e.grapheme = e.grapheme[:0]
		return
	}
	e.renderGraphemeBuffer()
	e.grapheme = e.grapheme[:0] // Reset the grapheme buffer.
}

// renderGraphemeBuffer draws every cluster held in the grapheme buffer. It does
// not clear the buffer; callers decide whether the trailing cluster stays open.
func (e *Emulator) renderGraphemeBuffer() {
	// We always use ansi.GraphemeWidth here to report accurate widths
	// and it's up to the caller to decide how to handle Unicode vs non-Unicode
	// modes.
	method := ansi.GraphemeWidth
	graphemes := string(e.grapheme)
	for len(graphemes) > 0 {
		cluster, width := ansi.FirstGraphemeCluster(graphemes, method)
		e.handleGrapheme(cluster, width)
		graphemes = graphemes[len(cluster):]
	}
}

// flushGraphemeAtWriteEnd draws the buffered clusters when a Write runs out of
// bytes mid-cluster.
//
// A PTY read boundary can fall anywhere, including between a base character and
// its combining marks. The trailing cluster must be drawn now, because the user
// has to see the last character of a burst without waiting for more output, but
// it must also stay open: runes arriving in a later Write belong to that same
// cluster and have to re-render the cell they were split from. Closing the
// cluster here instead would drop the marks already drawn and leave the
// continuation sitting in the next cell.
func (e *Emulator) flushGraphemeAtWriteEnd() {
	if len(e.grapheme) == 0 || e.openGrapheme.active {
		return
	}

	method := ansi.GraphemeWidth
	graphemes := string(e.grapheme)
	var open string
	for len(graphemes) > 0 {
		cluster, width := ansi.FirstGraphemeCluster(graphemes, method)
		e.handleGrapheme(cluster, width)
		graphemes = graphemes[len(cluster):]
		if len(graphemes) == 0 && e.lastCellW > 0 {
			// handleGrapheme records where it actually drew, which is not
			// derivable from the cursor beforehand: a pending wrap makes it
			// index to the next line first.
			open = cluster
			e.openGrapheme = openGrapheme{
				active: true,
				x:      e.lastCellX,
				y:      e.lastCellY,
				width:  width,
			}
		}
	}
	// Keep only the open cluster so a continuation extends it and nothing else.
	e.grapheme = append(e.grapheme[:0], []rune(open)...)
}

// extendOpenGrapheme re-renders the cluster left open by a previous Write, now
// that a continuation rune has arrived, into the cell it was originally drawn
// in rather than at the cursor.
func (e *Emulator) extendOpenGrapheme() {
	method := ansi.GraphemeWidth
	s := string(e.grapheme)
	cluster, width := ansi.FirstGraphemeCluster(s, method)
	if len(cluster) != len(s) {
		// The new rune began a fresh cluster instead of extending the open one.
		// Close the open cluster and leave the remainder buffered for the
		// normal path.
		e.openGrapheme.active = false
		e.grapheme = append(e.grapheme[:0], []rune(s[len(cluster):])...)
		return
	}

	og := e.openGrapheme
	style, link := e.scr.cursorPen(), e.scr.cursorLink()
	if c := e.scr.CellAt(og.x, og.y); c != nil && c.Width == og.width {
		style, link = c.Style, c.Link
	}
	e.lastCellX, e.lastCellY, e.lastCellW, e.lastCellScr = og.x, og.y, og.width, e.scr
	e.redrawLastCell(cluster, width, style, link)
	e.openGrapheme.x, e.openGrapheme.y, e.openGrapheme.width = e.lastCellX, e.lastCellY, e.lastCellW
}

// mayExtendCluster reports whether r is a rune that can only ever continue a
// grapheme cluster: a combining mark, a joiner, a variation selector, an emoji
// modifier or a tag. It is a cheap filter in front of the full segmentation in
// attachToLastCell, so ordinary non-ASCII text does not pay for it.
func mayExtendCluster(r rune) bool {
	switch {
	case r == 0x200C || r == 0x200D, // ZWNJ, ZWJ
		r >= 0xFE00 && r <= 0xFE0F,   // variation selectors
		r >= 0x1F3FB && r <= 0x1F3FF, // emoji skin-tone modifiers
		r >= 0xE0020 && r <= 0xE007F, // tags
		r >= 0xE0100 && r <= 0xE01EF: // variation selectors supplement
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc)
}

// attachToLastCell appends a cluster-extending rune to the cell drawn last,
// and reports whether it did.
//
// The printable-ASCII fast path draws its byte straight into the grid without
// going through the grapheme buffer, so a combining mark that follows an ASCII
// base ("e" U+0301, "1" U+FE0F U+20E3) would otherwise reach the buffer on its
// own and never join its base. Like ghostty and xterm, the mark joins the cell
// the cursor has just left. That only holds while the cursor is still where
// the last print left it; once it has moved, the rune is treated as the start
// of a new cluster.
func (e *Emulator) attachToLastCell(r rune) bool {
	if e.lastCellW <= 0 || e.lastCellScr != e.scr || !mayExtendCluster(r) {
		return false
	}
	cx, cy := e.scr.CursorPosition()
	if cy != e.lastCellY || cx != min(e.lastCellX+e.lastCellW, e.scr.Width()-1) {
		return false
	}
	prev := e.scr.CellAt(e.lastCellX, e.lastCellY)
	if prev == nil || prev.Width != e.lastCellW || prev.Content == "" {
		return false
	}
	s := prev.Content + string(r)
	cluster, width := ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth)
	if len(cluster) != len(s) {
		return false
	}
	e.redrawLastCell(cluster, width, prev.Style, prev.Link)
	return true
}

// redrawLastCell replaces the cell handleGrapheme drew last with a longer
// version of the same cluster, which may be wider than the original.
//
// A cluster that keeps its width is rewritten in place, and the cursor, pending
// wrap included, is left alone: appending a mark in the last column must not
// consume the wrap. A cluster that widens is laid out as if it had arrived
// whole: it grows into the next column or, when it no longer fits before the
// right edge, the columns it held are blanked and it wraps to the next line.
// Without auto-wrap it stays as it was, the way a wide rune that does not fit
// is not printed at all.
func (e *Emulator) redrawLastCell(content string, width int, style uv.Style, link uv.Link) {
	x, y, oldW := e.lastCellX, e.lastCellY, e.lastCellW
	if width <= 0 {
		return
	}
	cell := uv.Cell{Content: content, Width: width, Style: style, Link: link}
	if width == oldW {
		e.scr.SetCell(x, y, &cell)
		e.recordLast(content, width)
		return
	}

	awm := e.autoWrapMode()
	if right := e.scr.Width(); x+width > right {
		if !awm || width > right {
			return
		}
		e.scr.blankCells(x, y, right-x)
		e.scr.setLineWrap(y, SoftWrap(right-x))
		e.scr.setCursor(x, y, false)
		e.index()
		_, y = e.scr.CursorPosition()
		x = 0
	} else if width < oldW {
		e.scr.blankCells(x+width, y, oldW-width)
	}

	e.scr.prepareCell(x, y, width)
	e.scr.SetCell(x, y, &cell)
	e.lastCellX, e.lastCellY, e.lastCellW = x, y, width
	e.recordLast(content, width)
	e.advanceAfterPrint(x, y, width, awm)
}

// recordLast remembers the cluster REP (CSI b) repeats. It is the whole
// cluster with its width, so box drawing, CJK and emoji repeat as themselves.
func (e *Emulator) recordLast(content string, width int) {
	// lastChar only has to be non-zero while there is something to repeat
	// (fullReset zeroes it); the leading byte says that without a decode.
	if content != "" {
		e.lastChar = rune(content[0])
	}
	e.lastGrapheme = content
	e.lastGraphemeWidth = width
}

// advanceAfterPrint moves the cursor past a cell of the given width drawn at
// (x, y). A cell that reaches the right edge leaves the cursor on the last
// column with the wrap pending (when auto-wrap is on), whatever its width: a
// double-width rune in the second-to-last column fills the line just as a
// narrow one in the last column does.
func (e *Emulator) advanceAfterPrint(x, y, width int, awm bool) {
	if right := e.scr.Width(); x+width >= right {
		x = right - 1
		e.atPhantom = awm
	} else {
		x += width
		e.atPhantom = false
	}
	e.scr.setCursor(x, y, false)
}

// handleGrapheme handles UTF-8 graphemes.
func (e *Emulator) handleGrapheme(content string, width int) {
	awm := e.autoWrapMode()
	cell := uv.Cell{
		Content: content,
		Width:   width,
		Style:   e.scr.cursorPen(),
		Link:    e.scr.cursorLink(),
	}

	x, y := e.scr.CursorPosition()
	if e.atPhantom && awm {
		// moves cursor down similar to [Terminal.linefeed] except it doesn't
		// respects [ansi.LNM] mode.
		// This will reset the phantom state i.e. pending wrap state.
		// The row being left is the one that wrapped: from here on it and
		// the next row are one logical line.
		e.scr.setLineWrap(y, SoftWrap(0))
		e.index()
		_, y = e.scr.CursorPosition()
		x = 0
	}

	// Handle character set mappings
	if len(content) == 1 { //nolint:nestif
		var charset CharSet
		c := content[0]
		if e.gsingle > 1 && e.gsingle < 4 {
			charset = e.charsets[e.gsingle]
			e.gsingle = 0
		} else if c < 128 {
			charset = e.charsets[e.gl]
		} else {
			charset = e.charsets[e.gr]
		}

		if charset != nil {
			if r, ok := charset[c]; ok {
				cell.Content = r
				cell.Width = 1
			}
		}
	}

	if cell.Width <= 0 {
		// A cluster with nothing to draw (a lone combining mark with no base
		// to join, a zero-width space): like xterm and ghostty, ignore it
		// rather than plant a zero-width cell that reads as a continuation.
		e.lastCellW = 0
		return
	}

	right := e.scr.Width()
	if x+cell.Width > right {
		// A wide rune in the last column.
		if !awm || cell.Width > right {
			// xterm (and ghostty, which follows it) neither prints the rune
			// nor moves the cursor when it cannot wrap.
			e.lastCellW = 0
			return
		}
		// Blank what is left of the line, as ghostty's spacer head does, and
		// wrap so the rune lands whole at the start of the next one. The
		// blanks are recorded as spacer, so a reflow does not take them for
		// text.
		e.scr.blankCells(x, y, right-x)
		e.scr.setLineWrap(y, SoftWrap(right-x))
		e.scr.setCursor(x, y, false)
		e.index()
		_, y = e.scr.CursorPosition()
		x = 0
	}

	if e.insertMode() {
		// IRM: shift the rest of the line right by the rune's width first.
		e.scr.setCursor(x, y, false)
		e.scr.InsertCell(cell.Width)
	}

	// REP repeats what the guest sent, before charset mapping, as the single
	// rune bookkeeping this replaced did.
	e.recordLast(content, width)

	e.lastCellX, e.lastCellY, e.lastCellW, e.lastCellScr = x, y, cell.Width, e.scr
	if cell.Width != 1 || !e.scr.narrowAt(x, y) {
		e.scr.prepareCell(x, y, cell.Width)
	}
	e.scr.SetCell(x, y, &cell)

	// NOTE: We don't reset the phantom state here, we handle it up above.
	// This is advanceAfterPrint, written out because it runs per character.
	if x+cell.Width >= right {
		x = right - 1
		e.atPhantom = awm
	} else {
		x += cell.Width
		e.atPhantom = false
	}
	e.scr.setCursor(x, y, false)
}
