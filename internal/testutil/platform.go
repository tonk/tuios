package testutil

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// SocketDir gives the test a daemon socket directory of its own, points the
// socket path lookup at it, and returns it.
//
// t.TempDir is the wrong directory for this. A Unix socket path is limited to
// 104 bytes on macOS, and t.TempDir nests the full test name under a TMPDIR
// that is already some 50 bytes long, so a test with a long name fails to bind
// with "invalid argument" before it tests anything. A short prefix directly
// under the temp directory keeps the path, including the ".classroom" handoff
// socket beside it, inside the limit.
//
// Windows ignores XDG_RUNTIME_DIR and derives the socket from LOCALAPPDATA, so
// that moves too; otherwise every test there would share one socket in the
// developer's own profile.
func SocketDir(t testing.TB) string {
	t.Helper()
	dir := ShortTempDir(t)
	t.Setenv("XDG_RUNTIME_DIR", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", dir)
	}
	return dir
}

// ShortTempDir is t.TempDir for a test that binds a Unix socket in it: a
// directory removed when the test ends, but one whose path leaves room for a
// socket name under the platform's limit. See SocketDir for why t.TempDir
// does not.
func ShortTempDir(t testing.TB) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "tuios")
	if err != nil {
		t.Fatalf("create socket directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// RequireUnixPacket skips the test on a platform without SOCK_SEQPACKET Unix
// sockets. The PAM helper and the classroom login handoff both speak
// unixpacket, which Linux has and macOS and Windows do not, so there is
// nothing for these tests to exercise there. Probing for it rather than
// naming operating systems keeps the tests running wherever it does exist.
func RequireUnixPacket(t testing.TB) {
	t.Helper()
	dir, err := os.MkdirTemp("", "tuios")
	if err != nil {
		t.Fatalf("create probe directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	ln, err := net.Listen("unixpacket", filepath.Join(dir, "probe.sock"))
	if err != nil {
		t.Skipf("unixpacket sockets are not available here: %v", err)
	}
	_ = ln.Close()
}

// Shell is the shell tests run their panes under: /bin/sh, which every Unix
// has and which reaches its prompt without reading the developer's startup
// files, or cmd.exe on Windows, which has no /bin/sh.
func Shell() string {
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	return "/bin/sh"
}

// Cat is the path of cat for a test that wants a pane which echoes its input
// straight back, and skips the test where there is none, as on a stock
// Windows machine.
func Cat(t testing.TB) string {
	t.Helper()
	path, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("no cat on this machine to echo the pane's input")
	}
	return path
}
