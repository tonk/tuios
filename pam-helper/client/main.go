// Command pam-client is a minimal manual test client for pam-helper: it asks
// for a username and password, hands them to pam-helper over a Unix socket,
// and if login succeeds, receives back a PTY master fd for a shell already
// running as that trainee's own Unix account. It then acts as a minimal
// terminal, so you can drive that shell interactively and confirm
// `whoami`/`id`/`echo $HOME` all show the right identity - useful for trying
// the helper out on its own, independent of tuios-web.
//
// This client itself never runs privileged and never touches PAM; it only
// ever sees a file descriptor the helper already set up.
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"

	"github.com/tonk/tuios/pam-helper/internal/wire"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pam-client:", err)
		os.Exit(1)
	}
}

func run() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading username: %w", err)
	}
	username = strings.TrimSpace(username)

	fmt.Print("password: ")
	passwordB, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("reading password: %w", err)
	}

	conn, err := net.Dial("unixpacket", wire.SocketPath)
	if err != nil {
		return fmt.Errorf("dialing %s (is pam-helper running as root?): %w", wire.SocketPath, err)
	}
	defer func() { _ = conn.Close() }()
	uconn := conn.(*net.UnixConn)

	var loginPayload bytes.Buffer
	wire.PutField(&loginPayload, []byte(username))
	wire.PutField(&loginPayload, passwordB)
	if err := wire.WriteMessage(uconn, wire.MsgLogin, loginPayload.Bytes(), -1); err != nil {
		return fmt.Errorf("sending login: %w", err)
	}
	if err := expectOK(uconn, wire.MsgLoginResult, "login"); err != nil {
		return err
	}
	fmt.Println("logged in — press enter to open a shell (ctrl-] to close it), or ctrl-c to quit.")

	for {
		if _, err := reader.ReadString('\n'); err != nil {
			return nil
		}

		w, h := 80, 24
		if cw, ch, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
			w, h = cw, ch
		}
		var spawnPayload [8]byte
		binary.BigEndian.PutUint32(spawnPayload[0:4], uint32(w))
		binary.BigEndian.PutUint32(spawnPayload[4:8], uint32(h))
		if err := wire.WriteMessage(uconn, wire.MsgSpawnPTY, spawnPayload[:], -1); err != nil {
			return fmt.Errorf("sending spawn request: %w", err)
		}
		msgType, payload, fd, err := wire.ReadMessage(uconn)
		if err != nil {
			return fmt.Errorf("reading spawn result: %w", err)
		}
		if msgType != wire.MsgSpawnPTYResult {
			return fmt.Errorf("unexpected response type %d to spawn", msgType)
		}
		if len(payload) < 5 || payload[0] != 1 {
			return fmt.Errorf("spawn failed: %s", spawnErrorText(payload))
		}
		pid := binary.BigEndian.Uint32(payload[1:5])
		if fd < 0 {
			return fmt.Errorf("spawn succeeded but no pty fd was received")
		}

		ptyFile := os.NewFile(uintptr(fd), "pty")
		fmt.Printf("attached to pid %d — ctrl-] to detach\n", pid)
		if err := attach(ptyFile); err != nil {
			_ = ptyFile.Close()
			return err
		}
		_ = ptyFile.Close()
		fmt.Println("detached — press enter to open another shell, or ctrl-c to quit.")
	}
}

// expectOK reads one message, checks it's the expected type and status-ok,
// and turns a status-error response into a Go error.
func expectOK(conn *net.UnixConn, wantType byte, what string) error {
	msgType, payload, _, err := wire.ReadMessage(conn)
	if err != nil {
		return fmt.Errorf("reading %s result: %w", what, err)
	}
	if msgType != wantType {
		return fmt.Errorf("unexpected response type %d to %s", msgType, what)
	}
	if len(payload) < 1 || payload[0] != 1 {
		r := bytes.NewReader(payload[min(1, len(payload)):])
		msg, _ := wire.GetField(r)
		return fmt.Errorf("%s failed: %s", what, msg)
	}
	return nil
}

func spawnErrorText(payload []byte) string {
	if len(payload) < 5 {
		return "malformed response"
	}
	r := bytes.NewReader(payload[5:])
	msg, err := wire.GetField(r)
	if err != nil {
		return "malformed error message"
	}
	return string(msg)
}

// attach puts the local terminal in raw mode and pipes it to/from the PTY
// until either side closes or the detach key (ctrl-]) is pressed.
func attach(ptyFile *os.File) error {
	stdinFd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		return fmt.Errorf("entering raw mode: %w", err)
	}
	defer func() { _ = term.Restore(stdinFd, oldState) }()

	// Forward window resizes so full-screen programs (vim, less, tuios
	// itself) lay out correctly.
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	go func() {
		for range winch {
			if w, h, err := term.GetSize(stdinFd); err == nil {
				setPtySize(ptyFile, w, h)
			}
		}
	}()
	if w, h, err := term.GetSize(stdinFd); err == nil {
		setPtySize(ptyFile, w, h)
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(os.Stdout, ptyFile)
		close(done)
	}()

	const detachKey = 0x1d // ctrl-]
	buf := make([]byte, 1)
	for {
		select {
		case <-done:
			return nil
		default:
		}
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return nil
		}
		if n == 1 && buf[0] == detachKey {
			return nil
		}
		if _, err := ptyFile.Write(buf[:n]); err != nil {
			return nil
		}
	}
}

func setPtySize(f *os.File, cols, rows int) {
	_ = pty.Setsize(f, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
