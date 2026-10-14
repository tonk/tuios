package app

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/tonk/tuios/internal/config"
)

// Double-clicking a URL in a pane opens it in the default browser
// (appearance.open_url_on_double_click). The URL comes from the cell's OSC 8
// hyperlink when the program printed one, otherwise from the double-clicked
// word itself; see FindURL.
//
// Only http and https are ever opened. Whatever a program prints can be
// double-clicked, so a file://, javascript: or custom-scheme link handed to
// the desktop opener would let pane output start arbitrary handlers.
//
// The opener runs on the machine running this OS. That is the user's own for
// a local or attached session, but the server for the SSH server and for
// tuios-web, so those never open anything: the URL is still selected and
// copied, which they can paste into their own browser.

// urlPattern finds an http(s) URL inside a double-clicked word, which may
// carry text the word characters let in around it (a quote, a "url=" prefix).
var urlPattern = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `]+`)

// FindURL returns the http(s) URL a double-click opens: link, the cell's OSC 8
// hyperlink, when it has one, else the first URL inside word. It returns ""
// when neither holds an http(s) URL.
//
// Sentence punctuation directly after a URL is trimmed, along with a closing
// bracket the URL never opened, so "(see https://example.com/a)." opens
// https://example.com/a while https://en.wikipedia.org/wiki/Go_(game) keeps
// its own parenthesis.
func FindURL(link, word string) string {
	if isOpenableURL(link) {
		return link
	}
	raw := urlPattern.FindString(word)
	if raw == "" {
		return ""
	}
	raw = trimURLTail(raw)
	if !isOpenableURL(raw) {
		return ""
	}
	return raw
}

// URLSpans returns the byte ranges [start, end) of the http(s) URLs in a line
// of text, each trimmed the way FindURL trims one. It is what lets a
// double-click select a whole URL: the word characters stop at its ":".
func URLSpans(text string) [][2]int {
	var spans [][2]int
	for _, loc := range urlPattern.FindAllStringIndex(text, -1) {
		raw := trimURLTail(text[loc[0]:loc[1]])
		if isOpenableURL(raw) {
			spans = append(spans, [2]int{loc[0], loc[0] + len(raw)})
		}
	}
	return spans
}

// trimURLTail drops trailing characters that end the sentence around a URL
// rather than the URL itself.
func trimURLTail(raw string) string {
	for raw != "" {
		last := raw[len(raw)-1]
		switch last {
		case '.', ',', ';', ':', '!', '?':
		case ')':
			if strings.Count(raw, "(") >= strings.Count(raw, ")") {
				return raw
			}
		case ']':
			if strings.Count(raw, "[") >= strings.Count(raw, "]") {
				return raw
			}
		case '}':
			if strings.Count(raw, "{") >= strings.Count(raw, "}") {
				return raw
			}
		default:
			return raw
		}
		raw = raw[:len(raw)-1]
	}
	return raw
}

// isOpenableURL reports whether s is an http(s) URL with a host.
func isOpenableURL(s string) bool {
	if s == "" {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// CanOpenURLs reports whether a URL opened here would reach the user's own
// browser: on, and not an SSH or tuios-web session, whose host is the server.
func (m *OS) CanOpenURLs() bool {
	return config.OpenURLOnDoubleClick && !m.IsSSHMode && !m.IsWebSession
}

// startURLOpener hands a URL to the platform's opener without waiting for it.
// A variable so tests can stand in for the desktop.
var startURLOpener = func(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		// xdg-open over a console with no graphical session falls back to a
		// text browser on the controlling terminal, which is ours.
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return fmt.Errorf("no graphical display")
		}
		cmd = exec.Command("xdg-open", target)
	}
	// Keep the opener off the PTY the TUI draws on.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// SetURLOpener replaces what OpenURL starts and returns a func restoring the
// previous one. It exists for tests, which must not launch a real browser.
func SetURLOpener(fn func(target string) error) (restore func()) {
	prev := startURLOpener
	startURLOpener = fn
	return func() { startURLOpener = prev }
}

// OpenURL opens target in the default browser and says so. copied adds that
// the URL is on the clipboard too, so the gesture shows a single toast.
func (m *OS) OpenURL(target string, copied bool) {
	if err := startURLOpener(target); err != nil {
		m.ShowNotification(fmt.Sprintf("Could not open URL: %v", err), "warning", config.NotificationDuration)
		return
	}
	msg := "Opening " + target
	if copied {
		msg += " (copied)"
	}
	m.ShowNotification(msg, "info", config.NotificationDuration)
}
