//go:build !windows

package session

import (
	"fmt"
	"net"
	"syscall"
)

// readMsgWithOOB reads one datagram plus room for a single passed fd. It
// returns the bytes read into buf and the (possibly empty) ancillary data.
func readMsgWithOOB(conn *net.UnixConn, buf []byte) (n int, oob []byte, err error) {
	oobBuf := make([]byte, syscall.CmsgSpace(4))
	n, oobn, flags, _, err := conn.ReadMsgUnix(buf, oobBuf)
	if err != nil {
		return 0, nil, fmt.Errorf("reading handoff message: %w", err)
	}
	if flags&syscall.MSG_CTRUNC != 0 {
		return 0, nil, fmt.Errorf("ancillary data truncated (MSG_CTRUNC)")
	}
	return n, oobBuf[:oobn], nil
}

// parseSingleFD extracts the one fd carried in oob (SCM_RIGHTS).
func parseSingleFD(oob []byte) (int, error) {
	scms, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return -1, fmt.Errorf("parsing control message: %w", err)
	}
	if len(scms) != 1 {
		return -1, fmt.Errorf("expected 1 control message, got %d", len(scms))
	}
	fds, err := syscall.ParseUnixRights(&scms[0])
	if err != nil || len(fds) != 1 {
		return -1, fmt.Errorf("expected 1 fd, got %d (err=%v)", len(fds), err)
	}
	return fds[0], nil
}

// unixRightsOOB builds the ancillary data that passes fd to the peer.
func unixRightsOOB(fd int) []byte { return syscall.UnixRights(fd) }
