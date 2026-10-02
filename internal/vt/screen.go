package vt

import (
	uv "github.com/charmbracelet/ultraviolet"
)

// Screen represents a virtual terminal screen.
type Screen struct {
	// cb is the callbacks struct to use.
	cb *Callbacks
	// The buffer of the screen.
	buf *uv.RenderBuffer
	// The cur of the screen.
	cur, saved Cursor
	// savedState is the rest of what DECSC saved alongside saved.
	savedState savedCursorState
	// phantom holds the emulator's pending-wrap flag for this screen while
	// the other screen is active, so a screen switch neither carries a wrap
	// pending on one screen over to the other nor loses it.
	phantom bool
	// scroll is the scroll region.
	scroll uv.Rectangle
	// scrollback is the scrollback buffer for lines that have scrolled off the top.
	scrollback *Scrollback
	// wraps holds, per row, how the row ends: whether the print path wrapped
	// from it onto the next row (see LineWrap). It moves with the rows, goes
	// into the scrollback with them, and is what a reflow joins rows by.
	wraps []LineWrap
	// reflowScratch is kept between reflows so a resize does not allocate it.
	reflowScratch reflowScratch
}

// NewScreen creates a new screen.
func NewScreen(w, h int) *Screen {
	s := Screen{}
	s.scrollback = NewScrollback(0) // Use default size
	s.buf = uv.NewRenderBuffer(w, h)
	s.wraps = make([]LineWrap, s.buf.Height())
	s.scroll = s.buf.Bounds()
	return &s
}

// Reset resets the screen.
// It clears the screen, sets the cursor to the top left corner, reset the
// cursor styles, and resets the scroll region.
func (s *Screen) Reset() {
	s.buf.Clear()
	clear(s.wraps)
	s.cur = Cursor{}
	s.saved = Cursor{}
	s.savedState = savedCursorState{}
	s.phantom = false
	s.scroll = s.buf.Bounds()
}

// Bounds returns the bounds of the screen.
func (s *Screen) Bounds() uv.Rectangle {
	return s.buf.Bounds()
}

// Touched returns touched lines in the screen buffer.
func (s *Screen) Touched() []*uv.LineData {
	return s.buf.Touched
}

// CellAt returns the cell at the given x, y position.
func (s *Screen) CellAt(x int, y int) *uv.Cell {
	return s.buf.CellAt(x, y)
}

// SetCell sets the cell at the given x, y position.
func (s *Screen) SetCell(x, y int, c *uv.Cell) {
	s.buf.SetCell(x, y, c)
}

// prepareCell clears the way for a cell of width w about to be written at
// (x, y), so no wide rune is left with only one of its halves.
//
// The buffer's Set repairs a wide rune whose lead is overwritten, but not one
// whose lead sits under the last column of a wider new cell, and it blanks the
// broken half with the old rune's attributes. Writing "世" over the lead of the
// second of three "中" left the second 中's continuation standing on its own,
// which a reader takes for part of the cell before it. Here every rune the new
// cell cuts is broken up first, its leftover halves becoming erase cells in
// the current background like the ones EL and ECH produce, and every cell the
// new one covers is vacated so Set has nothing left to repair in the old style.
//
// The common case, a narrow cell over a narrow cell, costs one comparison.
func (s *Screen) prepareCell(x, y, w int) {
	if y < 0 || y >= len(s.buf.Lines) {
		return
	}
	line := s.buf.Lines[y]
	if x < 0 || x >= len(line) || (w <= 1 && line[x].Width == 1) {
		return
	}
	s.prepareCellSlow(line, x, y, w)
}

// narrowAt reports whether (x, y) holds an ordinary one-column cell, the case
// in which a narrow write needs no prepareCell. Small enough to inline.
func (s *Screen) narrowAt(x, y int) bool {
	lines := s.buf.Lines
	return y >= 0 && y < len(lines) && x >= 0 && x < len(lines[y]) && lines[y][x].Width == 1
}

// prepareCellSlow is the part of prepareCell that actually cuts runes, kept
// out of line so the check in front of it inlines into the print path.
func (s *Screen) prepareCellSlow(line uv.Line, x, y, w int) {
	end := min(x+max(w, 1), len(line))
	lo, hi := x, end

	blank := uv.EmptyCell
	if c := s.blankCell(); c != nil {
		blank = *c
	}

	// A continuation under the new cell's first column: the lead to its left
	// loses its second half.
	if line[x].Width == 0 {
		for j := x - 1; j >= 0; j-- {
			if lw := line[j].Width; lw != 0 {
				if lw > 1 && j+lw > x {
					for k := j; k < x; k++ {
						line[k] = blank
					}
					lo = j
				}
				break
			}
		}
	}
	// A lead under the new cell whose continuation reaches past it.
	for i := x; i < end; i++ {
		if cw := line[i].Width; cw > 1 && i+cw > end {
			stop := min(i+cw, len(line))
			for k := end; k < stop; k++ {
				line[k] = blank
			}
			hi = max(hi, stop)
		}
	}
	for i := x; i < end; i++ {
		if line[i].Width != 1 {
			line[i] = blank
		}
	}
	s.buf.TouchLine(lo, y, hi-lo)
}

// blankCells overwrites n cells from (x, y) with the erase cell, breaking up
// any wide rune the run cuts.
func (s *Screen) blankCells(x, y, n int) {
	if n <= 0 || y < 0 || y >= len(s.buf.Lines) {
		return
	}
	s.prepareCell(x, y, n)
	line := s.buf.Lines[y]
	blank := uv.EmptyCell
	if c := s.blankCell(); c != nil {
		blank = *c
	}
	for i := max(x, 0); i < x+n && i < len(line); i++ {
		line[i] = blank
	}
	s.buf.TouchLine(x, y, n)
}

// Height returns the height of the screen.
func (s *Screen) Height() int {
	return s.buf.Height()
}

// Resize resizes the screen, cutting or padding rows without reflowing them.
func (s *Screen) Resize(width int, height int) {
	widthChanged := width != s.buf.Width()
	s.buf.Resize(width, height)
	s.resizeWraps(s.buf.Height())
	if widthChanged {
		// A row cut or padded to a new width no longer ends where it wrapped.
		clear(s.wraps)
	}
	// Resize the Touched slice to match the new height.
	if h := s.buf.Height(); len(s.buf.Touched) != h {
		s.buf.Touched = make([]*uv.LineData, h)
	}
	s.blankWideRunesCutByTheEdge()
	s.scroll = s.buf.Bounds()
}

// resizeKeepingCursor resizes the screen the way a terminal resize does, for
// the active and the inactive screen alike.
//
// When the height shrinks below the cursor row, the whole screen scrolls up so
// that row stays on the bottom line, the rows leaving the top going into the
// scrollback (the alternate screen keeps none, so it drops them). That scroll
// ignores the guest's DECSTBM region: the region is about to be reset to the
// full screen anyway, as xterm and ghostty reset it on resize, and scrolling
// only inside it would delete the rows the cursor is meant to keep. The cursor
// and the saved cursor (DECSC) follow the content they were on and are then
// clamped into the new bounds, so neither a later print nor a DECRC lands
// outside the grid. Wide runes cut by a narrower edge are blanked.
//
// It does not reflow. The normal screen goes through reflowResize when the
// width changes, and this is what is left: a height-only change of it, and
// every change of the alternate screen.
func (s *Screen) resizeKeepingCursor(width, height int) {
	if n := s.cur.Y - (height - 1); n > 0 && height > 0 && s.buf.Height() > height {
		s.scroll = s.buf.Bounds()
		s.rotateWholeScreenUp(n, s.scrollback != nil)
		s.cur.Y -= n
		s.saved.Y -= n
	}
	s.Resize(width, height)
	s.cur.Position = s.clampPosition(s.cur.Position)
	s.saved.Position = s.clampPosition(s.saved.Position)
}

// resizeWraps keeps wraps one entry per row, the way Buffer.Resize keeps the
// rows: dropping from the bottom, and adding hard breaks there.
func (s *Screen) resizeWraps(height int) {
	if len(s.wraps) >= height {
		s.wraps = s.wraps[:height]
		return
	}
	s.wraps = append(s.wraps, make([]LineWrap, height-len(s.wraps))...)
}

// lineWrap returns how row y ends.
func (s *Screen) lineWrap(y int) LineWrap {
	if y < 0 || y >= len(s.wraps) {
		return HardBreak
	}
	return s.wraps[y]
}

// setLineWrap records how row y ends.
func (s *Screen) setLineWrap(y int, w LineWrap) {
	if y >= 0 && y < len(s.wraps) {
		s.wraps[y] = w
	}
}

// unwrapArea drops the soft wrap of every row whose end area erases. A row
// whose last column has been cleared no longer runs on into the next one: xterm
// and ghostty reset the wrap on EL and ED for the same reason. An erase that
// stops short of the last column leaves the wrap alone, since the text at the
// edge, which is what continues, is still there.
func (s *Screen) unwrapArea(area uv.Rectangle) {
	area = area.Intersect(s.buf.Bounds())
	if area.Empty() || area.Max.X < s.buf.Width() {
		return
	}
	for y := area.Min.Y; y < area.Max.Y && y < len(s.wraps); y++ {
		s.wraps[y] = HardBreak
	}
}

// shiftWraps moves the wrap flags of rows y and below by n rows within area,
// down when insert is set (IL) and up otherwise (DL), the way InsertLineArea and
// DeleteLineArea move the rows' cells, and clears the flags of the rows they
// blank. The flag is about a row's last column: a region that does not reach
// it leaves every row ending as it did, and one that reaches it without
// starting at the first column splices rows together, so their flags are
// cleared rather than moved.
func (s *Screen) shiftWraps(y, n int, area uv.Rectangle, insert bool) {
	if n <= 0 || y < area.Min.Y || y >= area.Max.Y || area.Max.X < s.buf.Width() {
		return
	}
	bottom := min(area.Max.Y, len(s.wraps))
	if y >= bottom {
		return
	}
	n = min(n, bottom-y)
	w := s.wraps
	switch {
	case area.Min.X > 0:
		clear(w[y:bottom])
	case insert:
		copy(w[y+n:bottom], w[y:bottom-n])
		clear(w[y : y+n])
	default:
		copy(w[y:bottom-n], w[y+n:bottom])
		clear(w[bottom-n : bottom])
	}
}

// clampPosition clamps p into the screen bounds.
func (s *Screen) clampPosition(p uv.Position) uv.Position {
	return uv.Pos(
		clamp(p.X, 0, max(s.buf.Width()-1, 0)),
		clamp(p.Y, 0, max(s.buf.Height()-1, 0)),
	)
}

// blankWideRunesCutByTheEdge clears a double-width rune left sitting in the
// last column by a narrowing resize.
//
// Dropping columns takes away the continuation cell a wide rune needs without
// touching the lead, so the grid comes out holding a rune two cells wide in a
// column one cell from the edge. Every reader then draws it whole and produces
// a row one cell wider than the screen it came from, which the compositor
// places over the pane next door: the guest's text appears somewhere it was
// never written. Blanking the half left standing is what the insert and delete
// paths already do to a rune they cut, and what ghostty does.
func (s *Screen) blankWideRunesCutByTheEdge() {
	x := s.buf.Width() - 1
	if x < 0 {
		return
	}
	for y := range s.buf.Height() {
		if c := s.buf.CellAt(x, y); c != nil && c.Width > 1 {
			s.buf.SetCell(x, y, nil)
		}
	}
}

// Width returns the width of the screen.
func (s *Screen) Width() int {
	return s.buf.Width()
}

// Clear clears the screen with blank cells.
func (s *Screen) Clear() {
	s.ClearArea(s.Bounds())
}

// ClearArea clears the given area.
func (s *Screen) ClearArea(area uv.Rectangle) {
	s.buf.ClearArea(area)
	s.unwrapArea(area)
}

// Fill fills the screen or part of it.
func (s *Screen) Fill(c *uv.Cell) {
	s.FillArea(c, s.Bounds())
}

// FillArea fills the given area with the given cell.
func (s *Screen) FillArea(c *uv.Cell, area uv.Rectangle) {
	s.buf.FillArea(c, area)
	s.unwrapArea(area)
}

// setHorizontalMargins sets the horizontal margins. They are clamped to the
// screen, because every line and cell operation indexes the buffer by them and
// a margin past the edge would run off the end of a line.
func (s *Screen) setHorizontalMargins(left, right int) {
	w := s.buf.Width()
	s.scroll.Min.X = clamp(left, 0, w)
	s.scroll.Max.X = clamp(right, s.scroll.Min.X, w)
}

// setVerticalMargins sets the vertical margins, clamped to the screen for the
// same reason as setHorizontalMargins.
func (s *Screen) setVerticalMargins(top, bottom int) {
	h := s.buf.Height()
	s.scroll.Min.Y = clamp(top, 0, h)
	s.scroll.Max.Y = clamp(bottom, s.scroll.Min.Y, h)
}

// region returns the scroll region clipped to the screen. Margins are clamped
// when set, so this only differs from scroll if something assigned it
// directly; the line and cell operations use it so that can never index past
// the buffer.
func (s *Screen) region() uv.Rectangle {
	return s.scroll.Intersect(s.buf.Bounds())
}

// setCursorX sets the cursor X position. If margins is true, the cursor is
// only set if it is within the scroll margins.
func (s *Screen) setCursorX(x int, margins bool) {
	s.setCursor(x, s.cur.Y, margins)
}

// setCursor sets the cursor position. If margins is true, the cursor is only
// set if it is within the scroll margins. This follows how [ansi.CUP] works.
func (s *Screen) setCursor(x, y int, margins bool) {
	old := s.cur.Position
	if !margins {
		y = clamp(y, 0, s.buf.Height()-1)
		x = clamp(x, 0, s.buf.Width()-1)
	} else {
		y = clamp(s.scroll.Min.Y+y, s.scroll.Min.Y, s.scroll.Max.Y-1)
		x = clamp(s.scroll.Min.X+x, s.scroll.Min.X, s.scroll.Max.X-1)
	}
	s.cur.X, s.cur.Y = x, y

	if s.cb.CursorPosition != nil && (old.X != x || old.Y != y) {
		s.cb.CursorPosition(old, uv.Pos(x, y))
	}
}

// moveCursor moves the cursor by the given x and y deltas. Each axis is
// bounded on its own, as in xterm: moving up stops at the top margin when the
// cursor starts on or below it and at the top of the screen otherwise, moving
// down stops at the bottom margin when the cursor starts on or above it, and
// left and right do the same with the left and right margins.
// This follows how [ansi.CUU], [ansi.CUD], [ansi.CUF], [ansi.CUB], [ansi.CNL],
// [ansi.CPL].
func (s *Screen) moveCursor(dx, dy int) {
	scroll := s.region()
	old := s.cur.Position
	x := moveWithin(old.X, dx, scroll.Min.X, scroll.Max.X-1, s.buf.Width()-1)
	y := moveWithin(old.Y, dy, scroll.Min.Y, scroll.Max.Y-1, s.buf.Height()-1)

	s.cur.X, s.cur.Y = x, y

	if s.cb.CursorPosition != nil && (old.X != x || old.Y != y) {
		s.cb.CursorPosition(old, uv.Pos(x, y))
	}
}

// moveWithin moves pos by delta along one axis, stopping at the near margin
// (low moving back, high moving forward) when pos starts on the inner side of
// it, and at the screen edge (0 or last) otherwise.
func moveWithin(pos, delta, low, high, last int) int {
	target := pos + delta
	if delta < 0 {
		limit := 0
		if pos >= low {
			limit = low
		}
		return clamp(max(target, limit), 0, last)
	}
	limit := last
	if pos <= high {
		limit = high
	}
	return clamp(min(target, limit), 0, last)
}

// Cursor returns the cursor.
func (s *Screen) Cursor() Cursor {
	return s.cur
}

// CursorPosition returns the cursor position.
func (s *Screen) CursorPosition() (x, y int) {
	return s.cur.X, s.cur.Y
}

// ScrollRegion returns the scroll region.
func (s *Screen) ScrollRegion() uv.Rectangle {
	return s.scroll
}

// SaveCursor saves the cursor.
func (s *Screen) SaveCursor() {
	s.saved = s.cur
}

// RestoreCursor restores the cursor. Visibility (DECTCEM) and shape (DECSCUSR)
// are not part of what DECSC saves, so they are kept as they are, and the
// position is clamped in case the screen shrank since the save.
func (s *Screen) RestoreCursor() {
	old := s.cur.Position
	hidden, style, steady := s.cur.Hidden, s.cur.Style, s.cur.Steady
	s.cur = s.saved
	s.cur.Hidden, s.cur.Style, s.cur.Steady = hidden, style, steady
	// Resize clamps the saved cursor too; this guards any other path that
	// changes the bounds between save and restore.
	s.cur.Position = s.clampPosition(s.cur.Position)

	if s.cb.CursorPosition != nil && (old.X != s.cur.X || old.Y != s.cur.Y) {
		s.cb.CursorPosition(old, s.cur.Position)
	}
}

// setCursorHidden sets the cursor hidden.
func (s *Screen) setCursorHidden(hidden bool) {
	changed := s.cur.Hidden != hidden
	s.cur.Hidden = hidden
	if changed && s.cb.CursorVisibility != nil {
		s.cb.CursorVisibility(!hidden)
	}
}

// setCursorStyle sets the cursor style.
// setCursorStyle sets the cursor style. The callback always runs, even when
// the style is unchanged: DECSCUSR is a guest request, and a blinking block
// (CSI 1 q) matches the emulator default, so a "changed" guard would drop it
// and leave appearance.cursor_blink in charge.
func (s *Screen) setCursorStyle(style CursorStyle, blink bool) {
	s.cur.Style = style
	s.cur.Steady = !blink
	if s.cb.CursorStyle != nil {
		s.cb.CursorStyle(style, !blink)
	}
}

// cursorPen returns the cursor pen.
func (s *Screen) cursorPen() uv.Style {
	return s.cur.Pen
}

// cursorLink returns the cursor link.
func (s *Screen) cursorLink() uv.Link {
	return s.cur.Link
}

// ShowCursor shows the cursor.
func (s *Screen) ShowCursor() {
	s.setCursorHidden(false)
}

// HideCursor hides the cursor.
func (s *Screen) HideCursor() {
	s.setCursorHidden(true)
}

// InsertCell inserts n blank characters at the cursor position pushing out
// cells to the right and out of the screen.
func (s *Screen) InsertCell(n int) {
	if n <= 0 {
		return
	}

	x, y := s.cur.X, s.cur.Y
	line, n, ok := s.shiftBounds(x, y, n)
	if !ok {
		return
	}
	right := s.region().Max.X

	// Copied by assignment rather than through the buffer's Set, which blanks
	// the other half of any wide rune it lands on. That is right for an
	// overwrite but ruinous inside a shift, where the cells being moved are
	// still live: blanking the neighbour of a cell that has just been copied
	// erases the copy, and a single shift over a line of CJK empties the line.
	for i := right - 1; i >= x+n; i-- {
		line[i] = line[i-n]
	}
	blank := s.blankCell()
	for i := x; i < x+n; i++ {
		putBlank(line, i, blank)
	}
	repairWide(line)
	s.buf.TouchLine(x, y, right-x)
}

// DeleteCell deletes n cells at the cursor position moving cells to the left.
// This has no effect if the cursor is outside the scroll region.
func (s *Screen) DeleteCell(n int) {
	if n <= 0 {
		return
	}

	x, y := s.cur.X, s.cur.Y
	line, n, ok := s.shiftBounds(x, y, n)
	if !ok {
		return
	}
	right := s.region().Max.X

	for i := x; i < right-n; i++ {
		line[i] = line[i+n]
	}
	blank := s.blankCell()
	for i := right - n; i < right; i++ {
		putBlank(line, i, blank)
	}
	repairWide(line)
	s.buf.TouchLine(x, y, right-x)
}

// shiftBounds validates a cell shift at (x, y) and returns the row it operates
// on together with the count clamped to the space between x and the right
// margin. It reports false when the position is outside the margins or the
// screen, which is the case every caller treats as a no-op.
func (s *Screen) shiftBounds(x, y, n int) (uv.Line, int, bool) {
	area := s.region()
	if n <= 0 || y < area.Min.Y || y >= area.Max.Y || y >= s.buf.Height() ||
		x < area.Min.X || x >= area.Max.X || x >= s.buf.Width() {
		return nil, 0, false
	}
	if x+n > area.Max.X {
		n = area.Max.X - x
	}
	return s.buf.Lines[y], n, true
}

// putBlank overwrites one column with the erase cell. Like the shift itself it
// assigns rather than calling Set, because the columns it fills have already
// been vacated and any wide rune the fill cuts is dealt with by repairWide.
func putBlank(line uv.Line, x int, blank *uv.Cell) {
	if blank == nil {
		line[x] = uv.EmptyCell
		return
	}
	line[x] = *blank
}

// repairWide blanks every half of a wide rune whose partner a shift left
// behind. A rune that moved by an odd number of columns, or whose second half
// fell off the right margin, leaves a cell claiming a column it no longer
// shares with anything; a terminal has no way to draw that, so both ends of the
// broken pair become spaces.
//
// One pass afterwards is deliberate. Proving which end of which shift can orphan
// which half is fiddly and easy to get subtly wrong, whereas the invariant here
// (a lead of width w is followed by exactly w-1 continuation cells, and no
// continuation stands alone) is stated once and checked over the whole row.
func repairWide(line uv.Line) {
	for i := 0; i < len(line); i++ {
		w := line[i].Width
		switch {
		case w > 1:
			whole := true
			for j := 1; j < w; j++ {
				if i+j >= len(line) || line[i+j].Width != 0 {
					whole = false
					break
				}
			}
			if !whole {
				line[i].Empty()
				continue
			}
			i += w - 1
		case w == 0:
			// Reached without being skipped over above, so there is no lead in
			// front of it.
			line[i].Empty()
		}
	}
}

// ScrollUp scrolls the content up n lines within the given region. Lines
// scrolled past the top margin are saved to the scrollback buffer if the
// scroll region encompasses the full screen width and starts at the top.
// This is equivalent to [ansi.SU] which moves the cursor to the top margin
// and performs a [ansi.DL] operation.
func (s *Screen) ScrollUp(n int) {
	if n <= 0 {
		return
	}

	scroll := s.scroll
	width := s.buf.Width()

	// Only save to scrollback if we're scrolling the main screen area
	// (not a limited scroll region) and the scroll region starts at Y=0
	save := s.scrollback != nil && scroll.Min.Y == 0 && scroll.Min.X == 0 && scroll.Dx() == width

	x, y := s.CursorPosition()
	s.setCursor(s.cur.X, 0, true)
	if !s.rotateWholeScreenUp(n, save) {
		// The rotation did not apply, so the departing rows stay where they are
		// and have to be copied out before DeleteLine overwrites them.
		if save {
			for i := 0; i < n && i < scroll.Dy(); i++ {
				line := extractLine(s.buf.Buffer, scroll.Min.Y+i, width)
				s.scrollback.pushOwned(line, s.lineWrap(scroll.Min.Y+i))
			}
		}
		s.DeleteLine(n)
	}
	s.setCursor(x, y, false)
}

// rotateWholeScreenUp scrolls the whole buffer up n lines by moving line
// headers instead of cells, and reports whether it applied.
//
// It only applies when the scroll region is the entire buffer, which is what a
// shell printing output uses and so is the overwhelming majority of scrolls. A
// limited region (DECSTBM) still goes through Buffer.DeleteLineArea.
//
// The cost being avoided is real: DeleteLineArea copies every cell of the
// region up one row in a nested loop, and a uv.Cell is 112 bytes carrying three
// colour interfaces and three strings, so one newline at 207x55 is 11,178
// struct copies each with a pointer write barrier, and the garbage collector
// then scans all of it. Rotating touches the rows themselves and reuses the
// slices that fall off the top as the new blank rows at the bottom, so nothing
// is allocated and no cell moves.
//
// When save is set, the rows leaving the top go straight into the scrollback
// instead of being copied there first, and the storage the ring evicts in
// exchange becomes the new blank rows at the bottom. In the steady state of a
// pane printing output that is a pure swap: no line is allocated and no cell is
// copied for a scroll that also has to be retained. The ownership rule is the
// one extractLine already established, that the ring holds a line nothing else
// writes; the only new part is that the screen takes back storage the ring has
// finished with.
//
// Every row is marked touched, because every row's index changed and the
// renderer diffs by index.
func (s *Screen) rotateWholeScreenUp(n int, save bool) bool {
	lines := s.buf.Lines
	height := len(lines)
	area := s.scroll
	if height == 0 || area.Min.X != 0 || area.Min.Y != 0 ||
		area.Max.Y != height || area.Dx() != s.buf.Width() {
		return false
	}

	// A scroll of the whole screen or more clears it; there is nothing to move.
	if n >= height {
		n = height
	}

	// Lift the rows leaving the top, slide the rest up, and put the lifted
	// slices back at the bottom to be blanked. The scratch array keeps the
	// common case (one line, printing output) free of allocation; only a large
	// CSI S needs the heap, and then for one slice of line headers rather than
	// for any cells.
	var scratch [16]uv.Line
	var recycled []uv.Line
	if n <= len(scratch) {
		recycled = scratch[:n]
	} else {
		recycled = make([]uv.Line, n)
	}
	copy(recycled, lines[:n])
	copy(lines, lines[n:])
	if save {
		for i, row := range recycled {
			reuse := s.scrollback.pushOwned(row, s.lineWrap(i))
			if len(reuse) == len(row) {
				recycled[i] = reuse
			} else {
				// Nothing evicted yet, or the ring is still holding lines from
				// before a resize, so the row it gave back is the wrong width.
				recycled[i] = make(uv.Line, len(row))
			}
		}
	}
	copy(lines[height-n:], recycled)
	if len(s.wraps) == height {
		copy(s.wraps, s.wraps[n:])
		clear(s.wraps[height-n:])
	}

	blank := uv.EmptyCell
	if c := s.blankCell(); c != nil {
		blank = *c
	}
	for y := height - n; y < height; y++ {
		row := lines[y]
		for x := range row {
			row[x] = blank
		}
	}

	for y := range height {
		s.buf.TouchLine(0, y, area.Max.X)
	}
	return true
}

// ScrollDown scrolls the content down n lines within the given region. Lines
// scrolled past the bottom margin are lost. This is equivalent to [ansi.SD]
// which moves the cursor to top margin and performs a [ansi.IL] operation.
func (s *Screen) ScrollDown(n int) {
	x, y := s.CursorPosition()
	s.setCursor(s.cur.X, 0, true)
	s.InsertLine(n)
	s.setCursor(x, y, false)
}

// InsertLine inserts n blank lines at the cursor position Y coordinate.
// Only operates if cursor is within scroll region. Lines below cursor Y
// are moved down, with those past bottom margin being discarded.
// It returns true if the operation was successful.
func (s *Screen) InsertLine(n int) bool {
	if n <= 0 {
		return false
	}

	scroll := s.region()
	x, y := s.cur.X, s.cur.Y

	// Only operate if cursor Y is within scroll region
	if y < scroll.Min.Y || y >= scroll.Max.Y ||
		x < scroll.Min.X || x >= scroll.Max.X {
		return false
	}

	s.buf.InsertLineArea(y, n, s.blankCell(), scroll)
	s.shiftWraps(y, n, scroll, true)

	return true
}

// DeleteLine deletes n lines at the cursor position Y coordinate.
// Only operates if cursor is within scroll region. Lines below cursor Y
// are moved up, with blank lines inserted at the bottom of scroll region.
// It returns true if the operation was successful.
func (s *Screen) DeleteLine(n int) bool {
	if n <= 0 {
		return false
	}

	scroll := s.region()
	x, y := s.cur.X, s.cur.Y

	// Only operate if cursor Y is within scroll region
	if y < scroll.Min.Y || y >= scroll.Max.Y ||
		x < scroll.Min.X || x >= scroll.Max.X {
		return false
	}

	s.buf.DeleteLineArea(y, n, s.blankCell(), scroll)
	s.shiftWraps(y, n, scroll, false)

	return true
}

// blankCell returns the cursor blank cell with the background color set to the
// current pen background color. If the pen background color is nil, the return
// value is nil.
func (s *Screen) blankCell() *uv.Cell {
	if s.cur.Pen.Bg == nil {
		return nil
	}

	c := uv.EmptyCell
	c.Style.Bg = s.cur.Pen.Bg
	return &c
}

// Scrollback returns the scrollback buffer for this screen.
func (s *Screen) Scrollback() *Scrollback {
	return s.scrollback
}

// DisableScrollback drops this screen's scrollback ring, so lines scrolled off
// the top are discarded instead of retained. Every other scrollback method here
// already tolerates the nil, and ScrollUp checks it before pushing.
func (s *Screen) DisableScrollback() {
	s.scrollback = nil
}

// ClearScrollback clears all lines from the scrollback buffer.
func (s *Screen) ClearScrollback() {
	if s.scrollback != nil {
		s.scrollback.Clear()
	}
}

// ScrollbackLen returns the number of lines currently in the scrollback buffer.
func (s *Screen) ScrollbackLen() int {
	if s.scrollback == nil {
		return 0
	}
	return s.scrollback.Len()
}

// ScrollbackLine returns the line at the specified index in the scrollback buffer.
// Index 0 is the oldest line. Returns nil if the index is out of bounds.
//
// The line is the ring's own storage, not a copy. Once the ring is full, the
// next scroll evicts the oldest line and rotateWholeScreenUp reuses that
// storage as a live screen row, so a line obtained here must not be kept past
// the lock (the window's IO lock) under which it was read; copy it if it has
// to outlive that.
func (s *Screen) ScrollbackLine(index int) uv.Line {
	if s.scrollback == nil {
		return nil
	}
	return s.scrollback.Line(index)
}

// SetScrollbackMaxLines sets the maximum number of lines for the scrollback buffer.
func (s *Screen) SetScrollbackMaxLines(maxLines int) {
	if s.scrollback != nil {
		s.scrollback.SetMaxLines(maxLines)
	}
}
