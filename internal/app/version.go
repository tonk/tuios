package app

// Version is the tuios build version, set by main() from the linker-injected
// build variable (see cmd/tuios/main.go, cmd/tuios-web/main.go). It defaults
// to "dev" so a `go run`/`go test` invocation that never calls main still has
// a sane value rather than an empty string.
var Version = "dev"

// versionLabel normalizes Version for display: goreleaser's version template
// omits the "v" prefix while the Makefile's ldflags add one, so builds from
// the two paths would otherwise show up differently in the same UI.
func versionLabel() string {
	if Version == "" || Version == "dev" {
		return Version
	}
	if Version[0] == 'v' {
		return Version
	}
	return "v" + Version
}
