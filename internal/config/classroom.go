package config

import (
	"regexp"
	"slices"
)

// IsTrainer reports whether username is authorized to open the trainer
// console and cross-attach to another trainee's session. This is the actual
// access-control decision - callers must use it instead of re-deriving it
// from TrainerUsers/TrainerConsole themselves. TrainerConsole is the master
// switch: it always wins, even if username is listed in TrainerUsers.
func (c *ClassroomConfig) IsTrainer(username string) bool {
	return c.TrainerConsole && slices.Contains(c.TrainerUsers, username)
}

// CompileTraineePattern compiles a trainee_pattern so it must match a whole
// username. The pattern is wrapped as ^(?:pattern)$: Go's MatchString finds a
// match anywhere in the string, so an unanchored "guru" would otherwise also
// accept "superguru7" or "guru-admin" as attach targets. A pattern that
// already carries its own ^ and $ behaves exactly as before.
func CompileTraineePattern(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(`^(?:` + pattern + `)$`)
}

// MatchesTrainee reports whether sessionName - a trainee's own username - is
// a valid cross-attach target under TraineePattern, matched against the whole
// name (see CompileTraineePattern). An empty or malformed pattern matches
// nothing; validateClassroomConfig warns about both at load time, so this
// fails closed rather than raising an error here.
func (c *ClassroomConfig) MatchesTrainee(sessionName string) bool {
	if c.TraineePattern == "" {
		return false
	}
	re, err := CompileTraineePattern(c.TraineePattern)
	if err != nil {
		return false
	}
	return re.MatchString(sessionName)
}
