// Package input implements vim-style copy mode for TUIOS.
package input

import (
	"unicode/utf8"

	"github.com/tonk/tuios/internal/terminal"
	vt "github.com/tonk/tuios/internal/vt"
	uv "github.com/charmbracelet/ultraviolet"
)

// getAbsoluteY calculates the absolute Y position in the entire scrollback+screen
func getAbsoluteY(cm *terminal.CopyMode, window *terminal.Window) int {
	scrollbackLen := window.ScrollbackLen()
	if cm.ScrollOffset > 0 {
		return scrollbackLen - cm.ScrollOffset + cm.CursorY
	}
	return scrollbackLen + cm.CursorY
}

// isVimWordChar returns true if the rune is part of a vim "word" (alphanumeric or underscore)
func isVimWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_'
}

// getCharType returns the type of character: 0=whitespace, 1=word char, 2=punctuation
func getCharType(content string) int {
	if content == "" || content == " " || content == "\t" {
		return 0 // whitespace
	}
	r := []rune(content)[0]
	if isVimWordChar(r) {
		return 1 // word character
	}
	return 2 // punctuation/special
}

// getCellAtCursor returns the cell at the current cursor position
func getCellAtCursor(cm *terminal.CopyMode, window *terminal.Window) *uv.Cell {
	absY := getAbsoluteY(cm, window)
	scrollbackLen := window.ScrollbackLen()

	if absY < scrollbackLen {
		line := window.ScrollbackLine(absY)
		if line != nil && cm.CursorX < len(line) {
			return &line[cm.CursorX]
		}
		return nil
	}

	screenY := absY - scrollbackLen
	return window.Terminal.CellAt(cm.CursorX, screenY)
}

// byteIndexToCharIndex converts a byte index in a UTF-8 string to a character (rune) index
// This is needed because strings.Index returns byte positions, not character positions
func byteIndexToCharIndex(s string, byteIdx int) int {
	if byteIdx <= 0 {
		return 0
	}
	if byteIdx >= len(s) {
		return len([]rune(s))
	}

	// Count runes up to the byte index
	charIdx := 0
	byteCount := 0
	for _, r := range s {
		if byteCount >= byteIdx {
			break
		}
		byteCount += utf8.RuneLen(r)
		charIdx++
	}
	return charIdx
}

// extractLineTextFromCells builds text string from cell array
func extractLineTextFromCells(cells []uv.Cell) string {
	var result []rune
	for _, cell := range cells {
		// Skip continuation cells (Width=0) of wide characters
		// These are placeholder cells for emoji, CJK, nerd fonts, etc.
		if cell.Width == 0 {
			continue
		}
		if cell.Content != "" {
			for _, r := range cell.Content {
				result = append(result, r)
			}
		} else {
			result = append(result, ' ')
		}
	}
	return string(result)
}

// extractScreenLineText builds text string from terminal screen line
func extractScreenLineText(term *vt.Emulator, y int) string {
	var result []rune
	width := term.Width()
	for x := range width {
		cell := term.CellAt(x, y)
		// Skip continuation cells (Width=0) of wide characters
		// These are placeholder cells for emoji, CJK, nerd fonts, etc.
		if cell != nil && cell.Width == 0 {
			continue
		}
		if cell != nil && cell.Content != "" {
			for _, r := range cell.Content {
				result = append(result, r)
			}
		} else {
			result = append(result, ' ')
		}
	}
	return string(result)
}

// getScreenLineCells returns all cells for a screen line
func getScreenLineCells(term *vt.Emulator, y int) []uv.Cell {
	width := term.Width()
	cells := make([]uv.Cell, width)

	for x := range width {
		cell := term.CellAt(x, y)
		if cell != nil {
			cells[x] = *cell
		} else {
			// Empty cell
			cells[x] = uv.Cell{Content: " ", Width: 1}
		}
	}

	return cells
}

// charIndexToColumn converts a character (rune) index in the text built by
// extractLineTextFromCells or extractScreenLineText to a column position.
//
// The cells array is structured so that each cell index IS the column position.
// For wide characters (Width=2), the next cell is a continuation (Width=0).
// A cell holds a whole grapheme cluster, which can be several runes: ❤️ is a
// heart and a variation selector, é may be e and a combining accent. The text
// carries every one of those runes, so the mapping has to advance by the
// cell's rune count, not by one per cell, or every match after such a cell
// lands a column too far right for each extra rune.
//
// Example:
//
//	Columns:  0   1  2  3  4
//	Cells:   [❤️][] [ ][a][b]
//	Width:    2   0  1  1  1
//	Text:    "❤\uFE0F ab" (runes: ❤, VS16, space, a, b)
//	Character index 3 ('a') → Column 3
//
// An index that falls inside a multi-rune cluster maps to the column after
// that cell, so a match end is never short of the cell it covers.
func charIndexToColumn(cells []uv.Cell, charIndex int) int {
	if charIndex <= 0 {
		return 0
	}

	if len(cells) == 0 {
		return 0
	}

	charsProcessed := 0

	for col, cell := range cells {
		// Skip continuation cells (Width=0) when counting characters
		if cell.Width == 0 {
			continue
		}

		// If we've reached the target character index, return the column
		// (which is the cell index)
		if charsProcessed >= charIndex {
			return col
		}

		// Empty content is extracted as a single space.
		if cell.Content == "" {
			charsProcessed++
		} else {
			charsProcessed += utf8.RuneCountInString(cell.Content)
		}
	}

	// Past the end - return the last column
	return len(cells)
}

// isBlankLine returns true if a line contains only whitespace
func isBlankLine(lineText string) bool {
	for _, r := range lineText {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}
