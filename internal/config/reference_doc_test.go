package config

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/tonk/tuios/internal/hooks"
)

// The configuration reference (docs/config-reference.adoc) is generated from
// the code: the option list, types and defaults come from UserConfig and
// exampleTables, the keybinding tables from DefaultConfig, and the hook
// events from hooks.AllEvents. Only the long explanations are hand-written,
// in refdocs/<table path>.adoc. Each of those files starts with the table's
// introduction, followed by one "@@ <key>" block per option.
//
// TestConfigReferenceDocUpToDate fails when the checked-in document no longer
// matches what the code generates. Regenerate it with:
//
//	go test ./internal/config -run TestConfigReferenceDoc -update
//
// or `make docs-config-reference`.

//go:embed refdocs/*.adoc
var refDocsFS embed.FS

var updateReferenceDoc = flag.Bool("update", false, "rewrite docs/config-reference.adoc")

const referenceDocPath = "../../docs/config-reference.adoc"

// refDocSection is one refdocs/<path>.adoc file: the introduction of a TOML
// table and the long explanation of each of its options.
type refDocSection struct {
	Intro   string
	Options map[string]string
}

var optionMarker = regexp.MustCompile(`^@@ ([a-z0-9_]+)\s*$`)

func loadRefDocs(t *testing.T) map[string]refDocSection {
	t.Helper()
	entries, err := refDocsFS.ReadDir("refdocs")
	if err != nil {
		t.Fatalf("reading refdocs: %v", err)
	}
	docs := map[string]refDocSection{}
	for _, entry := range entries {
		raw, err := refDocsFS.ReadFile("refdocs/" + entry.Name())
		if err != nil {
			t.Fatalf("reading refdocs/%s: %v", entry.Name(), err)
		}
		sec := refDocSection{Options: map[string]string{}}
		var current string
		var buf []string
		flush := func() {
			text := strings.TrimSpace(strings.Join(buf, "\n"))
			if current == "" {
				sec.Intro = text
			} else {
				sec.Options[current] = text
			}
			buf = nil
		}
		for line := range strings.SplitSeq(string(raw), "\n") {
			if m := optionMarker.FindStringSubmatch(line); m != nil {
				flush()
				current = m[1]
				if _, dup := sec.Options[current]; dup {
					t.Errorf("refdocs/%s: option %q documented twice", entry.Name(), current)
				}
				continue
			}
			buf = append(buf, line)
		}
		flush()
		docs[strings.TrimSuffix(entry.Name(), ".adoc")] = sec
	}
	return docs
}

// optionTypes maps every scalar TOML path in UserConfig to a readable type
// name, walking the struct the same way TestExampleConfigCoversAllFields does.
func optionTypes() map[string]string {
	types := map[string]string{}
	var walk func(rt reflect.Type, path string)
	walk = func(rt reflect.Type, path string) {
		for i := 0; i < rt.NumField(); i++ {
			field := rt.Field(i)
			parts := strings.Split(field.Tag.Get("toml"), ",")
			if parts[0] == "" || parts[0] == "-" {
				continue
			}
			key := parts[0]
			if path != "" {
				key = path + "." + parts[0]
			}
			ft := field.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			switch ft.Kind() {
			case reflect.Struct:
				walk(ft, key)
			case reflect.Bool:
				types[key] = "boolean"
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				types[key] = "integer"
			case reflect.Float32, reflect.Float64:
				types[key] = "number"
			case reflect.String:
				types[key] = "string"
			case reflect.Slice, reflect.Array:
				types[key] = "list of strings"
			}
		}
	}
	walk(reflect.TypeFor[UserConfig](), "")
	return types
}

// refID turns a TOML path into an AsciiDoc ID that cannot collide with
// section IDs elsewhere in the book.
func refID(path string) string {
	return "cfg-" + strings.NewReplacer(".", "-", "_", "-").Replace(path)
}

// mono renders text as literal monospace, whatever characters it contains.
// A "++" passthrough cannot hold a "+" at its edges or a "++" inside, so such
// text (a "+" key, say) goes through the pass macro instead.
func mono(text string) string {
	if strings.Contains(text, "++") || strings.HasPrefix(text, "+") || strings.HasSuffix(text, "+") {
		return "`pass:c[" + strings.ReplaceAll(text, "]", `\]`) + "]`"
	}
	return "`++" + text + "++`"
}

// generateConfigReference renders docs/config-reference.adoc without its
// revision lines, which scripts/sync-doc-revisions.sh owns.
func generateConfigReference(t *testing.T) string {
	t.Helper()
	docs := loadRefDocs(t)
	types := optionTypes()
	def := DefaultConfig()

	section := func(path string) refDocSection {
		sec, ok := docs[path]
		if !ok {
			t.Errorf("missing internal/config/refdocs/%s.adoc", path)
		} else if sec.Intro == "" {
			t.Errorf("refdocs/%s.adoc has no introduction", path)
		}
		return sec
	}

	type block struct {
		path   string
		render func(b *strings.Builder)
	}
	var blocks []block
	seen := map[string]bool{}

	for _, tbl := range exampleTables {
		sec := section(tbl.Path)
		seen[tbl.Path] = true
		fields := append([]exampleField(nil), tbl.Fields...)
		sort.Slice(fields, func(i, j int) bool { return fields[i].Key < fields[j].Key })
		for key := range sec.Options {
			if !hasField(fields, key) {
				t.Errorf("refdocs/%s.adoc documents %q, which is not an option of [%s]", tbl.Path, key, tbl.Path)
			}
		}
		blocks = append(blocks, block{tbl.Path, func(b *strings.Builder) {
			writeSectionHead(b, tbl.Path, sec.Intro)
			for _, f := range fields {
				path := tbl.Path + "." + f.Key
				text := sec.Options[f.Key]
				if text == "" {
					t.Errorf("refdocs/%s.adoc has no explanation for %q", tbl.Path, f.Key)
				}
				fmt.Fprintf(b, "[#%s]\n=== %s\n\n", refID(path), f.Key)
				fmt.Fprintf(b, "[horizontal]\nType:: %s\nDefault:: %s\n\n", types[path], mono(f.Example))
				b.WriteString(text + "\n\n")
			}
		}})
	}

	for _, kb := range keybindSectionDocs {
		path := "keybindings." + kb.Key
		blocks = append(blocks, block{path, func(b *strings.Builder) {
			intro := kb.Title
			if sec, ok := docs[path]; ok && sec.Intro != "" {
				intro = sec.Intro
			}
			writeSectionHead(b, path, intro)
			actions := kb.Get(def)
			if len(actions) == 0 {
				b.WriteString("This section has no default bindings.\n\n")
				return
			}
			names := make([]string, 0, len(actions))
			for name := range actions {
				names = append(names, name)
			}
			sort.Strings(names)
			b.WriteString("[cols=\"5,2,4\",options=\"header\"]\n|===\n|Action |Default keys |Description\n\n")
			for _, name := range names {
				keys := make([]string, len(actions[name]))
				for i, k := range actions[name] {
					keys[i] = mono(k)
				}
				fmt.Fprintf(b, "|%s\n|%s\n|%s\n\n", mono(name), cell(strings.Join(keys, ", ")), cell(describeAction(name)))
			}
			b.WriteString("|===\n\n")
		}})
	}

	for _, path := range []string{"env", "hooks"} {
		sec := section(path)
		seen[path] = true
		blocks = append(blocks, block{path, func(b *strings.Builder) {
			writeSectionHead(b, path, sec.Intro)
			if path != "hooks" {
				return
			}
			b.WriteString("[cols=\"2,5\",options=\"header\"]\n|===\n|Event |When it fires\n\n")
			events := hooks.AllEvents()
			sort.Slice(events, func(i, j int) bool { return events[i] < events[j] })
			for _, event := range events {
				fmt.Fprintf(b, "|%s\n|%s\n\n", mono(string(event)), hookEventDescriptions[event])
			}
			b.WriteString("|===\n\n")
		}})
	}

	for path := range docs {
		if !seen[path] && !strings.HasPrefix(path, "keybindings.") {
			t.Errorf("refdocs/%s.adoc does not match any configuration table", path)
		}
	}

	sort.Slice(blocks, func(i, j int) bool { return blocks[i].path < blocks[j].path })

	var b strings.Builder
	b.WriteString(referenceDocHeader)
	for _, blk := range blocks {
		blk.render(&b)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// cell escapes the table cell separator, which a key such as "alt+|"
// contains; the table parser turns "\|" back into "|".
func cell(text string) string {
	return strings.ReplaceAll(text, "|", `\|`)
}

func hasField(fields []exampleField, key string) bool {
	for _, f := range fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

func writeSectionHead(b *strings.Builder, path, intro string) {
	fmt.Fprintf(b, "[#%s]\n== [%s]\n\n%s\n\n", refID(path), path, intro)
}

const referenceDocHeader = `////
Generated from internal/config by
  go test ./internal/config -run TestConfigReferenceDoc -update
Do not edit by hand: change internal/config/refdocs/*.adoc (explanations)
or internal/config/example.go (options and defaults), then regenerate.
////
ifdef::book[]
[appendix]
endif::book[]
[#doc-config-reference]
= Configuration Reference
ifndef::book[]
Gaurav Gosain; Ton Kersten <tonk@smartowl.nl>
ifdef::backend-pdf[]
:title-page:
:pdf-theme: tuios
:pdf-themesdir: pdf-theme
endif::backend-pdf[]
:toc: left
:toc-title: Table of Contents
:toclevels: 2
:sectnums:
:sectanchors:
:idprefix:
:idseparator: -
:icons: font
:experimental:
:source-highlighter: rouge
endif::book[]

Every option of the TUIOS configuration file (` + "`config.toml`" + `), grouped by TOML table, with the tables and the options in each table in alphabetical order.
For where the file lives and how to start one, see xref:configuration.adoc#doc-configuration[Configuration Guide].
` + "`tuios config example`" + ` prints the same options as a commented TOML file.

Every option is optional: leave it out and TUIOS uses the default shown here.

`

var revisionLine = regexp.MustCompile(`(?m)^:rev(number|date): .*\n`)

func TestConfigReferenceDocUpToDate(t *testing.T) {
	want := generateConfigReference(t)
	if t.Failed() {
		return
	}

	current, err := os.ReadFile(referenceDocPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("reading %s: %v", referenceDocPath, err)
	}
	// The revision lines belong to scripts/sync-doc-revisions.sh: compare
	// without them, and carry the current ones over when rewriting.
	got := revisionLine.ReplaceAllString(string(current), "")

	if *updateReferenceDoc {
		revisions := strings.Join(revisionLine.FindAllString(string(current), -1), "")
		out := strings.Replace(want, "ifndef::book[]\nGaurav Gosain; Ton Kersten <tonk@smartowl.nl>\n",
			"ifndef::book[]\nGaurav Gosain; Ton Kersten <tonk@smartowl.nl>\n"+revisions, 1)
		if err := os.WriteFile(referenceDocPath, []byte(out), 0o644); err != nil {
			t.Fatalf("writing %s: %v", referenceDocPath, err)
		}
		return
	}

	if got != want {
		t.Errorf("%s is out of date; regenerate it with\n\tgo test ./internal/config -run TestConfigReferenceDoc -update", referenceDocPath)
	}
}
