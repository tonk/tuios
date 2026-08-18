package config

import "testing"

// withCursorBlink restores the global an apply pass writes.
func withCursorBlink(t *testing.T) {
	t.Helper()
	prev := CursorBlink
	t.Cleanup(func() { CursorBlink = prev })
}

// TestCursorBlinkDefaultsOn is the migration half of the tri-state: a config
// file written before the key existed has no line for it, and must come up
// with a blinking cursor rather than reading the absence as off.
func TestCursorBlinkDefaultsOn(t *testing.T) {
	withCursorBlink(t)
	CursorBlink = true

	cfg := loadTOML(t, `
[appearance]
border_style = "rounded"
`)
	if cfg.Appearance.CursorBlink != nil {
		t.Fatalf("an absent key parsed as non-nil: %v", *cfg.Appearance.CursorBlink)
	}
	ApplyAppearanceConfig(cfg)
	if !CursorBlink {
		t.Error("an old config file turned cursor blink off by saying nothing")
	}
}

// TestCursorBlinkExplicitFalseSurvivesApply is the other half: turning it off
// in the settings page has to survive a reload.
func TestCursorBlinkExplicitFalseSurvivesApply(t *testing.T) {
	withCursorBlink(t)
	CursorBlink = true

	cfg := loadTOML(t, `
[appearance]
cursor_blink = false
`)
	ApplyAppearanceConfig(cfg)
	if CursorBlink {
		t.Error("an explicit false was dropped")
	}
}
