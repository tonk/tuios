package app

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

// hostClipTimeout bounds one call to a clipboard helper. xclip and wl-copy
// fork into the background to keep owning the selection; the call itself only
// has to hand the text over, so anything slower is a hung helper, not work.
const hostClipTimeout = 2 * time.Second

// hostClipTool is one way to reach the display server's clipboard: the argv
// that writes stdin to the clipboard, the argv that prints it, and the argv
// that writes stdin to the primary selection. The primary selection is what
// Shift+Insert and a middle click paste in most terminals, so a copy fills
// both, the way selecting text does in any X11 terminal.
type hostClipTool struct {
	write   []string
	read    []string
	primary []string
}

// hostClipTools lists the helpers by display server, most specific first.
var (
	waylandClipTools = []hostClipTool{
		{[]string{"wl-copy"}, []string{"wl-paste", "--no-newline"}, []string{"wl-copy", "--primary"}},
	}
	x11ClipTools = []hostClipTool{
		{[]string{"xclip", "-selection", "clipboard", "-in"}, []string{"xclip", "-selection", "clipboard", "-out"}, []string{"xclip", "-selection", "primary", "-in"}},
		{[]string{"xsel", "--clipboard", "--input"}, []string{"xsel", "--clipboard", "--output"}, []string{"xsel", "--primary", "--input"}},
	}
)

// pickHostClipTool returns the first helper whose binary is found, for the
// display server the environment names. It returns nil when there is no
// display or no helper, which leaves OSC 52 as the only route.
func pickHostClipTool(getenv func(string) string, lookPath func(string) (string, error)) *hostClipTool {
	var candidates []hostClipTool
	if getenv("WAYLAND_DISPLAY") != "" {
		candidates = append(candidates, waylandClipTools...)
	}
	if getenv("DISPLAY") != "" {
		candidates = append(candidates, x11ClipTools...)
	}
	for i := range candidates {
		if _, err := lookPath(candidates[i].write[0]); err == nil {
			return &candidates[i]
		}
	}
	return nil
}

var (
	hostClipOnce sync.Once
	hostClip     *hostClipTool
)

// hostClipboardTool resolves the helper once; the environment does not change
// under a running client.
func hostClipboardTool() *hostClipTool {
	hostClipOnce.Do(func() {
		hostClip = pickHostClipTool(os.Getenv, exec.LookPath)
	})
	return hostClip
}

// nativeClipboard returns the helper to use for this client, or nil. Over SSH
// or in a web session the display belongs to the server, not to the person
// sitting at the terminal, so the clipboard there is OSC 52's to carry.
func (m *OS) nativeClipboard() *hostClipTool {
	if m.IsSSHMode || m.IsWebSession {
		return nil
	}
	return hostClipboardTool()
}

// SetHostClipboard writes text to the clipboard. OSC 52 always goes out, since
// it reaches a terminal on the far side of an SSH hop. When the client has a
// display of its own it also writes through the display's clipboard helper,
// because terminals built on VTE (xfce4-terminal, gnome-terminal) ignore OSC 52
// and the copy would otherwise vanish without a trace.
func (m *OS) SetHostClipboard(text string) tea.Cmd {
	osc := tea.SetClipboard(text)
	tool := m.nativeClipboard()
	if tool == nil {
		return osc
	}
	return tea.Batch(osc, func() tea.Msg {
		for _, argv := range [][]string{tool.write, tool.primary} {
			ctx, cancel := context.WithTimeout(context.Background(), hostClipTimeout)
			// #nosec G204 - argv comes from the fixed tables above
			cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
			cmd.Stdin = strings.NewReader(text)
			// Stdout and Stderr stay nil: the helper forks to hold the
			// selection, and an inherited pipe would make Run wait for that
			// child.
			_ = cmd.Run()
			cancel()
		}
		return nil
	})
}

// ReadHostClipboard asks for the clipboard's contents, delivered as a
// tea.ClipboardMsg. It prefers the display's own clipboard when there is one,
// and falls back to an OSC 52 read, which VTE-based terminals never answer.
func (m *OS) ReadHostClipboard() tea.Cmd {
	tool := m.nativeClipboard()
	if tool == nil {
		return tea.ReadClipboard
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), hostClipTimeout)
		defer cancel()
		// #nosec G204 - argv comes from the fixed tables above
		out, err := exec.CommandContext(ctx, tool.read[0], tool.read[1:]...).Output()
		if err != nil {
			return tea.ReadClipboard()
		}
		return tea.ClipboardMsg{Content: string(out), Selection: 'c'}
	}
}
