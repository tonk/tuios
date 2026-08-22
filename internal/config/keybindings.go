package config

import "fmt"

// Keybinding represents a single keybinding entry
type Keybinding struct {
	Key         string
	Description string
}

// KeybindingSection represents a section of related keybindings
type KeybindingSection struct {
	Title     string
	Condition string // Empty for always shown, "tiling" for tiling mode, "!tiling" for non-tiling
	Bindings  []Keybinding
}

// GetPrefixKeybindings returns keybindings for the prefix overlay (the
// which-key hint shown while a prefix chord is in progress).
// isDaemonSession indicates whether we're running in daemon mode (affects detach/quit descriptions).
//
// If registry is non-nil, keys are read live from the user's config so a
// rebound prefix action shows its actual key instead of the default; the
// literal fallback below only fires when no registry is available (e.g.
// early startup or a test that doesn't wire one up).
func GetPrefixKeybindings(prefixType string, registry *KeybindRegistry, isDaemonSession ...bool) []Keybinding {
	daemonMode := len(isDaemonSession) > 0 && isDaemonSession[0]
	if registry != nil {
		return getLivePrefixKeybindings(prefixType, registry, daemonMode)
	}
	switch prefixType {
	case "workspace":
		return []Keybinding{
			{"1-9", "Switch to workspace"},
			{"Shift+1-9", "Move window to workspace"},
			{"r", "Rename workspace"},
			{"Esc", "Cancel"},
		}
	case "minimize":
		return []Keybinding{
			{"m", "Minimize focused window"},
			{"1-9", "Restore window"},
			{"Shift+M", "Restore all"},
			{"Esc", "Cancel"},
		}
	case "window":
		return []Keybinding{
			{"n", "New window"},
			{"x", "Close window"},
			{"r", "Rename window"},
			{"Tab", "Next window"},
			{"Shift+Tab", "Previous window"},
			{"t", "Toggle tiling mode"},
			{"Esc", "Cancel"},
		}
	case "debug":
		return []Keybinding{
			{"l", "Toggle log viewer"},
			{"c", "Toggle cache statistics"},
			{"k", "Toggle showkeys overlay"},
			{"a", "Toggle animations"},
			{"r", "Reload custom theme files"},
			{"Esc", "Cancel"},
		}
	case "tape":
		return []Keybinding{
			{"m", "Open tape manager"},
			{"t", "Review project tape"},
			{"r", "Start recording"},
			{"s", "Stop recording"},
			{"Esc", "Cancel"},
		}
	case "layout":
		return []Keybinding{
			{"l", "Load layout"},
			{"s", "Save layout"},
			{"Esc", "Cancel"},
		}
	default: // general prefix
		bindings := []Keybinding{
			{"c", "Create window"},
			{"x", "Close window"},
			{"r", "Rename window"},
			{",", "Settings"},
			{"n", "Next window"},
			{"p", "Previous window"},
			{"1-9", "Switch to workspace"},
			{"z", "Toggle zoom"},
			{"space", "Toggle tiling"},
			{"-", "Split horizontal"},
			{"|/\\", "Split vertical"},
			{"R", "Rotate split"},
			{"=", "Equalize splits"},
			{"w", "Workspace commands..."},
			{"m", "Minimize commands..."},
			{"t", "Window commands..."},
			{"D", "Debug commands..."},
			{"T", "Tape manager..."},
			{"P", "Command palette"},
			{"S", "Session switcher"},
			{"W", "Workspace switcher"},
			{"L", "Layout commands..."},
			{"b", "Toggle sidebar"},
			{"M", "Toggle mouse mode"},
			{"e", "Focus/leave sidebar"},
			{"j", "Jump to newest message"},
			{"X", "Close session"},
		}

		// In daemon mode, d and Esc have different behaviors
		if daemonMode {
			bindings = append(bindings,
				Keybinding{"d", "Detach session"},
				Keybinding{"Esc", "Window mode"},
			)
		} else {
			// In local mode, both d and Esc do the same thing
			bindings = append(bindings,
				Keybinding{"d/Esc", "Window mode"},
			)
		}

		bindings = append(bindings,
			Keybinding{"[", "Scrollback mode"},
			Keybinding{"s", "Scrollback browser"},
			Keybinding{"?", "Toggle help"},
		)

		// Quit description differs based on mode
		if daemonMode {
			bindings = append(bindings, Keybinding{"q", "Quit menu"})
		} else {
			bindings = append(bindings, Keybinding{"q", "Quit application"})
		}

		return bindings
	}
}

// getLivePrefixKeybindings builds the prefix overlay rows from the user's
// actual configured keys, mirroring the labels of the hard-coded fallback
// above but never their key strings. Bindings whose action has no key
// configured (rebound to nothing) are omitted rather than shown as unbound.
func getLivePrefixKeybindings(prefixType string, registry *KeybindRegistry, daemonMode bool) []Keybinding {
	key := func(action string) string {
		return registry.GetKeysForDisplay(action)
	}
	add := func(bindings []Keybinding, action, desc string) []Keybinding {
		if k := key(action); k != "" {
			bindings = append(bindings, Keybinding{k, desc})
		}
		return bindings
	}
	// digitRange collapses a family of nine actions named actionPrefix+"_1"
	// through actionPrefix+"_9" into a single "first-last" row, the same
	// shape the hard-coded "1-9" rows had, but reflecting whatever keys those
	// nine actions actually carry.
	digitRange := func(bindings []Keybinding, actionPrefix, desc string) []Keybinding {
		first := key(actionPrefix + "_1")
		last := key(actionPrefix + "_9")
		switch {
		case first == "" && last == "":
			return bindings
		case last == "" || last == first:
			return append(bindings, Keybinding{first, desc})
		default:
			return append(bindings, Keybinding{first + "-" + last, desc})
		}
	}

	var bindings []Keybinding
	switch prefixType {
	case "workspace":
		bindings = digitRange(bindings, "workspace_prefix_switch", "Switch to workspace")
		bindings = digitRange(bindings, "workspace_prefix_move", "Move window to workspace")
		bindings = add(bindings, "workspace_prefix_rename", "Rename workspace")
		bindings = add(bindings, "workspace_prefix_cancel", "Cancel")
	case "minimize":
		bindings = add(bindings, "minimize_prefix_focused", "Minimize focused window")
		bindings = digitRange(bindings, "minimize_prefix_restore", "Restore window")
		bindings = add(bindings, "minimize_prefix_restore_all", "Restore all")
		bindings = add(bindings, "minimize_prefix_cancel", "Cancel")
	case "window":
		bindings = add(bindings, "window_prefix_new", "New window")
		bindings = add(bindings, "window_prefix_close", "Close window")
		bindings = add(bindings, "window_prefix_rename", "Rename window")
		bindings = add(bindings, "window_prefix_next", "Next window")
		bindings = add(bindings, "window_prefix_prev", "Previous window")
		bindings = add(bindings, "window_prefix_tiling", "Toggle tiling mode")
		bindings = add(bindings, "window_prefix_cancel", "Cancel")
	case "debug":
		bindings = add(bindings, "debug_prefix_logs", "Toggle log viewer")
		bindings = add(bindings, "debug_prefix_cache", "Toggle cache statistics")
		bindings = add(bindings, "debug_prefix_showkeys", "Toggle showkeys overlay")
		bindings = add(bindings, "debug_prefix_animations", "Toggle animations")
		bindings = add(bindings, "debug_prefix_reload_theme", "Reload custom theme files")
		bindings = add(bindings, "debug_prefix_cancel", "Cancel")
	case "tape":
		bindings = add(bindings, "tape_prefix_manager", "Open tape manager")
		bindings = add(bindings, "tape_prefix_review", "Review project tape")
		bindings = add(bindings, "tape_prefix_record", "Start recording")
		bindings = add(bindings, "tape_prefix_stop", "Stop recording")
		bindings = add(bindings, "tape_prefix_cancel", "Cancel")
	case "layout":
		// handleTerminalLayoutPrefix (internal/input/keyboard_terminal.go)
		// still matches "l"/"s" literally rather than through the registry,
		// so there is no live key to read yet for this submenu.
		bindings = []Keybinding{
			{"l", "Load layout"},
			{"s", "Save layout"},
			{"Esc", "Cancel"},
		}
	default: // general prefix
		bindings = add(bindings, "prefix_new_window", "Create window")
		bindings = add(bindings, "prefix_close_window", "Close window")
		bindings = add(bindings, "prefix_rename_window", "Rename window")
		bindings = add(bindings, "prefix_settings", "Settings")
		bindings = add(bindings, "prefix_next_window", "Next window")
		bindings = add(bindings, "prefix_prev_window", "Previous window")
		bindings = digitRange(bindings, "switch_workspace", "Switch to workspace")
		bindings = add(bindings, "prefix_fullscreen", "Toggle zoom")
		bindings = add(bindings, "prefix_toggle_tiling", "Toggle tiling")
		bindings = add(bindings, "prefix_split_horizontal", "Split horizontal")
		bindings = add(bindings, "prefix_split_vertical", "Split vertical")
		bindings = add(bindings, "prefix_rotate_split", "Rotate split")
		bindings = add(bindings, "prefix_equalize_splits", "Equalize splits")
		bindings = add(bindings, "prefix_workspace", "Workspace commands...")
		bindings = add(bindings, "prefix_minimize", "Minimize commands...")
		bindings = add(bindings, "prefix_window", "Window commands...")
		bindings = add(bindings, "prefix_debug", "Debug commands...")
		bindings = add(bindings, "prefix_tape", "Tape manager...")
		bindings = add(bindings, "prefix_command_palette", "Command palette")
		bindings = add(bindings, "prefix_session_switcher", "Session switcher")
		bindings = add(bindings, "prefix_workspace_switcher", "Workspace switcher")
		bindings = add(bindings, "prefix_layout", "Layout commands...")
		bindings = add(bindings, "prefix_toggle_sidebar", "Toggle sidebar")
		bindings = add(bindings, "prefix_toggle_mouse", "Toggle mouse mode")
		bindings = add(bindings, "prefix_explore", "Focus/leave sidebar")
		bindings = add(bindings, "prefix_jump_notif", "Jump to newest message")
		bindings = add(bindings, "prefix_close_session", "Close session")

		if daemonMode {
			bindings = add(bindings, "prefix_detach", "Detach session")
			bindings = add(bindings, "prefix_exit_mode", "Window mode")
		} else {
			// In local mode, both d and Esc do the same thing. Show whichever
			// of the two is actually bound; if both are, join them.
			d := key("prefix_detach")
			esc := key("prefix_exit_mode")
			switch {
			case d != "" && esc != "":
				bindings = append(bindings, Keybinding{d + "/" + esc, "Window mode"})
			case d != "":
				bindings = append(bindings, Keybinding{d, "Window mode"})
			case esc != "":
				bindings = append(bindings, Keybinding{esc, "Window mode"})
			}
		}

		bindings = add(bindings, "prefix_selection", "Scrollback mode")
		bindings = add(bindings, "prefix_scrollback", "Scrollback browser")
		bindings = add(bindings, "prefix_help", "Toggle help")

		if daemonMode {
			bindings = add(bindings, "prefix_quit", "Quit menu")
		} else {
			bindings = add(bindings, "prefix_quit", "Quit application")
		}
	}
	return bindings
}

// GetKeybindings returns all keybinding sections for the help menu
// If registry is provided, it generates bindings dynamically from user config
// If registry is nil, it falls back to hard-coded defaults
func GetKeybindings(registry *KeybindRegistry) []KeybindingSection {
	// If no registry provided, use static defaults
	if registry == nil {
		return getDefaultKeybindings()
	}

	// Generate dynamic help from config
	sections := []KeybindingSection{}

	// Window Management section
	windowMgmt := KeybindingSection{
		Title:    "WINDOW MANAGEMENT",
		Bindings: []Keybinding{},
	}
	addBinding(&windowMgmt, registry, "new_window", "New window")
	addBinding(&windowMgmt, registry, "close_window", "Close window")
	addBinding(&windowMgmt, registry, "rename_window", "Rename window")
	addBinding(&windowMgmt, registry, "minimize_window", "Minimize window")
	addBinding(&windowMgmt, registry, "restore_all", "Restore all")
	addBinding(&windowMgmt, registry, "next_window", "Next window")
	addBinding(&windowMgmt, registry, "prev_window", "Previous window")
	addBinding(&windowMgmt, registry, "toggle_last_window", "Toggle last focused window")
	addBinding(&windowMgmt, registry, "toggle_sidebar", "Toggle sidebar")
	addBinding(&windowMgmt, registry, "toggle_mouse", "Toggle mouse mode")
	if len(windowMgmt.Bindings) > 0 {
		sections = append(sections, windowMgmt)
	}

	// Workspaces section
	workspaces := KeybindingSection{
		Title:    "WORKSPACES",
		Bindings: []Keybinding{},
	}
	// Show all workspace switches (1-9). This is a prefix chord (leader, then
	// the digit) now, so it needs the leader spelled out rather than the plain
	// key addBinding would show.
	for i := 1; i <= 9; i++ {
		actionSwitch := fmt.Sprintf("switch_workspace_%d", i)
		descSwitch := fmt.Sprintf("Switch to workspace %d", i)
		if keys := registry.GetKeys(actionSwitch); len(keys) > 0 {
			workspaces.Bindings = append(workspaces.Bindings, Keybinding{
				Key:         LeaderKey + ", " + keys[0],
				Description: descSwitch,
			})
		}
	}
	// Show all move and follow (1-9)
	for i := 1; i <= 9; i++ {
		actionMove := fmt.Sprintf("move_and_follow_%d", i)
		descMove := fmt.Sprintf("Move to workspace %d and follow", i)
		addBinding(&workspaces, registry, actionMove, descMove)
	}
	if len(workspaces.Bindings) > 0 {
		sections = append(sections, workspaces)
	}

	// Modes section
	modes := KeybindingSection{
		Title:    "MODES",
		Bindings: []Keybinding{},
	}
	addBinding(&modes, registry, "enter_terminal_mode", "Insert mode")
	addBinding(&modes, registry, "toggle_tiling", "Toggle tiling")
	addBinding(&modes, registry, "toggle_help", "Toggle help")
	if len(modes.Bindings) > 0 {
		sections = append(sections, modes)
	}

	// Return the rest as static for now (tiling, selection, etc.)
	sections = append(sections, getStaticHelpSections()...)
	return sections
}

// addBinding adds a keybinding to a section if the action has keys configured
func addBinding(section *KeybindingSection, registry *KeybindRegistry, action, description string) {
	keys := registry.GetKeysForDisplay(action)
	if keys != "" {
		section.Bindings = append(section.Bindings, Keybinding{
			Key:         keys,
			Description: description,
		})
	}
}

// getDefaultKeybindings returns the original hard-coded keybindings (used as fallback)
func getDefaultKeybindings() []KeybindingSection {
	sections := []KeybindingSection{
		{
			Title: "WINDOW MANAGEMENT",
			Bindings: []Keybinding{
				{"n", "New window"},
				{"x", "Close window"},
				{"r", "Rename window"},
				{"m", "Minimize window"},
				{"Shift+M", "Restore all"},
				{"Tab", "Next window"},
				{"Shift+Tab", "Previous window"},
				{"1-9", "Select window"},
				{"%s+1-9", "Select window (any time)"}, // %s will be replaced with modifier key
				{"%s+`", "Toggle last focused window"}, // %s will be replaced with modifier key
				{"Alt+S", "Toggle sidebar"},
				{"Alt+M", "Toggle mouse mode"},
			},
		},
		{
			Title: "WORKSPACES",
			Bindings: []Keybinding{
				{"Ctrl+B, 1-9", "Switch workspace"},
				{"%s+Shift+1-9", "Move window and follow"}, // %s will be replaced with modifier key
				{"Ctrl+B, w, 1-9", "Switch workspace (prefix submenu)"},
				{"Ctrl+B, w, Shift+1-9", "Move window (prefix submenu)"},
			},
		},
		{
			Title: "MODES",
			Bindings: []Keybinding{
				{"i, Enter", "Insert mode"},
				{"t", "Toggle tiling"},
				{"?", "Toggle help"},
			},
		},
	}
	sections = append(sections, getStaticHelpSections()...)
	return sections
}

// getStaticHelpSections returns help sections that don't need dynamic binding info
// (mouse actions, special modes, etc.)
func getStaticHelpSections() []KeybindingSection {
	return []KeybindingSection{
		{
			Title:     "TILING:",
			Condition: "tiling",
			Bindings: []Keybinding{
				{"Shift+H/L, Ctrl+←/→", "Swap left/right"},
				{"Shift+K/J, Ctrl+↑/↓", "Swap up/down"},
				{"< / >", "Resize master width"},
				{"{ / }", "Resize focused window height"},
				{"Ctrl+B, -", "Split horizontal"},
				{"Ctrl+B, |/\\", "Split vertical"},
				{"Ctrl+B, R", "Rotate split"},
			},
		},
		{
			Title:     "WINDOW SNAPPING:",
			Condition: "!tiling",
			Bindings: []Keybinding{
				{"h, l", "Snap left/right"},
				{"1-4", "Snap to corners"},
				{"f", "Fullscreen"},
				{"u", "Unsnap"},
			},
		},
		{
			Title: "SCROLLBACK:",
			Bindings: []Keybinding{
				{"Ctrl+B, [", "Enter scrollback mode"},
				{"Mouse wheel ↑", "Enter scrollback mode"},
				{"↑/↓, j/k", "Scroll up/down one line"},
				{"PgUp/PgDn", "Scroll half screen"},
				{"Ctrl+U/D", "Scroll half screen"},
				{"g, Home", "Go to oldest line"},
				{"G, End", "Go to newest line (exit)"},
				{"q, Esc", "Exit scrollback mode"},
			},
		},
		{
			Title: "WINDOW NAVIGATION:",
			Bindings: []Keybinding{
				{"Ctrl+↑/↓", "Swap/maximize windows"},
			},
		},
		{
			Title: "SYSTEM:",
			Bindings: []Keybinding{
				{"Ctrl+L", "Toggle log viewer"},
			},
		},
		{
			Title: "PREFIX (Ctrl+B) - Works in all modes:",
			Bindings: []Keybinding{
				{"c", "Create window"},
				{"x", "Close window"},
				{",/r", "Rename window"},
				{"n/Tab", "Next window"},
				{"p/Shift+Tab", "Previous window"},
				{"1-9", "Switch to workspace"},
				{"z", "Toggle zoom"},
				{"space", "Toggle tiling"},
				{"-", "Split horizontal"},
				{"|/\\", "Split vertical"},
				{"R", "Rotate split"},
				{"=", "Equalize splits"},
				{"w", "Workspace commands"},
				{"m", "Minimize commands"},
				{"t", "Window commands"},
				{"P", "Command palette"},
				{"S", "Session switcher"},
				{"W", "Workspace switcher"},
				{"L", "Load layout"},
				{"D", "Debug commands"},
				{"T", "Tape manager"},
				{"d", "Detach (daemon) / Window mode (local)"},
				{"Esc", "Window mode"},
				{"[", "Enter scrollback mode"},
				{"s", "Scrollback browser"},
				{"?", "Toggle help"},
				{"q", "Quit"},
				{"Ctrl+B", "Send literal Ctrl+B"},
			},
		},
		{
			Title: "WINDOW PREFIX (Ctrl+B, t):",
			Bindings: []Keybinding{
				{"n", "New window"},
				{"x", "Close window"},
				{"r", "Rename window"},
				{"Tab/Shift+Tab", "Next/Previous window"},
				{"t", "Toggle tiling mode"},
			},
		},
		{
			Title: "TAPE PREFIX (Ctrl+B, T):",
			Bindings: []Keybinding{
				{"m", "Open tape manager"},
				{"t", "Review project tape"},
				{"r", "Start recording"},
				{"s", "Stop recording"},
			},
		},
		{
			Title: "",
			Bindings: []Keybinding{
				{"q, Ctrl+C", "Quit"},
			},
		},
	}
}
