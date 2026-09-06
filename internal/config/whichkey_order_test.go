package config

import (
	"slices"
	"testing"
)

// TestWhichKeyLeaderGroupOrder locks the main prefix panel's group order:
// windows → layout → navigate → tools → session.
func TestWhichKeyLeaderGroupOrder(t *testing.T) {
	wantTitles := []string{"windows", "layout", "navigate", "tools", "session"}
	landmarks := []string{
		"Create window",       // windows
		"Toggle tiling",       // layout
		"Switch to workspace", // navigate
		"Command palette",     // tools
		"Window mode",         // session
		"Toggle help",         // last in session
	}

	assertGroups := func(t *testing.T, label string, groups []KeybindingGroup) {
		t.Helper()
		if len(groups) != len(wantTitles) {
			t.Fatalf("%s: got %d groups, want %d", label, len(groups), len(wantTitles))
		}
		for i, title := range wantTitles {
			if groups[i].Title != title {
				t.Fatalf("%s: group %d titled %q, want %q", label, i, groups[i].Title, title)
			}
		}
		descs := make([]string, 0)
		for _, g := range groups {
			for _, row := range g.Bindings {
				descs = append(descs, row.Description)
			}
		}
		prev := -1
		for _, want := range landmarks {
			i := slices.Index(descs, want)
			if i < 0 {
				t.Fatalf("%s which-key leader panel missing %q", label, want)
			}
			if i <= prev {
				t.Fatalf("%s which-key leader panel out of group order: %q at %d, previous landmark at %d\n%s",
					label, want, i, prev, descs)
			}
			prev = i
		}
	}

	t.Run("fallback", func(t *testing.T) {
		assertGroups(t, "fallback", GetPrefixKeybindingGroups("", nil, false))
	})
	t.Run("live", func(t *testing.T) {
		assertGroups(t, "live", GetPrefixKeybindingGroups("", NewKeybindRegistry(DefaultConfig()), false))
	})
}
