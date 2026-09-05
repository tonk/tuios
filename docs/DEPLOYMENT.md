# Production Deployment (tuios-web)

A from-scratch walkthrough for running `tuios-web` as a real, unattended
service behind a domain name: systemd units for `tuios-web` and (optionally)
its `tuios-pam-helper` companion, an nginx reverse proxy with TLS, and how
the two deployment shapes - a single shared terminal, or PAM-authenticated
multi-tenant logins - differ.

This doc assumes you already understand *why* `--pam-auth` exists and how
it's wired together; see [pam-helper/README.md](../pam-helper/README.md) for
that. This doc is the "how do I actually keep this running" half.

## Table of Contents

- [Choosing a shape](#choosing-a-shape)
- [Prerequisites](#prerequisites)
- [1. Build and install the binaries](#1-build-and-install-the-binaries)
- [2. Create the tuios-web service account](#2-create-the-tuios-web-service-account)
- [3. Write tuios-web's config file](#3-write-tuios-webs-config-file)
- [4. PAM multi-tenant mode only: set up tuios-pam-helper](#4-pam-multi-tenant-mode-only-set-up-tuios-pam-helper)
- [5. Install and start the systemd services](#5-install-and-start-the-systemd-services)
- [6. nginx reverse proxy and TLS](#6-nginx-reverse-proxy-and-tls)
- [7. Verifying the deployment](#7-verifying-the-deployment)
- [Presenter mirror session (optional)](#presenter-mirror-session-optional)
- [Trainer console (optional)](#trainer-console-optional)
- [Hardening notes](#hardening-notes)
- [Troubleshooting](#troubleshooting)
- [Related Documentation](#related-documentation)

---

## Choosing a shape

Two deployment shapes, and everything below applies to both except where
called out:

- **Single shared terminal.** Everyone who reaches the URL gets the same
  session (or their own ephemeral one), running as the one unprivileged
  `tuios-web` service account. No PAM, no `tuios-pam-helper`. This is the
  default `tuios-web` flags with nothing extra.
- **PAM multi-tenant ("classroom").** Every connection gets a real login
  prompt (a browser's native HTTP Basic Auth popup) and, once authenticated,
  a shell running as *that person's own* Unix account - not the `tuios-web`
  service account. This needs the separate, root-run `tuios-pam-helper`
  daemon too. See [pam-helper/README.md](../pam-helper/README.md) for the
  full design; the short version is that all privileged work (PAM auth,
  `setuid`) is confined to that one small helper process, reached over a
  local Unix socket, so `tuios-web` itself never runs as root.

Pick one before starting; step 4 and the `--pam-auth` flag in step 5 are the
only places they diverge.

## Prerequisites

- A Linux host with systemd (these units use `Type=simple`,
  `StateDirectory=`, `Restart=` - all standard since systemd 235+, which
  every currently-supported distro has).
- Go 1.25+ to build from source, **or** a released binary (see
  [Installation](WEB.md#installation) in docs/WEB.md) - either works for
  step 1.
- nginx, if you want a real domain name and TLS in front of `tuios-web`
  (recommended for anything reachable outside `localhost`). `tuios-web` can
  also terminate its own TLS with `--auto-tls`/`--cert`+`--key`, covered in
  [docs/WEB.md's Security section](WEB.md#security) - use that instead of
  nginx if you have no domain name to put a real certificate on, e.g. a
  LAN-only classroom deployment reached by IP address.
- PAM multi-tenant mode only: `libpam0g-dev` (Debian/Ubuntu) or
  `pam-devel` (Fedora/RHEL) to build `tuios-pam-helper` - it's a cgo binding
  to the system's real PAM stack (`github.com/msteinert/pam/v2`).

## 1. Build and install the binaries

From the repo root, using the Makefile (recommended - it stamps a real
version/commit/date into the binary via `-ldflags`, matching what a release
build would produce):

```bash
make install               # tuios + tuios-web -> /usr/local/bin
make install-pam-helper    # PAM multi-tenant mode only; needs libpam0g-dev
                            # (or pam-devel) -> /usr/local/bin/tuios-pam-helper
                            # + /etc/pam.d/tuios-web
```

Or from a GitHub/Forgejo release, without a Go toolchain at all:

```bash
curl -fsSL https://raw.githubusercontent.com/tonk/tuios/main/install-web.sh | bash
curl -fsSL https://raw.githubusercontent.com/tonk/tuios/main/install-pam-helper.sh | bash  # PAM mode only
```

Both install scripts fetch a platform-matched release asset and drop it in
`/usr/local/bin` (falling back to `~/.local/bin` if that's not writable -
make sure it actually landed in `/usr/local/bin/tuios-web`, since that's the
path the systemd units in step 5 hardcode).

## 2. Create the tuios-web service account

`tuios-web` runs entirely unprivileged - even in PAM mode, where the real
privileged work happens in `tuios-pam-helper` instead (see
[pam-helper/README.md](../pam-helper/README.md)). Give it its own system
account rather than running it as your own user or as root:

```bash
sudo useradd --system --create-home --home-dir /var/lib/tuios-web \
    --shell /usr/sbin/nologin tuios-web
sudo mkdir -p /etc/tuios-web
sudo chown tuios-web:tuios-web /etc/tuios-web
```

Owned by `tuios-web`, not just its contents: the unit file (step 5) pins
`XDG_CONFIG_HOME=/etc/tuios-web`, and `tuios-web` creates
`tuios/{themes,layouts,tapes}/` under it itself the first time each is
needed - it needs write access to the directory itself for that, not just
to files already in it.

`packaging/systemd/tuios-web.service` (installed in step 5) also declares
`StateDirectory=tuios-web`, which makes systemd create and own
`/var/lib/tuios-web` (mode `0750`) itself on every start - the `useradd
--create-home` above is mainly so the account has a sane `$HOME` to point at
before that, and matters if you skip the unit file and run `tuios-web`
some other way.

## 3. Write tuios-web's config file

The unit file passes `--config /etc/tuios-web/config.toml` explicitly,
rather than relying on XDG defaults under the service account's `$HOME` -
easier to find, easier to keep under version control alongside the rest of
your infrastructure config. `tuios-web` reads the same `config.toml` format
as the main `tuios` binary - there's no separate `tuios-web config`
command.

Installed via the `.deb`/`.rpm` package instead of step 1's `make
install`/`install-web.sh` (grab one from the releases page and `dpkg -i`/
`rpm -i` it)? It already shipped an annotated
`/etc/tuios-web/config.toml.example` - never read by `tuios-web` itself,
just a reference to copy from:

```bash
sudo cp /etc/tuios-web/config.toml.example /etc/tuios-web/config.toml
sudo chown tuios-web:tuios-web /etc/tuios-web/config.toml
sudo -e /etc/tuios-web/config.toml
```

Otherwise there's no packaged example to copy - generate the same
reference yourself with `tuios`'s own `config example` subcommand (this is
exactly what the package generates at build time, so the result is
identical):

```bash
tuios config example > /tmp/config.toml.example
sudo cp /tmp/config.toml.example /etc/tuios-web/config.toml
sudo chown tuios-web:tuios-web /etc/tuios-web/config.toml
sudo -e /etc/tuios-web/config.toml
```

See [docs/CONFIGURATION.md](CONFIGURATION.md) for every option. A few worth
knowing about for a shared/public deployment specifically:

- `[appearance] lock_titles = true` and `initial_title_format` - see
  [docs/CONFIGURATION.md](CONFIGURATION.md#lock_titles) - stop a guest shell
  from renaming its own pane, useful when several people are looking at the
  same screen or a recording.
- Any theme referenced by `theme = "..."` (including a custom one) needs to
  exist before `tuios-web` starts, or it falls back to the default with a
  logged warning. A custom theme is a `.toml`/`.json` file in
  `packaging/systemd/tuios-web.service`'s `XDG_CONFIG_HOME` - by default
  `/etc/tuios-web/tuios/themes/`, next to `config.toml` (not
  `~/.config/tuios/themes/`; that would only apply if this unit didn't pin
  `XDG_CONFIG_HOME`, and would then depend on whatever `HOME` happens to be
  for this service account).

## 4. PAM multi-tenant mode only: set up tuios-pam-helper

Skip this whole section for the single-shared-terminal shape.

**The PAM service file.** `make install-pam-helper` (step 1) already
installed a starter `/etc/pam.d/tuios-web`
(from [`pam-helper/pam.d/tuios-web`](../pam-helper/pam.d/tuios-web)):

```
auth     required   pam_unix.so
account  required   pam_unix.so
session  required   pam_unix.so
session  optional   pam_systemd.so
```

This authenticates against local Unix accounts (`/etc/shadow`) with no
LDAP/SSSD - matches a typical disposable-training-account setup. Swap in
your site's real PAM stack here if you need one (LDAP, SSSD, 2FA, whatever
your `/etc/pam.d/sshd` or `/etc/pam.d/login` already does) - `tuios-web`
never sees this file directly, only `tuios-pam-helper` does.

**The trainee accounts themselves.** Nothing here provisions them; that's
still `useradd -m <username>` per person, same as any other local account.
`tuios-pam-helper` only authenticates and spawns shells for accounts that
already exist.

**The helper only ever needs root.** Its systemd unit
(`packaging/systemd/tuios-pam-helper.service`, installed in step 5)
deliberately sets no `User=`/`Group=` - the binary itself refuses to start
under a non-root `euid`, since it needs to authenticate against
`/etc/shadow` and `setuid()` to whichever trainee account just logged in.

## 5. Install and start the systemd services

```bash
sudo cp packaging/systemd/tuios-web.service /etc/systemd/system/
sudo cp packaging/systemd/tuios-pam-helper.service /etc/systemd/system/  # PAM mode only
sudo systemctl daemon-reload
```

**PAM mode only** - edit `tuios-web.service`'s `ExecStart=` line to add
`--pam-auth`, and uncomment the `Wants=`/`After=` pair at the top of the
`[Unit]` section so `tuios-web` doesn't come up before the helper does:

```bash
sudo systemctl edit --full tuios-web.service
```

```diff
 [Unit]
-#Wants=tuios-pam-helper.service
-#After=tuios-pam-helper.service
+Wants=tuios-pam-helper.service
+After=tuios-pam-helper.service

 [Service]
-ExecStart=/usr/local/bin/tuios-web --host 127.0.0.1 --port 7681 --config /etc/tuios-web/config.toml
+ExecStart=/usr/local/bin/tuios-web --host 127.0.0.1 --port 7681 --config /etc/tuios-web/config.toml --pam-auth --web-settings
```

(`--web-settings` - the in-browser theme/font picker, see
[docs/WEB.md](WEB.md#client-settings) - needs the same internal front-door
proxy `--pam-auth` does, which is why it's usually turned on alongside it,
but it works standalone too if you just want the picker without PAM.)

Then, for whichever shape you picked:

```bash
sudo systemctl enable --now tuios-pam-helper.service   # PAM mode only, start this first
sudo systemctl enable --now tuios-web.service
sudo systemctl status tuios-web.service tuios-pam-helper.service
```

`Restart=on-failure` is set on both, so a crash or an OOM kill recovers on
its own; it is not set to `always`, so a clean `systemctl stop` (or a
deliberate non-zero exit, if either binary is ever changed to have one)
stays stopped rather than fighting you.

## 6. nginx reverse proxy and TLS

`tuios-web` above is bound to `127.0.0.1:7681` - nothing outside the machine
can reach it directly yet. [`packaging/nginx/tuios-web.conf`](../packaging/nginx/tuios-web.conf)
is a ready-to-edit site config that terminates TLS and proxies everything
(the page, the `--web-settings` endpoints, and the WebSocket upgrade) to
that loopback address:

```bash
sudo apt install nginx certbot python3-certbot-nginx   # or your distro's equivalent
sudo cp packaging/nginx/tuios-web.conf /etc/nginx/sites-available/tuios-web
sudo sed -i 's/tuios\.example\.com/tuios.yourdomain.example/' /etc/nginx/sites-available/tuios-web
sudo ln -s /etc/nginx/sites-available/tuios-web /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
sudo certbot --nginx -d tuios.yourdomain.example   # obtains a cert and rewrites the ssl_certificate lines for you
```

**What this does and does not proxy:** WebTransport (tuios-web's low-latency
QUIC/UDP/HTTP3 transport) is not proxied by this config - that needs
nginx's `stream` module plus real HTTP/3 support, well beyond a single
`location` block. This is not a functional loss: the browser client already
falls back to plain WebSocket automatically whenever WebTransport is
unreachable, the same fallback `--pam-auth`'s own *internal* front-door
proxy already relies on for an identical reason (see
[pam-helper/README.md](../pam-helper/README.md), "A side effect of the
front door worth knowing about"). Expect a brief, sub-second connection
delay on first load, never a broken connection.

**No reverse proxy at all?** If you don't have a domain name to put a real
certificate on - a LAN-only classroom reached by IP address, say - skip
nginx entirely and let `tuios-web` terminate its own TLS instead:
`--host 192.168.1.31 --auto-tls`. See
[docs/WEB.md's Security section](WEB.md#binding-a-lan-address-reaching-the-server-from-a-phone)
for what that certificate covers and the one-time browser warning it comes
with.

## 7. Verifying the deployment

```bash
# tuios-web itself, on the loopback address the unit binds:
curl -o /dev/null -w '%{http_code}\n' http://127.0.0.1:7681/
# 200 without --pam-auth; 401 with it (no credentials supplied) - either is correct

# Through nginx, from outside the machine:
curl -o /dev/null -w '%{http_code}\n' https://tuios.yourdomain.example/

# PAM mode: confirm the settings/theme endpoints are gated too, not just "/"
# (a real bug this project shipped and fixed once - see git log
# cmd/tuios-web/pamfrontdoor.go - worth checking it stayed fixed):
curl -o /dev/null -w '%{http_code}\n' https://tuios.yourdomain.example/tuios-settings/themes
# 401 expected with --pam-auth and no credentials
```

Then open the URL in a real browser. PAM mode should show a native login
popup before anything else renders. Either mode should land in a working
shell; `whoami` inside it should be the trainee's own account in PAM mode,
or the `tuios-web` service account otherwise.

```bash
journalctl -u tuios-web.service -f            # tail logs live
journalctl -u tuios-pam-helper.service -f     # PAM mode only
```

## Presenter mirror session (optional)

PAM multi-tenant mode only: type on your laptop, show the same thing on a
projector via a second machine/tab, without giving that second tab any
input of its own. `--pam-auth` sessions can't do this by themselves - each
login is deliberately its own isolated, non-daemon `OS` instance
(`createPAMTUIOSInstance`'s own doc comment), never shared between
connections. The fix is a second, ordinary (non-PAM) `tuios-web` instance
serving one shared daemon session that both your laptop and the projector
machine attach to.

1. **A second config and unit**, differing from the main ones (steps 3 and
   5) only in port, session name, and dropping `--pam-auth`:

   ```bash
   sudo cp /etc/tuios-web/config.toml /etc/tuios-web/mirror-config.toml
   sudo -e /etc/tuios-web/mirror-config.toml
   ```

   Give it its own `[daemon] socket_path` (e.g.
   `/run/tuios-web-mirror/tuios.sock`) - this instance and `tuios-web.service`
   must never share a daemon socket, even though `PrivateTmp` on both units
   already keeps their *default* per-uid socket paths from colliding in
   practice when `User=` matches on both.

   ```bash
   sudo cp packaging/systemd/tuios-web-mirror.service /etc/systemd/system/
   sudo systemctl daemon-reload
   ```

   `--default-session mirror` in that unit's `ExecStart=` is what makes
   this shared: every connection to this instance attaches to the same
   daemon-backed session by name (see
   [Daemon Mode](WEB.md#daemon-mode-default)), the same feature
   [docs/MULTI_CLIENT.md](MULTI_CLIENT.md) covers for any other purpose. Its
   `StateDirectory=` is its own (`tuios-web-mirror`, not the main unit's
   `tuios-web`) so that changing its `User=` away from the main instance's
   never has systemd rechown the main instance's own `/var/lib/tuios-web`
   out from under it.

2. **Start it:**

   ```bash
   sudo systemctl enable --now tuios-web-mirror.service
   ```

3. **Gate it, since PAM isn't gating this one.** `tuios-web` has no
   CLI-exposed HTTP Basic Auth of its own (sip's `Config.BasicUsername`/
   `BasicPassword` support a single fixed pair, but nothing in `tuios-web`
   wires them up) - use nginx's instead, on a route separate from the main
   `location /` (step 6):

   ```nginx
   location /mirror/ {
       auth_basic           "presenter mirror";
       auth_basic_user_file /etc/nginx/.htpasswd-mirror;
       proxy_read_timeout   1d;
       proxy_send_timeout   1d;
       proxy_set_header     Upgrade $http_upgrade;
       proxy_set_header     Connection "upgrade";
       proxy_set_header     Host $http_host;
       rewrite ^/mirror/?$    / break;
       rewrite ^/mirror/(.*)$ /$1 break;
       proxy_pass            http://127.0.0.1:7682;
       proxy_http_version    1.1;
   }
   ```

   ```bash
   sudo sh -c 'echo -n "presenter:" >> /etc/nginx/.htpasswd-mirror'
   openssl passwd -apr1 | sudo tee -a /etc/nginx/.htpasswd-mirror
   ```

4. **Open the same URL and credentials on both machines** - your laptop and
   whatever drives the projector. Anything typed on one appears on the
   other, live.

There is no per-connection read-only in `tuios-web` - `--read-only`
disables input for every client of an instance, not just some, so it can't
single out the projector tab while leaving your laptop interactive.
Simplest mitigation: don't let anyone touch the projector machine's
keyboard.

## Trainer console (optional)

PAM multi-tenant mode only: a designated trainer account can attach to a
trainee's live session - view it, type into it - instead of only ever
getting their own. See [docs/CONFIGURATION.md's Classroom
Settings](CONFIGURATION.md#classroom-settings) for the `[classroom]` config
itself (`trainer_console`, `trainer_users`, `trainee_pattern`); this section
is only the deployment-shape requirement it adds.

**A `tuios` daemon must be running alongside `tuios-web`.** Classroom
sessions are daemon-backed (unlike an ordinary `--pam-auth` login, which is
its own isolated, non-daemon instance per connection - see
[pam-helper/README.md](../pam-helper/README.md)) so that more than one
connection can ever attach to the same one at all. `tuios-web` does not
start this daemon for you, deliberately - same reasoning as
`tuios-pam-helper` being its own separate systemd unit rather than something
`tuios-web` re-execs on demand: an explicit, independently-managed unit is
easier to reason about than automatic process spawning.

```bash
sudo cp packaging/systemd/tuios-daemon.service /etc/systemd/system/
sudo systemctl daemon-reload
```

**`tuios-web.service` needs one addition first: `Environment=XDG_RUNTIME_DIR=/var/lib/tuios-web`**,
next to its existing `Environment=` lines (`sudo systemctl edit --full
tuios-web.service`). `tuios-web.service` runs with `PrivateTmp=true`, which
gives it its own isolated `/tmp` - the session-daemon protocol's default
socket path (`$XDG_RUNTIME_DIR`, or a `/tmp/tuios-<uid>/` fallback if unset)
would otherwise never be reachable from `tuios-daemon.service`, a *separate*
unit with its own, different private `/tmp`. Pointing both at the same
real, non-tmp, already-`StateDirectory`-managed directory sidesteps that
entirely - `tuios-daemon.service` (above) already sets the identical value,
so this is the only edit needed on the `tuios-web.service` side. Skip this
whole paragraph if `tuios-web.service`'s `PrivateTmp` is ever turned off.

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now tuios-daemon.service
sudo systemctl restart tuios-web.service   # to pick up the new environment line
```

Without a reachable daemon, a classroom connection shows a plain
full-screen error instead of silently falling back to a different,
non-persistent kind of session - if trainees report that, check
`systemctl status tuios-daemon.service` first.

## Hardening notes

The shipped `tuios-web.service` applies only `NoNewPrivileges=true` and
`PrivateTmp=true` - narrow enough to never break the tool, since
`tuios-web` spawns a real, useful shell for whoever connects (running as
the `tuios-web` account itself, unless PAM hands that off to a trainee
account instead), and options like `ProtectSystem=strict` or
`ProtectHome=true` would cripple that shell's access to the rest of the
filesystem - the opposite of what a "terminal in the browser" tool is for.

If your deployment is **PAM-only and nobody is ever meant to get a shell as
the bare `tuios-web` account** (every real connection authenticates and
lands in a trainee account instead, spawned by `tuios-pam-helper`, entirely
outside `tuios-web`'s own process boundary), then `tuios-web`'s own
sandboxing can be tightened safely, since nothing of value happens as that
account:

```ini
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/tuios-web
```

`tuios-pam-helper.service` intentionally has no sandboxing directives at
all beyond the basics - it needs broad access (the system's real PAM stack,
`setuid` to arbitrary local users) that nearly every systemd hardening
option exists specifically to prevent. Its own trust boundary is PAM
itself and the fact that its Unix socket, while world-writable by design
(`chmod 0666` - see [pam-helper/README.md](../pam-helper/README.md), "What's
still not here"), is only reachable by someone who already has a login
shell on the same host.

## Troubleshooting

**`tuios-web.service` fails immediately with a bind error.** Something else
is already using port 7681, or a previous instance didn't shut down
cleanly. `sudo ss -tlnp | grep 7681` to find it.

**Browser shows no login prompt in PAM mode.** Confirm
`tuios-pam-helper.service` is actually running
(`systemctl status tuios-pam-helper.service`) and that `tuios-web.service`'s
`ExecStart=` actually has `--pam-auth` on it (a plain `systemctl edit` that
opened the *drop-in* editor rather than `--full` would leave the original
`ExecStart=` line in place, silently ignored). A login prompt that never
appears at all (not even a browser error) usually means `--pam-auth` isn't
actually set; see [pam-helper/README.md](../pam-helper/README.md), "How
it's wired into tuios-web" for why gating only the WebSocket layer (which
is what a misconfigured deployment falls back to) can never produce one.

**Login prompt appears but every password is rejected.**
`journalctl -u tuios-pam-helper.service` logs the real PAM failure reason
server-side (the browser only ever sees "authentication failed", by
design). Common causes: the account doesn't exist locally, `/etc/pam.d/tuios-web`
was edited to reference a PAM module that isn't installed, or the account's
password is locked/expired.

**nginx returns 502.** `tuios-web.service` isn't running, or is bound to a
different port than `proxy_pass` in the nginx config expects - both must
agree (`7681` in the examples above).

**Everything works over HTTP but WebTransport never connects.** Expected
behind this nginx config - see [step 6](#6-nginx-reverse-proxy-and-tls). Not
a bug; the client already falls back to WebSocket.

## Related Documentation

- [Web Terminal Mode (tuios-web)](WEB.md) - the full flag reference, transport/rendering details, and TLS options for running `tuios-web` without a reverse proxy at all
- [pam-helper/README.md](../pam-helper/README.md) - how PAM multi-tenant mode is actually wired together, and what it deliberately does not cover
- [Configuration](CONFIGURATION.md) - every `config.toml` option
- [CLI Reference](CLI_REFERENCE.md) - complete command reference
