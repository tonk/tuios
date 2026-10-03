package app

import "time"

// Version is the tuios build version, set by main() from the linker-injected
// build variable (see cmd/tuios/main.go, cmd/tuios-web/main.go). It defaults
// to "dev" so a `go run`/`go test` invocation that never calls main still has
// a sane value rather than an empty string.
var Version = "dev"

// BuildDate is the compile date, set by main() from the linker-injected
// build variable. It stays "unknown" when the build did not inject one.
var BuildDate = "unknown"

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

// buildDateLabel formats BuildDate as "YYYY-mm-dd HH:MM UTC". Make and
// goreleaser both inject RFC 3339; anything that doesn't parse (such as the
// "unknown" default) is shown as is.
func buildDateLabel() string {
	t, err := time.Parse(time.RFC3339, BuildDate)
	if err != nil {
		return BuildDate
	}
	return t.UTC().Format("2006-01-02 15:04") + " UTC"
}
