package vt

import (
	"fmt"
	"strings"
	"testing"
)

// logicalText joins the scrollback and the screen into the logical lines the
// guest wrote: soft-wrapped rows are glued to the next one. Two layouts of the
// same output at the same width agree on it whatever row each line starts on.
func logicalText(e *Emulator) []string {
	rows := append(dumpScrollback(e), dumpScreen(e)...)
	var out []string
	var cur strings.Builder
	for _, r := range rows {
		if strings.HasSuffix(r, "~") {
			cur.WriteString(strings.TrimSuffix(r, "~"))
			continue
		}
		cur.WriteString(r)
		out = append(out, cur.String())
		cur.Reset()
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// lazyReflowInput is output with long and short lines, wide runes and a
// prompt marker deep in the history, enough to fill a small scrollback.
func lazyReflowInput() string {
	var in strings.Builder
	for i := range 400 {
		switch {
		case i == 330:
			fmt.Fprintf(&in, "\x1b]133;A\x07$ marked prompt %d\r\n", i)
		case i%7 == 0:
			fmt.Fprintf(&in, "%03d %s\r\n", i, strings.Repeat("中文 wide ", 9))
		case i%3 == 0:
			fmt.Fprintf(&in, "%03d %s\r\n", i, strings.Repeat("long line ", 12))
		default:
			fmt.Fprintf(&in, "%03d short\r\n", i)
		}
	}
	in.WriteString("$ ")
	return in.String()
}

// TestLazyReflowMatchesReadingEveryStep resizes two emulators through the same
// widths. One reads its history after every step, which settles the reflow
// each time; the other reads only at the end, so its older lines wait out
// every step but the last. Both must end up holding the same history at the
// same width, with the prompt marker on the same text.
//
// The lazy one may hold more of it. Laid out at a narrow width, the history
// takes more rows, and the one settled every step loses its oldest lines to
// the line limit on the way; lines left pending keep their rows until they
// are laid out at the final width. So the eager history must be the end of
// the lazy one, not all of it.
func TestLazyReflowMatchesReadingEveryStep(t *testing.T) {
	steps := []struct {
		name   string
		widths []int
	}{
		{"narrowing", []int{70, 55, 41, 30}},
		{"widening", []int{30, 45, 90, 120}},
		{"back and forth", []int{50, 97, 33, 64, 80, 40}},
	}
	for _, tt := range steps {
		t.Run(tt.name, func(t *testing.T) {
			eager := NewEmulator(80, 12)
			lazy := NewEmulator(80, 12)
			eager.SetScrollbackMaxLines(300)
			lazy.SetScrollbackMaxLines(300)
			for _, e := range []*Emulator{eager, lazy} {
				_, _ = e.WriteString(lazyReflowInput())
			}

			for _, w := range tt.widths {
				eager.Resize(w, 12)
				_ = eager.ScrollbackLen()
				lazy.Resize(w, 12)
				if sb := lazy.scrs[0].scrollback; w != 80 && sb.pending == 0 {
					t.Fatalf("width %d: resize laid out the whole history at once", w)
				}
				checkGrid(t, lazy)
			}

			if d := suffixDiff(logicalText(lazy), logicalText(eager)); d != "" {
				t.Errorf("history differs from the one settled every step: %s", d)
			}
			if p := lazy.scrs[0].scrollback.pending; p != 0 {
				t.Errorf("%d lines still pending after reading the history", p)
			}
			if c, x := lazy.CursorPosition(), eager.CursorPosition(); c != x {
				t.Errorf("cursor = %v, want %v", c, x)
			}
			if got, want := markerText(lazy), markerText(eager); got != want || !strings.Contains(got, "marked prompt") {
				t.Errorf("marker on %q, want %q", got, want)
			}
		})
	}
}

// suffixDiff describes how end fails to be the last lines of all, or returns
// "". The first line of end may be cut short at the front: the line limit
// takes rows, and the oldest logical line can lose its first ones.
func suffixDiff(all, end []string) string {
	if len(end) > len(all) {
		return fmt.Sprintf("%d lines, want at least %d", len(all), len(end))
	}
	off := len(all) - len(end)
	for i, want := range end {
		got := all[off+i]
		if got == want || (i == 0 && strings.HasSuffix(got, want)) {
			continue
		}
		return fmt.Sprintf("line %d of %d: %q, want %q", off+i, len(all), got, want)
	}
	return ""
}

// markerText returns the text of the row the first semantic marker is on.
func markerText(e *Emulator) string {
	ms := e.SemanticMarkers().Markers()
	if len(ms) == 0 {
		return ""
	}
	line := ms[0].AbsLine
	if n := e.ScrollbackLen(); line < n {
		return rowText(e.ScrollbackLine(line), HardBreak)
	} else if y := line - n; y < e.Height() {
		return dumpScreen(e)[y]
	}
	return ""
}

// TestLazyReflowKeepsOutputWrittenBeforeTheRead writes output and pushes
// more lines into the history between a resize and the first read, the way a
// shell redraws its prompt at every step of a drag. Lines pending from the
// resize stay in order in front of the new ones.
func TestLazyReflowKeepsOutputWrittenBeforeTheRead(t *testing.T) {
	e := NewEmulator(40, 5)
	e.SetScrollbackMaxLines(50)
	for i := range 40 {
		_, _ = fmt.Fprintf(e, "line %02d %s\r\n", i, strings.Repeat("x", 30))
	}
	e.Resize(20, 5)
	for i := 40; i < 45; i++ {
		_, _ = fmt.Fprintf(e, "line %02d\r\n", i)
	}

	got := logicalText(e)
	var want []string
	for i := range 45 {
		if i < 40 {
			want = append(want, fmt.Sprintf("line %02d %s", i, strings.Repeat("x", 30)))
		} else {
			want = append(want, fmt.Sprintf("line %02d", i))
		}
	}
	// The ring holds 50 rows; at 20 columns the long lines take two each, so
	// the oldest are gone. What is left must be the newest, in order.
	if len(got) == 0 || got[len(got)-1] != want[len(want)-1] {
		t.Fatalf("history ends %q, want %q", got[len(got)-1:], want[len(want)-1])
	}
	if d := suffixDiff(want, got); d != "" {
		t.Errorf("history is not the newest lines in order: %s", d)
	}
	if e.ScrollbackLen() > 50 {
		t.Errorf("scrollback holds %d lines, limit 50", e.ScrollbackLen())
	}
}

// BenchmarkReflowSettle is the read that follows a resize of a full history:
// the one-off cost of laying out the lines a resize left pending.
func BenchmarkReflowSettle(b *testing.B) {
	e := NewEmulator(200, 50)
	var in strings.Builder
	for i := range DefaultScrollbackSize + 50 {
		if i%5 == 0 {
			fmt.Fprintf(&in, "%05d %s\r\n", i, strings.Repeat("wrapped text ", 40))
		} else {
			fmt.Fprintf(&in, "%05d short line\r\n", i)
		}
	}
	_, _ = e.WriteString(in.String())
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		e.Resize(200-(i+1)%2, 50)
		_ = e.ScrollbackLen()
	}
}

// TestLazyReflowSettlesOnceForConcurrentReaders has several readers ask for
// the history at once after a resize, as readers sharing the window's IO lock
// can. One settles; the others wait for it and see the finished layout.
func TestLazyReflowSettlesOnceForConcurrentReaders(t *testing.T) {
	e := NewEmulator(80, 12)
	_, _ = e.WriteString(lazyReflowInput())
	e.Resize(37, 12)

	const readers = 8
	lens := make(chan int, readers)
	start := make(chan struct{})
	for range readers {
		go func() {
			<-start
			n := e.ScrollbackLen()
			for i := range n {
				_ = e.ScrollbackLine(i)
			}
			lens <- n
		}()
	}
	close(start)
	first := <-lens
	for range readers - 1 {
		if n := <-lens; n != first {
			t.Fatalf("readers saw %d and %d lines", first, n)
		}
	}
	if p := e.scrs[0].scrollback.pending; p != 0 {
		t.Errorf("%d lines still pending", p)
	}
}
