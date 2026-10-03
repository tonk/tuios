package vt

import "github.com/charmbracelet/x/ansi/parser"

// The transition table x/ansi ships treats the byte 0x9C as the 8-bit String
// Terminator inside every string state (OSC, DCS, SOS, PM, APC). That is right
// for a terminal speaking 8-bit controls, and wrong for one speaking UTF-8,
// which is what every guest here does: 0x9C is then an ordinary continuation
// byte. Any character whose encoding contains it ends the string early, and
// the rest of the payload is printed into the screen as text. Claude Code sets
// the window title with OSC 0 and a spinner glyph (U+2733, U+273B and the other
// U+27xx dingbats are E2 9C xx), so every title update dropped its text into
// whatever row the cursor was on, leaving stray letters behind that the
// application never knew about and so never overwrote.
//
// Strings end on BEL, on ESC (including ESC \), on CAN and on SUB, as xterm in
// UTF-8 mode does. The table is the package-level one the parser reads, so this
// runs once at start-up, before any emulator exists. The same applies to the
// high bytes of SOS, PM and APC strings, which the table sends back to ground.
func init() {
	for _, state := range []parser.State{
		parser.OscStringState,
		parser.DcsStringState,
		parser.SosStringState,
		parser.PmStringState,
		parser.ApcStringState,
	} {
		parser.Table.AddRange(0x80, 0xFF, state, parser.PutAction, state)
	}
}
