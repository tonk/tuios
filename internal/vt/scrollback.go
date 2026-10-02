package vt

import (
	uv "github.com/charmbracelet/ultraviolet"
)

// DefaultScrollbackSize is the default number of lines to keep in the
// scrollback buffer.
const DefaultScrollbackSize = 10000

// Scrollback represents a scrollback buffer that stores lines that have
// scrolled off the top of the visible screen.
// Uses a ring buffer for O(1) insertions instead of O(n) slice reallocations.
// Every line carries how it ended (see LineWrap), which is what lets a resize
// lay the history out again at the new width.
type Scrollback struct {
	// lines stores the scrollback lines in a ring buffer
	lines []uv.Line
	// maxLines is the maximum number of lines to keep in scrollback
	maxLines int
	// head is the index of the oldest line in the ring buffer
	head int
	// tail is the index where the next line will be inserted
	tail int
	// full indicates whether the ring buffer is at capacity
	full bool
	// lastWidthCaptured tracks the terminal width when lines were last added
	// Used for detecting when reflow is needed on resize
	lastWidthCaptured int
	// wraps records, per slot, how the line in it ended: a hard break, or a
	// soft wrap into the next line, which a reflow joins it with.
	wraps []LineWrap
	// onTrim is called when oldest lines are overwritten by the ring buffer.
	// The argument is the number of lines trimmed (always 1 per overwrite).
	onTrim func(int)
}

// NewScrollback creates a new scrollback buffer with the specified maximum
// number of lines. If maxLines is 0, DefaultScrollbackSize is used.
func NewScrollback(maxLines int) *Scrollback {
	if maxLines <= 0 {
		maxLines = DefaultScrollbackSize
	}
	return &Scrollback{
		lines:             make([]uv.Line, maxLines), // Pre-allocate full ring buffer
		maxLines:          maxLines,
		head:              0,
		tail:              0,
		full:              false,
		lastWidthCaptured: 0,
		wraps:             make([]LineWrap, maxLines),
	}
}

// PushLine adds a line that ends in a hard break to the scrollback buffer. If
// the buffer is full, the oldest line is removed (by overwriting it in the ring
// buffer), an O(1) operation.
//
// It used to mark every line soft-wrapped, which meant nothing while no reader
// looked at the flag; now that a reflow joins soft-wrapped lines, a line whose
// ending is not known is safer kept on its own.
func (sb *Scrollback) PushLine(line uv.Line) {
	sb.PushLineWrap(line, HardBreak)
}

// SetOnTrim sets a callback that fires when the ring buffer overwrites oldest lines.
func (sb *Scrollback) SetOnTrim(fn func(int)) {
	sb.onTrim = fn
}

// PushLineWithWrap adds a line, soft-wrapped into the next one or not. A
// soft-wrapped line pushed this way has no spacer columns; PushLineWrap takes
// the full LineWrap.
func (sb *Scrollback) PushLineWithWrap(line uv.Line, isSoftWrapped bool) {
	sb.PushLineWrap(line, boolWrap(isSoftWrapped))
}

// PushLineWrap adds a copy of line, which ended as wrap says.
func (sb *Scrollback) PushLineWrap(line uv.Line, wrap LineWrap) {
	if len(line) == 0 {
		return
	}

	// Make a copy of the line to avoid aliasing issues
	lineCopy := make(uv.Line, len(line))
	copy(lineCopy, line)

	sb.pushOwned(lineCopy, wrap)
}

// boolWrap is the LineWrap of a bare soft-wrapped flag.
func boolWrap(softWrapped bool) LineWrap {
	if softWrapped {
		return SoftWrap(0)
	}
	return HardBreak
}

// PushLineOwned is PushLineWithWrap for a line the caller has just allocated
// and will not touch again, so the ring takes it as is.
//
// The defensive copy in PushLineWithWrap exists because most callers hand over
// a row of the live screen buffer, which keeps being written. The scroll path
// does not: extractLine allocates a fresh line per scrolled row and drops its
// only reference here, so copying it again doubled the cost of retaining a
// line, and at 112 bytes per cell and terminal width per line that was the bulk
// of everything the write path allocated.
func (sb *Scrollback) PushLineOwned(line uv.Line, isSoftWrapped bool) {
	sb.pushOwned(line, boolWrap(isSoftWrapped))
}

// PushLineOwnedRecycle is PushLineOwned that also hands back the line the ring
// just evicted, so the caller can reuse its storage instead of allocating.
//
// It returns nil while the ring still has room, because nothing has been
// evicted yet. The returned slice is unreachable through the scrollback once
// this call returns: head has already moved past it. Callers must treat it as
// uninitialised storage, since it still holds the evicted line's cells.
func (sb *Scrollback) PushLineOwnedRecycle(line uv.Line, isSoftWrapped bool) uv.Line {
	return sb.pushOwned(line, boolWrap(isSoftWrapped))
}

// pushOwned is the push all the others come down to: it takes line as it is,
// records how it ended, and returns the evicted line (see
// PushLineOwnedRecycle).
func (sb *Scrollback) pushOwned(line uv.Line, wrap LineWrap) uv.Line {
	if len(line) == 0 {
		return nil
	}

	lineCopy := line

	// The slot about to be written holds the oldest line once the ring is full,
	// and nothing can reach it after head advances below.
	var evicted uv.Line
	if sb.full {
		evicted = sb.lines[sb.tail]
	}

	// Insert at tail position
	sb.lines[sb.tail] = lineCopy
	sb.wraps[sb.tail] = wrap

	// Advance tail (wraps around at maxLines)
	sb.tail = (sb.tail + 1) % sb.maxLines

	// If buffer is full, advance head (oldest line pointer) as well
	if sb.full {
		sb.head = (sb.head + 1) % sb.maxLines
		if sb.onTrim != nil {
			sb.onTrim(1)
		}
	}

	// Mark as full when tail catches up to head
	if sb.tail == sb.head && len(lineCopy) > 0 {
		sb.full = true
	}

	return evicted
}

// Len returns the number of lines currently in the scrollback buffer.
func (sb *Scrollback) Len() int {
	if sb.full {
		return sb.maxLines
	}
	if sb.tail >= sb.head {
		return sb.tail - sb.head
	}
	return sb.maxLines - sb.head + sb.tail
}

// Line returns the line at the specified index in the scrollback buffer.
// Index 0 is the oldest line, and Len()-1 is the newest (most recently scrolled).
// Returns nil if the index is out of bounds.
func (sb *Scrollback) Line(index int) uv.Line {
	length := sb.Len()
	if index < 0 || index >= length {
		return nil
	}
	if sb.maxLines <= 0 {
		return nil
	}
	// Map logical index to physical ring buffer index
	physicalIndex := (sb.head + index) % sb.maxLines
	if physicalIndex < 0 || physicalIndex >= len(sb.lines) {
		return nil
	}
	return sb.lines[physicalIndex]
}

// Lines returns a slice of all lines in the scrollback buffer, from oldest
// to newest. The returned slice should not be modified.
func (sb *Scrollback) Lines() []uv.Line {
	length := sb.Len()
	if length == 0 {
		return nil
	}

	// Build a slice in correct order from the ring buffer
	result := make([]uv.Line, length)
	for i := range length {
		physicalIndex := (sb.head + i) % sb.maxLines
		result[i] = sb.lines[physicalIndex]
	}
	return result
}

// Clear removes all lines from the scrollback buffer.
func (sb *Scrollback) Clear() {
	count := sb.Len()
	sb.head = 0
	sb.tail = 0
	sb.full = false
	// Nil out the lines to help GC, but keep the slice
	for i := range sb.lines {
		sb.lines[i] = nil
		sb.wraps[i] = HardBreak
	}
	// Notify marker list so stale markers are removed
	if sb.onTrim != nil && count > 0 {
		sb.onTrim(count)
	}
}

// LineWrap returns how the line at index (0 is the oldest) ended. It is
// HardBreak for an index out of range.
func (sb *Scrollback) LineWrap(index int) LineWrap {
	if index < 0 || index >= sb.Len() {
		return HardBreak
	}
	return sb.wraps[(sb.head+index)%sb.maxLines]
}

// IsSoftWrapped reports whether the line at index (0 is the oldest) continues
// on the line after it.
func (sb *Scrollback) IsSoftWrapped(index int) bool {
	return sb.LineWrap(index).Wrapped()
}

// Reflow lays the scrollback out again at newWidth: soft-wrapped runs are
// joined into logical lines and split again at the new width, see reflower.
// The newest line keeps its soft wrap if it has one, since what it continues
// into is not here.
//
// The emulator does not call this. A resize reflows the scrollback and the
// normal screen together (Screen.reflowResize), because a logical line can
// start in one and end in the other; this is the scrollback on its own, for a
// buffer with no screen under it. It does not call onTrim: line numbers change
// all through the buffer, not only at the top, so whatever hangs off them
// cannot be fixed up by a count.
func (sb *Scrollback) Reflow(newWidth int) {
	if newWidth <= 0 {
		return
	}
	sb.lastWidthCaptured = newWidth
	rows, wraps := sb.appendTo(nil, nil)
	if len(rows) == 0 {
		return
	}
	r := newReflower(rows, wraps, newWidth, -1, 0)
	from := max(0, r.total-sb.maxLines)
	lines, lineWraps := r.build(from, r.total, r.total)
	sb.replace(lines, lineWraps)
}

// appendTo appends the retained lines and how each ended, oldest first.
func (sb *Scrollback) appendTo(lines []uv.Line, wraps []LineWrap) ([]uv.Line, []LineWrap) {
	for i, n := 0, sb.Len(); i < n; i++ {
		p := (sb.head + i) % sb.maxLines
		lines = append(lines, sb.lines[p])
		wraps = append(wraps, sb.wraps[p])
	}
	return lines, wraps
}

// replace makes lines (oldest first, at most maxLines of them) the whole
// content of the ring, taking them as they are. It does not call onTrim: a
// reflow moves the markers itself.
func (sb *Scrollback) replace(lines []uv.Line, wraps []LineWrap) {
	if len(lines) > sb.maxLines {
		drop := len(lines) - sb.maxLines
		lines, wraps = lines[drop:], wraps[drop:]
	}
	clear(sb.lines)
	clear(sb.wraps)
	n := copy(sb.lines, lines)
	copy(sb.wraps, wraps)
	sb.head = 0
	sb.tail = n % sb.maxLines
	sb.full = n == sb.maxLines
}

// SetCaptureWidth sets the terminal width at which scrollback lines are being captured.
// Should be called from the emulator when processing output.
func (sb *Scrollback) SetCaptureWidth(width int) {
	if width > 0 && width != sb.lastWidthCaptured {
		// Width changed - could trigger reflow if implemented
		sb.lastWidthCaptured = width
	}
}

// CaptureWidth returns the terminal width at which scrollback was captured.
func (sb *Scrollback) CaptureWidth() int {
	return sb.lastWidthCaptured
}

// MaxLines returns the maximum number of lines this scrollback can hold.
func (sb *Scrollback) MaxLines() int {
	return sb.maxLines
}

// SetMaxLines sets the maximum number of lines for the scrollback buffer.
// If the new limit is smaller than the current number of lines, older lines
// are discarded to fit the new limit.
func (sb *Scrollback) SetMaxLines(maxLines int) {
	if maxLines <= 0 {
		maxLines = DefaultScrollbackSize
	}

	if maxLines == sb.maxLines {
		return // No change needed
	}

	oldLen := sb.Len()
	if oldLen == 0 {
		// Empty buffer, just resize
		sb.lines = make([]uv.Line, maxLines)
		sb.wraps = make([]LineWrap, maxLines)
		sb.maxLines = maxLines
		sb.head = 0
		sb.tail = 0
		sb.full = false
		return
	}

	// Create new ring buffer and copy existing lines
	newLines := make([]uv.Line, maxLines)
	newWraps := make([]LineWrap, maxLines)
	newLen := min(oldLen, maxLines)

	// Copy the most recent newLen lines
	startIndex := oldLen - newLen // Skip oldest lines if downsizing
	for i := range newLen {
		physicalIndex := (sb.head + startIndex + i) % sb.maxLines
		newLines[i] = sb.lines[physicalIndex]
		newWraps[i] = sb.wraps[physicalIndex]
	}

	sb.lines = newLines
	sb.wraps = newWraps
	sb.maxLines = maxLines
	sb.head = 0
	sb.tail = newLen % maxLines
	sb.full = (newLen == maxLines)

	// Downsizing dropped the oldest oldLen-newLen lines; re-base semantic
	// markers so their AbsLine stays anchored to the oldest scrollback line,
	// matching PushLineWithWrap and Clear.
	if oldLen > newLen && sb.onTrim != nil {
		sb.onTrim(oldLen - newLen)
	}
}

// extractLine extracts a complete line from the buffer at the given Y coordinate.
// This is a helper function to copy cells from a buffer line.
func extractLine(buf *uv.Buffer, y, width int) uv.Line {
	line := make(uv.Line, width)
	for x := range width {
		if cell := buf.CellAt(x, y); cell != nil {
			line[x] = *cell
		} else {
			line[x] = uv.EmptyCell
		}
	}
	return line
}
