package config

import (
	"slices"
	"testing"
)

// TestTenthSlotIsOnZero pins the number keys' tenth slot: window 10, the tenth
// minimized window and the rail's tenth session all sit on 0, the key after 9.
// Workspaces stop at nine, so they get no tenth key.
func TestTenthSlotIsOnZero(t *testing.T) {
	cfg := DefaultConfig()
	kb := cfg.Keybindings
	chord := "alt+0"
	if isMacOS() {
		chord = "opt+0"
	}
	for _, tc := range []struct {
		section map[string][]string
		action  string
		want    []string
	}{
		{kb.WindowManagement, "select_window_10", []string{"0", chord}},
		{kb.RestoreMinimized, "restore_minimized_10", []string{"shift+0", ")"}},
		{kb.MinimizePrefix, "minimize_prefix_restore_10", []string{"0"}},
		{kb.Sidebar, "jump_10", []string{"0"}},
	} {
		if got := tc.section[tc.action]; !slices.Equal(got, tc.want) {
			t.Errorf("%s = %q, want %q", tc.action, got, tc.want)
		}
	}
	for _, action := range []string{"move_and_follow_10", "workspace_prefix_switch_10", "switch_workspace_10"} {
		if _, ok := getDefaultWorkspaceKeybinds()[action]; ok {
			t.Errorf("%s is bound, but there is no workspace 10", action)
		}
	}

	// The which-key menu keeps 1-9 as a range and adds the 0 after it.
	registry := NewKeybindRegistry(cfg)
	found := false
	for _, b := range GetPrefixKeybindings("minimize", registry) {
		if b.Description == "Restore window" {
			found = true
			if b.Key != "1-9, 0" {
				t.Errorf("minimize menu shows %q for restoring a window, want %q", b.Key, "1-9, 0")
			}
		}
	}
	if !found {
		t.Error("minimize menu has no restore-window row")
	}
}
