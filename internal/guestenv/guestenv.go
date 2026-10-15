// Package guestenv derives environment values tuios exports to the processes
// it spawns in its windows. Both the local terminal path and the daemon's PTY
// path build a guest environment, and they must agree on what they advertise.
package guestenv

import "strings"

// TermProgram returns the TERM_PROGRAM value for a guest process, given the
// graphics capabilities tuios can actually forward to the host terminal.
//
// Tools that draw images (chafa, yazi, kitten icat) pick their output format
// from the environment rather than by querying the terminal, and none of them
// know the name "TUIOS", so advertising it made every guest fall back to
// unicode block art even when tuios was forwarding kitty graphics to a capable
// host. Naming a terminal the tools do know makes them emit the protocol tuios
// passes through: ghostty for kitty graphics, WezTerm for sixel. TERM is left
// alone so no guest needs a terminfo entry that may not be installed, and
// tuios remains identifiable through TUIOS_SESSION and TUIOS_WINDOW_ID.
func TermProgram(kittyGraphics, sixelGraphics bool) string {
	switch {
	case kittyGraphics:
		return "ghostty"
	case sixelGraphics:
		return "WezTerm"
	default:
		return "TUIOS"
	}
}

// Environ returns the environment tuios starts a guest from: its own, minus the
// variables that describe the terminal tuios itself runs in rather than the
// one it presents to the guest.
//
// VTE_VERSION is the one that matters. It is inherited from a VTE host such as
// gnome-terminal, and Fedora/RHEL's /etc/profile.d/vte.sh keys off it alone to
// install a PROMPT_COMMAND that sends an OSC 777 "Command completed" desktop
// notification after every command. Inside tuios that is not a VTE, so every
// finished command raised a popup in the dock.
func Environ(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		if strings.HasPrefix(kv, "VTE_VERSION=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
