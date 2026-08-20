package app

import (
	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/charmbracelet/x/ansi"
)

// hostTerminalTitle is what the host terminal's window/tab shows when no
// window is focused (or the focused window has not set a title of its own):
// the fallback that answers "the host says Ghostty/foo, not tuios" for an
// otherwise-blank pane.
const hostTerminalTitle = "tuios"

// applyTerminalTitle sets the host terminal's window title (OSC 2) once,
// unconditionally - used at Init and when the setting is toggled back on, to
// put a title on screen immediately rather than waiting for the next sync
// point. syncHostTitle is what keeps it live afterward.
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
	m.lastHostTitle = hostTerminalTitle
	m.KittyPassthrough.WriteToHost([]byte(ansi.SetWindowTitle(hostTerminalTitle)))
}

// syncHostTitle keeps the host terminal's window title following whatever the
// focused pane has titled itself, the same way running that program directly
// (no tuios in between) would. A host like Kitty shows a child's OSC-set
// title in its own window title, which is what a status-bar/taskbar applet
// reads; pinning the host title to the fixed "tuios" string forever (the old
// behavior) meant that surface went dead the moment tuios sat in front of it.
//
// Falls back to hostTerminalTitle when there is no focused window or it has
// not set a title yet, so an empty pane still reads as tuios rather than
// carrying over a stale title from whatever was focused before it. Writes
// only on an actual change, so this is cheap to call from a hot path (every
// PTYDataMsg) as well as from the maintenance tick's slower fallback.
func (m *OS) syncHostTitle() {
	if !config.SetTerminalTitle || m.KittyPassthrough == nil {
		return
	}
	title := hostTerminalTitle
	if w := m.GetFocusedWindow(); w != nil {
		if t := w.Title(); t != "" {
			title = t
		}
	}
	if title == m.lastHostTitle {
		return
	}
	m.lastHostTitle = title
	m.KittyPassthrough.WriteToHost([]byte(ansi.SetWindowTitle(title)))
}
