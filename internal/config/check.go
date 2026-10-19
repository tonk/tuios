package config

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/tonk/tuios/internal/hooks"
)

// Severity of a ConfigIssue.
const (
	// IssueError is a problem that stops tuios from loading the file.
	IssueError = "error"
	// IssueWarning is a problem tuios loads past: the setting is ignored or
	// falls back to its default.
	IssueWarning = "warning"
)

// Kinds of ConfigIssue.
const (
	IssueSyntax    = "syntax"
	IssueInvalid   = "invalid"
	IssueObsolete  = "obsolete"
	IssueDuplicate = "duplicate"
)

// ConfigIssue is one problem CheckConfig found in a config file.
type ConfigIssue struct {
	Line     int    // 1-based line the problem is on, 0 when it can't be pinned to one
	Severity string // IssueError or IssueWarning
	Kind     string // IssueSyntax, IssueInvalid, IssueObsolete or IssueDuplicate
	Key      string // dotted path of the setting, when there is one
	Message  string
}

// obsoleteKeys maps a setting tuios still parses but no longer wants written to
// what replaces it.
var obsoleteKeys = map[string]string{
	"appearance.hide_clock":           "deprecated, use show_clock instead",
	"appearance.sidebar_enabled":      "moved to [appearance.sidebar] as enabled",
	"appearance.sidebar_position":     "moved to [appearance.sidebar] as position",
	"appearance.sidebar_width":        "moved to [appearance.sidebar] as width",
	"appearance.sidebar_show_windows": "moved to [appearance.sidebar] as show_windows",
	"appearance.sidebar_show_glyphs":  "moved to [appearance.sidebar] as show_glyphs",
	"appearance.sidebar_show_counts":  "moved to [appearance.sidebar] as show_counts",
	"appearance.sidebar.workspaces":   "no longer used: panes name their own workspace, and switching lives on the dock and alt+1..9",
}

// obsoleteActions maps a keybinding action tuios no longer has to why.
var obsoleteActions = map[string]string{
	"extend_up":             "removed with selection mode; copy mode (leader [) replaced it",
	"extend_down":           "removed with selection mode; copy mode (leader [) replaced it",
	"extend_left":           "removed with selection mode; copy mode (leader [) replaced it",
	"extend_right":          "removed with selection mode; copy mode (leader [) replaced it",
	"toggle_selection":      "removed with selection mode; copy mode (leader [) replaced it",
	"toggle_selection_term": "removed with selection mode; copy mode (leader [) replaced it",
}

// CheckConfig reports the invalid, obsolete and duplicate settings in the
// config file data, sorted by line. Unlike LoadUserConfig it keeps going past
// the first problem and pins each one to a line where it can.
func CheckConfig(data []byte) []ConfigIssue {
	entries, err := scanTOML(data)
	if err != nil {
		line := 0
		var scanErr *tomlScanError
		if errors.As(err, &scanErr) {
			line, err = scanErr.Line, scanErr.Err
		}
		return []ConfigIssue{{Line: line, Severity: IssueError, Kind: IssueSyntax, Message: err.Error()}}
	}

	var issues []ConfigIssue
	add := func(line int, severity, kind, key, format string, args ...any) {
		issues = append(issues, ConfigIssue{
			Line: line, Severity: severity, Kind: kind, Key: key,
			Message: fmt.Sprintf(format, args...),
		})
	}

	actions := knownActions()
	tables := map[string]int{}
	lines := map[string]int{}
	var table []string
	// Keys under an unknown section aren't reported one by one: the section
	// already was.
	tableUnknown := false
	for _, entry := range entries {
		switch entry.Kind {
		case tomlTable:
			table, tableUnknown = entry.Key, false
			path := joinKeyPath(table)
			if first, ok := tables[path]; ok {
				add(entry.Line, IssueError, IssueDuplicate, path, "section [%s] is already defined on line %d", path, first)
				continue
			}
			tables[path] = entry.Line
			if t, ok := lookupConfigType(table); !ok || !isTableType(t) {
				tableUnknown = true
				add(entry.Line, IssueWarning, IssueInvalid, path, "unknown section [%s]", path)
			}
		case tomlArrayTable:
			table, tableUnknown = entry.Key, true
			path := joinKeyPath(table)
			add(entry.Line, IssueWarning, IssueInvalid, path, "unknown section [[%s]]", path)
		case tomlKeyValue:
			full := append(slices.Clone(table), entry.Key...)
			path := joinKeyPath(full)
			if first, ok := lines[path]; ok {
				add(entry.Line, IssueError, IssueDuplicate, path, "%s is already set on line %d", path, first)
				continue
			}
			lines[path] = entry.Line
			// tuios itself writes hide_clock and sidebar.workspaces back at their
			// zero value on every save, so only a value someone chose counts.
			if reason, ok := obsoleteKeys[path]; ok {
				if !isZeroTOMLValue(entry.Value) {
					add(entry.Line, IssueWarning, IssueObsolete, path, "%s: %s", path, reason)
				}
				continue
			}
			if _, ok := lookupConfigType(full); !ok {
				if tableUnknown {
					continue
				}
				add(entry.Line, IssueWarning, IssueInvalid, path, "unknown setting %s", path)
				continue
			}
			switch {
			case len(full) == 3 && full[0] == "keybindings" && obsoleteActions[full[2]] != "":
				add(entry.Line, IssueWarning, IssueObsolete, path, "action %q in [keybindings.%s]: %s", full[2], full[1], obsoleteActions[full[2]])
			case len(full) == 3 && full[0] == "keybindings" && !actions[full[2]]:
				add(entry.Line, IssueWarning, IssueInvalid, path, "unknown action %q in [keybindings.%s]", full[2], full[1])
			case len(full) == 2 && full[0] == "hooks":
				if _, ok := hooks.ParseEventName(full[1]); !ok {
					add(entry.Line, IssueWarning, IssueInvalid, path, "unknown hook event %q", full[1])
				}
			}
		}
	}

	// A duplicate key makes the decoder refuse the whole file, so the checks
	// below that need a decoded config only run without one.
	if !slices.ContainsFunc(issues, func(i ConfigIssue) bool { return i.Severity == IssueError }) {
		issues = append(issues, checkDecodedConfig(data, lines)...)
	}

	sort.SliceStable(issues, func(i, j int) bool { return issues[i].Line < issues[j].Line })
	return issues
}

// checkDecodedConfig decodes data and runs the checks that need values rather
// than spelling: wrong types, a key listed twice for one action, and
// everything ValidateConfig reports. lines maps dotted setting paths to the
// line they are set on.
func checkDecodedConfig(data []byte, lines map[string]int) []ConfigIssue {
	var cfg UserConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		issue := ConfigIssue{Severity: IssueError, Kind: IssueInvalid, Message: err.Error()}
		var decodeErr *toml.DecodeError
		if errors.As(err, &decodeErr) {
			issue.Line, _ = decodeErr.Position()
			issue.Key = joinKeyPath(decodeErr.Key())
		}
		return []ConfigIssue{issue}
	}

	var issues []ConfigIssue
	for _, section := range keybindSectionNames(&cfg) {
		for action, keys := range section.binds {
			for i, key := range keys {
				if slices.Contains(keys[:i], key) {
					path := "keybindings." + section.name + "." + action
					issues = append(issues, ConfigIssue{
						Line: lines[path], Severity: IssueWarning, Kind: IssueDuplicate, Key: path,
						Message: fmt.Sprintf("%s lists key %q more than once", path, key),
					})
				}
			}
		}
	}

	defaultCfg := DefaultConfig()
	fillMissingAppearance(&cfg, defaultCfg)
	fillMissingDaemon(&cfg, defaultCfg)
	fillMissingTape(&cfg, defaultCfg)
	fillMissingKeybinds(&cfg, defaultCfg)

	locate := func(issue ValidationError) (string, int) {
		for _, path := range []string{
			issue.Field + "." + issue.Key,
			"keybindings." + issue.Field + "." + issue.Key,
			"keybindings." + issue.Field,
			issue.Field,
		} {
			if line, ok := lines[path]; ok {
				return path, line
			}
		}
		return "", 0
	}
	validation := ValidateConfig(&cfg)
	report := func(list []ValidationError, severity string) {
		for _, v := range list {
			// Already reported, with its line, as obsolete.
			if _, ok := obsoleteKeys[v.Field]; ok {
				continue
			}
			kind := IssueInvalid
			if strings.Contains(v.Message, "bound to multiple actions") {
				kind = IssueDuplicate
			}
			path, line := locate(v)
			if path == "" {
				path = v.Field
			}
			message := v.Message
			if v.Key != "" && !strings.Contains(message, v.Key) {
				message = v.Key + ": " + message
			}
			issues = append(issues, ConfigIssue{
				Line: line, Severity: severity, Kind: kind, Key: path,
				Message: fmt.Sprintf("[%s] %s", v.Field, message),
			})
		}
	}
	report(validation.Errors, IssueError)
	report(validation.Warnings, IssueWarning)
	return issues
}

// isZeroTOMLValue reports whether a raw TOML value is its type's zero value.
func isZeroTOMLValue(value string) bool {
	switch value {
	case "false", "0", `""`, "''", "[]":
		return true
	}
	return false
}

// namedKeybindSection is a keybinding section with its TOML table name.
type namedKeybindSection struct {
	name  string
	binds map[string][]string
}

// keybindSectionNames lists cfg's keybinding sections under their TOML names,
// read off the struct tags so a new section can't be missed.
func keybindSectionNames(cfg *UserConfig) []namedKeybindSection {
	value := reflect.ValueOf(&cfg.Keybindings).Elem()
	var sections []namedKeybindSection
	for i := range value.NumField() {
		binds, ok := value.Field(i).Interface().(map[string][]string)
		if !ok {
			continue
		}
		sections = append(sections, namedKeybindSection{tomlName(value.Type().Field(i)), binds})
	}
	sort.Slice(sections, func(i, j int) bool { return sections[i].name < sections[j].name })
	return sections
}

// knownActions is every keybinding action name tuios recognizes.
func knownActions() map[string]bool {
	known := map[string]bool{}
	for action := range ActionDescriptions {
		known[action] = true
	}
	defaults := DefaultConfig()
	for _, section := range keybindSectionNames(defaults) {
		for action := range section.binds {
			known[action] = true
		}
	}
	return known
}

// lookupConfigType resolves a dotted TOML path against UserConfig's struct
// tags and returns the Go type it decodes into, or false when nothing in
// UserConfig reads that path. Map keys (actions, env variables, hook events)
// are accepted as anything.
func lookupConfigType(path []string) (reflect.Type, bool) {
	t := reflect.TypeFor[UserConfig]()
	for _, part := range path {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		switch t.Kind() {
		case reflect.Struct:
			found := false
			for i := range t.NumField() {
				if tomlName(t.Field(i)) == part {
					t, found = t.Field(i).Type, true
					break
				}
			}
			if !found {
				return nil, false
			}
		case reflect.Map:
			t = t.Elem()
		case reflect.Interface:
			return t, true
		default:
			return nil, false
		}
	}
	return t, true
}

// isTableType reports whether a value of type t is written as a TOML table.
func isTableType(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct, reflect.Map, reflect.Interface:
		return true
	}
	return false
}

// tomlName is the key a struct field is read from.
func tomlName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("toml"), ",")
	if name == "" {
		return field.Name
	}
	return name
}
