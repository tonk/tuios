package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/theme"
)

func printConfigPath() error {
	path, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("could not determine config path: %w", err)
	}
	fmt.Println(path)
	return nil
}

func editConfigFile() error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("could not determine config path: %w", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("Config file doesn't exist, creating default at: %s\n", configPath)
		_, err := config.LoadUserConfig()
		if err != nil {
			return fmt.Errorf("could not create config file: %w", err)
		}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		for _, e := range []string{"vim", "vi", "nano", "emacs"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	if editor == "" {
		return fmt.Errorf("no editor found. Please set $EDITOR environment variable")
	}

	cmd := exec.Command(editor, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}
	return nil
}

func resetConfigToDefaults() error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("could not determine config path: %w", err)
	}

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Warning: This will overwrite your existing configuration at:\n")
		fmt.Printf("  %s\n\n", configPath)
		fmt.Printf("Are you sure you want to reset to defaults? (yes/no): ")

		var response string
		_, _ = fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))

		if response != "yes" && response != "y" {
			fmt.Println("Reset cancelled.")
			return nil
		}
	}

	defaultCfg := config.DefaultConfig()

	var sb strings.Builder
	sb.WriteString("# TUIOS Configuration File\n")
	sb.WriteString("# This file allows you to customize keybindings\n")
	sb.WriteString("# Edit keybindings by modifying the arrays of keys for each action\n")
	sb.WriteString("# Multiple keys can be bound to the same action\n")
	sb.WriteString("#\n")
	sb.WriteString("# Configuration location: " + configPath + "\n")
	sb.WriteString("# Documentation: https://github.com/tonk/tuios\n\n")

	data, err := toml.Marshal(defaultCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if _, err := sb.Write(data); err != nil {
		return fmt.Errorf("failed to write config data: %w", err)
	}

	if err := os.WriteFile(configPath, []byte(sb.String()), 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Configuration reset to defaults\n")
	fmt.Printf("  Location: %s\n", configPath)
	fmt.Println("\nYou can customize it with: tuios config edit")
	return nil
}

// printExampleConfig writes the fully commented reference config (see
// config.GenerateExampleConfig) to stdout, or to a ".example" file beside the
// real config when write is true. It never touches the real config.toml.
func printExampleConfig(write bool) error {
	content := config.GenerateExampleConfig()

	if !write {
		fmt.Print(content)
		return nil
	}

	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("could not determine config path: %w", err)
	}
	examplePath := configPath + ".example"

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
		return fmt.Errorf("failed to write example config: %w", err)
	}

	fmt.Printf("Wrote annotated reference config to:\n  %s\n\n", examplePath)
	fmt.Println("Copy the settings you want into your real config with: tuios config edit")
	return nil
}

func previewThemeColors(themeName string) error {
	if err := theme.Initialize(themeName); err != nil {
		return fmt.Errorf("failed to initialize theme: %w", err)
	}

	currentTheme := theme.Current()
	if currentTheme == nil {
		return fmt.Errorf("theme '%s' not found", themeName)
	}

	fmt.Printf("Theme: %s\n\n", themeName)

	palette := theme.GetANSIPalette()

	colorNames := []string{
		"Black", "Red", "Green", "Yellow",
		"Blue", "Magenta", "Cyan", "White",
		"Bright Black", "Bright Red", "Bright Green", "Bright Yellow",
		"Bright Blue", "Bright Magenta", "Bright Cyan", "Bright White",
	}

	fmt.Println("Normal Colors (0-7):")
	for i := range 8 {
		c := palette[i]
		r, g, b, _ := c.RGBA()
		r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)
		fmt.Printf("  \033[48;2;%d;%d;%dm    \033[0m  %-14s #%02x%02x%02x\n", r8, g8, b8, colorNames[i], r8, g8, b8)
	}

	fmt.Println()

	fmt.Println("Bright Colors (8-15):")
	for i := 8; i < 16; i++ {
		c := palette[i]
		r, g, b, _ := c.RGBA()
		r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)
		fmt.Printf("  \033[48;2;%d;%d;%dm    \033[0m  %-14s #%02x%02x%02x\n", r8, g8, b8, colorNames[i], r8, g8, b8)
	}

	return nil
}

// configFileArg returns the file named on the command line, or the active
// config file when none is.
func configFileArg(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	path, err := config.GetConfigPath()
	if err != nil {
		return "", fmt.Errorf("could not determine config path: %w", err)
	}
	return path, nil
}

// checkConfigFile prints every problem config.CheckConfig finds, one per line
// in the compiler-style "file:line: severity: message" form editors can jump
// to, and fails when there is any.
func checkConfigFile(args []string) error {
	path, err := configFileArg(args)
	if err != nil {
		return err
	}
	// #nosec G304 - reading the config file the user pointed at is the point
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	issues := config.CheckConfig(data)
	if len(issues) == 0 {
		fmt.Printf("%s: no problems found\n", path)
		return nil
	}

	errorCount := 0
	for _, issue := range issues {
		location := path
		if issue.Line > 0 {
			location = fmt.Sprintf("%s:%d", path, issue.Line)
		}
		if issue.Severity == config.IssueError {
			errorCount++
		}
		fmt.Printf("%s: %s: %s: %s\n", location, issue.Severity, issue.Kind, issue.Message)
	}
	return fmt.Errorf("found %d problem(s), %d of which stop tuios from loading the file", len(issues), errorCount)
}

// reformatConfigFile sorts and aligns the config file in place, keeping the
// original as <file>.bak, or prints the result when toStdout is set.
func reformatConfigFile(args []string, toStdout bool) error {
	path, err := configFileArg(args)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	// #nosec G304 - reading the config file the user pointed at is the point
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	formatted, err := config.ReformatConfig(data)
	if err != nil {
		return err
	}
	if toStdout {
		_, err := os.Stdout.Write(formatted)
		return err
	}
	if string(formatted) == string(data) {
		fmt.Printf("%s is already formatted\n", path)
		return nil
	}

	backup := path + ".bak"
	if err := os.WriteFile(backup, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("failed to write backup: %w", err)
	}
	if err := os.WriteFile(path, formatted, info.Mode().Perm()); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	fmt.Printf("Reformatted %s\n  Previous version kept as %s\n", path, backup)
	return nil
}

// cleanConfigFile removes the settings that change nothing from the config
// file, keeping the original as <file>.bak, or prints the result when
// toStdout is set. With reformat it also sorts and aligns what is left, in the
// same write, so the backup is still the original file. What was removed is
// reported on stderr when printing to stdout.
func cleanConfigFile(args []string, toStdout, reformat bool) error {
	path, err := configFileArg(args)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	// #nosec G304 - reading the config file the user pointed at is the point
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	cleaned, removed, err := config.CleanConfig(data)
	if err != nil {
		return err
	}
	if reformat {
		if cleaned, err = config.ReformatConfig(cleaned); err != nil {
			return err
		}
	}
	if toStdout {
		if _, err := os.Stdout.Write(cleaned); err != nil {
			return err
		}
		reportCleaned(os.Stderr, path, removed)
		return nil
	}
	if len(removed) == 0 && string(cleaned) == string(data) {
		fmt.Printf("%s has no default or unused settings\n", path)
		return nil
	}

	backup := path + ".bak"
	if err := os.WriteFile(backup, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("failed to write backup: %w", err)
	}
	if err := os.WriteFile(path, cleaned, info.Mode().Perm()); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	reportCleaned(os.Stdout, path, removed)
	fmt.Printf("  Previous version kept as %s\n", backup)
	return nil
}

// reportCleaned summarises what CleanConfig removed: the settings at their
// default as a count, and each one that had no effect by name, since those
// are the ones worth knowing about.
func reportCleaned(w io.Writer, path string, removed []config.CleanedSetting) {
	defaults := 0
	var noEffect []config.CleanedSetting
	for _, r := range removed {
		if r.Reason == config.CleanDefault {
			defaults++
		} else {
			noEffect = append(noEffect, r)
		}
	}
	_, _ = fmt.Fprintf(w, "Cleaned %s: removed %d setting(s) at their default and %d with no effect\n",
		path, defaults, len(noEffect))
	for _, r := range noEffect {
		_, _ = fmt.Fprintf(w, "  line %d: %s (no effect)\n", r.Line, r.Key)
	}
}
