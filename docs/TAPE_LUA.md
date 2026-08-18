# Lua Tape Scripting

## Overview

The [tape DSL](TAPE_SCRIPTING.md) has no variables, loops, or conditionals. Most
tapes don't need them, but some workflows genuinely do — "create N windows and
tile them," or "try an SSH key, and only prompt for a password if the server
actually asks for one." For those, a `.lua` tape script runs alongside `.tape`
DSL scripts in the same tape directory, giving you a real language (variables,
`if`/`else`, `for`/`while`, functions) that calls into the same window-manager
actions the DSL commands do.

A `.lua` script is a second, additive format, not a replacement — the DSL,
its recorder, and its `.tuios.tape` project-autorun flow are unchanged. Lua
tapes only ever run when you explicitly ask for them (see
[Explicit-run only](#explicit-run-only) below).

## Running a Lua tape

```sh
tuios tape run examples/lua/ssh_key_login.lua   # run a .lua file directly
tuios tape validate my_script.lua               # syntax-check without running it
```

Or drop a `.lua` file into the tape directory (`tuios tape dir` prints its
path) and play it from the in-app tape manager (`Ctrl+B, T`) — it lists
alongside your `.tape` files and Enter runs either kind.

While a Lua tape is running, `Ctrl+P` or `Esc` stops it. Unlike a DSL tape,
there's no pause/resume: a live script has no fixed step to resume from
mid-loop, only a "stop" makes sense.

## The `tuios` API

Every verb below runs on tuios's own event loop, the same as a DSL command —
it's safe to call in a tight loop or from inside a conditional.

**Windows**: `tuios.new_window([name])`, `tuios.close_window([name])`,
`tuios.next_window()`, `tuios.prev_window()`, `tuios.focus(name_or_id)`,
`tuios.rename(name)`, `tuios.minimize([name])`, `tuios.restore([name])`

**Keyboard**: `tuios.type(text)`, `tuios.key(combo)` (e.g. `"Ctrl+C"`,
`"Alt+1"`), `tuios.enter([count])`, `tuios.space([count])`,
`tuios.backspace([count])`, `tuios.tab([count])`, `tuios.escape([count])`,
`tuios.delete([count])`, `tuios.up/down/left/right([count])`,
`tuios.home([count])`, `tuios.end_([count])`

**Tiling**: `tuios.toggle_tiling()`, `tuios.enable_tiling()`,
`tuios.disable_tiling()`, `tuios.snap("left"|"right"|"fullscreen")`,
`tuios.split("horizontal"|"vertical")`, `tuios.rotate_split()`,
`tuios.equalize_splits()`, `tuios.preselect("left"|"right"|"up"|"down")`

**Workspaces**: `tuios.switch_workspace(n)`, `tuios.move_to_workspace(n)`,
`tuios.move_and_follow_workspace(n)`, `tuios.focus_direction(dir)`

**Misc**: `tuios.enable_animations()`, `tuios.disable_animations()`,
`tuios.toggle_animations()`, `tuios.toggle_zoom()`, `tuios.smart_split()`,
`tuios.command_palette()`, `tuios.save_layout(name)`,
`tuios.load_layout(name)`, `tuios.set_config(path, value)`,
`tuios.set_theme(name)`, `tuios.set_dockbar_position(pos)`,
`tuios.set_border_style(style)`, `tuios.notify(message, [kind])`

**Timing and state** (the two verbs that don't work like the rest — see
below): `tuios.sleep(ms)`, `tuios.wait_until(pattern, [timeout_ms],
[window_id])`, `tuios.focused_window_id()`, `tuios.window_content([window_id])`

### `sleep` and `wait_until`

`tuios.sleep(ms)` blocks the *script*, not the UI — tuios keeps rendering and
responding while a Lua tape sleeps.

`tuios.wait_until(pattern, timeout_ms)` polls a window's visible content
against a regex (Go's `regexp` syntax, not Lua patterns — see the escaping
note below) every 50ms and **returns `true` on a match or `false` on
timeout**. It never raises for "didn't happen in time," so branching on the
outcome is a plain `if`:

```lua
tuios.new_window()
tuios.type("ssh user@example.com")
tuios.enter()

if tuios.wait_until("[Pp]assword:", 8000) then
    -- a password prompt actually showed up; see
    -- examples/lua/ssh_try_key_then_password.lua for the rest of this one
else
    tuios.notify("Logged in with an SSH key", "success")
end
```

**Pattern escaping**: patterns are compiled by Go's `regexp` package. In a
normal `"..."` Lua string you'd have to double every backslash (`"\\$\\s*$"`);
Lua's long-bracket string form passes them through untouched, so
`[[\$\s*$]]` is easier to read than `"\\$\\s*$"` for the same pattern.

## Sandboxing

A `.lua` tape has no filesystem, process, or environment access: `io`, `os`
and `package` are never loaded, and `dofile`, `loadfile` and `require` are
removed even though they're normally part of Lua's base library. Only `base`
(language features, `pairs`/`pcall`/`type`/etc.), `table`, `string`, `math`
and `coroutine` are open, plus the `tuios.*` table above.

This means a Lua tape **cannot read a secret itself** — there's no
`os.getenv` or `io.popen` to shell out to a password manager. If a workflow
needs one (see `examples/lua/ssh_password_login.lua`), let the *shell being
typed into* resolve it via command substitution when the command runs:

```lua
tuios.type('SSHPASS="$(pass show cust/passwd)" sshpass -e ssh user@example.com')
```

The secret passes from `pass` to `sshpass` to `ssh` entirely inside that
shell; Lua never sees it, and it's never written to the tape file.

## Explicit-run only

Lua tapes are never eligible for the `.tuios.tape` project-autorun flow: that
flow's trust/review dialog shows you the raw source of a tape before it's
allowed to run unattended, which is a reasonable safety net for the DSL's
bounded command list but a much weaker one for an arbitrary Lua program. For
now, a `.lua` tape only ever runs when you explicitly invoke it — `tuios tape
run`, or Enter in the tape manager.

## See Also

- [Tape Scripting Language Reference](TAPE_SCRIPTING.md) - the `.tape` DSL
- [Example Lua Tape Scripts](../examples/lua/)
- [Project Tapes and Autorun](PROJECT_TAPES.md)
