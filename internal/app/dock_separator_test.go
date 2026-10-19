package app

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/tonk/tuios/internal/config"
)

// TestDockSeparatorFollowsItsGlyph: the hairline cache was keyed on width
// alone, so a change of separator character at an unchanged width kept serving
// the old glyph until the next resize.
func TestDockSeparatorFollowsItsGlyph(t *testing.T) {
	m := newNarrowOS(t, 80, 24)

	dock, _ := m.renderDockString()
	if !strings.Contains(dock, config.WindowSeparatorChar) {
		t.Fatalf("the dock hairline is not the separator character: %q", dock)
	}

	prev := config.UseASCIIOnly
	config.UseASCIIOnly = true
	t.Cleanup(func() { config.UseASCIIOnly = prev })

	dock, _ = m.renderDockString()
	if strings.Contains(dock, config.WindowSeparatorChar) {
		t.Errorf("the dock is still drawing the stale hairline glyph: %q", dock)
	}
	if !strings.Contains(dock, strings.Repeat(config.WindowSeparatorCharASCII, 10)) {
		t.Errorf("the dock hairline did not follow the ASCII separator: %q", dock)
	}
}

// TestDockSeparatorOffGivesItsRowToThePanes pins what dock_separator = false is
// for: the dock shrinks to the bar alone and the panes get the hairline's row.
// The bar has to land on the screen's edge row too, at either end, or the row
// is freed in the arithmetic only and the panes still lose it to a gap.
func TestDockSeparatorOffGivesItsRowToThePanes(t *testing.T) {
	prevSep, prevPos := config.ShowDockSeparator, config.DockbarPosition
	t.Cleanup(func() { config.ShowDockSeparator, config.DockbarPosition = prevSep, prevPos })

	const height = 24
	for _, tc := range []struct {
		pos       string
		separator bool
		rows, y   int
	}{
		{"bottom", true, 2, height - 2},
		{"bottom", false, 1, height - 1},
		{"top", true, 2, 0},
		{"top", false, 1, 0},
	} {
		config.DockbarPosition, config.ShowDockSeparator = tc.pos, tc.separator
		m := newNarrowOS(t, 80, height)
		if got, want := m.GetUsableHeight(), height-tc.rows; got != want {
			t.Errorf("%s, separator %v: usable height %d, want %d", tc.pos, tc.separator, got, want)
		}
		dock, y := m.renderDockString()
		if got := lipgloss.Height(dock); got != tc.rows {
			t.Errorf("%s, separator %v: dock is %d rows, want %d", tc.pos, tc.separator, got, tc.rows)
		}
		if y != tc.y {
			t.Errorf("%s, separator %v: dock at row %d, want %d", tc.pos, tc.separator, y, tc.y)
		}
	}
}
