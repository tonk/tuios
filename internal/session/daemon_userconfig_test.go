package session

import (
	"testing"
	"time"

	"github.com/tonk/tuios/internal/config"
)

// TestDaemonConfigApplyUserConfig pins that the [daemon] keys reach a daemon
// through ApplyUserConfig, the one path both a standalone `tuios daemon` and
// the daemon tuios ssh and tuios-web start in-process use. The in-process
// daemon used to be built with an empty DaemonConfig and ignore them all.
func TestDaemonConfigApplyUserConfig(t *testing.T) {
	prevLevel := GetDebugLevel()
	t.Cleanup(func() { SetDebugLevel(prevLevel) })
	t.Setenv("TUIOS_AGENT_AUTODETECT", "")
	t.Setenv("TUIOS_AGENT_DETECT_SECONDS", "")
	t.Setenv("TUIOS_AGENT_BINARIES", "")
	// NewDaemon loads harness manifests from the user config directory.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	t.Run("agent keys and log level reach the daemon", func(t *testing.T) {
		SetDebugLevel(DebugOff)
		user := config.DefaultConfig()
		user.Daemon.LogLevel = "basic"
		user.Daemon.AgentDetectSeconds = 7
		user.Daemon.AgentBinaries = []string{"forge"}
		user.Daemon.ExitWhenEmpty = true

		cfg := &DaemonConfig{Version: "in-process"}
		cfg.ApplyUserConfig(user)

		if got := GetDebugLevel(); got != DebugBasic {
			t.Errorf("debug level = %v, want %v from log_level", got, DebugBasic)
		}
		if cfg.ExitWhenEmpty {
			t.Error("ApplyUserConfig set ExitWhenEmpty; that is the standalone daemon's to decide")
		}

		d := NewDaemon(cfg)
		if d.agentDetectInterval != 7*time.Second {
			t.Errorf("agent detect interval = %v, want 7s from agent_detect_seconds", d.agentDetectInterval)
		}
		if _, ok := d.agentMatcher.names["forge"]; !ok {
			t.Error("agent_binaries entry \"forge\" did not reach the agent matcher")
		}
	})

	t.Run("agent_autodetect false disables detection", func(t *testing.T) {
		off := false
		user := config.DefaultConfig()
		user.Daemon.AgentAutoDetect = &off

		cfg := &DaemonConfig{}
		cfg.ApplyUserConfig(user)
		if d := NewDaemon(cfg); d.agentDetectInterval != 0 {
			t.Errorf("agent detect interval = %v with agent_autodetect = false, want 0 (off)", d.agentDetectInterval)
		}
	})

	t.Run("an explicit debug level wins over log_level", func(t *testing.T) {
		SetDebugLevel(DebugVerbose)
		user := config.DefaultConfig()
		user.Daemon.LogLevel = "errors"

		(&DaemonConfig{}).ApplyUserConfig(user)
		if got := GetDebugLevel(); got != DebugVerbose {
			t.Errorf("debug level = %v, want %v set by TUIOS_LOG_LEVEL or --log-level", got, DebugVerbose)
		}
	})

	t.Run("nil config changes nothing", func(t *testing.T) {
		SetDebugLevel(DebugOff)
		cfg := &DaemonConfig{}
		cfg.ApplyUserConfig(nil)
		if cfg.AgentAutoDetect != nil || cfg.AgentDetectInterval != 0 || cfg.AgentBinaries != nil || GetDebugLevel() != DebugOff {
			t.Errorf("ApplyUserConfig(nil) changed settings: %+v, level %v", cfg, GetDebugLevel())
		}
	})
}
