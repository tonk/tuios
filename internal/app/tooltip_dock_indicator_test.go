package app

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/theme"
)

// dockIndicatorOS is dockSessionOS with all three mode-indicator glyphs
// turned on, for tests of their hover tooltip.
func dockIndicatorOS(t testing.TB) *OS {
	t.Helper()
	m := dockSessionOS(t, 160, false)

	prevMouse, prevTiling, prevFFM := config.ShowMouseIndicator, config.ShowTilingIndicator, config.ShowFocusFollowsMouseIndicator
	config.ShowMouseIndicator, config.ShowTilingIndicator, config.ShowFocusFollowsMouseIndicator = true, true, true
	t.Cleanup(func() {
		config.ShowMouseIndicator, config.ShowTilingIndicator, config.ShowFocusFollowsMouseIndicator = prevMouse, prevTiling, prevFFM
	})
	return m
}

// indicatorTooltipAt hovers a mode-indicator glyph and returns the label
// layer it drew. The delay is faked rather than waited out, the same way
// dockTooltipAt fakes the session controls' delay: what's under test is what
// the frame does once the delay has elapsed, not the delay itself.
func indicatorTooltipAt(t *testing.T, m *OS, kind DockIndicatorKind) *lipgloss.Layer {
	t.Helper()
	m.renderDockString() // the rects a hover is tested against are recorded as they are drawn
	for _, h := range m.dockIndicatorHits {
		if h.Kind != kind {
			continue
		}
		if !m.DockIndicatorHoverAt(h.X0, h.Y) {
			t.Fatalf("the pointer on column %d of indicator %v was not on it", h.X0, kind)
		}
		if !m.TooltipPending() {
			t.Fatalf("hovering indicator %v armed no label", kind)
		}
		m.Tooltip.At = time.Now().Add(-2 * tooltipDelay)
		return m.renderTooltip()
	}
	t.Fatalf("the dock drew no glyph for indicator %v", kind)
	return nil
}

// TestDockIndicatorTooltipNamesTheMode is the whole trade: the glyph gave up
// its words for a single high-contrast/dull color, so hovering has to be able
// to say them again.
func TestDockIndicatorTooltipNamesTheMode(t *testing.T) {
	for _, tc := range []struct {
		kind DockIndicatorKind
		want string
	}{
		{DockIndicatorMouse, "Mouse mode"},
		{DockIndicatorTiling, "Tiling"},
		{DockIndicatorFocusFollowsMouse, "Focus follows mouse"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			m := dockIndicatorOS(t)
			layer := indicatorTooltipAt(t, m, tc.kind)
			if layer == nil {
				t.Fatal("the hover drew no label")
			}
			got := stripANSIForTrace(layer.GetContent())
			if !strings.Contains(got, tc.want) {
				t.Errorf("the label reads %q, want it to name %q", got, tc.want)
			}
		})
	}
}

// TestDockIndicatorGlyphsDoNotOverlap: three glyphs share one info block, so a
// bad width calculation could draw one span on top of a neighbor's cells,
// which would make hovering one glyph report another's tooltip.
func TestDockIndicatorGlyphsDoNotOverlap(t *testing.T) {
	m := dockIndicatorOS(t)
	m.renderDockString()
	if len(m.dockIndicatorHits) != 3 {
		t.Fatalf("got %d indicator hits, want 3", len(m.dockIndicatorHits))
	}
	sorted := append([]dockIndicatorHit{}, m.dockIndicatorHits...)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].X1 > sorted[j].X0 && sorted[j].X1 > sorted[i].X0 {
				t.Errorf("indicator %v (%d-%d) overlaps indicator %v (%d-%d)",
					sorted[i].Kind, sorted[i].X0, sorted[i].X1,
					sorted[j].Kind, sorted[j].X0, sorted[j].X1)
			}
		}
	}
}

// TestDockIndicatorGlyphColorReflectsState: active reads high-contrast
// (pal.Success by default), inactive reads dull (pal.FgMute), so a glance at
// the color alone answers "is this mode on" without the tooltip.
func TestDockIndicatorGlyphColorReflectsState(t *testing.T) {
	m := dockIndicatorOS(t)
	dr := currentDockRow(theme.UI())
	activeGlyph := m.dockIndicatorGlyph(dr, "X", true, DockIndicatorMouse)
	inactiveGlyph := m.dockIndicatorGlyph(dr, "X", false, DockIndicatorMouse)
	if activeGlyph == inactiveGlyph {
		t.Fatal("active and inactive glyphs rendered identically")
	}
}

// TestDockIndicatorTooltipClearsOnTheWayOut: moving off every glyph drops the
// hover, the same as the session controls and the workspace strip do.
func TestDockIndicatorTooltipClearsOnTheWayOut(t *testing.T) {
	m := dockIndicatorOS(t)
	m.renderDockString()
	if len(m.dockIndicatorHits) == 0 {
		t.Fatal("the dock drew no indicator glyphs")
	}
	h := m.dockIndicatorHits[0]
	if !m.DockIndicatorHoverAt(h.X0, h.Y) {
		t.Fatal("hovering the glyph reported no hit")
	}
	if m.Tooltip.Source != tooltipDockIndicator {
		t.Fatal("hovering the glyph did not arm the indicator tooltip")
	}
	if m.DockIndicatorHoverAt(0, h.Y+50) {
		t.Fatal("hovering off the dock reported a hit")
	}
	if m.Tooltip.Source == tooltipDockIndicator {
		t.Fatal("moving off the glyph left the tooltip armed")
	}
}
