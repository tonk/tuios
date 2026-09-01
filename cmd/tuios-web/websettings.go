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
	"github.com/Gaurav-Gosain/tuios/internal/theme"
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
	// Populates the tint registry with the built-in themes plus anything in
	// ~/.config/tuios/themes/ (custom themes tuios's own picker already
	// shows). Without this, tint.TintIDs()/tint.GetTint() below only ever
	// see the built-ins - a custom theme would silently be missing from the
	// list and rejected by handleSetTheme, since nothing else in tuios-web's
	// own startup path ever touches the tint registry before a request does.
	theme.EnsureRegistry()

	mux.HandleFunc("/tuios-settings/themes", handleListThemes)
	mux.HandleFunc("/tuios-settings/theme", handleSetTheme)
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
// theme like "training" pairing a larger, heavier font with its color
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
    @font-face {
        font-family: 'SauceCodePro NFM SemiBold';
        src: url('/tuios-settings/fonts/saucecodepro-semibold.ttf');
        font-weight: 100 900;
        font-style: normal;
        font-display: swap;
    }
    @font-face {
        font-family: 'FreeMono';
        src: url('/tuios-settings/fonts/freemono.ttf');
        font-weight: 100 900;
        font-style: normal;
        font-display: swap;
    }
    @font-face {
        font-family: 'FreeMono Bold';
        src: url('/tuios-settings/fonts/freemono-bold.ttf');
        font-weight: 100 900;
        font-style: normal;
        font-display: swap;
    }
    @font-face {
        font-family: 'Source Code Pro';
        src: url('/tuios-settings/fonts/sourcecodepro.ttf');
        font-weight: 100 900;
        font-style: normal;
        font-display: swap;
    }
    @font-face {
        font-family: 'Source Code Pro Bold';
        src: url('/tuios-settings/fonts/sourcecodepro-bold.ttf');
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
                    .then(function(resp) {
                        applyTheme(resp);
                        // resp.font/resp.fontSize are only set when the
                        // theme just selected carries a "web" preset (see
                        // setThemeResponse) - a theme like "training"
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
func injectSettingsUI(body, selectedFont, bgHex, initialThemeJSON string) string {
	body = strings.Replace(body, "</head>", settingsInjectHead(bgHex)+"</head>", 1)
	body = strings.Replace(body, `<button id="settings-apply"`, settingsInjectHTML(selectedFont)+`        <button id="settings-apply"`, 1)
	body = strings.Replace(body, "</body>", settingsInjectFooter(initialThemeJSON)+"</body>", 1)
	return body
}
