package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
	"github.com/tonk/tuios/internal/config"
)

// TestLoadTapeFilesFiltersByExtension pins the bug fix: only files matching
// the configured extensions are listed, so a shared .lua helper module that
// other tape scripts require (but which isn't a tape itself) is filtered out.
func TestLoadTapeFilesFiltersByExtension(t *testing.T) {
	t.Cleanup(xdg.Reload)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	xdg.Reload()

	tapeDir, err := GetTapeDirectory()
	if err != nil {
		t.Fatalf("GetTapeDirectory: %v", err)
	}

	names := []string{"a.tape", "b.tape.lua", "helper.lua", "notes.txt"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(tapeDir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	files, err := LoadTapeFiles(config.DefaultTapeExtensions)
	if err != nil {
		t.Fatalf("LoadTapeFiles: %v", err)
	}

	got := map[string]TapeFile{}
	for _, f := range files {
		got[f.Name] = f
	}

	if len(got) != 2 {
		t.Fatalf("got %d files, want 2 (a, b): %+v", len(got), got)
	}
	if f, ok := got["a"]; !ok || f.Kind != TapeFileDSL || f.Ext != ".tape" {
		t.Fatalf("a.tape not loaded as expected: %+v (ok=%v)", f, ok)
	}
	if f, ok := got["b"]; !ok || f.Kind != TapeFileLua || f.Ext != ".tape.lua" {
		t.Fatalf("b.tape.lua not loaded as expected: %+v (ok=%v)", f, ok)
	}
	if _, ok := got["helper"]; ok {
		t.Fatal("helper.lua should have been filtered out")
	}
}

// TestLoadTapeFilesCustomExtensions checks that a configured extension list
// beyond the defaults is honored, e.g. restoring the old permissive behavior
// of listing any .lua file.
func TestLoadTapeFilesCustomExtensions(t *testing.T) {
	t.Cleanup(xdg.Reload)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	xdg.Reload()

	tapeDir, err := GetTapeDirectory()
	if err != nil {
		t.Fatalf("GetTapeDirectory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tapeDir, "helper.lua"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	files, err := LoadTapeFiles([]string{".lua"})
	if err != nil {
		t.Fatalf("LoadTapeFiles: %v", err)
	}
	if len(files) != 1 || files[0].Name != "helper" || files[0].Kind != TapeFileLua {
		t.Fatalf("custom extensions not honored: %+v", files)
	}
}

// TestLoadTapeFilesEmptyExtensionsFallsBackToDefault ensures a nil/empty
// extensions list (e.g. an old config loaded before this option existed)
// still filters like the built-in default rather than showing nothing or
// everything.
func TestLoadTapeFilesEmptyExtensionsFallsBackToDefault(t *testing.T) {
	t.Cleanup(xdg.Reload)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	xdg.Reload()

	tapeDir, err := GetTapeDirectory()
	if err != nil {
		t.Fatalf("GetTapeDirectory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tapeDir, "a.tape"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	files, err := LoadTapeFiles(nil)
	if err != nil {
		t.Fatalf("LoadTapeFiles: %v", err)
	}
	if len(files) != 1 || files[0].Name != "a" {
		t.Fatalf("nil extensions should fall back to defaults: %+v", files)
	}
}
