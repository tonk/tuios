package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SauceCodePro Nerd Font Mono, SIL OFL 1.1 licensed (see fonts/SauceCodePro-LICENSE.txt),
// bundled as selectable fonts alongside sip's built-in JetBrains Mono Nerd
// Font. Only these two weights, each under its own family name: sip's
// custom-font @font-face declares a single file across the whole weight
// range (font-weight: 100 900), the same as any other --font-path, so a
// second weight has to be its own distinct family rather than a variant of
// "SauceCodePro NFM" - a Bold/Italic file under the same family name would
// never be selected. SemiBold exists for readability at a distance (a
// classroom's back row, a shared screen), not as a general bold-text
// weight - it replaces the whole font, not just SGR-bold characters.
//
//go:embed fonts/SauceCodeProNerdFontMono-Regular.ttf
var sauceCodeProFont []byte

//go:embed fonts/SauceCodeProNerdFontMono-SemiBold.ttf
var sauceCodeProSemiBoldFont []byte

// bundledFont pairs one embedded font's bytes with the CSS family name and
// filename it should be served/spilled under.
type bundledFont struct {
	family   string
	filename string
	data     []byte
}

// bundledFonts maps a normalized --font-family selector to the bundled font
// it names. Checked against normalizeFontKey(webFontFamily); anything else
// is passed through to sip as a plain CSS font-family name (see
// resolveFontConfig).
var bundledFonts = map[string]bundledFont{
	"saucecodepro":         {"SauceCodePro NFM", "SauceCodeProNerdFontMono-Regular.ttf", sauceCodeProFont},
	"sauce code pro":       {"SauceCodePro NFM", "SauceCodeProNerdFontMono-Regular.ttf", sauceCodeProFont},
	"saucecodepro nfm":     {"SauceCodePro NFM", "SauceCodeProNerdFontMono-Regular.ttf", sauceCodeProFont},
	"sauce code pro nfm":   {"SauceCodePro NFM", "SauceCodeProNerdFontMono-Regular.ttf", sauceCodeProFont},
	"saucecodeprofontmono": {"SauceCodePro NFM", "SauceCodeProNerdFontMono-Regular.ttf", sauceCodeProFont},

	"saucecodeprosemibold":      {"SauceCodePro NFM SemiBold", "SauceCodeProNerdFontMono-SemiBold.ttf", sauceCodeProSemiBoldFont},
	"sauce code pro semibold":   {"SauceCodePro NFM SemiBold", "SauceCodeProNerdFontMono-SemiBold.ttf", sauceCodeProSemiBoldFont},
	"saucecodepro nfm semibold": {"SauceCodePro NFM SemiBold", "SauceCodeProNerdFontMono-SemiBold.ttf", sauceCodeProSemiBoldFont},
}

// resolveFontConfig turns the --font-family/--font-path flags into the
// (FontFamily, FontPath) pair sip.Config wants. A recognized bundled name (see
// bundledFonts) is spilled to a temp file so sip's os.ReadFile-based static
// handler has a real path to serve, exactly as an explicit --font-path would;
// an explicit --font-path always wins over a bundled name, so a user pointing
// at their own file is never silently overridden.
func resolveFontConfig(family, path string) (resolvedFamily, resolvedPath string, err error) {
	if path != "" {
		return family, path, nil
	}
	if bf, ok := bundledFonts[normalizeFontKey(family)]; ok {
		spilled, err := spillBundledFont(bf)
		if err != nil {
			return "", "", fmt.Errorf("bundled font %q: %w", family, err)
		}
		return bf.family, spilled, nil
	}
	return family, path, nil
}

// normalizeFontKey lowercases and collapses whitespace, so "Sauce Code Pro",
// "sauce  code pro" and "SauceCodePro" all resolve the same way.
func normalizeFontKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// spillBundledFont writes an embedded font's bytes to a file sip's static
// handler can os.ReadFile, once per process.
func spillBundledFont(bf bundledFont) (string, error) {
	dir, err := os.MkdirTemp("", "tuios-web-font-")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, bf.filename)
	if err := os.WriteFile(path, bf.data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
