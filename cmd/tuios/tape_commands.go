package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/tonk/tuios/internal/app"
	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/input"
	"github.com/tonk/tuios/internal/session"
	"github.com/tonk/tuios/internal/tape"
	"github.com/tonk/tuios/internal/theme"
	lua "github.com/yuin/gopher-lua"
)

// isLuaTape reports whether path names a .lua tape script rather than the
// .tape DSL.
func isLuaTape(path string) bool {
	return strings.HasSuffix(path, ".lua")
}

func runTapeInteractive(tapeFile string) error {
	content, err := os.ReadFile(tapeFile)
	if err != nil {
		return fmt.Errorf("failed to read tape file: %w", err)
	}

	if isLuaTape(tapeFile) {
		return runLuaTapeInteractive(tapeFile, string(content))
	}

	commands, parseErrors := tape.ParseFile(string(content))
	if len(parseErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Tape parsing errors:\n")
		for _, err := range parseErrors {
			fmt.Fprintf(os.Stderr, "  %s\n", err)
		}
		return fmt.Errorf("failed to parse tape file")
	}

	fmt.Printf("Preparing tape script: %s\n", tapeFile)
	fmt.Printf("Total commands: %d\n", len(commands))
	fmt.Println("Press Ctrl+C to cancel, Ctrl+P to pause/resume playback")
	fmt.Println("\nStarting TUIOS with tape playback...")

	initialOS := newTapeInteractiveOS()

	player := tape.NewPlayer(commands)
	initialOS.ScriptMode = true
	initialOS.ScriptPlayer = player
	initialOS.ScriptPaused = false
	initialOS.ScriptExecutor = tape.NewCommandExecutor(initialOS)

	return runTapeProgram(initialOS)
}

// runLuaTapeInteractive is runTapeInteractive's counterpart for .lua tape
// scripts: same model setup and program lifecycle, but the script drives its
// own control flow instead of a fixed Player command list (see
// app.StartLuaPlayback).
func runLuaTapeInteractive(tapeFile, content string) error {
	fmt.Printf("Preparing Lua tape script: %s\n", tapeFile)
	fmt.Println("Press Ctrl+C to cancel, Ctrl+P or Esc to stop the script")
	fmt.Println("\nStarting TUIOS with tape playback...")

	initialOS := newTapeInteractiveOS()
	dir, err := filepath.Abs(filepath.Dir(tapeFile))
	if err != nil {
		dir = filepath.Dir(tapeFile)
	}
	initialOS.StartLuaPlayback(content, strings.TrimSuffix(filepath.Base(tapeFile), ".lua"), dir)

	return runTapeProgram(initialOS)
}

// newTapeInteractiveOS builds the app.OS model shared by every tape playback
// entry point (DSL or Lua): config/theme loading, the input handler and a
// freshly seeded, zero-window session. Animations are forced off for
// deterministic playback, matching recorded tapes (the recorder prepends
// DisableAnimations); a tape can still re-enable them explicitly.
func newTapeInteractiveOS() *app.OS {
	userConfig, err := config.LoadUserConfig()
	if err != nil {
		log.Printf("Warning: Failed to load config, using defaults: %v", err)
		userConfig = config.DefaultConfig()
	}

	// LoadUserConfig no longer applies globals; apply the config appearance so
	// tape playback honors the user's borders/dock/etc.
	config.ApplyAppearanceConfig(userConfig)

	if err := theme.Initialize(themeName); err != nil {
		log.Printf("Warning: Failed to load theme '%s': %v", themeName, err)
	}

	app.SetInputHandler(input.HandleInput)

	keybindRegistry := config.NewKeybindRegistry(userConfig)
	config.AnimationsEnabled = false

	return &app.OS{
		FocusedWindow:        -1,
		WindowExitChan:       make(chan string, 10),
		StateSyncChan:        make(chan *session.SessionState, 10),
		ClientEventChan:      make(chan app.ClientEvent, 10),
		MouseSnapping:        false,
		MasterRatio:          0.5,
		CurrentWorkspace:     1,
		NumWorkspaces:        9,
		WorkspaceFocus:       make(map[int]int),
		WorkspaceLayouts:     make(map[int][]app.WindowLayout),
		WorkspaceHasCustom:   make(map[int]bool),
		WorkspaceMasterRatio: make(map[int]float64),
		PendingResizes:       make(map[string][2]int),
		KeybindRegistry:      keybindRegistry,
		ShowKeys:             showKeys,
		RecentKeys:           []app.KeyEvent{},
		KeyHistoryMaxSize:    5,
	}
}

// runTapeProgram runs a tape-playback model to completion and restores the
// terminal, shared by the DSL and Lua interactive playback paths.
func runTapeProgram(initialOS *app.OS) error {
	p := tea.NewProgram(
		initialOS,
		tea.WithFPS(config.MaxFPSCap),
		tea.WithoutSignalHandler(),
		tea.WithFilter(filterMouseMotion),
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		p.Send(tea.QuitMsg{})
	}()

	finalModel, err := p.Run()

	if finalOS, ok := finalModel.(*app.OS); ok {
		finalOS.Cleanup()
	}

	fmt.Print("\033c")
	fmt.Print("\033[?1000l")
	fmt.Print("\033[?1002l")
	fmt.Print("\033[?1003l")
	fmt.Print("\033[?1004l")
	fmt.Print("\033[?1006l")
	fmt.Print("\033[?25h")
	fmt.Print("\033[?47l")
	fmt.Print("\033[0m")
	fmt.Print("\r\n")
	_ = os.Stdout.Sync()

	if err != nil {
		return fmt.Errorf("program error: %w", err)
	}

	return nil
}

func validateTapeFile(tapeFile string) error {
	content, err := os.ReadFile(tapeFile)
	if err != nil {
		return fmt.Errorf("failed to read tape file: %w", err)
	}

	if isLuaTape(tapeFile) {
		return validateLuaTapeFile(tapeFile, string(content))
	}

	commands, parseErrors := tape.ParseFile(string(content))
	if len(parseErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Parsing errors found:\n")
		for _, err := range parseErrors {
			fmt.Fprintf(os.Stderr, "  ✗ %s\n", err)
		}
		return fmt.Errorf("tape file has parsing errors")
	}

	checkmark := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("✓")
	validText := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("Tape file is valid")

	fmt.Printf("%s %s\n", checkmark, validText)

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))

	fmt.Printf("  %s: %s\n", labelStyle.Render("File"), valueStyle.Render(tapeFile))
	fmt.Printf("  %s: %s\n", labelStyle.Render("Commands"), valueStyle.Render(fmt.Sprintf("%d", len(commands))))

	if len(commands) > 0 {
		fmt.Print("\n  ")
		fmt.Print(headerStyle.Render("Command Summary:"))
		fmt.Println()

		numberStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
		cmdNameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
		argsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))

		for i, cmd := range commands {
			parts := strings.Split(cmd.String(), " ")
			if len(parts) > 1 {
				cmdName := parts[0]
				args := strings.Join(parts[1:], " ")
				fmt.Printf("    %s %s %s\n",
					numberStyle.Render(fmt.Sprintf("[%d]", i+1)),
					cmdNameStyle.Render(cmdName),
					argsStyle.Render(args))
			} else {
				fmt.Printf("    %s %s\n",
					numberStyle.Render(fmt.Sprintf("[%d]", i+1)),
					cmdNameStyle.Render(parts[0]))
			}
		}
	}

	return nil
}

// validateLuaTapeFile checks that content compiles as Lua without running it
// (lua.LState.LoadString parses and compiles a chunk but does not call it),
// so validation never executes a tuios.* side effect.
func validateLuaTapeFile(tapeFile, content string) error {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	if _, err := L.LoadString(content); err != nil {
		fmt.Fprintf(os.Stderr, "Parsing errors found:\n  ✗ %s\n", err)
		return fmt.Errorf("tape file has parsing errors")
	}

	checkmark := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("✓")
	validText := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("Tape file is valid Lua")
	fmt.Printf("%s %s\n", checkmark, validText)

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	fmt.Printf("  %s: %s\n", labelStyle.Render("File"), valueStyle.Render(tapeFile))

	return nil
}

func listTapeFiles() error {
	files, err := app.LoadTapeFiles()
	if err != nil {
		return fmt.Errorf("failed to load tape files: %w", err)
	}

	tapeDir, _ := app.GetTapeDirectory()

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	fmt.Printf("%s\n", headerStyle.Render("Tape Recordings"))
	fmt.Printf("%s\n\n", pathStyle.Render("Location: "+tapeDir))

	if len(files) == 0 {
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		fmt.Printf("%s\n", dimStyle.Render("No tape recordings found"))
		fmt.Printf("%s\n", dimStyle.Render("Use Ctrl+B, T, r in TUIOS to start recording"))
		return nil
	}

	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	sizeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	dateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	for _, file := range files {
		sizeStr := formatFileSize(file.Size)
		dateStr := file.Modified.Format("2006-01-02 15:04")
		fmt.Printf("  %s  %s  %s\n",
			nameStyle.Render(fmt.Sprintf("%-30s", file.Name)),
			sizeStyle.Render(fmt.Sprintf("%8s", sizeStr)),
			dateStyle.Render(dateStr))
	}

	fmt.Printf("\n%d tape(s) found\n", len(files))
	return nil
}

func showTapeDirectory() error {
	tapeDir, err := app.GetTapeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get tape directory: %w", err)
	}
	fmt.Println(tapeDir)
	return nil
}

// findTapeFile locates a tape by display name or by name with its .tape/.lua
// extension still attached, shared by every CLI subcommand that takes a tape
// name rather than a full path.
func findTapeFile(files []app.TapeFile, name string) *app.TapeFile {
	stripped := strings.TrimSuffix(strings.TrimSuffix(name, ".tape"), ".lua")
	for i := range files {
		if files[i].Name == name || files[i].Name == stripped {
			return &files[i]
		}
	}
	return nil
}

// tapeFileExt returns the extension a TapeFile was loaded with, for display.
func tapeFileExt(kind app.TapeFileKind) string {
	if kind == app.TapeFileLua {
		return ".lua"
	}
	return ".tape"
}

func deleteTapeFile(name string) error {
	files, err := app.LoadTapeFiles()
	if err != nil {
		return fmt.Errorf("failed to load tape files: %w", err)
	}

	targetFile := findTapeFile(files, name)
	if targetFile == nil {
		return fmt.Errorf("tape file '%s' not found", name)
	}

	fmt.Printf("Delete '%s'? (yes/no): ", targetFile.Name)
	var response string
	_, _ = fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))

	if response != "yes" && response != "y" {
		fmt.Println("Deletion cancelled.")
		return nil
	}

	if err := app.DeleteTapeFile(targetFile.Path); err != nil {
		return fmt.Errorf("failed to delete tape file: %w", err)
	}

	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	fmt.Printf("%s\n", successStyle.Render("Deleted: "+targetFile.Name))
	return nil
}

func showTapeFile(name string) error {
	files, err := app.LoadTapeFiles()
	if err != nil {
		return fmt.Errorf("failed to load tape files: %w", err)
	}

	targetFile := findTapeFile(files, name)
	if targetFile == nil {
		return fmt.Errorf("tape file '%s' not found", name)
	}

	content, err := os.ReadFile(targetFile.Path)
	if err != nil {
		return fmt.Errorf("failed to read tape file: %w", err)
	}

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	fmt.Printf("%s\n", headerStyle.Render(targetFile.Name+tapeFileExt(targetFile.Kind)))
	fmt.Printf("%s\n\n", pathStyle.Render(targetFile.Path))

	fmt.Print(string(content))

	return nil
}

func formatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%dB", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
}
