package session

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/tonk/tuios/internal/pamauth"
)

// This file is the classroom login-handoff listener: a second, dedicated
// Unix socket (distinct from the daemon's main session-protocol socket) that
// tuios-web dials exactly once per PAM-authenticated trainee session, to
// hand this daemon process the trainee's *pamauth.Login - reconstructed here
// from a file descriptor received via SCM_RIGHTS, since a Login's live
// connection to tuios-pam-helper cannot otherwise cross a process boundary.
// From then on Session.CreatePTY calls SpawnPTY/ClosePTY on it directly for
// every window that session ever opens, not just the first.
//
// It is deliberately a separate socket rather than a new message type
// layered onto the main one: the main protocol's per-message reads go
// through a bufio.Reader (see acceptLoop/handleConnection), and SCM_RIGHTS
// ancillary data is only delivered to a recvmsg call that reads the exact
// bytes sent alongside it - anything read through a buffering layer that
// doesn't know to ask for it loses the fd silently (the kernel discards
// ancillary data on an ordinary read). A whole connection whose only job is
// one raw ReadMsgUnix sidesteps that entirely.
//
// Wire format (unixpacket, so one packet is exactly one message - no
// reassembly needed): a single packet of
//   [4 bytes sessionName length][sessionName]
//   [4 bytes username length][username]
//   [4 bytes cols][4 bytes rows]
// plus exactly one fd via SCM_RIGHTS: the trainee's pamauth.Login
// connection. The daemon replies with a single byte (1 = ok, 0 = error)
// optionally followed by an error message, then closes.

const (
	classroomHandoffMaxField = 4096
	classroomHandoffTimeout  = 5 * time.Second
)

// classroomHandoffSocketPath derives the login-handoff socket's path from the
// daemon's main socket path: a sibling file in the same directory, so both
// live wherever GetSocketPath (or a configured override) already puts the
// main one.
func classroomHandoffSocketPath(mainSocketPath string) string {
	return mainSocketPath + ".classroom"
}

// startClassroomHandoffListener binds the login-handoff socket alongside the
// main one. Failing to bind it is not fatal to the daemon as a whole -
// classroom sessions simply cannot be created against this daemon instance
// until it is retried - so callers log and continue rather than treating it
// like the main socket's own bind failure.
func (d *Daemon) startClassroomHandoffListener() error {
	path := classroomHandoffSocketPath(d.manager.SocketPath())
	_ = os.Remove(path) // stale socket from a previous, uncleanly-stopped daemon

	ln, err := net.Listen("unixpacket", path)
	if err != nil {
		return fmt.Errorf("failed to listen on classroom handoff socket: %w", err)
	}
	if err := os.Chmod(path, 0700); err != nil {
		_ = ln.Close()
		return fmt.Errorf("failed to set classroom handoff socket permissions: %w", err)
	}
	d.classroomListener = ln

	go d.classroomHandoffAcceptLoop()
	return nil
}

func (d *Daemon) classroomHandoffAcceptLoop() {
	for {
		conn, err := d.classroomListener.Accept()
		if err != nil {
			select {
			case <-d.ctx.Done():
				return
			default:
				log.Printf("classroom handoff accept error: %v", err)
				continue
			}
		}
		go d.handleClassroomHandoff(conn)
	}
}

func (d *Daemon) handleClassroomHandoff(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	uconn, ok := conn.(*net.UnixConn)
	if !ok {
		log.Printf("classroom handoff: connection is not a unix socket (%T)", conn)
		return
	}
	_ = uconn.SetDeadline(time.Now().Add(classroomHandoffTimeout))

	sessionName, username, cols, rows, loginFile, err := readClassroomHandoff(uconn)
	if err != nil {
		log.Printf("classroom handoff: %v", err)
		_ = writeClassroomHandoffAck(uconn, err)
		return
	}
	// No defer loginFile.Close() here: NewLoginFromFD already closes
	// loginFile's underlying fd itself, every time, success or error (it
	// wraps then hands the fd to net.FileConn, which dups it, then closes
	// the original - see that function's own doc comment). A second close
	// here would be a double-close on that fd *number*: by the time this
	// function returns, the kernel may already have reassigned it to
	// something else entirely (in production, exactly the adopted PTY's own
	// fd, spawned moments later) - closing "loginFile" at that point closes
	// whatever now holds that number, not loginFile at all. This was a real,
	// silent bug: the adopted PTY's fd got closed out from under a running
	// shell, which then read as "bad file descriptor" with no indication
	// why - see internal/session/session.go's readOutput/AdoptPTY debug
	// logging, added while tracking this down.
	login, err := pamauth.NewLoginFromFD(loginFile.Fd(), username)
	if err != nil {
		log.Printf("classroom handoff for %q: %v", sessionName, err)
		_ = writeClassroomHandoffAck(uconn, err)
		return
	}

	sess, created, err := d.manager.GetOrCreateSession(sessionName, &SessionConfig{}, cols, rows)
	if err != nil {
		log.Printf("classroom handoff for %q: %v", sessionName, err)
		_ = login.Close()
		_ = writeClassroomHandoffAck(uconn, err)
		return
	}
	if !created {
		// A session-with-a-spawner already exists: either a genuine race with
		// another handoff for the same trainee, or (before the client's own
		// existence check) a redundant call. Either way, this login is not
		// the session's spawner - installing it would silently orphan the
		// spawner actually in use (never closed, since nothing else holds a
		// reference to replace) without the daemon knowing to stop using it,
		// and closing the login we DO hold spawns no shells to signal, so it
		// is always safe to just end here instead.
		_ = login.Close()
		log.Printf("Classroom session %q for %q already exists; discarding redundant login handoff", sessionName, username)
		_ = writeClassroomHandoffAck(uconn, nil)
		return
	}

	// SetClassroomSpawner must run before AddDaemonWindow: CreatePTY (which
	// AddDaemonWindow calls) checks for a classroom spawner and only routes
	// through it when one is already installed.
	sess.SetClassroomSpawner(login)
	sessionID := sess.ID
	onExit := func(ptyID string) { d.notifyPTYClosed(sessionID, ptyID) }
	if _, err := sess.AddDaemonWindow("", onExit); err != nil {
		log.Printf("classroom handoff for %q: failed to open initial window: %v", sessionName, err)
		_ = writeClassroomHandoffAck(uconn, err)
		return
	}
	log.Printf("Created classroom session %q for %q with an initial window", sessionName, username)

	_ = writeClassroomHandoffAck(uconn, nil)
}

// readClassroomHandoff reads exactly one handoff packet and its accompanying
// fd from conn.
func readClassroomHandoff(conn *net.UnixConn) (sessionName, username string, cols, rows int, loginFile *os.File, err error) {
	buf := make([]byte, 8+2*(4+classroomHandoffMaxField))
	oob := make([]byte, syscall.CmsgSpace(4))

	n, oobn, flags, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return "", "", 0, 0, nil, fmt.Errorf("reading handoff message: %w", err)
	}
	if flags&syscall.MSG_CTRUNC != 0 {
		return "", "", 0, 0, nil, fmt.Errorf("ancillary data truncated (MSG_CTRUNC)")
	}

	r := buf[:n]
	sessionName, r, err = readClassroomField(r)
	if err != nil {
		return "", "", 0, 0, nil, fmt.Errorf("reading session name: %w", err)
	}
	username, r, err = readClassroomField(r)
	if err != nil {
		return "", "", 0, 0, nil, fmt.Errorf("reading username: %w", err)
	}
	if len(r) < 8 {
		return "", "", 0, 0, nil, fmt.Errorf("short handoff message: no size fields")
	}
	cols = int(binary.BigEndian.Uint32(r[0:4]))
	rows = int(binary.BigEndian.Uint32(r[4:8]))

	if oobn == 0 {
		return "", "", 0, 0, nil, fmt.Errorf("handoff message carried no ancillary data (no fd)")
	}
	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return "", "", 0, 0, nil, fmt.Errorf("parsing control message: %w", err)
	}
	if len(scms) != 1 {
		return "", "", 0, 0, nil, fmt.Errorf("expected 1 control message, got %d", len(scms))
	}
	fds, err := syscall.ParseUnixRights(&scms[0])
	if err != nil || len(fds) != 1 {
		return "", "", 0, 0, nil, fmt.Errorf("expected 1 fd, got %d (err=%v)", len(fds), err)
	}
	return sessionName, username, cols, rows, os.NewFile(uintptr(fds[0]), "pam-login"), nil
}

// readClassroomField reads one length-prefixed field and returns the rest of
// buf after it.
func readClassroomField(buf []byte) (field string, rest []byte, err error) {
	if len(buf) < 4 {
		return "", nil, fmt.Errorf("short field header")
	}
	length := binary.BigEndian.Uint32(buf[:4])
	if length > classroomHandoffMaxField {
		return "", nil, fmt.Errorf("field too large: %d bytes", length)
	}
	buf = buf[4:]
	if uint32(len(buf)) < length {
		return "", nil, fmt.Errorf("field truncated: declared %d, have %d", length, len(buf))
	}
	return string(buf[:length]), buf[length:], nil
}

// writeClassroomHandoffAck sends a one-byte ok/error result, followed by the
// error text when handoffErr is non-nil.
func writeClassroomHandoffAck(conn *net.UnixConn, handoffErr error) error {
	if handoffErr == nil {
		_, err := conn.Write([]byte{1})
		return err
	}
	msg := handoffErr.Error()
	buf := make([]byte, 1, 1+len(msg))
	buf[0] = 0
	buf = append(buf, msg...)
	_, err := conn.Write(buf)
	return err
}

// SendClassroomLogin hands the trainee login behind loginFD over to the
// daemon at daemonSocketPath, so it can spawn every window of the named
// session itself from now on (see Session.CreatePTY). loginFD is
// typically the result of (*pamauth.Login).File in the caller - this
// function takes a bare *os.File rather than a *pamauth.Login so this
// package does not need to import internal/pamauth itself; only the
// daemon's own handoff listener (server side) does that, to reconstruct one
// from the fd it receives. Blocks until the daemon acknowledges (or
// rejects) the handoff, or classroomHandoffTimeout passes.
func SendClassroomLogin(daemonSocketPath, sessionName, username string, loginFD *os.File, cols, rows int) error {
	conn, err := net.DialTimeout("unixpacket", classroomHandoffSocketPath(daemonSocketPath), classroomHandoffTimeout)
	if err != nil {
		return fmt.Errorf("connecting to daemon's classroom handoff socket: %w", err)
	}
	defer func() { _ = conn.Close() }()
	uconn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("classroom handoff dial returned %T, not a unix connection", conn)
	}
	_ = uconn.SetDeadline(time.Now().Add(classroomHandoffTimeout))

	buf := make([]byte, 0, 16+len(sessionName)+len(username))
	buf = appendClassroomField(buf, sessionName)
	buf = appendClassroomField(buf, username)
	var sz [8]byte
	binary.BigEndian.PutUint32(sz[0:4], uint32(cols))
	binary.BigEndian.PutUint32(sz[4:8], uint32(rows))
	buf = append(buf, sz[:]...)

	oob := syscall.UnixRights(int(loginFD.Fd()))
	if _, _, err := uconn.WriteMsgUnix(buf, oob, nil); err != nil {
		return fmt.Errorf("sending handoff message: %w", err)
	}

	resp := make([]byte, 1+classroomHandoffMaxField)
	n, err := uconn.Read(resp)
	if err != nil {
		return fmt.Errorf("reading handoff acknowledgement: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("empty handoff acknowledgement")
	}
	if resp[0] == 1 {
		return nil
	}
	return fmt.Errorf("daemon rejected classroom login handoff: %s", string(resp[1:n]))
}

func appendClassroomField(buf []byte, s string) []byte {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(s)))
	buf = append(buf, n[:]...)
	buf = append(buf, s...)
	return buf
}
