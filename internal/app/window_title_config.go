package app

import (
	"strings"

	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/terminal"
)

// titleUserForFormat picks the username appearance.initial_title_format's
// {user} should expand to for this client's windows.
//
// Prefer InitialTitleUser when the caller set one (classroom web paths close
// the PAM Login before NewOS, so PAMLogin is nil there). Next, a live
// PAMLogin's username - except when this client is a trainer attached to
// someone else's session, in which case the panes belong to SessionName.
// Empty means FormatInitialTitleForUser falls back to the process user.
func (m *OS) titleUserForFormat() string {
	if m.InitialTitleUser != "" {
		return m.InitialTitleUser
	}
	if m.PAMLogin != nil {
		u := m.PAMLogin.Username()
		if m.IsDaemonSession && m.SessionName != "" && m.SessionName != u {
			return m.SessionName
		}
		return u
	}
	return ""
}

// configuredWindowTitle is the title appearance.initial_title_format expands
// to for this client, or "" when the format is unset.
func (m *OS) configuredWindowTitle() string {
	return config.FormatInitialTitleForUser(m.titleUserForFormat())
}

// applyConfiguredWindowTitle applies this client's appearance.initial_title_format
// / lock_titles onto one window. tuios-web loads those settings from --config;
// classroom windows are created by the daemon, which may load a different
// config file - without this, a web-only initial_title_format never reaches
// the panes a browser sees.
//
// When lock_titles is on, the format title is forced (replacing a shell OSC
// title the daemon may have echoed). When lock_titles is off, the format only
// fills in a still-generic "Terminal <id>" title, matching creation-time
// semantics.
func (m *OS) applyConfiguredWindowTitle(w *terminal.Window) bool {
	if w == nil {
		return false
	}
	changed := false
	if want := m.configuredWindowTitle(); want != "" {
		cur := w.Title()
		force := config.LockTitles || strings.HasPrefix(cur, "Terminal ")
		if force && cur != want {
			w.SetTitle(want)
			changed = true
		}
	}
	if config.LockTitles && !w.TitleLocked() {
		w.SetTitleLocked(true)
		changed = true
	}
	return changed
}

// applyConfiguredTitlesToWindows runs applyConfiguredWindowTitle across every
// window and reports whether any title or lock bit changed (so the caller can
// SyncStateToDaemon and stop the daemon re-echoing a shell OSC title).
func (m *OS) applyConfiguredTitlesToWindows() bool {
	changed := false
	for _, w := range m.Windows {
		if m.applyConfiguredWindowTitle(w) {
			changed = true
		}
	}
	return changed
}
