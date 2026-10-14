package app

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/theme"
)

// While the host terminal window is being resized, a small popup in the middle
// of the screen shows its new size in columns x rows, the way Ghostty's
// resize overlay does, so a window can be dragged to an exact size
// (appearance.show_resize_size).
//
// It stays up for resizeSizeHold after the last size arrives. Every resize
// restarts that wait by bumping resizeSizeGen, so a drag shows the popup
// throughout and only the hide scheduled by the final size takes it down.

// resizeSizeStartupGrace is how long after the first size a resize still
// counts as the window settling rather than the user resizing it. A terminal
// started maximized or fullscreen (kitty --start-as=maximized) opens at its
// default size and is enlarged by the window manager a moment later, which
// arrives as an ordinary resize; so can a tiling window manager placing a new
// window. None of that is the user dragging an edge.
const resizeSizeStartupGrace = 2 * time.Second

// resizeSizeHold is how long the popup stays after the last resize: long
// enough to read the final size once the pointer has let go.
const resizeSizeHold = 1200 * time.Millisecond

// ResizeSizeHideMsg takes the size popup down, unless a later resize has
// rescheduled it: Gen names the resize that sent it.
type ResizeSizeHideMsg struct {
	Gen uint64
}

// showResizeSize puts the size popup up for a resize and returns the command
// that takes it down again. Nil with appearance.show_resize_size off.
func (m *OS) showResizeSize() tea.Cmd {
	if !config.ShowResizeSize {
		return nil
	}
	m.resizeSizeShown = true
	m.resizeSizeGen++
	gen := m.resizeSizeGen
	return tea.Tick(resizeSizeHold, func(time.Time) tea.Msg {
		return ResizeSizeHideMsg{Gen: gen}
	})
}

// hideResizeSize takes the popup down if gen is still the newest resize. The
// frame it leaves behind has to be drawn now: the session may well have gone
// idle, and an idle tick would serve the cached frame with the popup in it.
func (m *OS) hideResizeSize(gen uint64) {
	if gen != m.resizeSizeGen || !m.resizeSizeShown {
		return
	}
	m.resizeSizeShown = false
	m.renderSkipped = false
	m.cachedViewContent = ""
}

// ResizeSizeShown reports whether the size popup is up, for tests.
func (m *OS) ResizeSizeShown() bool { return m.resizeSizeShown }

// resizeSizeLayer is the size popup, centred on the screen, or nil when it is
// not up. Turning the option off also takes a popup already up away.
func (m *OS) resizeSizeLayer() *lipgloss.Layer {
	if !m.resizeSizeShown || !config.ShowResizeSize {
		return nil
	}
	sep, border := "×", lipgloss.RoundedBorder()
	if config.UseASCIIOnly {
		sep, border = "x", lipgloss.ASCIIBorder()
	}
	pal := theme.UI()
	box := lipgloss.NewStyle().
		Border(border).
		BorderForeground(pal.Accent).
		BorderBackground(pal.Surface).
		Background(pal.Surface).
		Foreground(pal.Fg).
		Bold(true).
		Padding(0, 2).
		Render(fmt.Sprintf("%d %s %d", m.Width, sep, m.Height))
	return m.centeredBoxLayer(box, config.ZIndexNotifications, "resize-size")
}
