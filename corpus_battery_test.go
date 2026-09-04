package tomledit

import (
	"bytes"
	"errors"
	"math"
	"sort"
	"strconv"
	"testing"
	"time"

	tomltest "github.com/toml-lang/toml-test/v2"
)

// The corpus battery: the fidelity and equality properties of the design record,
// each asserted over every valid case of the toml-test corpus rather than over
// hand-written fixtures. The untouched round trip itself is asserted by
// TestTomlTestValid; what follows is what happens once a document is EDITED.

// battery runs check over every valid corpus case, and fails if the number of
// cases run changes -- a corpus that silently shrinks would pass every property
// below vacuously.
func battery(t *testing.T, check func(t *testing.T, data []byte)) {
	t.Helper()
	fsys := tomltest.TestCases()
	skip := tomlTestSkips(t, fsys)
	ran := runCorpus(t, fsys, "valid", skip, check)
	if ran != wantValidCases {
		t.Errorf("ran %d valid cases, want %d: the corpus or the 1.0 skip derivation changed",
			ran, wantValidCases)
	}
}

// --- walking a document's values ---

// scalarTarget is one value of a document, with the path that addresses it.
type scalarTarget struct {
	path string
	node Node
}

// scalarTargets returns every scalar of a document, addressed the way an editing
// caller would: through the read-layer for tables in any spelling, and through
// indices for array elements.
func scalarTargets(doc *Document) []scalarTarget {
	var out []scalarTarget

	appendSeg := func(prefix []PathSegment, seg PathSegment) []PathSegment {
		next := make([]PathSegment, 0, len(prefix)+1)
		next = append(next, prefix...)
		return append(next, seg)
	}

	var addValue func(prefix []PathSegment, n Node)
	addValue = func(prefix []PathSegment, n Node) {
		switch v := n.(type) {
		case *ArrayNode:
			for i, elem := range v.elements {
				addValue(appendSeg(prefix, PathSegment{Kind: SegmentIndex, Index: i}), elem)
			}
		case *InlineTableNode:
			// Only reachable as an array element; an inline table bound to a key
			// folds into a record and is walked below.
			for _, child := range v.children {
				kv, ok := child.(*KeyValueNode)
				if !ok {
					continue
				}
				p := prefix
				for _, part := range kv.key.parts {
					p = appendSeg(p, PathSegment{Kind: SegmentKey, Key: part})
				}
				addValue(p, kv.val)
			}
		default:
			out = append(out, scalarTarget{path: JoinPath(prefix), node: n})
		}
	}

	var walkRecord func(rec *Record, prefix []PathSegment)
	walkRecord = func(rec *Record, prefix []PathSegment) {
		for e := range rec.Entries() {
			p := appendSeg(prefix, PathSegment{Kind: SegmentKey, Key: e.Key()})
			switch e.Kind() {
			case EntryValue:
				if n, ok := e.Node(); ok {
					addValue(p, n)
				}
			case EntryRecord:
				if r, ok := e.Record(); ok {
					walkRecord(r, p)
				}
			case EntryRecords:
				rs, ok := e.Records()
				if !ok {
					continue
				}
				for i, r := range rs {
					walkRecord(r, appendSeg(p, PathSegment{Kind: SegmentIndex, Index: i}))
				}
			}
		}
	}

	walkRecord(doc.Root(), nil)
	return out
}

// perturbed returns a value of the node's own kind that is not the value it
// holds, and whether the kind has one. It is what the battery writes when it
// needs a write that really changes something.
func perturbed(n Node) (any, bool) {
	sc, ok := n.(Scalar)
	if !ok {
		return nil, false
	}
	switch v := sc.Value().(type) {
	case string:
		return v + "-battery", true
	case int64:
		if v == math.MaxInt64 {
			return v - 1, true
		}
		return v + 1, true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v+1 == v {
			return 1.5, true
		}
		return v + 1, true
	case bool:
		return !v, true
	case time.Time:
		// Shifting away from the end of the representable range rather than
		// past it: the corpus carries both 0001-01-01 and 9999-12-31.
		if v.Year() >= 9999 {
			return v.Add(-time.Hour), true
		}
		return v.Add(time.Hour), true
	case LocalDateTime:
		v.Minute = (v.Minute + 1) % 60
		return v, true
	case LocalDate:
		// A day, not a year: every month has a second day, and shifting the
		// year would land on 29 February of a common year.
		if v.Day > 1 {
			v.Day--
		} else {
			v.Day++
		}
		return v, true
	case LocalTime:
		v.Minute = (v.Minute + 1) % 60
		return v, true
	}
	return nil, false
}

// sameValue compares two decoded values, using time.Time's own equality so a
// value that survived a render and a re-parse is recognised across the location
// the offset was rebuilt with.
func sameValue(a, b any) bool {
	if at, ok := a.(time.Time); ok {
		bt, ok := b.(time.Time)
		return ok && at.Equal(bt)
	}
	return a == b
}

// --- the passes ---

// Fails if a value write stops being confined to the value's own byte range:
// every byte before and after the value the path names must survive a write to
// it, whatever construct the value sits in.
func TestBatteryValueMutationIsolatesSiblings(t *testing.T) {
	battery(t, func(t *testing.T, data []byte) {
		doc, err := Parse(data)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		for _, target := range scalarTargets(doc) {
			span := target.node.Span()
			if !span.IsValid() {
				t.Errorf("%s: the parsed value carries no span", target.path)
				continue
			}
			value, ok := perturbed(target.node)
			if !ok {
				continue
			}
			edited, err := Parse(data)
			if err != nil {
				t.Fatalf("re-parse failed: %v", err)
			}
			found, ok := edited.Lookup(target.path)
			if !ok || found.Span() != span {
				t.Errorf("%s: the path does not address the value at %d-%d",
					target.path, span.Start.Offset, span.End.Offset)
				continue
			}
			if err := edited.Set(target.path, value); err != nil {
				t.Errorf("%s: Set: %v", target.path, err)
				continue
			}
			out := edited.Bytes()
			prefix, suffix := data[:span.Start.Offset], data[span.End.Offset:]
			if len(out) < len(prefix)+len(suffix) ||
				!bytes.HasPrefix(out, prefix) || !bytes.HasSuffix(out, suffix) {
				t.Errorf("%s: writing the value changed bytes outside it\n got: %q\nwant: %q ... %q",
					target.path, out, prefix, suffix)
			}
		}
	})
}

// Fails if writing a comment starts changing any value's bytes: a trivia write
// invalidates the trivia fragment and nothing else.
func TestBatteryTriviaMutationLeavesValues(t *testing.T) {
	battery(t, func(t *testing.T, data []byte) {
		doc, err := Parse(data)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		targets := scalarTargets(doc)
		before := make([][]byte, len(targets))
		for i, target := range targets {
			before[i] = append([]byte(nil), serializeNode(target.node)...)
		}

		wrote := ""
		for _, target := range targets {
			// An inline table has nowhere to put a comment, and refuses; every
			// other line hosts one.
			if err := doc.SetComment(target.path, "battery"); err == nil {
				wrote = target.path
				break
			}
		}
		if wrote == "" {
			return
		}
		for i, target := range targets {
			if got := serializeNode(target.node); !bytes.Equal(got, before[i]) {
				t.Errorf("writing a comment at %s rewrote the value at %s: %q, was %q",
					wrote, target.path, got, before[i])
			}
		}
		if _, err := Parse(doc.Bytes()); err != nil {
			t.Errorf("the document with a comment written at %s no longer parses: %v", wrote, err)
		}
	})
}

// Fails if a rename stops being confined to the key parts that spell the
// renamed name: every other byte of every construct it touches must survive.
func TestBatteryRenameIsolatesTheKeyPart(t *testing.T) {
	const newKey = "batteryrenamed"
	battery(t, func(t *testing.T, data []byte) {
		doc, err := Parse(data)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		for _, keyPath := range renameTargets(doc) {
			edited, err := Parse(data)
			if err != nil {
				t.Fatalf("re-parse failed: %v", err)
			}
			spans := keyPartSpansFor(edited, keyPath)
			if len(spans) == 0 {
				t.Errorf("%v: no construct spells the name out", keyPath)
				continue
			}
			if err := edited.RenameKey(pathFromKeys(keyPath), newKey); err != nil {
				var diag *Error
				if errors.As(err, &diag) && diag.Kind == KindConflict {
					continue // the fresh name is taken in this document
				}
				t.Errorf("%v: RenameKey: %v", keyPath, err)
				continue
			}
			want := spliceOver(data, spans, []byte(newKey))
			if got := string(edited.Bytes()); got != string(want) {
				t.Errorf("renaming %v rendered %q, want %q", keyPath, got, want)
			}
		}
	})
}

// Fails if rendering a document from its semantic content alone stops producing
// the same document: with every splice of original bytes disabled, the
// renderers themselves must write TOML that re-parses to the value tree
// toml-test says the case denotes, and re-rendering it must be byte-stable.
func TestBatteryRendersFromValuesAlone(t *testing.T) {
	fsys := tomltest.TestCases()
	skip := tomlTestSkips(t, fsys)
	ran := runCorpusWithReference(t, fsys, skip, func(t *testing.T, data []byte, want map[string]any) {
		doc, err := Parse(data)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		out := renderFromValuesAlone(doc)
		reparsed, err := Parse(out)
		if err != nil {
			t.Fatalf("the re-rendered document does not parse: %v\n%s", err, out)
		}
		renderedCheck.record(t, "", reparsed.Root(), want)
		if again := renderFromValuesAlone(reparsed); !bytes.Equal(out, again) {
			t.Errorf("re-rendering is not idempotent:\nfirst:  %q\nsecond: %q", out, again)
		}
	})
	if ran != wantValidCases {
		t.Errorf("ran %d valid cases, want %d", ran, wantValidCases)
	}
}

// renderedCheck is the reference check this pass uses, and the reason it does
// not reuse the fold's: the fold's comparator asserts the payloads of strings,
// integers and booleans only, which is right for a question about folding and
// wrong for a question about RENDERING. Under it, a renderer that wrote the
// same wrong number for every float in the corpus would satisfy this whole
// pass -- the output would still parse, still fold to a float of the right
// kind, and still re-render identically. So this pass asserts every payload.
var renderedCheck = referenceCheck{scalar: compareRenderedScalar}

// compareRenderedScalar checks a scalar's kind against the reference tag and
// its payload against the reference spelling, for every kind.
//
// A float is compared bit for bit, so a signed zero is not the zero and a
// rounded value is not the value. NaN is the one exception, compared by kind:
// TOML has one NaN spelling, the reference says "nan" whatever sign the source
// carried, and the renderer writes "nan" for the same reason.
//
// A date-time is compared by value: the instant an offset date-time denotes,
// and the fields of each local flavor read as UTC.
func compareRenderedScalar(t *testing.T, path string, node Node, typ, value string) {
	t.Helper()
	compareScalar(t, path, node, typ, value)
	if wantKind, known := referenceKinds[typ]; !known || node.Type() != wantKind {
		return // compareScalar has already reported the kind
	}
	switch n := node.(type) {
	case *FloatNode:
		compareFloatPayload(t, path, n.val.get(), value)
	case *DateTimeNode:
		compareTimePayload(t, path, n.val.get(), value, time.RFC3339Nano)
	case *LocalDateTimeNode:
		compareTimePayload(t, path, goTime(n), value, "2006-01-02T15:04:05.999999999")
	case *LocalDateNode:
		compareTimePayload(t, path, goTime(n), value, "2006-01-02")
	case *LocalTimeNode:
		v := n.val.get()
		got := time.Date(0, time.January, 1, v.Hour, v.Minute, v.Second, v.Nanosecond, time.UTC)
		compareTimePayload(t, path, got, value, "15:04:05.999999999")
	}
}

// compareFloatPayload checks a float against its reference spelling, bit for
// bit, with NaN compared by kind.
func compareFloatPayload(t *testing.T, path string, got float64, value string) {
	t.Helper()
	want, err := strconv.ParseFloat(value, 64)
	if err != nil {
		t.Fatalf("%s: undecodable reference float %q: %v", path, value, err)
	}
	if math.IsNaN(want) {
		if !math.IsNaN(got) {
			t.Errorf("%s: float is %s, the reference says %s", path, FormatFloat(got), value)
		}
		return
	}
	if math.Float64bits(got) != math.Float64bits(want) {
		t.Errorf("%s: float is %s, the reference says %s", path, FormatFloat(got), value)
	}
}

// compareTimePayload checks a date-time against its reference spelling, read
// with the layout that spelling uses.
func compareTimePayload(t *testing.T, path string, got time.Time, value, layout string) {
	t.Helper()
	want, err := time.Parse(layout, value)
	if err != nil {
		t.Fatalf("%s: undecodable reference date-time %q: %v", path, value, err)
	}
	if !got.Equal(want) {
		t.Errorf("%s: date-time is %s, the reference says %s", path, got.Format(layout), value)
	}
}

// renderFromValuesAlone renders with every splice of original bytes disabled, so
// that only the renderers decide what the document says.
func renderFromValuesAlone(doc *Document) []byte {
	spliceOriginalBytes = false
	defer func() { spliceOriginalBytes = true }()
	return doc.Bytes()
}

// Fails if a written value stops dropping the lexeme it was read with, or if
// what the document renders stops reading back as the value that was written.
func TestBatteryWrittenValuesDropTheirLexeme(t *testing.T) {
	battery(t, func(t *testing.T, data []byte) {
		doc, err := Parse(data)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		for _, target := range scalarTargets(doc) {
			value, ok := perturbed(target.node)
			if !ok {
				continue
			}
			edited, err := Parse(data)
			if err != nil {
				t.Fatalf("re-parse failed: %v", err)
			}
			if err := edited.Set(target.path, value); err != nil {
				t.Errorf("%s: Set: %v", target.path, err)
				continue
			}
			written, ok := edited.Lookup(target.path)
			if !ok {
				t.Errorf("%s: the written value no longer resolves", target.path)
				continue
			}
			if raw := written.Raw(); raw != nil {
				t.Errorf("%s: the written value still carries the lexeme %q", target.path, raw)
			}
			out := edited.Bytes()
			if bytes.Equal(out, data) {
				t.Errorf("%s: writing a different value changed nothing", target.path)
				continue
			}
			reparsed, err := Parse(out)
			if err != nil {
				t.Errorf("%s: the edited document does not parse: %v", target.path, err)
				continue
			}
			back, ok := reparsed.Lookup(target.path)
			if !ok {
				t.Errorf("%s: the written value does not read back", target.path)
				continue
			}
			sc, ok := back.(Scalar)
			if !ok {
				t.Errorf("%s: the written value read back as %s", target.path, back.Type())
				continue
			}
			if !sameValue(sc.Value(), value) {
				t.Errorf("%s: wrote %v, read back %v", target.path, value, sc.Value())
			}
		}
	})
}

// Fails if writing the same value twice stops settling: the second write must
// be a byte-for-byte no-op that replaces no node.
func TestBatterySetThenSetIsStable(t *testing.T) {
	battery(t, func(t *testing.T, data []byte) {
		doc, err := Parse(data)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		for _, target := range scalarTargets(doc) {
			value, ok := perturbed(target.node)
			if !ok {
				continue
			}
			edited, err := Parse(data)
			if err != nil {
				t.Fatalf("re-parse failed: %v", err)
			}
			if err := edited.Set(target.path, value); err != nil {
				t.Errorf("%s: Set: %v", target.path, err)
				continue
			}
			first := string(edited.Bytes())
			before, _ := edited.Lookup(target.path)
			if err := edited.Set(target.path, value); err != nil {
				t.Errorf("%s: second Set: %v", target.path, err)
				continue
			}
			if after, _ := edited.Lookup(target.path); after != before {
				t.Errorf("%s: the second identical Set replaced the value node", target.path)
			}
			if second := string(edited.Bytes()); second != first {
				t.Errorf("%s: the second identical Set rendered %q, want %q", target.path, second, first)
			}
		}
	})
}

// Fails if writing a value its OWN value stops behaving as the equality rule
// says: a canonical spelling is left exactly as it stands, and any other
// spelling is normalised once and then left alone.
func TestBatterySameValueSetNormalisesOnce(t *testing.T) {
	battery(t, func(t *testing.T, data []byte) {
		doc, err := Parse(data)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		for _, target := range scalarTargets(doc) {
			sc, ok := target.node.(Scalar)
			if !ok {
				continue
			}
			// The canonical form, stated through the public write path rather
			// than read off the node: what Set writes for this value into a
			// slot that holds something else.
			canonical := []byte(setValueBytes(t, sc.Value()))
			alreadyCanonical := bytes.Equal(target.node.Raw(), canonical)

			edited, err := Parse(data)
			if err != nil {
				t.Fatalf("re-parse failed: %v", err)
			}
			before, _ := edited.Lookup(target.path)
			if err := edited.Set(target.path, sc.Value()); err != nil {
				t.Errorf("%s: Set: %v", target.path, err)
				continue
			}
			after, _ := edited.Lookup(target.path)
			first := string(edited.Bytes())

			if alreadyCanonical {
				if after != before {
					t.Errorf("%s: writing the value it already spells canonically replaced the node",
						target.path)
				}
				if first != string(data) {
					t.Errorf("%s: writing the value it already spells canonically changed the document",
						target.path)
				}
			} else if got := serializeNode(after); !bytes.Equal(got, canonical) {
				t.Errorf("%s: writing the same value rendered %q, want the canonical %q",
					target.path, got, canonical)
			}

			// Stable from here, whichever branch it took.
			settled, _ := edited.Lookup(target.path)
			if err := edited.Set(target.path, sc.Value()); err != nil {
				t.Errorf("%s: second Set: %v", target.path, err)
				continue
			}
			if again, _ := edited.Lookup(target.path); again != settled {
				t.Errorf("%s: the settled value was written again", target.path)
			}
			if second := string(edited.Bytes()); second != first {
				t.Errorf("%s: the settled document re-rendered as %q, want %q", target.path, second, first)
			}
		}
	})
}

// --- rename helpers ---

// renameTargets returns the full key path of every name a rename can address
// with a path of keys alone: the names bound in the document's own tables, in
// the tables their headers and dotted keys spell out, and nowhere else. Inline
// tables and array-of-tables entries are left out because reaching into them
// takes an index, and a name inside one entry cannot be told from the same name
// in another by key path alone.
func renameTargets(doc *Document) [][]string {
	var out [][]string
	var walk func(rec *Record, prefix []string)
	walk = func(rec *Record, prefix []string) {
		for e := range rec.Entries() {
			path := append(append([]string(nil), prefix...), e.Key())
			out = append(out, path)
			if e.Kind() != EntryRecord {
				continue
			}
			r, ok := e.Record()
			if !ok {
				continue
			}
			if n, backed := r.Node(); backed {
				if _, inline := n.(*InlineTableNode); inline {
					continue
				}
			}
			walk(r, path)
		}
	}
	walk(doc.Root(), nil)
	return out
}

// keyPartSpansFor returns the source range of every key part that spells the
// binding at keyPath, in source order: the part of each header whose key path
// passes through the name, and the part of each pair written under it. It is
// derived from the parse rather than from the rename, so it is an independent
// statement of which bytes a rename may touch.
func keyPartSpansFor(doc *Document, keyPath []string) []Span {
	var spans []Span
	at := len(keyPath) - 1

	pairsIn := func(children []Node, scope []string) {
		rel := at - len(scope)
		if rel < 0 {
			return
		}
		for _, child := range children {
			kv, ok := child.(*KeyValueNode)
			if !ok || len(kv.key.parts) <= rel {
				continue
			}
			full := append(append([]string(nil), scope...), kv.key.parts...)
			if len(full) <= at || !pathsEqual(full[:at+1], keyPath) {
				continue
			}
			spans = append(spans, kv.key.partSpan(rel))
		}
	}

	pairsIn(doc.children, nil)
	for _, child := range doc.children {
		var (
			path     []string
			keySpans []Span
			children []Node
		)
		switch h := child.(type) {
		case *TableNode:
			path, keySpans, children = h.keyPath, h.keySpans, h.children
		case *ArrayTableNode:
			path, keySpans, children = h.keyPath, h.keySpans, h.children
		default:
			continue
		}
		if len(path) > at && pathsEqual(path[:at+1], keyPath) {
			spans = append(spans, keySpans[at])
		}
		pairsIn(children, path)
	}

	sort.Slice(spans, func(i, j int) bool { return spans[i].Start.Offset < spans[j].Start.Offset })
	return spans
}

// spliceOver returns src with each span replaced by replacement. The spans are
// disjoint and in source order.
func spliceOver(src []byte, spans []Span, replacement []byte) []byte {
	var out []byte
	last := 0
	for _, span := range spans {
		out = append(out, src[last:span.Start.Offset]...)
		out = append(out, replacement...)
		last = span.End.Offset
	}
	return append(out, src[last:]...)
}
