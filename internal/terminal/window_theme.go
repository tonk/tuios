package terminal

import (
	"image/color"

	"github.com/tonk/tuios/internal/theme"
)

// UpdateThemeColors pushes the active theme's palette into the emulator so
// palette colors already on screen resolve to the new theme on the next
// render: cells keep SGR 30-37/90-97 and 38;5;n as palette entries and the
// renderer resolves them through Emulator.ResolveColor every frame. That holds
// for cells a daemon pane was rebuilt from as well, because the snapshot wire
// (session.CellState) carries the kind of each color and restores a palette
// entry as a palette entry. Only cells that were RGB to begin with (38;2;r;g;b)
// keep their shade. SetThemeColors mutates the emulator's color table, which
// the PTY reader goroutine reads under ioMu inside Terminal.Write, so it is
// taken here (this runs on the UI goroutine) to avoid a torn interface-value
// read.
func (w *Window) UpdateThemeColors() {
	w.ioMu.Lock()
	if w.Terminal != nil {
		if theme.IsEnabled() {
			w.Terminal.SetThemeColors(
				theme.TerminalFg(),
				theme.TerminalBg(),
				theme.TerminalCursor(),
				theme.GetANSIPalette(),
			)
		} else {
			w.Terminal.SetThemeColors(nil, nil, nil, [16]color.Color{})
		}
	}
	w.ioMu.Unlock()

	// Mark dirty and drop the cached render: the palette changed, so both the
	// cached content string and the cached styled layer are stale.
	w.Dirty = true
	w.ContentDirty = true
	w.InvalidateCache()
}
