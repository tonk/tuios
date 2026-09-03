package theme

// WebPreset is the optional "web" table/object a theme file can carry: a
// font (and/or point size) tuios-web should switch the browser terminal to
// when this theme is selected, letting a theme like "Trainer" bundle a
// larger, more legible face for a projector or a shared screen instead of
// leaving the operator to pick a matching font by hand every time the theme
// is picked. The TUI itself has no browser font to switch, so only
// tuios-web consults this.
type WebPreset struct {
	// Font is the theme file's raw font key, e.g. "saucecodepro nfm
	// semibold" - a bundled font alias if tuios-web recognizes one, or a
	// literal CSS font-family name otherwise. Resolving it is entirely
	// tuios-web's job (see its own bundledFonts); this package only carries
	// the string through.
	Font string
	// FontSize is a CSS point size for the browser terminal. Zero means "no
	// size override" - the browser keeps whatever size is already set.
	FontSize int
}

// webPresetByID holds the "web" preset parsed from custom theme files, keyed
// by theme ID. Populated by loadThemeBytes (custom.go); read-only after
// that, the same lifecycle as overridesByID.
var webPresetByID = map[string]*WebPreset{}

// WebPresetForID returns the web font preset for theme id, or nil if it has
// none - true of every built-in theme, and of any custom/bundled theme
// without a "web" section. Looked up by ID rather than the active theme
// (contrast overridesForCurrent) because tuios-web needs to answer "what
// does this theme want" for whichever theme the picker just switched to,
// not only the one already applied.
func WebPresetForID(id string) *WebPreset {
	return webPresetByID[id]
}

// webPresetRaw mirrors WebPreset for JSON/TOML decoding.
type webPresetRaw struct {
	Font     string `json:"font"      toml:"font"`
	FontSize int    `json:"font_size" toml:"font_size"`
}

// toWebPreset converts the raw decoded fields to a *WebPreset, or nil if r
// is nil or carries neither field (no "web" section in the file, or an
// empty one).
func (r *webPresetRaw) toWebPreset() *WebPreset {
	if r == nil || (r.Font == "" && r.FontSize == 0) {
		return nil
	}
	return &WebPreset{Font: r.Font, FontSize: r.FontSize}
}
