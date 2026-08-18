-- SSH login with a key already loaded in the agent.
--
-- Edit HOST below, then run with:
--   tuios tape run examples/lua/ssh_key_login.lua

local HOST = "user@example.com"

tuios.new_window()
tuios.type("ssh " .. HOST)
tuios.enter()

-- wait_until returns true on a match, false on timeout -- it never raises for
-- "didn't happen in time", so a plain if/else is enough to branch on it.
--
-- Patterns are Go regexes, not Lua patterns. [[...]] (Lua's long-bracket
-- string form) passes backslashes through untouched, which is easier to read
-- than escaping them in a normal "..." string.
if tuios.wait_until([[\$\s*$]], 10000) then
	tuios.notify("Logged in to " .. HOST, "success")
else
	tuios.notify("Timed out waiting for a shell prompt on " .. HOST, "warning")
end
