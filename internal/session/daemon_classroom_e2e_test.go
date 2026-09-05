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
// NewClassroomWindow).
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
	defer d.manager.Shutdown()
	mainSocketPath := filepath.Join(t.TempDir(), "daemon.sock")
	d.manager.SetSocketPath(mainSocketPath)
	if err := d.startClassroomHandoffListener(); err != nil {
		t.Fatalf("startClassroomHandoffListener: %v", err)
	}
	defer func() { _ = d.classroomListener.Close() }()

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
}
