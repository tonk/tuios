package main

import (
	"os"
	"testing"
)

// TestUtmpLoginLogoutRoundTrip exercises the real pututxline/updwtmpx path.
// Writing /var/run/utmp normally needs root (or membership of the utmp
// group); without that the test skips rather than failing a regular
// `go test ./helper` run.
func TestUtmpLoginLogoutRoundTrip(t *testing.T) {
	f, err := os.OpenFile("/var/run/utmp", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("cannot write /var/run/utmp (need root/utmp group): %v", err)
	}
	_ = f.Close()

	const line = "pts/tuios-test"
	utmpLogin("tuios-testuser", line, "203.0.113.50", os.Getpid())
	utmpLogout(line, os.Getpid())
}