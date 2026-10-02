package vt

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/parser"
)

// handleEsc handles an escape sequence.
func (e *Emulator) handleEsc(cmd ansi.Cmd) {
	e.flushGrapheme() // Flush any pending grapheme before handling ESC sequences.
	if !e.handlers.handleEsc(int(cmd)) {
		var str string
		if inter := cmd.Intermediate(); inter != 0 {
			str += string(inter) + " "
		}
		if final := cmd.Final(); final != 0 {
			str += string(final)
		}
		e.logf("unhandled sequence: ESC %q", str)
	}
}

// fullReset performs a full terminal reset as in [ansi.RIS].
func (e *Emulator) fullReset() {
	e.scrs[0].Reset()
	e.scrs[1].Reset()
	e.resetTabStops()

	// XXX: Do we reset all modes here? Investigate.
	e.resetModes()

	e.gl, e.gr = 0, 1
	e.gsingle = 0
	e.charsets = [4]CharSet{}
	e.charsetIDs = defaultCharsetIDs
	e.atPhantom = false
	e.grapheme = e.grapheme[:0]
	e.openGrapheme = openGrapheme{}
	e.lastChar = 0
	e.lastState = parser.GroundState

	// Reset kitty keyboard protocol state
	if e.kittyKbd != nil {
		e.kittyKbd.Reset()
		e.updateKittyKeyboardCache()
	}
}

// screenAlignment fills the screen with 'E' as in [ansi.DECALN], the VT100
// alignment test pattern. Like xterm it also resets the margins to the whole
// screen and homes the cursor. The cells take the default rendition.
func (e *Emulator) screenAlignment() {
	e.scr.scroll = e.scr.Bounds()
	cell := uv.Cell{Content: "E", Width: 1}
	e.scr.Fill(&cell)
	e.setCursor(0, 0)
}
