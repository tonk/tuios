package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/tonk/tuios/internal/vt"
)

// TestIngestPTYOutput_SignalsAfterWrite pins the order the local PTY reader
// publishes a chunk in: into the emulator first, and only then the "new output"
// flag and the PTYDataChan wakeup. The UI renders on that wakeup, so a signal
// that ran ahead of the write let a frame consume it and draw the emulator
// without the chunk, and the last chunk of a burst stayed off screen until
// something else asked for a frame.
//
// The test holds the read side of ioMu, the lock a render holds, so the write
// cannot land. A reader that signals first is caught red-handed: the wakeup
// arrives while the bytes are provably not in the emulator yet.
func TestIngestPTYOutput_SignalsAfterWrite(t *testing.T) {
	w := &Window{
		ID:          "signal-order",
		Terminal:    vt.NewEmulator(40, 5),
		PTYDataChan: make(chan struct{}, 1),
	}

	w.RLockIO()
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.ingestPTYOutput([]byte("burst-tail"))
	}()

	select {
	case <-w.PTYDataChan:
		w.RUnlockIO()
		t.Fatal("PTYDataChan was signalled while the chunk could not have been written yet")
	case <-time.After(50 * time.Millisecond):
	}
	if w.HasNewOutput.Load() {
		w.RUnlockIO()
		t.Fatal("HasNewOutput was set while the chunk could not have been written yet")
	}
	w.RUnlockIO()

	select {
	case <-w.PTYDataChan:
	case <-time.After(5 * time.Second):
		t.Fatal("PTYDataChan was never signalled")
	}
	<-done

	if !w.HasNewOutput.Load() {
		t.Fatal("HasNewOutput not set after the chunk was written")
	}
	w.RLockIO()
	got := w.Terminal.String()
	w.RUnlockIO()
	if !strings.Contains(got, "burst-tail") {
		t.Fatalf("emulator does not hold the chunk at signal time: %q", got)
	}
}
