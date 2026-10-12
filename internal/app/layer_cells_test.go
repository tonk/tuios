package app

import (
	"math/rand/v2"
	"slices"
	"strings"
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

				m.composeLayers(canvas, layers)
				if got := m.renderComposed(canvas); got != want.Render() {
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

// randomLayerContent builds a few lines mixing narrow, wide and emoji
// graphemes, styled runs, blank lines and ragged ends: the shapes that decide
// whether a layer's cells break up the wide cells of the layer beneath it.
func randomLayerContent(rng *rand.Rand) string {
	pieces := []string{"a", "bc", " ", "  ", "日", "本語", "🙂", "x日", "é", "e\u0301", "\u200b"}
	styles := []lipgloss.Style{
		lipgloss.NewStyle(),
		lipgloss.NewStyle().Bold(true),
		lipgloss.NewStyle().Background(lipgloss.Color("#204060")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Underline(true),
	}
	lines := make([]string, 1+rng.IntN(5))
	for i := range lines {
		var b strings.Builder
		for range rng.IntN(8) {
			b.WriteString(styles[rng.IntN(len(styles))].Render(pieces[rng.IntN(len(pieces))]))
		}
		lines[i] = b.String()
	}
	return strings.Join(lines, "\n")
}

// TestComposeLayersRandomFrames composes random frames, keeping, moving,
// replacing, dropping and reordering layers between them as a running session
// does, and checks every frame, and its rendered string, against a full
// compose by lipgloss's Compositor.
func TestComposeLayersRandomFrames(t *testing.T) {
	const width, height = 24, 10
	for seed := range uint64(300) {
		rng := rand.New(rand.NewPCG(seed, 0x7475696f73))
		newLayer := func() *lipgloss.Layer {
			return lipgloss.NewLayer(randomLayerContent(rng)).
				X(rng.IntN(width+6) - 4).Y(rng.IntN(height+4) - 2).Z(rng.IntN(4))
		}

		m := &OS{}
		w, h := width, height
		canvas := lipgloss.NewCanvas(w, h)
		layers := make([]*lipgloss.Layer, 1+rng.IntN(6))
		for i := range layers {
			layers[i] = newLayer()
		}

		for frame := range 12 {
			if frame > 0 {
				for i := range layers {
					switch rng.IntN(12) {
					case 0:
						layers[i] = newLayer()
					case 1:
						layers[i].X(layers[i].GetX() + rng.IntN(5) - 2).Y(layers[i].GetY() + rng.IntN(3) - 1)
					case 2:
						layers[i].Z(rng.IntN(4))
					}
				}
				switch rng.IntN(4) {
				case 0:
					layers = append(layers, newLayer())
				case 1:
					if len(layers) > 1 {
						layers = slices.Delete(layers, 0, 1)
					}
				}
			}

			// Now and then the screen is resized, which recomposes it whole.
			if rng.IntN(10) == 0 {
				w, h = width-rng.IntN(4), height-rng.IntN(3)
				canvas.Resize(w, h)
			}

			want := lipgloss.NewCanvas(w, h)
			want.Compose(lipgloss.NewCompositor(layers...))

			// Most frames change nothing or one layer; the canvas keeps the
			// previous frame, so only damaged rows are redrawn.
			m.composeLayers(canvas, layers)
			if got := canvas.Render(); got != want.Render() {
				t.Fatalf("seed %d frame %d: canvas differs\n got: %q\nwant: %q", seed, frame, got, want.Render())
			}
			if got := m.renderComposed(canvas); got != want.Render() {
				t.Fatalf("seed %d frame %d: rendered rows differ\n got: %q\nwant: %q", seed, frame, got, want.Render())
			}
		}
	}
}
