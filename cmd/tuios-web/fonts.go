package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SauceCodePro Nerd Font Mono, SIL OFL 1.1 licensed (see fonts/SauceCodePro-LICENSE.txt),
// bundled as a second selectable font alongside sip's built-in JetBrains Mono
// Nerd Font. Only the Regular weight: sip's custom-font @font-face declares a
// single file across the whole weight range (font-weight: 100 900), the same
// as any other --font-path, so a Bold/Italic file would never be selected.
//
//go:embed fonts/SauceCodeProNerdFontMono-Regular.ttf
var sauceCodeProFont []byte

// bundledFonts maps a normalized --font-family selector to the CSS name it
// renders under. Checked against normalizeFontKey(webFontFamily); anything
// else is passed through to sip as a plain CSS font-family name (see
// resolveFontConfig).
var bundledFonts = map[string]string{
	"saucecodepro":         "SauceCodePro NFM",
	"sauce code pro":       "SauceCodePro NFM",
	"saucecodepro nfm":     "SauceCodePro NFM",
	"sauce code pro nfm":   "SauceCodePro NFM",
	"saucecodeprofontmono": "SauceCodePro NFM",
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
	if canonical, ok := bundledFonts[normalizeFontKey(family)]; ok {
		spilled, err := spillBundledFont()
		if err != nil {
			return "", "", fmt.Errorf("bundled font %q: %w", family, err)
		}
		return canonical, spilled, nil
	}
	return family, path, nil
}

// normalizeFontKey lowercases and collapses whitespace, so "Sauce Code Pro",
// "sauce  code pro" and "SauceCodePro" all resolve the same way.
func normalizeFontKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// spillBundledFont writes the embedded SauceCodePro font to a file sip's
// static handler can os.ReadFile, once per process.
func spillBundledFont() (string, error) {
	dir, err := os.MkdirTemp("", "tuios-web-font-")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "SauceCodeProNerdFontMono-Regular.ttf")
	if err := os.WriteFile(path, sauceCodeProFont, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
