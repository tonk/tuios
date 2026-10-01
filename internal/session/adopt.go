package session

import (
	"errors"
	"os"
	"os/exec"

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
