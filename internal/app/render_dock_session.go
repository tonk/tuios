package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/tonk/tuios/internal/overlay"
	"github.com/tonk/tuios/internal/theme"
)

// dockSessionCell is one drawn session control: the styled cells and how many
// columns they take, so the caller can turn the strip's internal offsets into
// screen columns without measuring styled text a second time.
type dockSessionCell struct {
	Text   string
	Width  int
	Action DockSessionAction
}

// buildDockSessionStrip renders the session controls as they sit at the dock's
// right-hand end, and returns the cells in drawn order.
//
// The strip opens and closes with a bare column. The closing one is what keeps
// the destructive control off the screen's last column: a pointer thrown at the
// right edge stops on a cell that does nothing, rather than on the one button
// here that cannot be undone.
func (m *OS) buildDockSessionStrip() (string, []dockSessionCell) {
	if !dockSessionControlsFit(m.GetRenderWidth()) {
		return "", nil
	}

	pal := theme.UI()
	dr := currentDockRow(pal)
	cells := make([]dockSessionCell, 0, 2)
	if m.CanLeaveRunning() {
		cells = append(cells, m.dockSessionCell(DockSessionLeave, pal, dr))
	}
	cells = append(cells, m.dockSessionCell(DockSessionClose, pal, dr))

	var b strings.Builder
	b.WriteString(dr.fill(" "))
	for _, c := range cells {
		b.WriteString(c.Text)
	}
	b.WriteString(dr.fill(" "))
	return b.String(), cells
}

// dockSessionStripWidth is the strip's column span, used by the layout pass to
// lay the rest of the bar out against what the controls leave. It builds the
// same strip the renderer does rather than adding up the constants again, which
// is the arithmetic that would silently drift the moment a label changes.
func (m *OS) dockSessionStripWidth() int {
	strip, _ := m.buildDockSessionStrip()
	return lipgloss.Width(strip)
}

// dockSessionCell styles one control.
//
// The weight split is the whole design: leaving is normal dock text and bold,
// closing is quieter until the pointer arrives and then goes destructive.
// Neither wears a fill, which is still spent entirely on the mode pill.
//
// Every state goes through theme.Readable against dr.contrastBg: pal.Canvas,
// today's assumption about the bare canvas the bar sits on, or a theme's
// dock_bg override when one is set - whichever it measures against is what
// will actually be behind it once dr.background below paints the cell. The
// recessed one has to be recessed and still legible, and it was neither once
// the word beside it went: FgMute measured 2.60:1, which was a hint next to a
// label and is the whole control without one. Warn and AccentBright follow the
// terminal theme, so they are measured for the same reason the workspace pills'
// accent is.
func (m *OS) dockSessionCell(a DockSessionAction, pal overlay.Palette, dr dockRowStyle) dockSessionCell {
	// A column of padding either side, so the target is a button and not a
	// glyph, and so the two controls do not touch.
	body := " " + dockSessionIcon(a) + " "

	st := lipgloss.NewStyle()
	hovered := m.dockSessionHover == a
	switch {
	case a == DockSessionLeave && hovered:
		st = st.Foreground(theme.Readable(pal.AccentBright, dr.contrastBg)).Bold(true)
	case a == DockSessionLeave:
		st = st.Foreground(pal.Fg).Bold(true)
	case hovered:
		st = st.Foreground(theme.Readable(pal.Warn, dr.contrastBg)).Bold(true)
	default:
		st = st.Foreground(theme.Readable(pal.FgMute, dr.contrastBg))
	}
	st = dr.background(st)

	return dockSessionCell{Text: st.Render(body), Width: lipgloss.Width(body), Action: a}
}
