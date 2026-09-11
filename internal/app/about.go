package app

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/tonk/tuios/internal/overlay"
	"github.com/tonk/tuios/internal/theme"
)

// aboutRepoURL is the project's home, shown as a plain line rather than a
// clickable link: overlay bodies are plain styled text, and most terminals
// already turn a bare URL into a link on their own.
const aboutRepoURL = "github.com/tonk/tuios"

// OpenAbout shows the about overlay.
func (m *OS) OpenAbout() { m.ShowAbout = true }

// CloseAbout hides the about overlay.
func (m *OS) CloseAbout() { m.ShowAbout = false }

// ToggleAbout flips the about overlay, for callers (the command palette, the
// prefix chord) that don't otherwise track its state.
func (m *OS) ToggleAbout() {
	if m.ShowAbout {
		m.CloseAbout()
	} else {
		m.OpenAbout()
	}
}

// RenderAbout renders the about overlay on the shared panel grammar: name and
// tagline, the build version, and the runtime/platform the binary reports -
// the same facts a bug report would ask for.
func (m *OS) RenderAbout() (string, overlay.Geometry) {
	pal := theme.UI()
	bg := pal.Surface
	label := overlay.Style(bg).Foreground(pal.FgDim).Render
	value := overlay.Style(bg).Foreground(pal.Fg).Bold(true).Render

	version := versionLabel()
	if version == "" {
		version = "dev"
	}

	var lines []string
	lines = append(lines,
		overlay.Style(bg).Foreground(pal.Accent).Bold(true).Render("TUIOS")+
			overlay.Style(bg).Foreground(pal.FgDim).Render(" - Terminal UI Operating System"),
		"",
		label("Version:  ")+value(version),
		label("Go:       ")+value(runtime.Version()),
		label("Platform: ")+value(fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)),
		"",
		overlay.Style(bg).Foreground(pal.FgDim).Render(aboutRepoURL),
	)

	width := m.panelWidth(48)
	hints := []overlay.Hint{{Key: "esc", Label: "close"}}
	rows, hints := m.panelBody(len(lines), 0, width, nil, hints)
	panel := overlay.Panel{
		Title: "About",
		Width: width,
		Body:  clipStyledLines(strings.Join(squeezeLines(lines, rows), "\n"), width),
		Hints: hints,
	}
	return panel.Render(pal)
}
