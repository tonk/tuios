package session

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestClassroomHandoffWireRoundTrip pins the handoff socket's own framing
// (session name, username, cols, rows, and the accompanying fd via
// SCM_RIGHTS) independent of pamauth or the daemon/session machinery: it
// drives readClassroomHandoff/writeClassroomHandoffAck directly against a
// real listener, and SendClassroomLogin against it as the client. The fd
// itself is verified to be the same open file on both ends (not just a
// valid fd) by writing a marker to it before sending and reading it back
// after receiving.
func TestClassroomHandoffWireRoundTrip(t *testing.T) {
	mainSocketPath := filepath.Join(t.TempDir(), "daemon.sock")
	ln, err := net.Listen("unixpacket", classroomHandoffSocketPath(mainSocketPath))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	tmpFile, err := os.CreateTemp(t.TempDir(), "login-fd")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = tmpFile.Close() }()
	const marker = "classroom-handoff-marker"
	if _, err := tmpFile.WriteString(marker); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if _, err := tmpFile.Seek(0, 0); err != nil {
		t.Fatalf("Seek: %v", err)
	}

	type result struct {
		sessionName, username string
		cols, rows            int
		received              string
		err                   error
	}
	resultCh := make(chan result, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			resultCh <- result{err: err}
			return
		}
		defer func() { _ = conn.Close() }()
		uconn := conn.(*net.UnixConn)
		_ = uconn.SetDeadline(time.Now().Add(5 * time.Second))

		name, user, cols, rows, f, err := readClassroomHandoff(uconn)
		if err != nil {
			resultCh <- result{err: err}
			return
		}
		defer func() { _ = f.Close() }()

		buf := make([]byte, len(marker))
		n, _ := f.Read(buf)

		if err := writeClassroomHandoffAck(uconn, nil); err != nil {
			resultCh <- result{err: err}
			return
		}
		resultCh <- result{sessionName: name, username: user, cols: cols, rows: rows, received: string(buf[:n])}
	}()

	if err := SendClassroomLogin(mainSocketPath, "guru07", "guru07", tmpFile, 80, 24); err != nil {
		t.Fatalf("SendClassroomLogin: %v", err)
	}

	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("server side: %v", r.err)
		}
		if r.sessionName != "guru07" || r.username != "guru07" {
			t.Errorf("sessionName/username = %q/%q, want guru07/guru07", r.sessionName, r.username)
		}
		if r.cols != 80 || r.rows != 24 {
			t.Errorf("cols/rows = %d/%d, want 80/24", r.cols, r.rows)
		}
		if r.received != marker {
			t.Errorf("fd content received = %q, want %q (the fd itself did not transit correctly)", r.received, marker)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server side never responded")
	}
}

// TestClassroomHandoffRejectsMissingFD confirms a handoff attempt with no
// ancillary data at all is rejected with a clear error rather than silently
// creating a spawner-less session - readClassroomHandoff is the only thing
// standing between a malformed/adversarial connection and that.
func TestClassroomHandoffRejectsMissingFD(t *testing.T) {
	mainSocketPath := filepath.Join(t.TempDir(), "daemon.sock")
	ln, err := net.Listen("unixpacket", classroomHandoffSocketPath(mainSocketPath))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	errCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer func() { _ = conn.Close() }()
		uconn := conn.(*net.UnixConn)
		_ = uconn.SetDeadline(time.Now().Add(5 * time.Second))
		_, _, _, _, _, err = readClassroomHandoff(uconn)
		errCh <- err
	}()

	conn, err := net.DialTimeout("unixpacket", classroomHandoffSocketPath(mainSocketPath), 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	buf := appendClassroomField(nil, "guru07")
	buf = appendClassroomField(buf, "guru07")
	buf = append(buf, 0, 0, 0, 80, 0, 0, 0, 24)
	if _, err := conn.Write(buf); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("readClassroomHandoff accepted a message with no fd")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server side never responded")
	}
}
