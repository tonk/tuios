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

// GetPrefixKeybindings returns keybindings for the prefix overlay.
// isDaemonSession indicates whether we're running in daemon mode (affects detach/quit descriptions).
func GetPrefixKeybindings(prefixType string, isDaemonSession ...bool) []Keybinding {
	daemonMode := len(isDaemonSession) > 0 && isDaemonSession[0]
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
