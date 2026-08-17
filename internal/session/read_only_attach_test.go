package session

import (
	"testing"
	"time"
)

// TestReadOnlyAttachEchoesBack is the wire round trip: AttachPayload.ReadOnly
// reaches connState.readOnly and is echoed back in AttachedPayload, which is
// what TUIClient.IsReadOnly (and so app.OSOptions.ReadOnly) trusts.
func TestReadOnlyAttachEchoesBack(t *testing.T) {
	d, _ := startTestDaemon(t)
	if _, err := d.manager.CreateSession("ro-attach", &SessionConfig{}, 80, 24); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	c := NewTUIClient()
	if err := c.Connect("test", 80, 24); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.AttachSession("ro-attach", false, 80, 24, true); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if !c.IsReadOnly() {
		t.Error("AttachSession(..., readOnly=true) did not come back read-only")
	}
}

// TestReadOnlyCreatePTYRefused covers handleCreatePTY: window creation over
// the wire (what a keybinding ultimately triggers) must fail for a read-only
// client. CreatePTY waits for a response, so the refusal is a plain error.
func TestReadOnlyCreatePTYRefused(t *testing.T) {
	d, _ := startTestDaemon(t)
	if _, err := d.manager.CreateSession("ro-create", &SessionConfig{}, 80, 24); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	c := NewTUIClient()
	if err := c.Connect("test", 80, 24); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.AttachSession("ro-create", false, 80, 24, true); err != nil {
		t.Fatalf("attach: %v", err)
	}

	if _, err := c.CreatePTY("new window", "", 80, 24); err == nil {
		t.Error("CreatePTY succeeded from a read-only client")
	}
}

// TestReadOnlyClosePTYRefused covers handleClosePTY. ClosePTY is fire-and-
// forget on the wire, so the refusal is observed as the PTY surviving rather
// than as a returned error.
func TestReadOnlyClosePTYRefused(t *testing.T) {
	d, _ := startTestDaemon(t)
	sess, err := d.manager.CreateSession("ro-close", &SessionConfig{Shell: "/bin/cat"}, 80, 24)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	pty, err := sess.CreatePTY("win-1", 80, 24, func(string) {})
	if err != nil {
		t.Fatalf("CreatePTY: %v", err)
	}

	c := NewTUIClient()
	if err := c.Connect("test", 80, 24); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.AttachSession("ro-close", false, 80, 24, true); err != nil {
		t.Fatalf("attach: %v", err)
	}

	if err := c.ClosePTY(pty.ID); err != nil {
		t.Fatalf("ClosePTY send: %v", err)
	}

	// ClosePTY's refusal has no reply to wait for; give the daemon a moment to
	// have processed it (and be wrong) before checking it did not.
	time.Sleep(200 * time.Millisecond)
	if sess.GetPTY(pty.ID) == nil {
		t.Error("a read-only client's ClosePTY closed the window")
	}
}

// TestReadOnlyInputNeverReachesPTY covers handleInput, the core case: keyboard
// and mouse bytes from a read-only client must never reach the PTY. /bin/cat
// echoes anything it receives on stdin straight to stdout, so a silent,
// deterministic pass/fail is the PTY's outputSeq staying flat.
func TestReadOnlyInputNeverReachesPTY(t *testing.T) {
	d, _ := startTestDaemon(t)
	sess, err := d.manager.CreateSession("ro-input", &SessionConfig{Shell: "/bin/cat"}, 80, 24)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	pty, err := sess.CreatePTY("win-1", 80, 24, func(string) {})
	if err != nil {
		t.Fatalf("CreatePTY: %v", err)
	}

	outputSeq := func() int64 {
		pty.outputMu.RLock()
		defer pty.outputMu.RUnlock()
		return pty.outputSeq
	}

	c := NewTUIClient()
	if err := c.Connect("test", 80, 24); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.AttachSession("ro-input", false, 80, 24, true); err != nil {
		t.Fatalf("attach: %v", err)
	}

	before := outputSeq()
	if err := c.WritePTY(pty.ID, []byte("hello\n")); err != nil {
		t.Fatalf("WritePTY: %v", err)
	}

	// cat's echo, if delivered, lands in well under this; the wait is there to
	// give a bug time to manifest, not because success is expected to arrive.
	time.Sleep(300 * time.Millisecond)
	if after := outputSeq(); after != before {
		t.Errorf("a read-only client's input reached the PTY: outputSeq %d -> %d", before, after)
	}
}

// TestReadWriteInputStillReachesPTY is the control for the test above: with
// readOnly left off, the same input must still work, so the gate is proven to
// be the reason for the silence, not a broken test fixture.
func TestReadWriteInputStillReachesPTY(t *testing.T) {
	d, _ := startTestDaemon(t)
	sess, err := d.manager.CreateSession("rw-input", &SessionConfig{Shell: "/bin/cat"}, 80, 24)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	pty, err := sess.CreatePTY("win-1", 80, 24, func(string) {})
	if err != nil {
		t.Fatalf("CreatePTY: %v", err)
	}

	c := NewTUIClient()
	if err := c.Connect("test", 80, 24); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.AttachSession("rw-input", false, 80, 24, false); err != nil {
		t.Fatalf("attach: %v", err)
	}

	if err := c.WritePTY(pty.ID, []byte("hello\n")); err != nil {
		t.Fatalf("WritePTY: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pty.outputMu.RLock()
		seq := pty.outputSeq
		pty.outputMu.RUnlock()
		if seq > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("a normal (non-read-only) client's input never reached the PTY within 2s")
}

// TestReadOnlyKillSessionRefused covers handleKill. KillSessionByName waits
// for a response, so the refusal is a plain error, and the session must
// still be there afterward.
func TestReadOnlyKillSessionRefused(t *testing.T) {
	d, _ := startTestDaemon(t)
	if _, err := d.manager.CreateSession("ro-kill", &SessionConfig{}, 80, 24); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	c := NewTUIClient()
	if err := c.Connect("test", 80, 24); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.AttachSession("ro-kill", false, 80, 24, true); err != nil {
		t.Fatalf("attach: %v", err)
	}

	if err := c.KillSessionByName("ro-kill"); err == nil {
		t.Error("KillSessionByName succeeded from a read-only client")
	}
	if d.manager.GetSession("ro-kill") == nil {
		t.Error("a read-only client's kill request destroyed the session")
	}
}
