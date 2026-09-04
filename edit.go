package tomledit

import (
	"math"
	"reflect"
	"sort"
	"time"
)

// Set updates the value at the given path. If the final key does not exist in
// an existing parent, it is created as a new key-value pair. Returns an error
// if intermediate path segments do not exist.
//
// Supported value types: string, bool, int/int8-64, uint/uint8-64, float32/64,
// time.Time, LocalDateTime, LocalDate, LocalTime, []any, map[string]any, and
// any type implementing the Node interface. Use SetCreate to auto-create
// intermediate tables.
func (d *Document) Set(path string, value any) error {
	return d.diag(d.setInternal(path, value, false), path)
}

// SetCreate is like Set but auto-creates intermediate [table] headers when they
// do not exist. Missing tables are appended to the document. This is convenient
// for inserting values into deeply nested paths that may not yet exist.
func (d *Document) SetCreate(path string, value any) error {
	return d.diag(d.setInternal(path, value, true), path)
}

func (d *Document) setInternal(path string, value any, create bool) error {
	segments, err := ParsePath(path)
	if err != nil {
		return err
	}
	if len(segments) == 0 {
		return newError(KindBadPath, "empty path")
	}

	// The last segment is the target key/index to set.
	parentSegs := segments[:len(segments)-1]
	lastSeg := segments[len(segments)-1]

	// Resolve the parent node.
	parent, err := d.resolveParentForEdit(parentSegs, create)
	if err != nil {
		return err
	}

	// Convert value to a node.
	valNode, err := valueToNode(value)
	if err != nil {
		return err
	}

	switch lastSeg.Kind {
	case SegmentKey:
		return setKeyInParent(parent, lastSeg.Key, valNode)
	case SegmentIndex:
		return setIndexInParent(parent, lastSeg.Index, valNode)
	default:
		return newError(KindBadPath, "unknown segment type")
	}
}

// resolveParentForEdit resolves the parent container for an edit operation.
// When create is true, missing intermediate tables are auto-created.
func (d *Document) resolveParentForEdit(segments []PathSegment, create bool) (layerPos, error) {
	pos, err := d.walkPath(segments)
	if err == nil {
		return pos, nil
	}
	if !create {
		return layerPos{}, wrapError(err, "parent path not found")
	}
	return d.resolveOrCreateParent(segments)
}

// resolveOrCreateParent walks the path segments, creating a standard table for
// every key step that names nothing yet, and returns the final parent position.
func (d *Document) resolveOrCreateParent(segments []PathSegment) (layerPos, error) {
	for i := 1; i <= len(segments); i++ {
		root, err := foldDocument(d)
		if err != nil {
			return layerPos{}, err
		}
		if _, err := walkFrom(posFromRecord(root), segments[:i]); err == nil {
			continue
		} else if segments[i-1].Kind != SegmentKey {
			// Only a key can be created; an array element must already exist.
			return layerPos{}, wrapError(err, "cannot auto-create array index")
		}
		parent, err := walkFrom(posFromRecord(root), segments[:i-1])
		if err != nil {
			return layerPos{}, err
		}
		if err := d.createIntermediateTable(parent, segments[i-1].Key); err != nil {
			return layerPos{}, err
		}
	}

	pos, err := d.walkPath(segments)
	if err != nil {
		return layerPos{}, wrapError(err, "failed to resolve after creation")
	}
	return pos, nil
}

// createIntermediateTable appends a new [table] for the given key under the
// parent position. A parent no single node stands for cannot hold one.
func (d *Document) createIntermediateTable(parent layerPos, key string) error {
	var newPath []string

	switch scope := parent.node.(type) {
	case *Document:
		newPath = []string{key}
	case *TableNode:
		newPath = append(append([]string(nil), scope.KeyPath...), key)
	case *ArrayTableNode:
		newPath = append(append([]string(nil), scope.KeyPath...), key)
	default:
		return newError(KindWrongContainer, "cannot create intermediate table under %s", parent.describe())
	}

	tbl := &TableNode{
		KeyPath: newPath,
	}
	tbl.markDirty()
	tbl.nodeTrivia.TrailingNewline = []byte("\n")

	d.Children = append(d.Children, tbl)
	return nil
}

// setKeyInParent sets or replaces a key-value in a parent container.
func setKeyInParent(parent layerPos, key string, valNode Node) error {
	switch p := parent.node.(type) {
	case *Document:
		return setKeyInChildren(&p.Children, key, valNode, false)
	case *TableNode:
		return setKeyInChildren(&p.Children, key, valNode, false)
	case *ArrayTableNode:
		return setKeyInChildren(&p.Children, key, valNode, false)
	case *InlineTableNode:
		return setKeyInChildren(&p.Children, key, valNode, true)
	default:
		return newError(KindWrongContainer, "cannot set key %q in %s", key, parent.describe())
	}
}

// setKeyInChildren searches children for an existing KV with the given key.
// If found, replaces its value. Otherwise, appends a new KV.
func setKeyInChildren(children *[]Node, key string, valNode Node, markParentDirty bool) error {
	for _, child := range *children {
		if kv, ok := child.(*KeyValueNode); ok {
			if len(kv.Key.Parts) == 1 && kv.Key.Parts[0] == key {
				kv.Val = valNode
				kv.markDirty()
				return nil
			}
		}
	}
	// Key not found: create a new KV and append.
	kv := newKeyValueNode(key, valNode)
	*children = append(*children, kv)
	return nil
}

// setIndexInParent replaces an element at the given index in an array.
func setIndexInParent(parent layerPos, index int, valNode Node) error {
	switch p := parent.node.(type) {
	case *ArrayNode:
		idx, err := normalizeIndex(index, len(p.Elements))
		if err != nil {
			return err
		}
		// Transfer trivia (leading comments, leading whitespace, inline
		// comment) from the old element to the new one so that comments
		// on the replaced element survive re-rendering.
		oldTrivia := p.Elements[idx].trivia()
		newTrivia := valNode.trivia()
		newTrivia.LeadingComments = oldTrivia.LeadingComments
		newTrivia.LeadingWhitespace = oldTrivia.LeadingWhitespace
		newTrivia.InlineComment = oldTrivia.InlineComment
		p.Elements[idx] = valNode
		p.markDirty()
		return nil
	default:
		return newError(KindWrongContainer, "cannot set index [%d] in %s", index, parent.describe())
	}
}

// newKeyValueNode creates a dirty KeyValueNode for the given key and value.
func newKeyValueNode(key string, val Node) *KeyValueNode {
	keyNode := &KeyNode{
		Parts:    []string{key},
		RawParts: [][]byte{[]byte(key)},
		Styles:   []StringStyle{StringBasic},
	}
	keyNode.markDirty()

	kv := &KeyValueNode{
		Key: keyNode,
		Val: val,
	}
	kv.markDirty()
	kv.nodeTrivia.TrailingNewline = []byte("\n")
	return kv
}

// Delete removes the node at the given path from the document. It handles
// key-value pairs, tables, array-of-tables, and array elements. Returns nil
// (no error) if the path does not exist, making it safe to call unconditionally.
func (d *Document) Delete(path string) error {
	return d.diag(d.deleteAt(path), path)
}

func (d *Document) deleteAt(path string) error {
	segments, err := ParsePath(path)
	if err != nil {
		return err
	}
	if len(segments) == 0 {
		return newError(KindBadPath, "empty path")
	}

	parentSegs := segments[:len(segments)-1]
	lastSeg := segments[len(segments)-1]

	// Resolve the parent container.
	parent, err := d.resolveParentForEdit(parentSegs, false)
	if err != nil {
		// Parent doesn't exist -- silent no-op.
		return nil
	}

	switch lastSeg.Kind {
	case SegmentKey:
		return d.deleteKeyFromParent(parent, lastSeg.Key)
	case SegmentIndex:
		return d.deleteIndexFromParent(parent, lastSeg.Index)
	default:
		return newError(KindBadPath, "unknown segment type")
	}
}

// deleteKeyFromParent removes a key from a parent container.
func (d *Document) deleteKeyFromParent(parent layerPos, key string) error {
	switch p := parent.node.(type) {
	case *Document:
		deleteKeyFromChildren(&p.Children, key)
		d.deleteTableOrArrayTableByFirstKey(key)
		return nil
	case *TableNode:
		deleteKeyFromChildren(&p.Children, key)
		d.deleteSubTableByKey(p.KeyPath, key)
		return nil
	case *ArrayTableNode:
		deleteKeyFromChildren(&p.Children, key)
		return nil
	case *InlineTableNode:
		// Only a delete that removed something invalidates the table's bytes:
		// a key the table never carried leaves it exactly as written.
		if deleteKeyFromChildren(&p.Children, key) {
			p.markDirty()
		}
		return nil
	default:
		// Silent no-op for unsupported parent types.
		return nil
	}
}

// deleteKeyFromChildren removes the first KV with the given key from a children
// slice, and reports whether it removed one.
func deleteKeyFromChildren(children *[]Node, key string) bool {
	for i, child := range *children {
		if kv, ok := child.(*KeyValueNode); ok {
			if len(kv.Key.Parts) > 0 && kv.Key.Parts[0] == key {
				*children = append((*children)[:i], (*children)[i+1:]...)
				return true
			}
		}
	}
	return false
}

// deleteTableOrArrayTableByFirstKey removes a table or array-table from the
// document's top-level children by matching the key path.
func (d *Document) deleteTableOrArrayTableByFirstKey(key string) {
	targetPath := []string{key}
	i := 0
	for i < len(d.Children) {
		child := d.Children[i]
		switch n := child.(type) {
		case *TableNode:
			if len(n.KeyPath) > 0 && n.KeyPath[0] == key {
				d.Children = append(d.Children[:i], d.Children[i+1:]...)
				continue
			}
		case *ArrayTableNode:
			if pathsEqual(n.KeyPath, targetPath) {
				d.Children = append(d.Children[:i], d.Children[i+1:]...)
				continue
			}
		}
		i++
	}
}

// deleteSubTableByKey removes a sub-table from the document's children.
func (d *Document) deleteSubTableByKey(parentPath []string, key string) {
	targetPath := append(append([]string(nil), parentPath...), key)
	i := 0
	for i < len(d.Children) {
		child := d.Children[i]
		switch n := child.(type) {
		case *TableNode:
			if pathsEqual(n.KeyPath, targetPath) {
				d.Children = append(d.Children[:i], d.Children[i+1:]...)
				continue
			}
		case *ArrayTableNode:
			if pathsEqual(n.KeyPath, targetPath) {
				d.Children = append(d.Children[:i], d.Children[i+1:]...)
				continue
			}
		}
		i++
	}
}

// deleteIndexFromParent removes an element at the given index.
func (d *Document) deleteIndexFromParent(parent layerPos, index int) error {
	if parent.records != nil {
		if len(parent.records) == 0 {
			return nil
		}
		idx, err := normalizeIndex(index, len(parent.records))
		if err != nil {
			return nil // silent no-op for out-of-range
		}
		// Remove the array-of-tables entry from the document's children.
		target := parent.records[idx].node
		for i, child := range d.Children {
			if child == target {
				d.Children = append(d.Children[:i], d.Children[i+1:]...)
				break
			}
		}
		return nil
	}
	switch p := parent.node.(type) {
	case *ArrayNode:
		if len(p.Elements) == 0 {
			return nil // silent no-op
		}
		idx, err := normalizeIndex(index, len(p.Elements))
		if err != nil {
			return nil // silent no-op for out-of-range
		}
		p.Elements = append(p.Elements[:idx], p.Elements[idx+1:]...)
		p.markDirty()
		return nil
	default:
		return nil // silent no-op
	}
}

// RenameKey changes the key name of the node at the given path to newKey.
// Returns an error if the path does not exist, if newKey conflicts with an
// existing sibling key, or if the last path segment is an array index (only
// key segments can be renamed).
func (d *Document) RenameKey(path string, newKey string) error {
	return d.diag(d.renameKeyAt(path, newKey), path)
}

func (d *Document) renameKeyAt(path string, newKey string) error {
	segments, err := ParsePath(path)
	if err != nil {
		return err
	}
	if len(segments) == 0 {
		return newError(KindBadPath, "empty path")
	}

	// All segments must be key segments for rename to make sense.
	lastSeg := segments[len(segments)-1]
	if lastSeg.Kind != SegmentKey {
		return newError(KindWrongContainer, "cannot rename an array index")
	}

	parentSegs := segments[:len(segments)-1]

	parent, err := d.resolveParentForEdit(parentSegs, false)
	if err != nil {
		return wrapError(err, "path not found")
	}

	return renameKeyInParent(parent, lastSeg.Key, newKey)
}

// renameKeyInParent renames a key inside a parent container.
func renameKeyInParent(parent layerPos, oldKey, newKey string) error {
	var children *[]Node

	switch p := parent.node.(type) {
	case *Document:
		children = &p.Children
	case *TableNode:
		children = &p.Children
	case *ArrayTableNode:
		children = &p.Children
	case *InlineTableNode:
		children = &p.Children
	default:
		return newError(KindWrongContainer, "cannot rename key in %s", parent.describe())
	}

	// Check for duplicate: does newKey already exist?
	for _, child := range *children {
		if kv, ok := child.(*KeyValueNode); ok {
			if len(kv.Key.Parts) == 1 && kv.Key.Parts[0] == newKey {
				return newError(KindConflict, "key %q already exists in parent", newKey)
			}
		}
	}

	// Find the KV with the old key.
	for _, child := range *children {
		if kv, ok := child.(*KeyValueNode); ok {
			if len(kv.Key.Parts) > 0 && kv.Key.Parts[0] == oldKey {
				// Update the last matching part (for simple keys, index 0).
				kv.Key.Parts[0] = newKey
				kv.Key.RawParts[0] = []byte(newKey)
				kv.Key.markDirty()
				kv.markDirty()
				return nil
			}
		}
	}

	return newError(KindNotFound, "key %q not found", oldKey)
}

// NewTable creates a new [table] header at the given path and appends it to
// the document. The path must consist of key segments only (no array indices).
//
// The header must be able to bind its name. It is refused with KindConflict
// when anything else in the document already does: a value, an inline table, a
// table with its own header, an array-of-tables, or a table a dotted key
// implied (TOML does not allow extending one of those with a header). A table
// implied only by a LONGER header -- the "a" of an earlier [a.b] -- has no
// header of its own yet, and this is how it gets one. A prefix of the path
// that holds a value is refused on the same terms.
func (d *Document) NewTable(path string) error {
	return d.diag(d.newTableAt(path), path)
}

func (d *Document) newTableAt(path string) error {
	keyPath, err := headerKeyPath(path, "NewTable")
	if err != nil {
		return err
	}

	existing, bound, err := d.headerTarget(keyPath)
	if err != nil {
		return err
	}
	if bound {
		name := pathFromKeys(keyPath)
		switch {
		case existing.kind == EntryRecords:
			return newError(KindConflict,
				"%q is an array of tables: a [%s] header cannot bind the same name", name, name)
		case existing.kind != EntryRecord:
			return newError(KindConflict, "%q already holds a value", name)
		case existing.record.dottedImplied:
			return newError(KindConflict,
				"%q is a table a dotted key spelled out, and TOML does not allow giving one a header", name)
		case existing.record.anchored:
			if _, inline := existing.node.(*InlineTableNode); inline {
				return newError(KindConflict,
					"%q is an inline table: a [%s] header cannot bind the same name", name, name)
			}
			return newError(KindConflict, "table [%s] already exists", name)
		}
		// A table implied by a longer header: this header anchors it.
	}

	tbl := &TableNode{
		KeyPath: keyPath,
	}
	tbl.markDirty()
	tbl.nodeTrivia.TrailingNewline = []byte("\n")

	d.Children = append(d.Children, tbl)
	return nil
}

// NewArrayTable appends a new [[array-table]] entry at the given path.
// Multiple entries with the same path are valid in TOML and represent
// successive elements of the array. The path must consist of key segments
// only (no array indices).
//
// A name already bound to an array-of-tables is exactly what a new entry
// extends. Any other binding of the name -- a value, an inline table, a table
// with a header of its own or one another construct implied -- is refused with
// KindConflict, as is a prefix of the path that holds a value.
func (d *Document) NewArrayTable(path string) error {
	return d.diag(d.newArrayTableAt(path), path)
}

func (d *Document) newArrayTableAt(path string) error {
	keyPath, err := headerKeyPath(path, "NewArrayTable")
	if err != nil {
		return err
	}

	existing, bound, err := d.headerTarget(keyPath)
	if err != nil {
		return err
	}
	if bound && existing.kind != EntryRecords {
		name := pathFromKeys(keyPath)
		if existing.kind == EntryRecord {
			return newError(KindConflict,
				"%q is a table: a [[%s]] header cannot bind the same name", name, name)
		}
		return newError(KindConflict, "%q already holds a value", name)
	}

	atbl := &ArrayTableNode{
		KeyPath: keyPath,
	}
	atbl.markDirty()
	atbl.nodeTrivia.TrailingNewline = []byte("\n")

	d.Children = append(d.Children, atbl)
	return nil
}

// headerKeyPath parses a header path into its key parts, refusing an empty
// path and any index segment. op names the operation in the diagnostic.
func headerKeyPath(path string, op string) ([]string, error) {
	segments, err := ParsePath(path)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		return nil, newError(KindBadPath, "empty path")
	}
	keyPath := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg.Kind != SegmentKey {
			return nil, newError(KindBadPath, "%s path must contain only key segments, not indices", op)
		}
		keyPath = append(keyPath, seg.Key)
	}
	return keyPath, nil
}

// headerTarget reports what a header's key path names in the document as it
// stands: the entry its final key already binds, and whether one does. A
// prefix that cannot hold a table -- one holding a value, or one a dotted key
// spelled out -- is a conflict, and so is a document that no longer folds.
func (d *Document) headerTarget(keyPath []string) (Entry, bool, error) {
	root, err := foldDocument(d)
	if err != nil {
		return Entry{}, false, err
	}
	cur := root
	for i, part := range keyPath[:len(keyPath)-1] {
		e, ok := cur.Get(part)
		if !ok {
			// Nothing binds this prefix, so nothing binds the final key
			// either: the header creates the whole chain.
			return Entry{}, false, nil
		}
		switch {
		case e.kind == EntryRecords:
			cur = e.records[len(e.records)-1]
		case e.kind != EntryRecord:
			return Entry{}, false, newError(KindConflict,
				"%q holds a value and cannot hold a table", pathFromKeys(keyPath[:i+1]))
		case e.record.dottedImplied:
			return Entry{}, false, newError(KindConflict,
				"%q is a table a dotted key spelled out, and TOML does not allow extending one with a header",
				pathFromKeys(keyPath[:i+1]))
		default:
			cur = e.record
		}
	}
	e, ok := cur.Get(keyPath[len(keyPath)-1])
	return e, ok, nil
}

// valueToNode converts a Go value to the appropriate AST node.
// All created nodes are dirty (no raw bytes).
func valueToNode(v any) (Node, error) {
	if v == nil {
		return nil, newError(KindBadInput, "unsupported type: nil")
	}

	// Check if it already implements Node.
	if n, ok := v.(Node); ok {
		return n, nil
	}

	switch val := v.(type) {
	case string:
		n := &StringNode{Val: val, Style: StringBasic}
		n.markDirty()
		return n, nil

	case bool:
		n := &BooleanNode{Val: val}
		n.markDirty()
		return n, nil

	case int:
		n := &IntegerNode{Val: int64(val), Base: IntegerDecimal}
		n.markDirty()
		return n, nil
	case int8:
		n := &IntegerNode{Val: int64(val), Base: IntegerDecimal}
		n.markDirty()
		return n, nil
	case int16:
		n := &IntegerNode{Val: int64(val), Base: IntegerDecimal}
		n.markDirty()
		return n, nil
	case int32:
		n := &IntegerNode{Val: int64(val), Base: IntegerDecimal}
		n.markDirty()
		return n, nil
	case int64:
		n := &IntegerNode{Val: val, Base: IntegerDecimal}
		n.markDirty()
		return n, nil

	case uint:
		return unsignedToNode(uint64(val), val)
	case uint8:
		n := &IntegerNode{Val: int64(val), Base: IntegerDecimal}
		n.markDirty()
		return n, nil
	case uint16:
		n := &IntegerNode{Val: int64(val), Base: IntegerDecimal}
		n.markDirty()
		return n, nil
	case uint32:
		n := &IntegerNode{Val: int64(val), Base: IntegerDecimal}
		n.markDirty()
		return n, nil
	case uint64:
		return unsignedToNode(val, val)

	case float32:
		n := &FloatNode{Val: float64(val)}
		n.markDirty()
		return n, nil
	case float64:
		n := &FloatNode{Val: val}
		n.markDirty()
		return n, nil

	case time.Time:
		n := &DateTimeNode{Val: val}
		n.markDirty()
		return n, nil

	case LocalDateTime:
		n := &LocalDateTimeNode{Val: val}
		n.markDirty()
		return n, nil

	case LocalDate:
		n := &LocalDateNode{Val: val}
		n.markDirty()
		return n, nil

	case LocalTime:
		n := &LocalTimeNode{Val: val}
		n.markDirty()
		return n, nil

	case []any:
		return sliceToArrayNode(val)

	case map[string]any:
		return mapToInlineTableNode(val)
	}

	// Use reflection for typed slices (e.g., []string, []int).
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		items := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			items[i] = rv.Index(i).Interface()
		}
		return sliceToArrayNode(items)
	}

	return nil, newError(KindBadInput, "unsupported type: %T", v).withValue(v)
}

// unsignedToNode converts an unsigned Go value to an integer node, refusing one
// that does not fit in the int64 a TOML integer is. Wrapping it around would
// write a negative number the caller never asked for. orig is the value as the
// caller passed it, so the diagnostic reports its own type.
func unsignedToNode(v uint64, orig any) (Node, error) {
	if v > math.MaxInt64 {
		return nil, newError(KindBadInput,
			"unsigned value %d does not fit in a TOML integer (the maximum is %d)",
			v, int64(math.MaxInt64)).withValue(orig)
	}
	n := &IntegerNode{Val: int64(v), Base: IntegerDecimal}
	n.markDirty()
	return n, nil
}

// sliceToArrayNode converts a []any to an ArrayNode with recursive conversion.
func sliceToArrayNode(items []any) (Node, error) {
	arr := &ArrayNode{}
	arr.markDirty()
	for _, item := range items {
		elem, err := valueToNode(item)
		if err != nil {
			return nil, wrapError(err, "array element")
		}
		arr.Elements = append(arr.Elements, elem)
	}
	return arr, nil
}

// mapToInlineTableNode converts a map[string]any to an InlineTableNode.
// Keys are sorted alphabetically for deterministic output.
func mapToInlineTableNode(m map[string]any) (Node, error) {
	tbl := &InlineTableNode{}
	tbl.markDirty()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		valNode, err := valueToNode(m[k])
		if err != nil {
			return nil, wrapError(err, "inline table key %q", k)
		}
		kv := newKeyValueNode(k, valNode)
		tbl.Children = append(tbl.Children, kv)
	}
	return tbl, nil
}
