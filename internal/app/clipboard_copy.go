package app

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tonk/tuios/internal/config"
)

// PendingCopyMsg fires when a multi-click selection's window has closed and the
// gesture can finally be read as what it was. Seq names the gesture it belongs
// to; a message whose sequence has been superseded is dropped.
type PendingCopyMsg struct {
	Seq uint64
}

// The clipboard write for a multi-click selection is deferred, because until
// the multi-click window closes nobody knows what the gesture is.
//
// A triple-click arrives as a double-click followed by a third press. Copying
// on each release wrote the word first and the line second, so every
// triple-click clobbered the clipboard with the wrong text on the way to the
// right one, and a paste that landed in between got the word. Waiting until no
// further click can join the gesture means only its final reading is ever
// written.
//
// Only the write waits. The highlight is applied on the press, so a
// double-click still shows the word instantly and visibly widens to the line
// on the third click. Deferring the feedback as well would trade one problem
// for a laggier one.

// CopyToClipboard writes text to the system clipboard now and says so. It is
// the unambiguous path: a drag release, where the gesture ended when the button
// came up and nothing can supersede it.
func (m *OS) CopyToClipboard(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	m.CancelPendingCopy()
	m.ShowNotification(fmt.Sprintf("Copied %d chars", len(text)), "success", config.NotificationDuration)
	return m.SetHostClipboard(text)
}

// DeferCopyToClipboard holds text back until delay has passed with no further
// click, then writes it. A press arriving in the meantime calls
// CancelPendingCopy and this write never happens.
//
// The text is captured now rather than re-read when the timer fires: it is
// correct at this instant, and the pane may have scrolled or been closed by the
// time the message arrives.
func (m *OS) DeferCopyToClipboard(text string, delay time.Duration) tea.Cmd {
	return m.DeferSelection(text, "", delay)
}

// DeferSelection is DeferCopyToClipboard for a double-click that may also open
// a URL: both wait out the multi-click window together, so a triple-click on a
// URL selects its line without having started a browser on the way. Either
// may be empty; with both empty there is nothing to defer.
func (m *OS) DeferSelection(text, url string, delay time.Duration) tea.Cmd {
	if text == "" && url == "" {
		return nil
	}
	m.selectionSeq++
	m.pendingCopy = text
	m.pendingURL = url
	seq := m.selectionSeq
	return tea.Tick(max(delay, time.Millisecond), func(time.Time) tea.Msg {
		return PendingCopyMsg{Seq: seq}
	})
}

// CancelPendingCopy discards a deferred write and makes its timer a no-op when
// it fires. Every press calls this, so the clipboard only ever ends up holding
// what the last completed gesture selected.
func (m *OS) CancelPendingCopy() {
	m.selectionSeq++
	m.pendingCopy = ""
	m.pendingURL = ""
}

// HandlePendingCopy performs a deferred write if seq is still the current
// gesture. A superseded sequence means a later click reinterpreted the
// selection, and the text this timer was carrying was never what the user
// ended up with.
func (m *OS) HandlePendingCopy(seq uint64) tea.Cmd {
	if seq != m.selectionSeq || (m.pendingCopy == "" && m.pendingURL == "") {
		return nil
	}
	text, url := m.pendingCopy, m.pendingURL
	m.pendingCopy, m.pendingURL = "", ""
	if url != "" {
		// One toast for the gesture: opening says it all, and the copy rides
		// along in it rather than stacking a second notification.
		m.OpenURL(url, text != "")
	} else {
		m.ShowNotification(fmt.Sprintf("Copied %d chars", len(text)), "success", config.NotificationDuration)
	}
	if text == "" {
		return nil
	}
	return m.SetHostClipboard(text)
}

// PendingCopyText reports the text a deferred write is holding, for tests and
// for anything that needs to know a gesture has not settled yet.
func (m *OS) PendingCopyText() string { return m.pendingCopy }

// PendingURL reports the URL a settled double-click is waiting to open.
func (m *OS) PendingURL() string { return m.pendingURL }
