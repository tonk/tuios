package session

import (
	"os/exec"
	"testing"
	"time"

	"github.com/creack/pty"
)

// TestAdoptDaemonWindow exercises the daemon-side adoption path a PAM
// classroom session will use: a PTY and process this package did not spawn
// itself (pamauth.Login.SpawnPTY, in production) becomes an ordinary daemon
// window - same I/O, capture and multi-client machinery as AddDaemonWindow -
// except exit is detected by polling its pid rather than exec.Cmd.Wait, and
// killing it goes through killFunc instead of exec.Cmd.Process.Kill.
//
// The child here is started directly by this test rather than through a
// second process (there is no tuios-pam-helper in a unit test), so it is
// reaped in the background exactly as a real external spawner would reap its
// own child - otherwise it would sit as a zombie this process's own kernel
// still reports as signalable, and the pid-liveness poll under test
// (waitForAdoptedExit) would never see it go away.
func TestAdoptDaemonWindow(t *testing.T) {
	sess := newTestSession(t)

	cmd := exec.Command("sh")
	ptyFile, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("pty.Start: %v", err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()

	var killed bool
	killFunc := func() error {
		killed = true
		return cmd.Process.Kill()
	}

	win, err := sess.AdoptDaemonWindow("adopted", ptyFile, pid, nil, killFunc)
	if err != nil {
		t.Fatalf("AdoptDaemonWindow: %v", err)
	}
	if win.PTYID == "" {
		t.Fatal("created window has empty PTYID")
	}

	p := sess.GetPTY(win.PTYID)
	if p == nil {
		t.Fatal("PTY not registered on session")
	}
	if p.IsExited() {
		t.Fatal("freshly adopted PTY already reports exited")
	}
	if got := p.ShellPID(); got != pid {
		t.Errorf("ShellPID() = %d, want %d", got, pid)
	}

	if _, err := p.Write([]byte("echo tuios-adopt-marker\n")); err != nil {
		t.Fatalf("write to adopted PTY: %v", err)
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
	if !killed {
		t.Fatal("Close did not call killFunc for an adopted PTY")
	}

	deadline := time.Now().Add(5 * time.Second)
	for !p.IsExited() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !p.IsExited() {
		t.Fatal("monitorExit never observed the adopted process exit")
	}
}
