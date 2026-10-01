//go:build windows

package session

import (
	"errors"
	"net"
)

// errFDPassUnsupported: the classroom handoff passes a PAM login fd over a
// Unix socket (SCM_RIGHTS), which Windows has no equivalent for.
var errFDPassUnsupported = errors.New("fd passing is not supported on windows")

func readMsgWithOOB(_ *net.UnixConn, _ []byte) (int, []byte, error) {
	return 0, nil, errFDPassUnsupported
}

func parseSingleFD(_ []byte) (int, error) { return -1, errFDPassUnsupported }

func unixRightsOOB(_ int) []byte { return nil }
