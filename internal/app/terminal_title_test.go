package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/config"
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
