package input

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tonk/tuios/internal/app"
	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/terminal"
	"github.com/tonk/tuios/internal/vt"
)

// TestDoubleLeaderSendsConfiguredLeader pins that pressing the leader twice in
// terminal mode forwards the configured leader key to the pane, not a
// hard-coded Ctrl+B byte.
func TestDoubleLeaderSendsConfiguredLeader(t *testing.T) {
	tests := []struct {
		name   string
		leader string
		msg    tea.KeyPressMsg
		want   string
	}{
		{"default ctrl+b", "ctrl+b", tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}, "\x02"},
		{"ctrl+a", "ctrl+a", tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}, "\x01"},
		{"ctrl+space", "ctrl+space", tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModCtrl}, "\x00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved := config.LeaderKey
			config.LeaderKey = tt.leader
			t.Cleanup(func() { config.LeaderKey = saved })

			em := vt.NewEmulator(80, 24)
			t.Cleanup(func() { _ = em.Close() })
			pty := &capturePty{}
			win := &terminal.Window{ID: "leader-0001", Terminal: em, Pty: pty, Width: 82, Height: 26}
			o := &app.OS{Mode: app.TerminalMode, FocusedWindow: 0, Windows: []*terminal.Window{win}}

			if tt.msg.String() != tt.leader {
				t.Fatalf("test key %q does not match leader %q", tt.msg.String(), tt.leader)
			}
			HandleTerminalModeKey(tt.msg, o)
			if !o.PrefixActive {
				t.Fatal("first leader press did not activate prefix mode")
			}
			HandleTerminalModeKey(tt.msg, o)
			if o.PrefixActive {
				t.Error("second leader press left prefix mode active")
			}
			if got := string(pty.got); got != tt.want {
				t.Errorf("pane received %q, want %q", got, tt.want)
			}
		})
	}
}
