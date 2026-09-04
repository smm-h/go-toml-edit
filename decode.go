package tomledit

import (
	"encoding"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// The decode engine.
//
// There is one engine, and it walks the read-layer rather than the syntax
// tree: a dotted key, a [header] table and an inline table are the same thing
// to it, because they are the same thing to the document. What it walks
// against is a descriptor (see Spec), and the reflection front end below
// derives one from a Go type rather than re-deriving the document's structure
// a second time.
//
// Decoding is strict, and strictness is the only mode: a key matching no field
// is an error, a value of a kind the target refuses is an error, a value the
// target cannot hold exactly is an error, and a required key the document does
// not carry is an error. A map-typed or any-typed target is TOTAL -- every key
// matches it by construction -- which is why such a target reports no unknown
// keys; that is totality, not leniency.
//
// Violations are collected. After one, the engine continues across sibling
// keys and elements but never descends below the construct it refused, so a
// document reports every independent problem it has and none of the noise a
// broken construct's interior would produce.

// Unmarshaler is implemented by types that decode themselves from a node of
// the syntax tree. UnmarshalTOML is handed the node the key binds -- a
// StringNode, a TableNode, an ArrayNode -- before any rule of the conversion
// table applies, so a type that implements it decides its own decoding
// entirely.
type Unmarshaler interface {
	UnmarshalTOML(node Node) error
}

var (
	unmarshalerIface     = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
	textUnmarshalerIface = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

// noValue is the destination of a walk that only validates: an invalid
// reflect.Value, which every write below skips.
var noValue reflect.Value

// --- public entry points ---

// Unmarshal parses TOML data and decodes it into v, which must be a non-nil
// pointer.
//
// Decoding is strict: see Document.Decode, whose rules Unmarshal is exactly.
// A parse failure is reported alone (parsing cannot continue past a syntax
// error); decode failures are collected and reported together as an Errors
// aggregate.
func Unmarshal(data []byte, v any) error {
	doc, err := Parse(data)
	if err != nil {
		return err
	}
	return doc.Decode(v)
}

// Decode decodes the document into v, which must be a non-nil pointer -- to a
// struct, a map with string keys, or any.
//
// Struct fields are matched by their "toml" tag, or, with no tag, by their
// exact field name. Matching is exact: a document key that differs only in
// case from a field's name is an unknown key, not a match. A tag of "-"
// excludes the field, and an excluded
// name is not part of the document's universe at all: a document key naming
// one is as unknown as any other. The only tag option the package reads is
// "required" (`toml:"port,required"`), which makes the key's absence an error;
// any other option is refused, because an option nothing reads is a silent
// no-op. Embedded structs promote their fields, pointer fields are allocated
// as they are reached, and a type implementing Unmarshaler or
// encoding.TextUnmarshaler decodes itself.
//
// Decoding is strict and strictness is the only mode: an unknown key, an
// unknown table, a value of a refused kind, a value the target cannot hold
// exactly, and a missing required key are all errors. A map-typed or any-typed
// target matches every key by construction and so reports no unknown keys.
//
// Every independent violation is collected and reported together as an *Errors
// aggregate, in document order; it renders as its first diagnostic and exposes
// the rest through errors.As. Values that decoded before a violation stay
// decoded: a failed Decode leaves v partially written.
func (d *Document) Decode(v any) error {
	dst, err := decodeTarget(v, "Decode")
	if err != nil {
		return err
	}
	root, err := foldDocument(d)
	if err != nil {
		return d.diag(err, "")
	}
	en := newEngine()
	en.decodeRecord(root, d, dst, "")
	return d.diag(en.result(), "")
}

// DecodeNode decodes a single node into v, which must be a non-nil pointer. It
// is Decode scoped to one construct: a table, an array-table entry, an inline
// table or a document decodes like a table, an array decodes like an array,
// and a scalar node decodes into a scalar target. A key, a key-value pair and
// a comment carry no value of their own and are refused.
//
// The rules are Decode's, including strictness.
func DecodeNode(n Node, v any) error {
	dst, err := decodeTarget(v, "DecodeNode")
	if err != nil {
		return err
	}
	if n == nil {
		return fmt.Errorf("tomledit: DecodeNode needs a node, got nil")
	}
	en := newEngine()
	switch node := n.(type) {
	case *Document:
		root, err := foldDocument(node)
		if err != nil {
			return err
		}
		en.decodeRecord(root, node, dst, "")
	case *TableNode:
		rec := newRecord(node, node.Span(), true)
		if err := foldChildren(rec, node.Children); err != nil {
			return err
		}
		en.decodeRecord(rec, node, dst, "")
	case *ArrayTableNode:
		rec := newRecord(node, node.Span(), true)
		if err := foldChildren(rec, node.Children); err != nil {
			return err
		}
		en.decodeRecord(rec, node, dst, "")
	case *InlineTableNode:
		rec, err := foldInlineTable(node)
		if err != nil {
			return err
		}
		en.decodeRecord(rec, node, dst, "")
	case *KeyValueNode, *KeyNode, *CommentNode:
		return fmt.Errorf("tomledit: DecodeNode cannot decode a %s node", n.Type())
	default:
		d, err := en.descForType(dst.Type())
		if err != nil {
			return err
		}
		en.walkValue(node, d, dst, "", Span{})
	}
	return en.result()
}

// decodeTarget checks a decode target and returns the value to write into.
func decodeTarget(v any, what string) (reflect.Value, error) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return reflect.Value{}, fmt.Errorf("tomledit: %s requires a non-nil pointer, got %T", what, v)
	}
	return rv.Elem(), nil
}

// --- the compiled descriptor ---
//
// Both front ends compile into these types, so the engine has one shape to
// walk: a Spec compiles through spec.go, a Go type through descForType below.

// hookKind names the custom decoder a target carries, if any.
type hookKind int

const (
	hookNone hookKind = iota
	hookTOML          // the target implements Unmarshaler
)

// containerKind names what a table descriptor writes into.
type containerKind int

const (
	containerNone   containerKind = iota // a hand-built descriptor: nothing is written
	containerStruct                      // a Go struct: keys are fields
	containerMap                         // a Go map: keys are elements
)

// desc is one compiled expectation: what a value must be, and where it goes.
type desc struct {
	class targetClass // the row of the conversion table this target reads
	label string      // how a diagnostic names the target
	elem  *desc       // the elements of an array
	table *tableDesc  // the keys of a table

	rt   reflect.Type // the Go target type; nil for a hand-built descriptor
	hook hookKind     // the custom decoder the target carries

	// text is set when the target implements encoding.TextUnmarshaler. Unlike
	// hookTOML, which takes the target's decoding over entirely, it applies to
	// STRING values only: everything else the target takes still reads the
	// conversion table, which is what lets time.Time accept both a date-time
	// and an RFC 3339 string.
	text bool
}

// expects names what this target accepts, for a diagnostic.
func (d *desc) expects() string {
	if d.label != "" {
		return d.label
	}
	return d.class.describeAccepted()
}

// tableDesc is the compiled key set of a table.
type tableDesc struct {
	fields  map[string]*fieldSlot
	names   []string   // sorted: the order missing keys are reported in
	dynamic *fieldSlot // the descriptor every unnamed key reads

	container containerKind
	keyRT     reflect.Type // the map's key type
	elemRT    reflect.Type // the map's element type
}

// lookup finds the slot a document key binds: an exact match, then the dynamic
// slot that makes a table total. Matching never folds case -- a key that
// differs in case from every declared name is unknown, in both front ends.
func (td *tableDesc) lookup(key string) (*fieldSlot, bool) {
	if slot, ok := td.fields[key]; ok {
		return slot, true
	}
	if td.dynamic != nil {
		return td.dynamic, true
	}
	return nil, false
}

// fieldSlot is one key of a table descriptor: what it expects, and where it
// goes. In the reflection front end the descriptor is derived on first use, so
// a Go type whose SHAPE the document never reaches is never checked against the
// conversion table. Its TAGS are checked all the same -- see checkTags.
type fieldSlot struct {
	name     string
	required bool
	desc     *desc        // the compiled descriptor, once it exists
	rt       reflect.Type // the Go type to derive it from
	index    []int        // the struct field index path to reach it
}

// --- the engine ---

// engine walks a record against a descriptor, collecting diagnostics.
type engine struct {
	diags []*Error
	fatal error // a target or descriptor the engine cannot work with at all

	types  map[reflect.Type]*desc
	tables map[reflect.Type]*tableDesc
	tagged map[reflect.Type]bool // types whose tag rules have been checked
}

func newEngine() *engine {
	return &engine{
		types:  map[reflect.Type]*desc{},
		tables: map[reflect.Type]*tableDesc{},
		tagged: map[reflect.Type]bool{},
	}
}

// add records one diagnostic.
func (en *engine) add(d *Error) { en.diags = append(en.diags, d) }

// result reports what the walk found: the aggregate of its diagnostics, or the
// one error that stopped it.
func (en *engine) result() error {
	if en.fatal != nil {
		return en.fatal
	}
	return newErrors(en.diags)
}

// slotDesc returns a slot's descriptor, deriving it from the Go type on first
// use.
func (en *engine) slotDesc(slot *fieldSlot) (*desc, error) {
	if slot.desc != nil {
		return slot.desc, nil
	}
	d, err := en.descForType(slot.rt)
	if err != nil {
		return nil, err
	}
	slot.desc = d
	return d, nil
}

// decodeRecord decodes a record into a Go value, giving a custom decoder on
// the value itself precedence over every key it holds.
func (en *engine) decodeRecord(rec *Record, node Node, dst reflect.Value, path string) {
	d, err := en.descForType(dst.Type())
	if err != nil {
		en.fatal = err
		return
	}
	en.walkTable(rec, d, dst, path, node.Span())
}

// walkRecord checks a record's entries against a table descriptor and, when
// decoding, writes each into dst.
func (en *engine) walkRecord(rec *Record, td *tableDesc, dst reflect.Value, path string) {
	if en.fatal != nil {
		return
	}
	if dst.IsValid() && td.container == containerMap && dst.IsNil() {
		dst.Set(reflect.MakeMap(dst.Type()))
	}

	seen := map[*fieldSlot]bool{}
	for e := range rec.Entries() {
		if en.fatal != nil {
			return
		}
		slot, ok := td.lookup(e.key)
		if !ok {
			en.unknown(e, path)
			continue
		}
		seen[slot] = true
		d, err := en.slotDesc(slot)
		if err != nil {
			en.fatal = err
			return
		}
		dest := en.destination(td, slot, dst)
		if en.walkEntry(e, d, dest, keyPath(path, e.key)) && td.container == containerMap && dst.IsValid() {
			dst.SetMapIndex(reflect.ValueOf(e.key).Convert(td.keyRT), dest)
		}
	}

	// Missing required keys, in lexicographic order: a map has no order of its
	// own to report them in.
	for _, name := range td.names {
		slot := td.fields[name]
		if !slot.required || seen[slot] {
			continue
		}
		en.add(newError(KindMissingKey, "required key %q is missing", name).
			at(rec.span.Start).within(rec.span).atPath(keyPath(path, name)))
	}
}

// destination returns the value one key of a table writes into, or the invalid
// value when the engine is only validating.
func (en *engine) destination(td *tableDesc, slot *fieldSlot, dst reflect.Value) reflect.Value {
	if !dst.IsValid() {
		return noValue
	}
	switch td.container {
	case containerStruct:
		return fieldByIndex(dst, slot.index)
	case containerMap:
		return reflect.New(td.elemRT).Elem()
	}
	return noValue
}

// walkEntry checks one entry against its descriptor and, when decoding, writes
// it into dst. It reports whether dst received a value.
func (en *engine) walkEntry(e Entry, d *desc, dst reflect.Value, path string) bool {
	switch e.kind {
	case EntryRecord:
		return en.walkTable(e.record, d, dst, path, e.keySpan)
	case EntryRecords:
		return en.walkCollection(e, d, dst, path)
	default:
		return en.walkValue(e.node, d, dst, path, e.keySpan)
	}
}

// walkTable checks a table, in whatever spelling the document used, against
// its descriptor.
func (en *engine) walkTable(rec *Record, d *desc, dst reflect.Value, path string, keySpan Span) bool {
	pos, span := diagPlace(keySpan, rec.span)
	if d.hook == hookTOML {
		if rec.node == nil {
			en.add(newError(KindWrongContainer,
				"the custom decoder for %s needs the table's own node, and this table has none: it is implied by a longer header or a dotted key",
				d.expects()).at(pos).within(span).atPath(path))
			return false
		}
		return en.hookTOML(dst, rec.node, path)
	}
	if !d.class.accepts(valTable) {
		en.mismatch(d, valTable, path, pos, span)
		return false
	}
	dst = indirectValue(dst)
	if d.class == targetAny {
		return en.setNative(dst, func() (any, *Error) { return nativeRecord(rec) }, path, pos, span)
	}
	en.walkRecord(rec, d.table, dst, path)
	return true
}

// walkCollection checks an array-of-tables against its descriptor.
func (en *engine) walkCollection(e Entry, d *desc, dst reflect.Value, path string) bool {
	pos, span := diagPlace(e.keySpan, e.recordsSpan)
	if d.hook == hookTOML {
		en.add(newError(KindWrongContainer,
			"the custom decoder for %s needs one node, and an array-of-tables is not one", d.expects()).
			at(pos).within(span).atPath(path))
		return false
	}
	if !d.class.accepts(valArrayOfTables) {
		en.mismatch(d, valArrayOfTables, path, pos, span)
		return false
	}
	dst = indirectValue(dst)
	if d.class == targetAny {
		return en.setNative(dst, func() (any, *Error) { return nativeEntry(e) }, path, pos, span)
	}
	return en.fill(len(e.records), d, dst, path, pos, span, func(i int, elem reflect.Value) {
		en.walkTable(e.records[i], d.elem, elem, indexPath(path, i), Span{})
	})
}

// walkValue checks one value node against its descriptor. keySpan positions a
// diagnostic about a node that carries no source range of its own.
func (en *engine) walkValue(n Node, d *desc, dst reflect.Value, path string, keySpan Span) bool {
	pos, span := diagPlace(keySpan, n.Span())
	if d.hook == hookTOML {
		return en.hookTOML(dst, n, path)
	}
	kind, ok := valueKindOf(n)
	if !ok {
		en.add(newError(KindTypeMismatch, "a %s node carries no value", n.Type()).
			at(pos).within(span).atPath(path))
		return false
	}
	if d.text && kind == valString {
		// The text decoder takes the string rows; the table still governs
		// every other value this target accepts.
		return en.hookText(dst, n.(*StringNode).Val, path)
	}
	if !d.class.accepts(kind) {
		en.mismatch(d, kind, path, pos, span)
		return false
	}

	dst = indirectValue(dst)
	switch d.class {
	case targetAny:
		return en.setNative(dst, func() (any, *Error) { return nativeValue(n) }, path, pos, span)
	case targetArray:
		arr := n.(*ArrayNode)
		return en.fill(len(arr.Elements), d, dst, path, pos, span, func(i int, elem reflect.Value) {
			en.walkValue(arr.Elements[i], d.elem, elem, indexPath(path, i), span)
		})
	case targetTable:
		rec, err := foldInlineTable(n.(*InlineTableNode))
		if err != nil {
			en.add(asDiagnostic(err).at(pos).within(span).atPath(path))
			return false
		}
		en.walkRecord(rec, d.table, dst, path)
		return true
	default:
		if cerr := convertValue(n, d.class, dst); cerr != nil {
			en.add(cerr.at(pos).within(span).atPath(path))
			return false
		}
		return true
	}
}

// fill walks the n elements of an array or an array-of-tables into dst, which
// is a Go slice, a Go array, or invalid while validating. A fixed-size Go
// array declares its arity, so a length other than its own is refused rather
// than padded or truncated.
func (en *engine) fill(n int, d *desc, dst reflect.Value, path string, pos Position, span Span, element func(i int, dst reflect.Value)) bool {
	if !dst.IsValid() {
		for i := 0; i < n; i++ {
			element(i, noValue)
		}
		return false
	}
	switch dst.Kind() {
	case reflect.Slice:
		out := reflect.MakeSlice(dst.Type(), n, n)
		for i := 0; i < n; i++ {
			element(i, out.Index(i))
		}
		dst.Set(out)
		return true
	case reflect.Array:
		if n != dst.Len() {
			en.add(newError(KindInexact, "array has %d elements but %s requires exactly %d", n, dst.Type(), dst.Len()).
				withValue(n).at(pos).within(span).atPath(path))
			return false
		}
		for i := 0; i < n; i++ {
			element(i, dst.Index(i))
		}
		return true
	}
	return false
}

// setNative writes the native Go form of a document value into an any-typed
// target.
func (en *engine) setNative(dst reflect.Value, native func() (any, *Error), path string, pos Position, span Span) bool {
	value, err := native()
	if err != nil {
		en.add(err.at(pos).within(span).atPath(path))
		return false
	}
	if dst.IsValid() && value != nil {
		dst.Set(reflect.ValueOf(value))
	}
	return true
}

// hookTOML hands a node to the target's own decoder.
func (en *engine) hookTOML(dst reflect.Value, n Node, path string) bool {
	if !dst.IsValid() {
		return false
	}
	u, ok := tomlDecoderOf(dst)
	if !ok {
		en.fatal = fmt.Errorf("tomledit: %s implements Unmarshaler but the value at %s cannot be addressed", dst.Type(), pathLabel(path))
		return false
	}
	if err := u.UnmarshalTOML(n); err != nil {
		en.fatal = wrapError(err, "%s", pathLabel(path))
		return false
	}
	return true
}

// hookText hands a string to the target's own text decoder.
func (en *engine) hookText(dst reflect.Value, text string, path string) bool {
	if !dst.IsValid() {
		return false
	}
	tu, ok := textDecoderOf(dst)
	if !ok {
		en.fatal = fmt.Errorf("tomledit: %s implements encoding.TextUnmarshaler but the value at %s cannot be addressed", dst.Type(), pathLabel(path))
		return false
	}
	if err := tu.UnmarshalText([]byte(text)); err != nil {
		en.fatal = wrapError(err, "%s", pathLabel(path))
		return false
	}
	return true
}

// --- diagnostics ---

// unknown reports a key the descriptor does not carry. A table reports one
// diagnostic naming its direct child keys rather than one per key inside it.
func (en *engine) unknown(e Entry, path string) {
	sub := keyPath(path, e.key)
	switch e.kind {
	case EntryRecord:
		pos, span := diagKey(e.keySpan, e.record.span)
		en.add(newError(KindUnknownTable, "unknown table %q", e.key).
			listing(recordKeyList(e.record)).at(pos).within(span).atPath(sub))
	case EntryRecords:
		var keys []string
		if len(e.records) > 0 {
			keys = recordKeyList(e.records[0])
		}
		pos, span := diagKey(e.keySpan, e.recordsSpan)
		en.add(newError(KindUnknownTable, "unknown array of tables %q", e.key).
			listing(keys).at(pos).within(span).atPath(sub))
	default:
		pos, span := diagKey(e.keySpan, e.node.Span())
		en.add(newError(KindUnknownKey, "unknown key %q", e.key).at(pos).within(span).atPath(sub))
	}
}

// mismatch reports a value whose kind the conversion table refuses for this
// target.
func (en *engine) mismatch(d *desc, got valueKind, path string, pos Position, span Span) {
	en.add(newError(KindTypeMismatch, "cannot decode %s into %s", got, d.expects()).
		expects(d.expects(), got.String()).at(pos).within(span).atPath(path))
}

// diagPlace places a diagnostic about a VALUE: at the key when the construct
// names one, otherwise at the construct itself, and over the construct's own
// range when it has one. A diagnostic about the KEY places itself with diagKey
// instead.
func diagPlace(keySpan, span Span) (Position, Span) {
	pos := keySpan.Start
	if !keySpan.IsValid() {
		pos = span.Start
	}
	if !span.IsValid() {
		span = keySpan
	}
	return pos, span
}

// diagKey places a diagnostic about a KEY: at the key and over the key's own
// range, so the position and the range describe one construct rather than two.
// A consumer that underlines Span underlines what the message names.
//
// A key whose own range the document does not carry -- a node built
// programmatically rather than parsed -- falls back to the construct the key
// binds, which is the only range there is.
func diagKey(keySpan, span Span) (Position, Span) {
	if keySpan.IsValid() {
		return keySpan.Start, keySpan
	}
	return span.Start, span
}

// recordKeyList returns a record's keys in document order, for the inventory
// an unknown-table diagnostic carries.
func recordKeyList(rec *Record) []string {
	keys := make([]string, 0, rec.Len())
	for e := range rec.Entries() {
		keys = append(keys, e.key)
	}
	return keys
}

// pathLabel names a path for a message, or the document itself.
func pathLabel(path string) string {
	if path == "" {
		return "the document root"
	}
	return path
}

// --- the reflection front end ---
//
// A Go type IS a descriptor: this derives the same compiled form a Spec
// compiles to, so the engine cannot tell the two front ends apart.

// descForType derives the descriptor a Go type declares. Deriving is memoized
// per type, which is also what makes a recursive type terminate.
//
// A type entering derivation has its whole reachable tag graph checked first
// (see checkTags), so a tag defect is reported for every document the target is
// used with rather than only for the ones whose keys descend far enough to
// derive the offending type.
func (en *engine) descForType(rt reflect.Type) (*desc, error) {
	if err := en.checkTags(rt); err != nil {
		return nil, err
	}
	if d, ok := en.types[rt]; ok {
		return d, nil
	}
	d := &desc{rt: rt, label: rt.String()}
	en.types[rt] = d
	if err := en.fillDesc(d, rt); err != nil {
		delete(en.types, rt)
		return nil, err
	}
	return d, nil
}

// fillDesc derives one type's expectations: its custom decoder if it has one,
// otherwise its row of the conversion table and whatever it contains.
func (en *engine) fillDesc(d *desc, rt reflect.Type) error {
	if implementsHook(rt, unmarshalerIface) {
		d.hook = hookTOML
		d.class = targetAny
		return nil
	}
	text := implementsHook(rt, textUnmarshalerIface)
	if rt.Kind() == reflect.Pointer {
		// A pointer decodes as what it points to; the pointer itself is
		// allocated when the value is written.
		inner, err := en.descForType(rt.Elem())
		if err != nil {
			return err
		}
		label := d.label
		*d = *inner
		d.rt, d.label = rt, label
		d.text = d.text || text
		return nil
	}
	class, ok := targetClassOf(rt)
	if !ok {
		return fmt.Errorf("tomledit: cannot decode into %s", rt)
	}
	d.class = class
	d.text = text
	switch class {
	case targetArray:
		elem, err := en.descForType(rt.Elem())
		if err != nil {
			return err
		}
		d.elem = elem
	case targetTable:
		table, err := en.tableForType(rt)
		if err != nil {
			return err
		}
		d.table = table
	}
	return nil
}

// checkTags derives the field mapping of every struct type the target can
// reach, so a toml tag the package cannot honour is refused whatever the
// document says. A tag rule is a property of the type: "nonsense" is not an
// option this package reads and a tag on an unexported field names a key
// nothing can bind, and neither becomes true or false because a document
// happens to carry the key above it.
//
// Only TAG rules are eager. Whether a type can be decoded AT ALL stays lazy:
// an untagged field of a type no row of the conversion table accepts is
// refused when a document key reaches it, and is otherwise none of the
// document's business. So this walk never asks targetClassOf anything -- it
// follows only the type constructors decoding itself follows (a pointer, the
// element of a slice, an array or a map, and the fields a struct's mapping
// binds), and derives a struct's key set to surface what collectFields refuses.
//
// A type that decodes itself through Unmarshaler ends the walk: the engine
// hands it the node whole and maps none of its fields, so its tags are not
// field mappings at all.
//
// The visited set is what makes a recursive type terminate; it is dropped
// again on failure so a later derivation of the same type reports the same
// error rather than silently passing.
func (en *engine) checkTags(rt reflect.Type) error {
	if rt == nil || en.tagged[rt] {
		return nil
	}
	en.tagged[rt] = true
	if err := en.walkTags(rt); err != nil {
		delete(en.tagged, rt)
		return err
	}
	return nil
}

// walkTags is checkTags without the memoization, which its caller owns.
func (en *engine) walkTags(rt reflect.Type) error {
	if implementsHook(rt, unmarshalerIface) {
		return nil
	}
	switch rt.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return en.checkTags(rt.Elem())
	case reflect.Struct:
		td, err := en.tableForType(rt)
		if err != nil {
			return err
		}
		// In name order: the mapping's own order is a map's, which has none.
		for _, name := range td.names {
			if err := en.checkTags(td.fields[name].rt); err != nil {
				return err
			}
		}
	}
	return nil
}

// tableForType derives the key set of a struct or a map. A map is total: every
// key matches its element descriptor, so no key is ever unknown.
func (en *engine) tableForType(rt reflect.Type) (*tableDesc, error) {
	if td, ok := en.tables[rt]; ok {
		return td, nil
	}
	td := &tableDesc{fields: map[string]*fieldSlot{}}
	en.tables[rt] = td

	switch rt.Kind() {
	case reflect.Map:
		if rt.Key().Kind() != reflect.String {
			delete(en.tables, rt)
			return nil, fmt.Errorf("tomledit: cannot decode into %s: a map key must be a string", rt)
		}
		td.container = containerMap
		td.keyRT, td.elemRT = rt.Key(), rt.Elem()
		td.dynamic = &fieldSlot{rt: rt.Elem()}
	case reflect.Struct:
		td.container = containerStruct
		if err := en.collectFields(rt, nil, td); err != nil {
			delete(en.tables, rt)
			return nil, err
		}
		sort.Strings(td.names)
	}
	return td, nil
}

// collectFields builds a struct's key set, promoting the fields of embedded
// structs into it. The first name wins, as it does today: a name already
// bound is not rebound by a field found later.
func (en *engine) collectFields(rt reflect.Type, prefix []int, td *tableDesc) error {
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag, tagged := f.Tag.Lookup("toml")
		if !f.IsExported() {
			if tagged {
				return fmt.Errorf("tomledit: %s.%s carries a toml tag but is unexported, so no document key can reach it", rt, f.Name)
			}
			continue
		}
		name, required, excluded, err := parseFieldTag(tag, rt, f.Name)
		if err != nil {
			return err
		}
		if excluded {
			continue
		}

		index := append(append([]int(nil), prefix...), i)
		if f.Anonymous && name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				if required {
					return fmt.Errorf("tomledit: %s.%s is embedded, so the required tag option names no key", rt, f.Name)
				}
				if err := en.collectFields(ft, index, td); err != nil {
					return err
				}
				continue
			}
		}
		if name == "" {
			name = f.Name
		}

		slot := &fieldSlot{name: name, required: required, rt: f.Type, index: index}
		if _, exists := td.fields[name]; !exists {
			td.fields[name] = slot
			td.names = append(td.names, name)
		}
	}
	return nil
}

// parseFieldTag reads a "toml" struct tag: a name, then options. "required" is
// the one option the package reads; any other is refused, because an option
// nothing reads would silently do nothing.
func parseFieldTag(tag string, rt reflect.Type, field string) (name string, required, excluded bool, err error) {
	if tag == "-" {
		return "", false, true, nil
	}
	parts := strings.Split(tag, ",")
	for _, opt := range parts[1:] {
		switch opt {
		case "required":
			required = true
		default:
			return "", false, false, fmt.Errorf("tomledit: %s.%s carries the unknown toml tag option %q", rt, field, opt)
		}
	}
	return parts[0], required, false, nil
}

// --- reflection helpers ---

// implementsHook reports whether rt, or a pointer to it, implements iface. A
// method set declared on the pointer receiver still counts: every value the
// engine writes into is addressable.
func implementsHook(rt reflect.Type, iface reflect.Type) bool {
	return rt.Implements(iface) || reflect.PointerTo(rt).Implements(iface)
}

// tomlDecoderOf returns the target's own decoder.
func tomlDecoderOf(rv reflect.Value) (Unmarshaler, bool) {
	if v, ok := hookValue(rv, unmarshalerIface); ok {
		u, ok := v.(Unmarshaler)
		return u, ok
	}
	return nil, false
}

// textDecoderOf returns the target's own text decoder.
func textDecoderOf(rv reflect.Value) (encoding.TextUnmarshaler, bool) {
	if v, ok := hookValue(rv, textUnmarshalerIface); ok {
		tu, ok := v.(encoding.TextUnmarshaler)
		return tu, ok
	}
	return nil, false
}

// hookValue returns the interface value a target's custom decoder is reached
// through, allocating a nil pointer target on the way.
func hookValue(rv reflect.Value, iface reflect.Type) (any, bool) {
	if !rv.IsValid() {
		return nil, false
	}
	if rv.Kind() == reflect.Pointer && rv.Type().Implements(iface) {
		if rv.IsNil() {
			if !rv.CanSet() {
				return nil, false
			}
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		return rv.Interface(), true
	}
	if rv.CanAddr() && reflect.PointerTo(rv.Type()).Implements(iface) {
		return rv.Addr().Interface(), true
	}
	if rv.Type().Implements(iface) && rv.CanInterface() {
		return rv.Interface(), true
	}
	return nil, false
}

// indirectValue follows pointers to the value they address, allocating the nil
// ones on the way, so a pointer target decodes like the value it points to.
func indirectValue(rv reflect.Value) reflect.Value {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			if !rv.CanSet() {
				return rv
			}
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}
	return rv
}

// fieldByIndex walks a struct field index path, allocating the nil pointers of
// embedded structs on the way.
func fieldByIndex(rv reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		if rv.Kind() == reflect.Pointer {
			if rv.IsNil() {
				rv.Set(reflect.New(rv.Type().Elem()))
			}
			rv = rv.Elem()
		}
		rv = rv.Field(i)
	}
	return rv
}
