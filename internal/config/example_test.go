package config

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/tonk/tuios/internal/hooks"
	"github.com/pelletier/go-toml/v2"
)

// TestExampleConfigCoversAllFields walks UserConfig by reflection and fails
// if a scalar field's TOML path has no entry in exampleTables. This is what
// keeps GenerateExampleConfig from silently going stale: add a field to
// UserConfig without adding it here, and this test - not a human noticing a
// missing line in a generated file - is what catches it.
func TestExampleConfigCoversAllFields(t *testing.T) {
	documented := map[string]bool{}
	for _, tbl := range exampleTables {
		for _, f := range tbl.Fields {
			documented[tbl.Path+"."+f.Key] = true
		}
	}

	var walk func(rt reflect.Type, path string)
	walk = func(rt reflect.Type, path string) {
		for i := 0; i < rt.NumField(); i++ {
			field := rt.Field(i)
			tag := field.Tag.Get("toml")
			if tag == "" || tag == "-" {
				continue
			}
			parts := strings.Split(tag, ",")
			name := parts[0]
			// Legacy flat sidebar_* keys carry ,omitempty and are migrated
			// into [appearance.sidebar] rather than documented on their own.
			if len(parts) > 1 && parts[1] == "omitempty" {
				continue
			}

			key := name
			if path != "" {
				key = path + "." + name
			}

			ft := field.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}

			switch ft.Kind() {
			case reflect.Struct:
				walk(ft, key)
			case reflect.Map:
				// Keybinding sections and [hooks]: documented separately, see
				// TestExampleConfigCoversAllKeybindSections and
				// TestExampleConfigCoversAllHookEvents.
			default:
				if !documented[key] {
					t.Errorf("field %q has no entry in exampleTables (internal/config/example.go); "+
						"add one so `tuios config example` documents it", key)
				}
			}
		}
	}

	walk(reflect.TypeFor[UserConfig](), "")
}

// TestExampleConfigCoversAllKeybindSections fails if KeybindingsConfig gains
// a map-typed (i.e. keybinding-section) field with no entry in
// keybindSectionDocs.
func TestExampleConfigCoversAllKeybindSections(t *testing.T) {
	documented := map[string]bool{}
	for _, s := range keybindSectionDocs {
		documented[s.Key] = true
	}

	rt := reflect.TypeFor[KeybindingsConfig]()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if field.Type.Kind() != reflect.Map {
			continue
		}
		tag := strings.Split(field.Tag.Get("toml"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		if !documented[tag] {
			t.Errorf("keybindings section %q has no entry in keybindSectionDocs (internal/config/example.go)", tag)
		}
	}
}

// TestExampleConfigCoversAllHookEvents fails if hooks.AllEvents() gains an
// event with no entry in hookEventDescriptions.
func TestExampleConfigCoversAllHookEvents(t *testing.T) {
	for _, event := range hooks.AllEvents() {
		if _, ok := hookEventDescriptions[event]; !ok {
			t.Errorf("hook event %q has no entry in hookEventDescriptions (internal/config/example.go)", event)
		}
	}
}

// uncommentableLine matches a documented "# key = value ..." line so it can
// be uncommented for the round-trip parse test below, while leaving plain
// prose comment lines (which don't look like "key = value") alone.
var uncommentableLine = regexp.MustCompile(`^# ([A-Za-z0-9_.-]+ = .*)$`)

// TestGenerateExampleConfigParses uncomments every documented key in the
// generated file and parses the result as TOML, catching a malformed literal
// (unbalanced quotes, a bad array) before it ships. It also spot-checks a
// handful of values round-trip to the real defaults.
func TestGenerateExampleConfigParses(t *testing.T) {
	content := GenerateExampleConfig()

	var uncommented strings.Builder
	for line := range strings.SplitSeq(content, "\n") {
		if m := uncommentableLine.FindStringSubmatch(line); m != nil {
			uncommented.WriteString(m[1])
		} else {
			uncommented.WriteString(line)
		}
		uncommented.WriteString("\n")
	}

	var cfg UserConfig
	if err := toml.Unmarshal([]byte(uncommented.String()), &cfg); err != nil {
		t.Fatalf("generated example config does not parse once uncommented: %v\n---\n%s", err, uncommented.String())
	}

	def := DefaultConfig()
	if cfg.Appearance.BorderStyle != def.Appearance.BorderStyle {
		t.Errorf("appearance.border_style = %q, want %q", cfg.Appearance.BorderStyle, def.Appearance.BorderStyle)
	}
	if cfg.Keybindings.LeaderKey != def.Keybindings.LeaderKey {
		t.Errorf("keybindings.leader_key = %q, want %q", cfg.Keybindings.LeaderKey, def.Keybindings.LeaderKey)
	}
	if cfg.Notifications.Agent.States.NeedsInput == nil || !*cfg.Notifications.Agent.States.NeedsInput {
		t.Error("notifications.agent.states.needs_input did not round-trip to true")
	}
	if got := cfg.Keybindings.WindowManagement["new_window"]; len(got) == 0 {
		t.Error("keybindings.window_management.new_window did not round-trip to any keys")
	}
}
