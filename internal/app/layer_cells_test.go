package app

import (
	"testing"

	"charm.land/lipgloss/v2"
)

// TestComposeLayersMatchesCompositor checks that composing through the
// decoded-cell cache paints exactly what lipgloss's Compositor paints on each
// path a layer takes: drawn directly when new, decoded on its second frame, and
// replayed from the cache after that, including after being moved.
func TestComposeLayersMatchesCompositor(t *testing.T) {
	bold := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff8800"))
	boxed := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	link := "\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\"

	contents := []string{
		boxed.Render("pane one\nsecond line"),
		bold.Render("wide 日本語 🙂 text") + "\n" + link,
		"short\n\nlonger line after a blank",
		"",
		"\x1b[44m  blue  \x1b[0m\r\nCRLF line",
	}

	tests := []struct {
		name   string
		layers func() []*lipgloss.Layer
	}{
		{"overlapping", func() []*lipgloss.Layer {
			return []*lipgloss.Layer{
				lipgloss.NewLayer(contents[0]).X(2).Y(1).Z(1),
				lipgloss.NewLayer(contents[1]).X(5).Y(2).Z(2),
				lipgloss.NewLayer(contents[2]).X(0).Y(0).Z(0),
			}
		}},
		{"equal z keeps append order", func() []*lipgloss.Layer {
			return []*lipgloss.Layer{
				lipgloss.NewLayer(contents[0]).X(1).Y(1),
				lipgloss.NewLayer(contents[4]).X(3).Y(2),
				lipgloss.NewLayer(contents[1]).X(2).Y(3),
			}
		}},
		{"clipped and off screen", func() []*lipgloss.Layer {
			return []*lipgloss.Layer{
				lipgloss.NewLayer(contents[0]).X(-3).Y(-1).Z(1),
				lipgloss.NewLayer(contents[1]).X(30).Y(8).Z(2),
				lipgloss.NewLayer(contents[2]).X(100).Y(100).Z(3),
				lipgloss.NewLayer(contents[3]).X(4).Y(4).Z(4),
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &OS{}
			canvas := lipgloss.NewCanvas(40, 10)
			layers := tt.layers()

			for frame := range 3 {
				if frame == 2 {
					// Move a cached layer: its cells must follow it.
					layers[0].X(layers[0].GetX() + 3).Y(layers[0].GetY() + 1)
				}
				want := lipgloss.NewCanvas(40, 10)
				want.Compose(lipgloss.NewCompositor(layers...))

				canvas.Clear()
				m.composeLayers(canvas, layers)
				if got := canvas.Render(); got != want.Render() {
					t.Fatalf("frame %d differs\n got: %q\nwant: %q", frame, got, want.Render())
				}
			}
			if len(m.layerCells) != len(layers) {
				t.Errorf("cache holds %d layers, want %d", len(m.layerCells), len(layers))
			}
		})
	}
}

// TestComposeLayersDropsUnusedLayers checks that a layer no longer composed is
// released from the cache, so a window's replaced CachedLayer does not pin its
// old cells.
func TestComposeLayersDropsUnusedLayers(t *testing.T) {
	m := &OS{}
	canvas := lipgloss.NewCanvas(20, 5)
	old := lipgloss.NewLayer("old")
	kept := lipgloss.NewLayer("kept").Y(1)

	m.composeLayers(canvas, []*lipgloss.Layer{old, kept})
	m.composeLayers(canvas, []*lipgloss.Layer{kept})

	if _, ok := m.layerCells[old]; ok {
		t.Error("layer not composed this frame is still cached")
	}
	if _, ok := m.layerCells[kept]; !ok {
		t.Error("layer composed this frame is not cached")
	}
}
