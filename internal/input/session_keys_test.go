package input

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tonk/tuios/internal/app"
	"github.com/tonk/tuios/internal/config"
)

// altShift builds the key event a terminal actually delivers for alt+shift+<r>.
// Both the legacy escape prefix and the Kitty protocol decode to a lowercase code
// carrying ModAlt|ModShift, which stringifies as "alt+shift+n"; spelling the
// binding "alt+N" instead would silently normalize to plain alt+n, which is
// already bound to next-window.
func altShift(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, ShiftedCode: r - 32, Mod: tea.ModAlt | tea.ModShift}
}

func TestAltShiftKeysSpellWhatTheTerminalSends(t *testing.T) {
	cfg := config.DefaultConfig()
	registry := config.NewKeybindRegistry(cfg)

	for _, tc := range []struct {
		msg  tea.KeyPressMsg
		key  string
		want string
	}{
		{altShift('n'), "alt+shift+n", "next_session"},
		{altShift('p'), "alt+shift+p", "prev_session"},
	} {
		if got := tc.msg.String(); got != tc.key {
			t.Fatalf("terminal spells the chord %q, but the binding is written %q", got, tc.key)
		}
		if got := registry.GetAction(tc.msg.String()); got != tc.want {
			t.Errorf("%s resolved to %q, want %q", tc.key, got, tc.want)
		}
		if !GetDispatcher().HasAction(tc.want) {
			t.Errorf("%s has no registered handler", tc.want)
		}
	}

	// The shifted chords must not have swallowed the unshifted ones.
	for key, want := range map[string]string{"alt+n": "terminal_next_window", "alt+p": "terminal_prev_window"} {
		if got := registry.GetTerminalModeAction(key); got != want {
			t.Errorf("%s resolved to %q, want %q", key, got, want)
		}
	}
}

// A digit has no case to fold, so alt+shift+<digit> reaches the registry in
// three different spellings depending on what the host terminal speaks, and all
// three have to find the same action.
func TestAltShiftDigitsResolveInEverySpellingATerminalSends(t *testing.T) {
	registry := config.NewKeybindRegistry(config.DefaultConfig())

	for _, tc := range []struct {
		what string
		msg  tea.KeyPressMsg
		key  string
		want string
	}{
		{"kitty", tea.KeyPressMsg{Code: '1', ShiftedCode: '!', Mod: tea.ModAlt | tea.ModShift}, "alt+shift+1", "move_and_follow_1"},
		{"legacy", tea.KeyPressMsg{Code: '!', Mod: tea.ModAlt}, "alt+!", "move_and_follow_1"},
		{"modifyOtherKeys", tea.KeyPressMsg{Code: '!', Mod: tea.ModAlt | tea.ModShift}, "alt+shift+!", "move_and_follow_1"},
		{"kitty", tea.KeyPressMsg{Code: '9', ShiftedCode: '(', Mod: tea.ModAlt | tea.ModShift}, "alt+shift+9", "move_and_follow_9"},
		{"legacy", tea.KeyPressMsg{Code: '(', Mod: tea.ModAlt}, "alt+(", "move_and_follow_9"},
	} {
		if got := tc.msg.String(); got != tc.key {
			t.Fatalf("%s terminal spells the chord %q, want %q", tc.what, got, tc.key)
		}
		if got := registry.GetAction(tc.key); got != tc.want {
			t.Errorf("%s spelling %q resolved to %q, want %q", tc.what, tc.key, got, tc.want)
		}
		if !GetDispatcher().HasAction(tc.want) {
			t.Errorf("%s has no registered handler", tc.want)
		}
	}

	// Aliasing the shifted chords must leave the unshifted digits alone.
	for key, want := range map[string]string{"alt+1": "select_window_1", "alt+9": "select_window_9"} {
		if got := registry.GetAction(key); got != want {
			t.Errorf("%s resolved to %q, want %q", key, got, want)
		}
	}
}

// TestAltDigitSelectsWindowInFloatingMode is the regression test for
// select_window_N resolving to the right action but not actually focusing
// anything: the shared handler used to re-derive the target number from the
// pressed key's own first character, which is the modifier letter ('a') for
// a chord like "alt+2", not the digit. It also used to fall back to corner-
// snapping (or, past 4, nothing at all) whenever auto-tiling was off, which
// is this fixture's default (floating panes) and this repo's own default
// (startup.tiled = false) — so alt+N silently never selected a window in the
// most common layout.
func TestAltDigitSelectsWindowInFloatingMode(t *testing.T) {
	o := twoPaneOS(t)
	if o.AutoTiling {
		t.Fatal("fixture is meant to be floating (auto-tiling off)")
	}

	o, _ = HandleKeyPress(tea.KeyPressMsg{Code: '2', Mod: tea.ModAlt}, o)
	if got := o.FocusedWindow; got != 1 {
		t.Fatalf("alt+2 focused window %d, want 1 (pane b)", got)
	}

	o, _ = HandleKeyPress(tea.KeyPressMsg{Code: '1', Mod: tea.ModAlt}, o)
	if got := o.FocusedWindow; got != 0 {
		t.Fatalf("alt+1 focused window %d, want 0 (pane a)", got)
	}
}

// TestNumberKeySkipsMinimizedWindowsWhenTiled pins the one case handleNumberKey
// still has to get right without re-deriving anything from the key: in
// auto-tiling, select_window_N addresses the Nth window that is actually on
// screen, the same count workspacePosition uses for the tab title and the
// rail, so a minimized window in between must not shift the numbering a
// user's eyes are reading off the visible panes.
func TestNumberKeySkipsMinimizedWindowsWhenTiled(t *testing.T) {
	o := twoPaneOS(t)
	o.AutoTiling = true
	o.Windows[0].Minimized = true

	o, _ = HandleKeyPress(tea.KeyPressMsg{Code: '1', Mod: tea.ModAlt}, o)
	if got := o.FocusedWindow; got != 1 {
		t.Fatalf("alt+1 with window a minimized focused %d, want 1 (the one visible pane, b)", got)
	}
}

// TestSessionKeysReachTheActionFromBothModes checks the routing, not the switch:
// standalone has no other session to go to, so the proof that the key arrived is
// the hint it leaves behind. Terminal mode is the case that needs the check,
// since a main-section binding only fires there via isTerminalSafeAction.
func TestSessionKeysReachTheActionFromBothModes(t *testing.T) {
	for _, mode := range []app.Mode{app.WindowManagementMode, app.TerminalMode} {
		for _, msg := range []tea.KeyPressMsg{altShift('n'), altShift('p')} {
			o := twoPaneOS(t)
			o.Mode = mode
			o, _ = HandleKeyPress(msg, o)
			if len(o.Notifications) == 0 {
				t.Fatalf("%s in mode %v produced no response", msg.String(), mode)
			}
			if got := o.Notifications[len(o.Notifications)-1].Message; !strings.Contains(got, "No other sessions") {
				t.Errorf("%s in mode %v said %q", msg.String(), mode, got)
			}
		}
	}
}

// TestLeaderExploreTogglesRailFocus checks both halves of the toggle. The second
// half only works because the rail lets the leader through: it swallows every
// other unbound key, so ctrl+b could not otherwise start a chord from inside it.
func TestLeaderExploreTogglesRailFocus(t *testing.T) {
	prev := config.SidebarEnabled
	config.SidebarEnabled = true
	t.Cleanup(func() { config.SidebarEnabled = prev })

	leader := tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}
	explore := tea.KeyPressMsg{Code: 'e', Text: "e"}

	for _, mode := range []app.Mode{app.WindowManagementMode, app.TerminalMode} {
		o := twoPaneOS(t)
		o.Mode = mode

		o, _ = HandleKeyPress(leader, o)
		o, _ = HandleKeyPress(explore, o)
		if !o.SidebarFocused {
			t.Fatalf("ctrl+b e in mode %v did not focus the rail", mode)
		}

		o, _ = HandleKeyPress(leader, o)
		if !o.PrefixActive {
			t.Fatalf("the rail swallowed the leader in mode %v", mode)
		}
		o, _ = HandleKeyPress(explore, o)
		if o.SidebarFocused {
			t.Fatalf("ctrl+b e in mode %v did not leave the rail", mode)
		}
	}
}
