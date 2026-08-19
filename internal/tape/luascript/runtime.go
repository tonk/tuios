package luascript

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/Gaurav-Gosain/tuios/internal/tape"
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

	// wait_until polls a window's visible content against a regex, blocking the
	// script's goroutine (not Update()) until it matches or times out. It
	// returns true on a match and false on timeout, so a script can branch on
	// the outcome (e.g. "try an SSH key; if a password prompt appears, type the
	// password") instead of treating a timeout as fatal.
	reg("wait_until", func(b *binding, L *lua.LState) int {
		pattern := L.CheckString(1)
		timeoutMs := L.OptInt(2, 5000)
		windowID := L.OptString(3, "")

		re, err := regexp.Compile(pattern)
		if err != nil {
			L.RaiseError("wait_until: invalid pattern: %v", err)
			return 0
		}

		deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
		for {
			content, err := b.readWindowContent(windowID)
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
