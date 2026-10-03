package config

import "testing"

func TestOverlayUserSections(t *testing.T) {
	t.Run("takes appearance key by key", func(t *testing.T) {
		base := DefaultConfig()
		base.Appearance.BorderStyle = "thick"
		if err := OverlayUserSections(base, []byte("[appearance]\ntheme = \"nord\"\n")); err != nil {
			t.Fatal(err)
		}
		if base.Appearance.Theme != "nord" {
			t.Errorf("theme = %q, want nord", base.Appearance.Theme)
		}
		if base.Appearance.BorderStyle != "thick" {
			t.Errorf("border_style = %q, want the base's thick kept", base.Appearance.BorderStyle)
		}
	})

	t.Run("ignores sections that run as the server", func(t *testing.T) {
		base := DefaultConfig()
		src := "[classroom]\ntrainer_console = true\ntrainer_users = [\"me\"]\n" +
			"[startup]\nopen_default_window = true\n" +
			"[env]\nX = \"y\"\n" +
			"[appearance]\ntheme = \"nord\"\n"
		if err := OverlayUserSections(base, []byte(src)); err != nil {
			t.Fatal(err)
		}
		if base.Classroom.TrainerConsole || len(base.Classroom.TrainerUsers) != 0 {
			t.Errorf("classroom was taken from the user's file: %+v", base.Classroom)
		}
		if base.Startup.OpenDefaultWindow || len(base.Env) != 0 {
			t.Errorf("startup or env was taken from the user's file")
		}
		if base.Appearance.Theme != "nord" {
			t.Errorf("theme = %q, want nord", base.Appearance.Theme)
		}
	})

	t.Run("leaves base alone on a bad file", func(t *testing.T) {
		base := DefaultConfig()
		before := base.Appearance.Theme
		if err := OverlayUserSections(base, []byte("[appearance\n")); err == nil {
			t.Fatal("expected a parse error")
		}
		if base.Appearance.Theme != before {
			t.Errorf("base changed on error")
		}
	})
}
