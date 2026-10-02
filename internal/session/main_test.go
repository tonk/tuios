package session

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tonk/tuios/internal/testutil"
)

// TestMain isolates the whole test binary from the developer's own XDG
// directories and from their login shell.
func TestMain(m *testing.M) { os.Exit(testutil.RunIsolated(m, pinResurrectionDir, pinShell)) }

// pinResurrectionDir gives the resurrection state a directory of its own.
// Without it, every test that creates a session persists a real state file and
// leaves a phantom session for the next real daemon start to resurrect. Tests
// that need to inspect state files still point the override at their own
// directory; this only provides a safe default for the ones that do not.
func pinResurrectionDir(dir string) {
	setResurrectionDirOverride(filepath.Join(dir, "resurrection"))
}

// pinShell makes the daemon spawn a POSIX shell rather than the developer's
// login shell. A window is a real process, so tests that drive one were
// running whatever $SHELL named, with that shell's startup files and startup
// cost: TestWaitForWindowExit passes under a shell that reaches its prompt
// quickly and times out under one that does not, which makes it a test of the
// machine rather than of the daemon. Windows has no /bin/sh; testutil.Shell
// names the one it does have.
func pinShell(string) {
	if err := os.Setenv("SHELL", testutil.Shell()); err != nil {
		panic(err)
	}
}

// useResurrectionDir points resurrection state at dir and returns a function
// that restores the previous value. Restoring the previous value rather than
// clearing it keeps the TestMain default in place, so a later test cannot fall
// back to the developer's real state directory.
func useResurrectionDir(dir string) func() {
	prev := setResurrectionDirOverride(dir)
	return func() { setResurrectionDirOverride(prev) }
}

// skipWithoutUnixPTY skips a test that opens a PTY itself through creack/pty,
// which has no Windows implementation. Those tests stand in for the PAM
// helper, whose classroom and adoption paths only exist on Unix.
func skipWithoutUnixPTY(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("creack/pty cannot open a PTY on windows")
	}
}
