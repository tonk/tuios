// Package input implements vim-style copy mode for TUIOS.
package input

import (
	"strings"

	"github.com/tonk/tuios/internal/terminal"
	uv "github.com/charmbracelet/ultraviolet"
)

// Visual selection-related functions for copy mode (v/V/y and text extraction)

// enterVisualChar enters visual character selection mode
func enterVisualChar(cm *terminal.CopyMode, window *terminal.Window) {
	cm.State = terminal.CopyModeVisualChar
	absY := getAbsoluteY(cm, window)
	cm.VisualStart = terminal.Position{X: cm.CursorX, Y: absY}
	cm.VisualEnd = cm.VisualStart
}

// enterVisualLine enters visual line selection mode
func enterVisualLine(cm *terminal.CopyMode, window *terminal.Window) {
	cm.State = terminal.CopyModeVisualLine
	absY := getAbsoluteY(cm, window)

	// A whole-line selection starts at column 0 - including leading
	// indentation - through the line's last non-empty character.
	_, endX := getLineContentBounds(cm, window, absY)

	cm.VisualStart = terminal.Position{X: 0, Y: absY}
	cm.VisualEnd = terminal.Position{X: endX, Y: absY}
}

// UpdateVisualEnd updates the visual selection end position (exported for auto-scroll).
func UpdateVisualEnd(cm *terminal.CopyMode, window *terminal.Window) {
	updateVisualEnd(cm, window)
}

// updateVisualEnd updates the visual selection end position
func updateVisualEnd(cm *terminal.CopyMode, window *terminal.Window) {
	absY := getAbsoluteY(cm, window)

	switch cm.State {
	case terminal.CopyModeVisualChar:
		cm.VisualEnd = terminal.Position{X: cm.CursorX, Y: absY}
	case terminal.CopyModeVisualLine:
		// For visual line mode, we need to select entire lines
		// Start Y stays fixed, we only update end Y
		cm.VisualEnd.Y = absY

		// Determine which line is earlier and which is later
		startY := cm.VisualStart.Y
		endY := cm.VisualEnd.Y

		// Normalize: make sure startY <= endY for bounds calculation
		if startY > endY {
			startY, endY = endY, startY
		}

		// A whole-line selection always starts at column 0 (including leading
		// indentation); only the lower line's end needs clamping to its content.
		_, endLineEndX := getLineContentBounds(cm, window, endY)

		// If moving upwards (current Y < original start Y), we want:
		// - Start to be at beginning of the upper line (current position)
		// - End to be at end of the lower line (original start)
		if absY < cm.VisualStart.Y {
			// Moving upwards
			cm.VisualEnd.X = 0
			cm.VisualStart.X = endLineEndX
		} else {
			// Moving downwards or same line
			cm.VisualStart.X = 0
			cm.VisualEnd.X = endLineEndX
		}
	}
}

// extractVisualText extracts the text from the current visual selection
func extractVisualText(cm *terminal.CopyMode, window *terminal.Window) string {
	start, end := cm.VisualStart, cm.VisualEnd

	// Normalize selection
	if start.Y > end.Y || (start.Y == end.Y && start.X > end.X) {
		start, end = end, start
	}

	var text strings.Builder
	scrollbackLen := window.ScrollbackLen()

	// Single line
	if start.Y == end.Y {
		// Clamp selection to line content bounds to avoid copying empty cells
		_, lineEndX := getLineContentBounds(cm, window, start.Y)
		clampedEndX := min(end.X, lineEndX)

		if start.Y < scrollbackLen {
			line := window.ScrollbackLine(start.Y)
			for x := start.X; x <= clampedEndX && line != nil && x < len(line); x++ {
				if line[x].Content != "" && line[x].Content != " " {
					text.WriteString(line[x].Content)
				} else if line[x].Content == " " {
					// Preserve internal spaces but not empty cells
					text.WriteRune(' ')
				}
			}
		} else {
			screenY := start.Y - scrollbackLen
			for x := start.X; x <= clampedEndX && x < window.Width; x++ {
				cell := window.Terminal.CellAt(x, screenY)
				if cell != nil && cell.Content != "" && cell.Content != " " {
					text.WriteString(cell.Content)
				} else if cell != nil && cell.Content == " " {
					// Preserve internal spaces but not empty cells
					text.WriteRune(' ')
				}
			}
		}
		// Trim trailing space only: the selection's own leading whitespace
		// (indentation) is content the user dragged over, not padding.
		return strings.TrimRight(text.String(), " ")
	}

	// Multi-line
	for y := start.Y; y <= end.Y; y++ {
		startX, endX := 0, window.Width-1

		if y == start.Y {
			startX = start.X
		}
		if y == end.Y {
			endX = end.X
		}

		// Extract line content
		var lineCells []uv.Cell
		if y < scrollbackLen {
			lineCells = window.ScrollbackLine(y)
		} else {
			screenY := y - scrollbackLen
			// Build cells array from screen
			for x := range window.Width {
				cell := window.Terminal.CellAt(x, screenY)
				if cell != nil {
					lineCells = append(lineCells, *cell)
				} else {
					lineCells = append(lineCells, uv.Cell{})
				}
			}
		}

		// A row the guest's output soft-wrapped runs on into the next one:
		// the two are one line, joined without a newline. Its content is the
		// whole row up to any spacer the print path left for a wide rune it
		// moved down, spaces at the edge included, since those are part of the
		// line. The emulator records the wrap as it happens, so this is not
		// guessed from how far across the row the text reaches, which joined
		// any line that merely came close to the edge.
		wrap := window.LineWrap(y)
		softWrapped := y < end.Y && wrap.Wrapped()

		// Clamp to line content bounds to avoid copying empty cells at end
		_, lineEndX := getLineContentBounds(cm, window, y)
		if softWrapped {
			lineEndX = len(lineCells) - 1 - wrap.Spacer()
		}
		switch y {
		case start.Y:
			// First line: keep user's start but clamp end to content
			endX = min(endX, lineEndX)
		case end.Y:
			// Last line: keep user's end but clamp to content
			endX = min(endX, lineEndX)
		default:
			// Middle lines: keep leading indentation, but clamp the end to
			// content so trailing padding isn't copied.
			endX = lineEndX
		}

		// Append line content
		if lineCells != nil {
			for x := startX; x <= endX && x < len(lineCells); x++ {
				if lineCells[x].Content != "" && lineCells[x].Content != " " {
					text.WriteString(lineCells[x].Content)
				} else if lineCells[x].Content == " " {
					// Preserve internal spaces but not empty cells
					text.WriteRune(' ')
				}
			}
		}

		if y < end.Y && !softWrapped {
			text.WriteRune('\n')
		}
	}

	// Trim trailing space only: leading whitespace on any line (including
	// the first) is indentation the user selected, not padding.
	return strings.TrimRight(text.String(), " ")
}

// getLineContentBounds returns the X positions of the first and last non-empty characters on a line
func getLineContentBounds(_ *terminal.CopyMode, window *terminal.Window, absY int) (int, int) {
	scrollbackLen := window.ScrollbackLen()

	// Get cells for this line
	var cells []uv.Cell
	if absY < scrollbackLen {
		cells = window.ScrollbackLine(absY)
	} else {
		screenY := absY - scrollbackLen
		cells = getScreenLineCells(window.Terminal, screenY)
	}

	if len(cells) == 0 {
		return 0, 0
	}

	// Find first non-empty, non-continuation cell
	startX := 0
	for i, cell := range cells {
		if cell.Width > 0 && cell.Content != "" && cell.Content != " " {
			startX = i
			break
		}
	}

	// Find last non-empty, non-continuation cell
	endX := len(cells) - 1
	for i := len(cells) - 1; i >= 0; i-- {
		if cells[i].Width > 0 && cells[i].Content != "" && cells[i].Content != " " {
			endX = i
			break
		}
	}

	// If entire line is empty, just return 0, 0
	if endX < startX {
		return 0, 0
	}

	return startX, endX
}

// getLineText retrieves the text content of a line
func getLineText(_ *terminal.CopyMode, window *terminal.Window, absY int) string {
	scrollbackLen := window.ScrollbackLen()

	if absY < scrollbackLen {
		line := window.ScrollbackLine(absY)
		if line != nil {
			return extractLineTextFromCells(line)
		}
	} else {
		screenY := absY - scrollbackLen
		return extractScreenLineText(window.Terminal, screenY)
	}

	return ""
}
