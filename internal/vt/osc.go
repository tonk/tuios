// Package vt provides a virtual terminal implementation.
package vt

import (
	"bytes"
	"encoding/base64"
	"image/color"
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// handleOsc handles an OSC escape sequence.
func (e *Emulator) handleOsc(cmd int, data []byte) {
	e.flushGrapheme() // Flush any pending grapheme before handling OSC sequences.
	if !e.handlers.handleOsc(cmd, data) {
		e.logf("unhandled sequence: OSC %q", data)
	}
}

func (e *Emulator) handleTitle(cmd int, data []byte) {
	// Split on the first ';' only; titles may legitimately contain semicolons.
	parts := bytes.SplitN(data, []byte{';'}, 2)
	if len(parts) != 2 {
		// Invalid, ignore
		return
	}
	switch cmd {
	case 0: // Set window title and icon name
		name := string(parts[1])
		e.iconName, e.title = name, name
		if e.cb.Title != nil {
			e.cb.Title(name)
		}
		if e.cb.IconName != nil {
			e.cb.IconName(name)
		}
	case 1: // Set icon name
		name := string(parts[1])
		e.iconName = name
		if e.cb.IconName != nil {
			e.cb.IconName(name)
		}
	case 2: // Set window title
		name := string(parts[1])
		e.title = name
		if e.cb.Title != nil {
			e.cb.Title(name)
		}
	}
}

func (e *Emulator) handleDefaultColor(cmd int, data []byte) {
	if cmd != 10 && cmd != 11 && cmd != 12 &&
		cmd != 110 && cmd != 111 && cmd != 112 {
		// Invalid, ignore
		return
	}

	parts := bytes.Split(data, []byte{';'})
	if len(parts) == 0 {
		// Invalid, ignore
		return
	}

	cb := func(c color.Color) {
		switch cmd {
		case 10, 110: // Foreground color
			e.SetForegroundColor(c)
		case 11, 111: // Background color
			e.SetBackgroundColor(c)
		case 12, 112: // Cursor color
			e.SetCursorColor(c)
		}
	}

	switch len(parts) {
	case 1: // Reset color
		cb(nil)
	case 2: // Set/Query color
		arg := string(parts[1])
		if arg == "?" {
			var xrgb ansi.XRGBColor
			switch cmd {
			case 10: // Query foreground color
				xrgb.Color = e.ForegroundColor()
				if xrgb.Color != nil {
					_, _ = io.WriteString(e.pipe, ansi.SetForegroundColor(xrgb.String()))
				}
			case 11: // Query background color
				xrgb.Color = e.BackgroundColor()
				if xrgb.Color != nil {
					_, _ = io.WriteString(e.pipe, ansi.SetBackgroundColor(xrgb.String()))
				}
			case 12: // Query cursor color
				xrgb.Color = e.CursorColor()
				if xrgb.Color != nil {
					_, _ = io.WriteString(e.pipe, ansi.SetCursorColor(xrgb.String()))
				}
			}
		} else if c := ansi.XParseColor(arg); c != nil {
			cb(c)
		}
	}
}

func (e *Emulator) handleWorkingDirectory(cmd int, data []byte) {
	if cmd != 7 {
		// Invalid, ignore
		return
	}

	// The data is the working directory path.
	parts := bytes.Split(data, []byte{';'})
	if len(parts) != 2 {
		// Invalid, ignore
		return
	}

	path := string(parts[1])
	e.cwd = path

	if e.cb.WorkingDirectory != nil {
		e.cb.WorkingDirectory(path)
	}
}

func (e *Emulator) handleSemanticZone(data []byte) {
	// OSC 133 format: "133;<subcommand>[;params]"
	// data includes the "133;" prefix from the parser
	parts := bytes.Split(data, []byte{';'})
	if len(parts) < 2 || len(parts[1]) == 0 {
		return
	}

	subCmd := parts[1][0] // 'A', 'B', 'C', or 'D'
	switch subCmd {
	case 'A', 'B', 'C', 'D':
		// valid
	default:
		return
	}

	curX, curY := e.scr.CursorPosition()
	absLine := e.ScrollbackLen() + curY

	exitCode := -1
	if subCmd == 'D' && len(parts) >= 3 {
		// Parse exit code from params (e.g., "D;0" or "D;1")
		code := 0
		for _, b := range parts[2] {
			if b >= '0' && b <= '9' {
				code = code*10 + int(b-'0')
			}
		}
		if len(parts[2]) > 0 {
			exitCode = code
		}
	}

	if e.semanticMarkers != nil {
		marker := SemanticMarker{
			Type:     SemanticMarkerType(subCmd),
			AbsLine:  absLine,
			Col:      curX,
			ExitCode: exitCode,
		}

		// On C marker (command executed), capture the command text from the
		// terminal buffer before the program's output overwrites it.
		// This is the only reliable time to read the command text.
		if subCmd == 'C' {
			if bMarker := e.semanticMarkers.Last(MarkerCommandStart); bMarker != nil {
				marker.CapturedText = e.extractCommandText(bMarker.AbsLine, bMarker.Col, absLine, curX)
			}
		}

		e.semanticMarkers.Add(marker)
	}
}

func (e *Emulator) handleTextSizing(data []byte) {
	parts := bytes.SplitN(data, []byte{';'}, 3)
	if len(parts) < 3 {
		return
	}
	text := string(parts[2])
	if text == "" {
		return
	}

	scale, width := parseTextSizingMeta(parts[1])
	e.paintTextSizing(text, scale, width)
}

// parseTextSizingMeta reads OSC 66 key=value pairs. s is the scale (1-7) and w
// is the explicit cell width (0 means "split as normal text"). Other keys
// (fractional scale, alignment) only affect font size inside those cells, which
// a cell-grid multiplexer cannot honor, so they are ignored.
func parseTextSizingMeta(meta []byte) (scale, width int) {
	scale, width = 1, 0
	for kv := range bytes.SplitSeq(meta, []byte{':'}) {
		k, v, ok := bytes.Cut(kv, []byte{'='})
		if !ok || len(v) == 0 {
			continue
		}
		n := 0
		for _, b := range v {
			if b < '0' || b > '9' {
				n = -1
				break
			}
			n = n*10 + int(b-'0')
		}
		if n < 0 {
			continue
		}
		switch string(k) {
		case "s":
			if n >= 1 && n <= 7 {
				scale = n
			}
		case "w":
			if n >= 0 && n <= 7 {
				width = n
			}
		}
	}
	return scale, width
}

// paintTextSizing puts OSC 66 text into the cell grid at the current cursor.
//
// Kitty's protocol carries the glyphs inside the escape sequence, so a handler
// that only forwards or clears leaves nothing on the screen. OpenTUI (and
// Cursor's agent TUI) detect that OSC 66 is consumed, then emit it for every
// explicit-width cell. The previous implementation wiped whole rows and never
// wrote the payload, which shredded that output into scattered letters.
//
// Scale > 1 still occupies s*w cells so CPR probes and app layout stay honest;
// the extra cells are left blank rather than drawn at a larger font size.
func (e *Emulator) paintTextSizing(text string, scale, width int) {
	if width > 0 {
		e.handleGrapheme(text, width)
		e.finishScaledCell(width, scale)
		return
	}
	method := ansi.GraphemeWidth
	for len(text) > 0 {
		cluster, w := ansi.FirstGraphemeCluster(text, method)
		e.handleGrapheme(cluster, w)
		e.finishScaledCell(w, scale)
		text = text[len(cluster):]
	}
}

// finishScaledCell occupies the rest of an s-by-(unitWidth*s) block after the
// primary cell has been written. The spec moves the cursor s*unitWidth cells
// right on the same row; extra columns and the rows below are blanked so stale
// glyphs do not show through the interior of the scaled cell.
func (e *Emulator) finishScaledCell(unitWidth, scale int) {
	if scale <= 1 || unitWidth <= 0 {
		return
	}
	extra := unitWidth * (scale - 1)
	x, y := e.scr.CursorPosition()
	w := e.scr.Width()
	h := e.scr.Height()
	for range extra {
		if x >= w {
			break
		}
		e.scr.SetCell(x, y, nil)
		x++
	}
	if x >= w {
		x = w - 1
		e.atPhantom = e.autoWrapMode()
	} else {
		e.atPhantom = false
	}
	e.scr.setCursor(x, y, false)

	startX := e.lastCellX
	blockW := unitWidth * scale
	for row := 1; row < scale; row++ {
		yy := e.lastCellY + row
		if yy >= h {
			break
		}
		for col := range blockW {
			xx := startX + col
			if xx >= w {
				break
			}
			e.scr.SetCell(xx, yy, nil)
		}
	}
}

func (e *Emulator) handlePaletteColor(data []byte) {
	// OSC 4 format: "4;<index>;<spec>" where spec can be "?" for query or an X11 color
	parts := bytes.Split(data, []byte{';'})
	if len(parts) < 3 {
		return
	}

	// Parse color index
	idx := 0
	for _, b := range parts[1] {
		if b >= '0' && b <= '9' {
			idx = idx*10 + int(b-'0')
		}
	}
	if idx < 0 || idx > 255 {
		return
	}

	arg := string(parts[2])
	if arg == "?" {
		// Query: respond with current color
		c := e.IndexedColor(idx)
		if c != nil {
			var xrgb ansi.XRGBColor
			xrgb.Color = c
			response := "\x1b]4;" + string(parts[1]) + ";" + xrgb.String() + "\x1b\\"
			_, _ = io.WriteString(e.pipe, response)
		}
	} else if c := ansi.XParseColor(arg); c != nil {
		// Set: update the palette entry
		e.SetIndexedColor(idx, c)
	}
}

func (e *Emulator) handleClipboard(data []byte) {
	// OSC 52 format: "52;<selection>;<base64-data>"
	// selection: c = clipboard, p = primary, s = secondary, etc.
	// base64-data: "?" to query, base64-encoded string to set
	parts := bytes.Split(data, []byte{';'})
	if len(parts) < 3 {
		return
	}

	selection := string(parts[1])
	payload := string(parts[2])

	if payload == "?" {
		// Query clipboard
		if e.cb.ClipboardQuery != nil {
			content := e.cb.ClipboardQuery(selection)
			encoded := base64.StdEncoding.EncodeToString([]byte(content))
			response := "\x1b]52;" + selection + ";" + encoded + "\x1b\\"
			_, _ = io.WriteString(e.pipe, response)
		} else {
			// No callback - respond empty
			response := "\x1b]52;" + selection + ";\x1b\\"
			_, _ = io.WriteString(e.pipe, response)
		}
	} else {
		// Set clipboard
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return
		}
		if e.cb.ClipboardSet != nil {
			e.cb.ClipboardSet(selection, string(decoded))
		}
	}
}

func (e *Emulator) handleHyperlink(cmd int, data []byte) {
	parts := bytes.Split(data, []byte{';'})
	if len(parts) != 3 || cmd != 8 {
		// Invalid, ignore
		return
	}

	e.scr.cur.Link.URL = string(parts[1])
	e.scr.cur.Link.Params = string(parts[2])
}

// handleNotify9 handles OSC 9 (iTerm2 desktop notification): "9;<msg>".
func (e *Emulator) handleNotify9(data []byte) bool {
	parts := strings.SplitN(string(data), ";", 2)
	if len(parts) < 2 {
		return true
	}
	msg := parts[1]
	// OSC 9;4 is the ConEmu progress-report sequence, not a notification: the
	// program is describing its own progress rather than asking for a desktop
	// alert, so it goes to the progress callback and never to Notify.
	if isProgressPayload(msg) {
		if state, percent, ok := parseProgress(msg); ok && e.cb.Progress != nil {
			e.cb.Progress(state, percent)
		}
		return true
	}
	if e.cb.Notify != nil {
		e.cb.Notify("", msg)
	}
	return true
}

// handleNotify777 handles OSC 777 (urxvt desktop notification):
// "777;notify;<title>;<body>".
func (e *Emulator) handleNotify777(data []byte) bool {
	parts := strings.SplitN(string(data), ";", 4)
	if len(parts) < 4 || parts[1] != "notify" {
		return true
	}
	if e.cb.Notify != nil {
		e.cb.Notify(parts[2], parts[3])
	}
	return true
}

// handleNotify99 handles OSC 99 (kitty desktop notification):
// "99;<metadata>;<payload>". Metadata is a colon-separated list of key=val
// pairs. This is a best-effort v1 parse: e=1 base64-decodes the payload,
// p=title routes the payload as the title, and d=0 continuation chunks are
// ignored rather than accumulated.
func (e *Emulator) handleNotify99(data []byte) bool {
	parts := strings.SplitN(string(data), ";", 3)
	meta := map[string]string{}
	if len(parts) >= 2 {
		for kv := range strings.SplitSeq(parts[1], ":") {
			if k, v, ok := strings.Cut(kv, "="); ok {
				meta[k] = v
			}
		}
	}
	// d=0 signals more chunks follow; skip continuation chunks best-effort.
	if meta["d"] == "0" {
		return true
	}
	payload := ""
	if len(parts) >= 3 {
		payload = parts[2]
	}
	if meta["e"] == "1" {
		if decoded, err := base64.StdEncoding.DecodeString(payload); err == nil {
			payload = string(decoded)
		}
	}
	title, body := "", payload
	if meta["p"] == "title" {
		title, body = payload, ""
	}
	if e.cb.Notify != nil {
		e.cb.Notify(title, body)
	}
	return true
}
