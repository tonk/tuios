package config

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
)

// reformatItem is one key/value with the comment lines directly above it,
// which travel with it when the section is sorted.
type reformatItem struct {
	comments []string
	sortKey  string
	key      string
	value    string
	trailing string
}

// reformatSection is a [table] (or the keys before the first one) as
// ReformatConfig rebuilds it.
type reformatSection struct {
	headerComments []string // comment lines directly above the header
	header         string   // "[path]" as written, empty for the preamble
	trailing       string   // comment after the header on the same line
	top            [][]string
	items          []reformatItem
	bottom         [][]string
}

// ReformatConfig returns data with the keys of every section sorted
// alphabetically and their '=' signs aligned. Sections stay in the order they
// are written in.
//
// Comments are kept: a comment block directly above a key (no blank line in
// between) moves with that key, one directly above a section header stays with
// the header, and any other comment block stays at the top or bottom of its
// section, wherever it was relative to the section's keys. Blank lines between
// keys are not kept.
//
// The result is decoded and compared with the original before it is returned,
// so a reformat that would change any setting's value is refused rather than
// written. A file that doesn't decode (a syntax error, a duplicate key) is
// refused too: run CheckConfig on it first.
func ReformatConfig(data []byte) ([]byte, error) {
	var before map[string]any
	if err := toml.Unmarshal(data, &before); err != nil {
		return nil, fmt.Errorf("config does not parse, fix it first (see tuios config check): %w", err)
	}
	entries, err := scanTOML(data)
	if err != nil {
		return nil, fmt.Errorf("config does not parse, fix it first (see tuios config check): %w", err)
	}

	sections := []*reformatSection{{}}
	current := sections[0]
	var pending []tomlEntry

	// flush files the comments read since the last key or header, now that the
	// entry they precede (next, or nil at the end of the file) is known. It
	// returns the comment lines that belong to next.
	flush := func(next *tomlEntry) []string {
		var runs [][]tomlEntry
		for i, c := range pending {
			if i == 0 || c.Line != pending[i-1].Line+1 {
				runs = append(runs, nil)
			}
			runs[len(runs)-1] = append(runs[len(runs)-1], c)
		}
		pending = nil

		var attached []string
		if n := len(runs); n > 0 && next != nil && runs[n-1][len(runs[n-1])-1].Line+1 == next.Line {
			attached = commentLines(runs[n-1])
			runs = runs[:n-1]
		}
		for _, run := range runs {
			if len(current.items) == 0 {
				current.top = append(current.top, commentLines(run))
			} else {
				current.bottom = append(current.bottom, commentLines(run))
			}
		}
		return attached
	}

	for i := range entries {
		entry := entries[i]
		switch entry.Kind {
		case tomlComment:
			pending = append(pending, entry)
		case tomlTable, tomlArrayTable:
			attached := flush(&entry)
			header := "[" + entry.RawKey + "]"
			if entry.Kind == tomlArrayTable {
				header = "[" + header + "]"
			}
			current = &reformatSection{headerComments: attached, header: header, trailing: entry.Comment}
			sections = append(sections, current)
		case tomlKeyValue:
			attached := flush(&entry)
			current.items = append(current.items, reformatItem{
				comments: attached,
				sortKey:  joinKeyPath(entry.Key),
				key:      entry.RawKey,
				value:    entry.Value,
				trailing: entry.Comment,
			})
		}
	}
	flush(nil)

	var out strings.Builder
	for _, section := range sections {
		writeReformatSection(&out, section)
	}
	result := []byte(strings.TrimLeft(out.String(), "\n"))

	var after map[string]any
	if err := toml.Unmarshal(result, &after); err != nil {
		return nil, fmt.Errorf("reformatting produced invalid TOML, file left alone: %w", err)
	}
	if !reflect.DeepEqual(before, after) {
		return nil, fmt.Errorf("reformatting would change the configuration, file left alone")
	}
	return result, nil
}

func commentLines(run []tomlEntry) []string {
	lines := make([]string, len(run))
	for i, c := range run {
		lines[i] = c.Comment
	}
	return lines
}

// naturalLess orders strings with runs of digits compared by value, so
// select_window_2 sorts before select_window_10.
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			na, nb := digitRun(a), digitRun(b)
			x, y := strings.TrimLeft(a[:na], "0"), strings.TrimLeft(b[:nb], "0")
			if len(x) != len(y) {
				return len(x) < len(y)
			}
			if x != y {
				return x < y
			}
			a, b = a[na:], b[nb:]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func digitRun(s string) int {
	n := 0
	for n < len(s) && isDigit(s[n]) {
		n++
	}
	return n
}

// writeReformatSection writes one section, separated from what came before by
// a blank line, with its keys sorted and aligned.
func writeReformatSection(out *strings.Builder, section *reformatSection) {
	var blocks [][]string

	top := section.top
	if section.header != "" {
		header := section.header
		if section.trailing != "" {
			header += " " + section.trailing
		}
		block := append(section.headerComments, header)
		// A loose comment block right under the header reads as the
		// section's description, so it stays against the header.
		if len(top) > 0 {
			block = append(block, top[0]...)
			top = top[1:]
		}
		blocks = append(blocks, block)
	}
	blocks = append(blocks, top...)

	if len(section.items) > 0 {
		items := section.items
		sort.SliceStable(items, func(i, j int) bool { return naturalLess(items[i].sortKey, items[j].sortKey) })

		width := 0
		for _, item := range items {
			width = max(width, utf8.RuneCountInString(item.key))
		}
		var lines []string
		for i, item := range items {
			if i > 0 && len(item.comments) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, item.comments...)
			line := fmt.Sprintf("%-*s = %s", width, item.key, item.value)
			if item.trailing != "" {
				line += " " + item.trailing
			}
			lines = append(lines, line)
		}

		// Keys follow their header directly, unless a loose comment block sits
		// between them and needs its blank line to stay loose.
		if section.header != "" && len(section.top) == 0 {
			blocks[0] = append(blocks[0], lines...)
		} else {
			blocks = append(blocks, lines)
		}
	}
	blocks = append(blocks, section.bottom...)

	for _, block := range blocks {
		out.WriteString("\n")
		for _, line := range block {
			out.WriteString(line)
			out.WriteString("\n")
		}
	}
}
