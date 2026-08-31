package main

import (
	"os"
	"testing"
)

func TestNormalizeFontKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"saucecodepro", "saucecodepro"},
		{"SauceCodePro", "saucecodepro"},
		{"Sauce Code Pro", "sauce code pro"},
		{"  sauce   code  pro  ", "sauce code pro"},
		{"SauceCodePro NFM", "saucecodepro nfm"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeFontKey(tt.in); got != tt.want {
			t.Errorf("normalizeFontKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveFontConfig(t *testing.T) {
	t.Run("empty family and path pass through unchanged", func(t *testing.T) {
		family, path, err := resolveFontConfig("", "")
		if err != nil || family != "" || path != "" {
			t.Fatalf("got (%q, %q, %v), want (\"\", \"\", nil)", family, path, err)
		}
	})

	t.Run("plain CSS name passes through unchanged", func(t *testing.T) {
		family, path, err := resolveFontConfig("Fira Code", "")
		if err != nil || family != "Fira Code" || path != "" {
			t.Fatalf("got (%q, %q, %v), want (\"Fira Code\", \"\", nil)", family, path, err)
		}
	})

	t.Run("explicit font-path always wins over a bundled name", func(t *testing.T) {
		family, path, err := resolveFontConfig("saucecodepro", "/my/own/font.ttf")
		if err != nil || family != "saucecodepro" || path != "/my/own/font.ttf" {
			t.Fatalf("got (%q, %q, %v), want the explicit path untouched", family, path, err)
		}
	})

	t.Run("bundled name spills the embedded font and resolves the CSS name", func(t *testing.T) {
		family, path, err := resolveFontConfig("SauceCodePro", "")
		if err != nil {
			t.Fatalf("resolveFontConfig: %v", err)
		}
		if want := "SauceCodePro NFM"; family != want {
			t.Errorf("family = %q, want %q", family, want)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("spilled font not readable at %q: %v", path, err)
		}
		if len(data) != len(sauceCodeProFont) {
			t.Errorf("spilled file is %d bytes, embedded asset is %d", len(data), len(sauceCodeProFont))
		}
	})

	t.Run("bundled name aliases all resolve to the same family", func(t *testing.T) {
		for _, alias := range []string{"saucecodepro", "Sauce Code Pro", "SauceCodePro NFM", "sauce code pro nfm"} {
			family, _, err := resolveFontConfig(alias, "")
			if err != nil {
				t.Fatalf("resolveFontConfig(%q): %v", alias, err)
			}
			if want := "SauceCodePro NFM"; family != want {
				t.Errorf("resolveFontConfig(%q) family = %q, want %q", alias, family, want)
			}
		}
	})

	t.Run("semibold bundled name spills its own font under its own family", func(t *testing.T) {
		for _, alias := range []string{"saucecodeprosemibold", "Sauce Code Pro SemiBold", "SauceCodePro NFM SemiBold"} {
			family, path, err := resolveFontConfig(alias, "")
			if err != nil {
				t.Fatalf("resolveFontConfig(%q): %v", alias, err)
			}
			if want := "SauceCodePro NFM SemiBold"; family != want {
				t.Errorf("resolveFontConfig(%q) family = %q, want %q", alias, family, want)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("spilled font not readable at %q: %v", path, err)
			}
			if len(data) != len(sauceCodeProSemiBoldFont) {
				t.Errorf("spilled file is %d bytes, embedded asset is %d", len(data), len(sauceCodeProSemiBoldFont))
			}
			// The two weights must never collide on disk: spilling one after
			// the other into the same temp dir by filename would silently
			// serve the wrong bytes under a fresh --font-path/--font-family
			// pair that only ever asks for one of them.
			if len(data) == len(sauceCodeProFont) {
				t.Errorf("spilled SemiBold font happens to be the same size as Regular (%d bytes) - verify it is not actually the Regular file", len(data))
			}
		}
	})
}
