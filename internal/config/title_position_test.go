package config_test

import (
	"testing"

	"github.com/tonk/tuios/internal/config"
)

// withTitlePositionAndHideClock restores the globals these tests move, since
// they are package state shared with every other test in the run.
func withTitlePositionAndHideClock(t *testing.T) {
	t.Helper()
	prevPos, prevHide := config.WindowTitlePosition, config.HideClock
	t.Cleanup(func() { config.WindowTitlePosition, config.HideClock = prevPos, prevHide })
}

// A reload moves the title between every pair of positions. The apply step
// used to take the config value only while the title was still at "bottom",
// so once a reload had moved it, later reloads could not move it again.
func TestWindowTitlePositionFollowsEveryReload(t *testing.T) {
	withTitlePositionAndHideClock(t)
	config.WindowTitlePosition = "bottom"

	for _, pos := range []string{"top", "hidden", "bottom", "top"} {
		cfg := config.DefaultConfig()
		cfg.Appearance.WindowTitlePosition = pos
		config.ApplyAppearanceConfig(cfg)
		if config.WindowTitlePosition != pos {
			t.Errorf("WindowTitlePosition = %q after applying %q", config.WindowTitlePosition, pos)
		}
	}
}

// A typo lands on the default the validator says it falls back to, instead of
// an unknown position under which no title is drawn at all.
func TestWindowTitlePositionRejectsAnUnknownValue(t *testing.T) {
	withTitlePositionAndHideClock(t)
	config.WindowTitlePosition = "top"

	cfg := config.DefaultConfig()
	cfg.Appearance.WindowTitlePosition = "botom"
	config.ApplyAppearanceConfig(cfg)
	if config.WindowTitlePosition != "bottom" {
		t.Errorf("WindowTitlePosition = %q after applying a typo, want \"bottom\"", config.WindowTitlePosition)
	}

	config.WindowTitlePosition = "top"
	config.ApplyOverrides(config.Overrides{}, cfg)
	if config.WindowTitlePosition != "top" {
		t.Errorf("ApplyOverrides took the invalid config value: WindowTitlePosition = %q", config.WindowTitlePosition)
	}
}

// Removing the deprecated hide_clock from the file takes effect on a reload;
// it used to stay on until a restart.
func TestHideClockCanBeTurnedOffByReload(t *testing.T) {
	withTitlePositionAndHideClock(t)

	cfg := config.DefaultConfig()
	cfg.Appearance.HideClock = true
	config.ApplyAppearanceConfig(cfg)
	if !config.HideClock {
		t.Fatal("HideClock = false after applying hide_clock = true")
	}

	cfg.Appearance.HideClock = false
	config.ApplyAppearanceConfig(cfg)
	if config.HideClock {
		t.Error("HideClock still true after a reload without hide_clock")
	}
}
