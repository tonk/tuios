package session

import (
	"encoding/binary"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// This file's fake helper mirrors internal/pamauth's own wire protocol (see
// that package's doc comments and pamauth_test.go) closely enough to drive a
// reconstructed Login for real, without importing pamauth's unexported
// constants across the package boundary. It exists to prove the daemon's
// classroom handoff listener end to end: dial it, hand over a fd connected
// to this fake helper, and confirm a real session with a live window comes
// out the other side - the composition of pieces already tested in
// isolation (daemon_classroom_test.go's wire round trip,
// pamauth_test.go's Login-from-fd round trip, classroom_test.go's
// CreatePTY-dispatches-to-the-spawner behavior).
const (
	fakeMsgSpawnPTY       byte = 3
	fakeMsgSpawnPTYResult byte = 4
	fakeMsgClosePTY       byte = 5
	fakeMsgClosePTYResult byte = 6
)

func runFakePAMHelper(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pam-helper.sock")
	ln, err := net.Listen("unixpacket", path)
	if err != nil {
		t.Fatalf("fake helper listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveFakePAMHelper(conn.(*net.UnixConn))
		}
	}()
	return path
}

func serveFakePAMHelper(conn *net.UnixConn) {
	defer func() { _ = conn.Close() }()
	buf := make([]byte, 65541)
	for {
		n, _, _, _, err := conn.ReadMsgUnix(buf, nil)
		if err != nil || n < 5 {
			return
		}
		msgType := buf[0]
		payload := buf[5:n]

		switch msgType {
		case fakeMsgSpawnPTY:
			cols := binary.BigEndian.Uint32(payload[0:4])
			rows := binary.BigEndian.Uint32(payload[4:8])
			cmd := exec.Command("sh")
			ptyFile, err := pty.Start(cmd)
			if err != nil {
				writeFakeHelperMessage(conn, fakeMsgSpawnPTYResult, []byte{0, 0, 0, 0, 0}, -1)
				continue
			}
			_ = pty.Setsize(ptyFile, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
			go func() { _ = cmd.Wait() }()

			resp := make([]byte, 5)
			resp[0] = 1
			binary.BigEndian.PutUint32(resp[1:5], uint32(cmd.Process.Pid))
			writeFakeHelperMessage(conn, fakeMsgSpawnPTYResult, resp, int(ptyFile.Fd()))
			_ = ptyFile.Close()

		case fakeMsgClosePTY:
			pid := int(binary.BigEndian.Uint32(payload[0:4]))
			_ = exec.Command("kill", "-KILL", strconv.Itoa(pid)).Run()
			writeFakeHelperMessage(conn, fakeMsgClosePTYResult, []byte{1}, -1)
		}
	}
}

func writeFakeHelperMessage(conn *net.UnixConn, msgType byte, payload []byte, fd int) {
	out := make([]byte, 0, 5+len(payload))
	out = append(out, msgType)
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(payload)))
	out = append(out, n[:]...)
	out = append(out, payload...)

	var oob []byte
	if fd >= 0 {
		oob = syscall.UnixRights(fd)
	}
	_, _, _ = conn.WriteMsgUnix(out, oob, nil)
}

// TestDaemonClassroomHandoffCreatesSession drives the whole classroom login
// path through the daemon's real accept loop: a fake pam-helper stands in
// for the privileged process, SendClassroomLogin hands its connection to a
// running Daemon's classroom listener, and the result is a real named
// session with one live window whose shell was spawned by the fake helper,
// not by this session's own exec.Command.
func TestDaemonClassroomHandoffCreatesSession(t *testing.T) {
	helperPath := runFakePAMHelper(t)

	helperConn, err := net.Dial("unixpacket", helperPath)
	if err != nil {
		t.Fatalf("dialing fake helper: %v", err)
	}
	loginFile, err := helperConn.(*net.UnixConn).File()
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	_ = helperConn.Close()
	defer func() { _ = loginFile.Close() }()

	d := NewDaemon(&DaemonConfig{DisableAutoRestore: true})
	// Not a bare listener Close: that leaves d.ctx uncancelled, so
	// classroomHandoffAcceptLoop's ctx.Done() check never fires and it spins
	// logging accept errors instead of returning. Stop cancels first.
	defer d.Stop()
	mainSocketPath := filepath.Join(t.TempDir(), "daemon.sock")
	d.manager.SetSocketPath(mainSocketPath)
	if err := d.startClassroomHandoffListener(); err != nil {
		t.Fatalf("startClassroomHandoffListener: %v", err)
	}

	if err := SendClassroomLogin(mainSocketPath, "guru07", "guru07", loginFile, 80, 24); err != nil {
		t.Fatalf("SendClassroomLogin: %v", err)
	}

	var sess *Session
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sess = d.manager.GetSession("guru07"); sess != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if sess == nil {
		t.Fatal("classroom session \"guru07\" was never created")
	}
	if sess.ClassroomSpawner() == nil {
		t.Fatal("session has no classroom spawner")
	}

	state := sess.GetState()
	if len(state.Windows) != 1 {
		t.Fatalf("window count = %d, want 1", len(state.Windows))
	}
	p := sess.GetPTY(state.Windows[0].PTYID)
	if p == nil {
		t.Fatal("initial window has no live PTY")
	}
	if p.IsExited() {
		t.Fatal("initial window's PTY already reports exited")
	}

	// Regression test for a real, timing-dependent bug: closing the fd
	// NewLoginFromFile reconstructs a Login around - whether immediately, or
	// deferred to handleClassroomHandoff's own return - frees its number
	// for reuse while the Login is still actively used moments later to
	// receive the adopted PTY's own fd. When something grabbed that freed
	// number first, the PTY silently died within moments of creation
	// (logged as "bad file descriptor", confirmed live against stepper). A
	// single IsExited() check right after creation never caught this; only
	// reading/writing after a delay does. Fixed by tying the reconstructed
	// fd's lifetime to the Login's own (closed together in Login.Close),
	// removing the free-then-reuse window entirely rather than just moving
	// it.
	if _, err := p.Write([]byte("echo classroom-marker\n")); err != nil {
		t.Fatalf("write to PTY: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if p.IsExited() {
		t.Fatal("PTY exited shortly after creation - closed out from under the shell")
	}
	if _, err := p.Write([]byte("echo still-alive\n")); err != nil {
		t.Fatalf("second write to PTY after a delay: %v", err)
	}
	if p.IsExited() {
		t.Fatal("PTY exited after a second write - closed out from under the shell")
	}

	// A second handoff for the same session name (a race between two near-
	// simultaneous first connections, or a client that skipped its own
	// existence check) must not replace the spawner already in use, and must
	// not open a second window.
	firstSpawner := sess.ClassroomSpawner()
	secondHelperPath := runFakePAMHelper(t)
	secondHelperConn, err := net.Dial("unixpacket", secondHelperPath)
	if err != nil {
		t.Fatalf("dialing second fake helper: %v", err)
	}
	secondLoginFile, err := secondHelperConn.(*net.UnixConn).File()
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	_ = secondHelperConn.Close()
	defer func() { _ = secondLoginFile.Close() }()

	if err := SendClassroomLogin(mainSocketPath, "guru07", "guru07", secondLoginFile, 80, 24); err != nil {
		t.Fatalf("second SendClassroomLogin: %v", err)
	}
	if got := sess.ClassroomSpawner(); got != firstSpawner {
		t.Fatal("a redundant handoff replaced the session's existing spawner")
	}
	if state := sess.GetState(); len(state.Windows) != 1 {
		t.Fatalf("window count after redundant handoff = %d, want 1", len(state.Windows))
	}
}

// TestClassroomHandoffReplacesAStaleResurrectedSession is the regression
// test for a real bug confirmed live on stepper: a classroom session that
// survives a daemon restart comes back via ordinary resurrection
// (RestorePTY), which has no live PAM login to spawn through yet and so
// runs its window as the daemon's own account - and nothing ever replaced
// it. A trainee reconnecting after a restart silently inherited that
// stale, wrong-account window instead of getting a fresh one under their
// own name.
//
// This builds that exact "resurrected, no spawner" shape directly (a
// session with an ordinary AddDaemonWindow window, never touched by a
// classroom handoff) rather than exercising real daemon-restart
// resurrection, since the shape that matters here - session exists,
// ClassroomSpawner() is nil - is identical either way.
func TestClassroomHandoffReplacesAStaleResurrectedSession(t *testing.T) {
	d := NewDaemon(&DaemonConfig{DisableAutoRestore: true})
	defer d.Stop()
	mainSocketPath := filepath.Join(t.TempDir(), "daemon.sock")
	d.manager.SetSocketPath(mainSocketPath)
	if err := d.startClassroomHandoffListener(); err != nil {
		t.Fatalf("startClassroomHandoffListener: %v", err)
	}

	staleSess, err := d.manager.CreateSession("guru07", &SessionConfig{}, 80, 24)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	staleWin, err := staleSess.AddDaemonWindow("", nil)
	if err != nil {
		t.Fatalf("AddDaemonWindow (simulating a resurrected window): %v", err)
	}
	staleSessionID := staleSess.ID
	if staleSess.ClassroomSpawner() != nil {
		t.Fatal("test setup bug: the stale session must start with no spawner")
	}

	helperPath := runFakePAMHelper(t)
	helperConn, err := net.Dial("unixpacket", helperPath)
	if err != nil {
		t.Fatalf("dialing fake helper: %v", err)
	}
	loginFile, err := helperConn.(*net.UnixConn).File()
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	_ = helperConn.Close()
	defer func() { _ = loginFile.Close() }()

	if err := SendClassroomLogin(mainSocketPath, "guru07", "guru07", loginFile, 80, 24); err != nil {
		t.Fatalf("SendClassroomLogin: %v", err)
	}

	var sess *Session
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sess = d.manager.GetSession("guru07"); sess != nil && sess.ClassroomSpawner() != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if sess == nil || sess.ClassroomSpawner() == nil {
		t.Fatal("classroom session \"guru07\" never got a live spawner")
	}
	if sess.ID == staleSessionID {
		t.Fatal("the stale session was reused instead of torn down and rebuilt")
	}

	state := sess.GetState()
	if len(state.Windows) != 1 {
		t.Fatalf("window count = %d, want 1", len(state.Windows))
	}
	if state.Windows[0].ID == staleWin.ID {
		t.Fatal("the stale window survived; expected a fresh one from the new session")
	}
	if p := sess.GetPTY(state.Windows[0].PTYID); p == nil || p.IsExited() {
		t.Fatal("the new session's window has no live PTY")
	}
}
