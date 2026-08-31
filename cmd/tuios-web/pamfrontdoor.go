package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/Gaurav-Gosain/tuios/internal/pamauth"
	"github.com/Gaurav-Gosain/tuios/internal/theme"
)

// newFrontDoor builds the public-facing HTTP(S) server used whenever
// --pam-auth or --web-settings is set: a reverse proxy to sip's own server
// (bound loopback-only, at internalAddr - see runWebServer) that optionally
// gates every request behind PAM Basic Auth (pamSocketPath != "") and
// optionally injects the --web-settings theme/font picker into the page it
// serves (injectSettings).
//
// This exists for two independent reasons, either of which needs the same
// mechanic - a caller-controlled layer in front of sip, since sip itself has
// no hook for either:
//
//   - PAM: sip's own page-load auth hook (checkAuth, guarding "/") only
//     supports a single fixed username/password pair via
//     sip.Config.BasicUsername/BasicPassword, not a dynamic per-trainee PAM
//     check - and gating only the WebSocket layer (a sip.ConnectMiddleware
//     alone, which is all sip exposes for that) never produces a login
//     prompt at all: a browser's native Basic Auth popup only appears for a
//     401 on a plain HTTP request such as the page load, never on a
//     WebSocket handshake, and the WebSocket API gives page JS no way to
//     attach credentials itself. Challenging the page load here is what
//     makes the browser cache credentials it then attaches automatically -
//     at the browser's network layer, invisibly to the page's own JS - to
//     the later WebSocket upgrade, where pamAuthMiddleware (running inside
//     sip's internal instance) establishes the actual session identity.
//   - Settings: sip's static/index.html (the settings panel, the gear icon)
//     is embedded into the sip module at build time; there's no plugin
//     point for adding rows to it. Rewriting the "/" response as it passes
//     through here is the only way to add to it without forking sip.
func newFrontDoor(publicAddr, internalAddr string, tlsConfig *tls.Config, pamSocketPath string, injectSettings bool) *http.Server {
	target := &url.URL{Scheme: "http", Host: internalAddr}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorLog = log.Default()
	if injectSettings {
		proxy.ModifyResponse = rewriteIndexResponse
	}

	mux := http.NewServeMux()
	if injectSettings {
		registerWebSettingsRoutes(mux)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only a plain "/" response is a reliable place to set a cookie from:
		// a WebSocket upgrade hijacks the connection and writes the
		// backend's own response headers directly, bypassing whatever this
		// ResponseWriter's headers were set to beforehand. "/" is always
		// requested before the page's own JS opens the WebSocket, so this
		// is never actually a race in practice.
		if injectSettings && r.URL.Path == "/" {
			ensureSessionCookie(w, r)
		}
		proxy.ServeHTTP(w, r)
	})

	// Wrapped around the whole mux, not inlined into the "/" handler above:
	// Go's ServeMux picks the most specific registered pattern for a
	// request, so a check that only ran inside the "/" handler never fired
	// for a more specific pattern registered elsewhere - which is exactly
	// how /tuios-settings/* (registered above by registerWebSettingsRoutes)
	// ended up reachable without a PAM login. Wrapping here instead means
	// every path this server answers is gated the same way, including any
	// future route, without each one having to remember to check.
	var handler http.Handler = mux
	if pamSocketPath != "" {
		handler = requirePAM(pamSocketPath, mux)
	}

	return &http.Server{
		Addr:      publicAddr,
		Handler:   handler,
		TLSConfig: tlsConfig,
	}
}

// requirePAM wraps next so every request must present valid PAM credentials
// first. See newFrontDoor for why this wraps the whole mux rather than being
// inlined into one route's handler.
func requirePAM(pamSocketPath string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			unauthorizedHTTP(w)
			return
		}
		if err := pamauth.Verify(pamSocketPath, username, password); err != nil {
			// Same reasoning as pamAuthMiddleware in pamauth.go: log the
			// real reason server-side, tell the browser nothing more than
			// "authentication failed".
			log.Printf("PAM login for %q failed: %v", username, err)
			unauthorizedHTTP(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func unauthorizedHTTP(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="tuios-web"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// rewriteIndexResponse is a httputil.ReverseProxy.ModifyResponse hook: for
// the "/" response specifically, it splices the --web-settings picker's
// markup into the HTML (see injectSettingsUI) and fixes up Content-Length to
// match. Every other path (static assets, /ws, /health, ...) passes through
// completely untouched - this only ever reads resp.Request.URL.Path, never
// buffers a response it isn't going to rewrite.
func rewriteIndexResponse(resp *http.Response) error {
	if resp.Request == nil || resp.Request.URL.Path != "/" {
		return nil
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	selectedFont := ""
	if c, err := resp.Request.Cookie(fontCookieName); err == nil {
		// The cookie was set via encodeURIComponent client-side (see
		// settingsInjectFooter), so it needs the matching decode here to
		// compare equal against settingsInjectHTML's plain option values.
		if decoded, err := url.PathUnescape(c.Value); err == nil {
			selectedFont = decoded
		}
	}
	bgHex := ""
	initialThemeJSON := "null"
	if t := theme.Current(); t != nil {
		if t.Bg != nil {
			// t.Bg is a *tint.Color; passing a nil one through the
			// color.Color interface here would not compare equal to nil (a
			// nil pointer wrapped in a non-nil interface), so the nil check
			// has to happen on the concrete type before it crosses into
			// ColorToString.
			bgHex = theme.ColorToString(t.Bg)
		}
		if encoded, err := json.Marshal(webTermThemeFor(t)); err == nil {
			initialThemeJSON = string(encoded)
		}
	}
	rewritten := injectSettingsUI(string(body), selectedFont, bgHex, initialThemeJSON)
	resp.Body = io.NopCloser(bytes.NewReader([]byte(rewritten)))
	resp.ContentLength = int64(len(rewritten))
	resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	return nil
}
