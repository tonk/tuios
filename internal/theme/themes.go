package theme

import (
	"image/color"
	"log"
	"sort"
	"sync"

	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// defaultSwatch is the preview palette shown for the "no theme" option (the
// standard xterm-ish bright colors).
var defaultSwatch = []color.Color{
	lipgloss.Color("#ff5555"), lipgloss.Color("#f1fa8c"), lipgloss.Color("#50fa7b"),
	lipgloss.Color("#8be9fd"), lipgloss.Color("#6272a4"), lipgloss.Color("#bd93f9"),
	lipgloss.Color("#f8f8f2"), lipgloss.Color("#282a36"),
}

// ThemeSwatch returns a small, representative set of colors for a theme id, for
// previewing it in the theme picker. Unknown or empty ids return the default
// palette.
func ThemeSwatch(id string) []color.Color {
	EnsureRegistry()
	t, ok := tint.GetTint(id)
	if !ok || t == nil {
		return defaultSwatch
	}
	return []color.Color{
		t.BrightRed, t.BrightYellow, t.BrightGreen,
		t.BrightCyan, t.BrightBlue, t.BrightPurple,
		t.Fg, t.Bg,
	}
}

var ensureRegistryOnce sync.Once

// EnsureRegistry populates the tint registry with the built-in tints, the
// themes tuios bundles with its own binary (see bundled.go), and any custom
// themes from the user's themes directory - in that order, so a user's own
// file always wins a same-id collision with either of the other two -
// without enabling theming or changing the current theme. This lets the
// settings page list and preview themes even when the session started with
// no theme selected.
func EnsureRegistry() {
	ensureRegistryOnce.Do(func() {
		tint.NewDefaultRegistry()
		registerBundledThemes()
		if themesDir, err := GetThemesDir(); err == nil {
			if _, err := LoadCustomThemes(themesDir); err != nil {
				log.Printf("Warning: error loading custom themes: %v", err)
			}
		}
	})
}

// AvailableThemes returns the sorted list of registered theme IDs (built-in
// tints plus any custom themes loaded from the user's themes directory). The
// list is used by the in-app settings page to cycle themes.
func AvailableThemes() []string {
	EnsureRegistry()
	ids := tint.TintIDs()
	sorted := make([]string, len(ids))
	copy(sorted, ids)
	sort.Strings(sorted)
	return sorted
}

// CurrentThemeID returns the ID of the active theme, or an empty string when
// theming is disabled.
func CurrentThemeID() string {
	t := Current()
	if t == nil {
		return ""
	}
	return t.ID
}

// ReloadCustomThemes re-scans the themes directory and re-registers every
// theme file found, picking up edits made since EnsureRegistry's one-time
// load (or the last call to this) without a restart - unlike that one-time
// load, this can run again at any point in the process's life.
//
// tint.Register overwrites an existing ID unconditionally, and tint.Current
// looks its ID up in the registry fresh on every call rather than caching a
// *Tint, so if the active theme's file is among the ones reloaded, the new
// colors (and [ui] overrides, via LoadCustomThemeFile's own overridesByID
// bookkeeping) take effect immediately - no need to re-select it.
//
// A theme file that has been deleted since the last load stays registered
// (and any [ui] overrides it set stay in overridesByID) until the process
// restarts; nothing here removes a registration or an override, only adds or
// replaces one. Returns the reloaded theme IDs.
func ReloadCustomThemes() ([]string, error) {
	// Guarantees a registry exists to register into - a no-op once the
	// process has already ensured one, which every path that could reach
	// this already has (theme.Initialize, the settings page, the picker).
	EnsureRegistry()
	themesDir, err := GetThemesDir()
	if err != nil {
		return nil, err
	}
	return LoadCustomThemes(themesDir)
}
