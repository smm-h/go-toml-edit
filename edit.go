package tomledit

import (
	"fmt"
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
	return d.setInternal(path, value, false)
}

// SetCreate is like Set but auto-creates intermediate [table] headers when they
// do not exist. Missing tables are appended to the document. This is convenient
// for inserting values into deeply nested paths that may not yet exist.
func (d *Document) SetCreate(path string, value any) error {
	return d.setInternal(path, value, true)
}

func (d *Document) setInternal(path string, value any, create bool) error {
	segments, err := parsePath(path)
	if err != nil {
		return fmt.Errorf("path syntax error: %w", err)
	}
	if len(segments) == 0 {
		return fmt.Errorf("empty path")
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

	switch lastSeg.Type {
	case keySegment:
		return setKeyInParent(parent, lastSeg.Key, valNode)
	case indexSegment:
		return setIndexInParent(parent, lastSeg.Index, valNode)
	default:
		return fmt.Errorf("unknown segment type")
	}
}

// resolveParentForEdit resolves the parent container for an edit operation.
// When create is true, missing intermediate tables are auto-created.
func (d *Document) resolveParentForEdit(segments []pathSegment, create bool) (Node, error) {
	if len(segments) == 0 {
		return d, nil
	}

	// Try resolving the full parent path first.
	node, err := resolveNodeForEdit(d, segments)
	if err == nil {
		return node, nil
	}

	if !create {
		return nil, fmt.Errorf("parent path not found: %w", err)
	}

	// Auto-create mode: walk segments, creating tables as needed.
	return d.resolveOrCreateParent(segments)
}

// resolveOrCreateParent walks the path segments, creating intermediate tables
// as needed. Returns the final parent container.
func (d *Document) resolveOrCreateParent(segments []pathSegment) (Node, error) {
	var current Node = d
	var currentTablePath []string

	for _, seg := range segments {
		if seg.Type != keySegment {
			// For index segments, the collection must already exist.
			next, err := resolveIndexSegment(d, current, currentTablePath, seg.Index)
			if err != nil {
				return nil, fmt.Errorf("cannot auto-create array index: %w", err)
			}
			current = next
			continue
		}

		// Try to resolve this key segment.
		next, tablePath, err := resolveKeySegment(d, current, currentTablePath, seg.Key, nil, 0)
		if err == nil {
			current = next
			currentTablePath = tablePath
			continue
		}

		// Key not found -- create a table for it. The path it returns is
		// discarded: the re-resolution below recomputes it.
		if _, err := d.createIntermediateTable(current, currentTablePath, seg.Key); err != nil {
			return nil, err
		}

		// Re-resolve to get the newly created table.
		next, tablePath, err = resolveKeySegment(d, current, currentTablePath, seg.Key, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve after creation: %w", err)
		}
		current = next
		currentTablePath = tablePath
	}

	return current, nil
}

// createIntermediateTable creates a new [table] for the given key under the
// current scope. Returns the updated table path.
func (d *Document) createIntermediateTable(current Node, currentTablePath []string, key string) ([]string, error) {
	var newPath []string

	switch scope := current.(type) {
	case *Document:
		newPath = []string{key}
	case *TableNode:
		newPath = append(append([]string(nil), scope.KeyPath...), key)
	case *ArrayTableNode:
		newPath = append(append([]string(nil), scope.KeyPath...), key)
	default:
		return nil, fmt.Errorf("cannot create intermediate table under %s node", current.Type())
	}

	tbl := &TableNode{
		KeyPath: newPath,
	}
	tbl.markDirty()
	tbl.nodeTrivia.TrailingNewline = []byte("\n")

	d.Children = append(d.Children, tbl)
	return newPath, nil
}

// resolveNodeForEdit is like resolveNode but returns container nodes (tables,
// documents, inline tables) without unwrapping KeyValueNodes whose value is an
// inline table or array. This allows the edit operations to find the right
// parent for insertion.
func resolveNodeForEdit(doc *Document, segments []pathSegment) (Node, error) {
	if len(segments) == 0 {
		return doc, nil
	}

	var current Node = doc
	var currentTablePath []string

	for i, seg := range segments {
		switch seg.Type {
		case keySegment:
			node, tablePath, err := resolveKeySegment(doc, current, currentTablePath, seg.Key, segments, i)
			if err != nil {
				return nil, err
			}
			current = node
			currentTablePath = tablePath

		case indexSegment:
			node, err := resolveIndexSegment(doc, current, currentTablePath, seg.Index)
			if err != nil {
				return nil, err
			}
			current = node

		default:
			return nil, fmt.Errorf("unknown segment type")
		}
	}

	return current, nil
}

// setKeyInParent sets or replaces a key-value in a parent container.
func setKeyInParent(parent Node, key string, valNode Node) error {
	switch p := parent.(type) {
	case *Document:
		return setKeyInChildren(&p.Children, key, valNode, false)
	case *TableNode:
		return setKeyInChildren(&p.Children, key, valNode, false)
	case *ArrayTableNode:
		return setKeyInChildren(&p.Children, key, valNode, false)
	case *InlineTableNode:
		return setKeyInChildren(&p.Children, key, valNode, true)
	case *dottedKeyView:
		// The dotted key view points into a KV's value. If the value is an
		// inline table, set inside it.
		if p.partIndex >= len(p.kv.Key.Parts) {
			return setKeyInParent(p.kv.Val, key, valNode)
		}
		return fmt.Errorf("cannot set key %q: intermediate dotted key view", key)
	default:
		return fmt.Errorf("cannot set key %q in %s node", key, parent.Type())
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
func setIndexInParent(parent Node, index int, valNode Node) error {
	switch p := parent.(type) {
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
	case *KeyValueNode:
		return setIndexInParent(p.Val, index, valNode)
	default:
		return fmt.Errorf("cannot set index [%d] in %s node", index, parent.Type())
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
	segments, err := parsePath(path)
	if err != nil {
		return fmt.Errorf("path syntax error: %w", err)
	}
	if len(segments) == 0 {
		return fmt.Errorf("empty path")
	}

	parentSegs := segments[:len(segments)-1]
	lastSeg := segments[len(segments)-1]

	// Resolve the parent container.
	parent, err := d.resolveParentForEdit(parentSegs, false)
	if err != nil {
		// Parent doesn't exist -- silent no-op.
		return nil
	}

	switch lastSeg.Type {
	case keySegment:
		return d.deleteKeyFromParent(parent, lastSeg.Key)
	case indexSegment:
		return deleteIndexFromParent(parent, lastSeg.Index)
	default:
		return fmt.Errorf("unknown segment type")
	}
}

// deleteKeyFromParent removes a key from a parent container.
func (d *Document) deleteKeyFromParent(parent Node, key string) error {
	switch p := parent.(type) {
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
		deleteKeyFromChildren(&p.Children, key)
		p.markDirty()
		return nil
	default:
		// Silent no-op for unsupported parent types.
		return nil
	}
}

// deleteKeyFromChildren removes the first KV with the given key from a children slice.
func deleteKeyFromChildren(children *[]Node, key string) {
	for i, child := range *children {
		if kv, ok := child.(*KeyValueNode); ok {
			if len(kv.Key.Parts) > 0 && kv.Key.Parts[0] == key {
				*children = append((*children)[:i], (*children)[i+1:]...)
				return
			}
		}
	}
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
func deleteIndexFromParent(parent Node, index int) error {
	switch p := parent.(type) {
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
	case *arrayTableCollection:
		if len(p.entries) == 0 {
			return nil
		}
		idx, err := normalizeIndex(index, len(p.entries))
		if err != nil {
			return nil
		}
		// Remove the ArrayTableNode from the document's children.
		target := p.entries[idx]
		for i, child := range p.doc.Children {
			if child == target {
				p.doc.Children = append(p.doc.Children[:i], p.doc.Children[i+1:]...)
				break
			}
		}
		return nil
	case *KeyValueNode:
		return deleteIndexFromParent(p.Val, index)
	default:
		return nil // silent no-op
	}
}

// RenameKey changes the key name of the node at the given path to newKey.
// Returns an error if the path does not exist, if newKey conflicts with an
// existing sibling key, or if the last path segment is an array index (only
// key segments can be renamed).
func (d *Document) RenameKey(path string, newKey string) error {
	segments, err := parsePath(path)
	if err != nil {
		return fmt.Errorf("path syntax error: %w", err)
	}
	if len(segments) == 0 {
		return fmt.Errorf("empty path")
	}

	// All segments must be key segments for rename to make sense.
	lastSeg := segments[len(segments)-1]
	if lastSeg.Type != keySegment {
		return fmt.Errorf("cannot rename an array index")
	}

	parentSegs := segments[:len(segments)-1]

	parent, err := d.resolveParentForEdit(parentSegs, false)
	if err != nil {
		return fmt.Errorf("path not found: %w", err)
	}

	return renameKeyInParent(parent, lastSeg.Key, newKey)
}

// renameKeyInParent renames a key inside a parent container.
func renameKeyInParent(parent Node, oldKey, newKey string) error {
	var children *[]Node

	switch p := parent.(type) {
	case *Document:
		children = &p.Children
	case *TableNode:
		children = &p.Children
	case *ArrayTableNode:
		children = &p.Children
	case *InlineTableNode:
		children = &p.Children
	default:
		return fmt.Errorf("cannot rename key in %s node", parent.Type())
	}

	// Check for duplicate: does newKey already exist?
	for _, child := range *children {
		if kv, ok := child.(*KeyValueNode); ok {
			if len(kv.Key.Parts) == 1 && kv.Key.Parts[0] == newKey {
				return fmt.Errorf("key %q already exists in parent", newKey)
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

	return fmt.Errorf("key %q not found", oldKey)
}

// NewTable creates a new [table] header at the given path and appends it to
// the document. The path must consist of key segments only (no array indices).
// Returns an error if a table with that exact path already exists.
func (d *Document) NewTable(path string) error {
	segments, err := parsePath(path)
	if err != nil {
		return fmt.Errorf("path syntax error: %w", err)
	}
	if len(segments) == 0 {
		return fmt.Errorf("empty path")
	}

	// Build the key path from segments.
	keyPath := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg.Type != keySegment {
			return fmt.Errorf("NewTable path must contain only key segments, not indices")
		}
		keyPath = append(keyPath, seg.Key)
	}

	// Check if a table with this path already exists.
	for _, child := range d.Children {
		if tbl, ok := child.(*TableNode); ok {
			if pathsEqual(tbl.KeyPath, keyPath) {
				return fmt.Errorf("table [%s] already exists", joinPath(keyPath))
			}
		}
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
func (d *Document) NewArrayTable(path string) error {
	segments, err := parsePath(path)
	if err != nil {
		return fmt.Errorf("path syntax error: %w", err)
	}
	if len(segments) == 0 {
		return fmt.Errorf("empty path")
	}

	keyPath := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg.Type != keySegment {
			return fmt.Errorf("NewArrayTable path must contain only key segments, not indices")
		}
		keyPath = append(keyPath, seg.Key)
	}

	atbl := &ArrayTableNode{
		KeyPath: keyPath,
	}
	atbl.markDirty()
	atbl.nodeTrivia.TrailingNewline = []byte("\n")

	d.Children = append(d.Children, atbl)
	return nil
}

// valueToNode converts a Go value to the appropriate AST node.
// All created nodes are dirty (no raw bytes).
func valueToNode(v any) (Node, error) {
	if v == nil {
		return nil, fmt.Errorf("unsupported type: nil")
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
		n := &IntegerNode{Val: int64(val), Base: IntegerDecimal}
		n.markDirty()
		return n, nil
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
		n := &IntegerNode{Val: int64(val), Base: IntegerDecimal}
		n.markDirty()
		return n, nil

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

	return nil, fmt.Errorf("unsupported type: %T", v)
}

// sliceToArrayNode converts a []any to an ArrayNode with recursive conversion.
func sliceToArrayNode(items []any) (Node, error) {
	arr := &ArrayNode{}
	arr.markDirty()
	for _, item := range items {
		elem, err := valueToNode(item)
		if err != nil {
			return nil, fmt.Errorf("array element: %w", err)
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
			return nil, fmt.Errorf("inline table key %q: %w", k, err)
		}
		kv := newKeyValueNode(k, valNode)
		tbl.Children = append(tbl.Children, kv)
	}
	return tbl, nil
}
