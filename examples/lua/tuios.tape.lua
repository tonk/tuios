-- Project dir the tape was launched from
local dir = tuios.project_dir()

-- Keep windows floating instead of auto-tiling at startup
-- (tuios.split() forces tiling back on, so use new_window() here instead)
tuios.disable_tiling()

-- Rename the initial window
tuios.rename("build")

-- Open a floating editor window
tuios.new_window("editor")
tuios.type("cd '" .. dir .. "' && vim .")
tuios.enter()

-- Open a floating git status window
tuios.new_window("git")
tuios.type("cd '" .. dir .. "' && git status")
tuios.enter()

-- Leave focus on the editor
tuios.focus("editor")
