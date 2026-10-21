package vt

import (
	"slices"
	"testing"
)

// TestUnsettledReadersLeavePendingLines pins that the unsettled readers hand
// the history out as it stands. Reading it with them while a pane is narrow
// must not lay it out there: that trims it to the line limit at the narrow
// width, which is the history a snapshot used to cost the daemon.
func TestUnsettledReadersLeavePendingLines(t *testing.T) {
	e := NewEmulator(80, 12)
	e.SetScrollbackMaxLines(300)
	_, _ = e.WriteString(lazyReflowInput())

	e.Resize(30, 12)
	pending := e.ScrollbackPending()
	if pending == 0 {
		t.Fatal("the resize left nothing pending, so this tests nothing")
	}

	n := e.UnsettledScrollbackLen()
	for i := range n {
		_ = e.UnsettledScrollbackLine(i)
		_ = e.UnsettledScrollbackLineWrap(i)
	}
	laidOut := e.LaidOutScrollbackLen()
	if got := e.ScrollbackPending(); got != pending {
		t.Fatalf("unsettled reads changed the pending lines from %d to %d", pending, got)
	}

	if got := e.ScrollbackLen(); got != laidOut {
		t.Errorf("LaidOutScrollbackLen = %d, but laying the history out left %d lines", laidOut, got)
	}
	if got := e.ScrollbackPending(); got != 0 {
		t.Errorf("%d lines still pending after a settling read", got)
	}
}

// TestNarrowReadKeepsHistoryWithUnsettledReaders takes two emulators fed the
// same output through the same narrow width and back. One is read with the
// unsettled readers while narrow, the other not at all. They must end up
// holding the same history, which is what makes a snapshot taken mid-drag
// harmless to the emulator it is taken from.
func TestNarrowReadKeepsHistoryWithUnsettledReaders(t *testing.T) {
	read := NewEmulator(80, 12)
	untouched := NewEmulator(80, 12)
	for _, e := range []*Emulator{read, untouched} {
		e.SetScrollbackMaxLines(300)
		_, _ = e.WriteString(lazyReflowInput())
		e.Resize(30, 12)
	}

	for i := range read.UnsettledScrollbackLen() {
		_ = read.UnsettledScrollbackLine(i)
	}

	for _, e := range []*Emulator{read, untouched} {
		e.Resize(80, 12)
	}
	if got, want := logicalText(read), logicalText(untouched); !slices.Equal(got, want) {
		t.Errorf("history read unsettled while narrow differs from one never read: %s", suffixDiff(got, want))
	}
}

// TestMarkScrollbackPending pins that rows seeded into a fresh emulator and
// marked pending are laid out at its width on the first read, the way the
// emulator they were copied from would have laid them out.
func TestMarkScrollbackPending(t *testing.T) {
	src := NewEmulator(80, 12)
	src.SetScrollbackMaxLines(300)
	_, _ = src.WriteString(lazyReflowInput())
	src.Resize(40, 12)
	pending := src.ScrollbackPending()
	if pending == 0 {
		t.Fatal("the resize left nothing pending, so this tests nothing")
	}

	dst := NewEmulator(40, 12)
	dst.SetScrollbackMaxLines(300)
	sb := dst.Scrollback()
	for i := range src.UnsettledScrollbackLen() {
		sb.PushLineWrap(src.UnsettledScrollbackLine(i), src.UnsettledScrollbackLineWrap(i))
	}
	dst.MarkScrollbackPending(pending)
	if got := dst.ScrollbackPending(); got != pending {
		t.Fatalf("ScrollbackPending = %d after marking %d", got, pending)
	}

	want := make([]string, 0, src.ScrollbackLen())
	for i := range src.ScrollbackLen() {
		want = append(want, rowText(src.ScrollbackLine(i), src.ScrollbackLineWrap(i)))
	}
	got := make([]string, 0, dst.ScrollbackLen())
	for i := range dst.ScrollbackLen() {
		got = append(got, rowText(dst.ScrollbackLine(i), dst.ScrollbackLineWrap(i)))
	}
	if !slices.Equal(got, want) {
		t.Errorf("seeded history laid out differently from its source: %s", suffixDiff(got, want))
	}

	dst.MarkScrollbackPending(0)
	if got := dst.ScrollbackPending(); got != 0 {
		t.Errorf("MarkScrollbackPending(0) left %d lines pending", got)
	}
}
