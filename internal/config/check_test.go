package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckConfigFindsProblems(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		line     int
		severity string
		kind     string
		contains string
	}{
		{
			name:     "syntax error",
			input:    "[appearance]\ntheme = \n",
			line:     2,
			severity: IssueError,
			kind:     IssueSyntax,
		},
		{
			name:     "duplicate key",
			input:    "[appearance]\ntheme = \"nord\"\nshow_cpu = true\ntheme = \"dracula\"\n",
			line:     4,
			severity: IssueError,
			kind:     IssueDuplicate,
			contains: "already set on line 2",
		},
		{
			name:     "duplicate section",
			input:    "[appearance]\ntheme = \"nord\"\n\n[appearance]\nshow_cpu = true\n",
			line:     4,
			severity: IssueError,
			kind:     IssueDuplicate,
			contains: "already defined on line 1",
		},
		{
			name:     "duplicate key through a dotted key",
			input:    "[appearance]\nsidebar.width = 20\n\n[appearance.sidebar]\nwidth = 30\n",
			line:     5,
			severity: IssueError,
			kind:     IssueDuplicate,
		},
		{
			name:     "key listed twice for one action",
			input:    "[keybindings.window_management]\nnew_window = [\"n\", \"n\"]\n",
			line:     2,
			severity: IssueWarning,
			kind:     IssueDuplicate,
		},
		{
			name:     "unknown setting",
			input:    "[appearance]\nshow_clok = true\n",
			line:     2,
			severity: IssueWarning,
			kind:     IssueInvalid,
			contains: "appearance.show_clok",
		},
		{
			name:     "unknown section",
			input:    "[apperance]\ntheme = \"nord\"\n",
			line:     1,
			severity: IssueWarning,
			kind:     IssueInvalid,
			contains: "[apperance]",
		},
		{
			name:     "unknown action",
			input:    "[keybindings.window_management]\nfrobnicate = [\"f\"]\n",
			line:     2,
			severity: IssueWarning,
			kind:     IssueInvalid,
			contains: "frobnicate",
		},
		{
			name:     "unknown hook event",
			input:    "[hooks]\nafter-nonsense = \"true\"\n",
			line:     2,
			severity: IssueWarning,
			kind:     IssueInvalid,
		},
		{
			name:     "wrong type",
			input:    "[appearance]\n\nscroll_lines = \"three\"\n",
			line:     3,
			severity: IssueError,
			kind:     IssueInvalid,
		},
		{
			name:     "invalid value",
			input:    "[appearance]\nborder_style = \"wobbly\"\n",
			line:     2,
			severity: IssueWarning,
			kind:     IssueInvalid,
			contains: "wobbly",
		},
		{
			name:     "obsolete setting",
			input:    "[appearance]\nhide_clock = true\n",
			line:     2,
			severity: IssueWarning,
			kind:     IssueObsolete,
			contains: "show_clock",
		},
		{
			name:     "obsolete legacy sidebar key",
			input:    "[appearance]\nsidebar_width = 20\n",
			line:     2,
			severity: IssueWarning,
			kind:     IssueObsolete,
		},
		{
			name:     "removed daemon socket_path",
			input:    "[daemon]\nsocket_path = \"/run/tuios.sock\"\n",
			line:     2,
			severity: IssueWarning,
			kind:     IssueObsolete,
			contains: "XDG_RUNTIME_DIR",
		},
		{
			name:     "removed daemon default_codec",
			input:    "[daemon]\ndefault_codec = \"gob\"\n",
			line:     2,
			severity: IssueWarning,
			kind:     IssueObsolete,
		},
		{
			name:     "removed focus_follows_mouse_in_terminal",
			input:    "[appearance]\nfocus_follows_mouse_in_terminal = true\n",
			line:     2,
			severity: IssueWarning,
			kind:     IssueObsolete,
			contains: "focus_follows_mouse",
		},
		{
			name:     "obsolete action",
			input:    "[keybindings.navigation]\nextend_up = [\"shift+up\"]\n",
			line:     2,
			severity: IssueWarning,
			kind:     IssueObsolete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := CheckConfig([]byte(tt.input))
			for _, issue := range issues {
				if issue.Line == tt.line && issue.Severity == tt.severity && issue.Kind == tt.kind &&
					strings.Contains(issue.Message, tt.contains) {
					return
				}
			}
			t.Errorf("no %s %s issue on line %d containing %q; got %+v", tt.severity, tt.kind, tt.line, tt.contains, issues)
		})
	}
}

func TestCheckConfigUnknownSectionReportedOnce(t *testing.T) {
	issues := CheckConfig([]byte("[bogus]\na = 1\nb = 2\n"))
	if len(issues) != 1 {
		t.Fatalf("want one issue for the section, got %+v", issues)
	}
}

func TestCheckConfigAcceptsFreeFormTables(t *testing.T) {
	input := "[env]\nEDITOR = \"nvim\"\n\n[hooks]\nafter-new-window = [\"true\"]\n"
	if issues := CheckConfig([]byte(input)); len(issues) != 0 {
		t.Errorf("want no issues, got %+v", issues)
	}
}

// TestCheckConfigDefaultIsClean makes sure the file tuios writes for a fresh
// install passes its own check.
func TestCheckConfigDefaultIsClean(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteConfigFile(DefaultConfig(), path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if issues := CheckConfig(data); len(issues) != 0 {
		t.Errorf("default config has issues: %+v", issues)
	}
}
