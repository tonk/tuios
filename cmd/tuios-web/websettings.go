// The optional --web-settings feature: a Theme and Font Family control added
// to sip's own browser settings panel (the gear icon), even though sip
// itself has no hook for adding to it. It works by injecting extra HTML/CSS/
// JS into the "/" response as it passes through the front door (see
// pamfrontdoor.go), and by adding a couple of new routes the front door
// serves directly rather than proxying.
//
// The one real design problem this creates: a setting change reaches the
// browser as a plain HTTP request (fetch()), not something that flows over
// the already-open WebSocket - sip's own message types are a closed set this
// package can't extend without forking sip. So the request has to say *which
// running session* it's for. That's what the tuios_sid cookie is for: the
// front door sets it once, on the first "/" load; the browser then attaches
// it automatically (its own network layer, not something the page's JS has
// to do) to every later request to this origin, including the WebSocket
// upgrade and the settings fetch() calls. A ConnectMiddleware (see below)
// reads it off the (proxied) WebSocket upgrade request and hands it to the
// program handler, which is what lets programRegistry map a settings
// request back to the right *tea.Program.
//
// Known limitation: the cookie is per-browser-origin, not per-tab. Two tabs
// of the same tuios-web page share one tuios_sid, so a settings change
// affects whichever tab's WebSocket connected most recently (it overwrites
// the registry entry). Fine for the common case, worth knowing about.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/Gaurav-Gosain/sip"
	"github.com/lrstanley/bubbletint/v2"

	"github.com/tonk/tuios/internal/app"
	"github.com/tonk/tuios/internal/theme"
)

const sidCookieName = "tuios_sid"

// programRegistry maps a session id (see sidCookieName) to the running
// *tea.Program for that connection, so an out-of-band HTTP request can find
// it. Populated by wrapping sip's Handler into a ProgramHandler (see
// createTUIOSProgramHandler) and cleared when the session's context ends.
var programRegistry sync.Map // string -> *tea.Program

// openTabTargets stashes the username a trainer picker (see
// classroom_picker.go's armOpenTab) just armed an open-tab title for, keyed
// by the same tuios_sid cookie as programRegistry. handlePickerOpenTab is
// the other end: the injected page script polls it over plain HTTP instead
// of depending solely on that title actually reaching the browser over the
// real terminal rendering pipeline, which is not reliable for every
// connection. One-shot: a read via LoadAndDelete clears it, matching the
// picker's own "arm once per Enter" semantics.
var openTabTargets sync.Map // string -> string

// sessionIDCtxKey types the request's session id in the request context,
// read off the tuios_sid cookie by sessionIDMiddleware.
type sessionIDCtxKey struct{}

// sessionIDMiddleware carries the tuios_sid cookie (set by the front door on
// the "/" response, see ensureSessionCookie) into the session context, the
// same pattern touchMiddleware and pamAuthMiddleware already use.
func sessionIDMiddleware() sip.ConnectMiddleware {
	return func(next sip.ConnectHandler) sip.ConnectHandler {
		return func(r *http.Request) error {
			if c, err := r.Cookie(sidCookieName); err == nil && c.Value != "" {
				r = r.WithContext(context.WithValue(r.Context(), sessionIDCtxKey{}, c.Value))
			}
			return next(r)
		}
	}
}

// createTUIOSProgramHandler wraps createTUIOSHandler into a sip.ProgramHandler
// (mirroring what sip.Server.Serve does internally for a plain sip.Handler -
// see newDefaultProgramHandler in the sip module) so this package can keep
// the resulting *tea.Program long enough to register it, instead of handing
// sip a bare model it builds and owns the Program for internally.
func createTUIOSProgramHandler() sip.ProgramHandler {
	return func(sess sip.Session) *tea.Program {
		m, opts := createTUIOSHandler(sess)
		if m == nil {
			return nil
		}
		program := tea.NewProgram(m, append(opts, sip.MakeOptions(sess)...)...)

		if sid, ok := sessionIDFromContext(sess.Context()); ok {
			programRegistry.Store(sid, program)
			go func() {
				<-sess.Context().Done()
				programRegistry.Delete(sid)
			}()
		}
		return program
	}
}

func sessionIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(sessionIDCtxKey{}).(string)
	return id, ok
}

// ensureSessionCookie returns the request's tuios_sid, generating and
// setting one on w if it doesn't have one yet. Only meaningful to call while
// handling "/" - see the package doc for why a plain HTTP response, not the
// WebSocket upgrade, is the reliable place to set a cookie from.
func ensureSessionCookie(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(sidCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	sid := hex.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{
		Name:     sidCookieName,
		Value:    sid,
		Path:     "/",
		HttpOnly: false, // the injected JS never needs to read it; the browser attaches it automatically either way
		SameSite: http.SameSiteLaxMode,
	})
	return sid
}

// registerWebSettingsRoutes adds the routes the front door serves directly
// (never proxied to sip): the theme list, the theme-change endpoint, and the
// bundled font files the injected picker offers.
func registerWebSettingsRoutes(mux *http.ServeMux) {
	// Populates the tint registry with the built-in themes plus anything in
	// ~/.config/tuios/themes/ (custom themes tuios's own picker already
	// shows). Without this, tint.TintIDs()/tint.GetTint() below only ever
	// see the built-ins - a custom theme would silently be missing from the
	// list and rejected by handleSetTheme, since nothing else in tuios-web's
	// own startup path ever touches the tint registry before a request does.
	theme.EnsureRegistry()

	mux.HandleFunc("/tuios-settings/themes", handleListThemes)
	mux.HandleFunc("/tuios-settings/theme", handleSetTheme)
	mux.HandleFunc("/tuios-settings/picker-open-tab", handlePickerOpenTab)
	mux.HandleFunc("/tuios-settings/fonts/saucecodepro.ttf", serveBundledFont(sauceCodeProFont))
	mux.HandleFunc("/tuios-settings/fonts/saucecodepro-semibold.ttf", serveBundledFont(sauceCodeProSemiBoldFont))
	mux.HandleFunc("/tuios-settings/fonts/freemono.ttf", serveBundledFont(freeMonoFont))
	mux.HandleFunc("/tuios-settings/fonts/freemono-bold.ttf", serveBundledFont(freeMonoBoldFont))
	mux.HandleFunc("/tuios-settings/fonts/sourcecodepro.ttf", serveBundledFont(sourceCodeProFont))
	mux.HandleFunc("/tuios-settings/fonts/sourcecodepro-bold.ttf", serveBundledFont(sourceCodeProBoldFont))
}

func handleListThemes(w http.ResponseWriter, _ *http.Request) {
	names := tint.TintIDs()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(names)
}

type setThemeRequest struct {
	Theme string `json:"theme"`
}

// webTermTheme mirrors the shape of the xterm-style Theme object webterm.js
// accepts (see terminal.js's own THEME constant and webtermOptions' `theme:
// THEME` field). It exists because tuios itself never paints a pane
// background - SetTheme always passes a nil background to the VT emulator,
// so whatever is behind a real terminal (configurable independently of
// tuios) shows through - but over the web there is no such thing behind
// sip's canvas, only whatever THEME it was constructed with once, at
// startup. Sending this back out lets the injected JS re-apply it via
// webterm.setOptions({theme: ...}), the same live-patch mechanism sip's own
// settings panel already uses for fontSize/cursorBlink/etc: confirmed live
// (via the WebGL renderer's own setTheme, which forces a full repaint) by
// driving it directly through devtools before wiring this up, since neither
// webterm.js nor terminal.js document it anywhere.
type webTermTheme struct {
	Foreground          string `json:"foreground,omitempty"`
	Background          string `json:"background,omitempty"`
	Cursor              string `json:"cursor,omitempty"`
	CursorAccent        string `json:"cursorAccent,omitempty"`
	SelectionBackground string `json:"selectionBackground,omitempty"`
	Black               string `json:"black,omitempty"`
	Red                 string `json:"red,omitempty"`
	Green               string `json:"green,omitempty"`
	Yellow              string `json:"yellow,omitempty"`
	Blue                string `json:"blue,omitempty"`
	Magenta             string `json:"magenta,omitempty"`
	Cyan                string `json:"cyan,omitempty"`
	White               string `json:"white,omitempty"`
	BrightBlack         string `json:"brightBlack,omitempty"`
	BrightRed           string `json:"brightRed,omitempty"`
	BrightGreen         string `json:"brightGreen,omitempty"`
	BrightYellow        string `json:"brightYellow,omitempty"`
	BrightBlue          string `json:"brightBlue,omitempty"`
	BrightMagenta       string `json:"brightMagenta,omitempty"`
	BrightCyan          string `json:"brightCyan,omitempty"`
	BrightWhite         string `json:"brightWhite,omitempty"`
}

// webTermThemeFor converts a tint.Tint (bubbletint's palette type, the same
// one internal/theme drives the TUI's own rendering from) into the shape
// webterm.js expects. t is assumed non-nil; callers guard that themselves
// since "no theme" and "empty webTermTheme" mean different things to them.
func webTermThemeFor(t *tint.Tint) webTermTheme {
	hex := func(c *tint.Color) string {
		if c == nil {
			return ""
		}
		return c.Hex()
	}
	// Cursor is missing from most themes (see tint.Tint's own doc comment);
	// falling back to the foreground color matches what a real terminal
	// emulator does absent an explicit cursor color.
	cursor := hex(t.Cursor)
	if cursor == "" {
		cursor = hex(t.Fg)
	}
	return webTermTheme{
		Foreground:          hex(t.Fg),
		Background:          hex(t.Bg),
		Cursor:              cursor,
		CursorAccent:        hex(t.Bg),
		SelectionBackground: hex(t.SelectionBg),
		Black:               hex(t.Black),
		Red:                 hex(t.Red),
		Green:               hex(t.Green),
		Yellow:              hex(t.Yellow),
		Blue:                hex(t.Blue),
		Magenta:             hex(t.Purple),
		Cyan:                hex(t.Cyan),
		White:               hex(t.White),
		BrightBlack:         hex(t.BrightBlack),
		BrightRed:           hex(t.BrightRed),
		BrightGreen:         hex(t.BrightGreen),
		BrightYellow:        hex(t.BrightYellow),
		BrightBlue:          hex(t.BrightBlue),
		BrightMagenta:       hex(t.BrightPurple),
		BrightCyan:          hex(t.BrightCyan),
		BrightWhite:         hex(t.BrightWhite),
	}
}

// setThemeResponse is what handleSetTheme sends back: the webTermTheme JSON
// shape it always sent, plus an optional font/fontSize pair when the newly
// selected theme carries a "web" preset (see internal/theme.WebPreset) - a
// theme like "trainer" pairing a larger, heavier font with its color
// palette. Both are omitted entirely when the theme has no preset, so the
// injected JS's `if (resp.font)`/`if (resp.fontSize)` checks leave the
// operator's own font choice alone.
type setThemeResponse struct {
	webTermTheme
	Font     string `json:"font,omitempty"`
	FontSize int    `json:"fontSize,omitempty"`
}

func handleSetTheme(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c, err := r.Cookie(sidCookieName)
	if err != nil || c.Value == "" {
		http.Error(w, "no session", http.StatusBadRequest)
		return
	}
	var req setThemeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Theme == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	t, ok := tint.GetTint(req.Theme)
	if !ok {
		http.Error(w, "unknown theme", http.StatusBadRequest)
		return
	}
	v, ok := programRegistry.Load(c.Value)
	if !ok {
		http.Error(w, "session not found (reload the page)", http.StatusNotFound)
		return
	}
	program, ok := v.(*tea.Program)
	if !ok || program == nil {
		http.Error(w, "session not found (reload the page)", http.StatusNotFound)
		return
	}
	program.Send(app.SetThemeMsg{Theme: req.Theme})

	resp := setThemeResponse{webTermTheme: webTermThemeFor(t)}
	if wp := theme.WebPresetForID(req.Theme); wp != nil {
		resp.Font = webFontCSSValue(wp.Font)
		resp.FontSize = wp.FontSize
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handlePickerOpenTab is polled by the injected page script after Enter on
// the trainer picker: it hands back (and clears) whatever username
// armOpenTab last stashed in openTabTargets for this connection's tuios_sid,
// so the page can navigate there without depending on the window-title
// relay actually reaching the browser. Empty user (still 200, not 404)
// means nothing is armed yet - normal while a poll is still waiting on the
// server to process the keypress, not an error.
func handlePickerOpenTab(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	c, err := r.Cookie(sidCookieName)
	if err != nil || c.Value == "" {
		_ = json.NewEncoder(w).Encode(pickerOpenTabResponse{})
		return
	}
	user := ""
	if v, ok := openTabTargets.LoadAndDelete(c.Value); ok {
		if s, ok := v.(string); ok {
			user = s
		}
	}
	_ = json.NewEncoder(w).Encode(pickerOpenTabResponse{User: user})
}

type pickerOpenTabResponse struct {
	User string `json:"user"`
}

// serveBundledFont returns a handler that serves one embedded font's bytes,
// shared by every bundledFonts route registered in registerWebSettingsRoutes.
func serveBundledFont(data []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "font/ttf")
		_, _ = w.Write(data)
	}
}

// fontCookieName persists the chosen font-family across a reload, applied
// after the terminal exists (see settingsInjectFooter's applyFontFamily)
// rather than by overriding window.__sipConfig.fontFamily before
// terminal.js constructs the terminal - deliberately, not by oversight.
//
// webterm.js's own font-preload step (xC in webterm.js, driven by
// terminal.js's hardcoded `fonts: [...]` array of its four JetBrains Mono
// files, none of which carry a `family` field) falls back to whatever
// fontFamily the terminal is being constructed with as the family name for
// *all four* of those FontFace objects: `new FontFace(g.family ?? t, ...)`.
// Construct with a custom fontFamily already set (as overriding
// __sipConfig.fontFamily before construction used to do) and this
// registers four JetBrains-Mono-content FontFace objects under the custom
// family name, then marks them "loaded" - and a browser prefers an
// already-loaded face over triggering a new load for our own real
// @font-face rule at the same family/weight, so the canvas keeps rendering
// JetBrains Mono's outlines no matter which custom font is selected. Not
// theoretical: confirmed by inspecting document.fonts (duplicate entries
// under the same family, the bogus ones with literal quote characters
// baked into .family from how they were constructed) and by rendering
// both ways and diffing the pixels.
//
// The construction-time path is never used at all for a custom font, for
// exactly this reason: the terminal always constructs with sip's own
// default (JetBrains Mono), which is what xC's array actually matches, and
// a saved custom choice is only ever applied afterward via
// webterm.setOptions({fontFamily: ...}) - which does not go through
// doOpen/xC again, so it never re-triggers this.
const fontCookieName = "tuios_font"

// frontDoorWebsocketHead seeds sip's localStorage transport to "websocket",
// forwards ?attach= onto the WebSocket upgrade URL (sip drops search params
// when building /ws), and wires the trainer-console "open in new tab" bridge.
//
// Behind the front door (--pam-auth / --web-settings) sip's WebTransport
// listener is loopback-only and not reverse-proxied, so "auto" always fails
// over after a browser QUIC timeout; forcing websocket removes that delay.
// Overwrites a stored "auto" as well: that value is the unreachable default,
// not a deliberate user choice of webtransport.
//
// The open-tab bridge: while document.title is "tuios-trainer-picker", Enter
// synchronously opens about:blank (user-gesture, so popup blockers allow it).
// When the picker then sets title to tuios-open-tab:<seq>:<user>, that blank
// tab is navigated to the same URL with ?attach=<user>. Opening only after
// the title arrives would be async and get blocked.
func frontDoorWebsocketHead(ownUser string) string {
	ownUserJSON, _ := json.Marshal(ownUser)
	return `
    <script>
    (function() {
        // The authenticated user's own username, for the "My own session"
        // fallback below - empty if this request carried none (not a
        // trainer console front door, or auth was not Basic Auth).
        var TUIOS_OWN_USER = ` + string(ownUserJSON) + `;
        try {
            var raw = localStorage.getItem('sip-web-settings');
            var s = raw ? JSON.parse(raw) : {};
            var changed = !raw;
            if (s.cursorBlink === undefined) { s.cursorBlink = true; changed = true; }
            if (s.copyOnSelect === undefined) { s.copyOnSelect = true; changed = true; }
            // sip's default transport is "auto", which tries WebTransport
            // first. Behind this front door that listener is unreachable, so
            // force websocket (including overwriting a stale "auto").
            if (s.transport !== 'websocket') {
                s.transport = 'websocket';
                changed = true;
            }
            if (changed) {
                localStorage.setItem('sip-web-settings', JSON.stringify(s));
            }
        } catch (e) {}

        // sip's WebSocket URL is built from the page path alone and drops
        // location.search. pamAuthMiddleware reads ?attach= on the upgrade
        // request, so without this bridge a tab opened as ?attach=guru01
        // still lands on the trainer picker (no attach on /ws).
        (function forwardAttachQueryToWebSocket() {
            var attach = '';
            try {
                attach = new URLSearchParams(window.location.search).get('attach') || '';
            } catch (e) { return; }
            if (!attach || !/^[A-Za-z0-9._-]+$/.test(attach)) return;
            var Orig = window.WebSocket;
            if (!Orig) return;
            function Wrapped(url, protocols) {
                try {
                    var u = new URL(url, window.location.href);
                    if (/\/ws\/?$/.test(u.pathname) || u.pathname.endsWith('ws')) {
                        u.searchParams.set('attach', attach);
                        url = u.toString();
                    }
                } catch (e) {}
                if (protocols === undefined) return new Orig(url);
                return new Orig(url, protocols);
            }
            Wrapped.prototype = Orig.prototype;
            Wrapped.CONNECTING = Orig.CONNECTING;
            Wrapped.OPEN = Orig.OPEN;
            Wrapped.CLOSING = Orig.CLOSING;
            Wrapped.CLOSED = Orig.CLOSED;
            window.WebSocket = Wrapped;
        })();

        // Trainer picker → new browser tab (see classroom_picker.go).
        var inPicker = false;
        var pendingWin = null;
        var pendingTimer = null;
        var lastOpenTitle = '';
        // pickerCursor mirrors classroomPickerModel.cursor purely from the
        // up/down keys this page itself sees, so it is only ever a guess -
        // but cursor 0 ("My own session", the picker's fixed first entry) is
        // also where every fresh page load starts, matching the server's own
        // initial state, which is what makes it safe to use as a fallback
        // destination below when the normal title-relay round trip does not
        // arrive in time.
        var pickerCursor = 0;
        // attachStarted guards against openAttach running twice for the same
        // Enter press: it can now be reached two ways (the polled HTTP
        // endpoint below, and the older window-title relay, kept as a second
        // chance in case it ever does arrive first) - without this a title
        // that arrives just after the poll already navigated would open a
        // second, unwanted popup instead of a no-op.
        var attachStarted = false;
        // pollAttempt tags each Enter press's poll loop so an old one still
        // in flight when a new Enter starts stops touching shared state
        // instead of firing a stale navigation.
        var pollAttempt = 0;
        function pollOpenTabTarget(attempt) {
            if (attempt !== pollAttempt || attachStarted) return;
            fetch('/tuios-settings/picker-open-tab', {credentials: 'same-origin'})
                .then(function(r) { return r.ok ? r.json() : null; })
                .then(function(data) {
                    if (attempt !== pollAttempt || attachStarted) return;
                    if (data && data.user) {
                        openAttach(data.user);
                        return;
                    }
                    setTimeout(function() { pollOpenTabTarget(attempt); }, 200);
                })
                .catch(function() {
                    if (attempt !== pollAttempt || attachStarted) return;
                    setTimeout(function() { pollOpenTabTarget(attempt); }, 200);
                });
        }
        // showToast is a minimal, self-removing on-page notice. The picker is
        // rendered entirely server-side (a bubbletea View streamed over the
        // terminal), so there is no existing DOM element to put a message
        // into - this is the simplest way to surface a client-side-only
        // failure (the placeholder tab timing out) without a blocking
        // alert() the user has to dismiss.
        function showToast(text) {
            try {
                var el = document.createElement('div');
                el.textContent = text;
                el.style.cssText = 'position:fixed;top:12px;left:50%;transform:translateX(-50%);' +
                    'background:#7f1d1d;color:#fff;padding:8px 16px;border-radius:6px;' +
                    'font:14px/1.4 sans-serif;z-index:2147483647;box-shadow:0 2px 8px rgba(0,0,0,.4)';
                document.body.appendChild(el);
                setTimeout(function() { try { el.remove(); } catch (e) {} }, 5000);
            } catch (e) {}
        }
        function clearPending() {
            if (pendingTimer) { clearTimeout(pendingTimer); pendingTimer = null; }
            if (pendingWin && !pendingWin.closed) {
                try { pendingWin.close(); } catch (e) {}
            }
            pendingWin = null;
            pollAttempt++;
            attachStarted = false;
        }
        // clearPendingOnTimeout runs only when the placeholder tab's own
        // failsafe timer fires - i.e. the server's title-change response to
        // Enter never arrived in time - as opposed to clearPending() being
        // called proactively (a fresh Enter, or the tab having already
        // navigated for real). That distinction is what makes this the right
        // place to tell the user something actually went wrong, instead of
        // leaving them staring at a tab that silently vanished.
        //
        // When the target was "My own session" (cursor 0) and the page knows
        // its own username, this does not just report the failure - it
        // navigates the current tab there directly, bypassing the title
        // relay entirely. That relay has turned out to be unreliable in ways
        // that do not depend on browser, network, or proxy buffering, so a
        // path that does not need it at all is the only thing guaranteed to
        // actually get you to your own session.
        function clearPendingOnTimeout() {
            clearPending();
            // clearPending() just reset this for a fresh attempt, but there
            // is no fresh attempt here - this is the end of the one already
            // in flight (both branches below), so a stale poll response or
            // title arriving after this point must not also fire.
            attachStarted = true;
            if (pickerCursor === 0 && TUIOS_OWN_USER) {
                showToast("That's taking too long - going there directly instead.");
                try {
                    var u = new URL(window.location.href);
                    u.searchParams.set('attach', TUIOS_OWN_USER);
                    window.location.href = u.href;
                } catch (e) {}
                return;
            }
            showToast("Couldn't open that session in time - please try again.");
        }
        function openAttach(user) {
            if (attachStarted) return;
            attachStarted = true;
            if (pendingTimer) { clearTimeout(pendingTimer); pendingTimer = null; }
            try {
                var u = new URL(window.location.href);
                u.searchParams.set('attach', user);
                if (pendingWin && !pendingWin.closed) {
                    pendingWin.location = u.href;
                    pendingWin = null;
                    return;
                }
                window.open(u.href, '_blank');
            } catch (e) {}
        }
        function onTitle(t) {
            t = t || '';
            if (t === 'tuios-trainer-picker') {
                inPicker = true;
                return;
            }
            var m = /^tuios-open-tab:(\d+):([A-Za-z0-9._-]+)$/.exec(t);
            if (m) {
                inPicker = true;
                if (t === lastOpenTitle) return;
                lastOpenTitle = t;
                openAttach(m[2]);
                return;
            }
            // Left the picker for a real session title.
            if (t && t.indexOf('tuios-open-tab:') !== 0 && t !== 'tuios-trainer-picker') {
                inPicker = false;
                clearPending();
            }
        }
        document.addEventListener('keydown', function(e) {
            if (!inPicker || e.repeat || e.ctrlKey || e.altKey || e.metaKey) return;
            if (e.key === 'ArrowDown' || e.key === 'j') {
                pickerCursor++;
                return;
            }
            if (e.key === 'ArrowUp' || e.key === 'k') {
                if (pickerCursor > 0) pickerCursor--;
                return;
            }
            if (e.key !== 'Enter') return;
            clearPending();
            try { pendingWin = window.open('about:blank', '_blank'); } catch (err) { pendingWin = null; }
            // 10s: generous headroom for the poll below (normally sub-second)
            // plus the window-title relay kept as a second chance, in case a
            // slow connection needs it - either can win, openAttach's
            // attachStarted guard makes a race between them harmless.
            pendingTimer = setTimeout(clearPendingOnTimeout, 10000);
            pollOpenTabTarget(pollAttempt);
        }, true);
        // Poll rather than intercept the document.title property descriptor.
        // sip's own terminal.js also assigns document.title directly during
        // its own (later) init; redefining the property here is a race that
        // can be silently lost - whichever of the two runs last wins, with no
        // error either way, since redefining a configurable property is
        // silent. Polling never has an owner to lose to.
        var lastPolledTitle = null;
        setInterval(function() {
            if (document.title !== lastPolledTitle) {
                lastPolledTitle = document.title;
                onTitle(document.title);
            }
        }, 100);
        onTitle(document.title);
    })();
    </script>
`
}

// settingsInjectHead is spliced into "/" right before </head>.
//
// bgHex bakes in the currently active theme's background (see
// setThemeResponse for why sip's page needs this at all: tuios itself never
// paints a pane background, so nothing else would ever set it). This is what
// a fresh load or reload shows immediately, before the settings-panel JS has
// even run - without it, a reload after picking a theme would flash back to
// sip's own fixed background until the panel finished loading, if it ever
// corrected it at all. An empty bgHex (theming disabled) injects no rule, so
// sip's own CSS default applies exactly as it did before this feature
// existed. This rule and the live one settingsInjectFooter sets on a change
// use different specificity levels: this one is an ordinary "body, html"
// stylesheet rule, so live updates only need a plain element.style.background
// set (inline style always outranks a stylesheet rule, !important or not) to
// override it, with no !important tug-of-war between the two.
func settingsInjectHead(bgHex, ownUser string) string {
	bgRule := ""
	if bgHex != "" {
		bgRule = "body, html { background-color: " + bgHex + "; }\n    "
	}
	return `
    <style>
    @font-face {
        font-family: 'SauceCodePro NFM';
        src: url('tuios-settings/fonts/saucecodepro.ttf');
        font-weight: 100 900;
        font-style: normal;
        font-display: swap;
    }
    @font-face {
        font-family: 'SauceCodePro NFM SemiBold';
        src: url('tuios-settings/fonts/saucecodepro-semibold.ttf');
        font-weight: 100 900;
        font-style: normal;
        font-display: swap;
    }
    @font-face {
        font-family: 'FreeMono';
        src: url('tuios-settings/fonts/freemono.ttf');
        font-weight: 100 900;
        font-style: normal;
        font-display: swap;
    }
    @font-face {
        font-family: 'FreeMono Bold';
        src: url('tuios-settings/fonts/freemono-bold.ttf');
        font-weight: 100 900;
        font-style: normal;
        font-display: swap;
    }
    @font-face {
        font-family: 'Source Code Pro';
        src: url('tuios-settings/fonts/sourcecodepro.ttf');
        font-weight: 100 900;
        font-style: normal;
        font-display: swap;
    }
    @font-face {
        font-family: 'Source Code Pro Bold';
        src: url('tuios-settings/fonts/sourcecodepro-bold.ttf');
        font-weight: 100 900;
        font-style: normal;
        font-display: swap;
    }
    /* sip's own #settings-panel rule has no bottom constraint in the
       desktop (non-touch) case - only its body.sip-touch variant scrolls.
       The two rows this feature adds land at the very bottom, right before
       the Apply button, so on a short window the panel simply grows past
       the viewport edge with nothing to scroll: this makes it fit instead. */
    #settings-panel {
        max-height: calc(100vh - 70px);
        overflow-y: auto;
    }
    ` + bgRule + `</style>
` + frontDoorWebsocketHead(ownUser)
}

// settingsInjectHTML is spliced into the "/" response right before the
// settings panel's Apply button - see injectSettingsUI. Two new setting-group
// rows reusing the panel's own CSS classes, so they look native. selectedFont
// is the current fontCookieName value (possibly empty), used to pre-select
// the right option so the dropdown reflects what's actually active.
func settingsInjectHTML(selectedFont string) string {
	selected := func(value string) string {
		if value == selectedFont {
			return " selected"
		}
		return ""
	}
	return `
        <div class="setting-group">
            <label>tuios Theme</label>
            <select id="tuios-theme-select"><option value="">Loading…</option></select>
        </div>
        <div class="setting-group">
            <label>Font Family</label>
            <select id="tuios-font-select">
                <option value=""` + selected("") + `>Default</option>
                <option value="'JetBrainsMono Nerd Font Mono', monospace"` + selected("'JetBrainsMono Nerd Font Mono', monospace") + `>JetBrains Mono</option>
                <option value="'SauceCodePro NFM', monospace"` + selected("'SauceCodePro NFM', monospace") + `>SauceCodePro</option>
                <option value="'SauceCodePro NFM SemiBold', monospace"` + selected("'SauceCodePro NFM SemiBold', monospace") + `>SauceCodePro SemiBold</option>
                <option value="'FreeMono', monospace"` + selected("'FreeMono', monospace") + `>FreeMono</option>
                <option value="'FreeMono Bold', monospace"` + selected("'FreeMono Bold', monospace") + `>FreeMono Bold</option>
                <option value="'Source Code Pro', monospace"` + selected("'Source Code Pro', monospace") + `>Source Code Pro</option>
                <option value="'Source Code Pro Bold', monospace"` + selected("'Source Code Pro Bold', monospace") + `>Source Code Pro Bold</option>
            </select>
        </div>
`
}

// settingsInjectFooter is spliced into "/" right before </body>. initialTheme
// is the JSON-encoded webTermTheme for the theme active when this page was
// served ("null" when theming is disabled), applied once webterm exists so a
// reload does not silently drop back to sip's own built-in palette.
func settingsInjectFooter(initialTheme string) string {
	return `
    <script>
    (function() {
        function whenReady(fn) {
            if (window.sip && window.sip.term && window.sip.term.webterm) { fn(); return; }
            setTimeout(function() { whenReady(fn); }, 100);
        }

        // Shared by the initial-load application below and the dropdown's
        // change handler. webterm.setOptions({theme: ...}) is confirmed live
        // (its WebGL/Canvas renderer's own setTheme forces a full repaint) -
        // neither webterm.js nor terminal.js document this anywhere; sip's
        // own settings panel never had a reason to try it, only ever
        // patching fontSize/cursorBlink/etc. The plain CSS background is a
        // cheap fallback for the sliver of page the canvas does not cover.
        function applyTheme(t) {
            if (!t) { return; }
            window.sip.term.webterm.setOptions({ theme: t });
            if (t.background) {
                document.body.style.backgroundColor = t.background;
                document.documentElement.style.backgroundColor = t.background;
            }
        }

        // Applied only after the terminal already exists - never by setting
        // window.__sipConfig.fontFamily before construction. See
        // fontCookieName's doc comment: doing it before construction is what
        // makes webterm.js's own font-preload step register its four
        // hardcoded JetBrains Mono files under our custom family name
        // instead of theirs, and those end up winning over our real
        // @font-face rule. setOptions never goes through that preload step,
        // so it is the only safe way to apply a custom font at all - which
        // also means this needs no reload, unlike the font-family picker's
        // very first version.
        function applyFontFamily(family) {
            if (!family) { return; }
            window.sip.term.webterm.setOptions({ fontFamily: family });
            window.sip.term.fontFamily = family;
        }

        // fontSize is confirmed live via the exact same setOptions call sip's
        // own settings panel makes from its Apply button handler (see
        // terminal.js) - only that button also persists the change, which
        // this mirrors via window.sip.term.saveSettings() so a theme's font
        // preset survives a reload, and updates the panel's own #font-size
        // slider so it does not silently disagree with what is on screen.
        function applyFontSize(size) {
            if (!size) { return; }
            window.sip.term.webterm.setOptions({ fontSize: size });
            window.sip.settings.fontSize = size;
            window.sip.term.saveSettings();
            var input = document.getElementById('font-size');
            var label = document.getElementById('font-size-value');
            if (input) { input.value = size; }
            if (label) { label.textContent = size + 'px'; }
        }

        whenReady(function() {
            applyTheme(` + initialTheme + `);

            var savedFont = document.cookie.match(/(?:^|; )` + fontCookieName + `=([^;]*)/);
            if (savedFont && savedFont[1]) {
                applyFontFamily(decodeURIComponent(savedFont[1]));
            }

            var themeSelect = document.getElementById('tuios-theme-select');
            var fontSelect = document.getElementById('tuios-font-select');
            if (!themeSelect || !fontSelect) { return; }

            // Document-relative, not a leading-slash absolute path: sip's
            // own static assets (see its index.html: 'static/webterm.js',
            // not '/static/webterm.js') already resolve this way so the
            // whole app still works when a reverse proxy mounts it under a
            // subpath instead of a domain root (e.g. https://host/tuios/
            // rather than https://tuios.example.com/) - an absolute path
            // here would resolve to the proxy's own root and 404, even
            // though every other asset on the page loaded fine.
            fetch('tuios-settings/themes')
                .then(function(r) { return r.json(); })
                .then(function(themes) {
                    themeSelect.innerHTML = '<option value="">Unchanged</option>';
                    themes.forEach(function(name) {
                        var opt = document.createElement('option');
                        opt.value = name;
                        opt.textContent = name;
                        themeSelect.appendChild(opt);
                    });
                })
                .catch(function() {
                    themeSelect.innerHTML = '<option value="">(failed to load)</option>';
                });

            themeSelect.addEventListener('change', function() {
                if (!themeSelect.value) { return; }
                fetch('tuios-settings/theme', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ theme: themeSelect.value })
                })
                    .then(function(r) { return r.json(); })
                    .then(function(resp) {
                        applyTheme(resp);
                        // resp.font/resp.fontSize are only set when the
                        // theme just selected carries a "web" preset (see
                        // setThemeResponse) - a theme like "trainer"
                        // pairing a larger, heavier font with its palette.
                        // Sync the font dropdown and its cookie so the
                        // preset persists across a reload exactly like a
                        // manual font pick would.
                        if (resp.font) {
                            fontSelect.value = resp.font;
                            document.cookie = '` + fontCookieName + `=' + encodeURIComponent(resp.font) + '; path=/; max-age=31536000; SameSite=Lax';
                            applyFontFamily(resp.font);
                        }
                        applyFontSize(resp.fontSize);
                    })
                    .catch(function() {});
            });

            fontSelect.addEventListener('change', function() {
                document.cookie = '` + fontCookieName + `=' + encodeURIComponent(fontSelect.value) + '; path=/; max-age=31536000; SameSite=Lax';
                applyFontFamily(fontSelect.value);
            });
        });
    })();
    </script>
`
}

// injectSettingsUI splices settingsInjectHead/settingsInjectHTML/
// settingsInjectFooter into the "/" response's HTML. Called from
// rewriteIndexResponse (pamfrontdoor.go), which handles the actual
// Content-Length fixup; this function only does string surgery. bgHex and
// initialThemeJSON both describe the theme active when this page was served
// (empty / "null" when theming is disabled) - rewriteIndexResponse computes
// both from the same *tint.Tint, one for the CSS fallback, one for the JS
// webterm.setOptions call.
func injectSettingsUI(body, selectedFont, bgHex, initialThemeJSON, ownUser string) string {
	body = strings.Replace(body, "</head>", settingsInjectHead(bgHex, ownUser)+"</head>", 1)
	body = strings.Replace(body, `<button id="settings-apply"`, settingsInjectHTML(selectedFont)+`        <button id="settings-apply"`, 1)
	body = strings.Replace(body, "</body>", settingsInjectFooter(initialThemeJSON)+"</body>", 1)
	return body
}
