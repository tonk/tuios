package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/overlay"
	"github.com/tonk/tuios/internal/theme"
)

// whichKeyGap is the blank cells between adjacent which-key columns.
const whichKeyGap = 2

type whichKeyRowKind int

const (
	whichKeyBinding whichKeyRowKind = iota
	whichKeyHeader
)

type whichKeyRow struct {
	kind  whichKeyRowKind
	key   string
	desc  string
	title string
}

type whichKeyCol struct {
	rows     []whichKeyRow
	maxKey   int
	maxDesc  int
	maxTitle int
}

func (c *whichKeyCol) addHeader(title string) {
	c.rows = append(c.rows, whichKeyRow{kind: whichKeyHeader, title: title})
	if len(title) > c.maxTitle {
		c.maxTitle = len(title)
	}
}

func (c *whichKeyCol) addBinding(b config.Keybinding) {
	c.rows = append(c.rows, whichKeyRow{kind: whichKeyBinding, key: b.Key, desc: b.Description})
	if len(b.Key) > c.maxKey {
		c.maxKey = len(b.Key)
	}
	if len(b.Description) > c.maxDesc {
		c.maxDesc = len(b.Description)
	}
}

func (c *whichKeyCol) naturalWidth() int {
	bindingW := c.maxKey + 2 + c.maxDesc
	return max(bindingW, c.maxTitle, 1)
}

func (c *whichKeyCol) height() int {
	return len(c.rows)
}

// buildWhichKeyColumns packs labeled groups into one or more columns. Multiple
// columns are used when the screen is wide enough, so group headers can appear
// without forcing a scrolling single-column tower.
func buildWhichKeyColumns(groups []config.KeybindingGroup, screenW int) []whichKeyCol {
	filtered := make([]config.KeybindingGroup, 0, len(groups))
	for _, g := range groups {
		if len(g.Bindings) == 0 {
			continue
		}
		filtered = append(filtered, g)
	}
	if len(filtered) == 0 {
		return nil
	}

	// Available inner content width: screen minus overlay side pad (2+2) and
	// the two-cell margin from the screen edge on each side.
	avail := screenW - 8
	if avail < 1 {
		avail = 1
	}

	colCount := 1
	titled := false
	for _, g := range filtered {
		if g.Title != "" {
			titled = true
			break
		}
	}
	if titled && len(filtered) > 1 {
		// Widest group sets the per-column floor. Prefer more columns when
		// that many copies (plus gaps) still fit, so headers cost height in
		// parallel rather than as a taller stack.
		widest := 0
		for _, g := range filtered {
			var c whichKeyCol
			if g.Title != "" {
				c.addHeader(g.Title)
			}
			for _, b := range g.Bindings {
				c.addBinding(b)
			}
			if w := c.naturalWidth(); w > widest {
				widest = w
			}
		}
		for try := min(3, len(filtered)); try >= 2; try-- {
			need := try*widest + (try-1)*whichKeyGap
			if need <= avail {
				colCount = try
				break
			}
		}
	}

	cols := make([]whichKeyCol, colCount)
	perCol := (len(filtered) + colCount - 1) / colCount
	for i, g := range filtered {
		target := i / perCol
		if target >= colCount {
			target = colCount - 1
		}
		if g.Title != "" {
			cols[target].addHeader(g.Title)
		}
		for _, b := range g.Bindings {
			cols[target].addBinding(b)
		}
		// Blank spacer between stacked groups in the same column.
		nextTarget := (i + 1) / perCol
		if i+1 < len(filtered) {
			if nextTarget >= colCount {
				nextTarget = colCount - 1
			}
			if nextTarget == target {
				cols[target].rows = append(cols[target].rows, whichKeyRow{kind: whichKeyHeader, title: ""})
			}
		}
	}
	return cols
}

// shrinkWhichKeyColumns trims description columns (and then drops trailing
// bindings) so the overlay fits the screen. Returns how many bindings were
// omitted.
func shrinkWhichKeyColumns(cols []whichKeyCol, availW, maxRows int) int {
	if len(cols) == 0 {
		return 0
	}

	fitWidths := func() {
		for {
			total := (len(cols) - 1) * whichKeyGap
			for i := range cols {
				total += cols[i].naturalWidth()
			}
			if total <= availW {
				return
			}
			// Shrink the widest description column by one cell.
			widest := -1
			for i := range cols {
				if cols[i].maxDesc <= 1 {
					continue
				}
				if widest < 0 || cols[i].maxDesc > cols[widest].maxDesc {
					widest = i
				}
			}
			if widest < 0 {
				return
			}
			cols[widest].maxDesc--
		}
	}
	fitWidths()

	dropped := 0
	for maxRows > 0 {
		tallest := 0
		for i := 1; i < len(cols); i++ {
			if cols[i].height() > cols[tallest].height() {
				tallest = i
			}
		}
		if cols[tallest].height() <= maxRows {
			break
		}
		// Drop the last binding row from the tallest column (skip headers).
		c := &cols[tallest]
		cut := -1
		for i := len(c.rows) - 1; i >= 0; i-- {
			if c.rows[i].kind == whichKeyBinding {
				cut = i
				break
			}
		}
		if cut < 0 {
			// Only a header left; remove the whole column stub.
			if len(c.rows) > 0 {
				c.rows = c.rows[:len(c.rows)-1]
			}
			continue
		}
		c.rows = append(c.rows[:cut], c.rows[cut+1:]...)
		dropped++
		// Recompute key/desc maxima after a drop.
		c.maxKey, c.maxDesc, c.maxTitle = 0, 0, 0
		for _, row := range c.rows {
			switch row.kind {
			case whichKeyHeader:
				if len(row.title) > c.maxTitle {
					c.maxTitle = len(row.title)
				}
			case whichKeyBinding:
				if len(row.key) > c.maxKey {
					c.maxKey = len(row.key)
				}
				if len(row.desc) > c.maxDesc {
					c.maxDesc = len(row.desc)
				}
			}
		}
		fitWidths()
	}
	return dropped
}

// renderWhichKeyPanel styles the which-key columns into the floating overlay
// string (including outer padding rows).
func renderWhichKeyPanel(title string, groups []config.KeybindingGroup, screenW, screenH int) string {
	pal := theme.UI()
	bg := pal.Surface

	availW := screenW - 8
	if availW < 1 {
		availW = 1
	}
	// Title, rule, top/bottom empty pads, and a two-cell screen-edge margin.
	maxRows := max(screenH-6, 1)

	cols := buildWhichKeyColumns(groups, screenW)
	moreCount := shrinkWhichKeyColumns(cols, availW, maxRows)

	padLine := func(s string, targetWidth int) string {
		return overlay.Fill(s, targetWidth, bg)
	}

	contentWidth := len(title)
	if moreCount > 0 {
		moreLen := len(fmt.Sprintf("+%d more", moreCount))
		if moreLen > contentWidth {
			contentWidth = moreLen
		}
	}
	total := (len(cols) - 1) * whichKeyGap
	for i := range cols {
		total += cols[i].naturalWidth()
	}
	if total > contentWidth {
		contentWidth = total
	}
	if contentWidth > availW {
		contentWidth = availW
	}

	var bodyLines []string
	height := 0
	for _, c := range cols {
		if c.height() > height {
			height = c.height()
		}
	}
	for row := 0; row < height; row++ {
		var parts []string
		for i := range cols {
			c := &cols[i]
			colW := c.naturalWidth()
			if i == len(cols)-1 {
				// Last column absorbs leftover width so the panel edge is flush.
				used := 0
				for j := 0; j < i; j++ {
					used += cols[j].naturalWidth() + whichKeyGap
				}
				colW = max(colW, contentWidth-used)
			}
			var cell string
			if row < len(c.rows) {
				r := c.rows[row]
				switch r.kind {
				case whichKeyHeader:
					cell = overlay.Style(bg).Foreground(pal.FgMute).Bold(true).
						Render(truncateString(r.title, colW))
				case whichKeyBinding:
					descW := max(colW-c.maxKey-2, 1)
					cell = overlay.Style(bg).Foreground(pal.AccentBright).Bold(true).Render(r.key) +
						overlay.Style(bg).Render(strings.Repeat(" ", c.maxKey-len(r.key)+2)) +
						overlay.Style(bg).Foreground(pal.FgDim).Render(truncateString(r.desc, descW))
				}
			}
			parts = append(parts, padLine(cell, colW))
			if i < len(cols)-1 {
				parts = append(parts, overlay.Style(bg).Render(strings.Repeat(" ", whichKeyGap)))
			}
		}
		bodyLines = append(bodyLines, strings.Join(parts, ""))
	}

	var styledLines []string
	titleStyled := overlay.Style(bg).Foreground(pal.Fg).Bold(true).
		Render(truncateString(strings.ToLower(title), contentWidth))
	styledLines = append(styledLines, padLine(titleStyled, contentWidth))
	styledLines = append(styledLines, overlay.Rule(contentWidth, bg, pal))
	styledLines = append(styledLines, bodyLines...)
	if moreCount > 0 {
		more := overlay.Style(bg).Foreground(pal.FgMute).
			Render(truncateString(fmt.Sprintf("+%d more", moreCount), contentWidth))
		styledLines = append(styledLines, padLine(more, contentWidth))
	}

	paddingH := overlay.Style(bg).Render("  ")
	emptyLine := overlay.Style(bg).Render(strings.Repeat(" ", contentWidth+4))

	finalLines := make([]string, 0, len(styledLines)+2)
	finalLines = append(finalLines, emptyLine)
	for _, line := range styledLines {
		finalLines = append(finalLines, paddingH+line+paddingH)
	}
	finalLines = append(finalLines, emptyLine)
	return strings.Join(finalLines, "\n")
}

// whichKeyLayer builds the positioned which-key lipgloss layer, or nil when
// there is nothing to show.
func (m *OS) whichKeyLayer(title string, groups []config.KeybindingGroup) *lipgloss.Layer {
	if len(groups) == 0 {
		return nil
	}
	// Minimize submenu: annotate the restore row with how many are docked.
	if title == "Minimize" {
		minimizedCount := 0
		for _, win := range m.Windows {
			if win.Minimized && win.Workspace == m.CurrentWorkspace {
				minimizedCount++
			}
		}
		groups = append([]config.KeybindingGroup(nil), groups...)
		for gi := range groups {
			groups[gi].Bindings = append([]config.Keybinding(nil), groups[gi].Bindings...)
			for bi := range groups[gi].Bindings {
				if groups[gi].Bindings[bi].Description == "Restore window" {
					groups[gi].Bindings[bi].Description = fmt.Sprintf("Restore window (%d minimized)", minimizedCount)
				}
			}
		}
	}

	renderWidth := m.GetRenderWidth()
	renderHeight := m.GetRenderHeight()
	rendered := renderWhichKeyPanel(title, groups, renderWidth, renderHeight)

	overlayWidth := lipgloss.Width(rendered)
	overlayHeight := lipgloss.Height(rendered)
	var overlayX, overlayY int
	switch config.WhichKeyPosition {
	case "top-left":
		overlayX = 2
		overlayY = 1
	case "top-right":
		overlayX = renderWidth - overlayWidth - 2
		overlayY = 1
	case "bottom-left":
		overlayX = 2
		overlayY = renderHeight - overlayHeight - 2
	case "center":
		overlayX = (renderWidth - overlayWidth) / 2
		overlayY = (renderHeight - overlayHeight) / 2
	default:
		overlayX = renderWidth - overlayWidth - 2
		overlayY = renderHeight - overlayHeight - 2
	}
	overlayX = max(min(overlayX, renderWidth-overlayWidth), 0)
	overlayY = max(min(overlayY, renderHeight-overlayHeight), 0)

	return lipgloss.NewLayer(rendered).
		X(overlayX).
		Y(overlayY).
		Z(config.ZIndexWhichKey).
		ID("whichkey")
}
