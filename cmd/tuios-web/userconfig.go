package main

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/adrg/xdg"

	"github.com/tonk/tuios/internal/config"
)

// webUserConfigDir is where a PAM-authenticated user's own settings are saved
// (<dir>/<username>.toml). Empty means the default, see userConfigDir.
var webUserConfigDir string

// webOverrides are the CLI flags the server was started with, kept so the
// appearance globals can be put back under them every time a user's own
// settings are applied (see applyUserAppearance).
var webOverrides config.Overrides

// maxUserConfigSize bounds what is read from a user-controlled file.
const maxUserConfigSize = 1 << 20

// validWebUsername is what a username must look like before it is used in a
// file name. PAM hands back whatever the account is called, and this value ends
// up in a path, so anything with a separator or a leading dot is refused.
var validWebUsername = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.@-]{0,63}$`)

// userConfigDir returns the directory holding the per-user settings files:
// --user-config-dir, else systemd's STATE_DIRECTORY (the service's own state
// directory), else the XDG state home of the account tuios-web runs as.
func userConfigDir() string {
	if webUserConfigDir != "" {
		return webUserConfigDir
	}
	if dir := os.Getenv("STATE_DIRECTORY"); dir != "" {
		first, _, _ := strings.Cut(dir, ":")
		return filepath.Join(first, "users")
	}
	return filepath.Join(xdg.StateHome, "tuios-web", "users")
}

// userSettingsPath is where owner's saved settings live.
func userSettingsPath(owner string) (string, error) {
	if !validWebUsername.MatchString(owner) {
		return "", fmt.Errorf("refusing to use %q as a settings file name", owner)
	}
	return filepath.Join(userConfigDir(), owner+".toml"), nil
}

// userHomeConfigPath is the config file in owner's home directory.
func userHomeConfigPath(owner string) (string, error) {
	u, err := user.Lookup(owner)
	if err != nil {
		return "", err
	}
	if u.HomeDir == "" {
		return "", fmt.Errorf("no home directory for %q", owner)
	}
	return filepath.Join(u.HomeDir, ".config", "tuios", "config.toml"), nil
}

// readUserFile reads a user-controlled config file: a regular file (not a
// symlink to something else tuios-web happens to be able to read), of sane
// size. A missing or unreadable file is not an error worth reporting, since it
// is the ordinary case; ok is false and the caller moves on.
func readUserFile(path string) (data []byte, ok bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxUserConfigSize {
		return nil, false
	}
	data, err = os.ReadFile(path) // #nosec G304 - path is built from a validated username
	if err != nil {
		return nil, false
	}
	return data, true
}

// loadWebSessionConfig returns the config for one web connection and how its
// settings are saved.
//
// owner is the PAM-authenticated username, or empty for a connection with no
// identity. With no identity there is nobody whose home directory to read and
// nobody the settings belong to, so the server's config is used as it is and
// saving is a no-op: one visitor must not change what every other visitor gets.
//
// With an owner, the server's config is the base, then the settings from
// ~/.config/tuios/config.toml when the server can read it, then the settings
// the user last saved from the web settings page. Only [appearance],
// [notifications] and [keybindings] are taken from those files (see
// config.OverlayUserSections): the rest of a config runs as, or decides things
// for, the server's own account.
func loadWebSessionConfig(owner string) (*config.UserConfig, func(*config.UserConfig) error, error) {
	cfg, err := loadWebUserConfig()
	if err != nil {
		return nil, nil, err
	}
	if owner == "" {
		return cfg, func(*config.UserConfig) error { return nil }, nil
	}

	settingsPath, err := userSettingsPath(owner)
	if err != nil {
		// Without a usable name there is nowhere to save either.
		log.Printf("Warning: %v; this session's settings will not be saved", err)
		return cfg, func(*config.UserConfig) error { return nil }, nil
	}

	if homePath, err := userHomeConfigPath(owner); err == nil {
		if data, ok := readUserFile(homePath); ok {
			if err := config.OverlayUserSections(cfg, data); err != nil {
				log.Printf("Warning: ignoring %s: %v", homePath, err)
			}
		}
	}
	if data, ok := readUserFile(settingsPath); ok {
		if err := config.OverlayUserSections(cfg, data); err != nil {
			log.Printf("Warning: ignoring %s: %v", settingsPath, err)
		}
	}

	save := func(c *config.UserConfig) error {
		return config.WriteConfigFile(c, settingsPath)
	}
	return cfg, save, nil
}

// applyUserAppearance makes cfg's appearance the one the render loop reads,
// under the CLI flags the server was started with. The appearance settings are
// process-wide globals, so this is what lets a user's own settings take effect
// for the session being created; it also means the last session to connect
// decides what the globals hold until the next one does.
func applyUserAppearance(cfg *config.UserConfig) {
	config.ApplyAppearanceConfig(cfg)
	config.ApplyOverrides(webOverrides, cfg)
}

// webSessionConfigOrDefault is loadWebSessionConfig for a connection that
// should still come up when the config cannot be loaded: it falls back to the
// built-in defaults, with saving off. When owner has settings of their own they
// are also made the appearance the render loop reads.
func webSessionConfigOrDefault(owner string) (*config.UserConfig, func(*config.UserConfig) error) {
	cfg, save, err := loadWebSessionConfig(owner)
	if err != nil {
		log.Printf("Warning: Failed to load config for web session, using defaults: %v", err)
		return config.DefaultConfig(), func(*config.UserConfig) error { return nil }
	}
	if owner != "" {
		applyUserAppearance(cfg)
	}
	return cfg, save
}
