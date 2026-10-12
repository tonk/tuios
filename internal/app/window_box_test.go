package app

import (
	"image/color"
	"math/rand/v2"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// boxBorders are the borders config.GetBorderForStyle can hand the renderer.
var boxBorders = map[string]lipgloss.Border{
	"rounded":          lipgloss.RoundedBorder(),
	"normal":           lipgloss.NormalBorder(),
	"thick":            lipgloss.ThickBorder(),
	"double":           lipgloss.DoubleBorder(),
	"hidden":           lipgloss.HiddenBorder(),
	"block":            lipgloss.BlockBorder(),
	"outer-half-block": lipgloss.OuterHalfBlockBorder(),
	"inner-half-block": lipgloss.InnerHalfBlockBorder(),
	"ascii":            lipgloss.ASCIIBorder(),
}

// randomPaneContent builds pane-like content: SGR runs left open across line
// ends (as the focused path can leave them), hyperlinks, wide graphemes,
// hyphens and spaces where a word wrapper could act, and ragged line ends.
func randomPaneContent(rng *rand.Rand, maxWidth, maxLines int) string {
	pieces := []string{"word", "a", "-", " ", "  ", "日本", "🙂", "x-y", "é", "│", "⏺"}
	sgr := []string{"\x1b[1m", "\x1b[31m", "\x1b[38;2;10;200;30m", "\x1b[44m", "\x1b[0m", "\x1b[m", "\x1b[4:3m"}
	lines := make([]string, rng.IntN(maxLines+1))
	for i := range lines {
		var b strings.Builder
		width := 0
		for range rng.IntN(12) {
			switch rng.IntN(5) {
			case 0:
				b.WriteString(sgr[rng.IntN(len(sgr))])
			case 1:
				if rng.IntN(4) == 0 {
					b.WriteString("\x1b]8;;https://example.com\x1b\\")
				} else {
					b.WriteString("\x1b]8;;\x1b\\")
				}
			default:
				p := pieces[rng.IntN(len(pieces))]
				if pw := lipgloss.Width(p); width+pw <= maxWidth {
					b.WriteString(p)
					width += pw
				}
			}
		}
		lines[i] = b.String()
	}
	return strings.Join(lines, "\n")
}

// TestBoxSidesMatchesLipgloss checks that, wherever the fast path takes a box,
// it hands addToBorder exactly what lipgloss's box would: the same lines above
// the bottom edge and the same width.
func TestBoxSidesMatchesLipgloss(t *testing.T) {
	colors := []color.Color{
		lipgloss.Color("#5f87ff"),
		lipgloss.Color("4"),
		lipgloss.Color("208"),
	}
	fast := 0
	for seed := range uint64(3000) {
		rng := rand.New(rand.NewPCG(seed, 0x626f78))
		width, height := 3+rng.IntN(30), 2+rng.IntN(12)
		contentW, contentH := width-2, height-2
		// Now and then content that overflows, which the fast path must refuse.
		maxW, maxLines := contentW, contentH
		if rng.IntN(5) == 0 {
			maxW, maxLines = contentW+3, contentH+2
		}
		content := randomPaneContent(rng, maxW, maxLines)
		names := []string{"rounded", "normal", "thick", "double", "hidden", "block", "outer-half-block", "inner-half-block", "ascii"}
		name := names[rng.IntN(len(names))]
		border := boxBorders[name]
		fg := colors[rng.IntN(len(colors))]

		sides, chromeWidth, ok := boxSides(content, contentW, contentH, width, height-1, border, fg)
		if !ok {
			continue
		}
		fast++

		box := lipglossWindowBox(content, contentW, contentH, width, height-1, border, fg)
		lines := strings.Split(box, "\n")
		want := lines[:len(lines)-1]
		wantWidth := max(lipgloss.Width(box)-2, 0)
		if chromeWidth != wantWidth {
			t.Fatalf("seed %d (%s %dx%d): width %d, want %d", seed, name, width, height, chromeWidth, wantWidth)
		}
		if len(sides) != len(want) {
			t.Fatalf("seed %d (%s %dx%d): %d lines, want %d\ncontent: %q", seed, name, width, height, len(sides), len(want), content)
		}
		for i := range want {
			if sides[i] != want[i] {
				t.Fatalf("seed %d (%s %dx%d) line %d differs\ncontent: %q\n got: %q\nwant: %q", seed, name, width, height, i, content, sides[i], want[i])
			}
		}
	}
	if fast < 1500 {
		t.Errorf("fast path taken for only %d of 3000 boxes", fast)
	}
}

// TestRenderWindowBoxMatchesLipgloss renders real panes both ways: the
// renderer's output, fast path or not, must equal the lipgloss box with the
// window chrome added.
func TestRenderWindowBoxMatchesLipgloss(t *testing.T) {
	for _, n := range []int{1, 4, 9} {
		m := benchOS(t, n)
		for i, w := range m.Windows {
			w.Tiled = false
			color := lipgloss.Color("#ff8800")
			content := m.renderTerminal(w, i == 0, m.Mode == TerminalMode)
			want := addToBorder(
				lipglossWindowBox(content, w.ContentWidth(), w.ContentHeight(), w.Width, w.Height-1, getBorder(), color),
				color, w, m.workspacePosition(w), m.AutoTiling)
			if got := m.renderWindowBox(w, i, i == 0, color); got != want {
				t.Fatalf("%d windows, window %d: box differs\n got: %q\nwant: %q", n, i, got, want)
			}
			if _, _, ok := boxSides(content, w.ContentWidth(), w.ContentHeight(), w.Width, w.Height-1, getBorder(), color); !ok {
				t.Errorf("%d windows, window %d: fast path not taken for a fitting pane", n, i)
			}
		}
	}
}
