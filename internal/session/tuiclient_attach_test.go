package session

import (
	"net"
	"testing"
	"time"
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

// TestAwaitReplyPrefersADeliveredReply covers a daemon that answers and then
// closes the connection, as it does when killing the last session under
// exit_when_empty: the reply and the close are both ready by the time the
// caller waits, and the reply must win every time, not at select's whim.
func TestAwaitReplyPrefersADeliveredReply(t *testing.T) {
	done := make(chan struct{})
	close(done)
	want := &Message{Type: MsgSessionList}
	for i := range 1000 {
		respChan := make(chan *Message, 1)
		respChan <- want
		got, err := awaitReply(respChan, done, time.Second)
		if err != nil || got != want {
			t.Fatalf("iteration %d: got %v, %v; want the delivered reply", i, got, err)
		}
	}

	t.Run("closed without a reply", func(t *testing.T) {
		if _, err := awaitReply(make(chan *Message, 1), done, time.Second); err == nil {
			t.Fatal("want an error when the connection closed with no reply")
		}
	})
}
