package config_test

import (
	"os"
	"os/user"
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/config"
)

// TestFormatInitialTitle covers appearance.initial_title_format: the
// template used for a new window's title at creation, distinct from
// window_title_format's continuous display-time reformatting of an existing
// one.
func TestFormatInitialTitle(t *testing.T) {
	prev := config.InitialTitleFormat
	t.Cleanup(func() { config.InitialTitleFormat = prev })

	t.Run("empty format means no override", func(t *testing.T) {
		config.InitialTitleFormat = ""
		if got := config.FormatInitialTitle(); got != "" {
			t.Errorf("FormatInitialTitle() = %q, want \"\" (caller keeps its own default)", got)
		}
	})

	t.Run("literal text without placeholders is used verbatim", func(t *testing.T) {
		config.InitialTitleFormat = "trainee shell"
		if got := config.FormatInitialTitle(); got != "trainee shell" {
			t.Errorf("FormatInitialTitle() = %q, want %q", got, "trainee shell")
		}
	})

	t.Run("{user} expands to the current OS username", func(t *testing.T) {
		want := currentUsernameForTest(t)
		config.InitialTitleFormat = "{user}'s shell"
		if got := config.FormatInitialTitle(); got != want+"'s shell" {
			t.Errorf("FormatInitialTitle() = %q, want %q", got, want+"'s shell")
		}
	})
}

// TestInitialTitleFormatIsAppliedFromConfig pins the wiring rather than the
// formatting, the same way TestWindowTitleFormatIsAppliedFromConfig does for
// window_title_format: the option is only honest if loading a config
// actually reaches the global FormatInitialTitle reads.
func TestInitialTitleFormatIsAppliedFromConfig(t *testing.T) {
	prevFormat, prevLock := config.InitialTitleFormat, config.LockTitles
	t.Cleanup(func() {
		config.InitialTitleFormat = prevFormat
		config.LockTitles = prevLock
	})

	cfg := config.DefaultConfig()
	cfg.Appearance.InitialTitleFormat = "{user}"
	cfg.Appearance.LockTitles = true
	config.ApplyAppearanceConfig(cfg)

	if config.InitialTitleFormat != "{user}" {
		t.Fatalf("InitialTitleFormat = %q, want the configured format", config.InitialTitleFormat)
	}
	if !config.LockTitles {
		t.Fatalf("LockTitles = false, want true")
	}

	cfg.Appearance.InitialTitleFormat = ""
	cfg.Appearance.LockTitles = false
	config.ApplyAppearanceConfig(cfg)
	if config.InitialTitleFormat != "" {
		t.Errorf("InitialTitleFormat = %q, want it cleared", config.InitialTitleFormat)
	}
	if config.LockTitles {
		t.Errorf("LockTitles = true, want it cleared")
	}
}

// currentUsernameForTest mirrors the resolution FormatInitialTitle's {user}
// placeholder uses internally, so the test does not hardcode a username that
// would only be right on one machine.
func currentUsernameForTest(t *testing.T) string {
	t.Helper()
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("LOGNAME"); u != "" {
		return u
	}
	t.Skip("no way to resolve the current username in this environment")
	return ""
}
