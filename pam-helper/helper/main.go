// Command pam-helper is the privileged half of tuios-web's optional
// --pam-auth mode. It must run as root. One connection is one PAM login: it
// authenticates a username/password pair against PAM (service "tuios-web",
// see ../pam.d/tuios-web) once, then — for as long as the connection stays
// open — spawns as many shells as asked for that trainee's own uid/gid, each
// on its own fresh PTY, handing the master fd to the caller via SCM_RIGHTS.
// The caller never needs any privilege of its own, and this helper never
// touches a shell's stdin/stdout again once its fd has crossed over.
//
// Closing the connection is what ends the login: every child shell still
// running for it is signalled, then the PAM session is closed and its
// credentials released. This lets one authenticated trainee open and close
// as many windows as they like without re-entering a password, while still
// tying the whole login's lifetime to one clear, unambiguous event.
//
// The wire protocol (see ../internal/wire) sends the password in the clear
// over a local socket, with no client authentication of its own beyond "can
// connect to this socket." That's the accepted trust boundary: the socket
// is reachable only on this host, and every actual credential check still
// happens here, against PAM. See ../README.md for the full design and its
// known limitations.
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
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

	"github.com/Gaurav-Gosain/tuios/pam-helper/internal/wire"
)

// pamService is the PAM service name; /etc/pam.d/tuios-web must exist (see
// ../pam.d/tuios-web for a starter file to install there).
const pamService = "tuios-web"

// Version information, set via -ldflags by the root Makefile's
// dist-pam-helper/package-pam-helper targets - the same -X main.xxx pattern
// cmd/tuios and cmd/tuios-web use, kept in sync by hand since this is a
// separate module the root build can't reach with a shared var.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("tuios-pam-helper %s (commit %s, built %s by %s)\n", version, commit, date, builtBy)
		return
	}
	if os.Geteuid() != 0 {
		log.Fatal("pam-helper must run as root (it needs to authenticate against /etc/shadow and setuid to the trainee's account)")
	}

	_ = os.Remove(wire.SocketPath)
	// unixpacket (SOCK_SEQPACKET) rather than plain unix (SOCK_STREAM): each
	// WriteMsgUnix call is then guaranteed to arrive as exactly one
	// ReadMsgUnix call, so a message's SCM_RIGHTS fd can never end up
	// ambiguous about which read it belongs to.
	ln, err := net.Listen("unixpacket", wire.SocketPath)
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

// login is one authenticated PAM session and the shells spawned under it.
type login struct {
	username string
	tx       *pam.Transaction
	u        *user.User
	uid, gid uint32
	groups   []uint32
	shell    string
	env      []string

	mu       sync.Mutex
	children map[int]*os.Process // pid -> process, only pids this login itself spawned
}

func handleConn(conn *net.UnixConn) {
	defer func() { _ = conn.Close() }()

	msgType, payload, _, err := wire.ReadMessage(conn)
	if err != nil {
		log.Printf("read login message: %v", err)
		return
	}
	if msgType != wire.MsgLogin {
		log.Printf("expected MsgLogin, got type %d", msgType)
		return
	}
	username, password, err := decodeLogin(payload)
	if err != nil {
		log.Printf("decoding login message: %v", err)
		return
	}

	lg, err := authenticate(username, password)
	if err != nil {
		log.Printf("login for %q failed: %v", username, err)
		_ = wire.WriteMessage(conn, wire.MsgLoginResult, encodeResult(false, err), -1)
		return
	}
	defer lg.close()

	log.Printf("login for %q ok", username)
	if err := wire.WriteMessage(conn, wire.MsgLoginResult, encodeResult(true, nil), -1); err != nil {
		log.Printf("sending login result: %v", err)
		return
	}

	for {
		msgType, payload, _, err := wire.ReadMessage(conn)
		if err != nil {
			log.Printf("login for %q: connection ended (%v)", username, err)
			return
		}
		switch msgType {
		case wire.MsgSpawnPTY:
			handleSpawnPTY(conn, lg, payload)
		case wire.MsgClosePTY:
			handleClosePTY(conn, lg, payload)
		default:
			log.Printf("login for %q: unexpected message type %d", username, msgType)
			return
		}
	}
}

func handleSpawnPTY(conn *net.UnixConn, lg *login, payload []byte) {
	cols, rows, err := decodeSpawnPTY(payload)
	if err != nil {
		log.Printf("decoding spawn request: %v", err)
		_ = wire.WriteMessage(conn, wire.MsgSpawnPTYResult, encodeSpawnResult(false, 0, err), -1)
		return
	}

	ptmx, proc, err := lg.spawnShell(cols, rows)
	if err != nil {
		log.Printf("login for %q: spawning shell: %v", lg.username, err)
		_ = wire.WriteMessage(conn, wire.MsgSpawnPTYResult, encodeSpawnResult(false, 0, err), -1)
		return
	}
	defer func() { _ = ptmx.Close() }() // caller keeps its own dup via SCM_RIGHTS

	log.Printf("login for %q: spawned pid %d", lg.username, proc.Pid)
	if err := wire.WriteMessage(conn, wire.MsgSpawnPTYResult, encodeSpawnResult(true, proc.Pid, nil), int(ptmx.Fd())); err != nil {
		log.Printf("sending spawn result: %v", err)
		_ = proc.Kill()
	}
}

func handleClosePTY(conn *net.UnixConn, lg *login, payload []byte) {
	pid, err := decodeClosePTY(payload)
	if err != nil {
		log.Printf("decoding close request: %v", err)
		_ = wire.WriteMessage(conn, wire.MsgClosePTYResult, encodeResult(false, err), -1)
		return
	}
	err = lg.closeShell(pid)
	_ = wire.WriteMessage(conn, wire.MsgClosePTYResult, encodeResult(err == nil, err), -1)
}

// authenticate runs the PAM login sequence once. On success the returned
// *login can spawn any number of shells until close is called.
func authenticate(username, password string) (*login, error) {
	answered := false
	convo := func(style pam.Style, msg string) (string, error) {
		switch style {
		case pam.PromptEchoOff, pam.PromptEchoOn:
			if answered {
				// A second prompt (e.g. an expired-password change flow) is
				// not yet handled.
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
		return nil, fmt.Errorf("pam start: %w", err)
	}
	endOnce := sync.OnceFunc(func() { _ = tx.End() })
	fail := func(step string, err error) (*login, error) {
		endOnce()
		return nil, fmt.Errorf("%s: %w", step, err)
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
		return fail("looking up user after successful auth", err)
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		closeSession(tx)
		return fail("parsing uid", err)
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		closeSession(tx)
		return fail("parsing gid", err)
	}
	groups, err := supplementaryGroups(u)
	if err != nil {
		closeSession(tx)
		return fail("resolving supplementary groups", err)
	}
	shell := loginShell(username)

	return &login{
		username: username,
		tx:       tx,
		u:        u,
		uid:      uint32(uid),
		gid:      uint32(gid),
		groups:   groups,
		shell:    shell,
		env:      buildEnv(u, shell, tx),
		children: make(map[int]*os.Process),
	}, nil
}

// spawnShell starts one more shell for this login, on its own fresh PTY at
// the given size.
func (lg *login) spawnShell(cols, rows int) (ptmx *os.File, proc *os.Process, err error) {
	// #nosec G204 - lg.shell is resolved from /etc/passwd for the
	// already-authenticated user, not attacker input.
	cmd := exec.Command(lg.shell, "-l")
	cmd.Dir = lg.u.HomeDir
	cmd.Env = lg.env
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid:    lg.uid,
			Gid:    lg.gid,
			Groups: lg.groups,
		},
	}

	ptmx, err = pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, nil, fmt.Errorf("starting shell: %w", err)
	}

	lg.mu.Lock()
	lg.children[cmd.Process.Pid] = cmd.Process
	lg.mu.Unlock()

	go func() {
		_ = cmd.Wait() // reap; the caller detects exit itself via its copy of the pty fd
		lg.mu.Lock()
		delete(lg.children, cmd.Process.Pid)
		lg.mu.Unlock()
	}()

	return ptmx, cmd.Process, nil
}

// closeShell signals one of this login's own shells to exit. It refuses to
// touch a pid this login did not itself spawn — a compromised or careless
// client asking a root process to kill an arbitrary pid is exactly the kind
// of request this must never honor blindly.
func (lg *login) closeShell(pid int) error {
	lg.mu.Lock()
	proc, ok := lg.children[pid]
	lg.mu.Unlock()
	if !ok {
		return fmt.Errorf("pid %d does not belong to this login", pid)
	}
	return proc.Signal(syscall.SIGHUP)
}

// close signals every shell still running for this login, then tears down
// the PAM session. Called once the connection ends, whatever the reason.
func (lg *login) close() {
	lg.mu.Lock()
	remaining := make([]*os.Process, 0, len(lg.children))
	for _, p := range lg.children {
		remaining = append(remaining, p)
	}
	lg.mu.Unlock()
	for _, p := range remaining {
		_ = p.Signal(syscall.SIGHUP)
	}
	closeSession(lg.tx)
	_ = lg.tx.End()
	log.Printf("login for %q ended", lg.username)
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

// --- message payload encode/decode ---

func decodeLogin(payload []byte) (username, password string, err error) {
	r := bytes.NewReader(payload)
	u, err := wire.GetField(r)
	if err != nil {
		return "", "", fmt.Errorf("reading username: %w", err)
	}
	p, err := wire.GetField(r)
	if err != nil {
		return "", "", fmt.Errorf("reading password: %w", err)
	}
	return string(u), string(p), nil
}

func encodeResult(ok bool, err error) []byte {
	var buf bytes.Buffer
	if ok {
		buf.WriteByte(1)
		return buf.Bytes()
	}
	buf.WriteByte(0)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	wire.PutField(&buf, []byte(msg))
	return buf.Bytes()
}

func decodeSpawnPTY(payload []byte) (cols, rows int, err error) {
	if len(payload) != 8 {
		return 0, 0, fmt.Errorf("bad spawn request: %d bytes", len(payload))
	}
	return int(binary.BigEndian.Uint32(payload[0:4])), int(binary.BigEndian.Uint32(payload[4:8])), nil
}

func encodeSpawnResult(ok bool, pid int, err error) []byte {
	var buf bytes.Buffer
	if ok {
		buf.WriteByte(1)
		var p [4]byte
		binary.BigEndian.PutUint32(p[:], uint32(pid))
		buf.Write(p[:])
		return buf.Bytes()
	}
	buf.WriteByte(0)
	var p [4]byte
	buf.Write(p[:]) // pid unused on failure, but keep the layout fixed
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	wire.PutField(&buf, []byte(msg))
	return buf.Bytes()
}

func decodeClosePTY(payload []byte) (pid int, err error) {
	if len(payload) != 4 {
		return 0, fmt.Errorf("bad close request: %d bytes", len(payload))
	}
	return int(binary.BigEndian.Uint32(payload)), nil
}
