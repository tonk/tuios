package input

import (
	"strings"
	"testing"

	"github.com/tonk/tuios/internal/vt"
)

// TestSearchMatchColumnsAfterClusters checks that a search match is placed on
// the columns the text occupies when it follows a cell holding several runes.
// The extracted line text carries every rune of a cluster (❤️ is a heart plus
// VS16, é may be e plus a combining accent), but the column mapping counted
// one per cell, so the highlight landed a column too far right for every
// extra rune before the match.
func TestSearchMatchColumnsAfterClusters(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		query     string
		wantStart int
		wantEnd   int
	}{
		{"ascii", "xy ab", "ab", 3, 5},
		{"vs16 emoji", "❤️ ab", "ab", 3, 5},
		{"two vs16 emoji", "❤️✔️ab", "ab", 4, 6},
		{"combining accent", "é ab", "ab", 2, 4},
		{"flag", "🇳🇱ab", "ab", 2, 4},
		{"cjk", "中文ab", "ab", 4, 6},
		{"match on the emoji", "x❤️y", "❤️", 1, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := vt.NewEmulator(20, 2)
			defer func() { _ = e.Close() }()
			_, _ = e.Write([]byte(tc.line))

			cells := getScreenLineCells(e, 0)
			text := extractScreenLineText(e, 0)
			if fromCells := extractLineTextFromCells(cells); fromCells != text {
				t.Fatalf("screen and cell extraction disagree: %q vs %q", text, fromCells)
			}
			bytePos := strings.Index(text, tc.query)
			if bytePos < 0 {
				t.Fatalf("query %q not found in %q", tc.query, text)
			}
			charStart := byteIndexToCharIndex(text, bytePos)
			charEnd := charStart + len([]rune(tc.query))
			start := charIndexToColumn(cells, charStart)
			end := charIndexToColumn(cells, charEnd)
			if start != tc.wantStart || end != tc.wantEnd {
				t.Errorf("match columns = [%d,%d), want [%d,%d)", start, end, tc.wantStart, tc.wantEnd)
			}
		})
	}
}
