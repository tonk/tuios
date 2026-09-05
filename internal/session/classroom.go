package session

import (
	"fmt"
	"os"
)

// ClassroomSpawner is the capability a held PAM login provides: spawn and
// close PTYs for one already-authenticated Unix account, without this
// package needing to import internal/pamauth directly - only the daemon's
// classroom login-handoff listener does that, to reconstruct one from a
// received file descriptor (see internal/pamauth.NewLoginFromFD).
// *pamauth.Login satisfies this exactly.
type ClassroomSpawner interface {
	// SpawnPTY starts one more shell for this login, at the given terminal
	// size, and returns its PTY master and pid - see AdoptPTY for how the
	// result becomes a daemon window.
	SpawnPTY(cols, rows int) (ptyFile *os.File, pid int, err error)
	// ClosePTY signals one shell previously returned by SpawnPTY.
	ClosePTY(pid int) error
	// Close ends the whole login: every shell it ever spawned is signalled
	// and the underlying PAM session is torn down.
	Close() error
}

// SetClassroomSpawner attaches sp as this session's PAM login: the source of
// every window's PTY from now on, via NewClassroomWindow, in place of this
// session spawning shells itself via exec.Command. A session gets one only
// through the daemon's classroom login-handoff listener, exactly once, when
// the session is first created; an ordinary session never has one.
//
// Calling it again on a session that already has one is dangerous, not
// idempotent: pamauth.Login.Close (which a caller would need to call on the
// previous spawner to avoid leaking it) signals every shell that Login ever
// spawned - i.e. every window currently open under it - which is never what
// a mere browser reconnect should do. A trainee's browser reconnecting
// re-authenticates via PAM on every connection (see pamAuthMiddleware), but
// that freshly-dialed, as-yet-unused Login must be closed by the caller
// without ever reaching here once the daemon already holds a live spawner
// for the session; see the handoff client's own existence check before
// calling SendClassroomLogin.
func (s *Session) SetClassroomSpawner(sp ClassroomSpawner) {
	s.classroomSpawnerMu.Lock()
	s.classroomSpawner = sp
	s.classroomSpawnerMu.Unlock()
}

// ClassroomSpawner returns the session's held PAM login, or nil if this is
// not a classroom session.
func (s *Session) ClassroomSpawner() ClassroomSpawner {
	s.classroomSpawnerMu.RLock()
	defer s.classroomSpawnerMu.RUnlock()
	return s.classroomSpawner
}

// NewClassroomWindow opens a window whose shell is spawned by this session's
// held PAM login (SpawnPTY) rather than by this session directly, adopting
// the resulting PTY exactly like AddDaemonWindow's own exec.Command spawn -
// see AdoptDaemonWindow. Returns an error if this session has no classroom
// spawner.
func (s *Session) NewClassroomWindow(title string, onExit func(ptyID string)) (WindowState, error) {
	sp := s.ClassroomSpawner()
	if sp == nil {
		return WindowState{}, fmt.Errorf("session %q has no classroom login", s.Name)
	}

	width, height, ptyWidth, ptyHeight := s.daemonWindowSize()
	windowID, title := newDaemonWindowID(title)

	ptyFile, pid, err := sp.SpawnPTY(ptyWidth, ptyHeight)
	if err != nil {
		return WindowState{}, fmt.Errorf("spawning PTY via classroom login: %w", err)
	}
	pty, err := s.AdoptPTY(windowID, ptyFile, pid, ptyWidth, ptyHeight, onExit, func() error {
		return sp.ClosePTY(pid)
	})
	if err != nil {
		return WindowState{}, err
	}
	return s.registerDaemonWindow(windowID, title, width, height, pty), nil
}
