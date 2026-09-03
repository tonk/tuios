package input

import (
	"testing"

	"github.com/tonk/tuios/internal/app"
	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/terminal"
)

// dockIndicatorOS is dockSessionOS with the three mode-indicator glyphs
// turned on, for tests of clicking them.
func dockIndicatorOS(t *testing.T) *app.OS {
	t.Helper()
	cfg := config.DefaultConfig()
	o := app.NewOS(app.OSOptions{
		UserConfig:      cfg,
		KeybindRegistry: config.NewKeybindRegistry(cfg),
	})
	o.Width, o.Height = 160, 40
	o.EffectiveWidth, o.EffectiveHeight = 160, 40
	o.Windows = []*terminal.Window{
		{ID: "a", CustomName: "editor", X: 0, Y: 0, Width: 60, Height: 30, Workspace: 1},
	}
	o.CurrentWorkspace, o.FocusedWindow = 1, 0

	prevMouse, prevTiling, prevFFM := config.ShowMouseIndicator, config.ShowTilingIndicator, config.ShowFocusFollowsMouseIndicator
	config.ShowMouseIndicator, config.ShowTilingIndicator, config.ShowFocusFollowsMouseIndicator = true, true, true
	t.Cleanup(func() {
		config.ShowMouseIndicator, config.ShowTilingIndicator, config.ShowFocusFollowsMouseIndicator = prevMouse, prevTiling, prevFFM
	})

	_ = o.View()
	return o
}

// dockIndicatorColumn scans the dock row for a column routing to the given
// indicator, or -1.
func dockIndicatorColumn(o *app.OS, want app.DockIndicatorKind) int {
	y := o.Height - 1
	for x := range o.Width {
		if o.DockIndicatorAt(x, y) == want {
			return x
		}
	}
	return -1
}

// TestClickingIndicatorGlyphsTogglesTheirMode is the whole point of making
// them clickable: the same toggle a keybind or the settings page reaches,
// through a single cell in the dock.
func TestClickingIndicatorGlyphsTogglesTheirMode(t *testing.T) {
	t.Run("mouse", func(t *testing.T) {
		o := dockIndicatorOS(t)
		before := config.MouseEnabled
		t.Cleanup(func() { config.MouseEnabled = before })
		x := dockIndicatorColumn(o, app.DockIndicatorMouse)
		if x < 0 {
			t.Fatal("the dock drew no mouse indicator")
		}
		o, _ = handleMouseClick(clickAt(x, o.Height-1), o)
		if config.MouseEnabled == before {
			t.Error("clicking the mouse indicator did not toggle mouse mode")
		}
		if len(o.Notifications) == 0 {
			t.Error("clicking the mouse indicator raised no notification")
		}
	})

	t.Run("tiling", func(t *testing.T) {
		o := dockIndicatorOS(t)
		before := o.AutoTiling
		x := dockIndicatorColumn(o, app.DockIndicatorTiling)
		if x < 0 {
			t.Fatal("the dock drew no tiling indicator")
		}
		o, _ = handleMouseClick(clickAt(x, o.Height-1), o)
		if o.AutoTiling == before {
			t.Error("clicking the tiling indicator did not toggle tiling")
		}
		if len(o.Notifications) == 0 {
			t.Error("clicking the tiling indicator raised no notification")
		}
	})

	t.Run("focus follows mouse", func(t *testing.T) {
		o := dockIndicatorOS(t)
		before := config.FocusFollowsMouse
		t.Cleanup(func() { config.FocusFollowsMouse = before })
		x := dockIndicatorColumn(o, app.DockIndicatorFocusFollowsMouse)
		if x < 0 {
			t.Fatal("the dock drew no focus-follows-mouse indicator")
		}
		o, _ = handleMouseClick(clickAt(x, o.Height-1), o)
		if config.FocusFollowsMouse == before {
			t.Error("clicking the focus-follows-mouse indicator did not toggle it")
		}
		if len(o.Notifications) == 0 {
			t.Error("clicking the focus-follows-mouse indicator raised no notification")
		}
	})
}
