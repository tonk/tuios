package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// reportGraphemeWidth is a tea.Cmd that puts Bubble Tea's output buffer on
// grapheme-cluster width.
//
// Everything TUIOS lays out is measured by grapheme cluster: the VT emulator
// places cells with ansi.GraphemeWidth, and lipgloss and the compositor measure
// the same way. Bubble Tea parses the finished frame (view.SetContent) into its
// own cell buffer, and that buffer measures with wcwidth unless the host
// terminal answers a DECRQM ?2027 query, which Bubble Tea only sends for some
// TERM values and not at all for most SSH sessions. The two rules disagree on
// a VS16 emoji (❤️, ✔️, ⚠️) and on flags (🇳🇱): two cells to TUIOS, one to wcwidth.
// So every cell after such an emoji was a column off in Bubble Tea's model and
// pane borders came out jagged.
//
// There is no program option for the width method. The supported switch is the
// mode report itself: the event loop reacts to a ModeReportMsg for
// ansi.ModeUnicodeCore by moving the renderer to ansi.GraphemeWidth and
// writing DECSET ?2027 to the host. Delivering that report from Init makes the
// switch unconditional, for the local program and for every SSH and web
// session alike, because each runs this model's Init.
//
// The trade-off: on a host that supports ?2027 (Ghostty, foot, WezTerm and contour,
// among others) the DECSET makes host, Bubble Tea and TUIOS agree
// exactly. On a host that ignores it and draws with wcwidth, a VS16 emoji is
// drawn one cell wide while both TUIOS and Bubble Tea count two. That costs at
// most the rest of the line the emoji is on: Bubble Tea's terminal renderer
// re-anchors the cursor with an absolute column move after any line holding a
// wide cell when the host has not negotiated grapheme width, so the next line
// is placed correctly again. Staying on wcwidth instead would put Bubble Tea
// at odds with TUIOS's own layout on every host, including the ones that do
// grapheme width right, and shift everything after the emoji, borders
// included, on every frame.
func reportGraphemeWidth() tea.Msg {
	return tea.ModeReportMsg{Mode: ansi.ModeUnicodeCore, Value: ansi.ModeSet}
}
