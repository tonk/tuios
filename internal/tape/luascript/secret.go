package luascript

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// resolveSecretFn is the host-side lookup behind tuios.secret(). Tests replace
// it to avoid needing real password-manager binaries.
var resolveSecretFn = resolveSecret

// resolveSecret looks up a named secret from a supported source. It is the
// host-side implementation behind tuios.secret(); the Lua binding refuses to
// call it unless tape.allow_secrets is enabled.
//
// source selects the backend:
//   - "pass"      — pass(1) (https://www.passwordstore.org/)
//   - "gopass"    — gopass (https://www.gopass.pw/), via `gopass show -o`
//   - "passage"   — passage (https://github.com/FiloSottile/passage), age-based pass fork
//   - "keepassxc" — KeePassXC CLI (keepassxc-cli); needs a database path as
//     extras[0] or $TUIOS_KEEPASSXC_DATABASE
//
// Deliberately not an arbitrary shell, so enabling allow_secrets does not
// hand Lua general process execution.
//
// extras carries source-specific optional arguments. For keepassxc, extras[0]
// is the .kdbx path when not taken from the environment.
func resolveSecret(source, name string, extras ...string) (string, error) {
	switch source {
	case "pass":
		return resolvePassShow("pass", name)
	case "gopass":
		// -o / --password prints only the password line, which is what
		// scripts need; plain `gopass show` may print "Password: …" or
		// multi-field MIME content.
		return resolvePassShow("gopass", name, "show", "-o", name)
	case "passage":
		return resolvePassShow("passage", name)
	case "keepassxc":
		db := ""
		if len(extras) > 0 {
			db = extras[0]
		}
		return resolveKeepassxc(db, name)
	default:
		return "", fmt.Errorf("unknown secret source %q (supported: pass, gopass, passage, keepassxc)", source)
	}
}

// resolvePassShow runs a password-store style show command. For "pass" /
// "passage" the args default to `<bin> show <name>`; for gopass the caller
// supplies the full argv after the binary (show -o <name>).
func resolvePassShow(bin, name string, args ...string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%s entry name is empty", bin)
	}
	if len(args) == 0 {
		args = []string{"show", name}
	}
	return runSecretCmd(bin, args, nil)
}

// resolveKeepassxc runs `keepassxc-cli show` for the Password attribute of
// entry in database. database may be empty to fall back to
// TUIOS_KEEPASSXC_DATABASE.
//
// Unlock credentials (optional, via environment):
//   - TUIOS_KEEPASSXC_PASSWORD — database password, fed on stdin
//   - TUIOS_KEEPASSXC_KEYFILE  — key-file path (-k)
// If only a keyfile is set, --no-password is passed (keyfile-only databases).
func resolveKeepassxc(database, entry string) (string, error) {
	if database == "" {
		database = os.Getenv("TUIOS_KEEPASSXC_DATABASE")
	}
	if database == "" {
		return "", fmt.Errorf("keepassxc: database path required (3rd argument to tuios.secret or TUIOS_KEEPASSXC_DATABASE)")
	}
	if entry == "" {
		return "", fmt.Errorf("keepassxc: entry name is empty")
	}

	args := []string{"show", "-q", "-a", "Password", "-s"}
	keyfile := os.Getenv("TUIOS_KEEPASSXC_KEYFILE")
	password := os.Getenv("TUIOS_KEEPASSXC_PASSWORD")
	if keyfile != "" {
		args = append(args, "-k", keyfile)
		if password == "" {
			args = append(args, "--no-password")
		}
	}
	args = append(args, database, entry)

	var stdin *strings.Reader
	if password != "" {
		stdin = strings.NewReader(password + "\n")
	}
	return runSecretCmd("keepassxc-cli", args, stdin)
}

// runSecretCmd executes bin with args, optionally feeding stdin, and returns
// stdout with a single trailing newline stripped.
func runSecretCmd(bin string, args []string, stdin *strings.Reader) (string, error) {
	cmd := exec.Command(bin, args...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("%s %s: %w: %s", bin, strings.Join(args, " "), err, bytes.TrimSpace(ee.Stderr))
		}
		return "", fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
	}
	secret := string(out)
	secret = strings.TrimSuffix(secret, "\n")
	secret = strings.TrimSuffix(secret, "\r")
	if secret == "" {
		return "", fmt.Errorf("%s %s: empty secret", bin, strings.Join(args, " "))
	}
	return secret, nil
}
