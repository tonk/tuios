// Command pam-helper is the privileged half of the PAM trainee-auth
// prototype. It must run as root. It authenticates a username/password pair
// against PAM (service "tuios-web", see ../pam.d/tuios-web), and on success
// spawns that user's login shell attached to a fresh PTY, running as their
// own uid/gid with their own supplementary groups and home directory. The PTY
// master fd is then handed to whichever unprivileged client asked for it,
// over a Unix socket, using SCM_RIGHTS — the client never needs any
// privilege of its own, and this helper never has to touch the shell's
// stdin/stdout itself once the fd has crossed over.
//
// This is a prototype: the wire protocol (see ../internal/wire) sends the
// password in the clear over a local socket, with no client authentication of
// its own beyond "can connect to this socket." That is acceptable for proving
// the PAM + setuid + fd-passing mechanics out, and not acceptable as shipped:
// see ../README.md for what a real integration into tuios needs to add.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"github.com/msteinert/pam/v2"

	"github.com/tonk/tuios-pam-poc/internal/wire"
)

// pamService is the PAM service name; /etc/pam.d/tuios-web must exist (see
// ../pam.d/tuios-web for a starter file to install there).
const pamService = "tuios-web"

func main() {
	if os.Geteuid() != 0 {
		log.Fatal("pam-helper must run as root (it needs to authenticate against /etc/shadow and setuid to the trainee's account)")
	}

	_ = os.Remove(wire.SocketPath)
	ln, err := net.Listen("unix", wire.SocketPath)
	if err != nil {
		log.Fatalf("listen %s: %v", wire.SocketPath, err)
	}
	defer func() {
		_ = ln.Close()
		_ = os.Remove(wire.SocketPath)
	}()
	// Any local user can dial in and attempt a login; PAM is the actual gate.
	if err := os.Chmod(wire.SocketPath, 0o666); err != nil {
		log.Fatalf("chmod %s: %v", wire.SocketPath, err)
	}

	log.Printf("pam-helper listening on %s (service %q)", wire.SocketPath, pamService)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		uconn, ok := conn.(*net.UnixConn)
		if !ok {
			_ = conn.Close()
			continue
		}
		go handleConn(uconn)
	}
}

func handleConn(conn *net.UnixConn) {
	defer func() { _ = conn.Close() }()

	usernameB, err := wire.ReadFrame(conn)
	if err != nil {
		log.Printf("read username: %v", err)
		return
	}
	passwordB, err := wire.ReadFrame(conn)
	if err != nil {
		log.Printf("read password: %v", err)
		return
	}
	username := string(usernameB)
	password := string(passwordB)

	ptmx, cleanup, err := authenticateAndSpawn(username, password)
	if err != nil {
		log.Printf("login for %q failed: %v", username, err)
		if sendErr := wire.SendResult(conn, false, 0); sendErr != nil {
			log.Printf("sending failure result: %v", sendErr)
			return
		}
		_ = wire.WriteFrame(conn, []byte(err.Error()))
		return
	}
	defer func() { _ = ptmx.Close() }() // conn keeps its own dup via SCM_RIGHTS

	log.Printf("login for %q ok, handing off pty fd", username)
	if err := wire.SendResult(conn, true, int(ptmx.Fd())); err != nil {
		log.Printf("sending pty fd: %v", err)
		cleanup()
		return
	}
	// cleanup (pam close session) runs once the shell exits, from the
	// goroutine authenticateAndSpawn started; nothing more to do here.
}

// authenticateAndSpawn runs the PAM login sequence and, on success, starts
// the user's shell on a fresh PTY as their own uid/gid. cleanup must be
// called (it is, automatically, once the shell exits) to close the PAM
// session and release credentials.
func authenticateAndSpawn(username, password string) (ptmx *os.File, cleanup func(), err error) {
	answered := false
	convo := func(style pam.Style, msg string) (string, error) {
		switch style {
		case pam.PromptEchoOff, pam.PromptEchoOn:
			if answered {
				// A second prompt (e.g. an expired-password change flow) is
				// out of scope for this prototype.
				return "", errors.New("unexpected second PAM prompt")
			}
			answered = true
			return password, nil
		case pam.ErrorMsg, pam.TextInfo:
			log.Printf("pam(%s): %s", username, msg)
			return "", nil
		default:
			return "", fmt.Errorf("unhandled pam conversation style %v", style)
		}
	}

	tx, err := pam.StartFunc(pamService, username, convo)
	if err != nil {
		return nil, nil, fmt.Errorf("pam start: %w", err)
	}
	// On any error path below, End() releases the transaction; on the success
	// path, the goroutine that waits for the shell does it instead, after
	// CloseSession/SetCred(Delete).
	endOnce := sync.OnceFunc(func() { _ = tx.End() })
	fail := func(step string, err error) (*os.File, func(), error) {
		endOnce()
		return nil, nil, fmt.Errorf("%s: %w", step, err)
	}

	if err := tx.Authenticate(0); err != nil {
		return fail("authenticate", err)
	}
	if err := tx.AcctMgmt(0); err != nil {
		return fail("account management", err)
	}
	if err := tx.SetCred(pam.EstablishCred); err != nil {
		return fail("establish credentials", err)
	}
	if err := tx.OpenSession(0); err != nil {
		_ = tx.SetCred(pam.DeleteCred)
		return fail("open session", err)
	}

	u, err := user.Lookup(username)
	if err != nil {
		closeSession(tx)
		endOnce()
		return nil, nil, fmt.Errorf("looking up %q after successful auth: %w", username, err)
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		closeSession(tx)
		endOnce()
		return nil, nil, fmt.Errorf("parsing uid %q: %w", u.Uid, err)
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		closeSession(tx)
		endOnce()
		return nil, nil, fmt.Errorf("parsing gid %q: %w", u.Gid, err)
	}
	groups, err := supplementaryGroups(u)
	if err != nil {
		closeSession(tx)
		endOnce()
		return nil, nil, fmt.Errorf("resolving supplementary groups: %w", err)
	}

	shell := loginShell(username)
	env := buildEnv(u, shell, tx)

	cmd := exec.Command(shell, "-l")
	cmd.Dir = u.HomeDir
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid:    uint32(uid),
			Gid:    uint32(gid),
			Groups: groups,
		},
	}

	ptmx, err = pty.StartWithSize(cmd, nil)
	if err != nil {
		closeSession(tx)
		endOnce()
		return nil, nil, fmt.Errorf("starting shell: %w", err)
	}

	go func() {
		_ = cmd.Wait()
		closeSession(tx)
		endOnce()
		log.Printf("session for %q ended", username)
	}()

	return ptmx, func() { closeSession(tx); endOnce() }, nil
}

func closeSession(tx *pam.Transaction) {
	if err := tx.CloseSession(0); err != nil {
		log.Printf("pam close session: %v", err)
	}
	if err := tx.SetCred(pam.DeleteCred); err != nil {
		log.Printf("pam delete cred: %v", err)
	}
}

func supplementaryGroups(u *user.User) ([]uint32, error) {
	ids, err := u.GroupIds()
	if err != nil {
		return nil, err
	}
	groups := make([]uint32, 0, len(ids))
	for _, id := range ids {
		n, err := strconv.ParseUint(id, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("parsing group id %q: %w", id, err)
		}
		groups = append(groups, uint32(n))
	}
	return groups, nil
}

// loginShell reads the user's shell from /etc/passwd, falling back to
// /bin/sh. os/user does not expose the shell field.
func loginShell(username string) string {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return "/bin/sh"
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		// name:passwd:uid:gid:gecos:home:shell
		if len(fields) == 7 && fields[0] == username && fields[6] != "" {
			return fields[6]
		}
	}
	return "/bin/sh"
}

// buildEnv assembles a minimal login environment: the PAM-injected variables
// (from pam_env.conf, if configured) plus the basics every shell expects.
// PAM's list is applied first so the fixed values below always win, matching
// how login(1)/sshd behave.
func buildEnv(u *user.User, shell string, tx *pam.Transaction) []string {
	env := map[string]string{}
	if list, err := tx.GetEnvList(); err == nil {
		for k, v := range list {
			env[k] = v
		}
	}
	env["HOME"] = u.HomeDir
	env["USER"] = u.Username
	env["LOGNAME"] = u.Username
	env["SHELL"] = shell
	env["PATH"] = "/usr/local/bin:/usr/bin:/bin"
	env["TERM"] = "xterm-256color"

	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}
