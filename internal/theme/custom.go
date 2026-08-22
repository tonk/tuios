package theme

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	tint "github.com/lrstanley/bubbletint/v2"
	toml "github.com/pelletier/go-toml/v2"
)

// GetThemesDir returns the path to the custom themes directory (~/.config/tuios/themes/).
// Creates the directory if it doesn't exist.
func GetThemesDir() (string, error) {
	// Use xdg.ConfigFile to get the path and ensure parent dirs exist
	keepFile, err := xdg.ConfigFile("tuios/themes/.keep")
	if err != nil {
		return "", fmt.Errorf("failed to get themes directory: %w", err)
	}
	return filepath.Dir(keepFile), nil
}

// LoadCustomThemes reads all *.json and *.toml files from the themes
// directory, loads each as a custom theme, and registers them with
// bubbletint. Returns the list of successfully loaded theme IDs.
// Logs warnings for bad files but doesn't fail startup.
func LoadCustomThemes(themesDir string) ([]string, error) {
	entries, err := os.ReadDir(themesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read themes directory: %w", err)
	}

	var loaded []string
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if entry.IsDir() || !(strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".toml")) {
			continue
		}

		path := filepath.Join(themesDir, entry.Name())
		t, err := LoadCustomThemeFile(path)
		if err != nil {
			log.Printf("Warning: skipping custom theme %s: %v", entry.Name(), err)
			continue
		}

		tint.Register(t)
		loaded = append(loaded, t.ID)
	}

	return loaded, nil
}

// LoadCustomThemeFile reads a JSON or TOML theme file (by extension; anything
// else is treated as JSON, matching the format this loader has always used)
// and returns a *tint.Tint. Derives ID from filename if the id field is
// empty. Sets DisplayName from ID if empty. Fills missing color fields with
// defaults.
//
// A file may also carry an optional "ui" object/table assigning specific
// colors to specific UI elements (see UIOverrides); when present, it is
// stashed in overridesByID under the theme's ID for the accessors in
// theme.go and ui.go to consult.
func LoadCustomThemeFile(path string) (*tint.Tint, error) {
	// #nosec G304 - path is from user's config directory, reading custom themes is intentional
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read theme file: %w", err)
	}

	var t tint.Tint
	var ui *uiOverridesRaw

	if strings.EqualFold(filepath.Ext(path), ".toml") {
		var f tomlThemeFile
		if err := toml.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("failed to parse theme TOML: %w", err)
		}
		t = *f.toTint()
		ui = f.UI
	} else {
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("failed to parse theme JSON: %w", err)
		}
		// A second, best-effort pass for the "ui" object: kept separate from
		// the tint.Tint unmarshal above so an unparsed or absent "ui" section
		// never affects loading the theme's actual colors.
		var wrapper struct {
			UI *uiOverridesRaw `json:"ui"`
		}
		if err := json.Unmarshal(data, &wrapper); err == nil {
			ui = wrapper.UI
		}
	}

	// Derive ID from filename if not set in the file
	if t.ID == "" {
		base := filepath.Base(path)
		t.ID = strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
	}

	if t.ID == "" {
		return nil, fmt.Errorf("theme has no ID")
	}

	// Set DisplayName from ID if empty
	if t.DisplayName == "" {
		t.DisplayName = t.ID
	}

	fillDefaults(&t)

	if ov := ui.toUIOverrides(); ov != nil {
		overridesByID[t.ID] = ov
	} else {
		delete(overridesByID, t.ID)
	}

	return &t, nil
}

// tomlThemeFile mirrors tint.Tint for TOML decoding: bubbletint's Color type
// only implements json.Unmarshaler, not a TOML-compatible interface, so every
// color here is a plain "#rrggbb" string, converted to a *tint.Color by
// toTint below (which reuses tint.FromHex, the same helper fillDefaults uses).
type tomlThemeFile struct {
	ID          string `toml:"id"`
	DisplayName string `toml:"display_name"`
	Dark        bool   `toml:"dark"`

	Fg          string `toml:"fg"`
	Bg          string `toml:"bg"`
	Cursor      string `toml:"cursor"`
	SelectionBg string `toml:"selection_bg"`

	Black  string `toml:"black"`
	Red    string `toml:"red"`
	Green  string `toml:"green"`
	Yellow string `toml:"yellow"`
	Blue   string `toml:"blue"`
	Purple string `toml:"purple"`
	Cyan   string `toml:"cyan"`
	White  string `toml:"white"`

	BrightBlack  string `toml:"bright_black"`
	BrightRed    string `toml:"bright_red"`
	BrightGreen  string `toml:"bright_green"`
	BrightYellow string `toml:"bright_yellow"`
	BrightBlue   string `toml:"bright_blue"`
	BrightPurple string `toml:"bright_purple"`
	BrightCyan   string `toml:"bright_cyan"`
	BrightWhite  string `toml:"bright_white"`

	UI *uiOverridesRaw `toml:"ui"`
}

// toTint builds a *tint.Tint from the decoded TOML fields. Left-empty color
// strings become nil *tint.Color, which fillDefaults (called by the caller)
// then fills in exactly as it does for an omitted JSON field.
func (f *tomlThemeFile) toTint() *tint.Tint {
	hex := func(s string) *tint.Color {
		if s == "" {
			return nil
		}
		return tint.FromHex(s)
	}
	return &tint.Tint{
		ID:          f.ID,
		DisplayName: f.DisplayName,
		Dark:        f.Dark,

		Fg:          hex(f.Fg),
		Bg:          hex(f.Bg),
		Cursor:      hex(f.Cursor),
		SelectionBg: hex(f.SelectionBg),

		Black:  hex(f.Black),
		Red:    hex(f.Red),
		Green:  hex(f.Green),
		Yellow: hex(f.Yellow),
		Blue:   hex(f.Blue),
		Purple: hex(f.Purple),
		Cyan:   hex(f.Cyan),
		White:  hex(f.White),

		BrightBlack:  hex(f.BrightBlack),
		BrightRed:    hex(f.BrightRed),
		BrightGreen:  hex(f.BrightGreen),
		BrightYellow: hex(f.BrightYellow),
		BrightBlue:   hex(f.BrightBlue),
		BrightPurple: hex(f.BrightPurple),
		BrightCyan:   hex(f.BrightCyan),
		BrightWhite:  hex(f.BrightWhite),
	}
}

// fillDefaults fills nil color pointers with xterm defaults.
func fillDefaults(t *tint.Tint) {
	// Default foreground/background
	if t.Fg == nil {
		t.Fg = tint.FromHex("#e5e5e5")
	}
	if t.Bg == nil {
		t.Bg = tint.FromHex("#000000")
	}

	// Cursor defaults to Fg
	if t.Cursor == nil {
		t.Cursor = copyColor(t.Fg)
	}

	// Normal ANSI colors (xterm defaults)
	if t.Black == nil {
		t.Black = tint.FromHex("#000000")
	}
	if t.Red == nil {
		t.Red = tint.FromHex("#cd0000")
	}
	if t.Green == nil {
		t.Green = tint.FromHex("#00cd00")
	}
	if t.Yellow == nil {
		t.Yellow = tint.FromHex("#cdcd00")
	}
	if t.Blue == nil {
		t.Blue = tint.FromHex("#0000ee")
	}
	if t.Purple == nil {
		t.Purple = tint.FromHex("#cd00cd")
	}
	if t.Cyan == nil {
		t.Cyan = tint.FromHex("#00cdcd")
	}
	if t.White == nil {
		t.White = tint.FromHex("#e5e5e5")
	}

	// Bright variants default to normal if nil
	if t.BrightBlack == nil {
		t.BrightBlack = copyColor(t.Black)
	}
	if t.BrightRed == nil {
		t.BrightRed = copyColor(t.Red)
	}
	if t.BrightGreen == nil {
		t.BrightGreen = copyColor(t.Green)
	}
	if t.BrightYellow == nil {
		t.BrightYellow = copyColor(t.Yellow)
	}
	if t.BrightBlue == nil {
		t.BrightBlue = copyColor(t.Blue)
	}
	if t.BrightPurple == nil {
		t.BrightPurple = copyColor(t.Purple)
	}
	if t.BrightCyan == nil {
		t.BrightCyan = copyColor(t.Cyan)
	}
	if t.BrightWhite == nil {
		t.BrightWhite = copyColor(t.White)
	}
}

// copyColor creates a copy of a tint.Color.
func copyColor(c *tint.Color) *tint.Color {
	if c == nil {
		return nil
	}
	dup := *c
	return &dup
}
