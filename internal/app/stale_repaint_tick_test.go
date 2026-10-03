package app

import (
	"testing"
	"time"
)

// TestTickRepaintsWindowLeftDirty covers a pane that was served a stale frame
// (a busy pane shed, a synchronized update held, a blank frame) and whose guest
// then went quiet: with nothing else asking for a frame, the idle tick must
// still draw it, a bounded number of times.
func TestTickRepaintsWindowLeftDirty(t *testing.T) {
	win := newTestWindow(t, "stale-tick-0001", 60, 20)
	m := newTestOS(win)
	m.Width, m.Height = 120, 40
	win.ContentDirty = false // a new window starts dirty

	m.Update(TickerMsg(time.Now()))
	if !m.renderSkipped {
		t.Fatal("setup: an idle tick with a clean window should skip rendering")
	}

	win.ContentDirty = true
	for i := range maxStaleRepaintTicks {
		m.Update(TickerMsg(time.Now()))
		if m.renderSkipped {
			t.Fatalf("tick %d skipped rendering although a visible window is still dirty", i+1)
		}
	}
	m.Update(TickerMsg(time.Now()))
	if !m.renderSkipped {
		t.Error("a window that stays dirty kept the tick rendering past its bound")
	}

	win.ContentDirty = false
	m.Update(TickerMsg(time.Now()))
	if !m.renderSkipped || m.staleRepaintTicks != 0 {
		t.Errorf("a clean window should reset the bound: skipped=%v ticks=%d", m.renderSkipped, m.staleRepaintTicks)
	}
}
