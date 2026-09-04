package tomledit

import "iter"

// The read-layer: the logical view of a document.
//
// The AST answers syntactic questions -- what the file contains, in the form it
// was written. The read-layer answers logical ones -- what the document means:
// which keys exist, in what order, and what each holds. The two differ wherever
// TOML offers several spellings of one meaning, and the layer is where those
// differences disappear: a dotted key, a [header] table and an inline table all
// fold into the same kind of record, and the entries of an array-of-tables
// collect under one key no matter how far apart their headers sit.
//
// The layer is built by a fold over the parsed document, following these rules:
//
//  1. Keys appear in first-appearance order, across every binding form.
//  2. A dotted key expands: "a.b.c = 1" creates records "a" and "b" on the way
//     to the value "c".
//  3. A header whose prefix records do not exist creates them implicitly, and a
//     later header for one of those prefixes reopens it: "[a.b]" followed by
//     "[a]" yields one record "a" holding "b" first, anchored at its own
//     header.
//  4. Array-of-tables entries collect under one key, and a sub-table header
//     under an array-of-tables prefix addresses the LAST entry.
//  5. Inline tables fold into ordinary records: a record's origin spelling is
//     not distinguishable through the layer.
//  6. Every entry carries the source range of its key, and every record the
//     range of the construct it is anchored to.
//  7. A conflict the parser accepts cannot reach the fold. Anything the fold
//     finds impossible -- an unhandled node kind, two constructs claiming one
//     slot -- is a hard error, never a silent guess.

// EntryKind classifies what a read-layer entry holds.
type EntryKind int

const (
	// EntryValue is a scalar or a plain array: one concrete value node.
	EntryValue EntryKind = iota
	// EntryRecord is a table in any spelling -- a header table, an inline
	// table, or a table implied by a longer header or a dotted key.
	EntryRecord
	// EntryRecords is an array-of-tables: the entries collected under one key,
	// in document order.
	EntryRecords
)

var entryKindNames = [...]string{
	EntryValue:   "value",
	EntryRecord:  "record",
	EntryRecords: "records",
}

// String returns the human-readable name of the entry kind.
func (k EntryKind) String() string {
	if int(k) >= 0 && int(k) < len(entryKindNames) {
		return entryKindNames[k]
	}
	return "unknown"
}

// Record is a table of the read-layer: an ordered set of entries, whatever the
// spelling that produced it. Records are read-only; edit through the document's
// path API.
//
// A record is a snapshot of the document as it was when the layer was built. It
// stays valid until the next mutation of that document, and reading it
// afterwards reports what the document held before the change. Mutating a
// document while iterating its entries is unspecified.
type Record struct {
	entries []Entry
	index   map[string]int

	// span is the record's anchor: its own header where it has one, otherwise
	// the construct that created it.
	span Span

	// node is the single concrete node backing the record -- a document, a
	// table header, an array-of-tables entry, or an inline table. It is nil for
	// a record no single node stands for: one implied by a longer header or by
	// a dotted key.
	node Node

	// anchored records whether span and node come from the record's own header
	// (or its own inline table), rather than from the construct that implied
	// it. The first header wins: a later one cannot re-anchor a record.
	anchored bool

	// dottedImplied records that nothing but a dotted key spells this record
	// out: "a.b = 1" implies "a". TOML forbids giving such a table a header of
	// its own, so the structural creators refuse to bind its name, where a
	// record merely implied by a longer header may still be anchored.
	dottedImplied bool

	// container is the concrete node whose children carry the dotted key that
	// spelled this record out -- the document, the [header] table, the
	// array-of-tables entry or the inline table the pair was written in -- and
	// prefix is the key path from that container to this record. Together they
	// name the REGION a key written into this table belongs in, which is how a
	// table no single node stands for is still written to. Both are unset for a
	// record with a node of its own and for one only a longer header implies.
	container Node
	prefix    []string
}

// Entry is one key of a record: the key, where it was written, and what it
// holds. Entries are values; holding one does not keep the document alive in
// any special way, and like the records they come from they are snapshots.
type Entry struct {
	key     string
	keySpan Span
	kind    EntryKind

	// node is the concrete node this entry stands for, when exactly one does.
	node Node

	// record is set for EntryRecord, records for EntryRecords.
	record      *Record
	records     []*Record
	recordsSpan Span
}

// Root returns the read-layer view of the document: the record holding every
// top-level key, in first-appearance order.
//
// Parsing folds nothing; the layer is built the first time a logical question
// is asked and kept until the document is written to, so repeated reads share
// one fold and reads of a shared document stay safe under concurrency. A write
// drops it: the next call folds again and answers what the document says then.
// A record obtained BEFORE a write keeps answering what the document said
// before it -- layer handles are snapshots, and a stale one reports stale data
// rather than reporting an error. Mutating a document while iterating its
// entries is unspecified.
//
// Root panics when the document cannot be folded -- when two constructs claim
// one key, for instance. A parsed document never can be: the parser refuses
// every such conflict. Only an editing sequence that built an invalid document
// can reach it.
func (d *Document) Root() *Record {
	root, err := d.readLayer()
	if err != nil {
		panic("tomledit: " + err.Error())
	}
	return root
}

// Len returns the number of entries in the record.
func (r *Record) Len() int { return len(r.entries) }

// Span returns the record's anchoring range: its own header where it has one,
// the inline table that spells it out, or the construct that implied it.
func (r *Record) Span() Span { return r.span }

// Node returns the concrete node backing the record, and whether one does. A
// table written as a header or as an inline table is backed by that construct,
// an array-of-tables entry by its own [[header]], and the root record by the
// document. A record no single node stands for -- one implied by a longer
// header or by a dotted key -- reports false; it exists in the layer, and
// nothing in the source stands for it alone.
func (r *Record) Node() (Node, bool) {
	if r.node == nil {
		return nil, false
	}
	return r.node, true
}

// impliedRegion returns the concrete container whose children spell this record
// out and the key path from that container to it. It reports false for a record
// with a node of its own and for one only a longer header implies: neither has
// a region of dotted pairs to be written into.
func (r *Record) impliedRegion() (Node, []string, bool) {
	if r.container == nil {
		return nil, nil, false
	}
	return r.container, r.prefix, true
}

// dottedKV returns the pair that binds key inside this record's own dotted
// region -- the "a.b = 1" line for key "b" of the table "a.b = 1" implies --
// or nil when no pair there binds it.
func (r *Record) dottedKV(key string) *KeyValueNode {
	if r.container == nil {
		return nil
	}
	children, err := contents(r.container)
	if err != nil {
		return nil
	}
	want := len(r.prefix) + 1
	for _, child := range children {
		kv, ok := child.(*KeyValueNode)
		if !ok || len(kv.key.parts) != want {
			continue
		}
		if kv.key.parts[want-1] == key && pathsEqual(kv.key.parts[:want-1], r.prefix) {
			return kv
		}
	}
	return nil
}

// Entries iterates the record's entries in first-appearance order.
func (r *Record) Entries() iter.Seq[Entry] {
	return func(yield func(Entry) bool) {
		for _, e := range r.entries {
			if !yield(e) {
				return
			}
		}
	}
}

// Get returns the entry with the given key, and whether the record has one.
func (r *Record) Get(key string) (Entry, bool) {
	i, ok := r.index[key]
	if !ok {
		return Entry{}, false
	}
	return r.entries[i], true
}

// Key returns the entry's key, decoded (quotes and escapes resolved).
func (e Entry) Key() string { return e.key }

// KeySpan returns the source range of the key itself -- the one part of the
// construct the entry is anchored to that names it.
func (e Entry) KeySpan() Span { return e.keySpan }

// Kind reports what the entry holds.
func (e Entry) Kind() EntryKind { return e.kind }

// Record returns the record this entry holds, for an entry of kind
// EntryRecord: a table in any spelling. It reports false for any other kind.
func (e Entry) Record() (*Record, bool) {
	if e.kind != EntryRecord {
		return nil, false
	}
	return e.record, true
}

// Records returns the entries of the array-of-tables this entry holds, for an
// entry of kind EntryRecords. The returned slice is a copy: appending to it or
// reordering it changes nothing in the document. It reports false for any other
// kind.
func (e Entry) Records() ([]*Record, bool) {
	if e.kind != EntryRecords {
		return nil, false
	}
	out := make([]*Record, len(e.records))
	copy(out, e.records)
	return out, true
}

// RecordsSpan returns the synthesized range of an array-of-tables: from the
// first entry's header to the end of the last entry's content, or to the end of
// its header when that entry has no content of its own. It is the zero Span for
// an entry of any other kind -- no concrete node covers a collection, so this is
// the only range that describes one.
func (e Entry) RecordsSpan() Span { return e.recordsSpan }

// Node returns the value node the entry holds, and whether it holds one. It
// answers for entries of kind EntryValue -- a scalar or a plain array -- and
// reports false for every other kind: a record's own backing construct is
// Record.Node's answer, and an array-of-tables has no single node at all, only
// the entries Records returns.
func (e Entry) Node() (Node, bool) {
	if e.kind != EntryValue || e.node == nil {
		return nil, false
	}
	return e.node, true
}

// --- the fold ---

// foldDocument builds the read-layer for a document.
func foldDocument(d *Document) (*Record, error) {
	root := newRecord(d, d.Span(), true)
	for _, child := range d.children {
		var err error
		switch n := child.(type) {
		case *KeyValueNode:
			err = foldKeyValue(root, n)
		case *TableNode:
			err = foldTable(root, n)
		case *ArrayTableNode:
			err = foldArrayTable(root, n)
		case *CommentNode:
			// Trivia carries no logical content.
		default:
			err = foldError("a %s node cannot appear at document level", child.Type())
		}
		if err != nil {
			return nil, err
		}
	}
	return root, nil
}

// newRecord returns an empty record anchored at span, backed by node (which is
// nil for a record no single node stands for).
func newRecord(node Node, span Span, anchored bool) *Record {
	return &Record{index: map[string]int{}, node: node, span: span, anchored: anchored}
}

// add appends an entry, which must not already be present, and returns its
// position.
func (r *Record) add(e Entry) int {
	r.index[e.key] = len(r.entries)
	r.entries = append(r.entries, e)
	return len(r.entries) - 1
}

// foldError reports a document the fold cannot make sense of. The parser
// refuses every conflict that would produce one, so reaching this means an
// editing sequence built a document TOML cannot express.
func foldError(format string, args ...any) *Error {
	return newError(KindConflict, "the document cannot be folded: "+format, args...)
}

// foldTable folds a [header] table and its children into the layer.
func foldTable(root *Record, tbl *TableNode) error {
	if len(tbl.keyPath) == 0 {
		return foldError("a table header names no key")
	}
	parent, err := descend(root, tbl.keyPath[:len(tbl.keyPath)-1], tbl, tbl.keySpans)
	if err != nil {
		return err
	}
	last := len(tbl.keyPath) - 1
	key := tbl.keyPath[last]
	keySpan := spanAt(tbl.keySpans, last)

	i, exists := parent.index[key]
	if !exists {
		i = parent.add(Entry{
			key:     key,
			keySpan: keySpan,
			kind:    EntryRecord,
			node:    tbl,
			record:  newRecord(tbl, tbl.Span(), true),
		})
	} else {
		e := &parent.entries[i]
		if e.kind != EntryRecord {
			return foldError("key %q is a %s and cannot also be a table", key, e.kind)
		}
		if e.record.anchored {
			return foldError("table %q is defined twice", key)
		}
		// The record was implied by an earlier construct; its own header
		// anchors it, and names the key.
		e.record.node = tbl
		e.record.span = tbl.Span()
		e.record.anchored = true
		e.node = tbl
		if keySpan.IsValid() {
			e.keySpan = keySpan
		}
	}
	return foldChildren(parent.entries[i].record, tbl.children)
}

// foldArrayTable folds one [[header]] entry and its children into the layer,
// collecting it with any earlier entries under the same key.
func foldArrayTable(root *Record, atbl *ArrayTableNode) error {
	if len(atbl.keyPath) == 0 {
		return foldError("an array table header names no key")
	}
	parent, err := descend(root, atbl.keyPath[:len(atbl.keyPath)-1], atbl, atbl.keySpans)
	if err != nil {
		return err
	}
	last := len(atbl.keyPath) - 1
	key := atbl.keyPath[last]
	entryRecord := newRecord(atbl, atbl.Span(), true)

	i, exists := parent.index[key]
	if !exists {
		i = parent.add(Entry{
			key:     key,
			keySpan: spanAt(atbl.keySpans, last),
			kind:    EntryRecords,
			records: []*Record{entryRecord},
		})
	} else {
		e := &parent.entries[i]
		if e.kind != EntryRecords {
			return foldError("key %q is a %s and cannot also be an array of tables", key, e.kind)
		}
		e.records = append(e.records, entryRecord)
	}
	if err := foldChildren(entryRecord, atbl.children); err != nil {
		return err
	}
	e := &parent.entries[i]
	e.recordsSpan = collectionSpan(e.records)
	return nil
}

// foldChildren folds the children of a table header or an array-table entry
// into its record.
func foldChildren(rec *Record, children []Node) error {
	for _, child := range children {
		switch n := child.(type) {
		case *KeyValueNode:
			if err := foldKeyValue(rec, n); err != nil {
				return err
			}
		case *CommentNode:
			// Trivia carries no logical content.
		default:
			return foldError("a %s node cannot appear inside a table", child.Type())
		}
	}
	return nil
}

// foldKeyValue folds one key-value pair into rec, expanding a dotted key into
// the records it implies.
func foldKeyValue(rec *Record, kv *KeyValueNode) error {
	parts := kv.key.parts
	if len(parts) == 0 {
		return foldError("a key-value pair names no key")
	}
	cur := rec
	for i := 0; i < len(parts)-1; i++ {
		next, err := cur.implied(parts[i], kv.key.partSpan(i), kv, true)
		if err != nil {
			return err
		}
		if next.node == nil && next.container == nil {
			// This pair spells the table out, so the region it was written in
			// is where a key written into that table belongs.
			next.container = rec.node
			next.prefix = parts[:i+1]
		}
		cur = next
	}
	last := len(parts) - 1
	key := parts[last]
	if _, exists := cur.index[key]; exists {
		return foldError("key %q is defined twice", key)
	}
	if inline, ok := kv.val.(*InlineTableNode); ok {
		sub, err := foldInlineTable(inline)
		if err != nil {
			return err
		}
		cur.add(Entry{
			key:     key,
			keySpan: kv.key.partSpan(last),
			kind:    EntryRecord,
			node:    inline,
			record:  sub,
		})
		return nil
	}
	cur.add(Entry{
		key:     key,
		keySpan: kv.key.partSpan(last),
		kind:    EntryValue,
		node:    kv.val,
	})
	return nil
}

// foldInlineTable folds an inline table into a record. Rule 5: the result is an
// ordinary record, indistinguishable through the layer from a header table.
func foldInlineTable(inline *InlineTableNode) (*Record, error) {
	rec := newRecord(inline, inline.Span(), true)
	for _, child := range inline.children {
		kv, ok := child.(*KeyValueNode)
		if !ok {
			return nil, foldError("a %s node cannot appear inside an inline table", child.Type())
		}
		if err := foldKeyValue(rec, kv); err != nil {
			return nil, err
		}
	}
	return rec, nil
}

// descend walks the prefix parts of a header's key path, creating the records
// the header implies, and returns the record the final part belongs in. A part
// naming an array-of-tables addresses its LAST entry (rule 4).
func descend(rec *Record, parts []string, creator Node, spans []Span) (*Record, error) {
	cur := rec
	for i, part := range parts {
		e, ok := cur.Get(part)
		if !ok {
			next, err := cur.implied(part, spanAt(spans, i), creator, false)
			if err != nil {
				return nil, err
			}
			cur = next
			continue
		}
		switch e.kind {
		case EntryRecord:
			cur = e.record
		case EntryRecords:
			cur = e.records[len(e.records)-1]
		default:
			return nil, foldError("key %q is a %s and cannot hold a table", part, e.kind)
		}
	}
	return cur, nil
}

// implied returns the record at key, creating an implicit one anchored at the
// construct that implies it when the key is not there yet. dotted says whether
// the implying construct is a dotted key rather than a longer header.
func (r *Record) implied(key string, keySpan Span, creator Node, dotted bool) (*Record, error) {
	if e, ok := r.Get(key); ok {
		switch e.kind {
		case EntryRecord:
			return e.record, nil
		case EntryRecords:
			return e.records[len(e.records)-1], nil
		default:
			return nil, foldError("key %q is a %s and cannot hold a table", key, e.kind)
		}
	}
	rec := newRecord(nil, creator.Span(), false)
	rec.dottedImplied = dotted
	r.add(Entry{key: key, keySpan: keySpan, kind: EntryRecord, record: rec})
	return rec, nil
}

// collectionSpan synthesizes the range of an array-of-tables: from the first
// entry's header to the end of the last entry's content, or to the end of that
// entry's header when it holds nothing.
func collectionSpan(records []*Record) Span {
	if len(records) == 0 {
		return Span{}
	}
	first := records[0].span
	lastRec := records[len(records)-1]
	end := lastRec.span.End
	if atbl, ok := lastRec.node.(*ArrayTableNode); ok {
		for _, child := range atbl.children {
			if s := child.Span(); s.IsValid() && s.End.Offset > end.Offset {
				end = s.End
			}
		}
	}
	return Span{Start: first.Start, End: end}
}

// spanAt returns spans[i], or the zero Span when the construct carries no
// per-part spans (one built programmatically).
func spanAt(spans []Span, i int) Span {
	if i < 0 || i >= len(spans) {
		return Span{}
	}
	return spans[i]
}
