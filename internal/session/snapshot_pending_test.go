package session

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tonk/tuios/internal/vt"
)

// TestSnapshotLeavesPendingHistory pins that taking a snapshot of a pane
// while it is narrow costs it no history. A snapshot used to read the
// scrollback with the settling readers, which laid out the lines a resize left
// pending at the narrow width and dropped what did not fit the line limit
// there; a client watching the same pane kept them, and the two copies
// disagreed about old history from then on (docs/REHYDRATION.md).
//
// The snapshot now carries the pending rows as they stand, and a fresh
// emulator seeded from it lays them out at its own width when it is read, so
// it ends up holding what the source holds.
func TestSnapshotLeavesPendingHistory(t *testing.T) {
	const wide, narrow, rows, limit = 80, 12, 10, 200

	feed := func(e *vt.Emulator) {
		for i := range 150 {
			_, _ = fmt.Fprintf(e, "line %03d %s\r\n", i, strings.Repeat("x", 100))
		}
	}
	history := func(e *vt.Emulator) []string {
		out := make([]string, 0, e.ScrollbackLen())
		for i := range e.ScrollbackLen() {
			out = append(out, strings.TrimRight(e.ScrollbackLine(i).String(), " "))
		}
		return out
	}

	src := vt.NewEmulator(wide, rows)
	ref := vt.NewEmulator(wide, rows)
	for _, e := range []*vt.Emulator{src, ref} {
		e.SetScrollbackMaxLines(limit)
		feed(e)
		e.Resize(narrow, rows)
	}
	if src.ScrollbackPending() == 0 {
		t.Fatal("the resize left nothing pending, so this tests nothing")
	}

	// Taken while narrow, the moment a drag over a producing pane offers.
	state := TerminalStateOf(src, narrow, rows, limit)
	if state.ScrollbackPending == 0 {
		t.Fatal("the snapshot did not carry the pending rows as pending")
	}
	if got := src.ScrollbackPending(); got == 0 {
		t.Fatal("taking the snapshot laid out the source's pending history")
	}

	// The seeded copy, read at the snapshot's width, holds what the source
	// holds once that is read at the same width.
	dst := vt.NewEmulator(narrow, rows)
	dst.SetScrollbackMaxLines(limit)
	ApplyTerminalState(dst, state)
	if got, want := history(dst), history(src); !slices.Equal(got, want) {
		t.Errorf("seeded history has %d lines, source has %d, and they differ", len(got), len(want))
	}
	if got, want := state.ScrollbackLen, src.ScrollbackLen(); got != want {
		t.Errorf("snapshot ScrollbackLen = %d, laid-out history has %d lines", got, want)
	}

	// Back at full width, a pane snapshotted while narrow must hold what one
	// never snapshotted holds: the snapshot cost it nothing. src was read
	// above with the settling readers, so this takes a pane of its own.
	ref.Resize(wide, rows)
	src2 := vt.NewEmulator(wide, rows)
	src2.SetScrollbackMaxLines(limit)
	feed(src2)
	src2.Resize(narrow, rows)
	_ = TerminalStateOf(src2, narrow, rows, limit)
	src2.Resize(wide, rows)
	if got, want := history(src2), history(ref); !slices.Equal(got, want) {
		t.Errorf("a snapshot taken while narrow changed the history: %d lines, want %d", len(got), len(want))
	}
}
