package session

import (
	"testing"
	"time"
)

// waitForDaemonContextDone reports whether the daemon's context was cancelled
// within timeout - a proxy for "Run() would now call shutdown()" without
// actually running Run() (which blocks) in a test.
func waitForDaemonContextDone(t *testing.T, d *Daemon, timeout time.Duration) bool {
	t.Helper()
	select {
	case <-d.ctx.Done():
		return true
	case <-time.After(timeout):
		return false
	}
}

// TestExitWhenEmptyCancelsDaemonOnLastSessionKilled is the feature itself:
// killing the only session with exit_when_empty on signals the daemon to
// shut down, the way a real SIGTERM (tuios kill-server) would.
func TestExitWhenEmptyCancelsDaemonOnLastSessionKilled(t *testing.T) {
	d, _ := startTestDaemon(t)
	d.exitWhenEmpty = true

	makeSessionWithWindow(t, d, "only-one")
	client := attachTestClient(t, "only-one")

	if err := client.KillSessionByName("only-one"); err != nil {
		t.Fatalf("KillSessionByName reported an error killing the last session (the client's confirmation should not race the daemon's own shutdown): %v", err)
	}

	if !waitForDaemonContextDone(t, d, 2*time.Second) {
		t.Error("killing the only session with exit_when_empty=true did not signal the daemon to shut down")
	}
}

// TestExitWhenEmptyLeavesDaemonRunningWithSessionsRemaining is the other half:
// killing one of several sessions must never trigger a shutdown while others
// are still live.
func TestExitWhenEmptyLeavesDaemonRunningWithSessionsRemaining(t *testing.T) {
	d, _ := startTestDaemon(t)
	d.exitWhenEmpty = true

	makeSessionWithWindow(t, d, "keep-me")
	makeSessionWithWindow(t, d, "kill-me")
	client := attachTestClient(t, "kill-me")

	if err := client.KillSessionByName("kill-me"); err != nil {
		t.Fatalf("KillSessionByName: %v", err)
	}

	if waitForDaemonContextDone(t, d, 300*time.Millisecond) {
		t.Error("killing one of several sessions shut the daemon down; a session (keep-me) was still live")
	}
	if d.manager.SessionCount() != 1 {
		t.Errorf("SessionCount() = %d, want 1", d.manager.SessionCount())
	}
}

// TestExitWhenEmptyViaKillSessionVerb covers the other trigger path: the JSON
// kill-session verb (what `tuios kill-session` uses), whose response is
// written by the dispatch loop after the handler returns rather than by the
// handler itself - the reason that path defers the exit with a short delay
// instead of the direct ordering handleKill uses.
func TestExitWhenEmptyViaKillSessionVerb(t *testing.T) {
	d, sp := startTestDaemon(t)
	d.exitWhenEmpty = true
	makeSessionWithWindow(t, d, "doomed")

	c := dialVerb(t, sp)
	res := result(t, c.call(t, `{"verb":"kill-session","params":{"session":"doomed"}}`))
	if res["type"] != "ok" {
		t.Fatalf("kill-session result = %v, want ok", res["type"])
	}

	if !waitForDaemonContextDone(t, d, 2*time.Second) {
		t.Error("kill-session on the only session with exit_when_empty=true did not signal the daemon to shut down")
	}
}

// TestExitWhenEmptyOffLeavesDaemonRunning is the default-off control: without
// exit_when_empty, killing the last session must never touch the daemon
// itself, matching every daemon behavior before this option existed.
func TestExitWhenEmptyOffLeavesDaemonRunning(t *testing.T) {
	d, _ := startTestDaemon(t)
	// exitWhenEmpty left at its zero value (false).

	makeSessionWithWindow(t, d, "only-one")
	client := attachTestClient(t, "only-one")

	if err := client.KillSessionByName("only-one"); err != nil {
		t.Fatalf("KillSessionByName: %v", err)
	}

	if waitForDaemonContextDone(t, d, 300*time.Millisecond) {
		t.Error("killing the last session shut the daemon down with exit_when_empty left off")
	}
}
