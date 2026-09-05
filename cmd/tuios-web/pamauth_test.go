package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Gaurav-Gosain/sip"
	"github.com/tonk/tuios/internal/config"
)

// runFakePAMHelperForAuth is a minimal stand-in for tuios-pam-helper that
// only handles the login handshake (msgLogin/msgLoginResult) - enough to
// drive pamauth.Dial for real without needing PTY spawning, which these
// tests never reach. See internal/pamauth's own doc comments for the wire
// format.
func runFakePAMHelperForAuth(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pam-helper.sock")
	ln, err := net.Listen("unixpacket", path)
	if err != nil {
		t.Fatalf("fake helper listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

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
				// msgLogin = 1; always succeed. Response: msgLoginResult = 2,
				// payload [1] (ok).
				out := []byte{2, 0, 0, 0, 1, 1}
				_, _, _ = c.WriteMsgUnix(out, nil, nil)
			}(conn.(*net.UnixConn))
		}
	}()
	return path
}

// fakeNextCalled records whether the wrapped handler ran, and with what
// context, so tests can assert on classroomAttachTargetFromContext without
// needing a real sip.Session.
func fakeNext(called *bool, gotTarget *string, gotOK *bool) sip.ConnectHandler {
	return func(r *http.Request) error {
		*called = true
		target, ok := classroomAttachTargetFromContext(r.Context())
		*gotTarget = target
		*gotOK = ok
		return nil
	}
}

func classroomTestConfig() config.ClassroomConfig {
	return config.ClassroomConfig{
		TrainerConsole: true,
		TrainerUsers:   []string{"ton"},
		TraineePattern: "^guru[0-9]{2}$",
	}
}

func TestPAMAuthMiddlewareOwnSession(t *testing.T) {
	socketPath := runFakePAMHelperForAuth(t)
	var called bool
	var target string
	var ok bool
	mw := pamAuthMiddleware(socketPath, classroomTestConfig())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("guru07", "irrelevant")
	if err := mw(fakeNext(&called, &target, &ok))(req); err != nil {
		t.Fatalf("middleware returned an error: %v", err)
	}
	if !called {
		t.Fatal("next handler was not called for a valid login with no attach param")
	}
	if ok {
		t.Errorf("classroom attach target = %q, want none for an ordinary session", target)
	}
}

func TestPAMAuthMiddlewareAttachSelfIsNotClassroomAttach(t *testing.T) {
	socketPath := runFakePAMHelperForAuth(t)
	var called bool
	var target string
	var ok bool
	mw := pamAuthMiddleware(socketPath, classroomTestConfig())

	req := httptest.NewRequest(http.MethodGet, "/?attach=guru07", nil)
	req.SetBasicAuth("guru07", "irrelevant")
	if err := mw(fakeNext(&called, &target, &ok))(req); err != nil {
		t.Fatalf("middleware returned an error: %v", err)
	}
	if !called {
		t.Fatal("next handler was not called")
	}
	if ok {
		t.Errorf("attach=<own username> should not set a classroom attach target, got %q", target)
	}
}

func TestPAMAuthMiddlewareDeniesNonTrainer(t *testing.T) {
	socketPath := runFakePAMHelperForAuth(t)
	var called bool
	var target string
	var ok bool
	mw := pamAuthMiddleware(socketPath, classroomTestConfig())

	// guru08 is a valid trainee but not in TrainerUsers.
	req := httptest.NewRequest(http.MethodGet, "/?attach=guru07", nil)
	req.SetBasicAuth("guru08", "irrelevant")
	err := mw(fakeNext(&called, &target, &ok))(req)
	if err == nil {
		t.Fatal("expected an error denying a non-trainer's attach request")
	}
	var connErr *sip.ConnectError
	if !asConnectError(err, &connErr) || connErr.Status != http.StatusUnauthorized {
		t.Errorf("error = %v, want a 401 sip.ConnectError", err)
	}
	if called {
		t.Fatal("next handler ran despite an unauthorized attach request")
	}
}

func TestPAMAuthMiddlewareDeniesNonMatchingTrainee(t *testing.T) {
	socketPath := runFakePAMHelperForAuth(t)
	var called bool
	var target string
	var ok bool
	mw := pamAuthMiddleware(socketPath, classroomTestConfig())

	// ton is an authorized trainer, but "root" does not match trainee_pattern.
	req := httptest.NewRequest(http.MethodGet, "/?attach=root", nil)
	req.SetBasicAuth("ton", "irrelevant")
	err := mw(fakeNext(&called, &target, &ok))(req)
	if err == nil {
		t.Fatal("expected an error denying an attach target outside trainee_pattern")
	}
	if called {
		t.Fatal("next handler ran despite a non-matching attach target")
	}
}

func TestPAMAuthMiddlewareAllowsAuthorizedTrainer(t *testing.T) {
	socketPath := runFakePAMHelperForAuth(t)
	var called bool
	var target string
	var ok bool
	mw := pamAuthMiddleware(socketPath, classroomTestConfig())

	req := httptest.NewRequest(http.MethodGet, "/?attach=guru07", nil)
	req.SetBasicAuth("ton", "irrelevant")
	if err := mw(fakeNext(&called, &target, &ok))(req); err != nil {
		t.Fatalf("middleware returned an error for an authorized trainer: %v", err)
	}
	if !called {
		t.Fatal("next handler was not called for an authorized trainer")
	}
	if !ok || target != "guru07" {
		t.Errorf("classroom attach target = %q, %v; want \"guru07\", true", target, ok)
	}
}

func TestPAMAuthMiddlewareDeniesTrainerWhenConsoleOff(t *testing.T) {
	socketPath := runFakePAMHelperForAuth(t)
	var called bool
	var target string
	var ok bool
	cfg := classroomTestConfig()
	cfg.TrainerConsole = false
	mw := pamAuthMiddleware(socketPath, cfg)

	req := httptest.NewRequest(http.MethodGet, "/?attach=guru07", nil)
	req.SetBasicAuth("ton", "irrelevant")
	err := mw(fakeNext(&called, &target, &ok))(req)
	if err == nil {
		t.Fatal("expected an error: trainer_console is off, so no one may cross-attach")
	}
	if called {
		t.Fatal("next handler ran despite trainer_console being off")
	}
}

// asConnectError is a tiny type-assertion wrapper kept local to this file so
// the tests above read as plain assertions.
func asConnectError(err error, target **sip.ConnectError) bool {
	ce, ok := err.(*sip.ConnectError)
	if ok {
		*target = ce
	}
	return ok
}
