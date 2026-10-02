package vt

import (
	"image/color"
	"io"
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// TestHandleTitle_Semicolon verifies that an OSC title containing a semicolon
// is preserved rather than discarded (only the first ';' separates cmd/data).
func TestHandleTitle_Semicolon(t *testing.T) {
	e := NewEmulator(80, 24)
	defer e.Close()

	e.Write([]byte("\x1b]2;foo;bar\x1b\\"))

	if e.title != "foo;bar" {
		t.Errorf("title = %q, want %q", e.title, "foo;bar")
	}
}

// TestED2_RemovesOnScreenSemanticMarkers verifies that CSI 2J (clear) removes
// semantic markers referencing on-screen content so stale prompt/command
// markers do not survive a clear.
func TestED2_RemovesOnScreenSemanticMarkers(t *testing.T) {
	e := NewEmulator(80, 24)
	defer e.Close()

	// Emit an on-screen prompt marker (AbsLine = scrollbackLen + cursorY = 0).
	e.Write([]byte("\x1b]133;A\x1b\\"))
	if e.semanticMarkers.Len() == 0 {
		t.Fatal("expected a semantic marker after OSC 133;A")
	}

	e.Write([]byte("\x1b[2J"))

	if got := e.semanticMarkers.Len(); got != 0 {
		t.Errorf("on-screen markers = %d after CSI 2J, want 0", got)
	}
}

// TestThemedSGR_UnderlineSubparamNoLeak verifies that an out-of-range underline
// subparameter (4:7) is consumed instead of leaking as a separate SGR 7
// (reverse video) on the themed SGR path.
func TestThemedSGR_UnderlineSubparamNoLeak(t *testing.T) {
	e := NewEmulator(80, 24)
	defer e.Close()

	// Activate the themed path.
	var pal [16]color.Color
	for i := range pal {
		pal[i] = color.RGBA{R: uint8(i * 16), G: 0, B: 0, A: 255}
	}
	e.SetThemeColors(color.White, color.Black, color.White, pal)
	if !e.hasThemeColors() {
		t.Fatal("theme colors not active; test would exercise the wrong path")
	}

	e.Write([]byte("\x1b[4:7mX"))

	c := e.CellAt(0, 0)
	if c == nil {
		t.Fatal("no cell at (0,0)")
	}
	if c.Style.Attrs&uv.AttrReverse != 0 {
		t.Error("SGR 4:7 leaked a stray reverse-video attribute")
	}
	if c.Style.Underline != ansi.UnderlineNone {
		t.Errorf("SGR 4:7 unknown style should leave underline unset, got %v", c.Style.Underline)
	}
}

func TestOSC4_PaletteQuery(t *testing.T) {
	e := NewEmulator(80, 24)
	defer e.Close()

	// Set a custom color for index 1
	e.Write([]byte("\x1b]4;1;rgb:ff/00/00\x1b\\"))

	// Query color index 1
	responseChan := make(chan string, 1)
	errChan := make(chan error, 1)
	go func() {
		buf := make([]byte, 256)
		n, err := e.Read(buf)
		if err != nil && err != io.EOF {
			errChan <- err
			return
		}
		responseChan <- string(buf[:n])
	}()

	e.Write([]byte("\x1b]4;1;?\x1b\\"))

	select {
	case response := <-responseChan:
		// Should contain a color response with index 1
		if !strings.Contains(response, "4;1;") {
			t.Errorf("expected response containing '4;1;', got %q", response)
		}
	case err := <-errChan:
		t.Fatalf("Read error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for OSC 4 response")
	}
}

func TestOSC52_ClipboardQuery(t *testing.T) {
	e := NewEmulator(80, 24)
	defer e.Close()

	// Query clipboard
	responseChan := make(chan string, 1)
	errChan := make(chan error, 1)
	go func() {
		buf := make([]byte, 256)
		n, err := e.Read(buf)
		if err != nil && err != io.EOF {
			errChan <- err
			return
		}
		responseChan <- string(buf[:n])
	}()

	e.Write([]byte("\x1b]52;c;?\x1b\\"))

	select {
	case response := <-responseChan:
		// Should get an empty clipboard response
		if !strings.Contains(response, "52;c;") {
			t.Errorf("expected response containing '52;c;', got %q", response)
		}
	case err := <-errChan:
		t.Fatalf("Read error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for OSC 52 response")
	}
}

func TestModeSynchronizedOutput(t *testing.T) {
	e := NewEmulator(80, 24)
	defer e.Close()

	// Enable synchronized output mode
	e.Write([]byte("\x1b[?2026h"))

	// Query the mode via DECRQM
	responseChan := make(chan string, 1)
	errChan := make(chan error, 1)
	go func() {
		buf := make([]byte, 256)
		n, err := e.Read(buf)
		if err != nil && err != io.EOF {
			errChan <- err
			return
		}
		responseChan <- string(buf[:n])
	}()

	e.Write([]byte("\x1b[?2026$p"))

	select {
	case response := <-responseChan:
		// Should respond with mode set (1) or reset (2)
		// CSI ? 2026 ; Ps $ y where Ps=1 means set
		if !strings.Contains(response, "2026") {
			t.Errorf("expected response containing '2026', got %q", response)
		}
	case err := <-errChan:
		t.Fatalf("Read error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for DECRQM response")
	}
}

// TestHyperlinkOSC8 checks OSC 8 ; params ; URI ST: the parameters come
// first and the URI last. They used to be read the other way round, so every
// link was stored with an empty URL and re-emitted as a link terminator.
func TestHyperlinkOSC8(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantURL    string
		wantParams string
		linkedCols int // cells from column 0 that carry the link
	}{
		{"plain link", "\x1b]8;;https://example.com\x1b\\ab\x1b]8;;\x1b\\cd", "https://example.com", "", 2},
		{"with id parameter", "\x1b]8;id=42;https://example.com/x\x07ab\x1b]8;;\x07", "https://example.com/x", "id=42", 2},
		{"semicolon in the URI", "\x1b]8;;https://example.com/a;b\x1b\\ab\x1b]8;;\x1b\\", "https://example.com/a;b", "", 2},
		{"malformed is ignored", "\x1b]8;https://example.com\x1b\\ab", "", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEmulator(10, 1)
			defer e.Close()
			_, _ = e.WriteString(tt.input)

			for x := range 4 {
				c := e.CellAt(x, 0)
				if x < tt.linkedCols {
					if c.Link.URL != tt.wantURL || c.Link.Params != tt.wantParams {
						t.Errorf("cell %d link = {URL %q, Params %q}, want {URL %q, Params %q}",
							x, c.Link.URL, c.Link.Params, tt.wantURL, tt.wantParams)
					}
				} else if c.Link.URL != "" {
					t.Errorf("cell %d link URL = %q, want none (link closed)", x, c.Link.URL)
				}
			}

			// What the emulator renders back must be the same OSC 8, so the
			// host terminal sees a working link.
			if tt.linkedCols > 0 {
				want := ansi.SetHyperlink(tt.wantURL, tt.wantParams)
				if tt.wantParams == "" {
					want = ansi.SetHyperlink(tt.wantURL)
				}
				if got := e.Render(); !strings.Contains(got, want) {
					t.Errorf("Render() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}
