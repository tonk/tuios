package input

import "testing"

// TestCopyJoinsOnlySoftWrappedRows pins copy mode reading the emulator's wrap
// flags instead of guessing from how far across a row the text reaches. A row
// the output wrapped from is joined with the next, spaces at the edge kept; a
// row that merely ends near the edge keeps its newline.
func TestCopyJoinsOnlySoftWrappedRows(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		// 40 columns: "a"*36 + " bc" wraps after the space at column 39.
		{"soft wrap joins", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa bc", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa bc"},
		{"near the edge keeps the newline", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\r\nbc", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nbc"},
		{"exact fill then newline keeps it", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\r\nbc", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nbc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, win := selectPane(t, tc.input)
			pressAt(o, 0, 0)
			dragTo(o, 1, 1)
			release(o, 1, 1)
			if got := selectedText(win); got != tc.want {
				t.Errorf("copied %q, want %q", got, tc.want)
			}
		})
	}
}
