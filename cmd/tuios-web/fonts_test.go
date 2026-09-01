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

func TestWebFontCSSValue(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"saucecodepro nfm semibold", "'SauceCodePro NFM SemiBold', monospace"},
		{"SauceCodePro NFM SemiBold", "'SauceCodePro NFM SemiBold', monospace"},
		{"freemono", "'FreeMono', monospace"},
		// An unrecognized key (a custom theme author's own font, or a typo)
		// passes through as a literal CSS family name, the same fallback
		// resolveFontConfig gives an unrecognized --font-family.
		{"Fira Code", "'Fira Code', monospace"},
	}
	for _, tt := range tests {
		if got := webFontCSSValue(tt.in); got != tt.want {
			t.Errorf("webFontCSSValue(%q) = %q, want %q", tt.in, got, tt.want)
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

	t.Run("freemono and its bold weight resolve to distinct families", func(t *testing.T) {
		for _, tt := range []struct {
			aliases []string
			family  string
			data    []byte
		}{
			{[]string{"freemono", "Free Mono"}, "FreeMono", freeMonoFont},
			{[]string{"freemonobold", "Free Mono Bold"}, "FreeMono Bold", freeMonoBoldFont},
		} {
			for _, alias := range tt.aliases {
				family, path, err := resolveFontConfig(alias, "")
				if err != nil {
					t.Fatalf("resolveFontConfig(%q): %v", alias, err)
				}
				if family != tt.family {
					t.Errorf("resolveFontConfig(%q) family = %q, want %q", alias, family, tt.family)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("spilled font not readable at %q: %v", path, err)
				}
				if len(data) != len(tt.data) {
					t.Errorf("spilled file is %d bytes, embedded asset is %d", len(data), len(tt.data))
				}
			}
		}
	})

	t.Run("sourcecodepro and its bold weight resolve to distinct families", func(t *testing.T) {
		for _, tt := range []struct {
			aliases []string
			family  string
			data    []byte
		}{
			{[]string{"sourcecodepro", "Source Code Pro"}, "Source Code Pro", sourceCodeProFont},
			{[]string{"sourcecodeprobold", "Source Code Pro Bold"}, "Source Code Pro Bold", sourceCodeProBoldFont},
		} {
			for _, alias := range tt.aliases {
				family, path, err := resolveFontConfig(alias, "")
				if err != nil {
					t.Fatalf("resolveFontConfig(%q): %v", alias, err)
				}
				if family != tt.family {
					t.Errorf("resolveFontConfig(%q) family = %q, want %q", alias, family, tt.family)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("spilled font not readable at %q: %v", path, err)
				}
				if len(data) != len(tt.data) {
					t.Errorf("spilled file is %d bytes, embedded asset is %d", len(data), len(tt.data))
				}
			}
		}
	})
}
