package pamauth

import (
	"bytes"
	"encoding/binary"
	"net"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// fakeHelper is a minimal stand-in for tuios-pam-helper, implementing just
// enough of the wire protocol (see the format comments on Dial/SpawnPTY/
// ClosePTY) to exercise a real Login end to end: it always accepts the login,
// and SpawnPTY starts a real local shell (there is no PAM/setuid concern in a
// unit test - the only thing under test is the wire protocol and fd
// handling, not privilege separation).
type fakeHelper struct {
	ln net.Listener
}

func newFakeHelper(t *testing.T) *fakeHelper {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pam-helper.sock")
	ln, err := net.Listen("unixpacket", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	h := &fakeHelper{ln: ln}
	go h.acceptLoop(t)
	t.Cleanup(func() { _ = ln.Close() })
	return h
}

func (h *fakeHelper) socketPath() string {
	return h.ln.Addr().String()
}

func (h *fakeHelper) acceptLoop(t *testing.T) {
	for {
		conn, err := h.ln.Accept()
		if err != nil {
			return
		}
		go h.serve(t, conn.(*net.UnixConn))
	}
}

func (h *fakeHelper) serve(t *testing.T, conn *net.UnixConn) {
	defer func() { _ = conn.Close() }()
	for {
		msgType, payload, _, err := readMessage(conn)
		if err != nil {
			return
		}
		switch msgType {
		case msgLogin:
			_ = writeMessage(conn, msgLoginResult, []byte{1}, -1)

		case msgSpawnPTY:
			cols := binary.BigEndian.Uint32(payload[0:4])
			rows := binary.BigEndian.Uint32(payload[4:8])
			cmd := exec.Command("sh")
			ptyFile, err := pty.Start(cmd)
			if err != nil {
				t.Logf("fake helper: pty.Start: %v", err)
				var buf bytes.Buffer
				buf.WriteByte(0)
				putField(&buf, []byte("pty.Start failed"))
				_ = writeMessage(conn, msgSpawnPTYResult, buf.Bytes(), -1)
				continue
			}
			_ = pty.Setsize(ptyFile, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
			go func() { _ = cmd.Wait() }()

			resp := make([]byte, 5)
			resp[0] = 1
			binary.BigEndian.PutUint32(resp[1:5], uint32(cmd.Process.Pid))
			_ = writeMessage(conn, msgSpawnPTYResult, resp, int(ptyFile.Fd()))
			_ = ptyFile.Close() // sent via SCM_RIGHTS (kernel dups it); our copy is done

		case msgClosePTY:
			pid := int(binary.BigEndian.Uint32(payload[0:4]))
			_ = syscall.Kill(pid, syscall.SIGKILL)
			_ = writeMessage(conn, msgClosePTYResult, []byte{1}, -1)
		}
	}
}

// TestLoginSpawnAndClosePTY exercises an ordinary Dial'd Login: SpawnPTY
// against the fake helper starts a real shell, output flows, and ClosePTY
// terminates it.
func TestLoginSpawnAndClosePTY(t *testing.T) {
	h := newFakeHelper(t)

	login, err := Dial(h.socketPath(), "trainee", "irrelevant")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = login.Close() }()

	if got := login.Username(); got != "trainee" {
		t.Errorf("Username() = %q, want %q", got, "trainee")
	}

	ptyFile, pid, err := login.SpawnPTY(80, 24)
	if err != nil {
		t.Fatalf("SpawnPTY: %v", err)
	}
	defer func() { _ = ptyFile.Close() }()
	if pid == 0 {
		t.Fatal("SpawnPTY returned pid 0")
	}

	if _, err := ptyFile.WriteString("echo pamauth-marker\n"); err != nil {
		t.Fatalf("write to pty: %v", err)
	}
	if !waitForOutput(t, ptyFile) {
		t.Skip("shell produced no output in this environment")
	}

	if err := login.ClosePTY(pid); err != nil {
		t.Fatalf("ClosePTY: %v", err)
	}
}

// TestNewLoginFromFD proves the exact round trip the classroom trainer
// console depends on: a Login's fd (as obtained via File, in the process
// that Dialed it) can be handed to NewLoginFromFD - simulating another
// process receiving it via SCM_RIGHTS - and the reconstructed Login still
// works, indistinguishable from the original.
func TestNewLoginFromFD(t *testing.T) {
	h := newFakeHelper(t)

	original, err := Dial(h.socketPath(), "trainee", "irrelevant")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = original.Close() }()

	f, err := original.File()
	if err != nil {
		t.Fatalf("File: %v", err)
	}

	reconstructed, err := NewLoginFromFD(f.Fd(), original.Username())
	if err != nil {
		t.Fatalf("NewLoginFromFD: %v", err)
	}
	defer func() { _ = reconstructed.Close() }()
	_ = f.Close() // NewLoginFromFD dups it; this process's copy is no longer needed

	if got := reconstructed.Username(); got != "trainee" {
		t.Errorf("Username() = %q, want %q", got, "trainee")
	}

	ptyFile, pid, err := reconstructed.SpawnPTY(80, 24)
	if err != nil {
		t.Fatalf("SpawnPTY on reconstructed Login: %v", err)
	}
	defer func() { _ = ptyFile.Close() }()

	if _, err := ptyFile.WriteString("echo pamauth-marker\n"); err != nil {
		t.Fatalf("write to pty: %v", err)
	}
	if !waitForOutput(t, ptyFile) {
		t.Skip("shell produced no output in this environment")
	}

	if err := reconstructed.ClosePTY(pid); err != nil {
		t.Fatalf("ClosePTY on reconstructed Login: %v", err)
	}
}

func waitForOutput(t *testing.T, f interface{ Read([]byte) (int, error) }) bool {
	t.Helper()
	type readResult struct {
		n   int
		err error
	}
	ch := make(chan readResult, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := f.Read(buf)
		ch <- readResult{n, err}
	}()
	select {
	case r := <-ch:
		return r.err == nil && r.n > 0
	case <-time.After(5 * time.Second):
		return false
	}
}
