package config

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// tomlEntryKind says what a top-level line of a TOML document is.
type tomlEntryKind int

const (
	tomlComment tomlEntryKind = iota
	tomlTable
	tomlArrayTable
	tomlKeyValue
)

// tomlEntry is one top-level expression of a TOML document, with the source
// text it was written as. The decoder throws positions and spelling away and
// refuses a document with a duplicate key outright, so config check and
// config reformat read the file through this instead.
type tomlEntry struct {
	Kind    tomlEntryKind
	Line    int      // first line, 1-based
	EndLine int      // last line, which differs from Line for a multi-line value
	Key     []string // decoded key parts: the table path, or the key of a key/value
	RawKey  string   // the key as written, quotes and dots included
	Value   string   // the value as written, possibly spanning lines (key/values only)
	Comment string   // the comment itself, or the comment trailing a header or key/value
}

// tomlScanError is a syntax error with the line it was found on.
type tomlScanError struct {
	Line int
	Err  error
}

func (e *tomlScanError) Error() string {
	return fmt.Sprintf("line %d: %v", e.Line, e.Err)
}

func (e *tomlScanError) Unwrap() error { return e.Err }

// scanTOML splits data into its top-level expressions, comments included. It
// checks syntax only: a duplicate key or a value of the wrong type is not an
// error here.
func scanTOML(data []byte) ([]tomlEntry, error) {
	var parser unstable.Parser
	parser.KeepComments = true
	parser.Reset(data)

	var entries []tomlEntry
	for parser.NextExpression() {
		expr := parser.Expression()
		entry := tomlEntry{}
		switch expr.Kind {
		case unstable.Comment:
			entry.Kind = tomlComment
			entry.Comment = strings.TrimRight(string(expr.Data), " \t\r")
			entry.Line = parser.Shape(expr.Raw).Start.Line
			entry.EndLine = entry.Line
			entries = append(entries, entry)
			continue
		case unstable.Table:
			entry.Kind = tomlTable
		case unstable.ArrayTable:
			entry.Kind = tomlArrayTable
		case unstable.KeyValue:
			entry.Kind = tomlKeyValue
		default:
			continue
		}

		var keyStart, keyEnd uint32
		it := expr.Key()
		for it.Next() {
			node := it.Node()
			if len(entry.Key) == 0 {
				keyStart = node.Raw.Offset
			}
			keyEnd = node.Raw.Offset + node.Raw.Length
			entry.Key = append(entry.Key, string(node.Data))
		}
		entry.RawKey = string(data[keyStart:keyEnd])
		entry.Line = parser.Shape(unstable.Range{Offset: keyStart}).Start.Line
		entry.EndLine = entry.Line

		if entry.Kind == tomlKeyValue {
			end := expr.Raw.Offset + expr.Raw.Length
			rest := data[keyEnd:end]
			value := rest[bytes.IndexByte(rest, '=')+1:]
			entry.Value = strings.TrimRight(strings.TrimLeft(string(value), " \t"), " \t\r")
			entry.EndLine = parser.Shape(expr.Raw).End.Line
		}

		if trailing := expr.Next(); trailing != nil && trailing.Kind == unstable.Comment {
			entry.Comment = strings.TrimRight(string(trailing.Data), " \t\r")
		}
		entries = append(entries, entry)
	}

	if err := parser.Error(); err != nil {
		line := 0
		var perr *unstable.ParserError
		if errors.As(err, &perr) && len(perr.Highlight) > 0 {
			line = parser.Shape(parser.Range(perr.Highlight)).Start.Line
		}
		return nil, &tomlScanError{Line: line, Err: err}
	}
	return entries, nil
}

// joinKeyPath renders decoded key parts as a dotted path for messages.
func joinKeyPath(parts []string) string {
	return strings.Join(parts, ".")
}
