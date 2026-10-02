//go:build windows

package session

import (
	"errors"
	"syscall"
)

// isConnResetErrno reports whether err is Winsock's own reset or abort.
// Windows reports a peer that closed with unread data as WSAECONNRESET, not
// the ECONNRESET Go defines for portability, and its message ("An existing
// connection was forcibly closed") matches none of the Unix wordings.
func isConnResetErrno(err error) bool {
	return errors.Is(err, syscall.WSAECONNRESET) || errors.Is(err, syscall.WSAECONNABORTED)
}
