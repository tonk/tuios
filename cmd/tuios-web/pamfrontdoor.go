package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tonk/tuios/internal/pamauth"
	"github.com/tonk/tuios/internal/theme"
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
//
// A single page load is not one request here: sip's index.html alone pulls
// in two stylesheets, two font files and three scripts (see static/
// index.html in the vendored sip module), each proxied through this same
// handler, on top of "/" itself and the WebSocket upgrade that follows -
// nine-plus requests, all carrying the identical Basic Auth header a browser
// caches after the first 401. Verifying every one of them against the real
// pam-helper (a full PAM login: pam_unix's own crypt(), any slower module a
// deployment's /etc/pam.d stacks on top) paid that cost that many times for
// credentials that had not changed since the request before it, and was a
// real, measurable share of how long a browser took to get from opening the
// page to a working shell. A short-lived cache of already-verified
// (username, password) pairs collapses a page load's whole asset burst back
// down to the one real PAM call that actually has to happen, without
// weakening what a 401 gates: wrong or revoked credentials still fail
// immediately, and a cached pass expires quickly enough that it only ever
// covers requests that were always going to arrive within the same load.
func requirePAM(pamSocketPath string, next http.Handler) http.Handler {
	verified := newVerifiedCredentialCache()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			unauthorizedHTTP(w)
			return
		}
		if !verified.recentlyVerified(username, password) {
			if err := pamauth.Verify(pamSocketPath, username, password); err != nil {
				// Same reasoning as pamAuthMiddleware in pamauth.go: log the
				// real reason server-side, tell the browser nothing more than
				// "authentication failed".
				log.Printf("PAM login for %q failed: %v", username, err)
				unauthorizedHTTP(w)
				return
			}
			verified.remember(username, password)
		}
		next.ServeHTTP(w, r)
	})
}

// verifiedCredentialCacheTTL bounds how long a successful PAM verification is
// trusted without redialing pam-helper: long enough to cover one page load's
// burst of near-simultaneous asset requests (they all arrive within
// milliseconds of each other in practice), short enough that a revoked or
// changed password is only ever honored for a moment past whatever page load
// was already in flight when it changed - not meaningfully different from the
// ordinary window between a password change and this browser tab's next
// reload.
const verifiedCredentialCacheTTL = 10 * time.Second

// verifiedCredentialCache remembers which (username, password) pairs
// requirePAM has already confirmed with the real pam-helper recently, keyed
// by a hash rather than the raw password so a cache dump does not hand out
// plaintext credentials.
type verifiedCredentialCache struct {
	mu      sync.Mutex
	expires map[[sha256.Size]byte]time.Time
}

func newVerifiedCredentialCache() *verifiedCredentialCache {
	return &verifiedCredentialCache{expires: make(map[[sha256.Size]byte]time.Time)}
}

func verifiedCredentialKey(username, password string) [sha256.Size]byte {
	// A NUL separator: neither field can contain one, so two (username,
	// password) pairs can never collide onto the same concatenated bytes the
	// way naive concatenation could (e.g. "ab"+"c" vs "a"+"bc").
	return sha256.Sum256([]byte(username + "\x00" + password))
}

func (c *verifiedCredentialCache) recentlyVerified(username, password string) bool {
	key := verifiedCredentialKey(username, password)
	c.mu.Lock()
	defer c.mu.Unlock()
	exp, ok := c.expires[key]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(c.expires, key)
		return false
	}
	return true
}

// remember also sweeps every already-expired entry: a cache that only ever
// grows (one entry per distinct (username, password) pair anyone has ever
// tried, valid or not) would be a slow memory leak on a long-running
// service; piggybacking the sweep on the one call that adds an entry keeps
// this bounded without a separate goroutine or ticker for what is a tiny,
// infrequent map in practice.
func (c *verifiedCredentialCache) remember(username, password string) {
	key := verifiedCredentialKey(username, password)
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, exp := range c.expires {
		if now.After(exp) {
			delete(c.expires, k)
		}
	}
	c.expires[key] = now.Add(verifiedCredentialCacheTTL)
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
