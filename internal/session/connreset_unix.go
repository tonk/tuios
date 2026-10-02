//go:build !windows

package session

import (
	"errors"
	"syscall"
)

// isConnResetErrno reports whether err is the kernel's connection reset.
func isConnResetErrno(err error) bool {
	return errors.Is(err, syscall.ECONNRESET)
}
