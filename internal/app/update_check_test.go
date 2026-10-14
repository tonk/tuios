package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tonk/tuios/internal/config"
)

func TestNewerVersion(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"v0.8.62", "v0.8.61", true},
		{"v0.9.0", "v0.8.61", true},
		{"v1.0.0", "v0.99.99", true},
		{"v0.8.61", "v0.8.61", false},
		{"v0.8.60", "v0.8.61", false},
		{"v0.8.10", "v0.8.9", true}, // numeric, not string, order
		{"0.8.62", "v0.8.61", true}, // goreleaser's tag without a "v"
		{"v0.8.61", "v0.8.61+dirty", false},
		// A plain go build of a commit after v0.8.61 is a pseudo-version,
		// a pre-release of v0.8.62.
		{"v0.8.61", "v0.8.62-0.20261014092156-56ca623abcde", false},
		{"v0.8.62", "v0.8.62-0.20261014092156-56ca623abcde", true},
		{"v0.8.62", "dev", false},
		{"", "v0.8.61", false},
		{"nonsense", "v0.8.61", false},
	}
	for _, tt := range tests {
		if got := newerVersion(tt.latest, tt.current); got != tt.want {
			t.Errorf("newerVersion(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

// withUpdateCheck runs a test as a build of version current with the check
// on, answering it with answer instead of the network.
func withUpdateCheck(t *testing.T, current string, answer func() (string, string, error)) {
	t.Helper()
	t.Setenv(UpdateCheckDisableEnv, "")
	prevVersion, prevOption, prevFetch := Version, config.CheckForUpdates, fetchLatestRelease
	t.Cleanup(func() { Version, config.CheckForUpdates, fetchLatestRelease = prevVersion, prevOption, prevFetch })
	Version, config.CheckForUpdates = current, true
	fetchLatestRelease = func(context.Context, string) (string, string, error) { return answer() }
}

// runCheck starts a check at now and feeds its answer back, as Update does.
func runCheck(t *testing.T, m *OS, now time.Time) bool {
	t.Helper()
	cmd := m.maybeStartUpdateCheck(now)
	if cmd == nil {
		return false
	}
	m.handleUpdateCheck(cmd().(UpdateCheckMsg))
	return true
}

func TestUpdateCheckFindsANewerRelease(t *testing.T) {
	withUpdateCheck(t, "v0.8.61", func() (string, string, error) {
		return "v0.8.62", "https://github.com/tonk/tuios/releases/tag/v0.8.62", nil
	})
	m := &OS{}
	now := time.Now()

	if !runCheck(t, m, now) {
		t.Fatal("the first tick started no check")
	}
	if m.UpdateNotice() != "v0.8.62" || !strings.HasSuffix(m.UpdateURL, "/v0.8.62") {
		t.Fatalf("notice = %q, url = %q", m.UpdateNotice(), m.UpdateURL)
	}

	if runCheck(t, m, now.Add(time.Hour)) {
		t.Error("checked again an hour later; the answer stands for 12 hours")
	}
	if !runCheck(t, m, now.Add(updateCheckInterval)) {
		t.Error("no check once the interval had passed")
	}

	config.CheckForUpdates = false
	if m.UpdateNotice() != "" {
		t.Error("the notice stayed with the option off")
	}
}

func TestUpdateCheckCurrentOrFailed(t *testing.T) {
	latest, err := "v0.8.61", error(nil)
	withUpdateCheck(t, "v0.8.61", func() (string, string, error) { return latest, "u", err })
	m := &OS{}
	now := time.Now()

	runCheck(t, m, now)
	if m.UpdateNotice() != "" {
		t.Fatalf("the running release was offered as an update: %q", m.UpdateNotice())
	}

	// A release appears, then the network goes away: the release stays.
	latest = "v0.8.62"
	runCheck(t, m, now.Add(updateCheckInterval))
	err = errors.New("offline")
	runCheck(t, m, now.Add(2*updateCheckInterval))
	if m.UpdateNotice() != "v0.8.62" {
		t.Errorf("a failed check dropped the release found before: %q", m.UpdateNotice())
	}
}

func TestUpdateCheckDoesNotRun(t *testing.T) {
	answer := func() (string, string, error) { return "v9.9.9", "u", nil }
	tests := []struct {
		name    string
		version string
		setup   func(t *testing.T, m *OS)
	}{
		{"option off", "v0.8.61", func(*testing.T, *OS) { config.CheckForUpdates = false }},
		{"ssh", "v0.8.61", func(_ *testing.T, m *OS) { m.IsSSHMode = true }},
		{"web", "v0.8.61", func(_ *testing.T, m *OS) { m.IsWebSession = true }},
		{"disabled by environment", "v0.8.61", func(t *testing.T, _ *OS) { t.Setenv(UpdateCheckDisableEnv, "1") }},
		{"version unknown", "dev", func(*testing.T, *OS) {}},
		{"already running", "v0.8.61", func(_ *testing.T, m *OS) { m.updateCheckRunning = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withUpdateCheck(t, tt.version, answer)
			m := &OS{}
			tt.setup(t, m)
			if m.maybeStartUpdateCheck(time.Now()) != nil {
				t.Error("a check was started")
			}
		})
	}
}

// The notice is drawn at the right of the dock and is the dock's hit target
// for the release page.
func TestDockShowsTheUpdateNotice(t *testing.T) {
	withUpdateCheck(t, "v0.8.61", func() (string, string, error) { return "v0.8.62", "u", nil })
	m := NewOS(OSOptions{UserConfig: config.DefaultConfig()})
	t.Cleanup(func() { closeWindows(m) })
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	runCheck(t, m, time.Now())

	m.View()
	frame := ansi.Strip(m.cachedViewContent)
	lines := strings.Split(frame, "\n")
	dock := lines[len(lines)-1]
	if !strings.Contains(dock, "v0.8.62") {
		t.Fatalf("the dock does not show the update:\n%s", dock)
	}
	x := strings.Index(dock, "v0.8.62")
	x = ansi.StringWidth(dock[:x])
	if got := m.DockIndicatorAt(x, len(lines)-1); got != DockIndicatorUpdate {
		t.Errorf("the notice is not its own hit target: got %v", got)
	}
}

// The due message starts a check and always schedules the next one, whether or
// not a check was due, so the clock never stops.
func TestUpdateCheckDueKeepsTheClockRunning(t *testing.T) {
	withUpdateCheck(t, "v0.8.61", func() (string, string, error) { return "v0.8.62", "u", nil })
	m := &OS{}
	if _, cmd := m.Update(UpdateCheckDueMsg{}); cmd == nil {
		t.Fatal("a due message returned nothing")
	}
	if !m.updateCheckRunning {
		t.Error("a due message with no check yet did not start one")
	}

	config.CheckForUpdates = false
	if _, cmd := m.Update(UpdateCheckDueMsg{}); cmd == nil {
		t.Error("with the option off the clock stopped, so turning it on would never check")
	}
}
