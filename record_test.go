package tomledit

import (
	"errors"
	"strings"
	"testing"
)

// The fold suite: the executable form of the read-layer's rules. Each test
// names the rule it holds and what would make it fail.

// foldTestDoc parses src and returns its read-layer root.
func foldTestDoc(t *testing.T, src string) *Record {
	t.Helper()
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return doc.Root()
}

// unfoldableDoc builds a document TOML cannot express -- a [header] table and
// an array-of-tables claiming one key -- by appending the second header
// straight to the AST.
//
// No public editing sequence can build one: every operation that could bind a
// name another construct already holds refuses instead (refusals_test.go), so
// a test that needs an unfoldable document has to assemble it by hand.
func unfoldableDoc(t *testing.T) *Document {
	t.Helper()
	doc, err := Parse([]byte("[a]\nx = 1\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	atbl := &ArrayTableNode{KeyPath: []string{"a"}}
	atbl.markDirty()
	atbl.nodeTrivia.TrailingNewline = []byte("\n")
	doc.Children = append(doc.Children, atbl)
	return doc
}

// recordKeys returns a record's keys in iteration order.
func recordKeys(r *Record) []string {
	var keys []string
	for e := range r.Entries() {
		keys = append(keys, e.key)
	}
	return keys
}

// mustRecord returns the record at key, failing the test when the entry is
// missing or holds something else.
func mustRecord(t *testing.T, r *Record, key string) *Record {
	t.Helper()
	e, ok := r.Get(key)
	if !ok {
		t.Fatalf("no entry %q (keys: %v)", key, recordKeys(r))
	}
	sub, ok := e.Record()
	if !ok {
		t.Fatalf("entry %q is a %s, want a record", key, e.Kind())
	}
	return sub
}

// mustRecords returns the array-of-tables entries at key.
func mustRecords(t *testing.T, r *Record, key string) []*Record {
	t.Helper()
	e, ok := r.Get(key)
	if !ok {
		t.Fatalf("no entry %q (keys: %v)", key, recordKeys(r))
	}
	list, ok := e.Records()
	if !ok {
		t.Fatalf("entry %q is a %s, want records", key, e.Kind())
	}
	return list
}

// sourceOfSpan returns the bytes src covers over span.
func sourceOfSpan(src string, span Span) string {
	if !span.IsValid() {
		return ""
	}
	return src[span.Start.Offset:span.End.Offset]
}

// Rule 1. Fails if the fold stops ordering keys by first appearance -- if it
// groups by binding form, or sorts, or lets a later header move a key.
func TestFold_FirstAppearanceOrder(t *testing.T) {
	root := foldTestDoc(t, `zulu = 1
alpha.deep = 2
inline = {q = 3}

[mike]
x = 1

[[november]]
y = 2

[bravo]
z = 3
`)
	want := []string{"zulu", "alpha", "inline", "mike", "november", "bravo"}
	got := recordKeys(root)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("root keys = %v, want %v", got, want)
	}
}

// Rule 2. Fails if a dotted key stops creating the records it implies, or
// stops leaving the value at the last part.
func TestFold_DottedKeyExpansion(t *testing.T) {
	root := foldTestDoc(t, "a.b.c = 1\n")

	a := mustRecord(t, root, "a")
	if got := recordKeys(a); len(got) != 1 || got[0] != "b" {
		t.Fatalf("record a holds %v, want [b]", got)
	}
	b := mustRecord(t, a, "b")
	e, ok := b.Get("c")
	if !ok {
		t.Fatal("record a.b has no entry c")
	}
	if e.Kind() != EntryValue {
		t.Errorf("a.b.c is a %s, want a value", e.Kind())
	}
	node, ok := e.Node()
	if !ok {
		t.Fatal("a.b.c has no concrete node")
	}
	if n, isInt := node.(*IntegerNode); !isInt || n.Val != 1 {
		t.Errorf("a.b.c node = %T, want the integer 1", node)
	}
}

// Rule 2. Fails if two dotted keys sharing a prefix stop folding into one
// record -- each carrying its own would split the logical table in two.
func TestFold_DottedKeysShareAPrefix(t *testing.T) {
	root := foldTestDoc(t, "a.b = 1\na.c = 2\n")
	if got := recordKeys(root); len(got) != 1 || got[0] != "a" {
		t.Fatalf("root keys = %v, want [a]", got)
	}
	a := mustRecord(t, root, "a")
	if got := recordKeys(a); strings.Join(got, ",") != "b,c" {
		t.Errorf("record a holds %v, want [b c]", got)
	}
}

// Rule 3, the record's worked example. Fails if a later header stops reopening
// the record an earlier header implied, or if reopening reorders the entries,
// or if the record stays anchored at the construct that implied it rather than
// at its own header.
func TestFold_LaterHeaderReopensImpliedRecord(t *testing.T) {
	src := "[a.b]\nx = 1\n[a]\ny = 1\n"
	root := foldTestDoc(t, src)

	a := mustRecord(t, root, "a")
	if got := recordKeys(a); strings.Join(got, ",") != "b,y" {
		t.Errorf("record a holds %v, want [b y]", got)
	}
	if got := sourceOfSpan(src, a.Span()); got != "[a]" {
		t.Errorf("record a is anchored at %q, want %q (its own header)", got, "[a]")
	}
	entry, _ := root.Get("a")
	if got := sourceOfSpan(src, entry.KeySpan()); got != "a" {
		t.Errorf("entry a names its key at %q, want %q", got, "a")
	}
	b := mustRecord(t, a, "b")
	if got := sourceOfSpan(src, b.Span()); got != "[a.b]" {
		t.Errorf("record a.b is anchored at %q, want %q", got, "[a.b]")
	}
}

// Rule 3. Fails if an implied record stops being anchored at the construct
// that created it when no header of its own ever appears.
func TestFold_ImpliedRecordAnchorsAtItsCreator(t *testing.T) {
	src := "[a.b]\nx = 1\n"
	root := foldTestDoc(t, src)
	a := mustRecord(t, root, "a")
	if got := sourceOfSpan(src, a.Span()); got != "[a.b]" {
		t.Errorf("record a is anchored at %q, want the header that implied it", got)
	}
	if _, ok := a.Node(); ok {
		t.Error("an implied record reports a concrete node; no single node stands for it")
	}
}

// Rule 4, the record's worked example. Fails if a sub-table header under an
// array-of-tables prefix stops addressing the LAST entry.
func TestFold_SubTableAddressesLastEntry(t *testing.T) {
	root := foldTestDoc(t, "[[s]]\n[[s]]\n[s.t]\nx = 1\n")

	entries := mustRecords(t, root, "s")
	if len(entries) != 2 {
		t.Fatalf("s has %d entries, want 2", len(entries))
	}
	if got := recordKeys(entries[0]); len(got) != 0 {
		t.Errorf("the first s entry holds %v, want nothing", got)
	}
	if got := recordKeys(entries[1]); len(got) != 1 || got[0] != "t" {
		t.Fatalf("the second s entry holds %v, want [t]", got)
	}
	tRec := mustRecord(t, entries[1], "t")
	if _, ok := tRec.Get("x"); !ok {
		t.Error("s[1].t has no entry x")
	}
}

// Rule 4. Fails if array-of-tables entries stop collecting under one key --
// if a second header opened a second key, or replaced the first entry.
func TestFold_ArrayOfTablesCollects(t *testing.T) {
	root := foldTestDoc(t, `[[p]]
name = "a"

[[p]]
name = "b"

[[p]]
name = "c"
`)
	if got := recordKeys(root); len(got) != 1 || got[0] != "p" {
		t.Fatalf("root keys = %v, want [p]", got)
	}
	entries := mustRecords(t, root, "p")
	if len(entries) != 3 {
		t.Fatalf("p has %d entries, want 3", len(entries))
	}
	for i, want := range []string{"a", "b", "c"} {
		e, ok := entries[i].Get("name")
		if !ok {
			t.Fatalf("p[%d] has no name", i)
		}
		node, _ := e.Node()
		if s, isStr := node.(*StringNode); !isStr || s.Val != want {
			t.Errorf("p[%d].name = %v, want %q", i, node, want)
		}
	}
}

// Rule 5. Fails if an inline table stops folding into an ordinary record --
// if the layer starts distinguishing the spelling a table was written in.
func TestFold_InlineTableFoldsLikeAHeaderTable(t *testing.T) {
	inline := foldTestDoc(t, "t = {a = 1, b = 2}\n")
	header := foldTestDoc(t, "[t]\na = 1\nb = 2\n")

	for _, root := range []*Record{inline, header} {
		e, ok := root.Get("t")
		if !ok {
			t.Fatal("no entry t")
		}
		if e.Kind() != EntryRecord {
			t.Errorf("t is a %s, want a record", e.Kind())
		}
		rec, _ := e.Record()
		if got := recordKeys(rec); strings.Join(got, ",") != "a,b" {
			t.Errorf("t holds %v, want [a b]", got)
		}
	}
}

// Rule 5. Fails if a dotted key inside an inline table stops expanding the way
// it does at document level.
func TestFold_DottedKeyInsideInlineTable(t *testing.T) {
	root := foldTestDoc(t, "t = {a.b = 1}\n")
	a := mustRecord(t, mustRecord(t, root, "t"), "a")
	if _, ok := a.Get("b"); !ok {
		t.Errorf("t.a holds %v, want [b]", recordKeys(a))
	}
}

// Rule 6. Fails if an entry stops carrying the range of its own key part --
// if it carried the whole construct, or the first part of a dotted key.
func TestFold_EntriesCarryTheirKeySpan(t *testing.T) {
	src := "a.b = 1\n[server]\n\"quoted key\" = 2\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	root := doc.Root()

	aEntry, _ := root.Get("a")
	if got := sourceOfSpan(src, aEntry.KeySpan()); got != "a" {
		t.Errorf("entry a key span covers %q, want %q", got, "a")
	}
	a := mustRecord(t, root, "a")
	bEntry, _ := a.Get("b")
	if got := sourceOfSpan(src, bEntry.KeySpan()); got != "b" {
		t.Errorf("entry a.b key span covers %q, want %q", got, "b")
	}
	server := mustRecord(t, root, "server")
	quoted, ok := server.Get("quoted key")
	if !ok {
		t.Fatalf("server holds %v, want the quoted key", recordKeys(server))
	}
	if got := sourceOfSpan(src, quoted.KeySpan()); got != `"quoted key"` {
		t.Errorf("quoted entry key span covers %q, want the quoted key bytes", got)
	}
}

// Rule 6. Fails if a record stops being anchored at the construct it belongs
// to: an inline table at its braces, a header table at its header, an
// array-of-tables entry at its own header.
func TestFold_RecordsCarryTheirAnchor(t *testing.T) {
	src := "t = {a = 1}\n[h]\nb = 2\n[[p]]\nc = 3\n"
	root := foldTestDoc(t, src)

	if got := sourceOfSpan(src, mustRecord(t, root, "t").Span()); got != "{a = 1}" {
		t.Errorf("inline record anchor covers %q, want %q", got, "{a = 1}")
	}
	if got := sourceOfSpan(src, mustRecord(t, root, "h").Span()); got != "[h]" {
		t.Errorf("header record anchor covers %q, want %q", got, "[h]")
	}
	entries := mustRecords(t, root, "p")
	if got := sourceOfSpan(src, entries[0].Span()); got != "[[p]]" {
		t.Errorf("array-table entry anchor covers %q, want %q", got, "[[p]]")
	}
}

// Rule 6. Fails if the synthesized collection span stops running from the
// first entry's header to the last entry's content -- the only range that
// describes an array-of-tables, since no single node covers one.
func TestFold_RecordsSpan(t *testing.T) {
	src := "[[p]]\nname = \"a\"\n\n[[p]]\nname = \"b\"\n"
	root := foldTestDoc(t, src)
	e, _ := root.Get("p")
	if got := sourceOfSpan(src, e.RecordsSpan()); got != "[[p]]\nname = \"a\"\n\n[[p]]\nname = \"b\"" {
		t.Errorf("collection span covers %q", got)
	}

	// An entry with no content of its own ends the span at its header.
	src2 := "[[p]]\nx = 1\n[[p]]\n"
	root2 := foldTestDoc(t, src2)
	e2, _ := root2.Get("p")
	if got := sourceOfSpan(src2, e2.RecordsSpan()); got != "[[p]]\nx = 1\n[[p]]" {
		t.Errorf("collection span with an empty last entry covers %q", got)
	}

	// Every other kind carries no collection span.
	scalar, _ := foldTestDoc(t, "x = 1\n").Get("x")
	if scalar.RecordsSpan().IsValid() {
		t.Error("a scalar entry carries a collection span")
	}
}

// entryNodeDoc carries one entry of every kind and origin the layer can hold:
// two values, a table in each of its two written spellings, an
// array-of-tables, and a record implied by a longer header.
const entryNodeDoc = `scalar = 1
arr = [1, 2]
inline = {x = 1}

[header]
y = 1

[[coll]]
z = 1

[implied.leaf]
w = 1
`

// Fails if Kind stops classifying an entry by what it holds, or if Node stops
// answering for VALUE entries alone -- a scalar or a plain array. A table is a
// record whatever spelling wrote it, and a record's own construct is
// Record.Node's answer, not the entry's.
func TestFold_EntryKindAndNode(t *testing.T) {
	root := foldTestDoc(t, entryNodeDoc)
	cases := []struct {
		key      string
		kind     EntryKind
		hasNode  bool
		nodeType NodeType
	}{
		{"scalar", EntryValue, true, NodeInteger},
		{"arr", EntryValue, true, NodeArray},
		{"inline", EntryRecord, false, 0},
		{"header", EntryRecord, false, 0},
		{"coll", EntryRecords, false, 0},
		{"implied", EntryRecord, false, 0},
	}
	for _, c := range cases {
		e, ok := root.Get(c.key)
		if !ok {
			t.Errorf("no entry %q", c.key)
			continue
		}
		if e.Kind() != c.kind {
			t.Errorf("%s is a %s, want a %s", c.key, e.Kind(), c.kind)
		}
		node, has := e.Node()
		if has != c.hasNode {
			t.Errorf("%s Node() reported %v, want %v", c.key, has, c.hasNode)
			continue
		}
		if has && node.Type() != c.nodeType {
			t.Errorf("%s node is a %s, want a %s", c.key, node.Type(), c.nodeType)
		}
	}

	// Records() answers only for a collection, Record() only for a record.
	coll, _ := root.Get("coll")
	if _, ok := coll.Record(); ok {
		t.Error("a collection answered Record()")
	}
	header, _ := root.Get("header")
	if _, ok := header.Records(); ok {
		t.Error("a record answered Records()")
	}
}

// Fails if a record stops naming the construct that backs it. Every origin a
// record can have answers here: the two written table spellings, an
// array-of-tables entry, the document root, and a record implied by a longer
// header -- the one origin no single construct stands for.
func TestFold_RecordNode(t *testing.T) {
	root := foldTestDoc(t, entryNodeDoc)

	node, ok := root.Node()
	if !ok {
		t.Error("the root record names no node; the document is what backs it")
	} else if node.Type() != NodeDocument {
		t.Errorf("the root record is backed by a %s, want a %s", node.Type(), NodeDocument)
	}

	cases := []struct {
		key      string
		hasNode  bool
		nodeType NodeType
	}{
		{"inline", true, NodeInlineTable},
		{"header", true, NodeTable},
		{"implied", false, 0},
	}
	for _, c := range cases {
		e, found := root.Get(c.key)
		if !found {
			t.Errorf("no entry %q", c.key)
			continue
		}
		rec, isRecord := e.Record()
		if !isRecord {
			t.Errorf("%s is a %s, want a record", c.key, e.Kind())
			continue
		}
		node, has := rec.Node()
		if has != c.hasNode {
			t.Errorf("the %s record's Node() reported %v, want %v", c.key, has, c.hasNode)
			continue
		}
		if has && node.Type() != c.nodeType {
			t.Errorf("the %s record is backed by a %s, want a %s", c.key, node.Type(), c.nodeType)
		}
	}

	// Each entry of an array-of-tables is backed by its own header.
	coll, _ := root.Get("coll")
	entries, isCollection := coll.Records()
	if !isCollection {
		t.Fatalf("coll is a %s, want an array-of-tables", coll.Kind())
	}
	for i, rec := range entries {
		node, has := rec.Node()
		if !has {
			t.Errorf("coll[%d] names no node; its own [[header]] backs it", i)
			continue
		}
		if node.Type() != NodeArrayTable {
			t.Errorf("coll[%d] is backed by a %s, want a %s", i, node.Type(), NodeArrayTable)
		}
	}

	// A record implied by a dotted key is the other unbacked origin.
	dotted := foldTestDoc(t, "a.b = 1\n")
	e, _ := dotted.Get("a")
	rec, _ := e.Record()
	if _, has := rec.Node(); has {
		t.Error("a record implied by a dotted key names a node; no single construct stands for it")
	}
}

// Fails if Records stops copying: a caller reordering the returned slice would
// otherwise be reordering the layer itself.
func TestFold_RecordsReturnsACopy(t *testing.T) {
	root := foldTestDoc(t, "[[p]]\nx = 1\n[[p]]\nx = 2\n")
	e, _ := root.Get("p")
	list, _ := e.Records()
	list[0], list[1] = list[1], list[0]

	again, _ := e.Records()
	if _, ok := again[0].Get("x"); !ok {
		t.Fatal("the first entry lost its key")
	}
	first, _ := again[0].Get("x")
	node, _ := first.Node()
	if n, ok := node.(*IntegerNode); !ok || n.Val != 1 {
		t.Errorf("reordering the returned slice reordered the layer: first entry x = %v", node)
	}
}

// Fails if the layer is built once and cached: until the caching switch
// arrives with its invalidation, every access must rebuild, so an edit is
// visible to the next read.
func TestFold_RebuiltOnEveryAccess(t *testing.T) {
	doc, err := Parse([]byte("x = 1\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got := recordKeys(doc.Root()); len(got) != 1 {
		t.Fatalf("root keys = %v, want [x]", got)
	}
	if err := doc.SetCreate("y", 2); err != nil {
		t.Fatalf("SetCreate: %v", err)
	}
	if got := recordKeys(doc.Root()); strings.Join(got, ",") != "x,y" {
		t.Errorf("after the edit the root holds %v, want [x y]", got)
	}
}

// Rule 7. Fails if the fold starts guessing at a document TOML cannot express
// -- here a table and an array-of-tables claiming one key, assembled by hand
// because the editing surface refuses to build one.
func TestFold_ImpossibleDocumentIsAHardError(t *testing.T) {
	doc := unfoldableDoc(t)

	_, foldErr := foldDocument(doc)
	if foldErr == nil {
		t.Fatal("folding a document with two constructs on one key succeeded")
	}
	if !errors.Is(foldErr, ErrConflict) {
		t.Errorf("fold error is %v, want a conflict", foldErr)
	}

	defer func() {
		if recover() == nil {
			t.Error("Root() returned instead of panicking on an unfoldable document")
		}
	}()
	doc.Root()
}

// Rule 7. Fails if a key bound to a value stops refusing to also hold a table.
// The header is appended by hand: NewTable refuses this path.
func TestFold_ValueCannotHoldATable(t *testing.T) {
	doc, err := Parse([]byte("a = 1\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	tbl := &TableNode{KeyPath: []string{"a", "b"}}
	tbl.markDirty()
	tbl.nodeTrivia.TrailingNewline = []byte("\n")
	doc.Children = append(doc.Children, tbl)
	if _, err := foldDocument(doc); err == nil {
		t.Fatal("folding a table under a scalar key succeeded")
	}
}

// Fails if the root record stops standing for the document itself, or stops
// reporting how many top-level keys it holds.
func TestFold_RootRecord(t *testing.T) {
	doc, err := Parse([]byte("a = 1\nb = 2\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	root := doc.Root()
	if root.Len() != 2 {
		t.Errorf("root Len() = %d, want 2", root.Len())
	}
	if root.node != doc {
		t.Error("the root record does not stand for the document")
	}
	if _, ok := root.Get("missing"); ok {
		t.Error("Get reported a key the document does not carry")
	}
}

// Fails if Entries stops honouring an early break -- a caller that stops
// reading must not force the whole record to be walked.
func TestFold_EntriesStopsEarly(t *testing.T) {
	root := foldTestDoc(t, "a = 1\nb = 2\nc = 3\n")
	seen := 0
	for range root.Entries() {
		seen++
		break
	}
	if seen != 1 {
		t.Errorf("saw %d entries after breaking, want 1", seen)
	}
}
