# Changes Since the Fork

This fork started from [Gaurav-Gosain/tuios](https://github.com/Gaurav-Gosain/tuios)
at its `init` commit and has since diverged with changes specific to this
deployment (a Unix training lab run by AT Computing). This document lists
everything added or changed here, grouped by theme, in the order it happened.
It does not repeat anything from upstream's own history or from the
"What's New" sections in the main [README](../README.md) that predate the
fork. It also omits a handful of early, purely bootstrapping commits (CI
secret configuration, "my own version" checkpoints while getting a first
release pipeline running) that carry no independent technical content.

## Classroom trainer console (`tuios-web`, PAM mode)

The largest single feature: a trainer can log in through `tuios-web`, see a
live list of trainee accounts matching a configured pattern (e.g.
`guru[0-9][0-9]`), and attach to any of their sessions to view and type into
them alongside the trainee - scoped to `tuios-web` only (no SSH support).

- **PAM authentication for `tuios-web`**: a privileged helper process
  (`tuios-pam-helper`) authenticates a username/password pair against PAM and
  spawns shells as that trainee's own Unix account, reached over a Unix
  socket via SCM_RIGHTS fd-passing - `tuios-web` itself never runs as root.
  Gated behind `--pam-auth` (off by default). A front-door HTTP layer
  (`pamfrontdoor.go`) makes the browser's native Basic Auth prompt actually
  appear (a `sip.ConnectMiddleware` alone never triggers one), and also gates
  `/tuios-settings/*`, not just `/`.
- **`[classroom]` config**: `trainer_console` (off by default),
  `trainer_users` (the access-control allowlist), `trainee_pattern` (regex a
  requested target must match). Predicates: `ClassroomConfig.IsTrainer`,
  `.MatchesTrainee`.
- **Daemon-side PTY adoption**: `Session.AdoptPTY`/`AdoptDaemonWindow` let the
  daemon host a window around a PTY it did not spawn itself.
  `Session.ClassroomSpawner` holds a trainee's live PAM login and spawns
  every window that session ever opens through it, for the session's whole
  life - installed exactly once per session, never replaced while in use.
- **Login-handoff socket**: a second Unix socket
  (`internal/session/daemon_classroom.go`) `tuios-web` dials once per
  PAM-authenticated connection to hand the daemon a trainee's login fd
  (`SendClassroomLogin`); the daemon reconstructs it
  (`pamauth.NewLoginFromFile`) and creates/attaches the named session.
  `cmd/tuios-web` always sends this handoff, even when a session may already
  exist - "already exists" is not the same as "already has a live spawner"
  (see the resurrection fix below), and the daemon safely no-ops a redundant
  handoff against a session with a spawner already in use.
- **Trainer picker UI**: an authorized trainer with no specific target lands
  on a live, pattern-filtered, self-excluding, auto-refreshing list of
  trainee sessions (arrow keys + enter to attach), with a fixed "My own
  session" entry always available. `?attach=<username>` still works directly
  for a specific target, authorized server-side the same way, with a generic
  401 on denial regardless of which check tripped.
- **Real bugs found and fixed during rollout**, each confirmed live:
  - A trainer's login could accidentally create a session under their own
    account while labeled as the trainee's - split into a
    create-or-attach path used only for a trainee's own session and an
    attach-only path for a trainer viewing someone else's.
  - Only the first window of a classroom session routed through the PAM
    spawner; a second window (e.g. after the first shell exited) silently
    fell back to spawning as `tuios-web`'s own account. Fixed by making
    `Session.CreatePTY` itself the single classroom-aware dispatch point.
  - A classroom shell died silently within seconds of creation: a real,
    timing-dependent fd-reuse race in the login-handoff path (closing a
    received fd froze its number for reuse by something else in the
    daemon moments before the adopted PTY's own fd needed a fresh one).
    Took two attempts to fix for real - see `pamauth.Login.origFile`'s own
    doc comment for the final shape.
  - A session that survived a daemon restart came back via ordinary
    resurrection (no live PAM login exists yet at daemon startup), so it
    ran as the daemon's own account forever, even after the trainee
    reconnected. Fixed by detecting a resurrected-but-spawnerless session
    on the next handoff and tearing it down before recreating it fresh.
  - A session's single window, freshly created with nobody attached yet,
    got placed at half its real size and never retiled to fill the
    screen - stuck that way permanently once persisted. Fixed by folding a
    newly-placed window back into a full retile on the very first attach,
    the same way a live sync already did.
  - A session's shared terminal size (minimum across every attached
    client) never shrank back up after a client disconnected without an
    explicit detach (an ordinary browser reload) - nothing recalculated it
    except another client joining or leaving, and this was the one
    ordinary way a client leaves that never counted as either.
  - A single page load is nine-plus HTTP requests (stylesheets, fonts,
    scripts, the page itself, then the WebSocket upgrade), each re-running
    a full PAM login even though they all carry the same credentials - a
    real, measurable slice of connect time. Fixed with a short-lived
    (10s) cache of already-verified credentials.
- Packaging: a `tuios-daemon.service` unit (the classroom daemon needs to run
  alongside `tuios-web`, same account, same `XDG_RUNTIME_DIR`) and a
  `tuios-web-mirror.service` template for a read-only presenter-mirror
  deployment pattern, both documented in `docs/DEPLOYMENT.md`.

## PAM authentication groundwork (pre-classroom)

The prototype the classroom console builds on:

- Added the PAM trainee-auth prototype (privileged helper + fd-passing),
  wired it into `tuios-web` behind `--pam-auth`, then renamed
  `experimental/pam-trainee-auth` to `pam-helper` and added it to the build,
  packaging and release pipeline.
- A PAM-spawned window's `{user}` title placeholder now shows the trainee,
  not the `tuios-web` service account; PAM-spawned shells also get
  `TUIOS_ENV=1` like every other spawn path.

## `tuios-web` theming, fonts and appearance

- `--web-settings`: an optional in-browser theme/font picker, plus a
  title-lock toggle so a guest app can't overwrite a window's title, and
  config options for the default title-lock state and initial title text.
- Fixed the picker's theme switch leaving the terminal background stuck, and
  its font switch never actually changing the rendered font.
- Bundled three additional selectable web fonts: SauceCodePro Nerd Font Mono,
  SauceCodePro NFM SemiBold, FreeMono and Source Code Pro (themes can pin a
  font/size).
- Added bundled themes: "training" (later renamed "trainer", statusbar
  color matched to midnight, font size bumped 24px→26px) and "trainee" (a
  smaller-font variant, `tuios-web`'s default).
- `--config`, `--font-family` and `--font-path` flags for `tuios-web`;
  `[appearance]` config now applies there too. Web terminal defaults changed:
  Copy on select and Blink cursor on by default.

## Configuration and environment

- Added an `[env]` config table for extra environment variables.
- Fixed a config-watcher rename gap and a style-cache evict counter; removed
  the now-dead style-cache API (`docs/STYLE_CACHE.md` updated to match).

## Terminal emulation and UI fixes

- Fixed OSC 66 shredding Cursor's and OpenTUI's output.
- Handle CSI u (SCORC) to restore the cursor CSI s saved.
- Fixed the maximize button for zoomed and `MaximizeNewWindows` windows.
- The host terminal's own title and bell now follow the focused pane.
- No tiling forced at startup.
- Several rounds of tiling/focus/cursor/keybinding-display fixes, plus
  per-element theme overrides, character indicators, a pill-underline
  toggle, configurable scrollbar indicator colors, a configurable clock
  (format, position, color, optional week number), a sidebar/splitting fix,
  and a dedicated dock indicator for implicit copy mode.

## Lua tape scripting

- Added Lua scripting for tapes, `.tuios.tape.lua` project-tape autorun
  support, and extended it with workspace/session/agent-state verbs and
  structured queries.

## Packaging, deployment and CI

- A Forgejo release pipeline (Containerfile, nfpm packaging, Makefile
  targets), later simplified to a single tag-triggered binary build per
  platform, running self-sufficiently inside the runner's own job container
  with nfpm pinned to a specific version instead of `@latest`.
- Fixed the tape directory path (moved under config, not data), and pointed
  `install*.sh`/doc links at this fork instead of upstream.
- `scripts/download-latest-release.sh` for an easier manual download, later
  superseded by `deploy/get_tuios`, an update script for `tuios`/`tuios-web`
  that fetches `.deb`/`.rpm` on Debian/RHEL and a raw binary everywhere else.
- Packaging fixes: `.deb`/`.rpm` systemd units installed to
  `/etc/systemd/system` (not `/usr/lib`); `tuios-web`'s `XDG_CONFIG_HOME`
  pinned next to its own `config.toml` instead of under `HOME`;
  `/etc/tuios-web/config.toml.example` shipped in the package.
- systemd units, an nginx reverse-proxy config and a deployment guide
  (`docs/DEPLOYMENT.md`); the build version now shows on the zero-windows
  welcome screen.
- Renamed this fork's Go module path from `Gaurav-Gosain/tuios` to
  `tonk/tuios`; fixed the README's clone-origin URL.
