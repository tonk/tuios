# PAM trainee-auth prototype

A standalone proof of concept for the "one `tuios-web` daemon, many trainees,
each logged in as their own real Unix account" idea. It lives in its own Go
module (`go.mod` in this directory) precisely so it never touches tuios's own
module graph — this whole directory, or the branch it lives on, can be
deleted with zero footprint on the main project.

**This is not wired into tuios.** It is two standalone binaries that prove
the security-sensitive mechanics work: PAM authentication against local
`/etc/shadow` accounts, dropping privilege to an arbitrary trainee's uid/gid,
and handing a live PTY across a process boundary to code that never itself
runs privileged.

## Why two processes

`tuios-web` today runs entirely unprivileged. Spawning a shell as a different
Unix user needs root (to authenticate against shadow, and to `setuid`).
Rather than making the whole tuios daemon — the part that parses untrusted
network input — run as root, the privileged surface is confined to a small
helper that does *only* PAM auth and process spawning, then hands off a file
descriptor and gets out of the way:

- **`helper/`** — must run as root. Listens on a Unix socket
  (`/run/tuios-pam-poc.sock`). For each connection: reads a username and
  password, runs the full PAM login sequence, and on success starts that
  user's shell on a fresh PTY running as their own uid/gid/groups. The PTY
  master fd is sent to the caller via `SCM_RIGHTS` ancillary data
  (`internal/wire`) — the same fd-passing mechanism tools like `sudo` use.
  The helper itself never touches the shell's stdin/stdout again once the fd
  has crossed the socket.
- **`client/`** — never privileged, never touches PAM. Connects, sends
  credentials, and on success receives a PTY fd it can read/write freely
  (fd ownership isn't gated by which uid the process on the other end is
  running as — only the original `open()` was). It attaches as a minimal
  interactive terminal so you can drive the shell directly.

In a real integration, `client/` is replaced by `tuios-web`'s own daemon
(`internal/session`): it would dial the helper the same way, and use the
returned fd exactly where it currently uses the fd from its own PTY-spawning
code, keyed by the authenticated username instead of the hardcoded `"web"`
session name.

## Trying it

Needs `libpam0g-dev` (or your distro's PAM headers) to build, since
`github.com/msteinert/pam/v2` is a cgo binding.

```sh
go build -o pam-helper ./helper
go build -o pam-client ./client

sudo cp pam.d/tuios-web /etc/pam.d/tuios-web
sudo ./pam-helper &      # must be root

./pam-client              # ordinary user, no special privilege
# username: <a real local account>
# password: <its password>
```

On success you land in a real shell running as that account — `whoami`,
`id`, `echo $HOME` all show the trainee's own identity, not the account
`pam-helper` or `pam-client` were started as. Ctrl-`]` detaches.

### What was actually verified (not just compiled)

Tested end to end against a disposable local account (`useradd -m`, PAM
service file above, no LDAP/SSSD — matches the target training environment):

- Correct login: `whoami`/`id`/`$HOME`/`$SHELL`/`pwd` all resolved to the
  trainee account, with the right uid, gid, and supplementary groups.
- `pam_systemd.so` registered a real `loginctl`-visible session for that
  uid — confirms `pam_open_session` actually ran, not just `pam_authenticate`.
- A wrong password is rejected cleanly (`pam_authenticate` fails, no fd is
  ever sent, the helper logs the failure and moves on).
- `pam_close_session` / `SetCred(DeleteCred)` run when the shell exits (see
  the helper's log line `session for %q ended`).

## What a real integration would still need

This prototype deliberately skips everything that isn't the core mechanic:

- **Wire protocol security.** The client/helper protocol here sends the
  password in the clear over a local socket with no authentication of the
  *caller* beyond "can connect to the socket" (it's chmod 0666). That's fine
  for a same-host trust boundary proof; `tuios-web`'s actual browser-facing
  auth (the HTTP/WebSocket layer, TLS, rate limiting, lockout) is a separate
  concern layered on top, not solved here.
- **Reattach / session identity.** Wiring the returned fd into
  `internal/session`'s existing multi-session daemon, keyed by the
  authenticated username instead of `--default-session`, so a trainee
  reconnecting (browser refresh, different device) reattaches to their
  existing session instead of getting a second one.
- **Account lifecycle.** Local accounts mean course accounts are provisioned
  by hand today (`useradd` per trainee); nothing here automates that.
- **Resource limits / isolation beyond uid separation.** `pam_systemd`
  gives each trainee a cgroup via logind, which is a reasonable starting
  point, but nothing here enforces CPU/memory caps or restricts what a
  trainee's shell can reach on the network — worth deciding deliberately
  for a training environment before this goes anywhere near production.
- **The helper's own attack surface.** It's small and does one thing, but
  it's still a root-listening Unix socket reachable by any local user; a
  real deployment should audit it as carefully as `sudo` or `su` would be.
