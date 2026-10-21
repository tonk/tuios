package config

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Reasons CleanConfig gives for removing a setting.
const (
	// CleanDefault is a setting written at its default value.
	CleanDefault = "default"
	// CleanNoEffect is a setting whose removal changes nothing tuios loads:
	// an obsolete key, or a value such as 0 or "" that tuios reads as "use the
	// default".
	CleanNoEffect = "no effect"
)

// CleanedSetting is one setting CleanConfig removed.
type CleanedSetting struct {
	Line   int    // 1-based line it was on
	Key    string // dotted path, e.g. appearance.border_style
	Reason string // CleanDefault or CleanNoEffect
}

// CleanConfig returns data without the settings that do not change anything:
// those written at their default value, and those whose removal leaves the
// loaded configuration the same (obsolete keys, and 0 or "" where that means
// "use the default"). A comment directly above a removed setting goes with
// it, and a section left without settings is removed whole, comments and all.
// Everything else stays exactly as written, in the order it was written, so
// the '=' of the settings left over may no longer line up: ReformatConfig
// realigns them.
//
// Settings are judged one at a time against the file as it stands after the
// removals before them, so two settings that only have no effect because of
// each other (an old flat key next to its replacement) are never both
// removed. [env] and [hooks] have no defaults, so nothing in them is ever
// removed.
//
// A file that does not decode is refused: run CheckConfig on it first.
func CleanConfig(data []byte) ([]byte, []CleanedSetting, error) {
	if _, err := parseUserConfig(data); err != nil {
		return nil, nil, fmt.Errorf("config does not parse, fix it first (see tuios config check): %w", err)
	}
	entries, err := scanTOML(data)
	if err != nil {
		return nil, nil, fmt.Errorf("config does not parse, fix it first (see tuios config check): %w", err)
	}

	lines := strings.SplitAfter(string(data), "\n")
	removed := make([]bool, len(lines)+2) // 1-based, with room past the end
	defaults := exampleDefaults()

	var cleaned []CleanedSetting
	var table []string
	for i, entry := range entries {
		switch entry.Kind {
		case tomlTable, tomlArrayTable:
			table = entry.Key
			continue
		case tomlKeyValue:
		default:
			continue
		}
		full := append(append([]string(nil), table...), entry.Key...)
		path := joinKeyPath(full)
		if len(full) > 0 && (full[0] == "env" || full[0] == "hooks") {
			continue
		}

		reason := ""
		if def, ok := defaults[path]; ok && sameTOMLValue(entry.Value, def) {
			reason = CleanDefault
		} else {
			// Compared with the file as it stands, not as it started: the
			// defaults already removed above load as unset rather than as their
			// value, which is the same setting but not the same struct.
			current, _ := parseUserConfig([]byte(joinKept(lines, removed)))
			for l := entry.Line; l <= entry.EndLine; l++ {
				removed[l] = true
			}
			if cfg, err := parseUserConfig([]byte(joinKept(lines, removed))); err == nil && reflect.DeepEqual(cfg, current) {
				reason = CleanNoEffect
			}
			for l := entry.Line; l <= entry.EndLine; l++ {
				removed[l] = false
			}
		}
		if reason == "" {
			continue
		}

		for l := entry.Line; l <= entry.EndLine; l++ {
			removed[l] = true
		}
		removeAttachedComments(entries, i, removed)
		cleaned = append(cleaned, CleanedSetting{Line: entry.Line, Key: path, Reason: reason})
	}

	removeEmptySections(entries, removed)
	result := []byte(collapseBlankLines(joinKept(lines, removed)))

	if _, err := parseUserConfig(result); err != nil {
		return nil, nil, fmt.Errorf("cleaning produced invalid TOML, file left alone: %w", err)
	}
	return result, cleaned, nil
}

// exampleDefaults maps every scalar setting's dotted path to its default, as
// written in exampleTables, plus every keybinding action to its default keys.
func exampleDefaults() map[string]string {
	out := map[string]string{}
	for _, tbl := range exampleTables {
		for _, f := range tbl.Fields {
			out[tbl.Path+"."+f.Key] = f.Example
		}
	}
	def := DefaultConfig()
	for _, sec := range keybindSectionDocs {
		for action, keys := range sec.Get(def) {
			out["keybindings."+sec.Key+"."+action] = tomlStringArray(keys)
		}
	}
	return out
}

// sameTOMLValue reports whether two TOML values, as written, decode to the
// same thing: "rounded" and 'rounded', or [ "a" ] and ["a"].
func sameTOMLValue(a, b string) bool {
	var va, vb map[string]any
	if toml.Unmarshal([]byte("v = "+a), &va) != nil || toml.Unmarshal([]byte("v = "+b), &vb) != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}

// removeAttachedComments marks the comment lines directly above entry i (no
// blank line in between) as removed, the block reformat moves with the key.
func removeAttachedComments(entries []tomlEntry, i int, removed []bool) {
	next := entries[i].Line
	for j := i - 1; j >= 0 && entries[j].Kind == tomlComment && entries[j].Line == next-1; j-- {
		removed[entries[j].Line] = true
		next = entries[j].Line
	}
}

// removeEmptySections marks a section left without settings as removed too:
// its header, the comments directly above the header, and every comment
// inside it. That includes a section that was empty to begin with, which sets
// nothing either (tuios writes some of those itself when it saves).
func removeEmptySections(entries []tomlEntry, removed []bool) {
	for i, entry := range entries {
		if entry.Kind != tomlTable {
			continue
		}
		kept := false
		end := len(entries)
		for j := i + 1; j < len(entries); j++ {
			if entries[j].Kind == tomlTable || entries[j].Kind == tomlArrayTable {
				end = j
				break
			}
			if entries[j].Kind == tomlKeyValue {
				kept = kept || !removed[entries[j].Line]
			}
		}
		if kept {
			continue
		}
		removed[entry.Line] = true
		removeAttachedComments(entries, i, removed)
		for j := i + 1; j < end; j++ {
			removed[entries[j].Line] = true
		}
	}
}

// joinKept returns the lines not marked removed (removed is 1-based).
func joinKept(lines []string, removed []bool) string {
	var b strings.Builder
	for i, line := range lines {
		if !removed[i+1] {
			b.WriteString(line)
		}
	}
	return b.String()
}

// collapseBlankLines turns every run of blank lines into one and drops the
// blank lines at the start and end, which removed sections leave behind.
func collapseBlankLines(s string) string {
	var out []string
	blank := true // drops leading blank lines
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			if !blank {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, line)
		blank = false
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
}
