// Command pam-client is the unprivileged half of the PAM trainee-auth
// prototype: it asks for a username and password, hands them to pam-helper
// over a Unix socket, and if login succeeds, receives back a PTY master fd
// for a shell already running as that trainee's own Unix account. It then
// acts as a minimal terminal, so you can drive that shell interactively and
// confirm `whoami`/`id`/`echo $HOME` all show the right identity.
//
// This client itself never runs privileged and never touches PAM; it only
// ever sees a file descriptor the helper already set up.
package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"

	"github.com/tonk/tuios-pam-poc/internal/wire"
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

	conn, err := net.Dial("unix", wire.SocketPath)
	if err != nil {
		return fmt.Errorf("dialing %s (is pam-helper running as root?): %w", wire.SocketPath, err)
	}
	defer func() { _ = conn.Close() }()
	uconn := conn.(*net.UnixConn)

	if err := wire.WriteFrame(uconn, []byte(username)); err != nil {
		return fmt.Errorf("sending username: %w", err)
	}
	if err := wire.WriteFrame(uconn, passwordB); err != nil {
		return fmt.Errorf("sending password: %w", err)
	}

	ok, fd, err := wire.RecvResult(uconn)
	if err != nil {
		return fmt.Errorf("receiving result: %w", err)
	}
	if !ok {
		msg, rerr := wire.ReadFrame(uconn)
		if rerr != nil {
			return fmt.Errorf("login failed (and reading the reason failed too: %v)", rerr)
		}
		return fmt.Errorf("login failed: %s", msg)
	}

	ptyFile := os.NewFile(uintptr(fd), "pty")
	defer func() { _ = ptyFile.Close() }()

	fmt.Println("logged in — you are now attached to a real shell running as that user. ctrl-] to detach.")
	return attach(ptyFile)
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
