package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/tape/trust"
)

// luaTapeContent is a Lua project tape body that touches no tuios.* API. Every
// verb that reaches window/app state runs through the Lua bridge, which needs
// a live Bubble Tea Update() loop to service it; these unit tests never run
// one, so a script that called into it would hang its goroutine forever. A
// no-op is all that's needed here: StartLuaPlayback flips LuaRunning to true
// synchronously, before the script's goroutine even starts, so that flag alone
// proves playback began.
const luaTapeContent = "-- noop\n"

// luaTapeDir creates a temp directory containing a .tuios.tape.lua and returns
// the dir.
func luaTapeDir(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, trust.LuaTapeFileName), []byte(content), 0o600); err != nil {
		t.Fatalf("writing lua tape: %v", err)
	}
	return dir
}

func TestDetectionFindsLuaProjectTape(t *testing.T) {
	m, _ := newDetectOS(t, config.TapeAutorunAsk)
	dir := luaTapeDir(t, luaTapeContent)

	drive(t, m, "focused", dir)

	status, ok := m.tapeIndicatorStatus()
	if !ok || status != trust.StatusUntrusted {
		t.Fatalf("indicator = (%v, %v), want (untrusted, true)", status, ok)
	}
	if m.tapeDetect.indicator.kind != TapeFileLua {
		t.Fatalf("indicator kind = %v, want TapeFileLua", m.tapeDetect.indicator.kind)
	}
	if badge := m.tapeDockBadge(); badge != "tape(lua) ?" {
		t.Fatalf("dock badge = %q, want a lua-flavored badge", badge)
	}
}

func TestDSLTapeTakesPrecedenceOverLua(t *testing.T) {
	m, _ := newDetectOS(t, config.TapeAutorunAsk)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, trust.TapeFileName), []byte("Type \"echo hi\" Enter\n"), 0o600); err != nil {
		t.Fatalf("writing dsl tape: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, trust.LuaTapeFileName), []byte(luaTapeContent), 0o600); err != nil {
		t.Fatalf("writing lua tape: %v", err)
	}

	drive(t, m, "focused", dir)

	if m.tapeDetect.indicator.kind != TapeFileDSL {
		t.Fatalf("indicator kind = %v, want TapeFileDSL to win when both files exist", m.tapeDetect.indicator.kind)
	}
}

func TestReviewRunOnceStartsLuaTape(t *testing.T) {
	m, store := newDetectOS(t, config.TapeAutorunAsk)
	dir := luaTapeDir(t, luaTapeContent)

	m.openTapeReviewForDir(dir)
	if m.TapeReview == nil || m.TapeReview.Kind != TapeFileLua {
		t.Fatalf("review state kind = %v, want TapeFileLua", m.TapeReview)
	}

	if handled, _ := m.HandleTapeReviewInput("r"); !handled {
		t.Fatalf("run-once key not consumed")
	}
	if !m.LuaRunning {
		t.Fatalf("LuaRunning = false, want the lua tape to have started")
	}
	if got := checkLuaTape(t, store, dir).Status; got != trust.StatusUntrusted {
		t.Fatalf("trust status = %v after Run once, want still untrusted (must not persist)", got)
	}
}

func TestReviewTrustAndRunPersistsLuaTape(t *testing.T) {
	m, store := newDetectOS(t, config.TapeAutorunAsk)
	dir := luaTapeDir(t, luaTapeContent)

	m.openTapeReviewForDir(dir)
	if handled, _ := m.HandleTapeReviewInput("t"); !handled {
		t.Fatalf("trust-and-run key not consumed")
	}
	if !m.LuaRunning {
		t.Fatalf("LuaRunning = false, want the lua tape to have started")
	}
	if got := checkLuaTape(t, store, dir).Status; got != trust.StatusTrusted {
		t.Fatalf("trust status = %v after Trust and run, want trusted", got)
	}
}

func TestAutoModeRunsTrustedLuaTape(t *testing.T) {
	m, store := newDetectOS(t, config.TapeAutorunAuto)
	dir := luaTapeDir(t, luaTapeContent)
	res := checkLuaTape(t, store, dir)
	if err := store.Trust(res.Path, res.Hash); err != nil {
		t.Fatalf("trust: %v", err)
	}

	drive(t, m, "focused", dir)

	if !m.LuaRunning {
		t.Fatalf("LuaRunning = false, want auto mode to run a trusted lua tape")
	}
	if m.ShowTapeReview {
		t.Fatalf("auto mode must not open a dialog for a trusted tape")
	}
}

func TestAutoModeDoesNotRunUntrustedLuaTape(t *testing.T) {
	m, _ := newDetectOS(t, config.TapeAutorunAuto)
	dir := luaTapeDir(t, luaTapeContent)

	drive(t, m, "focused", dir)

	if m.LuaRunning {
		t.Fatalf("LuaRunning = true, an untrusted lua tape must never auto-run")
	}
}

// checkLuaTape returns the current trust status for the lua tape in dir, via a
// fresh store read.
func checkLuaTape(t *testing.T, store *trust.Store, dir string) trust.Result {
	t.Helper()
	res, err := store.Check(filepath.Join(dir, trust.LuaTapeFileName))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	return res
}
