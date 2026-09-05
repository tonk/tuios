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

// MatchesTrainee reports whether sessionName - a trainee's own username - is
// eligible to appear in the trainer's picker, per TraineePattern. An empty
// or malformed pattern matches nothing; validateClassroomConfig warns about
// both at load time, so this fails closed rather than raising an error here.
func (c *ClassroomConfig) MatchesTrainee(sessionName string) bool {
	if c.TraineePattern == "" {
		return false
	}
	re, err := regexp.Compile(c.TraineePattern)
	if err != nil {
		return false
	}
	return re.MatchString(sessionName)
}
