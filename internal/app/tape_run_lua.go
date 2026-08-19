package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/tape"
	"github.com/Gaurav-Gosain/tuios/internal/tape/luascript"
	lua "github.com/yuin/gopher-lua"
)

// LuaFinishedMsg reports that a .lua tape's goroutine has returned, either
// because the script finished, it errored, or it was canceled.
type LuaFinishedMsg struct{ Err error }

// listenForLuaDone mirrors ListenForPTYData: it blocks on the script's
// completion channel and turns the wakeup into a message back into Update().
func listenForLuaDone(done <-chan error) tea.Cmd {
	return func() tea.Msg {
		return LuaFinishedMsg{Err: <-done}
	}
}

// StartLuaPlayback runs a .lua tape script's source. dir is the directory the
// script's own file lives in, exposed to it as tuios.project_dir() (pass ""
// for a script with no file of its own). Unlike startTapePlayback, there is no
// upfront command list to hand to a Player: the script itself decides what
// runs and in what order, so execution happens on its own goroutine, bridged
// onto Update() one call at a time via LuaBridge (see
// internal/tape/luascript). It returns the tea.Cmds the caller must dispatch
// to start relaying calls and to be notified when the script finishes (the
// interactive tape manager path); a caller building the model before the
// Bubble Tea program exists (the `tuios tape run foo.lua` CLI path) can
// discard the return value, since Init() arms the same listeners once the
// program starts.
func (m *OS) StartLuaPlayback(script, name, dir string) []tea.Cmd {
	if m.ScriptMode || m.LuaRunning {
		m.ShowNotification("A tape is already running", "warning", config.NotificationDuration)
		return nil
	}
	if script == "" {
		m.ShowNotification("Tape is empty; nothing to run", "warning", config.NotificationDuration)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	bridge := luascript.NewBridge()
	ce := tape.NewCommandExecutor(m)

	m.LuaRunning = true
	m.LuaName = name
	m.LuaBridge = bridge
	m.LuaCancel = cancel
	m.LuaCanceled = false

	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	luascript.OpenSafeLibs(L)
	L.SetContext(ctx)
	luascript.Register(L, ce, m, bridge, ctx, dir)

	done := make(chan error, 1)
	m.luaDone = done
	go func() {
		defer L.Close()
		done <- L.DoString(script)
	}()

	m.ShowNotification("Running: "+name, "info", 2*time.Second)
	return []tea.Cmd{bridge.Listen(), listenForLuaDone(done)}
}

// CancelLuaPlayback stops the running Lua tape, if any. The script's goroutine
// unblocks on its context (either inside a bridge.Call, a sleep/wait_until, or
// at the next VM instruction boundary per LState.SetContext) and reports back
// through LuaFinishedMsg like any other completion.
func (m *OS) CancelLuaPlayback() {
	if !m.LuaRunning || m.LuaCancel == nil {
		return
	}
	m.LuaCanceled = true
	m.LuaCancel()
}

// runProjectTapeLua executes a reviewed, eligible .tuios.tape.lua. content is
// the exact bytes that were hashed and shown to the user; it is never re-read
// from disk. dir is the project root (the directory holding the tape).
//
// It is the Lua counterpart of runProjectTape, sharing the same trust/review
// callers ("Run once", "Trust and run", and auto-mode) and the same
// session-per-project behavior, but with two simplifications documented in
// docs/PROJECT_TAPES.md: there is no header (Session/Scope/Workspace/Require
// directives are a DSL-only concept), so a Lua project tape always builds a
// session named after the directory. The returned command is the Lua bridge's
// listener commands, which the caller must dispatch.
func (m *OS) runProjectTapeLua(content []byte, dir string) tea.Cmd {
	if m.ScriptMode || m.LuaRunning {
		m.ShowNotification("A tape is already running", "warning", config.NotificationDuration)
		return nil
	}
	if len(content) == 0 {
		m.ShowNotification("Tape is empty; nothing to run", "warning", config.NotificationDuration)
		return nil
	}

	// Suppress detection while this tape runs, and mark the project root handled
	// so re-entering it cannot chain another run.
	if m.tapeDetect.handled == nil {
		m.tapeDetect.handled = make(map[string]bool)
	}
	m.tapeDetect.handled[dir] = true

	name := sanitizeSessionName(filepath.Base(dir))
	if name == "" {
		name = "project"
	}

	// Session scope needs the daemon's named sessions. Outside a daemon session
	// there is nowhere to create one, so fall back to running the raw script in
	// the current session, honest best-effort like the DSL's Scope-current path.
	if m.DaemonClient == nil {
		m.ShowNotification("Tape: no session backend, running in current session", "warning", config.NotificationDuration*2)
		return tea.Batch(m.StartLuaPlayback(string(content), name, dir)...)
	}

	if m.sessionExists(name) {
		// The session is the constructor's output, run once. Re-entry just takes
		// the user back to it; it never re-runs the tape.
		if err := m.SwitchToSession(name); err != nil {
			m.ShowNotification("Tape: switch to "+name+" failed: "+err.Error(), "error", config.NotificationDuration*2)
		}
		return nil
	}

	if err := m.SwitchToSession(name); err != nil {
		m.ShowNotification("Tape: create session "+name+" failed: "+err.Error(), "error", config.NotificationDuration*2)
		return nil
	}

	// A freshly created session is empty. Seed a single window, give the daemon's
	// asynchronous window creation time to land, then put its shell at the
	// project root and enable tiling before the script's own commands run - the
	// same seed the DSL's seedCommands performs, expressed in Lua since a Lua
	// tape has no host-side command list to prepend to.
	m.AddWindow("")
	script := luaSeedScript(dir) + string(content)
	m.ShowNotification("Building project session "+name, "info", config.NotificationDuration)
	return tea.Batch(m.StartLuaPlayback(script, name, dir)...)
}

// luaSeedScript returns the Lua statements that settle a freshly created
// session's seed window, cd it to dir, and enable tiling, mirroring
// seedCommands' DSL command list.
func luaSeedScript(dir string) string {
	cmd := "cd " + shellSingleQuote(dir)
	return fmt.Sprintf(
		"tuios.sleep(%d)\ntuios.type(%s)\ntuios.enter()\ntuios.sleep(200)\ntuios.enable_tiling()\n",
		tapeSeedSettle.Milliseconds(), luaQuote(cmd),
	)
}

// luaQuote renders s as a double-quoted Lua string literal.
func luaQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
