package app

import (
	"testing"

	"github.com/tonk/tuios/internal/config"
)

// TestSetBorderStyleAcceptsEveryConfigStyle pins that the tape and remote
// SetBorderStyle command accepts every style border_style and --border-style
// accept. It used to keep its own list, which lacked the half-block styles.
func TestSetBorderStyleAcceptsEveryConfigStyle(t *testing.T) {
	saved := config.BorderStyle
	t.Cleanup(func() { config.BorderStyle = saved })

	m := &OS{}
	for _, style := range config.BorderStyles {
		if err := m.SetBorderStyle(style); err != nil {
			t.Errorf("SetBorderStyle(%q) = %v", style, err)
		} else if config.BorderStyle != style {
			t.Errorf("BorderStyle = %q after SetBorderStyle(%q)", config.BorderStyle, style)
		}
	}
	if err := m.SetBorderStyle("wavy"); err == nil {
		t.Error("SetBorderStyle accepted an unknown style")
	}
}
