package input

import (
	"fmt"
	"testing"

	"github.com/tonk/tuios/internal/app"
	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/terminal"
)

// TestDockWindowListClickFocusesAnUnminimizedEntry is dock_window_list's own
// half of the dock item click path: with it on, the strip carries windows
// that were never minimized, and clicking one has to focus it (RestoreWindow
// is a no-op for a window that is not minimized, since it exists to reverse
// the minimize animation).
func TestDockWindowListClickFocusesAnUnminimizedEntry(t *testing.T) {
	prev := config.DockWindowList
	config.DockWindowList = true
	t.Cleanup(func() { config.DockWindowList = prev })

	cfg := config.DefaultConfig()
	o := app.NewOS(app.OSOptions{
		UserConfig:      cfg,
		KeybindRegistry: config.NewKeybindRegistry(cfg),
	})
	o.Width, o.Height = 80, 30
	o.EffectiveWidth, o.EffectiveHeight = 80, 30
	o.CurrentWorkspace = 1
	for i := range 2 {
		o.Windows = append(o.Windows, &terminal.Window{
			ID: fmt.Sprintf("w%d", i), CustomName: fmt.Sprintf("win%d", i),
			Workspace: 1,
		})
	}
	o.FocusedWindow = 0
	_ = o.View()

	y := o.GetDockbarContentYPosition()
	target := -1
	for x := range o.Width {
		if idx := o.DockItemAt(x, y); idx == 1 {
			target = x
			break
		}
	}
	if target < 0 {
		t.Fatal("dock_window_list did not draw a dock entry for the unfocused, unminimized window")
	}

	o, cmd := handleMouseClick(clickAt(target, y), o)
	if o.FocusedWindow != 1 {
		t.Errorf("clicking an unminimized dock entry left focus at %d, want 1", o.FocusedWindow)
	}
	if o.Windows[1].Minimized {
		t.Error("focusing an unminimized entry through the dock minimized it")
	}
	if cmd != nil {
		t.Error("focusing a dock entry returned a command; it is a synchronous state change")
	}
}

// TestDockWindowListClickStillRestoresAMinimizedEntry guards the other branch
// of the same click path: a minimized entry in the list still goes through
// RestoreWindow, not a bare focus that would leave it minimized.
func TestDockWindowListClickStillRestoresAMinimizedEntry(t *testing.T) {
	prev := config.DockWindowList
	config.DockWindowList = true
	t.Cleanup(func() { config.DockWindowList = prev })

	cfg := config.DefaultConfig()
	o := app.NewOS(app.OSOptions{
		UserConfig:      cfg,
		KeybindRegistry: config.NewKeybindRegistry(cfg),
	})
	o.Width, o.Height = 80, 30
	o.EffectiveWidth, o.EffectiveHeight = 80, 30
	o.CurrentWorkspace = 1
	o.Windows = append(o.Windows, &terminal.Window{
		ID: "w0", CustomName: "win0", Workspace: 1,
	})
	o.Windows = append(o.Windows, &terminal.Window{
		ID: "w1", CustomName: "win1", Workspace: 1, Minimized: true, MinimizeOrder: 1,
	})
	o.FocusedWindow = 0
	_ = o.View()

	y := o.GetDockbarContentYPosition()
	target := -1
	for x := range o.Width {
		if idx := o.DockItemAt(x, y); idx == 1 {
			target = x
			break
		}
	}
	if target < 0 {
		t.Fatal("dock_window_list did not draw a dock entry for the minimized window")
	}

	o, _ = handleMouseClick(clickAt(target, y), o)
	if o.Windows[1].Minimized {
		t.Error("clicking a minimized entry in the dock window list left it minimized")
	}
	if o.FocusedWindow != 1 {
		t.Errorf("restoring a minimized dock entry left focus at %d, want 1", o.FocusedWindow)
	}
}
