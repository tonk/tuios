// Package theme provides color themes and styling for the TUIOS terminal.
package theme

import (
	"fmt"
	"image/color"
	"log"

	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

var enabled bool

// Border color overrides from user config. When non-nil they take precedence
// over the theme-derived border colors. A single focused override applies to
// both window-mode and terminal-mode focused borders.
var (
	borderFocusedOverride   color.Color
	borderUnfocusedOverride color.Color
)

// SetBorderOverrides sets custom border colors from hex strings (e.g. "#89b4fa").
// An empty string clears the corresponding override and restores the theme color.
func SetBorderOverrides(focusedHex, unfocusedHex string) {
	if focusedHex != "" {
		borderFocusedOverride = lipgloss.Color(focusedHex)
	} else {
		borderFocusedOverride = nil
	}
	if unfocusedHex != "" {
		borderUnfocusedOverride = lipgloss.Color(unfocusedHex)
	} else {
		borderUnfocusedOverride = nil
	}
}

// Initialize sets up the theme registry with the specified theme name.
// Call this once at application startup.
// If themeName is empty, theming will be disabled and standard terminal colors will be used.
func Initialize(themeName string) error {
	// If no theme specified, disable theming
	if themeName == "" {
		enabled = false
		return nil
	}

	enabled = true

	// Build the tint registry (built-ins plus custom themes) exactly once for
	// the process, via the same sync.Once EnsureRegistry uses. Calling
	// tint.NewDefaultRegistry() directly here would let a later EnsureRegistry()
	// (fired when the settings page or theme picker first opens) rebuild the
	// global registry and reset the active tint to the library default,
	// silently discarding the configured theme.
	EnsureRegistry()

	// Try to set the theme by ID. An unknown name leaves the registry on its
	// current tint; warn so a typo is visible instead of silently applying the
	// wrong palette. Behavior is otherwise unchanged (theming stays enabled).
	if ok := tint.SetTintID(themeName); !ok {
		log.Printf("Warning: theme %q not found; using default theme colors", themeName)
	}

	return nil
}

// IsEnabled returns true if theming is enabled
func IsEnabled() bool {
	return enabled
}

// Current returns the currently active theme.
// Returns nil if theming is disabled.
func Current() *tint.Tint {
	if !enabled {
		return nil
	}
	return tint.Current()
}

// GetANSIPalette returns the 16 ANSI colors (0-15) from the current theme.
// These are injected into the terminal emulator.
func GetANSIPalette() [16]color.Color {
	t := Current()
	if t == nil {
		// Fallback to default xterm colors
		return [16]color.Color{
			lipgloss.Color("#000000"), lipgloss.Color("#cd0000"), lipgloss.Color("#00cd00"), lipgloss.Color("#cdcd00"),
			lipgloss.Color("#0000ee"), lipgloss.Color("#cd00cd"), lipgloss.Color("#00cdcd"), lipgloss.Color("#e5e5e5"),
			lipgloss.Color("#7f7f7f"), lipgloss.Color("#ff0000"), lipgloss.Color("#00ff00"), lipgloss.Color("#ffff00"),
			lipgloss.Color("#5c5cff"), lipgloss.Color("#ff00ff"), lipgloss.Color("#00ffff"), lipgloss.Color("#ffffff"),
		}
	}
	return [16]color.Color{
		t.Black,        // 0
		t.Red,          // 1
		t.Green,        // 2
		t.Yellow,       // 3
		t.Blue,         // 4
		t.Purple,       // 5
		t.Cyan,         // 6
		t.White,        // 7
		t.BrightBlack,  // 8
		t.BrightRed,    // 9
		t.BrightGreen,  // 10
		t.BrightYellow, // 11
		t.BrightBlue,   // 12
		t.BrightPurple, // 13
		t.BrightCyan,   // 14
		t.BrightWhite,  // 15
	}
}

// TerminalFg returns the foreground color for terminal text.
func TerminalFg() color.Color {
	t := Current()
	if t == nil {
		return lipgloss.Color("#e5e5e5")
	}
	return t.Fg
}

// TerminalBg returns the background color for terminal emulator.
func TerminalBg() color.Color {
	t := Current()
	if t == nil {
		return lipgloss.Color("#000000")
	}
	return t.Bg
}

// TerminalCursor returns the color for the terminal cursor. This is the
// default the VT emulator falls back to (SetThemeColors -> SetDefaultCursorColor);
// a guest that has sent its own cursor-color escape sequence (OSC 12) still
// wins over it, exactly as it would with no theme at all - only the fallback
// itself is theme-driven, and now override-driven too.
func TerminalCursor() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.TerminalCursorFg != nil {
		return ov.TerminalCursorFg
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#00ff00")
	}
	return t.Cursor
}

// BorderUnfocused returns the color for unfocused window borders.
func BorderUnfocused() color.Color {
	if borderUnfocusedOverride != nil {
		return borderUnfocusedOverride
	}
	if ov := overridesForCurrent(); ov != nil && ov.BorderUnfocused != nil {
		return ov.BorderUnfocused
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#FAAAAA")
	}
	// Light pinkish red - use theme's red (or bright red depending on theme)
	// Using regular Red gives a softer, more muted tone for unfocused windows
	return t.Red
}

// BorderFocusedWindow returns the color for focused window borders in window management mode.
func BorderFocusedWindow() color.Color {
	if borderFocusedOverride != nil {
		return borderFocusedOverride
	}
	if ov := overridesForCurrent(); ov != nil && ov.BorderFocusedWindow != nil {
		return ov.BorderFocusedWindow
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#AFFFFF")
	}
	// Light cyan for window mode - use bright cyan
	return t.BrightCyan
}

// BorderFocusedTerminal returns the color for focused window borders in terminal mode.
func BorderFocusedTerminal() color.Color {
	if borderFocusedOverride != nil {
		return borderFocusedOverride
	}
	if ov := overridesForCurrent(); ov != nil && ov.BorderFocusedTerminal != nil {
		return ov.BorderFocusedTerminal
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#AAFFAA")
	}
	// Light green for terminal mode - use bright green
	return t.BrightGreen
}

// BorderMultifocused returns the color for a multifocused window's border
// (one of several panes selected together for a broadcast action), distinct
// from the single focused window's border.
func BorderMultifocused() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.BorderMultifocused != nil {
		return ov.BorderMultifocused
	}
	return lipgloss.Color("3")
}

// DockColorWindow returns the dock indicator color for window management mode.
func DockColorWindow() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.DockWindow != nil {
		return ov.DockWindow
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#5c5cff")
	}
	return t.BrightBlue
}

// DockColorTerminal returns the dock indicator color for terminal mode.
func DockColorTerminal() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.DockTerminal != nil {
		return ov.DockTerminal
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#7aa2f7") // Soft blue
	}
	return t.BrightGreen
}

// DockColorCopy returns the dock indicator color for copy mode.
func DockColorCopy() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.DockCopy != nil {
		return ov.DockCopy
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#e0af68") // Soft amber
	}
	return t.Yellow
}

// CopyModeCursor returns background and foreground colors for the copy mode cursor.
func CopyModeCursor() (bg color.Color, fg color.Color) {
	ov := overridesForCurrent()
	t := Current()
	if t == nil {
		bg, fg = lipgloss.Color("#00ffff"), lipgloss.Color("#000000")
	} else {
		bg, fg = t.BrightCyan, t.Black
	}
	if ov != nil {
		if ov.CopyCursorBg != nil {
			bg = ov.CopyCursorBg
		}
		if ov.CopyCursorFg != nil {
			fg = ov.CopyCursorFg
		}
	}
	return bg, fg
}

// CopyModeVisualSelection returns colors for visually selected text in copy mode.
func CopyModeVisualSelection() (bg color.Color, fg color.Color) {
	ov := overridesForCurrent()
	t := Current()
	if t == nil {
		bg, fg = lipgloss.Color("#cd00cd"), lipgloss.Color("#ffffff")
	} else {
		bg, fg = t.Purple, t.BrightWhite
	}
	if ov != nil {
		if ov.CopyVisualSelectionBg != nil {
			bg = ov.CopyVisualSelectionBg
		}
		if ov.CopyVisualSelectionFg != nil {
			fg = ov.CopyVisualSelectionFg
		}
	}
	return bg, fg
}

// CopyModeSearchCurrent returns colors for the current search match in copy mode.
func CopyModeSearchCurrent() (bg color.Color, fg color.Color) {
	ov := overridesForCurrent()
	t := Current()
	if t == nil {
		bg, fg = lipgloss.Color("#ff00ff"), lipgloss.Color("#000000")
	} else {
		bg, fg = t.BrightPurple, t.Black
	}
	if ov != nil {
		if ov.CopySearchCurrentBg != nil {
			bg = ov.CopySearchCurrentBg
		}
		if ov.CopySearchCurrentFg != nil {
			fg = ov.CopySearchCurrentFg
		}
	}
	return bg, fg
}

// CopyModeSearchOther returns colors for other search matches in copy mode.
func CopyModeSearchOther() (bg color.Color, fg color.Color) {
	ov := overridesForCurrent()
	t := Current()
	if t == nil {
		bg, fg = lipgloss.Color("#ffff00"), lipgloss.Color("#000000")
	} else {
		bg, fg = t.Yellow, t.Black
	}
	if ov != nil {
		if ov.CopySearchOtherBg != nil {
			bg = ov.CopySearchOtherBg
		}
		if ov.CopySearchOtherFg != nil {
			fg = ov.CopySearchOtherFg
		}
	}
	return bg, fg
}

// CopyModeTextSelection returns background and foreground colors for text selection in copy mode.
func CopyModeTextSelection() (bg color.Color, fg color.Color) {
	return lipgloss.Color("62"), lipgloss.Color("15")
}

// CopyModeSelectionCursor returns background and foreground colors for the selection cursor in copy mode.
func CopyModeSelectionCursor() (bg color.Color, fg color.Color) {
	return lipgloss.Color("208"), lipgloss.Color("0")
}

// CopyModeSearchBar returns the copy_search_bar_bg/fg override pair, or
// (nil, nil) if the active theme sets neither. Unlike every other CopyMode*
// accessor, this one has no theme-derived default to fall back to: the
// search input box (internal/app/render_overlays.go) styles itself like
// every other input dialog (text on the chrome's own Surface tone) rather
// than as a colored highlight, and a caller here should only repaint that
// when a theme actually asks for it.
func CopyModeSearchBar() (bg color.Color, fg color.Color) {
	ov := overridesForCurrent()
	if ov != nil {
		if ov.CopySearchBarBg != nil {
			bg = ov.CopySearchBarBg
		}
		if ov.CopySearchBarFg != nil {
			fg = ov.CopySearchBarFg
		}
	}
	return bg, fg
}

// TerminalCursorColors returns foreground and background colors for the terminal cursor rendering.
func TerminalCursorColors() (fg color.Color, bg color.Color) {
	ov := overridesForCurrent()
	t := Current()
	if t == nil {
		fg, bg = lipgloss.Color("#00ff00"), lipgloss.Color("#000000")
	} else {
		fg, bg = t.Cursor, t.Black
	}
	if ov != nil {
		if ov.TerminalCursorFg != nil {
			fg = ov.TerminalCursorFg
		}
		if ov.TerminalCursorBg != nil {
			bg = ov.TerminalCursorBg
		}
	}
	return fg, bg
}

// ButtonFg returns the foreground color for buttons.
func ButtonFg() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.ButtonFg != nil {
		return ov.ButtonFg
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#000000")
	}
	return t.Black
}

// NotificationError returns the color for error notifications.
//
// The no-theme fallbacks for the four severities are ink colors, not the raw
// ANSI primaries they used to be: these are drawn as a one-cell mark and a
// sliver of cap on a dark bar, and #0000ee blue on #1a1a2e was a smudge. A
// theme, when one is active, still wins outright.
func NotificationError() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.NotificationError != nil {
		return ov.NotificationError
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#dc2626")
	}
	return t.Red
}

// NotificationWarning returns the color for warning notifications.
func NotificationWarning() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.NotificationWarning != nil {
		return ov.NotificationWarning
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#d97706")
	}
	return t.Yellow
}

// NotificationSuccess returns the color for success notifications.
func NotificationSuccess() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.NotificationSuccess != nil {
		return ov.NotificationSuccess
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#16a34a")
	}
	return t.Green
}

// NotificationInfo returns the color for info notifications.
func NotificationInfo() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.NotificationInfo != nil {
		return ov.NotificationInfo
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#2563eb")
	}
	return t.Blue
}

// NotificationBg returns the background color for the message block: the dark
// body the weighted severity cap opens.
//
// With theming off this is the dock's own help background rather than black, so
// a message sitting in the dock's right-hand block is made of the same material
// as the copy-mode help line that shares the row. Black would read as a hole
// punched in the bar.
func NotificationBg() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.NotificationBg != nil {
		return ov.NotificationBg
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#1a1a2e")
	}
	return t.Bg
}

// NotificationFg returns the foreground color for notification message text.
func NotificationFg() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.NotificationFg != nil {
		return ov.NotificationFg
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#e5e5e5")
	}
	return t.Fg
}

// NotificationRule returns the color of the dock hairline that a message has
// not lit: the unburnt remainder of the rule, and the whole rule when nothing
// is on screen. It matches the separator the dock already draws.
func NotificationRule() color.Color {
	return lipgloss.Color("#303040")
}

// NotificationSeverity maps a notification type string to its color. The type
// strings are the ones every ShowNotification call site already passes, so this
// is the single place the renderer turns one into a color and the only place
// that has to know "warning" and "warn" are the same thing.
func NotificationSeverity(notifType string) color.Color {
	switch notifType {
	case "error":
		return NotificationError()
	case "warning", "warn":
		return NotificationWarning()
	case "success":
		return NotificationSuccess()
	default:
		return NotificationInfo()
	}
}

// DockBg returns the background color for the dock.
func DockBg() color.Color {
	return lipgloss.Color("#2a2a3e")
}

// DockFg returns the foreground color for the dock.
func DockFg() color.Color {
	return lipgloss.Color("#a0a0a8")
}

// DockHighlight returns the highlight color for the dock.
func DockHighlight() color.Color {
	if ov := overridesForCurrent(); ov != nil && ov.DockHighlight != nil {
		return ov.DockHighlight
	}
	t := Current()
	if t == nil {
		return lipgloss.Color("#00ff00")
	}
	return t.BrightGreen
}

// DockDimmed returns the dimmed color for the dock.
func DockDimmed() color.Color {
	return lipgloss.Color("#808090")
}

// DockAccent returns the accent color for the dock.
func DockAccent() color.Color {
	return lipgloss.Color("#a0a0b0")
}

// DockSeparator returns the separator color for the dock.
func DockSeparator() color.Color {
	return lipgloss.Color("#303040")
}

// ColorToString converts a color.Color to a hex string
// Used for dock_helpers.go where colors need to be stored as strings
func ColorToString(c color.Color) string {
	if c == nil {
		return "#000000"
	}
	r, g, b, _ := c.RGBA()
	// RGBA returns values in range 0-65535, convert to 0-255
	r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)
	// Format as hex string
	return fmt.Sprintf("#%02x%02x%02x", r8, g8, b8)
}
