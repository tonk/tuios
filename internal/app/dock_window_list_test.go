package app

import (
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/terminal"
)

// withDockWindowList sets config.DockWindowList for one test and restores it.
func withDockWindowList(t *testing.T, v bool) {
	t.Helper()
	prev := config.DockWindowList
	config.DockWindowList = v
	t.Cleanup(func() { config.DockWindowList = prev })
}

// dockListOS builds an OS with three windows on the current workspace - two
// on screen, one minimized - and a fourth on another workspace that neither
// mode should ever list.
func dockListOS() *OS {
	m := &OS{
		WorkspaceFocus:   map[int]int{},
		NumWorkspaces:    9,
		CurrentWorkspace: 1,
		FocusedWindow:    0,
		Width:            160,
		Height:           40,
	}
	m.Windows = []*terminal.Window{
		{ID: "a", CustomName: "alpha", Workspace: 1},
		{ID: "b", CustomName: "bravo", Workspace: 1, Minimized: true, MinimizeOrder: 1},
		{ID: "c", CustomName: "charlie", Workspace: 1},
		{ID: "d", CustomName: "delta", Workspace: 2},
	}
	return m
}

// TestGetDockItemsDefaultsToMinimizedOnly is the unchanged behavior: with
// dock_window_list off, only minimized windows in the current workspace get a
// dock entry, exactly as before the option existed.
func TestGetDockItemsDefaultsToMinimizedOnly(t *testing.T) {
	withDockWindowList(t, false)
	m := dockListOS()

	items := m.getDockItems()
	if len(items) != 1 {
		t.Fatalf("got %d dock items, want 1 (only bravo is minimized)", len(items))
	}
	if items[0].WindowIndex != 1 {
		t.Errorf("dock item names window %d, want 1 (bravo)", items[0].WindowIndex)
	}
}

// TestGetDockItemsListsEveryWorkspaceWindowWhenEnabled is dock_window_list's
// own behavior: every window of the current workspace gets an entry, on
// screen or minimized, and a window on another workspace still does not.
func TestGetDockItemsListsEveryWorkspaceWindowWhenEnabled(t *testing.T) {
	withDockWindowList(t, true)
	m := dockListOS()

	items := m.getDockItems()
	if len(items) != 3 {
		t.Fatalf("got %d dock items, want 3 (every workspace-1 window)", len(items))
	}
	want := []int{0, 1, 2}
	for i, item := range items {
		if item.WindowIndex != want[i] {
			t.Errorf("item %d names window %d, want %d", i, item.WindowIndex, want[i])
		}
	}
}

// TestDockWindowNeedsAttention covers dock_window_list's blink predicate: the
// states that raise it, the states that don't, and that the focused window
// never blinks regardless of its state.
func TestDockWindowNeedsAttention(t *testing.T) {
	tests := []struct {
		name       string
		agentState string
		seen       bool
		attention  bool
		focused    bool
		want       bool
	}{
		{name: "idle window, nothing happened", want: false},
		{name: "needs_input always alerts", agentState: "needs_input", want: true},
		{name: "errored always alerts", agentState: "errored", want: true},
		{name: "done and unseen alerts", agentState: "done", seen: false, want: true},
		{name: "done and seen is quiet", agentState: "done", seen: true, want: false},
		{name: "working is quiet", agentState: "working", want: false},
		{name: "bell/notification while unfocused alerts", attention: true, want: true},
		{name: "focused window never blinks, even needs_input", agentState: "needs_input", focused: true, want: false},
		{name: "focused window never blinks, even a fresh bell", attention: true, focused: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &OS{FocusedWindow: -1}
			w := &terminal.Window{ID: "w", AgentState: tt.agentState, DockAttention: tt.attention}
			m.Windows = []*terminal.Window{w}
			if tt.seen {
				m.SidebarAgentSeen = map[string]bool{"w": true}
			}
			if tt.focused {
				m.FocusedWindow = 0
			}

			if got := m.dockWindowNeedsAttention(0); got != tt.want {
				t.Errorf("dockWindowNeedsAttention() = %v, want %v", got, tt.want)
			}
		})
	}
}

// afterDockTrail is the columns of a dock row after the " 1:3 " workspace
// trail - where dock_window_list's own pills begin - or fails the test if the
// trail was never drawn.
func afterDockTrail(t *testing.T, row string) string {
	t.Helper()
	const trail = " 1:3 "
	_, after, ok := strings.Cut(row, trail)
	if !ok {
		t.Fatalf("the dock never drew the workspace trail %q:\n%s", trail, row)
	}
	return after
}

// TestDockWindowListSitsRightAfterTrailWithOneSpace is the positioning half of
// the ask: the strip reads as part of the left block - the "1:3" workspace
// count it follows - rather than as a thing centred in the bar's middle.
func TestDockWindowListSitsRightAfterTrailWithOneSpace(t *testing.T) {
	withDockWindowList(t, true)
	m := dockListOS()

	after := afterDockTrail(t, dockRow(t, m))
	// Exactly one space, then the first pill's own leading cap/space - not a
	// centred run of blanks.
	if !strings.HasPrefix(after, " ") || strings.HasPrefix(after, "  ") {
		t.Errorf("dock_window_list did not sit one space after the trail, got %q", after[:min(10, len(after))])
	}
}

// TestDockWindowListUsesWorkspacePillCaps is the styling half of the ask: the
// list's pills wear the workspace strip's own oval caps, not the minimized-only
// strip's flat/circle ones (which are off by default). Scoped to the columns
// after the trail (see the positioning test above), since the workspace strip
// earlier in the row wears the same caps for an unrelated reason.
func TestDockWindowListUsesWorkspacePillCaps(t *testing.T) {
	withDockWindowList(t, true)
	m := dockListOS()

	after := afterDockTrail(t, dockRow(t, m))

	lc, rc := config.GetDockWorkspaceCapLeft(), config.GetDockWorkspaceCapRight()
	if lc == "" || rc == "" {
		t.Skip("no workspace pill caps in this build (ASCII-only)")
	}
	if !strings.Contains(after, lc) || !strings.Contains(after, rc) {
		t.Errorf("dock_window_list entries did not wear the workspace strip's caps (%q/%q):\n%s", lc, rc, after)
	}
	pillLC, pillRC := config.GetDockPillLeftChar(), config.GetDockPillRightChar()
	if (pillLC != "" && strings.Contains(after, pillLC)) || (pillRC != "" && strings.Contains(after, pillRC)) {
		t.Errorf("dock_window_list entries wore the minimized-only strip's caps instead:\n%s", after)
	}
}

// TestActivityInBackgroundWindowSetsDockAttention is the generic ("something
// happened") activity monitor: plain new output in an unfocused window - no
// bell, no notification, no agent - is enough to raise DockAttention, the
// classic terminal-multiplexer monitor-activity behavior.
func TestActivityInBackgroundWindowSetsDockAttention(t *testing.T) {
	focused := newTestWindow(t, "focused", 40, 12)
	background := newTestWindow(t, "background", 40, 12)
	focused.Workspace, background.Workspace = 1, 1

	m := newTestOS(focused)
	m.Windows = []*terminal.Window{focused, background}
	m.FocusedWindow = 0
	m.CurrentWorkspace = 1

	background.HasNewOutput.Store(true)
	m.MarkTerminalsWithNewContent()

	if !background.DockAttention {
		t.Error("new output in an unfocused window did not set DockAttention")
	}
	if focused.DockAttention {
		t.Error("the focused window should never get DockAttention")
	}
}

// TestFocusedWindowActivityDoesNotSetDockAttention guards the other half: the
// window you are looking at never needs an attention marker of its own.
func TestFocusedWindowActivityDoesNotSetDockAttention(t *testing.T) {
	focused := newTestWindow(t, "focused", 40, 12)
	focused.Workspace = 1

	m := newTestOS(focused)
	m.FocusedWindow = 0
	m.CurrentWorkspace = 1

	focused.HasNewOutput.Store(true)
	m.MarkTerminalsWithNewContent()

	if focused.DockAttention {
		t.Error("activity in the focused window set DockAttention")
	}
}
