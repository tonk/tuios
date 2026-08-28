package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Gaurav-Gosain/tuios/internal/hooks"
)

// exampleField documents one scalar TOML key for GenerateExampleConfig.
// Example is the literal TOML right-hand side, already quoted where needed.
type exampleField struct {
	Key     string
	Example string
	Desc    string
}

// exampleTable is one [table] or [table.sub] block in the generated file.
type exampleTable struct {
	Path   string
	Fields []exampleField
}

// exampleTables documents every scalar option in UserConfig, grouped by TOML
// table path. example_test.go walks UserConfig with reflection and fails if a
// field has no entry here, so a field added to the struct without a matching
// entry breaks the build instead of silently missing from the generated file.
//
// Map-typed fields (the keybinding sections and [hooks]) are not scalars and
// are documented separately, from keybindSectionDocs and hookEventDescriptions
// below - each with its own coverage test.
var exampleTables = []exampleTable{
	{
		Path: "appearance",
		Fields: []exampleField{
			{"border_style", `"rounded"`, `Border style: rounded, normal, thick, double, hidden, block, ascii, outer-half-block, inner-half-block. "hidden" also hides the window buttons.`},
			{"hide_window_buttons", "false", "Hide window control buttons (minimize, maximize, close)."},
			{"hide_scrollbar", "false", "Hide the scrollbar thumb on the pane border."},
			{"scrollback_lines", "10000", "Lines of scrollback kept per window. Range 100-10000000."},
			{"scroll_lines", "3", "Lines scrolled per mouse-wheel notch. Range 1-50."},
			{"copy_on_select", "true", "Copy a mouse selection to the clipboard as soon as it's released."},
			{"focus_follows_mouse", "false", "Focus the pane under the cursor as the mouse moves."},
			{"focus_follows_mouse_in_terminal", "false", "Also hover-focus while in terminal mode."},
			{"alt_drag", "true", "Alt + left-drag moves a pane, like most desktop window managers."},
			{"click_to_type", `"single"`, "What a click on a pane's content does in window-management mode: single, double, off."},
			{"word_characters", `"@-./_~?&=%+#"`, "Punctuation counted as part of a word for double-click selection."},
			{"dockbar_position", `"bottom"`, "Dockbar position: bottom, top, hidden."},
			{"preferred_shell", `""`, "Shell to launch new windows with. Empty auto-detects based on platform."},
			{"animations_enabled", "true", "Enable UI animations. Set false for instant transitions."},
			{"mouse_enabled", "true", "Let tuios handle the mouse (hover, click, drag, scroll, selection). Set false to hand it to the terminal emulator."},
			{"confirm_quit", "false", "Always show the quit confirmation dialog, even with nothing running."},
			{"whichkey_enabled", "true", "Show the which-key popup after pressing the leader key and pausing."},
			{"whichkey_position", `"bottom-right"`, "Which-key popup position: bottom-right, bottom-left, top-right, top-left, center."},
			{"window_title_position", `"bottom"`, "Window title position: bottom, top, hidden."},
			{"hide_clock", "false", "Deprecated, use show_clock instead."},
			{"show_clock", "false", "Show the clock overlay."},
			{"clock_format", `"15:04"`, `Go reference-time layout for the clock overlay, e.g. "15:04" (HH:MM) or "15:04:05" (HH:MM:SS). May also contain "{week}" for the zero-padded ISO week number, e.g. "2006-01-02 15:04 W{week}".`},
			{"clock_position", `"left"`, "Where the clock badge sits along its row: left, center, right."},
			{"clock_pill", "false", "Draw the clock with rounded pill caps like the dock's pills."},
			{"clock_fg_color", `""`, `Hex color for the clock badge text, e.g. "#89b4fa". Empty uses the theme's color.`},
			{"clock_bg_color", `""`, `Hex color for the clock badge background, e.g. "#1e1e2e". Empty uses the theme's color.`},
			{"show_cpu", "false", "Show a CPU graph in the dock."},
			{"show_ram", "false", "Show RAM usage in the dock."},
			{"show_mouse_indicator", "false", "Show a Mouse:ON/OFF readout in the dock."},
			{"show_tiling_indicator", "false", "Show a Tile:ON/OFF readout in the dock."},
			{"show_focus_follows_mouse_indicator", "false", "Show a FFM:ON/OFF readout in the dock."},
			{"theme", `""`, `Color theme name (e.g. "dracula", "nord", or a custom theme in ~/.config/tuios/themes/). Empty uses the terminal's own colors.`},
			{"shared_borders", "false", "Share borders between adjacent tiled windows."},
			{"maximize_new_windows", "false", "A new floating window fills the content area instead of spawning at half size. No effect while auto-tiling is on."},
			{"border_focused_color", `""`, `Hex color for the focused pane border, e.g. "#89b4fa". Empty uses the theme's color.`},
			{"border_unfocused_color", `""`, "Hex color for unfocused pane borders. Empty uses the theme's color."},
			{"window_title_format", `""`, "Template overriding how a window's title is built: {title}, {index}, {cwd}. Empty shows the title as-is."},
			{"show_window_number", "true", `Prefix a window's title with its 1-based index, e.g. "1: bash". Ignored once window_title_format is set.`},
			{"zoom_max_width", "0", "Max width in cells for zoom mode. 0 means fullscreen."},
			{"niri_reverse_scroll", "false", "Reverse the mouse-wheel direction when scrolling the niri-style scrolling layout."},
			{"max_fps", "60", "Maximum render FPS. Range 10-240."},
			{"dock_workspace_tabs", "true", "Show a clickable workspace strip in the dock."},
			{"dock_workspace_tooltip", "true", "Pop a truncated workspace name in full on hover."},
			{"dock_pill_caps", "false", "Draw powerline caps on the dock's pills instead of flat ends."},
			{"dock_pill_underline", "true", "Underline the active workspace/window pill's label. The one mark that survives ASCII mode and monochrome; turn off if the pill's own fill already makes it obvious."},
			{"session_colors", "true", "Give each session its own color on the rail and the session switcher."},
			{"set_terminal_title", "true", `Set the host terminal's window title to "tuios" once at startup (OSC 2).`},
			{"dock_window_list", "false", "List every window of the current workspace in the dock, not just minimized ones. A window wanting attention (agent needs input/errored/unseen-done, or new output/a bell/a notification while unfocused) blinks until focused."},
			{"cursor_blink", "true", "Blink the focused pane's cursor. A guest app can still override it with a cursor-style sequence (DECSCUSR)."},
		},
	},
	{
		Path: "appearance.scrollbar",
		Fields: []exampleField{
			{"style", `"thin"`, "Scrollbar style: thin, track."},
			{"thumb", `""`, "One-cell glyph override for the thumb. Empty uses the style's own glyph."},
			{"track", `""`, `One-cell glyph override for the track, or "none". Empty uses the style's own glyph.`},
			{"tint", `"border"`, "Scrollbar color: border, muted, or a #RRGGBB literal."},
		},
	},
	{
		Path: "appearance.sidebar",
		Fields: []exampleField{
			{"enabled", "false", "Show the session sidebar rail."},
			{"position", `"left"`, "Rail edge: left, right, hidden."},
			{"width", "28", "Preferred rail width in columns on a wide screen."},
			{"show_windows", "true", "Show the terminals section."},
			{"show_glyphs", "true", "Show the agent-state glyph on each row."},
			{"show_counts", "true", "Show the window count on each session row."},
			{"show_agents", "true", "Show the agents section at the rail's bottom."},
			{"marquee", "true", "Scroll a hovered row's overflowing title."},
			{"tooltips", "true", "Label the collapsed strip on hover."},
			{"workspaces", `""`, "Deprecated, no longer used; kept so old config files still parse."},
		},
	},
	{
		Path: "notifications",
		Fields: []exampleField{
			{"duration", "6", "Seconds an info or success message stays up. A floor, not a cap; under 4 produces a config warning."},
			{"warning_duration", "8", "Seconds a warning stays up."},
			{"error_duration", "15", "Seconds an error stays up, used only when error_sticky = false."},
			{"error_sticky", "true", "Errors wait for esc instead of expiring on a timer."},
		},
	},
	{
		Path: "notifications.agent",
		Fields: []exampleField{
			{"enabled", "true", "Master switch. False silences every sink below, including the command."},
			{"notify", "true", "Write an in-band notification to the terminal you're attached to."},
			{"sound", "false", "Make the alert audible."},
			{"sound_mode", `"audio"`, `How sound is made: "audio" plays a cue, "bell" writes a BEL, "both" does each.`},
			{"sound_cooldown_seconds", "3", "Shortest gap between two audible cues, counted across every pane."},
			{"dock", "true", "Show the alert in tuios's own dock; click it to jump to the pane."},
			{"command", `""`, "Shell command to run on an alert. Shorthand for the after-agent-state hook; see docs/HOOKS.md."},
			{"settle_seconds", "2", "Hold an alert this long and drop it if the pane leaves the state before it expires."},
			{"suppress_focused", "true", "Say nothing about the pane you're already looking at."},
			{"quiet_hours", `""`, `Silence every sink inside a local-time window, e.g. "22:00-08:00". Empty means never quiet.`},
		},
	},
	{
		Path: "notifications.agent.states",
		Fields: []exampleField{
			{"needs_input", "true", "Alert when an agent is blocked on you."},
			{"errored", "true", "Alert when an agent stops on an error."},
			{"done", "true", "Alert when an agent finishes its task."},
			{"idle", "false", "Alert when an agent goes quiet. Guessed from silence, so it's the flappy one."},
			{"working", "false", "Alert when an agent starts working. Not usually news."},
		},
	},
	{
		Path: "notifications.agent.sounds",
		Fields: []exampleField{
			{"done", `""`, "Path to a custom cue for the \"agent stopped\" sound. Falls back to the built-in cue if the path doesn't exist."},
			{"needs_input", `""`, "Path to a custom cue for the \"agent wants you\" sound. Falls back to the built-in cue if the path doesn't exist."},
		},
	},
	{
		Path: "daemon",
		Fields: []exampleField{
			{"log_level", `"off"`, "Debug log level: off, errors, basic, messages, verbose, trace."},
			{"default_codec", `"gob"`, "Protocol codec between client and daemon: gob, json."},
			{"socket_path", `""`, "Custom daemon socket path. Empty uses $XDG_RUNTIME_DIR/tuios/daemon.sock."},
			{"agent_autodetect", "true", "Auto-detect a pane's foreground AI-agent CLI (claude, codex, aider, ...) to set its agent-state glyph."},
			{"agent_detect_seconds", "2", "How often the auto-detector polls each pane, in seconds. Negative disables detection."},
			{"agent_binaries", "[]", "Extra binary names to treat as agents, merged with the built-in list."},
			{"exit_when_empty", "false", "Shut the daemon down once its last session is killed (tmux's exit-empty). Detaching, or reaching zero windows, never counts - only killing the session does."},
		},
	},
	{
		Path: "startup",
		Fields: []exampleField{
			{"open_default_window", "false", "Open one terminal window automatically when a session starts with none."},
			{"tiled", "false", "Start a new session with tiling enabled instead of floating."},
			{"start_in_terminal_mode", "false", "Start focused in terminal mode, when a window is present, so typing goes straight to the shell."},
		},
	},
	{
		Path: "tape",
		Fields: []exampleField{
			{"autorun", `"ask"`, "Project tape (.tuios.tape) detection: off, ask, auto. See docs/PROJECT_TAPES.md."},
			{"auto_review", "false", "Auto-open the tape review/trust dialog on detection, instead of only the passive banner."},
		},
	},
	{
		Path: "debug",
		Fields: []exampleField{
			{"show_key_events", "false", "Show the on-screen showkeys overlay: the last several keypresses as pills, bottom-right."},
		},
	},
	{
		Path: "keybindings",
		Fields: []exampleField{
			{"leader_key", `"ctrl+b"`, "Prefix key for leader-based (tmux-style) commands."},
		},
	},
}

// keybindSectionDoc pairs one [keybindings.*] table with a short description
// and the accessor for its default bindings.
type keybindSectionDoc struct {
	Key   string
	Title string
	Get   func(*UserConfig) map[string][]string
}

// keybindSectionDocs lists every keybinding section in the order they read
// best in the generated file. example_test.go reflects over KeybindingsConfig
// and fails if a map-typed field's toml tag is missing here.
var keybindSectionDocs = []keybindSectionDoc{
	{"window_management", "Window creation, navigation, and control (active in window-management mode).", func(c *UserConfig) map[string][]string { return c.Keybindings.WindowManagement }},
	{"workspaces", "Moving a window to another workspace and following it there.", func(c *UserConfig) map[string][]string { return c.Keybindings.Workspaces }},
	{"layout", "Window positioning, snapping, and BSP tiling.", func(c *UserConfig) map[string][]string { return c.Keybindings.Layout }},
	{"mode_control", "Switching between window-management and terminal mode, help, and quit.", func(c *UserConfig) map[string][]string { return c.Keybindings.ModeControl }},
	{"system", "System-level controls. Currently empty; debug commands live under debug_prefix.", func(c *UserConfig) map[string][]string { return c.Keybindings.System }},
	{"navigation", "Arrow-key focus navigation between panes.", func(c *UserConfig) map[string][]string { return c.Keybindings.Navigation }},
	{"restore_minimized", "Restoring a specific minimized window by number (Shift+1..9).", func(c *UserConfig) map[string][]string { return c.Keybindings.RestoreMinimized }},
	{"prefix_mode", "Tmux-style commands reached with the leader key (leader_key above, then one of these).", func(c *UserConfig) map[string][]string { return c.Keybindings.PrefixMode }},
	{"window_prefix", `Window sub-menu, reached with the leader key then "t".`, func(c *UserConfig) map[string][]string { return c.Keybindings.WindowPrefix }},
	{"minimize_prefix", `Minimize sub-menu, reached with the leader key then "m".`, func(c *UserConfig) map[string][]string { return c.Keybindings.MinimizePrefix }},
	{"workspace_prefix", `Workspace sub-menu, reached with the leader key then "w".`, func(c *UserConfig) map[string][]string { return c.Keybindings.WorkspacePrefix }},
	{"debug_prefix", `Debug and development tools sub-menu, reached with the leader key then "D".`, func(c *UserConfig) map[string][]string { return c.Keybindings.DebugPrefix }},
	{"tape_prefix", `Project tape sub-menu, reached with the leader key then "T".`, func(c *UserConfig) map[string][]string { return c.Keybindings.TapePrefix }},
	{"terminal_mode", "Direct keybinds that work in terminal mode without pressing the leader key first.", func(c *UserConfig) map[string][]string { return c.Keybindings.TerminalMode }},
	{"sidebar", "Keys active only while the session sidebar rail has keyboard focus.", func(c *UserConfig) map[string][]string { return c.Keybindings.Sidebar }},
}

// hookEventDescriptions documents every hook event for the generated [hooks]
// block. example_test.go fails if hooks.AllEvents() gains an event missing
// here.
var hookEventDescriptions = map[hooks.Event]string{
	hooks.AfterNewWindow:       "A window has been created.",
	hooks.AfterCloseWindow:     "A window has been closed.",
	hooks.AfterFocusChange:     "Focus moved to a different window.",
	hooks.AfterWorkspaceSwitch: "The visible workspace changed.",
	hooks.AfterAttach:          "This client attached to a session.",
	hooks.AfterDetach:          "This client detached from a session that keeps running.",
	hooks.AfterLayoutChange:    "The layout changed, including tiling turning on or off.",
	hooks.AfterResize:          "A window settled at a new size.",
	hooks.AfterAgentState:      "A pane's agent state changed to one [notifications.agent] alerts on.",
}

const exampleHeader = `# TUIOS annotated reference configuration.
#
# Generated by ` + "`tuios config example`" + ` from the running binary's own defaults, so
# it always matches this version of tuios - unlike a hand-written example,
# it cannot describe an option that no longer exists or omit a new one.
#
# Every setting below is commented out and shown at its built-in default.
# Uncomment and edit only what you want to change; anything left commented
# keeps behaving exactly as it does with no config file at all.
#
# This file is a reference, not your config: it is never read by tuios.
# Copy the lines you want into your real config with ` + "`tuios config edit`" + `, or
# find its path with ` + "`tuios config path`" + `.
#
# Full prose docs: docs/CONFIGURATION.md, docs/KEYBINDINGS.md, docs/HOOKS.md
`

// GenerateExampleConfig renders the fully commented reference config: every
// option in UserConfig, at its default value, commented out, with a
// one-line description. It reads only the compiled-in defaults (DefaultConfig
// and the description tables above), never a file on disk.
func GenerateExampleConfig() string {
	var b strings.Builder
	b.WriteString(exampleHeader)

	def := DefaultConfig()

	for _, tbl := range exampleTables {
		if tbl.Path == "keybindings" {
			continue // rendered below, interleaved with its subsections
		}
		writeExampleTable(&b, tbl)
	}

	b.WriteString("\n[keybindings]\n")
	for _, f := range findExampleTable("keybindings").Fields {
		writeExampleField(&b, f)
	}

	for _, sec := range keybindSectionDocs {
		fmt.Fprintf(&b, "\n[keybindings.%s]\n", sec.Key)
		writeWrappedComment(&b, sec.Title)

		actions := sec.Get(def)
		if len(actions) == 0 {
			b.WriteString("# (no default bindings in this section)\n")
			continue
		}
		names := make([]string, 0, len(actions))
		for name := range actions {
			names = append(names, name)
		}
		sort.Strings(names)
		assignments := make([]string, len(names))
		comments := make([]string, len(names))
		for i, name := range names {
			assignments[i] = fmt.Sprintf("%s = %s", name, tomlStringArray(actions[name]))
			comments[i] = describeAction(name)
		}
		writeAlignedComments(&b, assignments, comments)
	}

	b.WriteString("\n[env]\n")
	writeWrappedComment(&b, "Extra environment variables exported into every shell tuios spawns, "+
		"local windows and daemon-backed sessions alike, on top of what tuios "+
		"itself already inherited.")
	b.WriteString("#\n")
	b.WriteString("# EDITOR = \"nvim\"\n")
	b.WriteString("# MY_VAR = \"some value\"\n")

	b.WriteString("\n[hooks]\n")
	writeWrappedComment(&b, "Run a shell command on a session event. A value can be a single command "+
		"string or an array of commands run in order. Every command receives the "+
		"parent environment plus event-specific TUIOS_* variables. See docs/HOOKS.md.")
	b.WriteString("#\n")
	events := hooks.AllEvents()
	assignments := make([]string, len(events))
	comments := make([]string, len(events))
	for i, event := range events {
		assignments[i] = fmt.Sprintf(`%s = ""`, event)
		comments[i] = hookEventDescriptions[event]
	}
	writeAlignedComments(&b, assignments, comments)

	return b.String()
}

func findExampleTable(path string) exampleTable {
	for _, tbl := range exampleTables {
		if tbl.Path == path {
			return tbl
		}
	}
	return exampleTable{}
}

func writeExampleTable(b *strings.Builder, tbl exampleTable) {
	fmt.Fprintf(b, "\n[%s]\n", tbl.Path)
	for _, f := range tbl.Fields {
		writeExampleField(b, f)
	}
}

func writeExampleField(b *strings.Builder, f exampleField) {
	writeWrappedComment(b, f.Desc)
	fmt.Fprintf(b, "# %s = %s\n", f.Key, f.Example)
}

// writeWrappedComment writes text as one or more "# "-prefixed lines, each no
// wider than 78 columns.
func writeWrappedComment(b *strings.Builder, text string) {
	const width = 78
	words := strings.Fields(text)
	line := "#"
	for _, w := range words {
		if line != "#" && len(line)+1+len(w) > width {
			b.WriteString(line)
			b.WriteString("\n")
			line = "#"
		}
		line += " " + w
	}
	if line != "#" {
		b.WriteString(line)
		b.WriteString("\n")
	}
}

// writeAlignedComments writes one "# assignment  # comment" line per pair,
// with every "# comment" column starting at the same position - the widest
// assignment in the batch plus one gap - so a section of keybindings or hooks
// reads as a table instead of a ragged list. A pair with an empty comment
// still gets the padding trimmed rather than left trailing.
func writeAlignedComments(b *strings.Builder, assignments, comments []string) {
	width := 0
	for _, a := range assignments {
		if len(a) > width {
			width = len(a)
		}
	}
	for i, a := range assignments {
		if comments[i] == "" {
			fmt.Fprintf(b, "# %s\n", a)
			continue
		}
		fmt.Fprintf(b, "# %-*s  # %s\n", width, a, comments[i])
	}
}

// tomlStringArray renders a []string as a TOML array literal of quoted
// strings, e.g. []string{"n", "alt+1"} -> `["n", "alt+1"]`.
func tomlStringArray(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// describeAction returns a human-readable description for a keybinding
// action, from the same ActionDescriptions map the help overlay uses, falling
// back to a humanized version of the action name for the handful of
// rail-scoped actions (sidebar section) that map doesn't cover.
func describeAction(action string) string {
	if desc, ok := ActionDescriptions[action]; ok {
		return desc
	}
	words := strings.Split(action, "_")
	if len(words) > 0 && words[0] != "" {
		words[0] = strings.ToUpper(words[0][:1]) + words[0][1:]
	}
	return strings.Join(words, " ")
}
