package session

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/adrg/xdg"
)

// withConfigTOML points XDG at a throwaway tree holding src as config.toml,
// so getShell's config.LoadUserConfig() call reads it instead of the
// developer's own ~/.config/tuios/config.toml. XDG's search paths are
// resolved once at package init, so the reload is what makes the redirect
// take (see internal/config's own writeConfig test helper, which this
// mirrors).
func withConfigTOML(t *testing.T, src string) {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(xdg.Reload)
	t.Setenv("XDG_CONFIG_HOME", dir)
	xdg.Reload()
	if err := os.MkdirAll(filepath.Join(dir, "tuios"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tuios", "config.toml"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGetShellFallsBackToConfiguredPreferredShell is the regression test for
// a daemon session's new windows silently ignoring appearance.preferred_shell:
// AttachPayload/HelloPayload carry no shell preference, so SessionConfig.Shell
// is always empty for a daemon session, and without this fallback every
// window fell through straight to the daemon process's own $SHELL.
func TestGetShellFallsBackToConfiguredPreferredShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a Unix shell path")
	}

	dir := t.TempDir()
	shellPath := filepath.Join(dir, "fake-shell")
	if err := os.WriteFile(shellPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	withConfigTOML(t, "[appearance]\npreferred_shell = '"+shellPath+"'\n")

	sess, err := NewSession("shell-fallback", &SessionConfig{}, 80, 24)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if got := sess.getShell(); got != shellPath {
		t.Errorf("getShell() = %q, want the configured preferred_shell %q", got, shellPath)
	}
}

// TestGetShellPrefersExplicitSessionConfigOverPreferredShell covers the other
// half: an explicit SessionConfig.Shell (set on the rare paths that do carry
// one) must still win over config.toml, matching the precedence documented
// on getShell.
func TestGetShellPrefersExplicitSessionConfigOverPreferredShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a Unix shell path")
	}

	dir := t.TempDir()
	configuredShell := filepath.Join(dir, "from-config")
	explicitShell := filepath.Join(dir, "from-session-config")
	for _, p := range []string{configuredShell, explicitShell} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	withConfigTOML(t, "[appearance]\npreferred_shell = '"+configuredShell+"'\n")

	sess, err := NewSession("shell-precedence", &SessionConfig{Shell: explicitShell}, 80, 24)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if got := sess.getShell(); got != explicitShell {
		t.Errorf("getShell() = %q, want the explicit SessionConfig.Shell %q", got, explicitShell)
	}
}

// TestGetShellIgnoresPreferredShellThatDoesNotExist mirrors
// terminal.detectShell's own existence check: a stale or platform-mismatched
// preferred_shell must not be handed to exec.Command as-is.
func TestGetShellIgnoresPreferredShellThatDoesNotExist(t *testing.T) {
	withConfigTOML(t, "[appearance]\npreferred_shell = '/definitely/not/a/real/shell'\n")

	sess, err := NewSession("shell-missing", &SessionConfig{}, 80, 24)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if got := sess.getShell(); got == "/definitely/not/a/real/shell" {
		t.Error("getShell() returned a preferred_shell that does not exist on disk")
	}
}
