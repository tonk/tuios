package theme

import (
	"embed"
	"io/fs"
	"log"

	tint "github.com/lrstanley/bubbletint/v2"
)

// bundledThemeFiles holds theme files tuios ships with the binary itself -
// distinct from bubbletint's own large built-in catalog (registered by
// tint.NewDefaultRegistry, an upstream dependency this package doesn't
// control) and from a user's own ~/.config/tuios/themes/ (LoadCustomThemes,
// which requires the user to have put a file there first). A bundled theme
// needs neither: it's just always there.
//
//go:embed bundled/*.toml
var bundledThemeFiles embed.FS

// registerBundledThemes registers every theme in bundledThemeFiles. Called
// from EnsureRegistry, after tint.NewDefaultRegistry and before
// LoadCustomThemes - in that order, so a user's own themes-dir file with
// the same id as a bundled one still wins (tint.Register overwrites an
// existing id unconditionally, and LoadCustomThemes runs after this).
func registerBundledThemes() {
	entries, err := fs.ReadDir(bundledThemeFiles, "bundled")
	if err != nil {
		// Should only happen if the embed itself is empty/misconfigured -
		// not a user-facing condition, so this is the same "warn and keep
		// going" treatment LoadCustomThemes gives a bad file.
		log.Printf("Warning: error listing bundled themes: %v", err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := bundledThemeFiles.ReadFile("bundled/" + entry.Name())
		if err != nil {
			log.Printf("Warning: skipping bundled theme %s: %v", entry.Name(), err)
			continue
		}
		t, err := loadThemeBytes(data, entry.Name())
		if err != nil {
			log.Printf("Warning: skipping bundled theme %s: %v", entry.Name(), err)
			continue
		}
		tint.Register(t)
	}
}
