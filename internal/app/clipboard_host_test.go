package app

import (
	"errors"
	"testing"
)

func TestPickHostClipTool(t *testing.T) {
	has := func(names ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			for _, n := range names {
				if n == name {
					return "/usr/bin/" + n, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	env := func(kv map[string]string) func(string) string {
		return func(k string) string { return kv[k] }
	}

	tests := []struct {
		name string
		env  map[string]string
		bins []string
		want string // write binary, "" for none
	}{
		{"no display", nil, []string{"xclip", "wl-copy"}, ""},
		{"x11 with xclip", map[string]string{"DISPLAY": ":0"}, []string{"xclip", "xsel"}, "xclip"},
		{"x11 falls back to xsel", map[string]string{"DISPLAY": ":0"}, []string{"xsel"}, "xsel"},
		{"x11 without a helper", map[string]string{"DISPLAY": ":0"}, []string{"wl-copy"}, ""},
		{"wayland", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, []string{"wl-copy", "xclip"}, "wl-copy"},
		{"xwayland without wl-copy", map[string]string{"WAYLAND_DISPLAY": "w", "DISPLAY": ":0"}, []string{"xclip"}, "xclip"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pickHostClipTool(env(tc.env), has(tc.bins...))
			switch {
			case tc.want == "" && got != nil:
				t.Fatalf("got %v, want no tool", got.write)
			case tc.want != "" && got == nil:
				t.Fatalf("got no tool, want %s", tc.want)
			case got != nil && got.write[0] != tc.want:
				t.Fatalf("got %s, want %s", got.write[0], tc.want)
			}
		})
	}
}

func TestNativeClipboardSkippedOverSSHAndWeb(t *testing.T) {
	for _, m := range []*OS{{IsSSHMode: true}, {IsWebSession: true}} {
		if m.nativeClipboard() != nil {
			t.Fatalf("native clipboard must stay off for SSH/web sessions")
		}
	}
}
