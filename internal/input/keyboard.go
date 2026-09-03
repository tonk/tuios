// Package input implements keyboard event handling for TUIOS.
package input

import (
	tea "charm.land/bubbletea/v2"
	"github.com/tonk/tuios/internal/app"
)

// handleNumberKey runs the select_window_N action for the given 1-9 number.
// num is the digit the registered action itself names (select_window_N ->
// N), not something re-derived from the key string: a bound key like
// "alt+1" or "ctrl+1" carries modifiers before the digit, and a macOS Option
// chord ("opt+1") normalizes to a composed unicode glyph with no digit
// character in it at all, so parsing the pressed key's own text can't recover
// which of the nine actions this is.
//
// Corner-snapping the bare digits 1-4 used to live here too, but that is
// snap_corner_N's own binding (see internal/config Layout section) and
// already wins the plain "1".."4" key by default; this only duplicated it,
// and stood in the way of select_window_N meaning the same thing on every
// key it's bound to, floating or tiled.
func handleNumberKey(_ tea.KeyPressMsg, o *app.OS, num int) (*app.OS, tea.Cmd) {
	o.FocusWindowAtWorkspacePosition(num)
	return o, nil
}

// The arrow keys belong to whatever overlay is up. Help scrolling is handled in
// HandleTerminalModeKey and HandleWindowManagementModeKey, so what is left here
// is the log viewer and the help's category strip.
func handleUpKey(_ tea.KeyPressMsg, o *app.OS) (*app.OS, tea.Cmd) {
	if o.ShowLogs {
		if o.LogScrollOffset > 0 {
			o.LogScrollOffset--
		}
		return o, nil
	}
	return o, nil
}

func handleDownKey(_ tea.KeyPressMsg, o *app.OS) (*app.OS, tea.Cmd) {
	if o.ShowLogs {
		o.LogScrollOffset++
		return o, nil
	}
	return o, nil
}

func handleLeftKey(msg tea.KeyPressMsg, o *app.OS) (*app.OS, tea.Cmd) {
	// Help menu category navigation
	if o.ShowHelp && !o.HelpSearchMode {
		if o.HelpCategory > 0 {
			o.HelpCategory--
		}
		return o, nil
	}
	return o, nil
}

func handleRightKey(msg tea.KeyPressMsg, o *app.OS) (*app.OS, tea.Cmd) {
	// Help menu category navigation
	if o.ShowHelp && !o.HelpSearchMode {
		categories := app.GetHelpCategories(o.KeybindRegistry)
		if o.HelpCategory < len(categories)-1 {
			o.HelpCategory++
		}
		return o, nil
	}
	return o, nil
}
