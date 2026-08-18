-- SSH login with a password pulled from `pass` (https://www.passwordstore.org/).
--
-- The Lua sandbox tuios runs tape scripts in deliberately has no filesystem
-- or process access (no os, io, or require -- see docs/TAPE_LUA.md), so this
-- script can never call `pass` itself. Instead the password is resolved by
-- the *shell being typed into*, via command substitution, the moment the ssh
-- command runs -- the secret passes from pass(1) to sshpass to ssh entirely
-- inside that shell and is never seen by Lua, logged by tuios, or written to
-- this file.
--
-- SSHPASS as an env var (sshpass -e) rather than `sshpass -p "$(...)"` also
-- keeps the password out of `ps` output on the remote/local host.
--
-- Edit HOST and the pass(1) entry name below, then run with:
--   tuios tape run examples/lua/ssh_password_login.lua

local HOST = "user@example.com"
local PASS_ENTRY = "cust/passwd"

tuios.new_window()
tuios.type('SSHPASS="$(pass show ' .. PASS_ENTRY .. ')" sshpass -e ssh ' .. HOST)
tuios.enter()

if tuios.wait_until([[\$\s*$]], 15000) then
	tuios.notify("Logged in to " .. HOST, "success")
else
	tuios.notify("Timed out waiting for a shell prompt on " .. HOST, "warning")
end
