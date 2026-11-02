package main

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

// ptsLine returns the utmp ut_line for ptmx's slave (e.g. "pts/3"), without
// the /dev/ prefix who(1) expects. Uses TIOCGPTN on the master, the same
// ioctl creack/pty uses internally - StartWithSize only hands back the
// master, whose Name() is /dev/ptmx, so we have to resolve the slave number
// ourselves.
func ptsLine(ptmx *os.File) (string, error) {
	var n uint32
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		ptmx.Fd(),
		uintptr(syscall.TIOCGPTN),
		uintptr(unsafe.Pointer(&n)),
	)
	if errno != 0 {
		return "", fmt.Errorf("TIOCGPTN: %w", errno)
	}
	return "pts/" + strconv.Itoa(int(n)), nil
}
