package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserSettingsPath(t *testing.T) {
	t.Cleanup(func(old string) func() { return func() { webUserConfigDir = old } }(webUserConfigDir))
	webUserConfigDir = "/state/users"

	tests := []struct {
		owner   string
		want    string
		wantErr bool
	}{
		{"guru01", "/state/users/guru01.toml", false},
		{"first.last", "/state/users/first.last.toml", false},
		{"", "", true},
		{"..", "", true},
		{"../etc/passwd", "", true},
		{"a/b", "", true},
		{".hidden", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.owner, func(t *testing.T) {
			got, err := userSettingsPath(tt.owner)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("userSettingsPath(%q) = %q, %v; want %q (err %v)", tt.owner, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestLoadWebSessionConfigNoOwnerSavesNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Cleanup(func(old string) func() { return func() { webUserConfigDir = old } }(webUserConfigDir))
	webUserConfigDir = filepath.Join(dir, "users")

	cfg, save, err := loadWebSessionConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if err := save(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(webUserConfigDir); !os.IsNotExist(err) {
		t.Errorf("a connection with no identity wrote settings: %v", err)
	}
}

func TestReadUserFileRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.toml")
	if err := os.WriteFile(target, []byte("[appearance]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, ok := readUserFile(target); !ok {
		t.Error("a regular file was refused")
	}
	if _, ok := readUserFile(link); ok {
		t.Error("a symlink was read")
	}
}
