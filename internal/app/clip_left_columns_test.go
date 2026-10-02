package app

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestClipLeftCountsColumns pins the left-edge clip to display columns. It
// used to count one position per rune, so "中中中ABCDEF|" (13 columns) clipped
// by three columns lost three runes, six columns, and came out 7 wide instead
// of 10, and an accented letter written as base plus combining mark lost its
// accent separately from its base.
func TestClipLeftCountsColumns(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		x     int
		want  string // visible text after the clip, escapes stripped
		width int
	}{
		{"cjk cut through the middle of a wide char", "中中中ABCDEF|", -3, " 中ABCDEF|", 10},
		{"cjk cut on a boundary", "中中中ABCDEF|", -4, "中ABCDEF|", 9},
		{"ascii", "abcdef", -2, "cdef", 4},
		{"combining mark stays with its base", "e\u0301e\u0301xyz", -1, "e\u0301xyz", 4},
		{"combining mark cut with its base", "e\u0301e\u0301xyz", -2, "xyz", 3},
		{"vs16 emoji counts two", "❤️ab", -2, "ab", 2},
		{"vs16 emoji cut in half", "❤️ab", -1, " ab", 3},
		{"flag counts two", "🇳🇱ab", -1, " ab", 3},
		{"styles survive the clip", "\x1b[31m中中\x1b[1mAB\x1b[0m", -2, "中AB", 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, finalX, _ := clipWindowContent(tc.line, tc.x, 0, 80, 24)
			if finalX != 0 {
				t.Fatalf("finalX = %d, want 0", finalX)
			}
			if got := ansi.Strip(out); got != tc.want {
				t.Errorf("clipped text = %q, want %q", got, tc.want)
			}
			if got := ansi.StringWidth(out); got != tc.width {
				t.Errorf("clipped width = %d, want %d", got, tc.width)
			}
		})
	}
}

// TestSkipColumnsKeepsEscapes: sequences before the cut still reach the
// output, so the pen and hyperlink in force at the edge are the ones the
// visible part was written under.
func TestSkipColumnsKeepsEscapes(t *testing.T) {
	in := "\x1b[31mab\x1b]8;;https://x.test\x1b\\cd\x1b]8;;\x1b\\ef"
	got := skipColumns(in, 3)
	want := "\x1b[31m\x1b]8;;https://x.test\x1b\\d\x1b]8;;\x1b\\ef"
	if got != want {
		t.Errorf("skipColumns = %q, want %q", got, want)
	}
}
