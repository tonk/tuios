package app

import (
	"testing"

	"github.com/tonk/tuios/internal/config"
)

func TestFindURL(t *testing.T) {
	tests := []struct {
		name string
		link string
		word string
		want string
	}{
		{"plain URL", "", "https://example.com/a?b=c&d=e#f", "https://example.com/a?b=c&d=e#f"},
		{"http", "", "http://example.com", "http://example.com"},
		{"sentence full stop", "", "https://example.com/a.", "https://example.com/a"},
		{"wrapped in a prefix", "", "url=https://example.com/x", "https://example.com/x"},
		{"unbalanced closing paren", "", "https://example.com/a)", "https://example.com/a"},
		{"balanced paren kept", "", "https://en.wikipedia.org/wiki/Go_(game)", "https://en.wikipedia.org/wiki/Go_(game)"},
		{"OSC 8 link wins", "https://link.example/", "click", "https://link.example/"},
		{"non-http OSC 8 falls back to text", "file:///etc/passwd", "https://example.com", "https://example.com"},
		{"file scheme refused", "file:///etc/passwd", "file:///etc/passwd", ""},
		{"javascript refused", "", "javascript:alert(1)", ""},
		{"no host", "", "https://", ""},
		{"plain word", "", "bravo", ""},
		{"path", "", "/usr/local/bin", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FindURL(tt.link, tt.word); got != tt.want {
				t.Errorf("FindURL(%q, %q) = %q, want %q", tt.link, tt.word, got, tt.want)
			}
		})
	}
}

// TestCanOpenURLs holds the line that keeps a browser from starting on a
// server: SSH and tuios-web sessions never open, whatever the option says.
func TestCanOpenURLs(t *testing.T) {
	prev := config.OpenURLOnDoubleClick
	t.Cleanup(func() { config.OpenURLOnDoubleClick = prev })

	tests := []struct {
		name   string
		option bool
		os     func() *OS
		want   bool
	}{
		{"local", true, func() *OS { return &OS{} }, true},
		{"option off", false, func() *OS { return &OS{} }, false},
		{"daemon attach", true, func() *OS { return &OS{IsDaemonSession: true} }, true},
		{"ssh", true, func() *OS { return &OS{IsSSHMode: true} }, false},
		{"web", true, func() *OS { return &OS{IsWebSession: true} }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.OpenURLOnDoubleClick = tt.option
			if got := tt.os().CanOpenURLs(); got != tt.want {
				t.Errorf("CanOpenURLs() = %v, want %v", got, tt.want)
			}
		})
	}
}
