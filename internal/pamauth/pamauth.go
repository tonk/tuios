// Package pamauth is tuios-web's client for the (optional, opt-in) PAM
// trainee-auth helper: a small privileged process that authenticates a
// username/password pair against PAM and, on success, spawns shells running
// as that trainee's own Unix account — see
// experimental/pam-trainee-auth/README.md for the full design and how to
// build/run the helper.
//
// This package is pure Go with no cgo dependency of its own: PAM itself
// (and the setuid/fork work) lives entirely in the separate, privileged
// helper process, reached only over a Unix socket. Nothing in this package
// requires any special privilege, and nothing here is reachable unless
// tuios-web is started with --pam-auth.
//
// One Login is one PAM session: dialing and sending credentials
// authenticates once; SpawnPTY may then be called any number of times, once
// per window a trainee opens, without re-sending the password. Closing the
// Login (or losing the connection) tells the helper the trainee's whole
// session is over, at which point it signals every shell it spawned for
// this login and tears down the PAM session.
//
// The wire format here is intentionally kept byte-for-byte identical to
// experimental/pam-trainee-auth/internal/wire — the two packages are
// deliberately not shared code (this module must stay free of the helper's
// cgo/PAM dependency), but they must be kept in sync by hand if the
// protocol changes.
package pamauth

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
)

// DefaultSocketPath is where the helper listens by default.
const DefaultSocketPath = "/run/tuios-pam-helper.sock"

const (
	msgLogin          byte = 1
	msgLoginResult    byte = 2
	msgSpawnPTY       byte = 3
	msgSpawnPTYResult byte = 4
	msgClosePTY       byte = 5
	msgClosePTYResult byte = 6
)

const (
	maxPayload = 1 << 16
	headerLen  = 5
	maxRead    = headerLen + maxPayload
)

// Login is one authenticated PAM session, able to spawn any number of
// shells until Close is called.
type Login struct {
	conn *net.UnixConn
}

// Dial connects to the helper at socketPath and authenticates username with
// password. A non-nil error means either the connection or the PAM login
// itself failed — the caller cannot distinguish which without inspecting
// the error text, which is deliberate: neither should be reported to the
// browser in more detail than "authentication failed".
func Dial(socketPath, username, password string) (*Login, error) {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	conn, err := net.Dial("unixpacket", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connecting to pam-helper at %s: %w", socketPath, err)
	}
	uconn := conn.(*net.UnixConn)

	var buf bytes.Buffer
	putField(&buf, []byte(username))
	putField(&buf, []byte(password))
	if err := writeMessage(uconn, msgLogin, buf.Bytes(), -1); err != nil {
		_ = uconn.Close()
		return nil, fmt.Errorf("sending login: %w", err)
	}

	msgType, payload, _, err := readMessage(uconn)
	if err != nil {
		_ = uconn.Close()
		return nil, fmt.Errorf("reading login result: %w", err)
	}
	if msgType != msgLoginResult {
		_ = uconn.Close()
		return nil, fmt.Errorf("unexpected response type %d to login", msgType)
	}
	ok, errMsg, err := decodeResult(payload)
	if err != nil {
		_ = uconn.Close()
		return nil, fmt.Errorf("decoding login result: %w", err)
	}
	if !ok {
		_ = uconn.Close()
		return nil, fmt.Errorf("login rejected: %s", errMsg)
	}

	return &Login{conn: uconn}, nil
}

// SpawnPTY asks the helper for one more shell on this login, at the given
// terminal size, and returns its PTY master and pid.
func (l *Login) SpawnPTY(cols, rows int) (ptyFile *os.File, pid int, err error) {
	var payload [8]byte
	binary.BigEndian.PutUint32(payload[0:4], uint32(cols))
	binary.BigEndian.PutUint32(payload[4:8], uint32(rows))
	if err := writeMessage(l.conn, msgSpawnPTY, payload[:], -1); err != nil {
		return nil, 0, fmt.Errorf("sending spawn request: %w", err)
	}

	msgType, respPayload, fd, err := readMessage(l.conn)
	if err != nil {
		return nil, 0, fmt.Errorf("reading spawn result: %w", err)
	}
	if msgType != msgSpawnPTYResult {
		return nil, 0, fmt.Errorf("unexpected response type %d to spawn", msgType)
	}
	ok, gotPid, errMsg, err := decodeSpawnResult(respPayload)
	if err != nil {
		return nil, 0, fmt.Errorf("decoding spawn result: %w", err)
	}
	if !ok {
		return nil, 0, fmt.Errorf("spawn rejected: %s", errMsg)
	}
	if fd < 0 {
		return nil, 0, errors.New("spawn succeeded but no pty fd was received")
	}
	return os.NewFile(uintptr(fd), "pty"), gotPid, nil
}

// ClosePTY asks the helper to signal one shell this login previously
// spawned. pid must be one this Login itself received from SpawnPTY.
func (l *Login) ClosePTY(pid int) error {
	var payload [4]byte
	binary.BigEndian.PutUint32(payload[:], uint32(pid))
	if err := writeMessage(l.conn, msgClosePTY, payload[:], -1); err != nil {
		return fmt.Errorf("sending close request: %w", err)
	}

	msgType, respPayload, _, err := readMessage(l.conn)
	if err != nil {
		return fmt.Errorf("reading close result: %w", err)
	}
	if msgType != msgClosePTYResult {
		return fmt.Errorf("unexpected response type %d to close", msgType)
	}
	ok, errMsg, err := decodeResult(respPayload)
	if err != nil {
		return fmt.Errorf("decoding close result: %w", err)
	}
	if !ok {
		return fmt.Errorf("close rejected: %s", errMsg)
	}
	return nil
}

// Close ends the login: the helper signals every shell still running for it
// and tears down the PAM session once it sees the connection close.
func (l *Login) Close() error {
	return l.conn.Close()
}

// String satisfies sip.Identity, so a *Login can be carried in a
// ConnectMiddleware's request context directly.
func (l *Login) String() string { return "pam-authenticated" }

// --- wire protocol (kept in sync by hand with experimental/pam-trainee-auth/internal/wire) ---

func writeMessage(conn *net.UnixConn, msgType byte, payload []byte, fd int) error {
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

func readMessage(conn *net.UnixConn) (msgType byte, payload []byte, fd int, err error) {
	buf := make([]byte, maxRead)
	oob := make([]byte, syscall.CmsgSpace(4))
	n, oobn, flags, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return 0, nil, -1, err
	}
	if flags&syscall.MSG_CTRUNC != 0 {
		return 0, nil, -1, errors.New("ancillary data truncated (MSG_CTRUNC)")
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

func putField(buf *bytes.Buffer, v []byte) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(v)))
	buf.Write(n[:])
	buf.Write(v)
}

func decodeResult(payload []byte) (ok bool, errMsg string, err error) {
	if len(payload) < 1 {
		return false, "", errors.New("empty result payload")
	}
	if payload[0] == 1 {
		return true, "", nil
	}
	r := bytes.NewReader(payload[1:])
	msg, err := getField(r)
	if err != nil {
		return false, "", fmt.Errorf("reading error message: %w", err)
	}
	return false, string(msg), nil
}

func decodeSpawnResult(payload []byte) (ok bool, pid int, errMsg string, err error) {
	if len(payload) < 5 {
		return false, 0, "", errors.New("short spawn result payload")
	}
	ok = payload[0] == 1
	pid = int(binary.BigEndian.Uint32(payload[1:5]))
	if ok {
		return true, pid, "", nil
	}
	r := bytes.NewReader(payload[5:])
	msg, err := getField(r)
	if err != nil {
		return false, 0, "", fmt.Errorf("reading error message: %w", err)
	}
	return false, 0, string(msg), nil
}

func getField(r *bytes.Reader) ([]byte, error) {
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
