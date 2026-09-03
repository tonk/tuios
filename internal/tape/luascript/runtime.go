package luascript

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"github.com/tonk/tuios/internal/tape"
	lua "github.com/yuin/gopher-lua"
)

// pollInterval is how often wait_until re-checks its pattern against the
// window's content while waiting.
const pollInterval = 50 * time.Millisecond

// binding holds everything a tuios.* Lua function needs to reach the app.
type binding struct {
	ce       *tape.CommandExecutor
	executor tape.Executor
	bridge   *Bridge
	ctx      context.Context
	// dir is the directory the running script's own file lives in - the
	// project root for a .tuios.tape.lua project tape, the tape directory for
	// one played from the tape manager or `tuios tape run`, or "" for a script
	// with no file of its own. It is host-provided, not filesystem access the
	// script performed itself, so it does not conflict with the sandbox having
	// no io/os.
	dir string
}

// Register builds the global `tuios` table that .lua tape scripts call into.
// Every verb that touches window/app state runs through bridge.Call so it
// executes on the Bubble Tea Update() goroutine, the same as every other
// mutation in the app; sleep and wait_until are the two exceptions and are
// documented at their definitions below. dir is exposed to the script as
// tuios.project_dir().
func Register(L *lua.LState, ce *tape.CommandExecutor, executor tape.Executor, bridge *Bridge, ctx context.Context, dir string) {
	b := &binding{ce: ce, executor: executor, bridge: bridge, ctx: ctx, dir: dir}
	tbl := L.NewTable()
	L.SetGlobal("tuios", tbl)

	reg := func(name string, fn func(*binding, *lua.LState) int) {
		L.SetField(tbl, name, L.NewFunction(func(L *lua.LState) int {
			return fn(b, L)
		}))
	}

	// Window management
	reg("new_window", func(b *binding, L *lua.LState) int {
		name := L.OptString(1, "")
		if name == "" {
			return b.dispatch(L, tape.CommandTypeNewWindow)
		}
		return b.dispatch(L, tape.CommandTypeNewWindow, name)
	})
	reg("close_window", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeCloseWindow, L.OptString(1, ""))
	})
	reg("next_window", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypeNextWindow) })
	reg("prev_window", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypePrevWindow) })
	reg("focus", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeFocusWindow, L.CheckString(1))
	})
	reg("rename", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeRenameWindow, L.CheckString(1))
	})
	reg("minimize", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeMinimizeWindow, L.OptString(1, ""))
	})
	reg("restore", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeRestoreWindow, L.OptString(1, ""))
	})

	// Keyboard input
	reg("type", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeType, L.CheckString(1))
	})
	reg("key", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeKeyCombo, L.CheckString(1))
	})
	for name, cmdType := range map[string]tape.CommandType{
		"enter":     tape.CommandTypeEnter,
		"space":     tape.CommandTypeSpace,
		"backspace": tape.CommandTypeBackspace,
		"tab":       tape.CommandTypeTab,
		"escape":    tape.CommandTypeEscape,
		"delete":    tape.CommandTypeDelete,
		"up":        tape.CommandTypeUp,
		"down":      tape.CommandTypeDown,
		"left":      tape.CommandTypeLeft,
		"right":     tape.CommandTypeRight,
		"home":      tape.CommandTypeHome,
		// "end" is a Lua reserved word, so tuios.end(...) would be a syntax
		// error (tuios["end"](...) would still work, but end_ reads better).
		"end_": tape.CommandTypeEnd,
	} {
		reg(name, func(b *binding, L *lua.LState) int {
			count := L.OptInt(1, 1)
			return b.dispatch(L, cmdType, strconv.Itoa(count))
		})
	}

	// Tiling
	reg("toggle_tiling", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypeToggleTiling) })
	reg("enable_tiling", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypeEnableTiling) })
	reg("disable_tiling", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypeDisableTiling) })
	reg("snap", func(b *binding, L *lua.LState) int {
		switch dir := L.CheckString(1); dir {
		case "left":
			return b.dispatch(L, tape.CommandTypeSnapLeft)
		case "right":
			return b.dispatch(L, tape.CommandTypeSnapRight)
		case "fullscreen":
			return b.dispatch(L, tape.CommandTypeSnapFullscreen)
		default:
			L.RaiseError("snap: unknown direction %q (use left, right or fullscreen)", dir)
			return 0
		}
	})
	reg("split", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeSplit, L.CheckString(1))
	})
	reg("rotate_split", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypeRotateSplit) })
	reg("equalize_splits", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypeEqualizeSplits) })
	reg("preselect", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypePreselect, L.CheckString(1))
	})

	// Workspaces
	reg("switch_workspace", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeSwitchWS, strconv.Itoa(L.CheckInt(1)))
	})
	reg("move_to_workspace", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeMoveToWS, strconv.Itoa(L.CheckInt(1)))
	})
	reg("move_and_follow_workspace", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeMoveAndFollowWS, strconv.Itoa(L.CheckInt(1)))
	})
	reg("focus_direction", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeFocusDirection, L.CheckString(1))
	})
	reg("set_workspace_name", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeSetWorkspaceName, strconv.Itoa(L.CheckInt(1)), L.OptString(2, ""))
	})

	// Session
	reg("set_session_name", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeSetSessionName, L.OptString(1, ""))
	})
	reg("set_session_accent", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeSetSessionAccent, L.OptString(1, ""))
	})

	// Agent state
	reg("set_agent_state", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeSetAgentState,
			L.CheckString(1), L.OptString(2, ""), L.OptString(3, ""), L.OptString(4, ""))
	})

	// Animations
	reg("enable_animations", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypeEnableAnimations) })
	reg("disable_animations", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypeDisableAnimations) })
	reg("toggle_animations", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypeToggleAnimations) })

	// Misc features
	reg("toggle_zoom", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypeToggleZoom) })
	reg("smart_split", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypeSmartSplit) })
	reg("command_palette", func(b *binding, L *lua.LState) int { return b.dispatch(L, tape.CommandTypeCommandPalette) })
	reg("save_layout", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeSaveLayout, L.CheckString(1))
	})
	reg("load_layout", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeLoadLayout, L.CheckString(1))
	})

	// Config
	reg("set_config", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeSetConfig, L.CheckString(1), L.CheckString(2))
	})
	reg("set_theme", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeSetTheme, L.CheckString(1))
	})
	reg("set_dockbar_position", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeSetDockbarPosition, L.CheckString(1))
	})
	reg("set_border_style", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeSetBorderStyle, L.CheckString(1))
	})
	reg("notify", func(b *binding, L *lua.LState) int {
		return b.dispatch(L, tape.CommandTypeShowNotification, L.CheckString(1), L.OptString(2, "info"))
	})

	// State
	reg("focused_window_id", func(b *binding, L *lua.LState) int {
		var id string
		if err := b.bridge.Call(b.ctx, func() { id = b.executor.GetFocusedWindowID() }); err != nil {
			L.RaiseError("focused_window_id: %v", err)
			return 0
		}
		L.Push(lua.LString(id))
		return 1
	})
	reg("window_content", func(b *binding, L *lua.LState) int {
		windowID := L.OptString(1, "")
		content, err := b.readWindowContent(windowID)
		if err != nil {
			L.RaiseError("window_content: %v", err)
			return 0
		}
		L.Push(lua.LString(content))
		return 1
	})
	// project_dir needs no bridge.Call: it is static data the host already
	// knew before the script started, not app state that can change.
	reg("project_dir", func(b *binding, L *lua.LState) int {
		L.Push(lua.LString(b.dir))
		return 1
	})

	// sleep does not touch app state at all, so unlike every other verb here it
	// runs a plain time.Sleep on the script's own goroutine instead of a
	// bridge.Call — there is nothing for Update() to do on its behalf, and
	// round-tripping through it would only add latency.
	reg("sleep", func(b *binding, L *lua.LState) int {
		ms := L.CheckInt(1)
		t := time.NewTimer(time.Duration(ms) * time.Millisecond)
		defer t.Stop()
		select {
		case <-t.C:
		case <-b.ctx.Done():
			L.RaiseError("sleep: %v", b.ctx.Err())
		}
		return 0
	})

	// wait_until polls a window's visible content (or, with scrollback=true,
	// its content plus everything scrolled off screen - the counterpart of
	// `tuios wait-for window-output`'s scrollback-wide match) against a regex,
	// blocking the script's goroutine (not Update()) until it matches or times
	// out. It returns true on a match and false on timeout, so a script can
	// branch on the outcome (e.g. "try an SSH key; if a password prompt
	// appears, type the password") instead of treating a timeout as fatal.
	reg("wait_until", func(b *binding, L *lua.LState) int {
		pattern := L.CheckString(1)
		timeoutMs := L.OptInt(2, 5000)
		windowID := L.OptString(3, "")
		scrollback := L.OptBool(4, false)

		re, err := regexp.Compile(pattern)
		if err != nil {
			L.RaiseError("wait_until: invalid pattern: %v", err)
			return 0
		}

		read := b.readWindowContent
		if scrollback {
			read = b.readWindowScrollback
		}

		deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
		for {
			content, err := read(windowID)
			if err != nil {
				L.RaiseError("wait_until: %v", err)
				return 0
			}
			if re.MatchString(content) {
				L.Push(lua.LBool(true))
				return 1
			}
			if time.Now().After(deadline) {
				L.Push(lua.LBool(false))
				return 1
			}

			t := time.NewTimer(pollInterval)
			select {
			case <-t.C:
			case <-b.ctx.Done():
				t.Stop()
				L.RaiseError("wait_until: %v", b.ctx.Err())
				return 0
			}
		}
	})

	// wait_for_idle polls a window's visible content and returns true once it
	// has stopped changing for idle_ms, or false if timeout_ms elapses first.
	// It is a content-diffing approximation of `tuios wait-for window-idle`,
	// which instead watches the daemon's own PTY output events; this is close
	// enough for the common case (waiting out a build or install) without
	// wiring the sandbox into that event stream.
	reg("wait_for_idle", func(b *binding, L *lua.LState) int {
		idleMs := L.CheckInt(1)
		timeoutMs := L.OptInt(2, 30000)
		windowID := L.OptString(3, "")

		idle := time.Duration(idleMs) * time.Millisecond
		deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)

		last, err := b.readWindowContent(windowID)
		if err != nil {
			L.RaiseError("wait_for_idle: %v", err)
			return 0
		}
		quietSince := time.Now()
		for {
			now := time.Now()
			if now.Sub(quietSince) >= idle {
				L.Push(lua.LBool(true))
				return 1
			}
			if now.After(deadline) {
				L.Push(lua.LBool(false))
				return 1
			}

			t := time.NewTimer(pollInterval)
			select {
			case <-t.C:
			case <-b.ctx.Done():
				t.Stop()
				L.RaiseError("wait_for_idle: %v", b.ctx.Err())
				return 0
			}

			content, err := b.readWindowContent(windowID)
			if err != nil {
				L.RaiseError("wait_for_idle: %v", err)
				return 0
			}
			if content != last {
				last = content
				quietSince = time.Now()
			}
		}
	})

	// wait_for_exit polls a window's shell process and returns true once it
	// has exited, or false if timeout_ms elapses first. The counterpart of
	// `tuios wait-for window-exit`.
	reg("wait_for_exit", func(b *binding, L *lua.LState) int {
		timeoutMs := L.OptInt(1, 30000)
		windowID := L.OptString(2, "")

		deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
		for {
			var exited bool
			var err error
			if callErr := b.bridge.Call(b.ctx, func() { exited, err = b.executor.WindowProcessExited(windowID) }); callErr != nil {
				L.RaiseError("wait_for_exit: %v", callErr)
				return 0
			}
			if err != nil {
				L.RaiseError("wait_for_exit: %v", err)
				return 0
			}
			if exited {
				L.Push(lua.LBool(true))
				return 1
			}
			if time.Now().After(deadline) {
				L.Push(lua.LBool(false))
				return 1
			}

			t := time.NewTimer(pollInterval)
			select {
			case <-t.C:
			case <-b.ctx.Done():
				t.Stop()
				L.RaiseError("wait_for_exit: %v", b.ctx.Err())
				return 0
			}
		}
	})

	// Structured, read-only queries. Each answers the same question as the
	// matching CLI verb (get-window, list-windows, session-info) but, like
	// window_content, reads this client's own live state directly instead of
	// round-tripping to the daemon.
	reg("get_window", func(b *binding, L *lua.LState) int {
		identifier := L.OptString(1, "")
		var data map[string]any
		var err error
		if callErr := b.bridge.Call(b.ctx, func() {
			if identifier == "" {
				data, err = b.executor.GetFocusedWindowData()
			} else {
				data, err = b.executor.GetWindowData(identifier)
			}
		}); callErr != nil {
			L.RaiseError("get_window: %v", callErr)
			return 0
		}
		if err != nil {
			L.RaiseError("get_window: %v", err)
			return 0
		}
		L.Push(toLuaValue(L, data))
		return 1
	})
	reg("list_windows", func(b *binding, L *lua.LState) int {
		var data map[string]any
		if err := b.bridge.Call(b.ctx, func() { data = b.executor.GetWindowListData() }); err != nil {
			L.RaiseError("list_windows: %v", err)
			return 0
		}
		L.Push(toLuaValue(L, data))
		return 1
	})
	reg("session_info", func(b *binding, L *lua.LState) int {
		var data map[string]any
		if err := b.bridge.Call(b.ctx, func() { data = b.executor.GetSessionInfoData() }); err != nil {
			L.RaiseError("session_info: %v", err)
			return 0
		}
		L.Push(toLuaValue(L, data))
		return 1
	})
}

// dispatch builds a *tape.Command and runs it through the same
// CommandExecutor.Execute the .tape DSL uses, on the Update() goroutine via
// bridge.Call. This is what lets nearly every tuios.* verb be a one-liner:
// key-combo conversion, repeat counts and name/ID fallback all already live
// in Execute and don't need reimplementing here.
func (b *binding) dispatch(L *lua.LState, cmdType tape.CommandType, args ...string) int {
	cmd := &tape.Command{Type: cmdType, Args: args}
	var execErr error
	if err := b.bridge.Call(b.ctx, func() { execErr = b.ce.Execute(cmd) }); err != nil {
		L.RaiseError("%s: %v", cmdType, err)
		return 0
	}
	if execErr != nil {
		L.RaiseError("%s: %v", cmdType, execErr)
		return 0
	}
	return 0
}

// readWindowContent fetches a window's visible content through the bridge.
func (b *binding) readWindowContent(windowID string) (string, error) {
	var content string
	var readErr error
	if err := b.bridge.Call(b.ctx, func() { content, readErr = b.executor.GetWindowContent(windowID) }); err != nil {
		return "", err
	}
	if readErr != nil {
		return "", fmt.Errorf("read window content: %w", readErr)
	}
	return content, nil
}

// readWindowScrollback is readWindowContent's scrollback-inclusive sibling,
// used when wait_until is asked to match against more than the visible screen.
func (b *binding) readWindowScrollback(windowID string) (string, error) {
	var content string
	var readErr error
	if err := b.bridge.Call(b.ctx, func() { content, readErr = b.executor.GetWindowScrollback(windowID) }); err != nil {
		return "", err
	}
	if readErr != nil {
		return "", fmt.Errorf("read window scrollback: %w", readErr)
	}
	return content, nil
}

// toLuaValue converts a Go value built by one of the structured-query
// Executor methods (map[string]any, with nested maps, slices, strings, bools
// and numbers - the same shapes the CLI's --json output serializes) into the
// equivalent Lua value, so tuios.get_window/list_windows/session_info hand a
// script something it can index instead of a string it would have to parse.
func toLuaValue(L *lua.LState, v any) lua.LValue {
	if v == nil {
		return lua.LNil
	}
	switch val := v.(type) {
	case string:
		return lua.LString(val)
	case bool:
		return lua.LBool(val)
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		t := L.NewTable()
		for _, key := range rv.MapKeys() {
			L.SetField(t, fmt.Sprint(key.Interface()), toLuaValue(L, rv.MapIndex(key).Interface()))
		}
		return t
	case reflect.Slice, reflect.Array:
		t := L.NewTable()
		for i := range rv.Len() {
			t.Append(toLuaValue(L, rv.Index(i).Interface()))
		}
		return t
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return lua.LNumber(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return lua.LNumber(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return lua.LNumber(rv.Float())
	default:
		return lua.LString(fmt.Sprint(v))
	}
}
