package config

import "testing"

func TestClassroomIsTrainer(t *testing.T) {
	cases := []struct {
		name     string
		cfg      ClassroomConfig
		username string
		want     bool
	}{
		{
			name:     "console off, user listed",
			cfg:      ClassroomConfig{TrainerConsole: false, TrainerUsers: []string{"ton"}},
			username: "ton",
			want:     false,
		},
		{
			name:     "console on, user listed",
			cfg:      ClassroomConfig{TrainerConsole: true, TrainerUsers: []string{"ton"}},
			username: "ton",
			want:     true,
		},
		{
			name:     "console on, user not listed",
			cfg:      ClassroomConfig{TrainerConsole: true, TrainerUsers: []string{"ton"}},
			username: "guru07",
			want:     false,
		},
		{
			name:     "console on, empty allowlist",
			cfg:      ClassroomConfig{TrainerConsole: true},
			username: "ton",
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.IsTrainer(tc.username); got != tc.want {
				t.Errorf("IsTrainer(%q) = %v, want %v", tc.username, got, tc.want)
			}
		})
	}
}

func TestClassroomMatchesTrainee(t *testing.T) {
	cases := []struct {
		name        string
		pattern     string
		sessionName string
		want        bool
	}{
		{"empty pattern matches nothing", "", "guru07", false},
		{"malformed pattern matches nothing", "guru[0-9", "guru07", false},
		{"matching session", "^guru[0-9]{2}$", "guru07", true},
		{"non-matching session", "^guru[0-9]{2}$", "root", false},
		{"pattern anchors reject partial match", "^guru[0-9]{2}$", "guru075", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ClassroomConfig{TraineePattern: tc.pattern}
			if got := cfg.MatchesTrainee(tc.sessionName); got != tc.want {
				t.Errorf("MatchesTrainee(%q) with pattern %q = %v, want %v", tc.sessionName, tc.pattern, got, tc.want)
			}
		})
	}
}
