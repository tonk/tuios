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

	"github.com/Gaurav-Gosain/tuios/internal/app"
)

const sidCookieName = "tuios_sid"

// programRegistry maps a session id (see sidCookieName) to the running
// *tea.Program for that connection, so an out-of-band HTTP request can find
// it. Populated by wrapping sip's Handler into a ProgramHandler (see
// createTUIOSProgramHandler) and cleared when the session's context ends.
var programRegistry sync.Map // string -> *tea.Program

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
	mux.HandleFunc("/tuios-settings/themes", handleListThemes)
	mux.HandleFunc("/tuios-settings/theme", handleSetTheme)
	mux.HandleFunc("/tuios-settings/fonts/saucecodepro.ttf", handleBundledFont)
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(webTermThemeFor(t))
}

func handleBundledFont(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "font/ttf")
	_, _ = w.Write(sauceCodeProFont)
}

// fontCookieName persists the chosen font-family across a reload. Font
// family isn't part of sip's own live-appliable settings (unlike font
// *size*, which sip's stock panel already patches into a running terminal
// without a reload) - webterm's renderer is a minified third-party bundle
// with no confirmed way to rebuild its glyph atlas for a new face on the
// fly, so this uses the same reload-based pattern sip's own Renderer
// dropdown already falls back to for a setting that can't apply live (see
// terminal.js's settings-apply handler: "the renderer addon is attached
// once... so switching it needs a reload"). settingsInjectHead reads this
// cookie and overrides window.__sipConfig.fontFamily before terminal.js
// ever constructs the terminal, so the chosen font is what gets built with,
// not something patched in after.
const fontCookieName = "tuios_font"

// settingsInjectHead is spliced into "/" right before </head> - before any
// of sip's own body scripts run, which is what lets it override
// window.__sipConfig.fontFamily in time for terminal.js's construction of
// the terminal to see it.
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
func settingsInjectHead(bgHex string) string {
	bgRule := ""
	if bgHex != "" {
		bgRule = "body, html { background-color: " + bgHex + "; }\n    "
	}
	return `
    <style>
    @font-face {
        font-family: 'SauceCodePro NFM';
        src: url('/tuios-settings/fonts/saucecodepro.ttf');
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
    <script>
    (function() {
        var m = document.cookie.match(/(?:^|; )` + fontCookieName + `=([^;]*)/);
        if (m && m[1]) {
            window.__sipConfig = window.__sipConfig || {};
            window.__sipConfig.fontFamily = decodeURIComponent(m[1]);
        }
    })();
    </script>
`
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
            <label>Font Family (reloads the page)</label>
            <select id="tuios-font-select">
                <option value=""` + selected("") + `>Default</option>
                <option value="'JetBrainsMono Nerd Font Mono', monospace"` + selected("'JetBrainsMono Nerd Font Mono', monospace") + `>JetBrains Mono</option>
                <option value="'SauceCodePro NFM', monospace"` + selected("'SauceCodePro NFM', monospace") + `>SauceCodePro</option>
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

        whenReady(function() {
            applyTheme(` + initialTheme + `);

            var themeSelect = document.getElementById('tuios-theme-select');
            var fontSelect = document.getElementById('tuios-font-select');
            if (!themeSelect || !fontSelect) { return; }

            fetch('/tuios-settings/themes')
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
                fetch('/tuios-settings/theme', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ theme: themeSelect.value })
                })
                    .then(function(r) { return r.json(); })
                    .then(applyTheme)
                    .catch(function() {});
            });

            fontSelect.addEventListener('change', function() {
                document.cookie = '` + fontCookieName + `=' + encodeURIComponent(fontSelect.value) + '; path=/; max-age=31536000; SameSite=Lax';
                window.location.reload();
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
func injectSettingsUI(body, selectedFont, bgHex, initialThemeJSON string) string {
	body = strings.Replace(body, "</head>", settingsInjectHead(bgHex)+"</head>", 1)
	body = strings.Replace(body, `<button id="settings-apply"`, settingsInjectHTML(selectedFont)+`        <button id="settings-apply"`, 1)
	body = strings.Replace(body, "</body>", settingsInjectFooter(initialThemeJSON)+"</body>", 1)
	return body
}
