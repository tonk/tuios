package app

import (
	"testing"

	"github.com/tonk/tuios/internal/config"
)

// TestToggleSidebarPersistsToSidebarTable pins that the sidebar toggle records
// its new state in [appearance.sidebar] enabled, the key a reload reads. It
// used to write the legacy flat sidebar_enabled key, so a config that set the
// table kept its old value on the next start and the toggle was lost.
func TestToggleSidebarPersistsToSidebarTable(t *testing.T) {
	saved := config.SidebarEnabled
	t.Cleanup(func() { config.SidebarEnabled = saved })

	off := false
	config.SidebarEnabled = false
	cfg := config.DefaultConfig()
	cfg.Appearance.Sidebar.Enabled = &off
	m := &OS{UserConfig: cfg, Width: 120, Height: 40}

	m.ToggleSidebar()

	got := m.UserConfig.Appearance.Sidebar.Enabled
	if got == nil || !*got {
		t.Fatalf("[appearance.sidebar] enabled = %v after toggling on, want true", got)
	}
	if m.UserConfig.Appearance.SidebarEnabled != nil {
		t.Errorf("legacy sidebar_enabled was written (%v); it must stay unset", *m.UserConfig.Appearance.SidebarEnabled)
	}

	m.ToggleSidebar()
	if got := m.UserConfig.Appearance.Sidebar.Enabled; got == nil || *got {
		t.Fatalf("[appearance.sidebar] enabled = %v after toggling off, want false", got)
	}
}
