package app

import (
	"testing"

	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/terminal"
)

func TestApplyConfiguredWindowTitleForcesFormatWhenLocked(t *testing.T) {
	prevFormat, prevLock := config.InitialTitleFormat, config.LockTitles
	config.InitialTitleFormat = "{user}@ansiblelab"
	config.LockTitles = true
	t.Cleanup(func() {
		config.InitialTitleFormat = prevFormat
		config.LockTitles = prevLock
	})

	m := &OS{
		SessionName:      "guru01",
		IsDaemonSession:  true,
		InitialTitleUser: "guru01",
	}
	w := terminal.NewDaemonWindow("id", "guru01@stepper:~", 0, 0, 80, 24, 0, "pty", nil)
	if w == nil {
		t.Fatal("NewDaemonWindow returned nil")
	}
	if !m.applyConfiguredWindowTitle(w) {
		t.Fatal("expected a title change")
	}
	if got := w.Title(); got != "guru01@ansiblelab" {
		t.Errorf("Title = %q, want guru01@ansiblelab", got)
	}
	if !w.TitleLocked() {
		t.Error("expected TitleLocked after apply with lock_titles")
	}
}

func TestTitleUserForFormatPrefersInitialTitleUser(t *testing.T) {
	m := &OS{
		SessionName:      "guru01",
		IsDaemonSession:  true,
		InitialTitleUser: "guru01",
	}
	if got := m.titleUserForFormat(); got != "guru01" {
		t.Errorf("titleUserForFormat = %q, want guru01", got)
	}
}
