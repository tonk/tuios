package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/terminal"
)

// withSetTerminalTitle sets config.SetTerminalTitle for one test and restores it.
func withSetTerminalTitle(t *testing.T, v bool) {
	t.Helper()
	prev := config.SetTerminalTitle
	config.SetTerminalTitle = v
	t.Cleanup(func() { config.SetTerminalTitle = prev })
}

// TestApplyTerminalTitleWritesOSC2 is the option on: the host terminal gets an
// OSC 2 sequence naming tuios, through the same passthrough a guest's OSC 9
// notification already reaches the host by.
func TestApplyTerminalTitleWritesOSC2(t *testing.T) {
	withSetTerminalTitle(t, true)

	var buf bytes.Buffer
	m := &OS{KittyPassthrough: NewKittyPassthroughWithOptions(KittyPassthroughOptions{Output: &buf})}
	m.applyTerminalTitle()

	got := buf.String()
	if !strings.Contains(got, "\x1b]2;tuios\x07") {
		t.Errorf("host terminal write = %q, want it to contain the OSC 2 title sequence for %q", got, "tuios")
	}
}

// TestApplyTerminalTitleOffWritesNothing is the option off: the host
// terminal's title is left exactly as it was.
func TestApplyTerminalTitleOffWritesNothing(t *testing.T) {
	withSetTerminalTitle(t, false)

	var buf bytes.Buffer
	m := &OS{KittyPassthrough: NewKittyPassthroughWithOptions(KittyPassthroughOptions{Output: &buf})}
	m.applyTerminalTitle()

	if buf.Len() != 0 {
		t.Errorf("set_terminal_title = false still wrote %q to the host terminal", buf.String())
	}
}

// TestApplyTerminalTitleNoPassthroughIsSafe guards the nil case: a caller that
// never enabled graphics passthrough (not true in any real entrypoint today,
// but not something Init should be able to crash over) must not panic.
func TestApplyTerminalTitleNoPassthroughIsSafe(t *testing.T) {
	withSetTerminalTitle(t, true)
	m := &OS{}
	m.applyTerminalTitle()
}

// TestSyncHostTitleFollowsFocusedWindow is the reported bug: the host title
// must track whatever the focused pane has titled itself, not stay pinned to
// "tuios" forever.
func TestSyncHostTitleFollowsFocusedWindow(t *testing.T) {
	withSetTerminalTitle(t, true)

	win := &terminal.Window{ID: "w1"}
	win.SetTitle("claude - working")
	var buf bytes.Buffer
	m := &OS{
		KittyPassthrough: NewKittyPassthroughWithOptions(KittyPassthroughOptions{Output: &buf}),
		Windows:          []*terminal.Window{win},
		FocusedWindow:    0,
	}

	m.syncHostTitle()
	if got := buf.String(); !strings.Contains(got, "\x1b]2;claude - working\x07") {
		t.Errorf("host terminal write = %q, want it to contain the focused pane's title", got)
	}

	// No change: no repeat write.
	buf.Reset()
	m.syncHostTitle()
	if buf.Len() != 0 {
		t.Errorf("unchanged title still wrote %q to the host terminal", buf.String())
	}

	// Title drifts: exactly one new write, with the new title.
	win.SetTitle("claude - done")
	buf.Reset()
	m.syncHostTitle()
	if got := buf.String(); !strings.Contains(got, "\x1b]2;claude - done\x07") {
		t.Errorf("host terminal write = %q, want the drifted title", got)
	}
}

// TestSyncHostTitleStripsDecorative covers the bug this guards against: a
// guest program's raw OSC-set title (Claude Code's own spinner glyph is the
// known real-world case, see printableRune) reaching the host terminal
// unfiltered, which shows up as a tofu box in the host's own window/tab
// title for anyone whose font lacks that glyph - even though the exact same
// title is already laundered through printableTitle everywhere tuios draws
// it in its own chrome (the per-pane title bar, the rail, the palette).
func TestSyncHostTitleStripsDecorative(t *testing.T) {
	withSetTerminalTitle(t, true)

	win := &terminal.Window{ID: "w1"}
	win.SetTitle("✳ Claude Code")
	var buf bytes.Buffer
	m := &OS{
		KittyPassthrough: NewKittyPassthroughWithOptions(KittyPassthroughOptions{Output: &buf}),
		Windows:          []*terminal.Window{win},
		FocusedWindow:    0,
	}

	m.syncHostTitle()
	got := buf.String()
	if strings.ContainsRune(got, '✳') {
		t.Errorf("host terminal write = %q, want the spinner glyph stripped", got)
	}
	if !strings.Contains(got, "Claude Code") {
		t.Errorf("host terminal write = %q, want the rest of the title kept", got)
	}
}

// TestSyncHostTitleFallsBackWithNoFocus covers an empty workspace (no focused
// window) and a focused window that has not set a title yet: both must show
// the "tuios" fallback rather than carrying over a stale title.
func TestSyncHostTitleFallsBackWithNoFocus(t *testing.T) {
	withSetTerminalTitle(t, true)

	var buf bytes.Buffer
	m := &OS{
		KittyPassthrough: NewKittyPassthroughWithOptions(KittyPassthroughOptions{Output: &buf}),
		FocusedWindow:    -1,
	}
	m.syncHostTitle()
	if got := buf.String(); !strings.Contains(got, "\x1b]2;tuios\x07") {
		t.Errorf("host terminal write = %q, want the tuios fallback", got)
	}
}
