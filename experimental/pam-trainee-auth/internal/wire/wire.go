// Package wire is the protocol between the pam-helper (root) and its
// clients, over a Unix socket. One connection is one PAM login: the client
// sends a Login message once, then may send any number of SpawnPTY/ClosePTY
// messages on the same connection — one per window a trainee opens or closes
// — without re-authenticating. Closing the connection is what tells the
// helper the login is over, at which point it tears down the PAM session and
// signals every child shell it spawned for that connection.
//
// Each message is a 1-byte type, a 4-byte big-endian length, and that many
// payload bytes; a SpawnPTY response additionally carries the PTY master fd
// as SCM_RIGHTS ancillary data on the same write. This is deliberately
// minimal — a real integration could replace this whole package with
// something richer, but it only needs to do this one job.
//
// Every message, regardless of type, is sent as exactly one WriteMsgUnix
// call and read back as exactly one ReadMsgUnix call sized for the largest
// possible message. That is not a style choice: SCM_RIGHTS ancillary data on
// a Unix *stream* socket is delivered on whichever recvmsg call consumes the
// bytes it rode in on, so splitting one logical message across two reads (a
// "peek the header, then read the payload" approach) risks the fd arriving
// attached to the header-only read and being silently missed. One write, one
// correctly-sized read, every time, sidesteps that entirely.
package wire

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"syscall"
)

// SocketPath is where the helper listens and clients dial by default. Kept
// in sync by hand with internal/pamauth.DefaultSocketPath in the main tuios
// module.
const SocketPath = "/run/tuios-pam-helper.sock"

// Message types.
const (
	MsgLogin          byte = 1 // client -> helper: username, password
	MsgLoginResult    byte = 2 // helper -> client: ok, [error]
	MsgSpawnPTY       byte = 3 // client -> helper: cols, rows
	MsgSpawnPTYResult byte = 4 // helper -> client: ok, pid, [error]; fd rides along on success
	MsgClosePTY       byte = 5 // client -> helper: pid
	MsgClosePTYResult byte = 6 // helper -> client: ok, [error]
)

const (
	maxPayload = 1 << 16       // plenty for a username, password or error string
	headerLen  = 5             // 1 type byte + 4 length bytes
	maxRead    = headerLen + maxPayload
)

// WriteMessage writes one message — type, length-prefixed payload, and
// (fd >= 0) the given fd as SCM_RIGHTS ancillary data — as a single
// WriteMsgUnix call.
func WriteMessage(conn *net.UnixConn, msgType byte, payload []byte, fd int) error {
	if len(payload) > maxPayload {
		return fmt.Errorf("payload too large: %d bytes", len(payload))
	}
	buf := make([]byte, 0, headerLen+len(payload))
	buf = append(buf, msgType)
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(payload)))
	buf = append(buf, n[:]...)
	buf = append(buf, payload...)

	var oob []byte
	if fd >= 0 {
		oob = syscall.UnixRights(fd)
	}
	_, _, err := conn.WriteMsgUnix(buf, oob, nil)
	return err
}

// ReadMessage reads one message written by WriteMessage, in a single
// ReadMsgUnix call (see the package doc for why that matters). fd is -1 when
// the message carried none.
func ReadMessage(conn *net.UnixConn) (msgType byte, payload []byte, fd int, err error) {
	buf := make([]byte, maxRead)
	oob := make([]byte, syscall.CmsgSpace(4)) // room for exactly one fd
	n, oobn, flags, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return 0, nil, -1, err
	}
	if flags&syscall.MSG_CTRUNC != 0 {
		return 0, nil, -1, fmt.Errorf("ancillary data truncated (MSG_CTRUNC)")
	}
	if n < headerLen {
		return 0, nil, -1, fmt.Errorf("short message: %d bytes", n)
	}
	msgType = buf[0]
	declaredLen := int(binary.BigEndian.Uint32(buf[1:headerLen]))
	if headerLen+declaredLen != n {
		return 0, nil, -1, fmt.Errorf("message length mismatch: header says %d, read %d", declaredLen, n-headerLen)
	}
	payload = buf[headerLen:n]

	fd = -1
	if oobn > 0 {
		scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			return 0, nil, -1, fmt.Errorf("parsing control message: %w", err)
		}
		if len(scms) == 1 {
			if fds, err := syscall.ParseUnixRights(&scms[0]); err == nil && len(fds) == 1 {
				fd = fds[0]
			}
		}
	}
	return msgType, payload, fd, nil
}

// PutField appends v to buf as a 4-byte length prefix followed by v — the
// sub-field framing used inside a message payload (e.g. Login's username and
// password, both riding in one MsgLogin payload).
func PutField(buf *bytes.Buffer, v []byte) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(v)))
	buf.Write(n[:])
	buf.Write(v)
}

// GetField reads one PutField-encoded sub-field from r.
func GetField(r *bytes.Reader) ([]byte, error) {
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(n[:])
	if length > maxPayload {
		return nil, fmt.Errorf("field too large: %d bytes", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
