package config

import (
	"fmt"

	"github.com/pelletier/go-toml/v2"
)

// OverlayUserSections applies the per-user sections of a TOML config file on
// top of base: [appearance], [notifications] and [keybindings], key by key, so
// a file that sets only a theme leaves every other setting as base has it.
//
// Every other section ([hooks], [tape], [classroom], [daemon], [startup],
// [debug], env) is decoded and thrown away. A server that reads a config file
// from a user-controlled location (tuios-web reading a trainee's home) must not
// let that file reach what runs as the server's own account: hooks execute
// commands, classroom decides who may attach to whom.
//
// base is left untouched when data does not parse or the result does not
// validate, and the error says why.
func OverlayUserSections(base *UserConfig, data []byte) error {
	over := UserConfig{
		Appearance:    base.Appearance,
		Notifications: base.Notifications,
		Keybindings:   base.Keybindings,
	}
	if err := toml.Unmarshal(data, &over); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	candidate := *base
	candidate.Appearance = over.Appearance
	candidate.Notifications = over.Notifications
	candidate.Keybindings = over.Keybindings
	if validation := ValidateConfig(&candidate); validation.HasErrors() {
		return fmt.Errorf("config has %d error(s)", len(validation.Errors))
	}

	*base = candidate
	return nil
}
