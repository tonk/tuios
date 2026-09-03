package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/session"
	"github.com/tonk/tuios/internal/sessiontree"
	"github.com/tonk/tuios/internal/terminal"
)

// sectionsTestOS is a rail attached to "main" beside two foreign sessions that
// carry real panes, which is what a peek needs to have something to preview.
func sectionsTestOS(t *testing.T, w, h int) (*OS, sessiontree.Tree) {
	t.Helper()
	m := newNarrowOS(t, w, h)
	m.CurrentWorkspace = 1
	m.SessionName = "main"
	m.Windows = []*terminal.Window{
		{ID: "aaaaaaaa1111", CustomName: "nvim", Width: 40, Height: 20, Workspace: 1},
		{ID: "bbbbbbbb2222", CustomName: "refactor", Width: 40, Height: 20, Workspace: 1, AgentState: "done"},
		{ID: "cccccccc3333", CustomName: "build", Width: 40, Height: 20, Workspace: 2, AgentState: "working"},
	}
	m.FocusedWindow = 0
	m.DaemonClient = &session.TUIClient{}
	m.IsDaemonSession = true
	withSidebar(t, true, "left", config.SidebarDefaultWidth)
	m.SidebarOrder = nil

	tree := sessiontree.Build([]sessiontree.SessionInput{
		{Name: "main", Attached: true, IsCurrent: true, CurrentWorkspace: 1, Windows: []sessiontree.WindowInput{
			{ID: "aaaaaaaa1111", Title: "nvim", Focused: true, Workspace: 1},
			{ID: "bbbbbbbb2222", Title: "refactor", AgentState: "done", Workspace: 1},
			{ID: "cccccccc3333", Title: "build", AgentState: "working", Workspace: 2},
		}},
		{Name: "api", CurrentWorkspace: 1, Windows: []sessiontree.WindowInput{
			{ID: "dddddddd4444", Title: "server", AgentState: "needs_input", Workspace: 1},
			{ID: "eeeeeeee5555", Title: "worker", Workspace: 3},
		}},
		{Name: "docs"},
	})
	return m, tree
}

// railPlain renders the rail and strips the styling, which is what most of the
// claims below are about: where a row landed, not how it was painted.
func railPlain(t *testing.T, m *OS, tree sessiontree.Tree) []string {
	t.Helper()
	lines, _ := m.sidebarPanelLinesForTree(tree)
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = stripANSIForTrace(l)
	}
	return out
}

// lineOf returns the index of the first rendered line containing want, or -1.
func lineOf(lines []string, want string) int {
	for i, l := range lines {
		if strings.Contains(l, want) {
			return i
		}
	}
	return -1
}

// TestRailDrawsThreeSectionsInOrder pins the shape the tree was replaced with:
// sessions, then terminals, then agents, with the agents block pinned to the
// rail's bottom so the alarm sits at a stable screen position whatever else the
// rail is carrying.
func TestRailDrawsThreeSectionsInOrder(t *testing.T) {
	m, tree := sectionsTestOS(t, 120, 30)
	lines := railPlain(t, m, tree)

	sessions, terminals, agents := lineOf(lines, " sessions"), lineOf(lines, " terminals"), lineOf(lines, " agents")
	if sessions < 0 || terminals < 0 || agents < 0 {
		t.Fatalf("a section header is missing:\n%s", strings.Join(lines, "\n"))
	}
	if !(sessions < terminals && terminals < agents) {
		t.Errorf("headers landed at sessions=%d terminals=%d agents=%d, want that order", sessions, terminals, agents)
	}

	// Pinned means the last agent row is the line directly above the footer.
	last := -1
	for _, h := range m.SidebarHits {
		if h.Kind == sidebarRowAgent && h.Y0-m.GetTopMargin() > last {
			last = h.Y0 - m.GetTopMargin()
		}
	}
	footer := lineOf(lines, "«")
	if last < 0 || footer < 0 {
		t.Fatalf("no agent rows or no footer:\n%s", strings.Join(lines, "\n"))
	}
	if last != footer-1 {
		t.Errorf("last agent row on line %d, footer on %d: the agents block is not pinned to the bottom", last, footer)
	}
}

// TestRailTerminalsShowEveryWorkspace checks the terminals section lists every
// pane of the attached session whatever workspace it is on, and that only the
// ones elsewhere carry the tag: "here" is not information.
func TestRailTerminalsShowEveryWorkspace(t *testing.T) {
	m, tree := sectionsTestOS(t, 120, 30)
	lines := railPlain(t, m, tree)

	terminals := lineOf(lines, " terminals")
	rows := map[string]string{}
	for _, h := range m.SidebarHits {
		if h.Kind == sidebarRowWindow {
			rows[h.WindowID] = lines[h.Y0-m.GetTopMargin()]
		}
	}
	if len(rows) != 3 {
		t.Fatalf("terminals section drew %d rows, want all 3 panes:\n%s", len(rows), strings.Join(lines[terminals:], "\n"))
	}
	if got := rows["cccccccc3333"]; !strings.Contains(got, "w2") {
		t.Errorf("a pane on workspace 2 did not name it: %q", got)
	}
	if got := rows["aaaaaaaa1111"]; strings.Contains(got, "w1") {
		t.Errorf("a pane on the current workspace tagged itself: %q", got)
	}
}

// interleavedWorkspaceOS is a session whose panes were created bouncing between
// workspaces, so creation order is not workspace order. The terminals section
// has to regroup them or the list reads as one mixed stack.
func interleavedWorkspaceOS(t *testing.T) (*OS, sessiontree.Tree) {
	t.Helper()
	m := newNarrowOS(t, 120, 40)
	m.CurrentWorkspace = 2
	m.SessionName = "main"
	m.Windows = []*terminal.Window{
		{ID: "a1", CustomName: "one-a", Width: 40, Height: 20, Workspace: 1},
		{ID: "b1", CustomName: "two-a", Width: 40, Height: 20, Workspace: 2},
		{ID: "a2", CustomName: "one-b", Width: 40, Height: 20, Workspace: 1},
		{ID: "c1", CustomName: "three-a", Width: 40, Height: 20, Workspace: 3},
		{ID: "b2", CustomName: "two-b", Width: 40, Height: 20, Workspace: 2},
	}
	m.FocusedWindow = 1
	withSidebar(t, true, "left", config.SidebarDefaultWidth)
	m.SidebarOrder = nil
	tree := m.BuildSessionTree()
	return m, tree
}

func terminalWindowIDs(entries []sidebarTerminalEntry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.WindowID
	}
	return ids
}

func drawnTerminalWindowIDs(m *OS) []string {
	var ids []string
	for _, h := range m.SidebarHits {
		if h.Kind == sidebarRowWindow {
			ids = append(ids, h.WindowID)
		}
	}
	return ids
}

// TestSidebarTerminalsGroupedByWorkspace is the claim: panes that share a
// workspace sit together, the workspace on screen leads, and within a workspace
// the session's own pane order is kept. Creation order here is 1, 2, 1, 3, 2.
func TestSidebarTerminalsGroupedByWorkspace(t *testing.T) {
	m, tree := interleavedWorkspaceOS(t)
	got := terminalWindowIDs(m.sidebarTerminals(tree.Sessions, "main"))
	want := []string{"b1", "b2", "a1", "a2", "c1"}
	if !slices.Equal(got, want) {
		t.Fatalf("terminals order = %v, want %v (workspace 2, then 1, then 3)", got, want)
	}

	m.sidebarPanelLinesForTree(tree)
	if drawn := drawnTerminalWindowIDs(m); !slices.Equal(drawn, want) {
		t.Fatalf("drawn terminals order = %v, want %v", drawn, want)
	}
}

// TestPeekedTerminalsGroupedByWorkspace: a peek uses the peeked session's own
// current workspace as "here", so its on-screen panes still lead even though
// this client is attached to a different session.
func TestPeekedTerminalsGroupedByWorkspace(t *testing.T) {
	m, _ := sectionsTestOS(t, 120, 30)
	tree := sessiontree.Build([]sessiontree.SessionInput{
		{Name: "api", CurrentWorkspace: 3, Windows: []sessiontree.WindowInput{
			{ID: "w1a", Title: "one-a", Workspace: 1},
			{ID: "w3a", Title: "three-a", Workspace: 3},
			{ID: "w1b", Title: "one-b", Workspace: 1},
			{ID: "w2a", Title: "two-a", Workspace: 2},
			{ID: "w3b", Title: "three-b", Workspace: 3},
		}},
	})
	got := terminalWindowIDs(m.sidebarTerminals(tree.Sessions, "api"))
	want := []string{"w3a", "w3b", "w1a", "w1b", "w2a"}
	if !slices.Equal(got, want) {
		t.Fatalf("peeked terminals order = %v, want %v (workspace 3, then 1, then 2)", got, want)
	}
}

// TestUnknownWorkspaceSortsLast: a pane whose workspace is not known must not
// sit above panes that do name one. An older daemon omits the field, and
// hoisting those rows would shuffle a mixed listing for a reason the user
// cannot see.
func TestUnknownWorkspaceSortsLast(t *testing.T) {
	m, _ := sectionsTestOS(t, 120, 30)
	tree := sessiontree.Build([]sessiontree.SessionInput{
		{Name: "api", CurrentWorkspace: 1, Windows: []sessiontree.WindowInput{
			{ID: "unk", Title: "legacy"},
			{ID: "here", Title: "nvim", Workspace: 1},
			{ID: "there", Title: "build", Workspace: 2},
		}},
	})
	got := terminalWindowIDs(m.sidebarTerminals(tree.Sessions, "api"))
	want := []string{"here", "there", "unk"}
	if !slices.Equal(got, want) {
		t.Fatalf("terminals order = %v, want %v (known workspaces, then unknown)", got, want)
	}
}

// TestRailHitsAreASubsequenceOfNav is the invariant three sections with filters
// and a peek make much easier to break than the tree did: every drawn row's
// rectangle names the same target as its nav row, and the two lists run in the
// same order. Nav also carries the rows scrolled out of sight, which is what
// lets the keyboard reach them, so it is a superset rather than a copy.
func TestRailHitsAreASubsequenceOfNav(t *testing.T) {
	for _, pos := range []string{"left", "right"} {
		for _, peek := range []string{"", "api"} {
			for _, h := range []int{30, 12, 9} {
				t.Run(pos+"/"+peek+"/"+string(rune('0'+h/10))+string(rune('0'+h%10)), func(t *testing.T) {
					m, tree := sectionsTestOS(t, 120, h)
					withSidebar(t, true, pos, config.SidebarDefaultWidth)
					m.SidebarPeek = peek
					m.sidebarPanelLinesForTree(tree)

					j := 0
					for _, hit := range m.SidebarHits {
						want := navRowOf(hit)
						for j < len(m.SidebarNav) && !sidebarNavRowsEqual(m.SidebarNav[j], want) {
							j++
						}
						if j >= len(m.SidebarNav) {
							t.Fatalf("hit %+v has no nav row after the ones already matched", want)
						}
						j++
					}
				})
			}
		}
	}
}

// TestRailSectionBudget pins the design's table: sessions take about a quarter
// and agents about a third, the terminals section takes the slack, and a rail
// too short for its floor shrinks agents before sessions and sessions before
// the working list.
func TestRailSectionBudget(t *testing.T) {
	for _, tc := range []struct {
		name                string
		avail, nS, nT, nA   int
		wantS, wantT, wantA int
	}{
		{"roomy", 20, 4, 6, 5, 4, 6, 5},
		{"sessions capped at a quarter", 20, 12, 20, 0, 5, 15, 0},
		{"agents capped at a third", 24, 2, 20, 12, 2, 14, 8},
		{"tight rail shrinks agents first", 8, 6, 6, 6, 2, 4, 2},
		{"no agents at all", 12, 3, 20, 0, 3, 9, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, tt, a := sidebarBudget(tc.avail, tc.nS, tc.nT, tc.nA)
			if s != tc.wantS || tt != tc.wantT || a != tc.wantA {
				t.Errorf("budget(%d, %d, %d, %d) = %d/%d/%d, want %d/%d/%d",
					tc.avail, tc.nS, tc.nT, tc.nA, s, tt, a, tc.wantS, tc.wantT, tc.wantA)
			}
		})
	}

	// Whatever the shape, the three sections never claim more lines than exist
	// and never claim more rows than they hold.
	for avail := 0; avail <= 30; avail++ {
		for nS := 0; nS <= 8; nS++ {
			for nT := 0; nT <= 8; nT++ {
				for nA := 0; nA <= 8; nA++ {
					s, tt, a := sidebarBudget(avail, nS, nT, nA)
					if s < 0 || tt < 0 || a < 0 || s+tt+a > avail {
						t.Fatalf("budget(%d, %d, %d, %d) = %d/%d/%d overruns the rail", avail, nS, nT, nA, s, tt, a)
					}
					if s > nS || tt > nT || a > nA {
						t.Fatalf("budget(%d, %d, %d, %d) = %d/%d/%d claims lines for rows that do not exist",
							avail, nS, nT, nA, s, tt, a)
					}
				}
			}
		}
	}
}

// TestRailSectionOverflowOwnsUpToWhatItHides checks a section given fewer lines
// than rows spends its last line on the tally, and that scrolling to the bottom
// still reaches the last row rather than parking the tally over it.
func TestRailSectionOverflowOwnsUpToWhatItHides(t *testing.T) {
	start, shown, hidden := sidebarWindowSection(0, 10, 4)
	if start != 0 || shown != 3 || hidden != 7 {
		t.Errorf("at rest: start=%d shown=%d hidden=%d, want 0/3/7", start, shown, hidden)
	}
	start, shown, hidden = sidebarWindowSection(2, 10, 4)
	if start != 2 || shown != 3 || hidden != 5 {
		t.Errorf("scrolled: start=%d shown=%d hidden=%d, want 2/3/5", start, shown, hidden)
	}
	start, shown, hidden = sidebarWindowSection(99, 10, 4)
	if start != 6 || shown != 4 || hidden != 0 {
		t.Errorf("at the bottom: start=%d shown=%d hidden=%d, want 6/4/0 so the last row is reachable", start, shown, hidden)
	}
	if _, shown, hidden = sidebarWindowSection(0, 3, 4); shown != 3 || hidden != 0 {
		t.Errorf("a section that fits drew %d rows and hid %d, want 3 and 0", shown, hidden)
	}
}

// TestRailWheelScrollsOnlyTheSectionUnderThePointer: one rail-wide offset
// unpins the headers and can scroll the agents section, the alarm, off screen.
func TestRailWheelScrollsOnlyTheSectionUnderThePointer(t *testing.T) {
	m, tree := sectionsTestOS(t, 120, 30)
	m.sidebarPanelLinesForTree(tree)

	bands := m.sidebarSectionY
	m.SidebarWheel(1, bands[sidebarSectionTerminals][0], false)
	if m.SidebarScrollT == 0 {
		t.Error("a wheel over the terminals section did not scroll it")
	}
	if m.SidebarScrollS != 0 || m.SidebarScrollA != 0 {
		t.Errorf("a wheel over the terminals section also moved sessions=%d agents=%d", m.SidebarScrollS, m.SidebarScrollA)
	}

	m.SidebarScrollT = 0
	m.SidebarWheel(1, bands[sidebarSectionAgents][0], false)
	if m.SidebarScrollA == 0 {
		t.Error("a wheel over the agents section did not scroll it")
	}
	if m.SidebarScrollT != 0 {
		t.Errorf("a wheel over the agents section also scrolled the terminals section to %d", m.SidebarScrollT)
	}
}

// TestRailSectionScrollsAreInTheSignature: an offset the cache cannot see
// leaves yesterday's rows on screen.
func TestRailSectionScrollsAreInTheSignature(t *testing.T) {
	m, tree := sectionsTestOS(t, 120, 30)
	m.sidebarPanelLinesForTree(tree)
	base := m.sidebarSignature()

	for name, field := range map[string]*int{
		"sessions":  &m.SidebarScrollS,
		"terminals": &m.SidebarScrollT,
		"agents":    &m.SidebarScrollA,
	} {
		*field = 3
		if m.sidebarSignature() == base {
			t.Errorf("the %s section's scroll offset is not in the rail signature", name)
		}
		*field = 0
	}
}
