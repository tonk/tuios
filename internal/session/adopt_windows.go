//go:build windows

package session

// waitForAdoptedExit is a stub for Windows. Adopted PIDs come from the
// Unix-only PAM helper, so there is never a process to wait on.
func waitForAdoptedExit(_ int) {}
