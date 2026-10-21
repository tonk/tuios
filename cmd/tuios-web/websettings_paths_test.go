package main

import (
	"regexp"
	"testing"
)

// absoluteAppPath matches a path the injected page code would resolve from the
// site root rather than from the page: a quoted or url()-wrapped string that
// starts with a single slash.
var absoluteAppPath = regexp.MustCompile(`(?:['"` + "`" + `]|url\()/[A-Za-z]`)

// commentLine matches a whole-line JavaScript or CSS comment, which may quote
// a root-relative path as the thing not to do.
var commentLine = regexp.MustCompile(`(?m)^\s*(?://|/?\*).*$`)

// TestInjectedPageCodeUsesRelativePaths pins that everything tuios-web injects
// into the page addresses its own routes relative to the page. Behind a
// reverse proxy that serves tuios-web under a prefix (location /shell/), a
// path starting with "/" goes to whatever the proxy serves at the site root
// instead. The trainer picker's open-tab poll did that: it never got an
// answer, so selecting a trainee timed out with "Couldn't open that session
// in time" while opening ?attach=<user> by hand worked.
func TestInjectedPageCodeUsesRelativePaths(t *testing.T) {
	sources := map[string]string{
		"frontDoorWebsocketHead": frontDoorWebsocketHead("tonk"),
		"settingsInjectHead":     settingsInjectHead("#000000", "tonk"),
		"settingsInjectHTML":     settingsInjectHTML(""),
		"settingsInjectFooter":   settingsInjectFooter("null"),
	}
	for name, src := range sources {
		src = commentLine.ReplaceAllString(src, "")
		for _, m := range absoluteAppPath.FindAllStringIndex(src, -1) {
			start, end := max(m[0]-40, 0), min(m[1]+40, len(src))
			t.Errorf("%s uses a root-relative path, which breaks under a proxy prefix:\n...%s...", name, src[start:end])
		}
	}
}
