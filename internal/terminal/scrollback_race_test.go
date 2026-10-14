package terminal

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/tonk/tuios/internal/vt"
)

// raceScrollbackLen hammers a pane the way the PTY reader does - under the
// exclusive I/O lock, resize it narrower and wider, which leaves a reflow of
// the scrollback pending, and write output - while another goroutine asks for
// the scrollback length through read. The length read settles that reflow, so
// a read that does not keep out of the reader's way corrupts the scrollback or
// panics (index out of range in the reflower), and -race reports it.
func raceScrollbackLen(t *testing.T, read func(*Window) int) {
	t.Helper()
	em := vt.NewEmulator(80, 25)
	t.Cleanup(func() { _ = em.Close() })
	win := &Window{Terminal: em}
	for i := range 2000 {
		_, _ = em.Write([]byte(fmt.Sprintf("line %d with enough text to wrap when the pane gets narrow\r\n", i)))
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			win.LockIO()
			em.Resize(40+i%60, 25)
			_, _ = em.Write([]byte(fmt.Sprintf("output %d\r\n", i)))
			win.UnlockIO()
		}
	}()

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if n := read(win); n < 0 {
			t.Fatalf("negative scrollback length %d", n)
		}
	}
	close(stop)
	wg.Wait()
}

// TestScrollbackLenSyncKeepsOutOfTheReader is the regression test for the
// crash a finished window animation caused by reading the length with no lock.
func TestScrollbackLenSyncKeepsOutOfTheReader(t *testing.T) {
	raceScrollbackLen(t, (*Window).ScrollbackLenSync)
}
