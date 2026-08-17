package app

import (
	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/charmbracelet/x/ansi"
)

// hostTerminalTitle is what the host terminal's window/tab shows once tuios
// sets it. A fixed string rather than something dynamic (session name, focused
// window): the ask this answers is "the host says Ghostty/foo, not tuios",
// and set_terminal_title is the on/off switch, not a template.
const hostTerminalTitle = "tuios"

// applyTerminalTitle sets the host terminal's window title (OSC 2), so a host
// that otherwise shows its own name (Ghostty, etc.) shows tuios's instead.
//
// It goes through KittyPassthrough.WriteToHost, which is always non-nil in
// every real entrypoint (EnableGraphicsPassthrough is unconditionally true in
// cmd/tuios and internal/server/ssh.go) and already routes to the right
// writer for the mode this client is running in - local stdout/tty, the SSH
// session, or the web session's PTY - the same way a guest's OSC 9 desktop
// notification already reaches the host in notify.go. Called once from Init,
// so each attached client sets its own host terminal's title independently.
func (m *OS) applyTerminalTitle() {
	if !config.SetTerminalTitle || m.KittyPassthrough == nil {
		return
	}
	m.KittyPassthrough.WriteToHost([]byte(ansi.SetWindowTitle(hostTerminalTitle)))
}
