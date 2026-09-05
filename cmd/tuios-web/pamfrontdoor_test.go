package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// TestSettingsRoutesRequirePAM pins a routing gap: Go's ServeMux picks the
// most specific registered pattern for a request, so /tuios-settings/*
// (registered by registerWebSettingsRoutes) never went through the PAM
// check that used to live only inside the "/" handler - a request there was
// served without ever presenting credentials. requirePAM now wraps the
// whole mux instead of being inlined into one route, so every path demands
// credentials before reaching any handler.
func TestSettingsRoutesRequirePAM(t *testing.T) {
	// internalAddr does not need to be reachable: every case here is
	// rejected by requirePAM before the proxy ever runs.
	front := newFrontDoor("127.0.0.1:0", "127.0.0.1:1", nil, "/nonexistent.sock", true)

	paths := []string{
		"/",
		"/tuios-settings/themes",
		"/tuios-settings/theme",
		"/tuios-settings/fonts/saucecodepro.ttf",
		"/tuios-settings/fonts/saucecodepro-semibold.ttf",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			front.Handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("GET %s with no credentials = %d, want %d", path, rec.Code, http.StatusUnauthorized)
			}
			if rec.Header().Get("WWW-Authenticate") == "" {
				t.Errorf("GET %s: missing WWW-Authenticate header on a 401", path)
			}
		})
	}
}

// TestSettingsRoutesReachableWithoutPAM confirms the fix does not
// accidentally start gating tuios-web's other main use case: plain
// --web-settings with no --pam-auth must keep working credential-free.
func TestSettingsRoutesReachableWithoutPAM(t *testing.T) {
	front := newFrontDoor("127.0.0.1:0", "127.0.0.1:1", nil, "", true)

	req := httptest.NewRequest(http.MethodGet, "/tuios-settings/themes", nil)
	rec := httptest.NewRecorder()
	front.Handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Errorf("GET /tuios-settings/themes with pamSocketPath=\"\" was rejected; --web-settings alone must not require PAM")
	}
}

// runCountingFakePAMHelper is like runFakePAMHelperForAuth but tracks how many
// real login attempts it received and only accepts one specific password, so
// a test can tell a genuine pam-helper round trip apart from a cached one.
func runCountingFakePAMHelper(t *testing.T, wantPassword string) (socketPath string, logins *atomic.Int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pam-helper.sock")
	ln, err := net.Listen("unixpacket", path)
	if err != nil {
		t.Fatalf("fake helper listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var count atomic.Int64
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c *net.UnixConn) {
				defer func() { _ = c.Close() }()
				buf := make([]byte, 65541)
				n, _, _, _, err := c.ReadMsgUnix(buf, nil)
				if err != nil || n < 5 {
					return
				}
				count.Add(1)
				_, password := decodeFakeLoginFields(buf[5:n])
				ok := byte(0)
				if password == wantPassword {
					ok = 1
				}
				out := []byte{2, 0, 0, 0, 1, ok} // msgLoginResult = 2
				_, _, _ = c.WriteMsgUnix(out, nil, nil)
			}(conn.(*net.UnixConn))
		}
	}()
	return path, &count
}

// decodeFakeLoginFields reads the two length-prefixed fields (username,
// password) msgLogin's payload carries - see internal/pamauth.Dial's own
// putField calls, which this mirrors just enough to read a real password
// back out.
func decodeFakeLoginFields(payload []byte) (username, password string) {
	if len(payload) < 4 {
		return "", ""
	}
	ulen := int(payload[0])<<24 | int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
	payload = payload[4:]
	if len(payload) < ulen {
		return "", ""
	}
	username = string(payload[:ulen])
	payload = payload[ulen:]
	if len(payload) < 4 {
		return username, ""
	}
	plen := int(payload[0])<<24 | int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
	payload = payload[4:]
	if len(payload) < plen {
		return username, ""
	}
	return username, string(payload[:plen])
}

// TestRequirePAMCachesVerifiedCredentials pins the fix for a real, measurable
// slowdown: a single page load is nine-plus separate HTTP requests (two
// stylesheets, two fonts, three scripts, "/" itself, then the WebSocket
// upgrade - see requirePAM's own doc comment), and requirePAM used to run a
// full PAM login for every one of them even though they all carry the exact
// same Basic Auth header. Repeated requests with the same credentials within
// the cache TTL must hit the real pam-helper only once.
func TestRequirePAMCachesVerifiedCredentials(t *testing.T) {
	socketPath, logins := runCountingFakePAMHelper(t, "correct-password")

	handler := requirePAM(socketPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := range 5 {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetBasicAuth("trainee", "correct-password")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i, rec.Code)
		}
	}

	if got := logins.Load(); got != 1 {
		t.Errorf("real pam-helper logins for 5 requests with the same credentials = %d, want 1 (the rest should have hit the cache)", got)
	}
}

// TestRequirePAMNeverCachesAFailedVerification pins the other half of the
// same cache: a wrong password must never be remembered as good, no matter
// how many times it is retried, and must not poison the cache for the
// correct password either.
func TestRequirePAMNeverCachesAFailedVerification(t *testing.T) {
	socketPath, logins := runCountingFakePAMHelper(t, "correct-password")

	handler := requirePAM(socketPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := range 3 {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetBasicAuth("trainee", "wrong-password")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong-password request %d: status = %d, want 401", i, rec.Code)
		}
	}
	if got := logins.Load(); got != 3 {
		t.Errorf("real pam-helper logins for 3 wrong-password requests = %d, want 3 (a failure must never be cached)", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("trainee", "correct-password")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("correct-password request after failed attempts: status = %d, want 200", rec.Code)
	}
	if got := logins.Load(); got != 4 {
		t.Errorf("real pam-helper logins after the correct password finally arrived = %d, want 4", got)
	}
}
