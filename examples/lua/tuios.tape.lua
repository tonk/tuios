local dir = tuios.project_dir()

tuios.rename("build")

tuios.split("vertical")
tuios.rename("editor")
tuios.type("cd '" .. dir .. "' && vim .")
tuios.enter()

tuios.split("horizontal")
tuios.rename("git")
tuios.type("cd '" .. dir .. "' && git status")
tuios.enter()

tuios.focus("editor")
