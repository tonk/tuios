package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/Gaurav-Gosain/tuios/internal/pamauth"
)

// newPAMFrontDoor builds the public-facing HTTP(S) server for --pam-auth
// mode: a reverse proxy to sip's own server (which is bound loopback-only,
// at internalAddr, when --pam-auth is set - see runWebServer) that gates
// every request behind PAM Basic Auth before anything reaches it.
//
// This exists because sip's own page-load auth hook (checkAuth, guarding
// "/") only supports a single fixed username/password pair via
// sip.Config.BasicUsername/BasicPassword, not a dynamic per-trainee PAM
// check - and gating only the WebSocket layer (a sip.ConnectMiddleware
// alone, which is all sip exposes for that) never gets a chance to produce
// a login prompt at all: a browser's native Basic Auth popup only appears
// for a 401 on a plain HTTP request such as the page load, never for one on
// a WebSocket handshake, and the WebSocket API gives page JS no way to
// attach credentials itself. Challenging the page load here is what makes
// the browser cache credentials it then attaches automatically - at the
// browser's network layer, invisibly to the page's own JS - to the later
// WebSocket upgrade request, which is where pamAuthMiddleware (running
// inside sip's internal instance) establishes the actual session identity.
func newPAMFrontDoor(publicAddr, internalAddr, socketPath string, tlsConfig *tls.Config) *http.Server {
	target := &url.URL{Scheme: "http", Host: internalAddr}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorLog = log.Default()

	gate := func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			unauthorizedHTTP(w)
			return
		}
		if err := pamauth.Verify(socketPath, username, password); err != nil {
			// Same reasoning as pamAuthMiddleware in pamauth.go: log the
			// real reason server-side, tell the browser nothing more than
			// "authentication failed".
			log.Printf("PAM login for %q failed: %v", username, err)
			unauthorizedHTTP(w)
			return
		}
		proxy.ServeHTTP(w, r)
	}

	return &http.Server{
		Addr:      publicAddr,
		Handler:   http.HandlerFunc(gate),
		TLSConfig: tlsConfig,
	}
}

func unauthorizedHTTP(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="tuios-web"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}
