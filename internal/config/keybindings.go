package config

// Keybinding represents a single keybinding entry
type Keybinding struct {
	Key         string
	Description string
}

// KeybindingGroup is a labeled cluster of which-key rows. The leader prefix
// overlay renders these as titled columns so related actions stay together
// without a single scrolling tower of bindings.
type KeybindingGroup struct {
	Title    string
	Bindings []Keybinding
}

// GetPrefixKeybindingGroups returns the which-key overlay content as labeled
// groups. Submenus are a single untitled group; the top-level leader prefix
// is split into windows / layout / navigate / tools / session.
//
// If registry is non-nil, keys are read live from the user's config so a
// rebound prefix action shows its actual key instead of the default; the
// literal fallback below only fires when no registry is available.
func GetPrefixKeybindingGroups(prefixType string, registry *KeybindRegistry, isDaemonSession ...bool) []KeybindingGroup {
	daemonMode := len(isDaemonSession) > 0 && isDaemonSession[0]
	if registry != nil {
		return getLivePrefixKeybindingGroups(prefixType, registry, daemonMode)
	}
	switch prefixType {
	case "workspace":
		return []KeybindingGroup{{Bindings: []Keybinding{
			{"1-9", "Switch to workspace"},
			{"Shift+1-9", "Move window to workspace"},
			{"r", "Rename workspace"},
			{"Esc", "Cancel"},
		}}}
	case "minimize":
		return []KeybindingGroup{{Bindings: []Keybinding{
			{"m", "Minimize focused window"},
			{"1-9", "Restore window"},
			{"Shift+M", "Restore all"},
			{"Esc", "Cancel"},
		}}}
	case "window":
		return []KeybindingGroup{{Bindings: []Keybinding{
			{"n", "New window"},
			{"x", "Close window"},
			{"r", "Rename window"},
			{"Tab", "Next window"},
			{"Shift+Tab", "Previous window"},
			{"t", "Toggle tiling mode"},
			{"Esc", "Cancel"},
		}}}
	case "debug":
		return []KeybindingGroup{{Bindings: []Keybinding{
			{"l", "Toggle log viewer"},
			{"c", "Toggle cache statistics"},
			{"k", "Toggle showkeys overlay"},
			{"a", "Toggle animations"},
			{"r", "Reload custom theme files"},
			{"Esc", "Cancel"},
		}}}
	case "tape":
		return []KeybindingGroup{{Bindings: []Keybinding{
			{"m", "Open tape manager"},
			{"t", "Review project tape"},
			{"r", "Start recording"},
			{"s", "Stop recording"},
			{"Esc", "Cancel"},
		}}}
	case "layout":
		return []KeybindingGroup{{Bindings: []Keybinding{
			{"l", "Load layout"},
			{"s", "Save layout"},
			{"Esc", "Cancel"},
		}}}
	default: // general prefix
		session := []Keybinding{}
		if daemonMode {
			session = append(session,
				Keybinding{"d", "Detach session"},
				Keybinding{"Esc", "Window mode"},
			)
		} else {
			session = append(session, Keybinding{"d/Esc", "Window mode"})
		}
		session = append(session, Keybinding{"X", "Close session"})
		if daemonMode {
			session = append(session, Keybinding{"q", "Quit menu"})
		} else {
			session = append(session, Keybinding{"q", "Quit application"})
		}
		session = append(session, Keybinding{"?", "Toggle help"})

		return []KeybindingGroup{
			{Title: "windows", Bindings: []Keybinding{
				{"c", "Create window"},
				{"x", "Close window"},
				{"r", "Rename window"},
				{"n", "Next window"},
				{"p", "Previous window"},
				{"z", "Toggle zoom"},
			}},
			{Title: "layout", Bindings: []Keybinding{
				{"space", "Toggle tiling"},
				{"-", "Split horizontal"},
				{"|/\\", "Split vertical"},
				{"R", "Rotate split"},
				{"=", "Equalize splits"},
			}},
			{Title: "navigate", Bindings: []Keybinding{
				{"1-9", "Switch to workspace"},
				{"w", "Workspace commands..."},
				{"m", "Minimize commands..."},
				{"t", "Window commands..."},
				{"W", "Workspace switcher"},
				{"e", "Focus/leave sidebar"},
				{"j", "Jump to newest message"},
			}},
			{Title: "tools", Bindings: []Keybinding{
				{"P", "Command palette"},
				{"S", "Session switcher"},
				{"L", "Layout commands..."},
				{"T", "Tape manager..."},
				{"D", "Debug commands..."},
				{",", "Settings"},
				{"[", "Scrollback mode"},
				{"s", "Scrollback browser"},
				{"b", "Toggle sidebar"},
				{"M", "Toggle mouse mode"},
				{"f", "Toggle focus follows mouse"},
			}},
			{Title: "session", Bindings: session},
		}
	}
}

// GetPrefixKeybindings returns a flat list of which-key rows (groups
// concatenated in order). Prefer GetPrefixKeybindingGroups when rendering.
func GetPrefixKeybindings(prefixType string, registry *KeybindRegistry, isDaemonSession ...bool) []Keybinding {
	groups := GetPrefixKeybindingGroups(prefixType, registry, isDaemonSession...)
	var bindings []Keybinding
	for _, g := range groups {
		bindings = append(bindings, g.Bindings...)
	}
	return bindings
}

// getLivePrefixKeybindingGroups builds labeled which-key groups from the
// user's configured keys. Bindings whose action has no key configured are
// omitted rather than shown as unbound; empty groups are dropped.
func getLivePrefixKeybindingGroups(prefixType string, registry *KeybindRegistry, daemonMode bool) []KeybindingGroup {
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
	group := func(title string, bindings []Keybinding) KeybindingGroup {
		return KeybindingGroup{Title: title, Bindings: bindings}
	}
	nonEmpty := func(groups []KeybindingGroup) []KeybindingGroup {
		out := make([]KeybindingGroup, 0, len(groups))
		for _, g := range groups {
			if len(g.Bindings) > 0 {
				out = append(out, g)
			}
		}
		return out
	}

	switch prefixType {
	case "workspace":
		var b []Keybinding
		b = digitRange(b, "workspace_prefix_switch", "Switch to workspace")
		b = digitRange(b, "workspace_prefix_move", "Move window to workspace")
		b = add(b, "workspace_prefix_rename", "Rename workspace")
		b = add(b, "workspace_prefix_cancel", "Cancel")
		return []KeybindingGroup{{Bindings: b}}
	case "minimize":
		var b []Keybinding
		b = add(b, "minimize_prefix_focused", "Minimize focused window")
		b = digitRange(b, "minimize_prefix_restore", "Restore window")
		b = add(b, "minimize_prefix_restore_all", "Restore all")
		b = add(b, "minimize_prefix_cancel", "Cancel")
		return []KeybindingGroup{{Bindings: b}}
	case "window":
		var b []Keybinding
		b = add(b, "window_prefix_new", "New window")
		b = add(b, "window_prefix_close", "Close window")
		b = add(b, "window_prefix_rename", "Rename window")
		b = add(b, "window_prefix_next", "Next window")
		b = add(b, "window_prefix_prev", "Previous window")
		b = add(b, "window_prefix_tiling", "Toggle tiling mode")
		b = add(b, "window_prefix_cancel", "Cancel")
		return []KeybindingGroup{{Bindings: b}}
	case "debug":
		var b []Keybinding
		b = add(b, "debug_prefix_logs", "Toggle log viewer")
		b = add(b, "debug_prefix_cache", "Toggle cache statistics")
		b = add(b, "debug_prefix_showkeys", "Toggle showkeys overlay")
		b = add(b, "debug_prefix_animations", "Toggle animations")
		b = add(b, "debug_prefix_reload_theme", "Reload custom theme files")
		b = add(b, "debug_prefix_cancel", "Cancel")
		return []KeybindingGroup{{Bindings: b}}
	case "tape":
		var b []Keybinding
		b = add(b, "tape_prefix_manager", "Open tape manager")
		b = add(b, "tape_prefix_review", "Review project tape")
		b = add(b, "tape_prefix_record", "Start recording")
		b = add(b, "tape_prefix_stop", "Stop recording")
		b = add(b, "tape_prefix_cancel", "Cancel")
		return []KeybindingGroup{{Bindings: b}}
	case "layout":
		// handleTerminalLayoutPrefix (internal/input/keyboard_terminal.go)
		// still matches "l"/"s" literally rather than through the registry,
		// so there is no live key to read yet for this submenu.
		return []KeybindingGroup{{Bindings: []Keybinding{
			{"l", "Load layout"},
			{"s", "Save layout"},
			{"Esc", "Cancel"},
		}}}
	default: // general prefix
		var windows, layout, navigate, tools, session []Keybinding

		windows = add(windows, "prefix_new_window", "Create window")
		windows = add(windows, "prefix_close_window", "Close window")
		windows = add(windows, "prefix_rename_window", "Rename window")
		windows = add(windows, "prefix_next_window", "Next window")
		windows = add(windows, "prefix_prev_window", "Previous window")
		windows = add(windows, "prefix_fullscreen", "Toggle zoom")

		layout = add(layout, "prefix_toggle_tiling", "Toggle tiling")
		layout = add(layout, "prefix_split_horizontal", "Split horizontal")
		layout = add(layout, "prefix_split_vertical", "Split vertical")
		layout = add(layout, "prefix_rotate_split", "Rotate split")
		layout = add(layout, "prefix_equalize_splits", "Equalize splits")

		navigate = digitRange(navigate, "switch_workspace", "Switch to workspace")
		navigate = add(navigate, "prefix_workspace", "Workspace commands...")
		navigate = add(navigate, "prefix_minimize", "Minimize commands...")
		navigate = add(navigate, "prefix_window", "Window commands...")
		navigate = add(navigate, "prefix_workspace_switcher", "Workspace switcher")
		navigate = add(navigate, "prefix_explore", "Focus/leave sidebar")
		navigate = add(navigate, "prefix_jump_notif", "Jump to newest message")

		tools = add(tools, "prefix_command_palette", "Command palette")
		tools = add(tools, "prefix_session_switcher", "Session switcher")
		tools = add(tools, "prefix_layout", "Layout commands...")
		tools = add(tools, "prefix_tape", "Tape manager...")
		tools = add(tools, "prefix_debug", "Debug commands...")
		tools = add(tools, "prefix_settings", "Settings")
		tools = add(tools, "prefix_selection", "Scrollback mode")
		tools = add(tools, "prefix_scrollback", "Scrollback browser")
		tools = add(tools, "prefix_toggle_sidebar", "Toggle sidebar")
		tools = add(tools, "prefix_toggle_mouse", "Toggle mouse mode")
		tools = add(tools, "prefix_toggle_focus_follows_mouse", "Toggle focus follows mouse")

		if daemonMode {
			session = add(session, "prefix_detach", "Detach session")
			session = add(session, "prefix_exit_mode", "Window mode")
		} else {
			d := key("prefix_detach")
			esc := key("prefix_exit_mode")
			switch {
			case d != "" && esc != "":
				session = append(session, Keybinding{d + "/" + esc, "Window mode"})
			case d != "":
				session = append(session, Keybinding{d, "Window mode"})
			case esc != "":
				session = append(session, Keybinding{esc, "Window mode"})
			}
		}
		session = add(session, "prefix_close_session", "Close session")
		if daemonMode {
			session = add(session, "prefix_quit", "Quit menu")
		} else {
			session = add(session, "prefix_quit", "Quit application")
		}
		session = add(session, "prefix_help", "Toggle help")

		return nonEmpty([]KeybindingGroup{
			group("windows", windows),
			group("layout", layout),
			group("navigate", navigate),
			group("tools", tools),
			group("session", session),
		})
	}
}
