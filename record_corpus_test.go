package tomledit

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strconv"
	"testing"

	tomltest "github.com/toml-lang/toml-test/v2"
)

// The reference fold: toml-test ships, beside every valid case, the logical
// value tree that case denotes -- keys, nesting, array-of-tables entries and
// tagged scalars. That tree is exactly what the read-layer is supposed to
// produce, so the corpus doubles as the fold's expectation set: every key the
// reference names must exist under the same nesting, with the same kind, and no
// key the reference does not name may appear.
//
// Values are compared where the comparison is about the fold rather than about
// rendering: strings, integers and booleans. Floats and date-times are compared
// by kind only -- their spellings belong to the renderer and the decoder.
//
// The walk over a reference tree is shared, and the only thing a pass chooses
// is which scalar payloads it asserts: referenceCheck carries that choice, so
// the renderer pass can demand every payload without loosening or tightening
// what the fold pass says.

// referenceCheck walks a folded document against a toml-test reference tree,
// asserting scalars through the comparator it carries.
type referenceCheck struct {
	scalar func(t *testing.T, path string, node Node, typ, value string)
}

// foldCheck is the fold's own pass: kinds always, and the payloads whose
// reference spelling is unambiguous. See the carve-out above.
var foldCheck = referenceCheck{scalar: compareScalar}

// Fails if the fold stops agreeing with toml-test's own reference tree for any
// valid case: a missing or invented key, a table folded as a value (or the
// reverse), an array-of-tables whose entries did not collect, or a scalar of
// the wrong kind. Also fails if the number of compared cases changes.
func TestFoldMatchesReferenceCorpus(t *testing.T) {
	fsys := tomltest.TestCases()
	skip := tomlTestSkips(t, fsys)

	ran := runCorpusWithReference(t, fsys, skip, func(t *testing.T, data []byte, want map[string]any) {
		doc, err := Parse(data)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		foldCheck.record(t, "", doc.Root(), want)
	})

	if ran != wantValidCases {
		t.Errorf("compared %d valid cases against their reference trees, want %d: the "+
			"toml-test corpus or the 1.0 skip derivation changed -- re-verify the corpus",
			ran, wantValidCases)
	}
}

// runCorpusWithReference runs check for every non-skipped valid case, passing
// the case's source and its decoded reference tree.
func runCorpusWithReference(t *testing.T, fsys fs.FS, skip map[string]bool, check func(t *testing.T, data []byte, want map[string]any)) int {
	t.Helper()
	ran := 0
	for _, name := range corpusNames(t, fsys, "valid") {
		if skip[name] {
			continue
		}
		data, err := fs.ReadFile(fsys, name+".toml")
		if err != nil {
			t.Errorf("[%s] read error: %v", name, err)
			continue
		}
		refBytes, err := fs.ReadFile(fsys, name+".json")
		if err != nil {
			t.Errorf("[%s] no reference tree: %v", name, err)
			continue
		}
		var want map[string]any
		if err := json.Unmarshal(refBytes, &want); err != nil {
			t.Errorf("[%s] undecodable reference tree: %v", name, err)
			continue
		}
		ran++
		t.Run(name, func(t *testing.T) {
			check(t, data, want)
		})
	}
	return ran
}

// taggedValue reports whether v is one of toml-test's tagged scalars
// ({"type": ..., "value": ...}) and returns the tag and the rendered value.
func taggedValue(v any) (typ, value string, ok bool) {
	m, isMap := v.(map[string]any)
	if !isMap || len(m) != 2 {
		return "", "", false
	}
	typ, okType := m["type"].(string)
	value, okValue := m["value"].(string)
	if !okType || !okValue {
		return "", "", false
	}
	return typ, value, true
}

// referenceKinds maps toml-test's scalar tags to this package's node types.
var referenceKinds = map[string]NodeType{
	"string":         NodeString,
	"integer":        NodeInteger,
	"float":          NodeFloat,
	"bool":           NodeBoolean,
	"datetime":       NodeDateTime,
	"datetime-local": NodeLocalDateTime,
	"date-local":     NodeLocalDate,
	"time-local":     NodeLocalTime,
}

// record checks a folded record against a reference table.
func (c referenceCheck) record(t *testing.T, path string, rec *Record, want map[string]any) {
	t.Helper()
	if rec.Len() != len(want) {
		t.Errorf("%s: the record holds %d keys, the reference names %d (%v vs %v)",
			pathOrRoot(path), rec.Len(), len(want), recordKeys(rec), referenceKeys(want))
	}
	for key, wantVal := range want {
		e, ok := rec.Get(key)
		if !ok {
			t.Errorf("%s: no entry %q (holds %v)", pathOrRoot(path), key, recordKeys(rec))
			continue
		}
		c.entry(t, joinTestPath(path, key), e, wantVal)
	}
	for e := range rec.Entries() {
		if _, ok := want[e.Key()]; !ok {
			t.Errorf("%s: entry %q is not in the reference tree", pathOrRoot(path), e.Key())
		}
	}
}

// entry checks one entry against its reference value.
func (c referenceCheck) entry(t *testing.T, path string, e Entry, want any) {
	t.Helper()
	switch {
	case isReferenceTable(want):
		rec, ok := e.Record()
		if !ok {
			t.Errorf("%s: the reference names a table, the fold holds a %s", path, e.Kind())
			return
		}
		c.record(t, path, rec, want.(map[string]any))

	case isReferenceArray(want):
		c.array(t, path, e, want.([]any))

	default:
		typ, value, ok := taggedValue(want)
		if !ok {
			t.Fatalf("%s: undecodable reference value %#v", path, want)
		}
		node, has := e.Node()
		if !has {
			t.Errorf("%s: the reference names a %s value, the fold holds a %s with no node", path, typ, e.Kind())
			return
		}
		c.scalar(t, path, node, typ, value)
	}
}

// array checks an entry the reference tree spells as a JSON array: an
// array-of-tables collection, or a plain array node.
func (c referenceCheck) array(t *testing.T, path string, e Entry, want []any) {
	t.Helper()
	if records, ok := e.Records(); ok {
		if len(records) != len(want) {
			t.Errorf("%s: the collection has %d entries, the reference names %d", path, len(records), len(want))
			return
		}
		for i, rec := range records {
			table, isTable := want[i].(map[string]any)
			if !isTable {
				t.Errorf("%s[%d]: the reference names a value, the fold holds an array-of-tables entry", path, i)
				continue
			}
			c.record(t, fmt.Sprintf("%s[%d]", path, i), rec, table)
		}
		return
	}
	node, has := e.Node()
	if !has {
		t.Errorf("%s: the reference names an array, the fold holds a %s with no node", path, e.Kind())
		return
	}
	c.arrayNode(t, path, node, want)
}

// arrayNode checks a plain array node against a reference array.
func (c referenceCheck) arrayNode(t *testing.T, path string, node Node, want []any) {
	t.Helper()
	arr, ok := node.(*ArrayNode)
	if !ok {
		t.Errorf("%s: the reference names an array, the fold holds a %s", path, node.Type())
		return
	}
	if len(arr.elements) != len(want) {
		t.Errorf("%s: the array holds %d elements, the reference names %d", path, len(arr.elements), len(want))
		return
	}
	for i, elem := range arr.elements {
		elemPath := fmt.Sprintf("%s[%d]", path, i)
		switch {
		case isReferenceTable(want[i]):
			inline, isInline := elem.(*InlineTableNode)
			if !isInline {
				t.Errorf("%s: the reference names a table, the fold holds a %s", elemPath, elem.Type())
				continue
			}
			rec, err := foldInlineTable(inline)
			if err != nil {
				t.Errorf("%s: folding the inline table: %v", elemPath, err)
				continue
			}
			c.record(t, elemPath, rec, want[i].(map[string]any))
		case isReferenceArray(want[i]):
			c.arrayNode(t, elemPath, elem, want[i].([]any))
		default:
			typ, value, ok := taggedValue(want[i])
			if !ok {
				t.Fatalf("%s: undecodable reference value %#v", elemPath, want[i])
			}
			c.scalar(t, elemPath, elem, typ, value)
		}
	}
}

// compareScalar checks a scalar node's kind against the reference tag, and its
// value for the kinds whose reference spelling is unambiguous.
func compareScalar(t *testing.T, path string, node Node, typ, value string) {
	t.Helper()
	wantKind, known := referenceKinds[typ]
	if !known {
		t.Fatalf("%s: unknown reference tag %q", path, typ)
	}
	if node.Type() != wantKind {
		t.Errorf("%s: the fold holds a %s, the reference names a %s", path, node.Type(), typ)
		return
	}
	switch n := node.(type) {
	case *StringNode:
		if n.val.get() != value {
			t.Errorf("%s: string is %q, the reference says %q", path, n.val.get(), value)
		}
	case *IntegerNode:
		if got := strconv.FormatInt(n.val.get(), 10); got != value {
			t.Errorf("%s: integer is %s, the reference says %s", path, got, value)
		}
	case *BooleanNode:
		if got := strconv.FormatBool(n.val.get()); got != value {
			t.Errorf("%s: boolean is %s, the reference says %s", path, got, value)
		}
	}
}

// isReferenceTable reports whether a reference value is a table rather than a
// tagged scalar.
func isReferenceTable(v any) bool {
	if _, ok := v.(map[string]any); !ok {
		return false
	}
	_, _, tagged := taggedValue(v)
	return !tagged
}

// isReferenceArray reports whether a reference value is an array.
func isReferenceArray(v any) bool {
	_, ok := v.([]any)
	return ok
}

// referenceKeys returns a reference table's keys, for failure messages.
func referenceKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// joinTestPath appends a key to a reference path, for failure messages.
func joinTestPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// pathOrRoot names the document root in failure messages.
func pathOrRoot(path string) string {
	if path == "" {
		return "(root)"
	}
	return path
}
