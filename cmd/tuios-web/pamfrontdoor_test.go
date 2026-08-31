package main

import (
	"net/http"
	"net/http/httptest"
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
