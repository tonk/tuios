package session

import (
	"net"
	"testing"
)

// TestAttachSkipsBroadcastsAheadOfTheReply reproduces an attach that failed
// with "unexpected response: 48" under load: the daemon registers the
// connection with the session before replying, so a broadcast such as
// MsgClientLeft can reach the attaching client first.
func TestAttachSkipsBroadcastsAheadOfTheReply(t *testing.T) {
	tests := []struct {
		name  string
		ahead []MessageType
	}{
		{"none", nil},
		{"client left", []MessageType{MsgClientLeft}},
		{"joined, resized, synced", []MessageType{MsgClientJoined, MsgSessionResize, MsgStateSync}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, client := net.Pipe()
			defer server.Close()
			c := newTestTUIClient(client)
			defer c.Close()

			go func() {
				if _, _, err := ReadMessageWithCodec(server); err != nil {
					return
				}
				for _, typ := range tt.ahead {
					msg, _ := NewMessageWithCodec(typ, &ClientLeftPayload{ClientID: "other"}, DefaultCodec())
					_ = WriteMessageWithCodec(server, msg, DefaultCodec())
				}
				reply, _ := NewMessageWithCodec(MsgAttached, &AttachedPayload{
					SessionName: "s", SessionID: "id", State: &SessionState{Name: "s"},
				}, DefaultCodec())
				_ = WriteMessageWithCodec(server, reply, DefaultCodec())
			}()

			state, err := c.AttachSession("s", false, 80, 24, false)
			if err != nil {
				t.Fatalf("AttachSession: %v", err)
			}
			if state == nil || state.Name != "s" {
				t.Fatalf("state = %+v, want session s", state)
			}
		})
	}
}
