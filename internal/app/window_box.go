package app

import (
	"image/color"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// lipglossWindowBox is how a bordered window's box is built in general:
// content fitted to the content area, then rendered by lipgloss in a box of
// width x height with every border side but the top. boxSides is the fast path
// for the common case; this is the fallback and the reference it must match.
func lipglossWindowBox(content string, contentW, contentH, width, height int, border lipgloss.Border, fg color.Color) string {
	return lipgloss.NewStyle().
		Align(lipgloss.Left).
		AlignVertical(lipgloss.Top).
		Border(border).
		BorderTop(false).
		Width(width).
		Height(height).
		BorderForeground(fg).
		Render(fitToContentBox(content, contentW, contentH))
}

// boxSides returns, for the common case, what addToBorder takes from
// lipglossWindowBox: the box's lines without the bottom edge, which the chrome
// replaces, and the box width less two. ok is false when the case is not the
// common one, and the caller must use lipglossWindowBox.
//
// The common case is content that already fits: no line wider than the box's
// inside or the content area, no more lines than the content area, no tabs or
// carriage returns, and border runes one cell wide. lipgloss then wraps
// nothing and trims nothing, so its Render reduces to closing open styles at
// each line end (its WrapWriter), padding every line with spaces to the
// inside width, adding blank lines to the inside height and putting a styled
// border rune on either side. lipgloss measures every line about four times
// on the way, through its word wrap, its alignment and its border; here each
// line is measured once.
func boxSides(content string, contentW, contentH, width, height int, border lipgloss.Border, fg color.Color) (sides []string, chromeWidth int, ok bool) {
	innerW, innerH := width-2, height-1
	if innerW < 1 || fg == nil || strings.ContainsAny(content, "\t\r") {
		return nil, 0, false
	}
	if _, none := fg.(lipgloss.NoColor); none {
		return nil, 0, false
	}
	for _, edge := range []string{border.Left, border.Right, border.Bottom, border.BottomLeft, border.BottomRight} {
		if len([]rune(edge)) != 1 || ansi.StringWidth(edge) != 1 {
			return nil, 0, false
		}
	}

	lines := strings.Split(content, "\n")
	if len(lines) > contentH {
		return nil, 0, false
	}
	widths := make([]int, len(lines))
	for i, l := range lines {
		w := ansi.StringWidth(l)
		if w > innerW || w > contentW {
			return nil, 0, false
		}
		widths[i] = w
	}

	// lipgloss's WrapWriter: a style or hyperlink still open at a line end is
	// closed before the newline and reopened after it. It inserts no newlines,
	// so the lines and their widths stay as measured. lipgloss.Wrap takes the
	// string before its deferred Close runs, so whatever Close would write to
	// end the last line never makes it into the box; it is left out here too.
	if strings.Contains(content, "\x1b") {
		var buf strings.Builder
		buf.Grow(len(content))
		ww := lipgloss.NewWrapWriter(&buf)
		_, _ = io.WriteString(ww, content)
		wrapped := buf.String()
		_ = ww.Close()
		lines = strings.Split(wrapped, "\n")
	}

	style := ansi.Style{}.ForegroundColor(fg)
	left, right := style.Styled(border.Left), style.Styled(border.Right)
	spaces := strings.Repeat(" ", innerW)

	sides = make([]string, max(len(lines), innerH))
	for i := range sides {
		var line string
		w := 0
		if i < len(lines) {
			line, w = lines[i], widths[i]
		}
		sides[i] = left + line + spaces[:innerW-w] + right
	}
	return sides, innerW, true
}
