package vt

import (
	"io"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// modeAltScreenLegacy is DEC private mode 47, the original alternate screen
// switch that neither clears the screen nor saves the cursor. x/ansi only
// names its successors 1047 and 1049.
const modeAltScreenLegacy = ansi.DECMode(47)

func (e *Emulator) handleMode(params ansi.Params, set, isAnsi bool) {
	for _, p := range params {
		param := p.Param(-1)
		if param == -1 {
			// Missing parameter, ignore
			continue
		}

		var mode ansi.Mode = ansi.DECMode(param)
		if isAnsi {
			mode = ansi.ANSIMode(param)
		}

		setting := e.modes[mode]
		if setting == ansi.ModePermanentlyReset || setting == ansi.ModePermanentlySet {
			// Permanently set modes are ignored.
			continue
		}

		setting = ansi.ModeReset
		if set {
			setting = ansi.ModeSet
		}

		e.setMode(mode, setting)
	}
}

// setAltScreenMode switches to the alternate screen (on) or back to the
// normal one, and reports whether it switched. With clear set, the alternate
// screen is cleared as part of the switch: on entry it is cleared and the
// cursor homed (1049), on exit it is cleared before leaving it (1047). Mode 47
// switches without clearing either way.
//
// The pending-wrap flag belongs to the screen it was set on. It is parked on
// the screen being left and picked up from the screen being entered, so a wrap
// left pending by the last line an application drew does not wrap the shell's
// next character on the normal screen.
func (e *Emulator) setAltScreenMode(on, clear bool) bool {
	if (on && e.scr == &e.scrs[1]) || (!on && e.scr == &e.scrs[0]) {
		// Already in alternate screen mode, or normal screen, do nothing.
		return false
	}
	e.scr.phantom = e.atPhantom
	if on {
		e.scr = &e.scrs[1]
		e.scrs[1].cur = e.scrs[0].cur
		// The cursor came over as it was, wrap pending included.
		if clear {
			e.scr.Clear()
			e.scr.buf.Touched = nil
			e.setCursor(0, 0)
		}
	} else {
		if clear {
			e.scrs[1].Clear()
		}
		// Cursor visibility and style are process-global in a real terminal,
		// not scoped to whichever screen buffer happened to be current: an
		// ncurses app commonly hides the cursor before ever entering the
		// alternate screen and shows it again just before leaving (curs_set(0)
		// in initscr, curs_set(1) in endwin, both ahead of the smcup/rmcup
		// pair). Left alone, that show lands on the alt screen's cursor and
		// the primary screen keeps the "hidden" it had from before the app
		// ever started, so the shell prompt comes back with no visible
		// cursor. Carry the alt screen's final visibility and style back to
		// the primary screen on exit to match.
		altCur := e.scrs[1].cur
		e.scr = &e.scrs[0]
		e.scr.setCursorHidden(altCur.Hidden)
		e.scr.setCursorStyle(altCur.Style, !altCur.Steady)
		e.atPhantom = e.scr.phantom
	}
	// A screen switch ends any frame in progress; clear a stuck sync flag so a
	// window is never left holding a stale frame (e.g. when an app exits without
	// closing its synchronized update).
	e.cachedSyncOutput.Store(false)
	if e.cb.AltScreen != nil {
		e.cb.AltScreen(on)
	}
	if e.cb.CursorVisibility != nil {
		e.cb.CursorVisibility(!e.scr.cur.Hidden)
	}
	return true
}

// savedCursorState is what DECSC saves besides the [Cursor] itself (position,
// pen and hyperlink): the pending-wrap flag, origin mode and the character set
// state. It is kept per screen, next to the saved cursor. A zero value means
// nothing was saved, which DECRC treats as the power-on defaults.
type savedCursorState struct {
	valid      bool
	phantom    bool
	origin     bool
	charsets   [4]CharSet
	charsetIDs [4]byte
	gl, gr     int
	gsingle    int
}

// saveCursor saves the cursor as in [ansi.DECSC]: the position, the pen, the
// pending-wrap flag, origin mode and the character sets. Cursor visibility and
// shape are not part of it.
func (e *Emulator) saveCursor() {
	e.scr.SaveCursor()
	e.scr.savedState = savedCursorState{
		valid:      true,
		phantom:    e.atPhantom,
		origin:     e.isModeSet(ansi.ModeOrigin),
		charsets:   e.charsets,
		charsetIDs: e.charsetIDs,
		gl:         e.gl,
		gr:         e.gr,
		gsingle:    e.gsingle,
	}
}

// restoreCursor restores what saveCursor saved, as in [ansi.DECRC]. With
// nothing saved, the cursor goes home with the default pen, origin mode off and
// the default character sets, which is what xterm does.
func (e *Emulator) restoreCursor() {
	e.scr.RestoreCursor()
	st := e.scr.savedState
	if !st.valid {
		st = savedCursorState{charsetIDs: defaultCharsetIDs, gr: 1}
	}
	e.charsets = st.charsets
	e.charsetIDs = st.charsetIDs
	e.gl, e.gr, e.gsingle = st.gl, st.gr, st.gsingle
	e.setOriginMode(st.origin)
	e.atPhantom = st.phantom
}

// setOriginMode sets [ansi.DECOM] in the modes map without going through
// setMode, which DECRC and DECSTR would otherwise re-enter while setMode is
// already handling a mode change of its own (1048, 1049).
func (e *Emulator) setOriginMode(on bool) {
	setting := ansi.ModeReset
	if on {
		setting = ansi.ModeSet
	}
	e.modesMu.Lock()
	e.modes[ansi.ModeOrigin] = setting
	e.modesMu.Unlock()
}

// softReset performs a soft terminal reset as in [ansi.DECSTR], following
// xterm: the cursor is shown, insert mode, origin mode, application keypad and
// application cursor keys are reset, auto-wrap is set, the margins cover the
// whole screen, the pen and the character sets go back to their defaults and
// the saved cursor to home. The screen contents and the cursor position are
// left alone.
func (e *Emulator) softReset() {
	e.setMode(ansi.ModeTextCursorEnable, ansi.ModeSet)
	e.setMode(ansi.ModeInsertReplace, ansi.ModeReset)
	e.setMode(ansi.ModeOrigin, ansi.ModeReset)
	e.setMode(ansi.ModeAutoWrap, ansi.ModeSet)
	e.setMode(ansi.ModeNumericKeypad, ansi.ModeReset)
	e.setMode(ansi.ModeCursorKeys, ansi.ModeReset)
	e.scr.scroll = e.scr.Bounds()
	e.scr.cur.Pen = uv.Style{}
	e.scr.cur.Link = uv.Link{}
	e.charsets = [4]CharSet{}
	e.charsetIDs = defaultCharsetIDs
	e.gl, e.gr, e.gsingle = 0, 1, 0
	e.scr.saved = Cursor{}
	e.scr.savedState = savedCursorState{}
	e.atPhantom = false
}

// setMode sets the mode to the given value.
func (e *Emulator) setMode(mode ansi.Mode, setting ansi.ModeSetting) {
	e.logf("setting mode %T(%v) to %v", mode, mode, setting)
	e.modesMu.Lock()
	e.modes[mode] = setting
	e.modesMu.Unlock()
	switch mode {
	case ansi.ModeTextCursorEnable:
		e.scr.setCursorHidden(!setting.IsSet())
	case modeAltScreenLegacy: // 47: switch only
		e.setAltScreenMode(setting.IsSet(), false)
	case ansi.ModeAltScreen: // 1047: switch, clearing the alt screen on exit
		e.setAltScreenMode(setting.IsSet(), !setting.IsSet())
	case ansi.ModeSaveCursor:
		if setting.IsSet() {
			e.saveCursor()
		} else {
			e.restoreCursor()
		}
	case ansi.ModeAltScreenSaveCursor: // Alternate Screen Save Cursor (1049)
		// Save the primary screen cursor and switch to a cleared alternate
		// screen; on the way back restore the cursor as DECRC would, wrap flag
		// and character sets included. The alternate screen keeps no
		// scrollback.
		if setting.IsSet() {
			if e.scr == &e.scrs[0] {
				e.saveCursor()
			}
			e.setAltScreenMode(true, true)
		} else if e.setAltScreenMode(false, false) {
			e.restoreCursor()
		}
	case ansi.ModeInBandResize:
		if setting.IsSet() {
			_, _ = io.WriteString(e.pipe, ansi.InBandResize(e.Height(), e.Width(), 0, 0))
		}
	}
	if setting.IsSet() {
		if e.cb.EnableMode != nil {
			e.cb.EnableMode(mode)
		}
	} else if setting.IsReset() {
		if e.cb.DisableMode != nil {
			e.cb.DisableMode(mode)
		}
	}

	// Update thread-safe mode caches read from the render goroutine.
	e.updateMouseModeCache()
	if mode == ansi.ModeSynchronizedOutput {
		e.cachedSyncOutput.Store(setting.IsSet())
		if setting.IsSet() {
			e.syncSetAtNanos.Store(time.Now().UnixNano())
		}
	}
	if mode == ansi.ModeAutoWrap {
		e.cachedAutoWrap.Store(setting.IsSet())
	}
	if mode == ansi.ModeInsertReplace {
		e.cachedInsert.Store(setting.IsSet())
	}
}

// insertMode reports IRM (ANSI mode 4) without touching the modes map.
func (e *Emulator) insertMode() bool {
	return e.cachedInsert.Load()
}

// autoWrapMode reports DECAWM (?7) without touching the modes map.
//
// It exists for the callers that ask once per printed character or per line
// feed; everything colder should keep using isModeSet, which reads the map that
// remains authoritative. cachedAutoWrap is kept in step by setMode and
// RestoreModes, the only two writers of that entry.
func (e *Emulator) autoWrapMode() bool {
	return e.cachedAutoWrap.Load()
}

// isModeSet returns true if the mode is set.
func (e *Emulator) isModeSet(mode ansi.Mode) bool {
	e.modesMu.RLock()
	m, ok := e.modes[mode]
	e.modesMu.RUnlock()
	return ok && m.IsSet()
}

// ApplicationCursorKeys returns true if DECCKM (application cursor keys mode) is enabled.
// When this mode is set, cursor keys send SS3 sequences (ESC O A) instead of CSI sequences (ESC [ A).
func (e *Emulator) ApplicationCursorKeys() bool {
	return e.isModeSet(ansi.ModeCursorKeys)
}

// BracketedPasteEnabled returns true if bracketed paste mode (?2004) is enabled.
// When enabled, pasted text should be wrapped with escape sequences.
func (e *Emulator) BracketedPasteEnabled() bool {
	return e.isModeSet(ansi.ModeBracketedPaste)
}
