package vt

import (
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// readReply returns the next response the emulator writes for a query.
func readReply(t *testing.T, e *Emulator, query string) string {
	t.Helper()
	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, err := e.Read(buf)
		if err != nil && err != io.EOF {
			got <- "error: " + err.Error()
			return
		}
		got <- string(buf[:n])
	}()
	if _, err := e.Write([]byte(query)); err != nil {
		t.Fatalf("Write(%q): %v", query, err)
	}
	select {
	case s := <-got:
		return s
	case <-time.After(2 * time.Second):
		t.Fatalf("no reply to %q", query)
		return ""
	}
}

// TestUnicodeCoreModeMatchesLayout pins DECRQM ?2027 to what the emulator
// actually does. The print path always measures by grapheme cluster, so a
// guest that asks must hear "permanently set" (value 3): reporting "reset" told
// Bubble Tea and friends to measure with wcwidth, which disagrees on VS16 emoji
// and flags, and every cell after one landed a column off. Neither DECSET nor
// DECRST nor a restored snapshot may change the answer, because none of them
// changes the layout.
func TestUnicodeCoreModeMatchesLayout(t *testing.T) {
	want := ansi.ReportMode(ansi.ModeUnicodeCore, ansi.ModePermanentlySet)

	tests := []struct {
		name  string
		setup func(e *Emulator)
	}{
		{"fresh", func(*Emulator) {}},
		{"after DECRST", func(e *Emulator) { _, _ = e.Write([]byte(ansi.ResetModeUnicodeCore)) }},
		{"after DECSET", func(e *Emulator) { _, _ = e.Write([]byte(ansi.SetModeUnicodeCore)) }},
		{"after RestoreModes off", func(e *Emulator) { e.RestoreModes(map[int]bool{2027: false}) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEmulator(20, 2)
			defer func() { _ = e.Close() }()
			tc.setup(e)
			if got := readReply(t, e, ansi.RequestModeUnicodeCore); got != want {
				t.Errorf("DECRQM ?2027 reply = %q, want %q", got, want)
			}
			if got := e.WidthMethod(); got != ansi.GraphemeWidth {
				t.Errorf("WidthMethod() = %v, want GraphemeWidth", got)
			}
			// The layout the reply promises: a VS16 heart is two cells wide.
			_, _ = e.Write([]byte("\r❤️x"))
			if c := e.CellAt(2, 0); c == nil || c.Content != "x" {
				t.Errorf("cell after ❤️ = %+v, want x at column 2", c)
			}
		})
	}
}
