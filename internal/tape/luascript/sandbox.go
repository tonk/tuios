package luascript

import lua "github.com/yuin/gopher-lua"

// OpenSafeLibs opens only the Lua standard libraries a tape script needs to
// script control flow and text: base language features, tables, strings and
// math. It deliberately skips io, os, package (require/loadfile) and debug,
// so a .lua tape has no filesystem, process or environment access beyond
// whatever the tuios.* API explicitly grants it — the same "explicit-run
// only" trust posture as running an unreviewed script at all, not a reason to
// also hand it the host.
func OpenSafeLibs(L *lua.LState) {
	for _, lib := range []lua.LGFunction{
		lua.OpenBase,
		lua.OpenTable,
		lua.OpenString,
		lua.OpenMath,
		lua.OpenCoroutine,
	} {
		L.Push(L.NewFunction(lib))
		L.Call(0, 0)
	}

	// OpenBase also registers a few functions that reach the filesystem or a
	// module search path even without the io/os/package libraries: dofile and
	// loadfile load and run an arbitrary file by path, and require resolves
	// against package.path. None of that belongs in a sandboxed tape script.
	for _, name := range []string{"dofile", "loadfile", "require", "module"} {
		L.SetGlobal(name, lua.LNil)
	}
}
