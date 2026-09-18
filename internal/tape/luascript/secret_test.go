package luascript

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestSecretDisabledByDefault: tape.allow_secrets defaults off, so
// tuios.secret must refuse rather than resolve anything.
func TestSecretDisabledByDefault(t *testing.T) {
	exec := &fakeExecutor{}
	err := runScript(t, `tuios.secret("pass", "any/entry")`, exec, time.Second)
	if err == nil {
		t.Fatal("expected tuios.secret to error when allow_secrets is off")
	}
	if !strings.Contains(err.Error(), "allow_secrets") {
		t.Errorf("error %q does not mention allow_secrets", err)
	}
}

// TestSecretResolvesWhenEnabled covers the happy path with a stub resolver so
// the test does not need a real pass(1) store.
func TestSecretResolvesWhenEnabled(t *testing.T) {
	prev := resolveSecretFn
	t.Cleanup(func() { resolveSecretFn = prev })
	resolveSecretFn = func(source, name string, extras ...string) (string, error) {
		if source != "pass" || name != "cust/passwd" {
			return "", errors.New("unexpected lookup")
		}
		return "s3cret", nil
	}

	exec := &fakeExecutor{}
	script := `
		local s = tuios.secret("pass", "cust/passwd")
		tuios.notify(s, "info")
	`
	if err := runScriptAllowSecrets(t, script, exec, time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	calls := exec.Calls()
	if len(calls) != 1 || calls[0].method != "ShowNotificationCmd" || calls[0].args[0] != "s3cret" {
		t.Fatalf("calls = %+v, want one notify with the resolved secret", calls)
	}
}

// TestSecretRejectsUnknownSource: only the listed backends are supported;
// enabling the gate must not open arbitrary process execution.
func TestSecretRejectsUnknownSource(t *testing.T) {
	exec := &fakeExecutor{}
	err := runScriptAllowSecrets(t, `tuios.secret("vault", "x")`, exec, time.Second)
	if err == nil {
		t.Fatal("expected unknown source to error")
	}
	if !strings.Contains(err.Error(), "unknown secret source") {
		t.Errorf("error %q does not mention unknown source", err)
	}
}

// TestSecretAcceptsGopassSource: gopass is a first-class source name; the
// resolver is stubbed so the test does not need a real gopass binary.
func TestSecretAcceptsGopassSource(t *testing.T) {
	prev := resolveSecretFn
	t.Cleanup(func() { resolveSecretFn = prev })
	resolveSecretFn = func(source, name string, extras ...string) (string, error) {
		if source != "gopass" || name != "cust/passwd" {
			return "", errors.New("unexpected lookup")
		}
		return "gopass-secret", nil
	}

	exec := &fakeExecutor{}
	script := `
		local s = tuios.secret("gopass", "cust/passwd")
		tuios.notify(s, "info")
	`
	if err := runScriptAllowSecrets(t, script, exec, time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	calls := exec.Calls()
	if len(calls) != 1 || calls[0].args[0] != "gopass-secret" {
		t.Fatalf("calls = %+v, want notify with gopass-secret", calls)
	}
}

// TestSecretAcceptsPassageSource stubs passage the same way as pass.
func TestSecretAcceptsPassageSource(t *testing.T) {
	prev := resolveSecretFn
	t.Cleanup(func() { resolveSecretFn = prev })
	resolveSecretFn = func(source, name string, extras ...string) (string, error) {
		if source != "passage" || name != "cust/passwd" {
			return "", errors.New("unexpected lookup")
		}
		return "passage-secret", nil
	}

	exec := &fakeExecutor{}
	if err := runScriptAllowSecrets(t, `
		tuios.notify(tuios.secret("passage", "cust/passwd"), "info")
	`, exec, time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if got := exec.Calls()[0].args[0]; got != "passage-secret" {
		t.Fatalf("got %q, want passage-secret", got)
	}
}

// TestSecretKeepassxcPassesDatabaseExtra: the optional 3rd argument is the
// .kdbx path for keepassxc.
func TestSecretKeepassxcPassesDatabaseExtra(t *testing.T) {
	prev := resolveSecretFn
	t.Cleanup(func() { resolveSecretFn = prev })
	resolveSecretFn = func(source, name string, extras ...string) (string, error) {
		if source != "keepassxc" || name != "Web/GitHub" {
			return "", errors.New("unexpected lookup")
		}
		if len(extras) != 1 || extras[0] != "/home/me/passwords.kdbx" {
			return "", errors.New("missing database path")
		}
		return "kp-secret", nil
	}

	exec := &fakeExecutor{}
	if err := runScriptAllowSecrets(t, `
		tuios.notify(tuios.secret("keepassxc", "Web/GitHub", "/home/me/passwords.kdbx"), "info")
	`, exec, time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if got := exec.Calls()[0].args[0]; got != "kp-secret" {
		t.Fatalf("got %q, want kp-secret", got)
	}
}

// TestResolveKeepassxcRequiresDatabase: without a path or env, keepassxc
// fails with a clear error rather than invoking the CLI.
func TestResolveKeepassxcRequiresDatabase(t *testing.T) {
	t.Setenv("TUIOS_KEEPASSXC_DATABASE", "")
	_, err := resolveKeepassxc("", "entry")
	if err == nil || !strings.Contains(err.Error(), "database path required") {
		t.Fatalf("error = %v, want database path required", err)
	}
}
