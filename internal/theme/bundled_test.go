package theme

import (
	"testing"

	tint "github.com/lrstanley/bubbletint/v2"
)

// TestRegisterBundledThemesIncludesTraining pins tuios's own bundled
// "training" theme (internal/theme/bundled/training.toml) actually reaching
// the tint registry - the whole point of shipping it in the binary is that
// it needs no ~/.config/tuios/themes/ file to show up.
func TestRegisterBundledThemesIncludesTraining(t *testing.T) {
	tint.NewDefaultRegistry()
	registerBundledThemes()

	tn, ok := tint.GetTint("training")
	if !ok {
		t.Fatal(`registerBundledThemes did not register "training"`)
	}
	if tn.DisplayName != "Training" {
		t.Errorf("DisplayName = %q, want %q", tn.DisplayName, "Training")
	}
	if tn.Dark {
		t.Error("Dark = true, want false (training is a light theme)")
	}
	if got := tn.Bg.Hex(); got != "#ffffff" {
		t.Errorf("Bg = %q, want #ffffff", got)
	}
	if got := tn.Fg.Hex(); got != "#000000" {
		t.Errorf("Fg = %q, want #000000", got)
	}

	wp := WebPresetForID("training")
	if wp == nil {
		t.Fatal(`expected a [web] preset for "training", got none`)
	}
	if wp.Font != "saucecodepro nfm semibold" {
		t.Errorf("web preset font = %q, want %q", wp.Font, "saucecodepro nfm semibold")
	}
	if wp.FontSize != 24 {
		t.Errorf("web preset font_size = %d, want 24", wp.FontSize)
	}
}

// TestRegisterBundledThemesParsesEveryEmbeddedFile is a defensive check on
// the embed itself: every *.toml under bundled/ must actually parse and
// register, not silently get skipped with a logged warning - the state a
// typo in the TOML would otherwise leave unnoticed.
func TestRegisterBundledThemesParsesEveryEmbeddedFile(t *testing.T) {
	entries, err := bundledThemeFiles.ReadDir("bundled")
	if err != nil {
		t.Fatalf("reading embedded bundled/ dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("bundled/ has no theme files embedded")
	}

	tint.NewDefaultRegistry()
	registerBundledThemes()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := bundledThemeFiles.ReadFile("bundled/" + entry.Name())
		if err != nil {
			t.Fatalf("reading embedded %s: %v", entry.Name(), err)
		}
		want, err := loadThemeBytes(data, entry.Name())
		if err != nil {
			t.Fatalf("bundled/%s does not parse: %v", entry.Name(), err)
		}
		if _, ok := tint.GetTint(want.ID); !ok {
			t.Errorf("bundled/%s parsed to id %q, but registerBundledThemes did not register it", entry.Name(), want.ID)
		}
	}
}
