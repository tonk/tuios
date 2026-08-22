package theme

import (
	"fmt"
	"strings"
)

const exampleThemeHeader = `# TUIOS annotated example theme.
#
# A theme supplies the 16 ANSI colors plus foreground, background and cursor;
# TUIOS derives its own UI colors (borders, dock indicators, the cursor,
# copy-mode highlighting, notification colors, and the overlay accent) from
# that same handful of fields. An optional [ui] table further down can assign
# a color to one of those elements directly instead of accepting whatever the
# derivation above picks.
#
# Every color below is commented out and shown at the default TUIOS falls
# back to when a theme omits it (see docs/THEMES.md, "Defaults for Omitted
# Colors"). Uncomment and edit only what you want to set; anything left
# commented is filled in exactly the same way an omitted field always is.
#
# This file is a reference, not a theme: it is never read by tuios as-is.
# Give it a real id, save it under ~/.config/tuios/themes/ as a .toml file
# (the format you're reading - .json also still works, without comments),
# and select it with theme = "<id>" in config.toml.
#
# Full prose docs: docs/THEMES.md
`

// exampleColorField documents one scalar color key, the same shape
// config.exampleField uses for GenerateExampleConfig - duplicated rather than
// shared, since internal/theme has no reason to depend on internal/config for
// three struct fields.
type exampleColorField struct {
	Key     string
	Example string
	Desc    string
}

// exampleBaseColors documents every tint.Tint scalar field, in the order
// THEMES.md's own sample theme lists them, at the same fallback value
// custom.go's fillDefaults would apply to an omitted field.
var exampleBaseColors = []exampleColorField{
	{"fg", `"#e5e5e5"`, "Default foreground for terminal text."},
	{"bg", `"#000000"`, "Default background."},
	{"cursor", `"#e5e5e5"`, "Terminal cursor color. Defaults to fg if omitted."},
	{"selection_bg", `""`, "Background for selected text. No fallback of its own - most themes omit it."},

	{"black", `"#000000"`, "ANSI 0 - normal black."},
	{"red", `"#cd0000"`, "ANSI 1 - normal red."},
	{"green", `"#00cd00"`, "ANSI 2 - normal green."},
	{"yellow", `"#cdcd00"`, "ANSI 3 - normal yellow."},
	{"blue", `"#0000ee"`, "ANSI 4 - normal blue."},
	{"purple", `"#cd00cd"`, `ANSI 5 - normal purple (tuios's name for it; not "magenta").`},
	{"cyan", `"#00cdcd"`, "ANSI 6 - normal cyan."},
	{"white", `"#e5e5e5"`, "ANSI 7 - normal white."},

	{"bright_black", `"#000000"`, "ANSI 8. Defaults to black if omitted - define it, or bright text reads the same as normal text."},
	{"bright_red", `"#cd0000"`, "ANSI 9. Defaults to red if omitted."},
	{"bright_green", `"#00cd00"`, "ANSI 10. Defaults to green if omitted."},
	{"bright_yellow", `"#cdcd00"`, "ANSI 11. Defaults to yellow if omitted."},
	{"bright_blue", `"#0000ee"`, "ANSI 12. Defaults to blue if omitted."},
	{"bright_purple", `"#cd00cd"`, "ANSI 13. Defaults to purple if omitted."},
	{"bright_cyan", `"#00cdcd"`, "ANSI 14. Defaults to cyan if omitted."},
	{"bright_white", `"#e5e5e5"`, "ANSI 15. Defaults to white if omitted."},
}

// exampleUIOverrides documents every [ui] key from UIOverrides. Unlike the
// base colors above, none of these have a real default value to show - each
// is commented with what it replaces and an illustrative color, not a
// fallback tuios would ever apply on its own.
var exampleUIOverrides = []exampleColorField{
	{"border_focused_terminal", `"#a6e3a1"`, "Focused pane border in terminal mode. Instead of the derived bright_green."},
	{"border_focused_window", `"#89b4fa"`, "Focused pane border in window-management mode. Instead of the derived bright_cyan."},
	{"border_unfocused", `"#585b70"`, "Unfocused pane border. Instead of the derived red."},
	{"border_multifocused", `"#f9e2af"`, "Border for panes selected together for a broadcast action. Instead of the fixed ANSI yellow."},

	{"dock_window", `"#89b4fa"`, "Dock mode indicator, window-management mode. Instead of the derived bright_blue."},
	{"dock_terminal", `"#a6e3a1"`, "Dock mode indicator, terminal mode. Instead of the derived bright_green."},
	{"dock_copy", `"#f9e2af"`, "Dock mode indicator, copy mode. Instead of the derived yellow."},
	{"dock_highlight", `"#a6e3a1"`, "Dock highlight accent. Instead of the derived bright_green."},
	{"dock_bg", `"#11111b"`, "The dock/statusbar row's own background - tuios paints none by default, the way tmux's status-style bg does. Adds a color rather than replacing a derived one."},
	{"dock_trail_fg", `"#a6adc8"`, "The dock's trailing status text: the \"<workspace>:<windows>\" readout plus the project-tape and view-only badges. Instead of the derived FgMute."},

	{"workspace_pill_active_bg", `"#89b4fa"`, "The current-workspace tab's fill. Instead of the neutral chrome Panel step every pill otherwise rests on."},
	{"workspace_pill_active_fg", `"#11111b"`, "The current-workspace tab's label ink. Instead of the derived accent."},
	{"workspace_pill_inactive_bg", `"#11111b"`, "Every other workspace tab's fill. Instead of the same neutral Panel step."},
	{"workspace_pill_inactive_fg", `"#a6adc8"`, "Every other workspace tab's label ink. Instead of the derived FgDim."},

	{"terminal_cursor_fg", `"#f5e0dc"`, "The real terminal cursor's color. Instead of the derived cursor. A guest app's own cursor-color escape sequence still wins over this."},
	{"button_fg", `"#11111b"`, "Window control-button ink (minimize/maximize/close). Instead of the fixed black."},

	{"copy_cursor_bg", `"#94e2d5"`, "Copy-mode cursor cell, background half. Instead of the derived bright_cyan."},
	{"copy_cursor_fg", `"#11111b"`, "Copy-mode cursor cell, foreground half. Instead of the derived black."},
	{"copy_visual_selection_bg", `"#cba6f7"`, "Copy-mode visual selection, background half. Instead of the derived purple."},
	{"copy_visual_selection_fg", `"#ffffff"`, "Copy-mode visual selection, foreground half. Instead of the derived bright_white."},
	{"copy_search_current_bg", `"#f38ba8"`, "Current search match in copy mode, background half. Instead of the derived bright_purple."},
	{"copy_search_current_fg", `"#11111b"`, "Current search match in copy mode, foreground half. Instead of the derived black."},
	{"copy_search_other_bg", `"#f9e2af"`, "Other search matches in copy mode, background half. Instead of the derived yellow."},
	{"copy_search_other_fg", `"#11111b"`, "Other search matches in copy mode, foreground half. Instead of the derived black."},
	{"copy_search_bar_bg", `"#f9e2af"`, "The /search input box's background. No derived default - it styles itself like every other input dialog until a theme sets this."},
	{"copy_search_bar_fg", `"#11111b"`, "The /search input box's text color. No derived default, same as above."},

	{"notification_error", `"#f38ba8"`, "Error notification accent. Instead of the derived red."},
	{"notification_warning", `"#f9e2af"`, "Warning notification accent. Instead of the derived yellow."},
	{"notification_success", `"#a6e3a1"`, "Success notification accent. Instead of the derived green."},
	{"notification_info", `"#89b4fa"`, "Info notification accent. Instead of the derived blue."},
	{"notification_bg", `"#1e1e2e"`, "Notification message body background. Instead of the derived bg."},
	{"notification_fg", `"#d4d4d4"`, "Notification message body text. Instead of the derived fg."},

	{"accent", `"#89b4fa"`, "Overlay chrome accent: dock highlights, active tabs, badges. Instead of the derived bright_blue."},
	{"accent_bright", `"#94e2d5"`, "Brighter accent, for icons and key badges. Instead of the derived bright_cyan."},
	{"selected", `"#89b4fa"`, "Strong selection tint in overlays. Instead of the derived bright_blue."},
	{"warn", `"#f38ba8"`, "Destructive/reset accent in overlays. Instead of the derived bright_red."},
	{"success", `"#a6e3a1"`, "On/enabled accent in overlays. Instead of the derived bright_green."},
	{"info", `"#89b4fa"`, "Informational accent in overlays. Instead of the derived bright_blue."},
	{"warning", `"#f9e2af"`, "Caution accent in overlays. Instead of the derived yellow."},
}

// GenerateExampleTheme renders a fully commented reference theme file in
// TOML - every field a custom theme (see custom.go) can carry, at the
// default color it falls back to when omitted, with a one-line description,
// plus the full set of optional [ui] overrides. It reads only the compiled-in
// tables above, never a file on disk.
func GenerateExampleTheme() string {
	var b strings.Builder
	b.WriteString(exampleThemeHeader)

	b.WriteString("\n# id is the name you select the theme by. If omitted, it is derived from\n")
	b.WriteString("# the filename: my-theme.toml becomes \"my-theme\".\n")
	b.WriteString("# id = \"my-theme\"\n\n")
	b.WriteString("# display_name is what the theme picker shows. Falls back to id if omitted.\n")
	b.WriteString("# display_name = \"My Theme\"\n\n")
	b.WriteString("# dark marks the theme as dark, for any future UI heuristic that cares.\n")
	b.WriteString("# dark = true\n")

	writeExampleColorSection(&b, "Base colors", exampleBaseColors)

	b.WriteString("\n# ─────────────────────────────────────────────────────────────────────\n")
	b.WriteString("# Per-element overrides (all optional; see docs/THEMES.md, \"Per-Element\n")
	b.WriteString("# Overrides\" for the full explanation, especially dock_bg).\n")
	b.WriteString("# ─────────────────────────────────────────────────────────────────────\n")
	b.WriteString("# [ui]\n")
	for _, f := range exampleUIOverrides {
		writeExampleColorField(&b, f)
	}

	return b.String()
}

func writeExampleColorSection(b *strings.Builder, title string, fields []exampleColorField) {
	fmt.Fprintf(b, "\n# ─────────────────────────────────────────────────────────────────────\n")
	fmt.Fprintf(b, "# %s\n", title)
	fmt.Fprintf(b, "# ─────────────────────────────────────────────────────────────────────\n")
	for _, f := range fields {
		writeExampleColorField(b, f)
	}
}

func writeExampleColorField(b *strings.Builder, f exampleColorField) {
	writeWrappedExampleComment(b, f.Desc)
	fmt.Fprintf(b, "# %s = %s\n", f.Key, f.Example)
}

// writeWrappedExampleComment writes text as one or more "# "-prefixed lines,
// each no wider than 78 columns - the same wrapping GenerateExampleConfig
// uses for config.toml, duplicated here for the reason noted on
// exampleColorField above.
func writeWrappedExampleComment(b *strings.Builder, text string) {
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
