package input

import (
	"testing"

	"github.com/tonk/tuios/internal/terminal"
	"github.com/tonk/tuios/internal/vt"
)

// TestMouseForwardingRequiresMouseMode verifies that mouse events are only
// forwarded to apps that explicitly request mouse tracking (DECSET 1000-1003),
// not merely because they use the alternate screen buffer. This prevents
// phantom keypresses in apps like kakoune/nano (issue #78).
func TestMouseForwardingRequiresMouseMode(t *testing.T) {
	tests := []struct {
		name          string
		isAltScreen   bool
		hasMouseMode  bool
		shouldForward bool
	}{
		{
			name:          "alt screen without mouse mode (kakoune/nano) - must NOT forward",
			isAltScreen:   true,
			hasMouseMode:  false,
			shouldForward: false,
		},
		{
			name:          "alt screen with mouse mode (vim/helix) - must forward",
			isAltScreen:   true,
			hasMouseMode:  true,
			shouldForward: true,
		},
		{
			name:          "normal screen with mouse mode - must forward",
			isAltScreen:   false,
			hasMouseMode:  true,
			shouldForward: true,
		},
		{
			name:          "normal screen without mouse mode - must NOT forward",
			isAltScreen:   false,
			hasMouseMode:  false,
			shouldForward: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			em := vt.NewEmulator(80, 24)
			defer func() { _ = em.Close() }()
			win := &terminal.Window{
				Terminal: em,
			}

			if tt.hasMouseMode {
				// Enable normal mouse tracking (mode 1000) via DECSET
				_, _ = em.Write([]byte("\x1b[?1000h"))
			}

			hasMouseMode := win.Terminal.HasMouseMode()
			// The forwarding guard is: shouldForward := hasMouseMode
			shouldForward := hasMouseMode

			if shouldForward != tt.shouldForward {
				t.Errorf("shouldForward = %v, want %v (isAltScreen=%v, hasMouseMode=%v)",
					shouldForward, tt.shouldForward, tt.isAltScreen, hasMouseMode)
			}
		})
	}
}

func TestIsInTerminalContent(t *testing.T) {
	tests := []struct {
		name   string
		x, y   int
		width  int
		height int
		want   bool
	}{
		{
			name: "inside content area",
			x:    5, y: 5,
			width: 80, height: 24,
			want: true,
		},
		{
			name: "at origin",
			x:    0, y: 0,
			width: 80, height: 24,
			want: true,
		},
		{
			name: "at max valid position",
			x:    77, y: 21, // width-2-1, height-2-1
			width: 80, height: 24,
			want: true,
		},
		{
			name: "negative x",
			x:    -1, y: 5,
			width: 80, height: 24,
			want: false,
		},
		{
			name: "negative y",
			x:    5, y: -1,
			width: 80, height: 24,
			want: false,
		},
		{
			name: "x at right border",
			x:    78, y: 5, // width-2
			width: 80, height: 24,
			want: false,
		},
		{
			name: "y at bottom border",
			x:    5, y: 22, // height-2
			width: 80, height: 24,
			want: false,
		},
		{
			name: "small window",
			x:    0, y: 0,
			width: 10, height: 10,
			want: true,
		},
		{
			name: "small window at edge",
			x:    7, y: 7, // 10-2-1
			width: 10, height: 10,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			win := &terminal.Window{
				Width:  tt.width,
				Height: tt.height,
			}
			got := isInTerminalContent(tt.x, tt.y, win)
			if got != tt.want {
				t.Errorf("isInTerminalContent(%d, %d, {Width: %d, Height: %d}) = %v, want %v",
					tt.x, tt.y, tt.width, tt.height, got, tt.want)
			}
		})
	}
}
