package app

import (
	"context"
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

// StartLuaPlayback runs a .lua tape script's source. Unlike startTapePlayback,
// there is no upfront command list to hand to a Player: the script itself
// decides what runs and in what order, so execution happens on its own
// goroutine, bridged onto Update() one call at a time via LuaBridge (see
// internal/tape/luascript). It returns the tea.Cmds the caller must dispatch
// to start relaying calls and to be notified when the script finishes (the
// interactive tape manager path); a caller building the model before the
// Bubble Tea program exists (the `tuios tape run foo.lua` CLI path) can
// discard the return value, since Init() arms the same listeners once the
// program starts.
func (m *OS) StartLuaPlayback(script, name string) []tea.Cmd {
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
	luascript.Register(L, ce, m, bridge, ctx)

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
