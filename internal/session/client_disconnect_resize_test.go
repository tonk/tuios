package session

import (
	"testing"
	"time"
)

// TestSessionResizesUpAfterUngracefulClientDisconnect is the regression test
// for a real bug confirmed live: a browser tab reloading or closing never
// sends an explicit MsgDetach - the daemon just sees its connection go away,
// if it notices at all. handleConnection's own EOF-triggered cleanup
// (daemon.go) removed the client from d.clients and unsubscribed its PTYs,
// but never called notifyClientLeft, so calculateEffectiveSize's min-of-all-
// attached-clients (daemon_size.go) never re-derived a smaller session's
// width/height back up once the client that had shrunk it was gone - nothing
// else ever recalculates except another client joining or leaving, and this
// was the one ordinary way a client leaves that never counted as "leaving" at
// all. Confirmed live: a classroom trainer-console session's shared terminal
// stayed clamped to a stale, narrower client's size indefinitely, surviving
// browser reloads and even a full OS-level browser restart - only
// restarting tuios-web itself (dropping every connection to the daemon at
// once) ever recovered it.
//
// This drives two real TUIClient connections against a real, started daemon:
// a wide one creates the session, a narrow one attaches and clamps it down,
// and then the narrow one's connection is closed directly (Close, not
// Detach) - exactly what a browser tab disappearing looks like from the
// daemon's side. The session's size must recalculate back up to the
// remaining (wide) client's own size, the same way it already would if the
// narrow client had sent a real MsgDetach first.
func TestSessionResizesUpAfterUngracefulClientDisconnect(t *testing.T) {
	d, _ := startTestDaemon(t)

	wide := NewTUIClient()
	if err := wide.ConnectWithCapabilities("test-wide", 200, 60, nil); err != nil {
		t.Fatalf("wide Connect: %v", err)
	}
	defer func() { _ = wide.Close() }()
	if _, err := wide.AttachSession("shared", true, 200, 60, false); err != nil {
		t.Fatalf("wide AttachSession: %v", err)
	}

	narrow := NewTUIClient()
	if err := narrow.ConnectWithCapabilities("test-narrow", 80, 24, nil); err != nil {
		t.Fatalf("narrow Connect: %v", err)
	}
	if _, err := narrow.AttachSession("shared", false, 80, 24, false); err != nil {
		t.Fatalf("narrow AttachSession: %v", err)
	}

	sess := d.manager.GetSession("shared")
	if sess == nil {
		t.Fatal("session \"shared\" not found")
	}

	// The join alone must already have clamped the shared size down to the
	// narrow client's own - otherwise the rest of this test cannot tell a
	// "recalculated back up" from a "was never clamped in the first place".
	w, h := sess.Size()
	if w != 80 || h != 24 {
		t.Fatalf("session size after narrow client joined = %dx%d, want 80x24 (clamped to the narrow client)", w, h)
	}

	// This is the disconnect under test: a plain Close, the same thing a
	// browser tab reloading or closing does - no MsgDetach, nothing graceful.
	if err := narrow.Close(); err != nil {
		t.Fatalf("narrow Close: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if w, h := sess.Size(); w == 200 && h == 60 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	w, h = sess.Size()
	t.Fatalf("session size after the narrow client's connection closed = %dx%d, want 200x60 (recalculated back up to the remaining wide client) - notifyClientLeft was not called for this disconnect", w, h)
}
