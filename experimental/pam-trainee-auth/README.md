# PAM trainee-auth

The "one `tuios-web` server, many trainees, each logged in as their own real
Unix account" idea, now wired into `tuios-web` itself behind `--pam-auth`
(off by default). Everything that needs root — PAM authentication, `setuid`
to an arbitrary trainee's uid/gid, spawning their shell — stays confined to
the small standalone privileged helper in this directory, in its own Go
module (`go.mod` here) so it never touches tuios's own module graph or
build. `tuios-web` only ever talks to it over a Unix socket as an
unprivileged client (`internal/pamauth` in the main module, pure Go, no cgo).

## Why a separate process

`tuios-web` runs entirely unprivileged. Spawning a shell as a different Unix
user needs root (to authenticate against `/etc/shadow`, and to `setuid`).
Rather than making the whole tuios daemon — the part that parses untrusted
network input — run as root, the privileged surface is confined to this one
small helper that does *only* PAM auth and process spawning, then hands off
file descriptors and gets out of the way:

- **`helper/`** — must run as root. Listens on a Unix socket
  (`/run/tuios-pam-helper.sock`, `internal/pamauth.DefaultSocketPath` on the
  `tuios-web` side). **One connection is one PAM login**: it authenticates a
  username/password once, then — for as long as that connection stays open —
  spawns as many shells as asked, each on its own fresh PTY, each running as
  the trainee's own uid/gid/groups. Every PTY master fd is sent to the caller
  via `SCM_RIGHTS` ancillary data (`internal/wire`), the same fd-passing
  mechanism tools like `sudo` use. Closing the connection is what ends the
  login: the helper signals every shell still running for it, then tears
  down the PAM session.
- **`internal/pamauth`** (main tuios module) — `tuios-web`'s client for the
  above. Never privileged, never touches PAM itself.
- **`client/`** (this module) — a minimal manual test client: log in, get a
  shell, type in it, confirm the identity, ctrl-`]` to detach and open
  another. Exists for trying the helper out on its own, independent of
  `tuios-web`.

## How it's wired into tuios-web

- **A front door in front of sip** (`cmd/tuios-web/pamfrontdoor.go`). This is
  the piece that makes a login prompt actually appear in the browser, and it
  exists for a specific reason: sip's own page-load auth hook (`checkAuth`,
  guarding `/`) only supports one fixed username/password pair via
  `sip.Config.BasicUsername`/`BasicPassword`, with no hook for a dynamic
  per-trainee check — and a browser's native Basic Auth popup only ever
  appears for a 401 on a plain HTTP request (the page load), never for one on
  a WebSocket handshake, which is the only place a `sip.ConnectMiddleware`
  alone can run. So in `--pam-auth` mode, `runWebServer` rebinds sip itself
  to a loopback-only internal address (an OS-assigned free port, probed via
  `probeLoopbackPort`) and instead runs its own `http.Server` on the address
  the user actually asked for, gating *every* request — page load, static
  assets, the WebSocket upgrade, all of it — with real PAM Basic Auth
  (`pamauth.Verify`, a check-only login: dial, authenticate, close
  immediately) before reverse-proxying it through. This is also why sip's
  own TLS handling gets bypassed for `--pam-auth`: the front door terminates
  TLS itself now, since it's the one holding the public-facing listener.
- `pamAuthMiddleware`, an `sip.ConnectMiddleware` (`cmd/tuios-web/pamauth.go`),
  still runs *inside* sip's (now-internal) instance, at the WebSocket
  handshake specifically. This is where the real, lasting session identity
  gets created: a browser that already passed the front door's gate has its
  credentials cached for the origin and attaches them automatically to the
  WebSocket upgrade too (this happens in the browser's network layer,
  invisible to the page's own JS, which has no way to attach an
  `Authorization` header to a `WebSocket` itself). On success the resulting
  `*pamauth.Login` rides into the session via `sip.WithIdentity`, the same
  context-carrying mechanism `touchMiddleware` already uses for touch-device
  detection.
- `createTUIOSHandler` (`cmd/tuios-web/main.go`) checks for that identity
  before falling through to the normal ephemeral/daemon-backed paths, and
  when present builds an ordinary local `OS` instance with `OSOptions.PAMLogin`
  set instead.
- `OS.PAMLogin` (`internal/app/os.go`) is threaded into `AddWindow`
  (`internal/app/os_window.go`): when set, **every** window — the first one
  opened automatically and every later "new window" — is spawned by calling
  `PAMLogin.SpawnPTY` instead of the normal local shell spawn, so a trainee's
  whole session runs as their own account, not just its first window. Closing
  a window calls `PAMLogin.ClosePTY` on that window's pid through the same
  connection.
- `internal/terminal.NewAdoptedWindow` (`internal/terminal/window.go`) is the
  actual adoption point: it builds a `Window` exactly like `NewWindow` does
  (same VT emulator, same theme/callback wiring — both now share
  `newWindowSkeleton`), except it wraps a PTY fd that something else already
  started rather than calling `exec.Command` itself. Since there is no local
  `*exec.Cmd` to `Wait()` on a process this Go process never forked, exit is
  detected by polling the reported pid's liveness instead
  (`waitForAdoptedExit`).

Not touched at all: `internal/session`'s daemon/wire protocol
(`internal/session/protocol.go`, `daemon_handlers.go`). A PAM-authenticated
session is deliberately scoped to the same local (non-daemon) code path
`--ephemeral` mode already uses, so it doesn't persist across a `tuios-web`
restart — the same limitation `--ephemeral` already has, for the same reason:
there's no separate daemon process for either kind of session to live in
independently of the one connection that created it.

**A side effect of the front door worth knowing about**: sip generates a
self-signed certificate and starts a WebTransport listener automatically for
any loopback bind, including the now-internal one — but that listener is
unreachable (it's not something the front door proxies, and QUIC/UDP
proxying is a different, harder problem than the HTTP/WebSocket proxying
implemented here). A browser will briefly try WebTransport, fail to reach
it, and fall back to WebSocket — the same fallback path sip already uses
whenever WebTransport isn't available for any other reason, just triggered
here by an unreachable rather than absent endpoint. Effect: a short
(sub-second, in testing) connection delay on first load, not a broken
connection.

## Trying it

Needs `libpam0g-dev` (or your distro's PAM headers) to build the helper,
since `github.com/msteinert/pam/v2` is a cgo binding. `tuios-web` itself
needs no new system dependency — `internal/pamauth` is pure Go.

```sh
# from experimental/pam-trainee-auth
go build -o pam-helper ./helper
sudo cp pam.d/tuios-web /etc/pam.d/tuios-web
sudo ./pam-helper &      # must be root

# from the repo root
go build -o tuios-web ./cmd/tuios-web
./tuios-web --pam-auth --port 7681
```

Open `http://localhost:7681`, and the browser's own native login prompt
(HTTP Basic Auth) asks for a real local account's username and password —
right on page load, before the terminal UI even appears, since that's what
the front door in `pamfrontdoor.go` gates. On success you land in a real
shell running as that account.

The standalone `client/` (log in, get a shell, no browser needed) still
works the same way for quick manual checks:

```sh
go build -o pam-client ./client
./pam-client
# username: <a real local account>
# password: <its password>
# press enter to open a shell, ctrl-] to close it, enter again for another
```

### What was actually verified (not just compiled)

Tested end to end against disposable local accounts (`useradd -m`, the PAM
service file above, no LDAP/SSSD — matches the target training environment),
through the real `tuios-web` binary and a real WebSocket connection, not
just the standalone helper/client:

- **Front-door auth gate**: `GET /` with no credentials and with a wrong
  password both got a clean 401 with `WWW-Authenticate: Basic
  realm="tuios-web"` directly from the public port — the actual thing that
  makes a browser show its native login popup — before ever reaching sip's
  internal instance; a correct login proxied through to the real
  `index.html` (200, correct content type).
- **Full web session through the proxy**: connected over WebSocket with HTTP
  Basic Auth to the front door, opened a window, typed into it, and got back
  `whoami`/`id`/`$HOME` all resolved to the trainee account — through actual
  tuios VT rendering and the reverse proxy, not a direct connection to sip's
  internal instance.
- **Independent multi-spawn** (the reason the protocol became persistent
  rather than one-shot): two `SpawnPTY` calls on the same login produced two
  different pids, both correctly isolated as the trainee's account — this is
  exactly what `AddWindow` does for a session's first window vs. every later
  one.
- **`ClosePTY`**: signaling a specific pid actually terminated it, verified
  by `ps -p` afterward, without disturbing the login's other still-running
  shell.
- **Disconnect cleanup**: closing the connection with a shell still running
  and never explicitly closed still killed it, and the helper's log recorded
  the PAM session ending (`CloseSession`/`SetCred(DeleteCred)` running).
- `pam_systemd.so` registered a real `loginctl`-visible session for the
  trainee's uid, confirming `pam_open_session` actually ran, not just
  `pam_authenticate`.

## What's still not here

- **Wire protocol security.** The helper/tuios-web protocol sends the
  password in the clear over a local socket with no authentication of the
  *caller* beyond "can connect to the socket" (it's chmod 0666). That's fine
  for a same-host trust boundary; `tuios-web`'s browser-facing side (TLS,
  rate limiting, lockout) is a separate concern layered on top by `--pam-auth`
  itself, not by the helper.
- **Reattach across a tuios-web restart.** As above: a PAM session lives only
  as long as the one `tuios-web` process and connection that created it,
  same as `--ephemeral` mode. Reattaching would mean routing the adopted PTY
  through `internal/session`'s daemon protocol instead, a materially bigger
  change deliberately left out of this pass.
- **Account lifecycle.** Local accounts mean course accounts are provisioned
  by hand today (`useradd` per trainee); nothing here automates that.
- **Resource limits / isolation beyond uid separation.** `pam_systemd` gives
  each trainee a cgroup via logind, which is a reasonable starting point, but
  nothing here enforces CPU/memory caps or restricts what a trainee's shell
  can reach on the network — worth deciding deliberately for a training
  environment before this goes anywhere near production.
- **The helper's own attack surface.** It's small and does one thing, but
  it's still a root-listening Unix socket reachable by any local user; a real
  deployment should audit it as carefully as `sudo` or `su` would be.
