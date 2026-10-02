package vt

import (
	"sort"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// LineWrap records how a row ends.
//
// The zero value, HardBreak, is a row that ended any way other than by the
// print path running off its right edge: a newline, a cursor move, or simply
// never being written to the end. Any other value is a soft wrap: with DECAWM
// on, a print found the wrap pending (or a wide cluster that did not fit) and
// carried on at the start of the next row, so the two rows are one logical
// line. Spacer is the number of columns at the end of a soft-wrapped row that
// the print path blanked because a wide cluster did not fit there and moved to
// the next row whole, as ghostty's spacer head does. Those columns are padding,
// not content, and a reflow drops them.
type LineWrap uint8

// HardBreak is a row that does not continue on the next one.
const HardBreak LineWrap = 0

// SoftWrap is a row that continues on the next one, ending in spacer columns
// of padding.
func SoftWrap(spacer int) LineWrap {
	return LineWrap(1 + clamp(spacer, 0, 254))
}

// Wrapped reports whether the row continues on the next one.
func (w LineWrap) Wrapped() bool { return w != HardBreak }

// Spacer returns the number of padding columns at the end of a soft-wrapped
// row, and 0 for a hard break.
func (w LineWrap) Spacer() int {
	if w == HardBreak {
		return 0
	}
	return int(w) - 1
}

// isBlankCell reports whether c shows nothing, so that a run of them at the
// end of a row is not content a reflow has to keep on a row of its own.
//
// The colours do not count. A space painted with a background is what an erase
// leaves under BCE, and a shell that erases to the end of the line in its
// prompt colour would otherwise make every such row as long as the screen and
// wrap it onto a second row of colour at any narrower width. Those cells are
// still carried along where the reflowed row has room for them (see
// reflower.emit), so a coloured row keeps its colour as far as it fits, which
// is what the plain truncation before this did. Reverse video, an underline,
// strikethrough or a hyperlink do show on a space, so those cells are kept.
func isBlankCell(c *uv.Cell) bool {
	return c.Width == 1 && (c.Content == " " || c.Content == "") &&
		c.Link.URL == "" && c.Style.Underline == ansi.UnderlineNone &&
		c.Style.Attrs&(uv.AttrReverse|uv.AttrStrikethrough) == 0
}

// trimmedLen is the length of row without its trailing blank cells.
func trimmedLen(row uv.Line) int {
	n := len(row)
	for n > 0 && isBlankCell(&row[n-1]) {
		n--
	}
	return n
}

// reflowLine is one logical line: a run of soft-wrapped source rows ended by
// a hard break (or by the end of the input), and where it lands once laid out
// again.
type reflowLine struct {
	first, last int // source rows, inclusive
	start       int // first output row
	count       int // output rows
}

// reflower lays a run of rows out again at a new width.
//
// The rows are grouped into logical lines by their wrap flags, and each line is
// split again at the new width the way the print path would have split it had
// the text arrived at that width: a wide cluster never straddles the edge but
// moves to the next row whole, leaving spacer columns, and a cluster wider than
// the whole row (a wide rune at width 1) becomes a blank, which is as close as
// a grid can get to xterm not printing it. Cells are copied as they are, so
// styles, hyperlinks and grapheme clusters come through unchanged.
//
// Laying out is split from building so the caller can decide which output rows
// to keep before any of them is allocated. The scrollback has a line limit and a
// narrowing can multiply the row count, so only the rows that survive are ever
// materialised.
//
// A reflower is reusable: reset lays out a new input in the slices the last one
// left, and release lets go of the rows it held. A screen keeps one, so a pane
// being dragged wider and narrower does not allocate its bookkeeping on every
// step.
type reflower struct {
	rows  []uv.Line
	wraps []LineWrap
	width int

	lines   []reflowLine
	rowLine []int32 // source row -> index into lines
	content []int32 // source row -> contentLen, worked out once
	total   int     // output rows

	// free holds source rows build has finished reading, for emit to lay
	// later rows out in. Resizing back and forth, a pane's long lines are
	// rebuilt on every step, and recycling their rows instead of allocating
	// fresh ones keeps a drag from churning the heap.
	free []uv.Line

	// out and outWraps hold what build returns, valid until the next reset.
	out      []uv.Line
	outWraps []LineWrap
}

// newReflower is reset on a new reflower.
func newReflower(rows []uv.Line, wraps []LineWrap, width, extRow, extLen int) *reflower {
	r := &reflower{}
	r.reset(rows, wraps, width, extRow, extLen)
	return r
}

// reset groups rows into logical lines and counts the rows each takes at
// width, keeping the spare rows release left in free. Row extRow (the cursor's) counts as content up to at least extLen
// cells, so the blanks between the text and the cursor survive; pass -1, 0 for
// none.
func (r *reflower) reset(rows []uv.Line, wraps []LineWrap, width, extRow, extLen int) {
	r.rows, r.wraps, r.width, r.total = rows, wraps, max(width, 1), 0
	r.lines = r.lines[:0]
	r.rowLine = resizeInt32(r.rowLine, len(rows))
	r.content = resizeInt32(r.content, len(rows))
	for i := 0; i < len(rows); {
		j := i
		for j < len(rows)-1 && wraps[j].Wrapped() {
			j++
		}
		idx := int32(len(r.lines))
		for k := i; k <= j; k++ {
			r.rowLine[k] = idx
		}
		r.lines = append(r.lines, reflowLine{first: i, last: j})
		i = j + 1
	}

	// The cursor's own line keeps the blanks up to the cursor, so a prompt
	// ending in a space still ends in one; a line with no text at all has
	// nothing to keep the cursor in step with, and keeps it on its one row.
	for i, row := range rows {
		if w := wraps[i]; w.Wrapped() {
			r.content[i] = int32(max(len(row)-w.Spacer(), 0))
		} else {
			r.content[i] = int32(trimmedLen(row))
		}
	}
	if extRow >= 0 && extRow < len(rows) && !wraps[extRow].Wrapped() {
		l := r.lines[r.rowLine[extRow]]
		if l.first != l.last || r.content[extRow] > 0 {
			ext := min(extLen, len(rows[extRow]))
			r.content[extRow] = int32(max(int(r.content[extRow]), ext))
		}
	}

	for i := range r.lines {
		l := &r.lines[i]
		l.start = r.total
		l.count = r.countRows(l)
		r.total += l.count
	}
}

// release drops the rows the reflower refers to, so the scratch it keeps for
// next time does not keep rows alive that the screen and the scrollback have
// let go of, except for up to keep spare rows in free. Those are rows nothing
// else holds, kept for the next reflow to lay rows out in: a pane dragged back
// and forth gives up rows when it widens and needs them again when it
// narrows, and keeping a screenful of them is what keeps the drag from
// allocating every step.
func (r *reflower) release(keep int) {
	if len(r.free) > keep {
		clear(r.free[keep:])
		r.free = r.free[:keep]
	}
	clear(r.out)
	r.rows, r.wraps = nil, nil
	r.out, r.outWraps = r.out[:0], r.outWraps[:0]
}

// resizeInt32 returns s at length n, reusing its storage when it can.
func resizeInt32(s []int32, n int) []int32 {
	if cap(s) >= n {
		return s[:n]
	}
	return make([]int32, n)
}

// contentLen is how many leading cells of source row i are content: all but
// the spacer of a soft-wrapped row, and up to the last cell that is not blank
// of any other, or up to the cursor on the cursor's row.
func (r *reflower) contentLen(i int) int {
	return int(r.content[i])
}

// fits reports whether l is a single hard row whose content fits the new width,
// the overwhelmingly common case, which keeps its source row as it is.
func (r *reflower) fits(l *reflowLine) bool {
	return l.first == l.last && !r.wraps[l.first].Wrapped() && r.contentLen(l.first) <= r.width
}

// groupWidth is the number of source cells the cluster at x occupies: its
// width for a whole wide rune, 1 otherwise. A continuation with no lead, or a
// lead missing its continuation, counts as one cell and is blanked on build.
func groupWidth(row uv.Line, x, n int) int {
	w := row[x].Width
	if w <= 1 || x+w > n {
		return 1
	}
	return w
}

// visitFn is called by walk for each cluster of a logical line: the source row
// and column it comes from, the source cells it spans, and the output row
// (relative to the line) and column it lands on with the width it takes there.
// Returning false stops the walk.
type visitFn func(srcRow, srcX, gw, row, col, pw int) bool

// walk lays l out at the reflower's width, calling visit (if not nil) for
// every cluster, and returns the row and column the layout ends on. The column
// can equal the width: a full row is only broken when another cluster arrives.
func (r *reflower) walk(l *reflowLine, visit visitFn) (row, col int) {
	width := r.width
	for i := l.first; i <= l.last; i++ {
		src := r.rows[i]
		n := r.contentLen(i)
		for x := 0; x < n; {
			gw := groupWidth(src, x, n)
			pw := gw
			if pw > width {
				pw = 1
			}
			if col+pw > width {
				row++
				col = 0
			}
			if visit != nil && !visit(i, x, gw, row, col, pw) {
				return row, col
			}
			col += pw
			x += gw
		}
	}
	return row, col
}

// countRows returns how many output rows l takes.
func (r *reflower) countRows(l *reflowLine) int {
	if r.fits(l) {
		return 1
	}
	row, _ := r.walk(l, nil)
	return row + 1
}

// locate maps source position (srcRow, srcX) to its output row and column.
//
// A position on content lands on the cell its cluster went to. One past the
// content (the cursor after the last thing printed) lands after it, and when
// that is exactly the end of a full row the position becomes the last column
// with the wrap pending, which is where the print path would have left the
// cursor at the new width; atEnd reports that case. Further out, in the blanks
// of a row, it keeps its distance from the content up to the last column of
// the line's last row: it may not move onto a row of the line below.
func (r *reflower) locate(srcRow, srcX int) (row, col int, atEnd bool) {
	if len(r.rows) == 0 {
		return 0, 0, false
	}
	srcRow = clamp(srcRow, 0, len(r.rows)-1)
	l := &r.lines[r.rowLine[srcRow]]
	width := r.width

	if r.fits(l) {
		n := r.contentLen(l.first)
		switch {
		case srcX < n:
			return l.start, srcX, false
		case srcX == n && n == width:
			return l.start, width - 1, true
		default:
			return l.start, min(srcX, width-1), false
		}
	}

	// The clusters of a row cover its content without gaps, so a position not
	// on one of them is past the content, and endRow, endCol is then where the
	// layout stood after the last cluster up to it (0, 0 if there was none).
	found := false
	endRow, endCol := 0, 0
	r.walk(l, func(sr, sx, gw, orow, ocol, pw int) bool {
		if sr > srcRow {
			return false
		}
		if sr == srcRow && srcX >= sx && srcX < sx+gw {
			row, col = orow, ocol+min(srcX-sx, pw-1)
			found = true
			return false
		}
		endRow, endCol = orow, ocol+pw
		return true
	})
	if found {
		return l.start + row, col, false
	}
	extra := max(srcX-r.contentLen(srcRow), 0)
	if extra == 0 && endCol == width {
		return l.start + endRow, width - 1, true
	}
	if endCol >= width && extra > 0 {
		// Past a full row: the blanks continue on the next row of the line if
		// it has one, and stay on the last column otherwise.
		if endRow+1 < l.count {
			return l.start + endRow + 1, min(extra-1, width-1), false
		}
	}
	return l.start + endRow, min(endCol+extra, width-1), false
}

// build materialises output rows [from, to). Rows from exactFrom on are made
// exactly the new width, as screen rows must be; rows before it may keep a
// source row of another width when its content fits, as scrollback rows can,
// which keeps the common case free of copying.
//
// It recycles the source rows of the lines it rebuilds, so it is the last thing
// done with a reflower: walk, and so locate, cannot run after it.
func (r *reflower) build(from, to, exactFrom int) ([]uv.Line, []LineWrap) {
	out, wraps := r.out[:0], r.outWraps[:0]
	defer func() { r.out, r.outWraps = out, wraps }()
	if to <= from {
		return out, wraps
	}
	li := sort.Search(len(r.lines), func(i int) bool {
		return r.lines[i].start+r.lines[i].count > from
	})
	for ; li < len(r.lines) && r.lines[li].start < to; li++ {
		l := &r.lines[li]
		if r.fits(l) {
			row := r.rows[l.first]
			if l.start >= exactFrom {
				row = fitWidth(row, r.width)
			}
			out = append(out, row)
			wraps = append(wraps, HardBreak)
			continue
		}
		out, wraps = r.emit(l, from, to, out, wraps)
		// emit copied out of these rows and never hands one back, so nothing
		// reads them again.
		r.free = append(r.free, r.rows[l.first:l.last+1]...)
	}
	return out, wraps
}

// newRow returns storage for one output row, recycled when it can be. Every
// cell of it is written by its caller.
func (r *reflower) newRow() uv.Line {
	for len(r.free) > 0 {
		row := r.free[len(r.free)-1]
		r.free = r.free[:len(r.free)-1]
		if cap(row) >= r.width {
			return row[:r.width]
		}
	}
	// Nothing to recycle: allocate a slab of rows at once, the rest going
	// into free for the rows that follow, since a reflow that runs out needs
	// many. Each row is capped so it cannot grow into its neighbour, and has
	// a little room to spare so it can be recycled when the pane is next made
	// a few columns wider, which is what a drag does.
	const slab = 16
	c := r.width + r.width/8 + 4
	cells := make([]uv.Cell, slab*c)
	for i := slab - 1; i > 0; i-- {
		r.free = append(r.free, cells[i*c:i*c:(i+1)*c])
	}
	return cells[0:r.width:c]
}

// emit lays l out and appends the rows of it that fall in [from, to).
func (r *reflower) emit(l *reflowLine, from, to int, out []uv.Line, wraps []LineWrap) ([]uv.Line, []LineWrap) {
	width := r.width
	var cur uv.Line
	curRow := -1 // relative row cur holds, -1 when none
	curEnd := 0  // columns of cur in use

	inRange := func(rel int) bool {
		abs := l.start + rel
		return abs >= from && abs < to
	}
	flush := func(wrap LineWrap) {
		if cur == nil {
			return
		}
		for x := curEnd; x < width; x++ {
			cur[x] = uv.EmptyCell
		}
		out = append(out, cur)
		wraps = append(wraps, wrap)
		cur = nil
	}

	r.walk(l, func(srcRow, srcX, gw, row, col, pw int) bool {
		if l.start+row >= to {
			return false
		}
		if row != curRow {
			// The previous row broke here: whatever it did not fill is spacer.
			flush(SoftWrap(width - curEnd))
			curRow, curEnd = row, 0
			if inRange(row) {
				cur = r.newRow()
			}
		}
		if cur != nil {
			src := r.rows[srcRow]
			if pw == gw && src[srcX].Width == gw {
				copy(cur[col:col+gw], src[srcX:srcX+gw])
			} else {
				// A broken pair, or a rune wider than the whole row.
				blank := uv.EmptyCell
				blank.Style = src[srcX].Style
				cur[col] = blank
			}
		}
		curEnd = col + pw
		return true
	})

	if curRow < 0 {
		// An empty line is one blank row.
		curRow = 0
		if inRange(0) {
			cur = r.newRow()
		}
	}
	if cur == nil {
		return out, wraps
	}
	if lastRow := l.count - 1; curRow != lastRow {
		// Stopped early at to, on a row that broke where the next cluster did
		// not fit.
		flush(SoftWrap(width - curEnd))
		return out, wraps
	}
	if w := r.wraps[l.last]; w.Wrapped() {
		// The input ended mid-line: the rest is on a row this reflow does not
		// hold (the screen, for a scrollback-only reflow).
		flush(SoftWrap(width - curEnd))
		return out, wraps
	}
	// The last row keeps what followed the content in its source row, the
	// blanks with their colours, as far as there is room for them.
	src := r.rows[l.last]
	for k := r.contentLen(l.last); k < len(src) && curEnd < width; k++ {
		cur[curEnd] = src[k]
		curEnd++
	}
	flush(HardBreak)
	return out, wraps
}

// fitWidth returns row at exactly width cells. Only called on a row whose
// content fits, so cutting it drops blanks and never half a wide rune.
func fitWidth(row uv.Line, width int) uv.Line {
	n := len(row)
	switch {
	case n == width:
		return row
	case n > width:
		return row[:width]
	case cap(row) >= width:
		row = row[:width]
	default:
		// Grown the way append grows, with room to spare, so a pane dragged
		// wider a column at a time does not reallocate every row every step.
		row = append(row, make(uv.Line, width-n)...)
	}
	for x := n; x < width; x++ {
		row[x] = uv.EmptyCell
	}
	return row
}

// blankLine returns a new row of width empty cells.
func blankLine(width int) uv.Line {
	row := make(uv.Line, width)
	for x := range row {
		row[x] = uv.EmptyCell
	}
	return row
}

// reflowResize resizes the screen to width x height, laying its rows and its
// scrollback out again at the new width the way ghostty, kitty and wezterm do.
// It is only for the normal screen: the alternate screen belongs to a program
// that repaints it, and is never reflowed. It reports whether the cursor ended
// up with the wrap pending (see reflower.locate); the caller owns that state.
//
// The scrollback and the screen are one run of rows for this, since a logical
// line can start in the scrollback and continue on the screen. Once laid out,
// the screen starts on the row that holds what was in its top-left cell or, if
// the text above the cursor shrank, as far up the scrollback as keeps the
// cursor on its screen row. When the cursor would then be below the bottom,
// the screen moves down to keep it on the last row and what leaves the top goes
// into the scrollback, as resizeKeepingCursor does for a height change. Rows
// that fall off the bottom are blank in the usual case (the content grew into
// the space under the cursor) and dropped either way, and rows the screen has
// left over are blank. The scrollback keeps its line limit: when the reflow makes more
// rows than it holds, the oldest are dropped without ever being built.
//
// The cursor keeps the character it was on, the saved cursor (DECSC) the same,
// and the semantic markers in markers move with the text they mark.
func (s *Screen) reflowResize(width, height int, markers *SemanticMarkerList) bool {
	oldH := s.buf.Height()
	sc := &s.reflowScratch
	rows, wraps := sc.rows[:0], sc.wraps[:0]
	if s.scrollback != nil {
		rows, wraps = s.scrollback.appendTo(rows, wraps)
	}
	base := len(rows)
	rows = append(rows, s.buf.Lines...)
	wraps = append(wraps, s.wraps...)
	defer func() {
		clear(rows)
		sc.rows, sc.wraps = rows[:0], wraps[:0]
		sc.r.release(max(2*height, sc.r.total/8))
	}()

	curY := clamp(s.cur.Y, 0, oldH-1)
	savedY := clamp(s.saved.Y, 0, oldH-1)
	r := &sc.r
	r.reset(rows, wraps, width, base+curY, s.cur.X)

	top, _, _ := r.locate(base, 0)
	cy, cx, atEnd := r.locate(base+curY, s.cur.X)
	sy, sx, _ := r.locate(base+savedY, s.saved.X)

	// The cursor keeps its screen row when it can. Text that grew above it
	// (narrowing) pushes it down into the room under it rather than pushing
	// the top into the scrollback, and text that shrank (widening) brings
	// rows back down from the scrollback rather than leaving the cursor
	// higher up the screen, so a narrowing and the widening back are a round
	// trip. A cursor that would fall off the bottom stays on the last row.
	screenTop := max(cy-(height-1), min(top, cy-curY), 0)
	from := screenTop
	if s.scrollback != nil {
		from = max(0, screenTop-s.scrollback.MaxLines())
	}
	end := min(screenTop+height, r.total)

	// Before build, which recycles the rows locate reads.
	if markers != nil && markers.Len() > 0 {
		markers.remap(func(line, col int) (int, int, bool) {
			if line < 0 || line >= len(rows) {
				return 0, 0, false
			}
			row, c, _ := r.locate(line, col)
			if row < from || row >= end {
				return 0, 0, false
			}
			return row - from, c, true
		})
	}

	out, outWraps := r.build(from, end, screenTop)

	kept := screenTop - from
	if s.scrollback != nil {
		s.scrollback.replace(out[:kept], outWraps[:kept])
		s.scrollback.lastWidthCaptured = width
	}

	// The screen's own slices are free to reuse: rows holds its old rows.
	lines := s.buf.Lines[:0]
	lines = append(lines, out[kept:]...)
	newWraps := append(s.wraps[:0], outWraps[kept:]...)
	for len(lines) < height {
		row := r.newRow()
		for x := range row {
			row[x] = uv.EmptyCell
		}
		lines = append(lines, row)
		newWraps = append(newWraps, HardBreak)
	}
	clear(lines[len(lines):cap(lines)])
	s.buf.Lines = lines
	s.wraps = newWraps
	if len(s.buf.Touched) != height {
		s.buf.Touched = make([]*uv.LineData, height)
	}
	for y := range height {
		s.buf.TouchLine(0, y, width)
	}
	s.scroll = s.buf.Bounds()

	s.cur.X, s.cur.Y = cx, cy-screenTop
	s.saved.Position = s.clampPosition(uv.Pos(sx, sy-screenTop))
	return atEnd
}

// reflowScratch is what a screen keeps between reflows (see reflower).
type reflowScratch struct {
	rows  []uv.Line
	wraps []LineWrap
	r     reflower
}
