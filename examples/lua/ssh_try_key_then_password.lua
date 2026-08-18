-- SSH login when you don't know up front whether a key will work: try key
-- auth first, and only fall back to a password if the server actually asks
-- for one. This is the case the .tape DSL can't express (no if/else) and the
-- reason to reach for Lua at all -- see docs/TAPE_LUA.md.
--
-- Edit HOST and the pass(1) entry name below, then run with:
--   tuios tape run examples/lua/ssh_try_key_then_password.lua

local HOST = "user@example.com"
local PASS_ENTRY = "cust/passwd"

tuios.new_window()
tuios.type("ssh " .. HOST)
tuios.enter()

if tuios.wait_until("[Pp]assword:", 8000) then
	tuios.notify("Key auth didn't work, retrying with a password", "info")

	-- A live "Password:" prompt is ssh reading raw keystrokes, not a shell --
	-- there's nothing there to expand "$(pass show ...)" the way there is in
	-- ssh_password_login.lua. So back out of this attempt (Ctrl+C) and redial
	-- with sshpass instead, which lets the *shell* resolve the secret before
	-- ssh ever asks for it. Lua still never sees the password either way.
	tuios.key("Ctrl+C")
	tuios.type('SSHPASS="$(pass show ' .. PASS_ENTRY .. ')" sshpass -e ssh ' .. HOST)
	tuios.enter()
	tuios.wait_until([[\$\s*$]], 15000)
end

tuios.notify("Logged in to " .. HOST, "success")
