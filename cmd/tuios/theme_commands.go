package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/theme"
)

// printExampleTheme writes the fully commented reference theme (see
// theme.GenerateExampleTheme) to stdout, or to example.toml in the themes
// directory when write is true. It never touches an existing theme file.
func printExampleTheme(write bool) error {
	content := theme.GenerateExampleTheme()

	if !write {
		fmt.Print(content)
		return nil
	}

	themesDir, err := theme.GetThemesDir()
	if err != nil {
		return fmt.Errorf("could not determine themes directory: %w", err)
	}
	// .example, not .toml: LoadCustomThemes only scans *.json and *.toml, so
	// this sits in the themes directory for easy reference without tuios
	// ever loading it as a real (blank) theme on its own.
	examplePath := filepath.Join(themesDir, "example.toml.example")

	if _, err := os.Stat(examplePath); err == nil {
		fmt.Printf("Warning: this will overwrite the existing file at:\n  %s\n\n", examplePath)
		fmt.Printf("Overwrite? (yes/no): ")

		var response string
		_, _ = fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))

		if response != "yes" && response != "y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	if err := os.WriteFile(examplePath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write example theme: %w", err)
	}

	fmt.Printf("Wrote annotated example theme to:\n  %s\n\n", examplePath)
	fmt.Println("It's named .example so tuios never loads it as-is. Copy it to a .toml file")
	fmt.Println("in the same directory, give it a real id, uncomment and edit the colors you want,")
	if hint := reloadThemeChordHint(); hint != "" {
		fmt.Printf("then select it with `theme = \"<id>\"` in config.toml (or reload it live with\n%s once you've edited it).\n", hint)
	} else {
		fmt.Println("then select it with `theme = \"<id>\"` in config.toml.")
	}
	return nil
}

// reloadThemeChordHint spells out the actual key chord for
// debug_prefix_reload_theme, from the user's own config rather than a
// hardcoded default - this CLI command has no reason to assume the leader is
// ctrl+b or that the debug submenu is still bound to "D". Returns "" if
// either the debug submenu or the reload action itself is unbound, rather
// than print a hint that presses nothing.
func reloadThemeChordHint() string {
	userConfig, err := config.LoadUserConfig()
	if err != nil {
		userConfig = config.DefaultConfig()
	}
	registry := config.NewKeybindRegistry(userConfig)

	leader := userConfig.Keybindings.LeaderKey
	if leader == "" {
		leader = "ctrl+b"
	}
	debugKeys := registry.GetKeys("prefix_debug")
	reloadKeys := registry.GetKeys("debug_prefix_reload_theme")
	if len(debugKeys) == 0 || len(reloadKeys) == 0 {
		return ""
	}
	return leader + ", " + debugKeys[0] + ", " + reloadKeys[0]
}
