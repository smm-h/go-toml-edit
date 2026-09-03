package tomledit

import "time"

// resolveKeySegment handles a key lookup within the current scope.
// It returns the resolved node, the updated table path, and any error.
func resolveKeySegment(
	doc *Document,
	current Node,
	currentTablePath []string,
	key string,
	segments []PathSegment,
	segIndex int,
) (Node, []string, error) {
	switch scope := current.(type) {
	case *Document:
		return resolveKeyInDocument(doc, scope, nil, key)

	case *TableNode:
		return resolveKeyInTable(doc, scope, currentTablePath, key)

	case *ArrayTableNode:
		return resolveKeyInArrayTable(doc, scope, currentTablePath, key)

	case *KeyValueNode:
		// Unwrap to value and re-resolve
		return resolveKeySegment(doc, scope.Val, currentTablePath, key, segments, segIndex)

	case *InlineTableNode:
		return resolveKeyInInlineTable(scope, key)

	case *dottedKeyView:
		return resolveKeyInDottedView(doc, scope, key)

	case *dottedKeyGroup:
		return resolveKeyInDottedGroup(doc, scope, key)

	case *compoundTableView:
		return resolveKeyInCompoundView(doc, scope, key)

	case *arrayTableCollection:
		// Cannot look up a key directly on a collection; must index first
		return nil, nil, newError(KindWrongContainer, "cannot look up key %q on array-of-tables (use an index first)", key)

	default:
		return nil, nil, newError(KindWrongContainer, "cannot look up key %q in %s node", key, current.Type())
	}
}

// resolveKeyInDocument searches the document's top-level children for a key.
func resolveKeyInDocument(doc *Document, scope *Document, tablePath []string, key string) (Node, []string, error) {
	// Check top-level KVs -- collect all matching dotted keys with this prefix
	node, tablePath2, err := resolveKeyInKVList(doc, scope.Children, tablePath, key)
	if err == nil {
		return node, tablePath2, nil
	}

	// Check for a TableNode or ArrayTableNode with matching path component
	targetPath := append(append([]string(nil), tablePath...), key)

	// Collect array-of-tables -- exact match
	var arrayEntries []*ArrayTableNode
	for _, child := range doc.Children {
		if at, ok := child.(*ArrayTableNode); ok {
			if pathsEqual(at.KeyPath, targetPath) {
				arrayEntries = append(arrayEntries, at)
			}
		}
	}
	if len(arrayEntries) > 0 {
		return &arrayTableCollection{entries: arrayEntries, doc: doc}, targetPath, nil
	}

	// Check for regular table -- exact match
	for _, child := range doc.Children {
		if tbl, ok := child.(*TableNode); ok {
			if pathsEqual(tbl.KeyPath, targetPath) {
				return tbl, targetPath, nil
			}
		}
	}

	// Bug 1 fix: Check for compound table paths where targetPath is a PREFIX
	// of a TableNode's KeyPath (implicit intermediate table).
	view := buildCompoundView(doc, nil, targetPath)
	if view != nil {
		return view, targetPath, nil
	}

	return nil, nil, newError(KindNotFound, "key %q not found", key)
}

// resolveKeyInTable searches a table's children and sub-tables for a key.
func resolveKeyInTable(doc *Document, scope *TableNode, currentTablePath []string, key string) (Node, []string, error) {
	// Check direct children (KVs) -- collect all matching dotted keys
	node, tablePath, err := resolveKeyInKVList(doc, scope.Children, currentTablePath, key)
	if err == nil {
		return node, tablePath, nil
	}

	// Check for sub-tables in the document
	targetPath := append(append([]string(nil), scope.KeyPath...), key)

	// Collect array-of-tables
	var arrayEntries []*ArrayTableNode
	for _, child := range doc.Children {
		if at, ok := child.(*ArrayTableNode); ok {
			if pathsEqual(at.KeyPath, targetPath) {
				arrayEntries = append(arrayEntries, at)
			}
		}
	}
	if len(arrayEntries) > 0 {
		return &arrayTableCollection{entries: arrayEntries, doc: doc}, targetPath, nil
	}

	// Check for regular sub-table
	for _, child := range doc.Children {
		if tbl, ok := child.(*TableNode); ok {
			if pathsEqual(tbl.KeyPath, targetPath) {
				return tbl, targetPath, nil
			}
		}
	}

	// Bug 1 fix: Check for compound table paths where targetPath is a PREFIX
	// of a TableNode's KeyPath (implicit intermediate table).
	view := buildCompoundView(doc, nil, targetPath)
	if view != nil {
		return view, targetPath, nil
	}

	return nil, nil, newError(KindNotFound, "key %q not found in table [%s]", key, pathFromKeys(scope.KeyPath))
}

// resolveKeyInArrayTable searches an array table's children for a key.
// Sub-tables are scoped to the specific array entry: only TableNode/ArrayTableNode
// children that appear after this ArrayTableNode in doc.Children, and before the
// next ArrayTableNode with the same key path, belong to this entry.
func resolveKeyInArrayTable(doc *Document, scope *ArrayTableNode, currentTablePath []string, key string) (Node, []string, error) {
	// Check direct children (KVs) -- collect all matching dotted keys
	node, tablePath, err := resolveKeyInKVList(doc, scope.Children, currentTablePath, key)
	if err == nil {
		return node, tablePath, nil
	}

	// Find the position of this specific ArrayTableNode in doc.Children.
	scopeIdx := -1
	for i, child := range doc.Children {
		if child == scope {
			scopeIdx = i
			break
		}
	}
	if scopeIdx == -1 {
		return nil, nil, newError(KindNotFound, "key %q not found in array table [[%s]]", key, pathFromKeys(scope.KeyPath))
	}

	// Check for sub-tables scoped to this array entry: scan forward from
	// scopeIdx+1, stopping at the next ArrayTableNode with the same key path.
	targetPath := append(append([]string(nil), scope.KeyPath...), key)

	// Collect array-of-tables entries scoped to this array entry
	var arrayEntries []*ArrayTableNode
	for i := scopeIdx + 1; i < len(doc.Children); i++ {
		child := doc.Children[i]
		// Stop at the next entry of the same array-of-tables
		if at, ok := child.(*ArrayTableNode); ok {
			if pathsEqual(at.KeyPath, scope.KeyPath) {
				break
			}
			if pathsEqual(at.KeyPath, targetPath) {
				arrayEntries = append(arrayEntries, at)
			}
		}
	}
	if len(arrayEntries) > 0 {
		return &arrayTableCollection{entries: arrayEntries, doc: doc}, targetPath, nil
	}

	// Check for regular sub-table scoped to this array entry
	for i := scopeIdx + 1; i < len(doc.Children); i++ {
		child := doc.Children[i]
		// Stop at the next entry of the same array-of-tables
		if at, ok := child.(*ArrayTableNode); ok {
			if pathsEqual(at.KeyPath, scope.KeyPath) {
				break
			}
		}
		if tbl, ok := child.(*TableNode); ok {
			if pathsEqual(tbl.KeyPath, targetPath) {
				return tbl, targetPath, nil
			}
		}
	}

	// Bug 1 fix: Check for compound sub-tables scoped to this array entry
	var compoundTables []*TableNode
	var compoundArrayTbls []*ArrayTableNode
	for i := scopeIdx + 1; i < len(doc.Children); i++ {
		child := doc.Children[i]
		if at, ok := child.(*ArrayTableNode); ok {
			if pathsEqual(at.KeyPath, scope.KeyPath) {
				break
			}
			if hasPrefix(at.KeyPath, targetPath) && len(at.KeyPath) > len(targetPath) {
				compoundArrayTbls = append(compoundArrayTbls, at)
			}
		}
		if tbl, ok := child.(*TableNode); ok {
			if hasPrefix(tbl.KeyPath, targetPath) && len(tbl.KeyPath) > len(targetPath) {
				compoundTables = append(compoundTables, tbl)
			}
		}
	}
	if len(compoundTables) > 0 || len(compoundArrayTbls) > 0 {
		return &compoundTableView{
			doc:       doc,
			prefix:    targetPath,
			tables:    compoundTables,
			arrayTbls: compoundArrayTbls,
		}, targetPath, nil
	}

	return nil, nil, newError(KindNotFound, "key %q not found in array table [[%s]]", key, pathFromKeys(scope.KeyPath))
}

// resolveKeyInInlineTable searches an inline table's children for a key.
func resolveKeyInInlineTable(scope *InlineTableNode, key string) (Node, []string, error) {
	node, tablePath, err := resolveKeyInKVList(nil, scope.Children, nil, key)
	if err == nil {
		return node, tablePath, nil
	}
	return nil, nil, newError(KindNotFound, "key %q not found in inline table", key)
}

// resolveIndexSegment handles an index lookup into an array or array-of-tables.
func resolveIndexSegment(doc *Document, current Node, currentTablePath []string, index int) (Node, error) {
	switch scope := current.(type) {
	case *ArrayNode:
		idx, err := normalizeIndex(index, len(scope.Elements))
		if err != nil {
			return nil, err
		}
		return scope.Elements[idx], nil

	case *arrayTableCollection:
		idx, err := normalizeIndex(index, len(scope.entries))
		if err != nil {
			return nil, err
		}
		return scope.entries[idx], nil

	case *KeyValueNode:
		return resolveIndexSegment(doc, scope.Val, currentTablePath, index)

	default:
		return nil, newError(KindWrongContainer, "cannot index into %s node", current.Type())
	}
}

// normalizeIndex converts a possibly-negative index to a valid non-negative index.
func normalizeIndex(index, length int) (int, error) {
	if length == 0 {
		return 0, newError(KindNotFound, "index %d out of range (empty collection)", index)
	}
	idx := index
	if idx < 0 {
		idx = length + idx
	}
	if idx < 0 || idx >= length {
		return 0, newError(KindNotFound, "index %d out of range (length %d)", index, length)
	}
	return idx, nil
}

// unwrapKV returns the value node from a KeyValueNode.
func unwrapKV(kv *KeyValueNode) Node {
	return kv.Val
}

// dottedKeyView is an internal node representing intermediate access into a
// dotted key like a.b.c = val. When someone looks up "a", they get a view
// pointing at part index 1; looking up "b" advances to index 2, etc.
type dottedKeyView struct {
	nullNode
	kv        *KeyValueNode
	partIndex int
	doc       *Document
}

func (d *dottedKeyView) Type() NodeType { return NodeKeyValue }
func (d *dottedKeyView) Value() any     { return d.kv.Val }

// dottedKeyGroup groups multiple KeyValueNodes that share a common dotted-key
// prefix. For example, "database.host" and "database.port" both share the
// prefix "database" at depth 0. This acts as a virtual table for resolution.
type dottedKeyGroup struct {
	nullNode
	kvs   []*KeyValueNode
	depth int // how many leading parts have been consumed
	doc   *Document
}

func (g *dottedKeyGroup) Type() NodeType { return NodeTable }
func (g *dottedKeyGroup) Value() any     { return g.kvs }

// compoundTableView is a virtual node representing an implicit intermediate
// table created by a compound KeyPath like [a.b.c]. When resolving "a", we
// create this view; resolving "b" in it will match the remaining KeyPath.
type compoundTableView struct {
	nullNode
	doc    *Document
	prefix []string // the path segments consumed so far
	// The tables/array-tables whose KeyPaths start with this prefix
	tables    []*TableNode
	arrayTbls []*ArrayTableNode
}

func (c *compoundTableView) Type() NodeType { return NodeTable }
func (c *compoundTableView) Value() any     { return nil }

// arrayTableCollection groups all ArrayTableNodes with the same KeyPath
// so they can be indexed. It is not a real AST node.
type arrayTableCollection struct {
	nullNode
	entries []*ArrayTableNode
	doc     *Document
}

func (a *arrayTableCollection) Type() NodeType { return NodeArrayTable }
func (a *arrayTableCollection) Value() any     { return a.entries }

// pathsEqual returns true if two string slices are equal.
func pathsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// hasPrefix returns true if path starts with prefix.
func hasPrefix(path, prefix []string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

// resolveKeyInKVList handles key lookups within a list of children (KVs).
// It collects all KVs whose first part matches key. If there's a single
// exact match (Parts == [key]), it returns the value. If there are multiple
// dotted keys sharing the prefix, it returns a dottedKeyGroup.
func resolveKeyInKVList(doc *Document, children []Node, tablePath []string, key string) (Node, []string, error) {
	var matching []*KeyValueNode
	for _, child := range children {
		if kv, ok := child.(*KeyValueNode); ok {
			if len(kv.Key.Parts) > 0 && kv.Key.Parts[0] == key {
				matching = append(matching, kv)
			}
		}
	}
	if len(matching) == 0 {
		return nil, nil, newError(KindNotFound, "key %q not found", key)
	}
	if len(matching) == 1 {
		kv := matching[0]
		if len(kv.Key.Parts) == 1 {
			return unwrapKV(kv), tablePath, nil
		}
		return &dottedKeyView{kv: kv, partIndex: 1, doc: doc}, tablePath, nil
	}
	// Multiple KVs share this prefix -- return a group
	return &dottedKeyGroup{kvs: matching, depth: 1, doc: doc}, tablePath, nil
}

// resolveKeyInDottedGroup handles key lookups within a dottedKeyGroup.
func resolveKeyInDottedGroup(doc *Document, group *dottedKeyGroup, key string) (Node, []string, error) {
	var matching []*KeyValueNode
	for _, kv := range group.kvs {
		if group.depth < len(kv.Key.Parts) && kv.Key.Parts[group.depth] == key {
			matching = append(matching, kv)
		}
	}
	if len(matching) == 0 {
		return nil, nil, newError(KindNotFound, "key %q not found", key)
	}
	if len(matching) == 1 {
		kv := matching[0]
		if group.depth+1 >= len(kv.Key.Parts) {
			// Last part consumed, return the value
			return kv.Val, nil, nil
		}
		return &dottedKeyView{kv: kv, partIndex: group.depth + 1, doc: doc}, nil, nil
	}
	// Still multiple matches -- continue grouping at the next depth
	return &dottedKeyGroup{kvs: matching, depth: group.depth + 1, doc: doc}, nil, nil
}

// buildCompoundView checks if any TableNode or ArrayTableNode has a KeyPath
// that starts with the given prefix. If so, returns a compoundTableView
// representing the implicit intermediate table.
func buildCompoundView(doc *Document, scopeChildren []Node, prefix []string) *compoundTableView {
	var tables []*TableNode
	var arrayTbls []*ArrayTableNode
	for _, child := range doc.Children {
		if tbl, ok := child.(*TableNode); ok {
			if hasPrefix(tbl.KeyPath, prefix) && len(tbl.KeyPath) > len(prefix) {
				tables = append(tables, tbl)
			}
		}
		if at, ok := child.(*ArrayTableNode); ok {
			if hasPrefix(at.KeyPath, prefix) && len(at.KeyPath) > len(prefix) {
				arrayTbls = append(arrayTbls, at)
			}
		}
	}
	if len(tables) == 0 && len(arrayTbls) == 0 {
		return nil
	}
	return &compoundTableView{
		doc:       doc,
		prefix:    prefix,
		tables:    tables,
		arrayTbls: arrayTbls,
	}
}

// resolveKeyInCompoundView handles key lookups within a compoundTableView.
func resolveKeyInCompoundView(doc *Document, view *compoundTableView, key string) (Node, []string, error) {
	targetPath := append(append([]string(nil), view.prefix...), key)

	// Check for exact table match
	for _, tbl := range view.tables {
		if pathsEqual(tbl.KeyPath, targetPath) {
			return tbl, targetPath, nil
		}
	}

	// Check for exact array-table match
	var arrayEntries []*ArrayTableNode
	for _, at := range view.arrayTbls {
		if pathsEqual(at.KeyPath, targetPath) {
			arrayEntries = append(arrayEntries, at)
		}
	}
	if len(arrayEntries) > 0 {
		return &arrayTableCollection{entries: arrayEntries, doc: doc}, targetPath, nil
	}

	// Check for further compound paths (targetPath is a prefix of deeper tables)
	var subTables []*TableNode
	var subArrayTbls []*ArrayTableNode
	for _, tbl := range view.tables {
		if hasPrefix(tbl.KeyPath, targetPath) && len(tbl.KeyPath) > len(targetPath) {
			subTables = append(subTables, tbl)
		}
	}
	for _, at := range view.arrayTbls {
		if hasPrefix(at.KeyPath, targetPath) && len(at.KeyPath) > len(targetPath) {
			subArrayTbls = append(subArrayTbls, at)
		}
	}
	if len(subTables) > 0 || len(subArrayTbls) > 0 {
		return &compoundTableView{
			doc:       doc,
			prefix:    targetPath,
			tables:    subTables,
			arrayTbls: subArrayTbls,
		}, targetPath, nil
	}

	return nil, nil, newError(KindNotFound, "key %q not found", key)
}

// resolveKeyInDottedView handles key lookups within a dottedKeyView.
func resolveKeyInDottedView(doc *Document, view *dottedKeyView, key string) (Node, []string, error) {
	if view.partIndex >= len(view.kv.Key.Parts) {
		// We've consumed all parts; delegate to the value
		return resolveKeySegment(doc, view.kv.Val, nil, key, nil, 0)
	}
	if view.kv.Key.Parts[view.partIndex] == key {
		if view.partIndex+1 >= len(view.kv.Key.Parts) {
			// This is the last part, so the value is the target
			return view.kv.Val, nil, nil
		}
		return &dottedKeyView{kv: view.kv, partIndex: view.partIndex + 1, doc: doc}, nil, nil
	}
	return nil, nil, newError(KindNotFound, "key %q not found", key)
}

// --- Public API methods on Document ---
//
// Resolve, Lookup and Has live in resolve.go, with the read-layer walk they
// share. The typed getters below are their conveniences.

// GetString resolves the path and returns the string value. Returns ("", false)
// if the path is not found or the value is not a string.
func (d *Document) GetString(path string) (string, bool) {
	node, ok := d.Lookup(path)
	if !ok {
		return "", false
	}
	if s, ok := node.(*StringNode); ok {
		return s.Val, true
	}
	return "", false
}

// GetInt resolves the path and returns the integer value. Returns (0, false)
// if the path is not found or the value is not an integer.
func (d *Document) GetInt(path string) (int64, bool) {
	node, ok := d.Lookup(path)
	if !ok {
		return 0, false
	}
	if n, ok := node.(*IntegerNode); ok {
		return n.Val, true
	}
	return 0, false
}

// GetBool resolves the path and returns the boolean value. Returns (false, false)
// if the path is not found or the value is not a boolean.
func (d *Document) GetBool(path string) (bool, bool) {
	node, ok := d.Lookup(path)
	if !ok {
		return false, false
	}
	if b, ok := node.(*BooleanNode); ok {
		return b.Val, true
	}
	return false, false
}

// GetFloat resolves the path and returns the float64 value. Returns (0, false)
// if the path is not found or the value is not a float.
func (d *Document) GetFloat(path string) (float64, bool) {
	node, ok := d.Lookup(path)
	if !ok {
		return 0, false
	}
	if f, ok := node.(*FloatNode); ok {
		return f.Val, true
	}
	return 0, false
}

// GetTime resolves the path and returns a time.Time value. Returns (time.Time{}, false)
// if the path is not found or the value is not an offset date-time.
func (d *Document) GetTime(path string) (time.Time, bool) {
	node, ok := d.Lookup(path)
	if !ok {
		return time.Time{}, false
	}
	if dt, ok := node.(*DateTimeNode); ok {
		return dt.Val, true
	}
	return time.Time{}, false
}
