//go:build windows

package pamauth

import (
	"errors"
	"net"
)

// errUnsupported is returned for every wire operation on Windows: the PAM
// helper protocol passes PTY file descriptors over a Unix socket (SCM_RIGHTS),
// which Windows has no equivalent for.
var errUnsupported = errors.New("pam auth is not supported on windows")

func writeMessage(_ *net.UnixConn, _ byte, _ []byte, _ int) error {
	return errUnsupported
}

func readMessage(_ *net.UnixConn) (msgType byte, payload []byte, fd int, err error) {
	return 0, nil, -1, errUnsupported
}
