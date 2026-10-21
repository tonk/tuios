package session

import (
	"os"
	"testing"
)

// TestResurrectionStateRecordsClassroomUser pins that a classroom session's
// saved state says whose login spawned its shells, which is what lets a
// restarted daemon tell it apart from an ordinary session.
func TestResurrectionStateRecordsClassroomUser(t *testing.T) {
	sess := newTestSession(t)
	if got := sess.ResurrectionState().ClassroomUser; got != "" {
		t.Errorf("an ordinary session saved ClassroomUser %q, want none", got)
	}

	sp := newFakeClassroomSpawner()
	sp.username = "guru07"
	sess.SetClassroomSpawner(sp)
	if got := sess.ResurrectionState().ClassroomUser; got != "guru07" {
		t.Errorf("classroom session saved ClassroomUser %q, want %q", got, "guru07")
	}
	if got := sess.GetState().ClassroomUser; got != "" {
		t.Errorf("GetState carries ClassroomUser %q; it belongs to saved state only", got)
	}
}

// TestDaemonRestoreSkipsClassroomSession covers a daemon restart with a
// classroom session on disk. Restoring it spawned the trainee's windows as the
// daemon's own account, under the trainee's name: the trainer's picker listed
// it as if the trainee were logged in, and attaching handed out a shell as the
// service account. It must not come back, its file must go so it is not
// offered again, and an ordinary session beside it must still restore.
func TestDaemonRestoreSkipsClassroomSession(t *testing.T) {
	tmpDir := t.TempDir()
	defer useResurrectionDir(tmpDir)()

	cwd := t.TempDir()
	classroom := &SessionState{
		Name: "guru07", ClassroomUser: "guru07", Width: 120, Height: 40,
		Windows: []WindowState{{ID: "win-1", Width: 60, Height: 40, Workspace: 1, Cwd: cwd}},
	}
	ordinary := &SessionState{
		Name: "work", Width: 120, Height: 40,
		Windows: []WindowState{{ID: "win-1", Width: 60, Height: 40, Workspace: 1, Cwd: cwd}},
	}
	for _, st := range []*SessionState{classroom, ordinary} {
		if err := SaveSessionForResurrection(st); err != nil {
			t.Fatalf("failed to save %q: %v", st.Name, err)
		}
	}

	d := NewDaemon(&DaemonConfig{})
	defer d.manager.Shutdown()
	d.restoreAllSessions()

	if d.manager.GetSession("guru07") != nil {
		t.Error("the classroom session was restored; its shells would run as the daemon's account")
	}
	if _, err := os.Stat(getResurrectionPath("guru07")); !os.IsNotExist(err) {
		t.Errorf("the classroom session's state file is still on disk (stat err: %v)", err)
	}
	if d.manager.GetSession("work") == nil {
		t.Error("an ordinary session beside the classroom one was not restored")
	}
}

// TestOnDemandResurrectRefusesClassroomSession covers the same refusal on the
// 'tuios resurrect <name>' path, which loads the state itself.
func TestOnDemandResurrectRefusesClassroomSession(t *testing.T) {
	tmpDir := t.TempDir()
	defer useResurrectionDir(tmpDir)()

	d := NewDaemon(&DaemonConfig{})
	defer d.manager.Shutdown()

	state := &SessionState{
		Name: "guru07", ClassroomUser: "guru07",
		Windows: []WindowState{{ID: "win-1", Width: 60, Height: 40, Workspace: 1, Cwd: t.TempDir()}},
	}
	if _, err := d.restoreSession(state); err == nil {
		t.Fatal("resurrecting a classroom session succeeded; its shells would run as the daemon's account")
	}
	if d.manager.GetSession("guru07") != nil {
		t.Error("the refused classroom session was registered anyway")
	}
}
