package session

import "testing"

// TestClientIDsNeverRepeat is the regression test for the session-ended
// notification that sometimes never reached a client on Windows. Connection
// IDs came from the wall clock alone, which on Windows ticks coarsely enough
// that two connections accepted back to back shared one, and the second
// silently replaced the first in the daemon's client table. Generating IDs as
// fast as possible is the worst case a coarse clock can produce.
func TestClientIDsNeverRepeat(t *testing.T) {
	const n = 100_000
	seen := make(map[string]struct{}, n)
	for range n {
		id := newClientID()
		if _, dup := seen[id]; dup {
			t.Fatalf("client ID %q handed out twice", id)
		}
		seen[id] = struct{}{}
	}
}
