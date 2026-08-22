package theme

import (
	"image/color"

	tint "github.com/lrstanley/bubbletint/v2"
)

// UIOverrides lets a custom theme assign a specific color to one UI element
// directly, instead of only supplying the 16 ANSI colors plus fg/bg/cursor and
// letting TUIOS derive chrome from them (see the accessor functions in
// theme.go and ui.go's UI(), which is what every field here shadows). Every
// field is optional; a nil field falls back to the theme-derived default the
// corresponding accessor already computes.
//
// Only custom (file-based) themes can carry these: built-in themes come from
// the vendored bubbletint package as plain tint.Tint values, with no room for
// an extra section.
type UIOverrides struct {
	BorderUnfocused       color.Color
	BorderFocusedWindow   color.Color
	BorderFocusedTerminal color.Color
	BorderMultifocused    color.Color

	DockWindow    color.Color
	DockTerminal  color.Color
	DockCopy      color.Color
	DockHighlight color.Color

	// DockTrailFg is the ink for the dock's trailing status text - the
	// "<workspace>:<window count>" readout plus whatever badges ride after
	// it (the project-tape indicator, view-only). Instead of the derived
	// FgMute.
	DockTrailFg color.Color

	// DockIndicatorActiveFg and DockIndicatorInactiveFg are the ink for the
	// dock's mode-indicator glyphs (mouse mode, tiling, focus-follows-mouse):
	// a high-contrast color while the mode is on, a dull one while it is off.
	// Falls back to pal.Success / pal.FgMute when unset.
	DockIndicatorActiveFg   color.Color
	DockIndicatorInactiveFg color.Color

	// The workspace-tab pills in the dock strip (and dock_window_list's
	// window pills, which share the same rendering). Bg is the pill's own
	// fill, distinct per active/inactive state; Fg is the label ink.
	WorkspacePillActiveBg   color.Color
	WorkspacePillActiveFg   color.Color
	WorkspacePillInactiveBg color.Color
	WorkspacePillInactiveFg color.Color

	// DockBg, unlike every other field here, has no theme-derived default to
	// fall back to: the dock row paints no background at all today (its bare
	// cells show the real terminal's own colour), and DockRowBackground below
	// reports whether this is set at all so callers know whether to paint one.
	// Setting it is the equivalent of tmux's status-style bg.
	DockBg color.Color

	CopyCursorBg          color.Color
	CopyCursorFg          color.Color
	CopyVisualSelectionBg color.Color
	CopyVisualSelectionFg color.Color
	CopySearchCurrentBg   color.Color
	CopySearchCurrentFg   color.Color
	CopySearchOtherBg     color.Color
	CopySearchOtherFg     color.Color
	CopySearchBarBg       color.Color
	CopySearchBarFg       color.Color

	TerminalCursorFg color.Color
	TerminalCursorBg color.Color

	ButtonFg color.Color

	NotificationError   color.Color
	NotificationWarning color.Color
	NotificationSuccess color.Color
	NotificationInfo    color.Color
	NotificationBg      color.Color
	NotificationFg      color.Color

	// These four mirror ui.Palette's own theme-derived fields (Accent,
	// AccentBright, Selected, Warn/Success/Info/Warning) - the overlay chrome
	// tokens, not a pane's own colors.
	Accent       color.Color
	AccentBright color.Color
	Selected     color.Color
	Warn         color.Color
	Success      color.Color
	Info         color.Color
	Warning      color.Color
}

// overridesByID holds the [ui] overrides parsed from custom theme files,
// keyed by theme ID. Populated once at startup by LoadCustomThemes /
// LoadCustomThemeFile (internal/theme/custom.go); read-only after that.
var overridesByID = map[string]*UIOverrides{}

// overridesForCurrent returns the active theme's overrides, or nil if it has
// none - true of every built-in theme, and of any custom theme without a
// [ui] table.
func overridesForCurrent() *UIOverrides {
	t := Current()
	if t == nil {
		return nil
	}
	return overridesByID[t.ID]
}

// DockRowBackground returns the active theme's dock_bg override and true, or
// (nil, false) if none is set. Unlike the other accessors in theme.go, there
// is no theme-derived default for this one to fall back to - the dock row
// paints no background at all unless a theme opts in with dock_bg, so
// callers need the bool to tell "paint this" from "leave it alone".
func DockRowBackground() (color.Color, bool) {
	ov := overridesForCurrent()
	if ov == nil || ov.DockBg == nil {
		return nil, false
	}
	return ov.DockBg, true
}

// DockTrailFg returns the active theme's dock_trail_fg override, or nil if
// unset - the caller's own derived default (pal.FgMute) then applies.
func DockTrailFg() color.Color {
	ov := overridesForCurrent()
	if ov == nil {
		return nil
	}
	return ov.DockTrailFg
}

// WorkspacePillBg returns the active theme's workspace_pill_active_bg or
// workspace_pill_inactive_bg override for the given state, or nil if unset -
// the caller's own neutral default then applies, the same shape as every
// other override that has one.
func WorkspacePillBg(active bool) color.Color {
	ov := overridesForCurrent()
	if ov == nil {
		return nil
	}
	if active {
		return ov.WorkspacePillActiveBg
	}
	return ov.WorkspacePillInactiveBg
}

// DockIndicatorFg returns the active theme's dock_indicator_active_fg or
// dock_indicator_inactive_fg override for the given state, or nil if unset -
// the caller's own derived default (pal.Success / pal.FgMute) then applies.
func DockIndicatorFg(active bool) color.Color {
	ov := overridesForCurrent()
	if ov == nil {
		return nil
	}
	if active {
		return ov.DockIndicatorActiveFg
	}
	return ov.DockIndicatorInactiveFg
}

// WorkspacePillFg is WorkspacePillBg for workspace_pill_active_fg /
// workspace_pill_inactive_fg.
func WorkspacePillFg(active bool) color.Color {
	ov := overridesForCurrent()
	if ov == nil {
		return nil
	}
	if active {
		return ov.WorkspacePillActiveFg
	}
	return ov.WorkspacePillInactiveFg
}

// uiOverridesRaw mirrors UIOverrides but as hex-string fields ("#rrggbb"),
// matching how a theme file spells a color everywhere else. It decodes the
// optional "ui" object/table from either a JSON or a TOML theme file.
type uiOverridesRaw struct {
	BorderUnfocused       string `json:"border_unfocused"        toml:"border_unfocused"`
	BorderFocusedWindow   string `json:"border_focused_window"   toml:"border_focused_window"`
	BorderFocusedTerminal string `json:"border_focused_terminal" toml:"border_focused_terminal"`
	BorderMultifocused    string `json:"border_multifocused"     toml:"border_multifocused"`

	DockWindow    string `json:"dock_window"    toml:"dock_window"`
	DockTerminal  string `json:"dock_terminal"  toml:"dock_terminal"`
	DockCopy      string `json:"dock_copy"      toml:"dock_copy"`
	DockHighlight string `json:"dock_highlight" toml:"dock_highlight"`
	DockBg        string `json:"dock_bg"        toml:"dock_bg"`
	DockTrailFg   string `json:"dock_trail_fg"  toml:"dock_trail_fg"`

	DockIndicatorActiveFg   string `json:"dock_indicator_active_fg"   toml:"dock_indicator_active_fg"`
	DockIndicatorInactiveFg string `json:"dock_indicator_inactive_fg" toml:"dock_indicator_inactive_fg"`

	WorkspacePillActiveBg   string `json:"workspace_pill_active_bg"   toml:"workspace_pill_active_bg"`
	WorkspacePillActiveFg   string `json:"workspace_pill_active_fg"   toml:"workspace_pill_active_fg"`
	WorkspacePillInactiveBg string `json:"workspace_pill_inactive_bg" toml:"workspace_pill_inactive_bg"`
	WorkspacePillInactiveFg string `json:"workspace_pill_inactive_fg" toml:"workspace_pill_inactive_fg"`

	CopyCursorBg          string `json:"copy_cursor_bg"           toml:"copy_cursor_bg"`
	CopyCursorFg          string `json:"copy_cursor_fg"           toml:"copy_cursor_fg"`
	CopyVisualSelectionBg string `json:"copy_visual_selection_bg" toml:"copy_visual_selection_bg"`
	CopyVisualSelectionFg string `json:"copy_visual_selection_fg" toml:"copy_visual_selection_fg"`
	CopySearchCurrentBg   string `json:"copy_search_current_bg"   toml:"copy_search_current_bg"`
	CopySearchCurrentFg   string `json:"copy_search_current_fg"   toml:"copy_search_current_fg"`
	CopySearchOtherBg     string `json:"copy_search_other_bg"     toml:"copy_search_other_bg"`
	CopySearchOtherFg     string `json:"copy_search_other_fg"     toml:"copy_search_other_fg"`
	CopySearchBarBg       string `json:"copy_search_bar_bg"       toml:"copy_search_bar_bg"`
	CopySearchBarFg       string `json:"copy_search_bar_fg"       toml:"copy_search_bar_fg"`

	TerminalCursorFg string `json:"terminal_cursor_fg" toml:"terminal_cursor_fg"`
	TerminalCursorBg string `json:"terminal_cursor_bg" toml:"terminal_cursor_bg"`

	ButtonFg string `json:"button_fg" toml:"button_fg"`

	NotificationError   string `json:"notification_error"   toml:"notification_error"`
	NotificationWarning string `json:"notification_warning" toml:"notification_warning"`
	NotificationSuccess string `json:"notification_success" toml:"notification_success"`
	NotificationInfo    string `json:"notification_info"    toml:"notification_info"`
	NotificationBg      string `json:"notification_bg"      toml:"notification_bg"`
	NotificationFg      string `json:"notification_fg"      toml:"notification_fg"`

	Accent       string `json:"accent"        toml:"accent"`
	AccentBright string `json:"accent_bright" toml:"accent_bright"`
	Selected     string `json:"selected"      toml:"selected"`
	Warn         string `json:"warn"          toml:"warn"`
	Success      string `json:"success"       toml:"success"`
	Info         string `json:"info"          toml:"info"`
	Warning      string `json:"warning"       toml:"warning"`
}

// toUIOverrides converts the raw hex strings to a *UIOverrides, or nil if r
// itself is nil (no "ui" section in the file at all). A field left empty in
// the file stays nil in the result, which is exactly "no override" to every
// accessor that consults it.
func (r *uiOverridesRaw) toUIOverrides() *UIOverrides {
	if r == nil {
		return nil
	}
	hex := func(s string) color.Color {
		if s == "" {
			return nil
		}
		return tint.FromHex(s)
	}
	return &UIOverrides{
		BorderUnfocused:       hex(r.BorderUnfocused),
		BorderFocusedWindow:   hex(r.BorderFocusedWindow),
		BorderFocusedTerminal: hex(r.BorderFocusedTerminal),
		BorderMultifocused:    hex(r.BorderMultifocused),

		DockWindow:    hex(r.DockWindow),
		DockTerminal:  hex(r.DockTerminal),
		DockCopy:      hex(r.DockCopy),
		DockHighlight: hex(r.DockHighlight),
		DockBg:        hex(r.DockBg),
		DockTrailFg:   hex(r.DockTrailFg),

		DockIndicatorActiveFg:   hex(r.DockIndicatorActiveFg),
		DockIndicatorInactiveFg: hex(r.DockIndicatorInactiveFg),

		WorkspacePillActiveBg:   hex(r.WorkspacePillActiveBg),
		WorkspacePillActiveFg:   hex(r.WorkspacePillActiveFg),
		WorkspacePillInactiveBg: hex(r.WorkspacePillInactiveBg),
		WorkspacePillInactiveFg: hex(r.WorkspacePillInactiveFg),

		CopyCursorBg:          hex(r.CopyCursorBg),
		CopyCursorFg:          hex(r.CopyCursorFg),
		CopyVisualSelectionBg: hex(r.CopyVisualSelectionBg),
		CopyVisualSelectionFg: hex(r.CopyVisualSelectionFg),
		CopySearchCurrentBg:   hex(r.CopySearchCurrentBg),
		CopySearchCurrentFg:   hex(r.CopySearchCurrentFg),
		CopySearchOtherBg:     hex(r.CopySearchOtherBg),
		CopySearchOtherFg:     hex(r.CopySearchOtherFg),
		CopySearchBarBg:       hex(r.CopySearchBarBg),
		CopySearchBarFg:       hex(r.CopySearchBarFg),

		TerminalCursorFg: hex(r.TerminalCursorFg),
		TerminalCursorBg: hex(r.TerminalCursorBg),

		ButtonFg: hex(r.ButtonFg),

		NotificationError:   hex(r.NotificationError),
		NotificationWarning: hex(r.NotificationWarning),
		NotificationSuccess: hex(r.NotificationSuccess),
		NotificationInfo:    hex(r.NotificationInfo),
		NotificationBg:      hex(r.NotificationBg),
		NotificationFg:      hex(r.NotificationFg),

		Accent:       hex(r.Accent),
		AccentBright: hex(r.AccentBright),
		Selected:     hex(r.Selected),
		Warn:         hex(r.Warn),
		Success:      hex(r.Success),
		Info:         hex(r.Info),
		Warning:      hex(r.Warning),
	}
}
