package config

import "testing"

func TestWindowTitleFixed(t *testing.T) {
	oldFormat, oldLock := WindowTitleFormat, LockTitles
	t.Cleanup(func() { WindowTitleFormat, LockTitles = oldFormat, oldLock })

	tests := []struct {
		name       string
		format     string
		lock       bool
		wantLocked bool
		wantTitle  string
	}{
		{"default", "", false, false, "bash"},
		{"lock_titles", "", true, true, "bash"},
		{"template", "{index}: {title}", false, false, "2: bash"},
		{"fixed", WindowTitleFixed, false, true, "bash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			WindowTitleFormat, LockTitles = tt.format, tt.lock
			if got := TitlesLocked(); got != tt.wantLocked {
				t.Errorf("TitlesLocked() = %v, want %v", got, tt.wantLocked)
			}
			if got := FormatWindowTitle("bash", 2, "/tmp"); got != tt.wantTitle {
				t.Errorf("FormatWindowTitle() = %q, want %q", got, tt.wantTitle)
			}
		})
	}
}
