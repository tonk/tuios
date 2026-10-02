package scrollback

import (
	"image/color"
	"strings"
	"testing"

	"github.com/tonk/tuios/internal/vt"
)

func TestStyledTextFollowsTheTheme(t *testing.T) {
	var palette [16]color.Color
	for i := range palette {
		palette[i] = color.RGBA{R: uint8(i), G: 0x11, B: 0x22, A: 0xff}
	}

	tests := []struct {
		name   string
		input  string
		themed bool
		want   string // must appear in the styled line
		absent string // must not appear
	}{
		{"palette red resolves through the theme", "\x1b[31mred\x1b[m", true, "38;2;1;17;34", "\x1b[31m"},
		{"palette red stays palette without a theme", "\x1b[31mred\x1b[m", false, "\x1b[31m", "38;2;"},
		{"256-colour slot 1 resolves through the theme", "\x1b[38;5;1mred\x1b[m", true, "38;2;1;17;34", ""},
		{"curly underline survives", "\x1b[4:3mwavy\x1b[m", false, "4:3", ""},
		{"truecolour passes through", "\x1b[38;2;10;20;30mrgb\x1b[m", true, "38;2;10;20;30", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := vt.NewEmulator(20, 2)
			if tt.themed {
				term.SetThemeColors(color.White, color.Black, nil, palette)
			}
			_, _ = term.WriteString(tt.input)

			got := extractAbsLineStyledText(term, term.ScrollbackLen())
			if !strings.Contains(got, tt.want) {
				t.Errorf("styled line %q does not contain %q", got, tt.want)
			}
			if tt.absent != "" && strings.Contains(got, tt.absent) {
				t.Errorf("styled line %q contains %q", got, tt.absent)
			}
		})
	}
}
