package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReformatConfig(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "sorts and aligns",
			input: "[appearance]\ntheme = \"nord\"\nborder_style=\"rounded\"\n\nshow_cpu   =   true\n",
			want:  "[appearance]\nborder_style = \"rounded\"\nshow_cpu     = true\ntheme        = \"nord\"\n",
		},
		{
			name:  "numbers sort by value",
			input: "[keybindings.window_management]\nselect_window_10 = [\"0\"]\nselect_window_2 = [\"2\"]\nselect_window_1 = [\"1\"]\n",
			want:  "[keybindings.window_management]\nselect_window_1  = [\"1\"]\nselect_window_2  = [\"2\"]\nselect_window_10 = [\"0\"]\n",
		},
		{
			name:  "comments travel with their key",
			input: "[appearance]\n# the theme\ntheme = \"nord\" # trailing\n# cpu\nshow_cpu = true\n",
			want:  "[appearance]\n# cpu\nshow_cpu = true\n\n# the theme\ntheme    = \"nord\" # trailing\n",
		},
		{
			name:  "loose comments stay put",
			input: "# file header\n\n[appearance]\n# about appearance\n\ntheme = \"nord\"\nshow_cpu = true\n\n# the end\n",
			want:  "# file header\n\n[appearance]\n# about appearance\n\nshow_cpu = true\ntheme    = \"nord\"\n\n# the end\n",
		},
		{
			name:  "comment above a header stays with it",
			input: "[startup]\ntiled = true\n# appearance next\n[appearance]\ntheme = \"nord\"\n",
			want:  "[startup]\ntiled = true\n\n# appearance next\n[appearance]\ntheme = \"nord\"\n",
		},
		{
			name:  "multi-line values are kept",
			input: "[keybindings.window_management]\nnew_window = [\n  \"n\",\n  \"alt+n\",\n]\nclose_window = [\"x\"]\n",
			want:  "[keybindings.window_management]\nclose_window = [\"x\"]\nnew_window   = [\n  \"n\",\n  \"alt+n\",\n]\n",
		},
		{
			name:  "sections keep their order",
			input: "[startup]\ntiled = true\n[appearance]\ntheme = \"nord\"\n",
			want:  "[startup]\ntiled = true\n\n[appearance]\ntheme = \"nord\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReformatConfig([]byte(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
			again, err := ReformatConfig(got)
			if err != nil {
				t.Fatal(err)
			}
			if string(again) != string(got) {
				t.Errorf("not idempotent, second pass:\n%s", again)
			}
		})
	}
}

func TestReformatConfigRefusesBrokenFile(t *testing.T) {
	for _, input := range []string{
		"[appearance]\ntheme = \n",
		"[appearance]\ntheme = \"a\"\ntheme = \"b\"\n",
	} {
		if _, err := ReformatConfig([]byte(input)); err == nil {
			t.Errorf("expected an error for %q", input)
		}
	}
}

// TestReformatConfigDefaultIsStable reformats the file tuios writes for a
// fresh install, which must keep every setting and come out stable.
func TestReformatConfigDefaultIsStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteConfigFile(DefaultConfig(), path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	once, err := ReformatConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := ReformatConfig(once)
	if err != nil {
		t.Fatal(err)
	}
	if string(once) != string(twice) {
		t.Error("reformatting the default config is not idempotent")
	}
}
