package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tonk/tuios/internal/config"
)

// newResizeSizeOS is an OS that has already received its first size, so the
// next WindowSizeMsg is a resize the user made.
func newResizeSizeOS(t *testing.T) *OS {
	t.Helper()
	prev := config.ShowResizeSize
	config.ShowResizeSize = true
	t.Cleanup(func() { config.ShowResizeSize = prev })

	m := NewOS(OSOptions{UserConfig: config.DefaultConfig()})
	t.Cleanup(func() { closeWindows(m) })
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	// Past the startup grace, so the next size is a resize the user made.
	m.firstSizeAt = time.Now().Add(-resizeSizeStartupGrace)
	return m
}

// frameText composes a fresh frame and returns it without escapes.
func frameText(m *OS) string {
	m.View()
	return ansi.Strip(m.cachedViewContent)
}

func TestResizeSizeNotShownForTheFirstSize(t *testing.T) {
	m := newResizeSizeOS(t)
	if m.ResizeSizeShown() {
		t.Error("the first size, the one tuios started in, popped the size")
	}
}

func TestResizeSizeShowsTheNewSizeAndHides(t *testing.T) {
	m := newResizeSizeOS(t)

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	firstGen := m.resizeSizeGen
	m.Update(tea.WindowSizeMsg{Width: 101, Height: 30})
	if !m.ResizeSizeShown() {
		t.Fatal("a resize did not show the size")
	}
	if got := frameText(m); !strings.Contains(got, "99 × 26") {
		t.Errorf("the frame does not show 99 × 26")
	}

	// The hide the first resize scheduled is superseded by the second.
	m.Update(ResizeSizeHideMsg{Gen: firstGen})
	if !m.ResizeSizeShown() {
		t.Fatal("a superseded hide took the popup down mid-drag")
	}

	m.Update(ResizeSizeHideMsg{Gen: m.resizeSizeGen})
	if m.ResizeSizeShown() {
		t.Fatal("the last resize's hide left the popup up")
	}
	if got := frameText(m); strings.Contains(got, "99 × 26") {
		t.Error("the frame after hiding still shows the size")
	}
}

func TestResizeSizeSameSizeDoesNotShow(t *testing.T) {
	m := newResizeSizeOS(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.ResizeSizeShown() {
		t.Error("a size message that changed nothing popped the size")
	}
}

func TestResizeSizeOptionOff(t *testing.T) {
	m := newResizeSizeOS(t)
	config.ShowResizeSize = false
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.ResizeSizeShown() {
		t.Error("show_resize_size = false still popped the size")
	}
}

func TestResizeSizeASCII(t *testing.T) {
	m := newResizeSizeOS(t)
	prev := config.UseASCIIOnly
	config.UseASCIIOnly = true
	t.Cleanup(func() { config.UseASCIIOnly = prev })

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if got := frameText(m); !strings.Contains(got, "98 x 26") {
		t.Error("ASCII mode does not show 98 x 26")
	}
}

// A window that is enlarged right after it opens (kitty --start-as=maximized,
// a tiling window manager placing it) is settling, not being resized.
func TestResizeSizeNotShownWhileTheWindowSettles(t *testing.T) {
	prev := config.ShowResizeSize
	config.ShowResizeSize = true
	t.Cleanup(func() { config.ShowResizeSize = prev })

	m := NewOS(OSOptions{UserConfig: config.DefaultConfig()})
	t.Cleanup(func() { closeWindows(m) })
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(tea.WindowSizeMsg{Width: 278, Height: 70})
	if m.ResizeSizeShown() {
		t.Error("the window maximizing right after start popped the size")
	}
}
