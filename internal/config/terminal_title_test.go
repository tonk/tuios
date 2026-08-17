package config

import "testing"

// withSetTerminalTitle restores the global an apply pass writes.
func withSetTerminalTitle(t *testing.T) {
	t.Helper()
	prev := SetTerminalTitle
	t.Cleanup(func() { SetTerminalTitle = prev })
}

// TestSetTerminalTitleDefaultsOn is the migration half of the tri-state: a
// config file written before the key existed has no line for it, and must come
// up with the title-setting behavior on rather than reading the absence as off.
func TestSetTerminalTitleDefaultsOn(t *testing.T) {
	withSetTerminalTitle(t)
	SetTerminalTitle = true

	cfg := loadTOML(t, `
[appearance]
border_style = "rounded"
`)
	if cfg.Appearance.SetTerminalTitle != nil {
		t.Fatalf("an absent key parsed as non-nil: %v", *cfg.Appearance.SetTerminalTitle)
	}
	ApplyAppearanceConfig(cfg)
	if !SetTerminalTitle {
		t.Error("an old config file turned terminal-title-setting off by saying nothing")
	}
}

// TestSetTerminalTitleExplicitFalseSurvivesApply is the other half: turning it
// off in the settings page has to survive a reload.
func TestSetTerminalTitleExplicitFalseSurvivesApply(t *testing.T) {
	withSetTerminalTitle(t)
	SetTerminalTitle = true

	cfg := loadTOML(t, `
[appearance]
set_terminal_title = false
`)
	ApplyAppearanceConfig(cfg)
	if SetTerminalTitle {
		t.Error("an explicit false was dropped")
	}
}

// withDockWindowList restores the global an apply pass writes.
func withDockWindowList(t *testing.T) {
	t.Helper()
	prev := DockWindowList
	t.Cleanup(func() { DockWindowList = prev })
}

// TestDockWindowListDefaultsOff is the migration half of the tri-state: a
// config file written before the key existed has no line for it, and must come
// up with the dock exactly as it was (minimized windows only).
func TestDockWindowListDefaultsOff(t *testing.T) {
	withDockWindowList(t)
	DockWindowList = false

	cfg := loadTOML(t, `
[appearance]
border_style = "rounded"
`)
	if cfg.Appearance.DockWindowList != nil {
		t.Fatalf("an absent key parsed as non-nil: %v", *cfg.Appearance.DockWindowList)
	}
	ApplyAppearanceConfig(cfg)
	if DockWindowList {
		t.Error("an old config file turned the window list on by saying nothing")
	}
}

// TestDockWindowListExplicitTrueSurvivesApply is the other half: turning it on
// in the settings page has to survive a reload.
func TestDockWindowListExplicitTrueSurvivesApply(t *testing.T) {
	withDockWindowList(t)
	DockWindowList = false

	cfg := loadTOML(t, `
[appearance]
dock_window_list = true
`)
	ApplyAppearanceConfig(cfg)
	if !DockWindowList {
		t.Error("an explicit true was dropped")
	}
}
