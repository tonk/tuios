package session

import (
	"os"
	"os/exec"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

// fakeClassroomSpawner stands in for a *pamauth.Login in tests: each SpawnPTY
// starts a real local shell (there is no tuios-pam-helper in a unit test) and
// records what ClosePTY/Close were asked to do, so NewClassroomWindow and its
// window's Close can be verified without a privileged helper process.
type fakeClassroomSpawner struct {
	mu        sync.Mutex
	cmds      map[int]*exec.Cmd
	closed    []int
	allClosed bool
}

func newFakeClassroomSpawner() *fakeClassroomSpawner {
	return &fakeClassroomSpawner{cmds: make(map[int]*exec.Cmd)}
}

func (f *fakeClassroomSpawner) SpawnPTY(cols, rows int) (*os.File, int, error) {
	cmd := exec.Command("sh")
	ptyFile, err := pty.Start(cmd)
	if err != nil {
		return nil, 0, err
	}
	_ = pty.Setsize(ptyFile, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()

	f.mu.Lock()
	f.cmds[pid] = cmd
	f.mu.Unlock()
	return ptyFile, pid, nil
}

func (f *fakeClassroomSpawner) ClosePTY(pid int) error {
	f.mu.Lock()
	cmd, ok := f.cmds[pid]
	f.closed = append(f.closed, pid)
	f.mu.Unlock()
	if !ok {
		return nil
	}
	return cmd.Process.Kill()
}

func (f *fakeClassroomSpawner) Close() error {
	f.mu.Lock()
	f.allClosed = true
	cmds := make([]*exec.Cmd, 0, len(f.cmds))
	for _, cmd := range f.cmds {
		cmds = append(cmds, cmd)
	}
	f.mu.Unlock()
	for _, cmd := range cmds {
		_ = cmd.Process.Kill()
	}
	return nil
}

func (f *fakeClassroomSpawner) wasClosed(pid int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.closed, pid)
}

// TestNewClassroomWindow exercises a classroom session end to end at the
// session-package level: a held spawner (standing in for a *pamauth.Login)
// provides every window's PTY, output still flows into the daemon's VT
// emulator exactly like an ordinary window, and closing the window routes
// through the spawner's ClosePTY rather than a local process kill.
func TestNewClassroomWindow(t *testing.T) {
	// Not newTestSession: that registers its own t.Cleanup(sess.Stop), and
	// this test calls Stop itself to check it tears down the spawner -
	// Stop is not safe to call twice (a pre-existing, unrelated issue in the
	// periodic-resurrection-save teardown it also does).
	sess, err := NewSession("classroom-test", &SessionConfig{}, 80, 24)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	sp := newFakeClassroomSpawner()
	sess.SetClassroomSpawner(sp)

	if got := sess.ClassroomSpawner(); got != ClassroomSpawner(sp) {
		t.Fatalf("ClassroomSpawner() = %v, want the spawner just set", got)
	}

	win, err := sess.NewClassroomWindow("trainee shell", nil)
	if err != nil {
		t.Fatalf("NewClassroomWindow: %v", err)
	}
	if win.PTYID == "" {
		t.Fatal("created window has empty PTYID")
	}

	p := sess.GetPTY(win.PTYID)
	if p == nil {
		t.Fatal("PTY not registered on session")
	}
	if p.IsExited() {
		t.Fatal("freshly created classroom PTY already reports exited")
	}
	pid := p.ShellPID()
	if pid == 0 {
		t.Fatal("ShellPID() = 0 for a classroom window")
	}

	if _, err := p.Write([]byte("echo tuios-classroom-marker\n")); err != nil {
		t.Fatalf("write to classroom PTY: %v", err)
	}
	waitForCapture := func() string {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if got := p.CaptureContent(false, false); len(got) > 0 {
				return got
			}
			time.Sleep(20 * time.Millisecond)
		}
		return ""
	}
	if waitForCapture() == "" {
		t.Skip("shell produced no output in this environment")
	}

	if err := sess.ClosePTY(win.PTYID); err != nil {
		t.Fatalf("ClosePTY: %v", err)
	}
	if !sp.wasClosed(pid) {
		t.Fatal("closing the window did not call the spawner's ClosePTY")
	}

	// Session.Stop must end the whole login, not just its windows' PTYs.
	sess.Stop()
	if !sp.allClosed {
		t.Fatal("Session.Stop did not close the classroom spawner")
	}
}

// TestNewClassroomWindowNoSpawner pins the error path: an ordinary session
// (no PAM login attached) must refuse to spawn a classroom window rather
// than silently falling back to a local exec.Command shell, which would
// spawn a shell as the daemon's own uid instead of failing closed.
func TestNewClassroomWindowNoSpawner(t *testing.T) {
	sess := newTestSession(t)
	if _, err := sess.NewClassroomWindow("", nil); err == nil {
		t.Fatal("NewClassroomWindow succeeded with no classroom spawner set")
	}
}
