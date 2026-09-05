package session

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// adoptedPty wraps a PTY master file that something else already opened and
// started a process on - a privileged helper authenticating a trainee via
// PAM and spawning their shell as their own Unix account (see
// internal/pamauth), rather than this session spawning a shell itself via
// CreatePTY. It satisfies xpty.Pty so AdoptPTY can hand it to newPTY exactly
// like a freshly-created one; only Resize/Size (ioctl on the fd directly, since
// there is no xpty.Pty of our own to ask) and Start (never valid here) differ.
// Mirrors internal/terminal's identically-named, identically-shaped type for
// the non-daemon PAM path.
type adoptedPty struct {
	*os.File
}

func (p *adoptedPty) Resize(width, height int) error {
	return pty.Setsize(p.File, &pty.Winsize{Cols: uint16(width), Rows: uint16(height)}) //nolint:gosec // width/height are terminal cell counts, never near uint16 overflow
}

func (p *adoptedPty) Size() (width, height int, err error) {
	rows, cols, err := pty.Getsize(p.File)
	if err != nil {
		return 0, 0, err
	}
	return cols, rows, nil
}

func (p *adoptedPty) Start(*exec.Cmd) error {
	return errors.New("adoptedPty: Start is not supported; the process is already running")
}

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
