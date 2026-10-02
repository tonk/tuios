package app

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/overlay"
	"github.com/tonk/tuios/internal/pool"
	"github.com/tonk/tuios/internal/session"
	"github.com/tonk/tuios/internal/terminal"
	"github.com/tonk/tuios/internal/theme"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// agentStateIndicator returns the one-cell glyph that marks a pane's agent state
// in its window title, or the empty string for none/unset. The glyphs are
// deliberately distinct shapes rather than the same shape in different colors, so
// the state reads at a glance and survives a monochrome capture: a filled circle
// working, a triangle needs-input, a hollow circle idle, a filled square done,
// and a cross errored.
//
// ASCII-only terminals get a parallel set that keeps the same five states
// apart in one cell. Every surface that shows agent state goes through this
// function, so the rail, the title bars and the palette can never disagree.
func agentStateIndicator(state string) string {
	if overlay.UseASCII() {
		switch session.AgentState(state) {
		case session.AgentStateWorking:
			return "*"
		case session.AgentStateNeedsInput:
			return "!"
		case session.AgentStateIdle:
			return "o"
		case session.AgentStateDone:
			return "#"
		case session.AgentStateErrored:
			return "x"
		default:
			return ""
		}
	}
	switch session.AgentState(state) {
	case session.AgentStateWorking:
		return "●"
	case session.AgentStateNeedsInput:
		return "▲"
	case session.AgentStateIdle:
		return "○"
	case session.AgentStateDone:
		return "■"
	case session.AgentStateErrored:
		return "×"
	default:
		return ""
	}
}

// baseButtonStyle is the window-button ink, built fresh each call rather than
// cached so a theme's button_fg override (or a reload) takes effect.
func baseButtonStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.ButtonFg())
}

// titleBadgeText renders the text segment of a window title badge, coloring a
// leading agent-state glyph with the state's palette color while keeping the
// badge background intact. Each segment carries the full style, so the glyph's
// color can never bleed into the name or reset the badge behind it. The shape
// stays the state carrier (agentStateIndicator); the color is reinforcement,
// exactly as in the sidebar and the palette rows.
func titleBadgeText(windowName, agentState string, badgeBg color.Color) string {
	nameStyle := baseButtonStyle().Background(badgeBg)
	glyph := agentStateIndicator(agentState)
	if glyph == "" || !strings.HasPrefix(windowName, glyph) {
		return nameStyle.Render(" " + windowName + " ")
	}
	rest := strings.TrimPrefix(windowName, glyph)
	glyphStyle := lipgloss.NewStyle().
		Background(badgeBg).
		Foreground(agentGlyphColor(agentState, theme.UI())).
		Bold(true)
	return nameStyle.Render(" ") + glyphStyle.Render(glyph) + nameStyle.Render(rest+" ")
}

func getBorder() lipgloss.Border {
	return config.GetBorderForStyle()
}

func getNormalBorder() lipgloss.Border {
	return getBorder()
}

// RightString returns a right-aligned string with decorative borders.
func RightString(str string, width int, color color.Color) string {
	spaces := width - lipgloss.Width(str)
	style := pool.GetStyle()
	defer pool.PutStyle(style)
	fg := style.Foreground(color)

	if spaces < 0 {
		return ""
	}

	return fg.Render(config.GetWindowBorderTopLeft()+strings.Repeat(config.GetWindowBorderTop(), spaces)) +
		str +
		fg.Render(config.GetWindowBorderTopRight())
}

func makeRounded(content string, color color.Color) string {
	style := pool.GetStyle()
	defer pool.PutStyle(style)
	render := style.Foreground(color).Render
	content = render(config.GetWindowPillLeft()) + content + render(config.GetWindowPillRight())
	return content
}

// isDefaultTitle checks if the title is the auto-generated default (e.g., "Terminal 8bf1c038").
func isDefaultTitle(title, windowID string) bool {
	if len(windowID) < 8 {
		return false
	}
	return title == "Terminal "+windowID[:8]
}

// numberWindowName applies the window-number prefix or appearance.window_title_format
// a window's tab title carries to an already-resolved display name, so a name
// numbered for one surface (the tab) is numbered identically for another (the
// sidebar row), regardless of how each surface chose the name itself.
// position is the window's 1-based place in its workspace, used by the {index}
// placeholder of appearance.window_title_format.
func numberWindowName(windowName string, position int, cwd string) string {
	switch {
	case config.WindowTitleFormat != "":
		// A format that mentions only {index} or {cwd} still has something to
		// say about a window whose title is empty.
		return config.FormatWindowTitle(windowName, position, cwd)
	case config.ShowWindowNumber:
		if windowName != "" {
			return fmt.Sprintf("%d: %s", position, windowName)
		}
		return fmt.Sprintf("%d", position)
	}
	return windowName
}

// getWindowTitle returns the display name for a window, truncated to fit within maxWidth.
// Returns empty string if title should be hidden or doesn't fit.
// position is the window's 1-based place in its workspace, used by the {index}
// placeholder of appearance.window_title_format.
func getWindowTitle(window *terminal.Window, position int, maxWidth int) string {
	// Titles reach the badge as chrome, so launder the same decorative junk the
	// sidebar and palette drop; our own state glyph is added below, untouched.
	windowName := ""
	if window.CustomName != "" {
		windowName = printableTitle(window.CustomName)
	} else if t := window.Title(); t != "" && !isDefaultTitle(t, window.ID) {
		// Only show terminal-set title if it's not the default "Terminal <id>" format
		windowName = printableTitle(t)
	}
	windowName = numberWindowName(windowName, position, window.CWD())

	// The agent-state indicator shows even for a window with no name, so a pane
	// running an agent is always marked.
	indicator := agentStateIndicator(window.AgentState)

	if windowName == "" {
		return indicator
	}

	// Reserve room for the indicator and its trailing space before truncating.
	if indicator != "" {
		maxWidth = max(maxWidth-2, 0)
	}

	maxNameLen := max(maxWidth-6, 0)
	nameWidth := ansi.StringWidth(windowName)
	if nameWidth > maxNameLen {
		if maxNameLen > 3 {
			// Truncate by runes to handle unicode properly
			runes := []rune(windowName)
			truncated := string(runes)
			for ansi.StringWidth(truncated) > maxNameLen-3 && len(runes) > 0 {
				runes = runes[:len(runes)-1]
				truncated = string(runes)
			}
			windowName = truncated + "..."
		} else {
			return indicator
		}
	}
	if indicator != "" {
		return indicator + " " + windowName
	}
	return windowName
}

func addToBorder(content string, color color.Color, window *terminal.Window, position int, isTiling bool) string {
	width := max(lipgloss.Width(content)-2, 0)
	titlePos := config.WindowTitlePosition

	style := pool.GetStyle()
	defer pool.PutStyle(style)

	// Build window buttons first so we know their width
	var buttons string
	var buttonsWidth int
	if config.HideWindowButtons {
		buttons = ""
		buttonsWidth = 0
	} else {
		buttonStyle := baseButtonStyle().Background(color)
		cross := buttonStyle.Render(config.GetWindowButtonClose())
		dash := buttonStyle.Render("  - ")

		if isTiling {
			buttons = makeRounded(dash+cross, color)
		} else {
			square := buttonStyle.Render(config.GetWindowButtonMaximize())
			buttons = makeRounded(dash+square+cross, color)
		}
		buttonsWidth = lipgloss.Width(buttons)
	}

	// Calculate available width for title based on position
	var titleMaxWidth int
	if titlePos == "top" {
		// Title on top shares space with buttons
		titleMaxWidth = width - buttonsWidth - 2 // -2 for some padding
	} else {
		titleMaxWidth = width
	}

	windowName := ""
	if titlePos != "hidden" {
		windowName = getWindowTitle(window, position, titleMaxWidth)
	}

	borderStyle := style.Foreground(color)

	// Build top border
	var topBorder string
	if titlePos == "top" && windowName != "" {
		// Title on top with buttons on the right
		topBorder = renderTitleWithButtons(windowName, window.AgentState, buttons, width, color, true)
	} else {
		// Normal top border with buttons on right
		topBorder = RightString(buttons, width, color)
	}

	// Build bottom border with optional scrollback position indicator
	var bottomBorder string
	scrollIndicator := ""
	// Show scroll position when in copy mode with scroll offset
	if window.CopyMode != nil && window.CopyMode.Active && window.CopyMode.ScrollOffset > 0 {
		scrollbackLen := 0
		if window.Terminal != nil {
			scrollbackLen = window.Terminal.ScrollbackLen()
		}
		if scrollbackLen > 0 {
			scrollIndicator = fmt.Sprintf(" %d/%d ", window.CopyMode.ScrollOffset, scrollbackLen)
		}
	}

	if titlePos == "bottom" && windowName != "" {
		bottomBorder = renderTitleBadge(windowName, window.AgentState, width, color, false)
	} else if scrollIndicator != "" {
		// Bottom border with scrollback position indicator on the right
		indicatorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#fbbf24")).Bold(true)
		indicator := indicatorStyle.Render(scrollIndicator)
		indicatorWidth := lipgloss.Width(indicator)
		lineWidth := max(width-indicatorWidth, 0)
		bottomBorder = borderStyle.Render(config.GetWindowBorderBottomLeft()+strings.Repeat(config.GetWindowBorderBottom(), lineWidth)) + indicator + borderStyle.Render(config.GetWindowBorderBottomRight())
	} else {
		bottomBorder = borderStyle.Render(config.GetWindowBorderBottomLeft() + strings.Repeat(config.GetWindowBorderBottom(), width) + config.GetWindowBorderBottomRight())
	}

	lines := strings.Split(content, "\n")

	if len(lines) > 0 {
		lines[len(lines)-1] = bottomBorder
	}
	return topBorder + "\n" + strings.Join(lines, "\n")
}

// renderTitleWithButtons renders a top/bottom border with a title badge and buttons.
func renderTitleWithButtons(windowName, agentState string, buttons string, width int, color color.Color, isTop bool) string {
	style := pool.GetStyle()
	defer pool.PutStyle(style)
	borderStyle := style.Foreground(color)

	var borderChar, cornerLeft, cornerRight string
	if isTop {
		borderChar = config.GetWindowBorderTop()
		cornerLeft = config.GetWindowBorderTopLeft()
		cornerRight = config.GetWindowBorderTopRight()
	} else {
		borderChar = config.GetWindowBorderBottom()
		cornerLeft = config.GetWindowBorderBottomLeft()
		cornerRight = config.GetWindowBorderBottomRight()
	}

	// Build name badge
	leftCircle := borderStyle.Render(config.GetWindowPillLeft())
	nameText := titleBadgeText(windowName, agentState, color)
	rightCircle := borderStyle.Render(config.GetWindowPillRight())
	nameBadge := leftCircle + nameText + rightCircle

	nameBadgeWidth := lipgloss.Width(nameBadge)
	buttonsWidth := lipgloss.Width(buttons)

	// Calculate padding between title and buttons
	middlePadding := width - nameBadgeWidth - buttonsWidth
	if middlePadding < 0 {
		// Not enough space, just show buttons
		return RightString(buttons, width, color)
	}

	return borderStyle.Render(cornerLeft) +
		nameBadge +
		borderStyle.Render(strings.Repeat(borderChar, middlePadding)) +
		buttons +
		borderStyle.Render(cornerRight)
}

// renderTitleBadge renders a border with a centered title badge.
func renderTitleBadge(windowName, agentState string, width int, color color.Color, isTop bool) string {
	style := pool.GetStyle()
	defer pool.PutStyle(style)
	borderStyle := style.Foreground(color)

	var borderChar, cornerLeft, cornerRight string
	if isTop {
		borderChar = config.GetWindowBorderTop()
		cornerLeft = config.GetWindowBorderTopLeft()
		cornerRight = config.GetWindowBorderTopRight()
	} else {
		borderChar = config.GetWindowBorderBottom()
		cornerLeft = config.GetWindowBorderBottomLeft()
		cornerRight = config.GetWindowBorderBottomRight()
	}

	if windowName == "" {
		return borderStyle.Render(cornerLeft + strings.Repeat(borderChar, width) + cornerRight)
	}

	leftCircle := borderStyle.Render(config.GetWindowPillLeft())
	nameText := titleBadgeText(windowName, agentState, color)
	rightCircle := borderStyle.Render(config.GetWindowPillRight())
	nameBadge := leftCircle + nameText + rightCircle

	badgeWidth := lipgloss.Width(nameBadge)
	totalPadding := width - badgeWidth

	if totalPadding < 0 {
		return borderStyle.Render(cornerLeft + strings.Repeat(borderChar, width) + cornerRight)
	}

	leftPadding := totalPadding / 2
	rightPadding := totalPadding - leftPadding

	return borderStyle.Render(cornerLeft+strings.Repeat(borderChar, leftPadding)) +
		nameBadge +
		borderStyle.Render(strings.Repeat(borderChar, rightPadding)+cornerRight)
}

func styleToANSI(s lipgloss.Style) (prefix string, suffix string) {
	var te ansi.Style

	fg := s.GetForeground()
	bg := s.GetBackground()

	if _, ok := fg.(lipgloss.NoColor); !ok && fg != nil {
		te = te.ForegroundColor(ansi.Color(fg))
	}
	if _, ok := bg.(lipgloss.NoColor); !ok && bg != nil {
		te = te.BackgroundColor(ansi.Color(bg))
	}

	if s.GetBold() {
		te = te.Bold()
	}
	if s.GetItalic() {
		te = te.Italic(true)
	}
	if s.GetUnderline() {
		te = te.Underline(true)
	}
	if s.GetStrikethrough() {
		te = te.Strikethrough(true)
	}
	if s.GetBlink() {
		te = te.Blink(true)
	}
	if s.GetFaint() {
		te = te.Faint()
	}
	if s.GetReverse() {
		te = te.Reverse(true)
	}

	ansiStr := te.String()
	if ansiStr != "" {
		return ansiStr, "\x1b[0m"
	}
	return "", ""
}

func renderStyledText(style lipgloss.Style, text string) string {
	prefix, suffix := styleToANSI(style)
	if prefix == "" {
		return text
	}
	return prefix + text + suffix
}

// shouldApplyStyle reports whether cell needs any escape at all: a colour, a
// text attribute, an underline, or a hyperlink.
func shouldApplyStyle(cell *uv.Cell) bool {
	if cell == nil {
		return false
	}
	return !cell.Style.IsZero() || cell.Link.URL != ""
}

// buildOptimizedCellStyleCachedANSI returns the cached style together with its
// cached ANSI escape prefix/suffix, avoiding a styleToANSI rebuild on flush.
// It keeps every attribute, exactly like buildCellStyleCachedANSI.
func buildOptimizedCellStyleCachedANSI(cell *uv.Cell) (lipgloss.Style, string, string) {
	return GetGlobalStyleCache().GetWithANSI(cell, false, true)
}

// buildCellStyleCachedANSI returns the cached style together with its cached
// ANSI escape prefix/suffix, avoiding a styleToANSI rebuild on flush.
func buildCellStyleCachedANSI(cell *uv.Cell, isCursor bool) (lipgloss.Style, string, string) {
	return GetGlobalStyleCache().GetWithANSI(cell, isCursor, false)
}

// cellDisplayStyle is the uv.Style a cell is drawn with: its own, or the fake
// cursor block over it.
func cellDisplayStyle(cell *uv.Cell, isCursor bool) uv.Style {
	if cell == nil {
		if isCursor {
			return cursorCellStyle(uv.Style{})
		}
		return uv.Style{}
	}
	style := resolveCellStyle(cell.Style, nil)
	if isCursor {
		return cursorCellStyle(style)
	}
	return style
}

func isColorSafe(c color.Color) bool {
	if c == nil {
		return false
	}
	switch c.(type) {
	case lipgloss.ANSIColor, lipgloss.NoColor, lipgloss.RGBColor,
		color.RGBA, color.NRGBA, color.Gray, color.Gray16,
		color.RGBA64, color.CMYK, color.Alpha, color.Alpha16,
		color.YCbCr:
		return true
	default:
		// Unknown type  - attempt RGBA() and recover on panic
		safe := true
		func() {
			defer func() {
				if recover() != nil {
					safe = false
				}
			}()
			_, _, _, _ = c.RGBA()
		}()
		return safe
	}
}

// buildCellStyle returns the lipgloss rendering of the style a cell is drawn
// with. lipgloss has no conceal or rapid blink, so the escape the render loop
// writes comes from cellDisplayStyle instead (see styleEntry); this is kept for
// callers that want a lipgloss.Style.
func buildCellStyle(cell *uv.Cell, isCursor bool) lipgloss.Style {
	display := cellDisplayStyle(cell, isCursor)
	cellStyle := lipgloss.NewStyle()
	if display.Fg != nil {
		cellStyle = cellStyle.Foreground(display.Fg)
	}
	if display.Bg != nil {
		cellStyle = cellStyle.Background(display.Bg)
	}
	attrs := display.Attrs
	if attrs&uv.AttrBold != 0 {
		cellStyle = cellStyle.Bold(true)
	}
	if attrs&uv.AttrFaint != 0 {
		cellStyle = cellStyle.Faint(true)
	}
	if attrs&uv.AttrItalic != 0 {
		cellStyle = cellStyle.Italic(true)
	}
	if attrs&(uv.AttrBlink|uv.AttrRapidBlink) != 0 {
		cellStyle = cellStyle.Blink(true)
	}
	if attrs&uv.AttrReverse != 0 {
		cellStyle = cellStyle.Reverse(true)
	}
	if attrs&uv.AttrStrikethrough != 0 {
		cellStyle = cellStyle.Strikethrough(true)
	}
	if display.Underline != uv.UnderlineNone {
		cellStyle = cellStyle.UnderlineStyle(display.Underline)
	}
	if display.UnderlineColor != nil {
		cellStyle = cellStyle.UnderlineColor(display.UnderlineColor)
	}
	return cellStyle
}

// truncateToWidth cuts line to at most width cells as measured by
// ansi.StringWidth.
//
// ansi.Truncate and ansi.StringWidth disagree about malformed UTF-8: for a line
// carrying invalid bytes, Truncate can return a string that StringWidth then
// measures one or more cells over the limit. That reaches the compositor as a
// row wider than the space the layer was given, which bleeds into the pane next
// door. Guest programs can put arbitrary bytes in an OSC title and those titles
// are rendered into the window chrome, so this is reachable input, not a
// theoretical one. Re-measure and keep cutting until the result really fits.
func truncateToWidth(line string, width int) string {
	if width <= 0 {
		return ""
	}
	out := ansi.Truncate(line, width, "")
	// The overshoot is a cell or two in practice; the loop is bounded by width
	// regardless so a pathological line cannot spin here.
	for target := width; ansi.StringWidth(out) > width && target > 0; {
		target--
		out = ansi.Truncate(line, target, "")
	}
	return out
}

func clipWindowContent(content string, x, y, viewportWidth, viewportHeight int) (string, int, int) {
	lines := strings.Split(content, "\n")
	windowHeight := len(lines)

	// Reject on the axes that cost nothing to test first. Measuring the frame
	// width walks every line, and a window rejected for being off the right
	// edge or off the top or bottom does not need the width at all, so paying
	// for it before these three tests was pure waste.
	if x >= viewportWidth || y+windowHeight <= 0 || y >= viewportHeight {
		return "", max(x, 0), max(y, 0)
	}

	// The window is as wide as its widest line, not as wide as its first one.
	// Measuring only lines[0] under-reports the width whenever the top row is
	// blank, and the unfocused fast render path trims trailing spaces, so a
	// full-screen application with an empty first row (nvim, among others)
	// produced a frame starting with an empty line and measured as zero wide.
	// The offscreen guard below then read x+0 <= 0 as true for the leftmost
	// tile and discarded the whole frame, compositing the pane as bare
	// background while the rest of the layout carried on. The same
	// under-measurement also let the horizontal clip below be skipped for
	// content that really did overrun the viewport.
	windowWidth := framesWidth(lines)

	if x+windowWidth <= 0 {
		return "", max(x, 0), max(y, 0)
	}

	clipTop := 0
	clipLeft := 0
	finalX := x
	finalY := y

	if y < 0 {
		clipTop = -y
		finalY = 0
	}

	if x < 0 {
		clipLeft = -x
		finalX = 0
	}

	if clipTop >= len(lines) {
		return "", finalX, finalY
	}
	visibleLines := lines[clipTop:]

	maxVisibleLines := viewportHeight - finalY
	if maxVisibleLines < len(visibleLines) {
		visibleLines = visibleLines[:maxVisibleLines]
	}

	if clipLeft > 0 || finalX+windowWidth > viewportWidth {
		maxWidth := viewportWidth - finalX
		clippedLines := make([]string, len(visibleLines))

		for lineIdx, line := range visibleLines {
			w := lineWidth(line)

			if clipLeft >= w {
				clippedLines[lineIdx] = ""
				continue
			}

			tempLine := line
			if w > maxWidth+clipLeft {
				tempLine = truncateToWidth(line, maxWidth+clipLeft)
			}

			if clipLeft > 0 {
				clippedLines[lineIdx] = skipColumns(tempLine, clipLeft) + "\x1b[0m"
			} else {
				clippedLines[lineIdx] = tempLine
				if w > maxWidth {
					clippedLines[lineIdx] += "\x1b[0m"
				}
			}
		}

		// Enforce the width contract on the finished rows rather than trusting
		// the arithmetic that built them. A line that carries invalid bytes can
		// measure differently to the decoder in skipColumns and to
		// ansi.StringWidth, so the assembled row could come out wider than the
		// space it is being placed in and bleed into the pane next door. Guest programs can put arbitrary bytes in an
		// OSC title and those titles are rendered into the window chrome.
		for i, line := range clippedLines {
			if ansi.StringWidth(line) > maxWidth {
				clippedLines[i] = truncateToWidth(line, maxWidth) + "\x1b[0m"
			}
		}

		return strings.Join(clippedLines, "\n"), finalX, finalY
	}

	return strings.Join(visibleLines, "\n"), finalX, finalY
}

// skipColumns drops the first n columns of line, keeping every escape
// sequence so the styles and hyperlinks in force carry over to what remains.
//
// It counts columns the way the emulator and the compositor do, by grapheme
// cluster at ansi.GraphemeWidth, so a wide character is two columns and a
// combining mark travels with its base. Counting one per rune made a window
// clipped at the left screen edge lose too little of a CJK line and too much
// of an accented one, and shifted the rest of the row against its neighbours.
// A wide character cut in half by the edge leaves its visible half as a space,
// which keeps the row exactly width-n columns wide.
func skipColumns(line string, n int) string {
	var out strings.Builder
	out.Grow(len(line))
	col := 0
	var state byte
	for len(line) > 0 {
		seq, width, consumed, newState := ansi.DecodeSequence(line, state, nil)
		if consumed <= 0 {
			consumed = 1
			seq = line[:1]
		}
		state = newState
		line = line[consumed:]
		switch {
		case width == 0 && (ansi.HasEscPrefix(seq) || isControlSequence(seq)):
			// Escape and control sequences set state and take no column.
			out.WriteString(seq)
		case width == 0:
			// A stray zero-width cluster belongs with whatever precedes it.
			if col > n {
				out.WriteString(seq)
			}
		case col >= n:
			out.WriteString(seq)
		case col+width > n:
			// Straddles the edge: the part past it shows as blank columns.
			out.WriteString(strings.Repeat(" ", col+width-n))
		}
		col += width
	}
	return out.String()
}

// isControlSequence reports whether seq is a C0/C1 control or a string
// sequence introducer rather than text.
func isControlSequence(seq string) bool {
	if seq == "" {
		return false
	}
	c := seq[0]
	if c < 0x20 || c == 0x7f {
		return true
	}
	// C1 controls in their two-byte UTF-8 form (U+0080..U+009F).
	return c == 0xc2 && len(seq) > 1 && seq[1] >= 0x80 && seq[1] <= 0x9f
}

// workspacePosition returns the window's 1-based place among the windows of its
// workspace, the same number the leader-digit shortcuts address it by. Returns
// 0 for a window that is not in the list, which the title format renders as-is.
func (m *OS) workspacePosition(window *terminal.Window) int {
	position := 0
	for _, w := range m.Windows {
		if w.Workspace != window.Workspace {
			continue
		}
		if m.AutoTiling && w.Minimized {
			continue
		}
		position++
		if w == window {
			return position
		}
	}
	return 0
}
