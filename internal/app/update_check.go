package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tonk/tuios/internal/config"
)

// tuios asks GitHub for the newest release now and then and, when it is newer
// than the build that is running, shows it at the right-hand end of the dock
// (appearance.check_for_updates). Clicking it opens the release page.
//
// The check keeps a clock of its own rather than riding the frame tick, which
// skips its work while the session is idle: an UpdateCheckDueMsg arrives every
// updateCheckPoll and starts a check when one is due, so the first comes
// shortly after startup and the next every updateCheckInterval. Polling also
// covers turning the option on in the settings page, which has no way to start
// a command itself. Nothing is checked over SSH or in tuios-web,
// whose host is the server rather than the user's own machine, nor by a build
// that cannot say which version it is.

// UpdateCheckDisableEnv turns the update check off however it is configured,
// for the cases where config is the wrong lever: a test suite, a CI job.
const UpdateCheckDisableEnv = "TUIOS_NO_UPDATE_CHECK"

// updateCheckURL is the newest release of this fork.
const updateCheckURL = "https://api.github.com/repos/tonk/tuios/releases/latest"

// updateCheckInterval is how long a successful check's answer stands. Releases
// are rare, and GitHub allows an unauthenticated address sixty requests an hour.
const updateCheckInterval = 12 * time.Hour

// updateCheckRetryInterval is how long a failed check waits before trying again.
// updateCheckedAt is stamped when a check starts, so without a shorter failure
// cooldown a flaky first request would suppress retries for the full success interval.
const updateCheckRetryInterval = 15 * time.Minute

// updateCheckFirst is how long after startup the first check waits, so it
// does not compete with restoring a session for the first frames.
const updateCheckFirst = 5 * time.Second

// updateCheckPoll is how often the check asks itself whether it is due. It
// costs a comparison, and bounds how long turning the option on takes to show.
const updateCheckPoll = time.Minute

// UpdateCheckDueMsg asks whether an update check is due.
type UpdateCheckDueMsg struct{}

// updateCheckDue delivers an UpdateCheckDueMsg after d.
func updateCheckDue(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return UpdateCheckDueMsg{} })
}

// updateCheckTimeout bounds one request, so a network that swallows packets
// does not keep a check in flight, and the next one from starting, forever.
const updateCheckTimeout = 10 * time.Second

// UpdateCheckMsg is the answer to one check: the newest release's tag and
// page, or the error that stopped the check.
type UpdateCheckMsg struct {
	Latest string
	URL    string
	Err    error
}

// fetchLatestRelease is what a check asks. A variable so tests can answer
// without the network.
var fetchLatestRelease = func(ctx context.Context, current string) (tag, page string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateCheckURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "tuios/"+current)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("release lookup: %s", resp.Status)
	}
	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", fmt.Errorf("release lookup: %w", err)
	}
	return release.TagName, release.HTMLURL, nil
}

// runningVersion is the version of this build, or "" when it cannot tell. A
// release or `make` build has one injected; a plain `go build` in a checkout
// does not, but Go records the module version from git in the binary, as a
// tag (v0.8.61) or a pseudo-version for commits after one.
func runningVersion() string {
	if v := versionLabel(); v != "" && v != "dev" {
		return v
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return ""
}

// maybeStartUpdateCheck returns the command for a check when one is due, and
// nil otherwise.
func (m *OS) maybeStartUpdateCheck(now time.Time) tea.Cmd {
	if !config.CheckForUpdates || m.IsSSHMode || m.IsWebSession || m.updateCheckRunning ||
		os.Getenv(UpdateCheckDisableEnv) != "" {
		return nil
	}
	if !m.updateCheckedAt.IsZero() && now.Sub(m.updateCheckedAt) < updateCheckInterval {
		return nil
	}
	current := runningVersion()
	if parseVersion(current) == nil {
		return nil
	}
	m.updateCheckRunning = true
	m.updateCheckedAt = now
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()
		tag, page, err := fetchLatestRelease(ctx, current)
		return UpdateCheckMsg{Latest: tag, URL: page, Err: err}
	}
}

// handleUpdateCheck records a check's answer. A failed check keeps what the
// last good one found: a network that is down for a moment has not taken a
// release away. It also rewinds updateCheckedAt so the next due poll can retry
// after updateCheckRetryInterval instead of waiting the full success interval.
func (m *OS) handleUpdateCheck(msg UpdateCheckMsg) {
	m.updateCheckRunning = false
	if msg.Err != nil {
		m.LogInfo("[UPDATE] check failed: %v", msg.Err)
		m.updateCheckedAt = time.Now().Add(updateCheckRetryInterval - updateCheckInterval)
		return
	}
	latest, url := "", ""
	if newerVersion(msg.Latest, runningVersion()) {
		latest, url = msg.Latest, msg.URL
	}
	if latest != m.UpdateAvailable {
		m.renderSkipped = false
		m.cachedViewContent = ""
	}
	m.UpdateAvailable, m.UpdateURL = latest, url
}

// UpdateNotice is the version the dock offers, or "" when there is none to
// offer or the option is off.
func (m *OS) UpdateNotice() string {
	if !config.CheckForUpdates {
		return ""
	}
	return m.UpdateAvailable
}

// OpenUpdatePage opens the release page of the version the dock offers, or
// says where it is when no browser can be started here.
func (m *OS) OpenUpdatePage() {
	if m.UpdateURL == "" {
		return
	}
	if m.CanStartBrowser() {
		m.OpenURL(m.UpdateURL, false)
		return
	}
	m.ShowNotification("tuios "+m.UpdateAvailable+": "+m.UpdateURL, "info", config.NotificationDuration*2)
}

// newerVersion reports whether latest is a later version than current. Either
// one failing to parse means no.
func newerVersion(latest, current string) bool {
	l, c := parseVersion(latest), parseVersion(current)
	if l == nil || c == nil {
		return false
	}
	return compareVersions(l, c) > 0
}

// parsedVersion is vMAJOR.MINOR.PATCH with an optional pre-release; build
// metadata (+dirty) is dropped, as semver says it does not order.
type parsedVersion struct {
	core [3]int
	pre  string
}

// parseVersion reads a semantic version with or without its leading "v", or
// returns nil.
func parseVersion(v string) *parsedVersion {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	var p parsedVersion
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v, p.pre = v[:i], v[i+1:]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil
	}
	for i, s := range parts {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return nil
		}
		p.core[i] = n
	}
	return &p
}

// compareVersions orders two versions the way semver does for the cases a
// release and a build of this repository produce: by number, and a release
// above a pre-release of the same number. Go's pseudo-versions are
// pre-releases of the next patch (v0.8.62-0.20261014...), so a build of a
// commit after v0.8.61 is newer than v0.8.61 and older than v0.8.62.
func compareVersions(a, b *parsedVersion) int {
	for i := range a.core {
		if a.core[i] != b.core[i] {
			if a.core[i] < b.core[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.pre == b.pre:
		return 0
	case a.pre == "":
		return 1
	case b.pre == "":
		return -1
	}
	return strings.Compare(a.pre, b.pre)
}

// updateNoticeText is the dock's update notice: the glyph and the version,
// padded a cell either side like the indicators beside it.
func updateNoticeText(version string) string {
	return " " + config.GetDockIndicatorUpdateGlyph() + " " + version + " "
}
