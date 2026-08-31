// Package wire is the tiny local protocol between the pam-helper (root) and
// its clients: a length-prefixed request/response pair over a Unix socket,
// with the PTY master fd riding along on the response as SCM_RIGHTS ancillary
// data. This is deliberately minimal — a real integration would replace this
// whole package with tuios's own daemon protocol (internal/session).
package wire

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"syscall"
)

// SocketPath is where the helper listens and clients dial by default.
const SocketPath = "/run/tuios-pam-poc.sock"

const maxFrame = 1 << 16 // plenty for a username or password; refuses anything absurd

// WriteFrame writes b as a 4-byte big-endian length prefix followed by b.
func WriteFrame(w io.Writer, b []byte) error {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

// ReadFrame reads one length-prefixed frame written by WriteFrame.
func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxFrame {
		return nil, fmt.Errorf("frame too large: %d bytes", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// Status bytes, sent as the single-byte payload of the response message.
const (
	StatusOK  byte = 0
	StatusErr byte = 1
)

// SendResult writes the response: one status byte, plus (on success) the PTY
// master fd as ancillary data, or (on failure) a length-prefixed error frame
// written separately by the caller after this call.
func SendResult(conn *net.UnixConn, ok bool, fd int) error {
	status := StatusErr
	var oob []byte
	if ok {
		status = StatusOK
		oob = syscall.UnixRights(fd)
	}
	_, _, err := conn.WriteMsgUnix([]byte{status}, oob, nil)
	return err
}

// RecvResult reads the response status and, on success, the passed fd. The fd
// is only valid (and only present) when ok is true.
func RecvResult(conn *net.UnixConn) (ok bool, fd int, err error) {
	buf := make([]byte, 1)
	oob := make([]byte, syscall.CmsgSpace(4)) // one int fd
	n, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return false, 0, err
	}
	if n < 1 {
		return false, 0, fmt.Errorf("short response")
	}
	if buf[0] != StatusOK {
		return false, 0, nil
	}
	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return false, 0, fmt.Errorf("parsing control message: %w", err)
	}
	if len(scms) != 1 {
		return false, 0, fmt.Errorf("expected 1 control message, got %d", len(scms))
	}
	fds, err := syscall.ParseUnixRights(&scms[0])
	if err != nil {
		return false, 0, fmt.Errorf("parsing unix rights: %w", err)
	}
	if len(fds) != 1 {
		return false, 0, fmt.Errorf("expected 1 fd, got %d", len(fds))
	}
	return true, fds[0], nil
}
