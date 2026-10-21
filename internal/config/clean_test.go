package config

import (
	"slices"
	"strings"
	"testing"
)

// TestCleanConfig pins what config clean removes and what it leaves alone.
func TestCleanConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		removed []string // keys reported, in order
	}{
		{
			name:    "default value removed, changed value kept",
			input:   "[appearance]\nborder_style = \"rounded\"\nshow_clock = true\n",
			want:    "[appearance]\nshow_clock = true\n",
			removed: []string{"appearance.border_style"},
		},
		{
			// The pointer bools load as unset without the line, which is the
			// same setting but not the same struct.
			name:    "default written with other quoting or as a pointer bool",
			input:   "[appearance]\nmouse_enabled = true\ndockbar_position = 'bottom'\ntheme = 'nord'\n",
			want:    "[appearance]\ntheme = 'nord'\n",
			removed: []string{"appearance.mouse_enabled", "appearance.dockbar_position"},
		},
		{
			name:    "zero that means use the default",
			input:   "[appearance]\nmax_fps = 0\nwhichkey_position = \"\"\ntheme = 'nord'\n",
			want:    "[appearance]\ntheme = 'nord'\n",
			removed: []string{"appearance.max_fps", "appearance.whichkey_position"},
		},
		{
			name:    "obsolete keys removed, section with them removed",
			input:   "[daemon]\ndefault_codec = \"gob\"\nsocket_path = \"\"\n\n[startup]\ntiled = true\n",
			want:    "[startup]\ntiled = true\n",
			removed: []string{"daemon.default_codec", "daemon.socket_path"},
		},
		{
			name: "keybinding at its default removed, changed and unbound kept",
			input: "[keybindings.layout]\ntoggle_tiling = [\"t\"]\nsnap_left = [\"ctrl+h\"]\n" +
				"[keybindings.terminal_mode]\nterminal_focus_left = []\n",
			want: "[keybindings.layout]\nsnap_left = [\"ctrl+h\"]\n" +
				"[keybindings.terminal_mode]\nterminal_focus_left = []\n",
			removed: []string{"keybindings.layout.toggle_tiling"},
		},
		{
			name:  "env and hooks are never removed",
			input: "[env]\nEDITOR = \"\"\n\n[hooks]\nafter-new-window = \"\"\n",
			want:  "[env]\nEDITOR = \"\"\n\n[hooks]\nafter-new-window = \"\"\n",
		},
		{
			name: "comment above a removed key goes with it, others stay",
			input: "# my settings\n\n[appearance]\n# the border\nborder_style = \"rounded\"\n" +
				"# my clock\nshow_clock = true\n",
			want:    "# my settings\n\n[appearance]\n# my clock\nshow_clock = true\n",
			removed: []string{"appearance.border_style"},
		},
		{
			name:  "empty section removed with its comments",
			input: "[appearance]\nshow_clock = true\n\n# states\n[notifications.agent.states]\n\n[startup]\ntiled = true\n",
			want:  "[appearance]\nshow_clock = true\n\n[startup]\ntiled = true\n",
		},
		{
			// The old flat key has no effect while its replacement is there.
			// Judged one at a time, removing it is fine, and after that the
			// replacement is no longer a no-op, so it stays.
			name:    "old flat key and its replacement are not both removed",
			input:   "[appearance]\nsidebar_width = 20\n\n[appearance.sidebar]\nwidth = 20\n",
			want:    "[appearance.sidebar]\nwidth = 20\n",
			removed: []string{"appearance.sidebar_width"},
		},
		{
			name:  "nothing to remove",
			input: "[appearance]\ntheme = \"nord\"\n",
			want:  "[appearance]\ntheme = \"nord\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, cleaned, err := CleanConfig([]byte(tt.input))
			if err != nil {
				t.Fatalf("CleanConfig: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("result:\n%s\nwant:\n%s", got, tt.want)
			}
			var keys []string
			for _, c := range cleaned {
				keys = append(keys, c.Key)
			}
			if !slices.Equal(keys, tt.removed) {
				t.Errorf("reported %v, want %v", keys, tt.removed)
			}
		})
	}
}

// TestCleanConfigRefusesBrokenFile pins that a file that does not decode is
// left alone rather than cleaned around.
func TestCleanConfigRefusesBrokenFile(t *testing.T) {
	if _, _, err := CleanConfig([]byte("[appearance\nshow_clock = true\n")); err == nil ||
		!strings.Contains(err.Error(), "tuios config check") {
		t.Errorf("CleanConfig on a broken file returned %v, want an error pointing at tuios config check", err)
	}
}

// TestCleanConfigOfTheExampleIsEmpty cleans the example config with every
// setting uncommented. It sets every option at its default, so nothing but
// [env] and [hooks], which clean never touches, may be left.
func TestCleanConfigOfTheExampleIsEmpty(t *testing.T) {
	var uncommented strings.Builder
	for line := range strings.SplitSeq(GenerateExampleConfig(), "\n") {
		if m := uncommentableLine.FindStringSubmatch(line); m != nil {
			line = m[1]
		}
		if !strings.HasPrefix(line, "#") {
			uncommented.WriteString(line + "\n")
		}
	}
	got, _, err := CleanConfig([]byte(uncommented.String()))
	if err != nil {
		t.Fatalf("CleanConfig: %v", err)
	}
	section := ""
	for line := range strings.SplitSeq(string(got), "\n") {
		switch {
		case strings.HasPrefix(line, "["):
			section = line
		case line == "" || section == "[env]" || section == "[hooks]":
		default:
			t.Errorf("cleaning the example left a setting behind in %s: %q", section, line)
		}
	}
}
