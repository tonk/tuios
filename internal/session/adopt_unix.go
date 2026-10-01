//go:build !windows

package session

import (
	"errors"
	"syscall"
	"time"
)

// waitForAdoptedExit blocks until pid is gone. Signal 0 sends nothing but
// still asks the kernel whether the pid exists and, separately, whether this
// process would be allowed to signal it - ESRCH means gone, any other result
// (including EPERM, expected here since the pid runs as a different uid)
// means it is still alive. This is the standard way to poll a process this
// one did not fork and so cannot wait4/reap. Mirrors
// internal/terminal.waitForAdoptedExit on the non-daemon PAM path.
func waitForAdoptedExit(pid int) {
	const pollInterval = 500 * time.Millisecond
	for {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(pollInterval)
	}
}
