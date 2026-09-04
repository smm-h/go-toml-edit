package tomledit

// Merge merges all values from the other document into d. Only keys that do
// not exist in d are set; existing values are never overwritten. Seeding a
// document from a list of defaults rather than from another document is
// EnsureDefaults.
//
// Comment handling:
//   - For existing keys: other's leading comments are appended to d's leading
//     comments; if d has no inline comment and other does, it is copied.
//   - For new keys: comments from other are brought along with the value.
//
// Array-of-tables are treated atomically: if d already has entries for a
// given path, all of other's entries for that path are skipped.
func (d *Document) Merge(other *Document) error {
	return d.diag(mergeChildren(d, other, other, ""), "")
}

// mergeChildren walks the source scope's children (KVs) and sub-tables,
// merging them into the target document at the given path prefix.
func mergeChildren(target *Document, source *Document, scope Node, prefix string) error {
	children := scopeChildren(scope)

	// Process KV children in the source scope.
	for _, child := range children {
		kv, ok := child.(*KeyValueNode)
		if !ok {
			continue
		}

		if len(kv.key.parts) > 1 {
			continue // handled below
		}

		part := kv.key.parts[0]
		fullPath := part
		if prefix != "" {
			fullPath = prefix + "." + part
		}

		existing, present := target.probe(fullPath)
		if !present {
			// New key: copy value and comments.
			if err := target.SetCreate(fullPath, nodeToValue(kv.val)); err != nil {
				return wrapError(err, "merging key %q", fullPath)
			}
			// Copy comments to the newly created node.
			copyComments(target, fullPath, kv)
		} else {
			// Existing key: check if both are tables for recursive merge.
			if existing.rec != nil && isTableLike(kv.val) {
				if err := mergeChildren(target, source, kv.val, fullPath); err != nil {
					return err
				}
			}
			// Merge comments on existing key.
			mergeCommentsOnExisting(target, fullPath, kv)
		}
	}

	// Process dotted keys: flatten them to single-key entries.
	for _, child := range children {
		kv, ok := child.(*KeyValueNode)
		if !ok {
			continue
		}
		if len(kv.key.parts) <= 1 {
			continue
		}
		// Multi-part dotted key like a.b.c = val.
		// Build the full path and merge as a leaf.
		fullPath := buildPathFromParts(prefix, kv.key.parts)
		if _, present := target.probe(fullPath); !present {
			if err := target.SetCreate(fullPath, nodeToValue(kv.val)); err != nil {
				return wrapError(err, "merging dotted key %q", fullPath)
			}
			copyComments(target, fullPath, kv)
		} else {
			mergeCommentsOnExisting(target, fullPath, kv)
		}
	}

	// Process sub-tables and array-tables from the source document that
	// belong to the current scope.
	scopePath := scopeKeyPath(scope)

	// Collect array-table entries grouped by path so we can handle them
	// atomically (all-or-nothing per path).
	type arrayTableGroup struct {
		subPrefix string
		entries   []*ArrayTableNode
	}
	var arrayTableGroups []arrayTableGroup
	seenArrayPaths := map[string]int{} // subPrefix -> index into arrayTableGroups

	for _, child := range source.children {
		switch n := child.(type) {
		case *TableNode:
			if !isDirectChild(n.keyPath, scopePath) {
				continue
			}
			suffix := n.keyPath[len(scopePath):]
			subPrefix := buildPathFromParts(prefix, suffix)

			existing, present := target.probe(subPrefix)
			if !present {
				// Entire sub-table is new. Create it and copy all children.
				if err := mergeSubTable(target, source, n, subPrefix); err != nil {
					return err
				}
			} else if existing.rec != nil {
				// Both sides have this table: recurse.
				if err := mergeChildren(target, source, n, subPrefix); err != nil {
					return err
				}
				// Merge comments on the table header.
				mergeTableComments(target, subPrefix, n)
			}

		case *ArrayTableNode:
			if !isDirectChild(n.keyPath, scopePath) {
				continue
			}
			suffix := n.keyPath[len(scopePath):]
			subPrefix := buildPathFromParts(prefix, suffix)

			if idx, ok := seenArrayPaths[subPrefix]; ok {
				arrayTableGroups[idx].entries = append(arrayTableGroups[idx].entries, n)
			} else {
				seenArrayPaths[subPrefix] = len(arrayTableGroups)
				arrayTableGroups = append(arrayTableGroups, arrayTableGroup{
					subPrefix: subPrefix,
					entries:   []*ArrayTableNode{n},
				})
			}
		}
	}

	// Process array-table groups: if target has zero entries, copy all;
	// if target already has entries, skip (atomic).
	for _, group := range arrayTableGroups {
		if _, present := target.probe(group.subPrefix); present {
			// Target already has this array-of-tables: keep it (atomic).
			continue
		}
		// Target has no entries: copy all source entries.
		for _, entry := range group.entries {
			if err := mergeArrayTableEntry(target, source, entry, group.subPrefix); err != nil {
				return err
			}
		}
	}

	return nil
}

// mergeSubTable creates a new table at subPrefix in target and copies all KVs
// from the source table node.
func mergeSubTable(target *Document, source *Document, tbl *TableNode, subPrefix string) error {
	for _, child := range tbl.children {
		kv, ok := child.(*KeyValueNode)
		if !ok {
			continue
		}
		fullPath := buildPathFromParts(subPrefix, kv.key.parts)
		if err := target.SetCreate(fullPath, nodeToValue(kv.val)); err != nil {
			return wrapError(err, "merging sub-table key %q", fullPath)
		}
		copyComments(target, fullPath, kv)
	}

	// Also copy sub-sub-tables.
	for _, child := range source.children {
		if sub, ok := child.(*TableNode); ok {
			if hasPrefix(sub.keyPath, tbl.keyPath) && len(sub.keyPath) > len(tbl.keyPath) {
				suffix := sub.keyPath[len(tbl.keyPath):]
				nestedPrefix := buildPathFromParts(subPrefix, suffix)
				if err := mergeSubTable(target, source, sub, nestedPrefix); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// mergeArrayTableEntry creates a new [[array-table]] entry in the target.
func mergeArrayTableEntry(target *Document, _ *Document, atn *ArrayTableNode, subPrefix string) error {
	if err := target.NewArrayTable(subPrefix); err != nil {
		return wrapError(err, "creating array table %q", subPrefix)
	}
	// Find the last entry index (the one we just created is at index 0 if new).
	// Use SetCreate to add children.
	for _, child := range atn.children {
		kv, ok := child.(*KeyValueNode)
		if !ok {
			continue
		}
		// For array-of-tables, we need to target the last entry.
		// Since we just created it with NewArrayTable, the path with [-1]
		// should work.
		fullPath := subPrefix + "[-1]." + buildPathFromParts("", kv.key.parts)
		if err := target.SetCreate(fullPath, nodeToValue(kv.val)); err != nil {
			return wrapError(err, "merging array table entry key %q", fullPath)
		}
	}
	return nil
}

// scopeChildren returns the children of a scope node.
func scopeChildren(scope Node) []Node {
	switch s := scope.(type) {
	case *Document:
		return s.children
	case *TableNode:
		return s.children
	case *ArrayTableNode:
		return s.children
	case *InlineTableNode:
		return s.children
	default:
		return nil
	}
}

// scopeKeyPath returns the key path of a scope node.
func scopeKeyPath(scope Node) []string {
	switch s := scope.(type) {
	case *Document:
		return nil
	case *TableNode:
		return s.keyPath
	case *ArrayTableNode:
		return s.keyPath
	default:
		return nil
	}
}

// isDirectChild returns true if childPath is a direct child of parentPath.
// For example, ["a","b"] is a direct child of ["a"] but not of [].
// For the root scope (parentPath is nil), any single-element path is direct.
func isDirectChild(childPath, parentPath []string) bool {
	if !hasPrefix(childPath, parentPath) {
		return false
	}
	return len(childPath) > len(parentPath)
}

// isTableLike returns true if the node represents a table-like structure
// that can be recursively merged.
func isTableLike(n Node) bool {
	switch n.(type) {
	case *TableNode, *InlineTableNode, *ArrayTableNode:
		return true
	default:
		return false
	}
}

// probe reports what the path names in the read-layer, and whether it names
// anything at all. Unlike Lookup it answers for logical positions too: a merge
// asks whether the target already carries a key, and a table the target spells
// with a longer header carries one just as much as a table with its own.
func (d *Document) probe(path string) (layerPos, bool) {
	pos, err := d.resolvePos(path)
	if err != nil {
		return layerPos{}, false
	}
	return pos, true
}

// nodeToValue converts an AST node back to a Go value suitable for SetCreate.
func nodeToValue(n Node) any {
	switch v := n.(type) {
	case *StringNode:
		return v.val.get()
	case *IntegerNode:
		return v.val.get()
	case *FloatNode:
		return v.val.get()
	case *BooleanNode:
		return v.val.get()
	case *DateTimeNode:
		return v.val.get()
	case *LocalDateTimeNode:
		return v.val.get()
	case *LocalDateNode:
		return v.val.get()
	case *LocalTimeNode:
		return v.val.get()
	case *ArrayNode:
		items := make([]any, len(v.elements))
		for i, elem := range v.elements {
			items[i] = nodeToValue(elem)
		}
		return items
	case *InlineTableNode:
		m := make(map[string]any)
		for _, child := range v.children {
			if kv, ok := child.(*KeyValueNode); ok {
				key := buildPathFromParts("", kv.key.parts)
				m[key] = nodeToValue(kv.val)
			}
		}
		return m
	default:
		return nil
	}
}

// copyComments copies leading and inline comments from a source KV node to
// the target document at the given path.
func copyComments(target *Document, path string, srcKV *KeyValueNode) {
	// The getters answer in the same normalized text the setters take, so a
	// comment moves from one document to another without being reformatted.
	srcLeading := srcKV.LeadingComments()
	srcInline := srcKV.Comment()

	if len(srcLeading) > 0 {
		_ = target.SetLeadingComments(path, srcLeading)
	}
	if srcInline != "" {
		_ = target.SetComment(path, srcInline)
	}
}

// mergeCommentsOnExisting appends source comments to existing target comments.
func mergeCommentsOnExisting(target *Document, path string, srcKV *KeyValueNode) {
	srcLeading := srcKV.LeadingComments()
	srcInline := srcKV.Comment()

	if len(srcLeading) > 0 {
		// Get existing leading comments on the target.
		targetNode, err := target.resolveCommentTarget(path)
		if err != nil {
			return
		}
		// Append source comments to existing.
		combined := append(targetNode.LeadingComments(), srcLeading...)
		if len(combined) > 0 {
			_ = target.SetLeadingComments(path, combined)
		}
	}

	if srcInline != "" {
		// Only copy inline comment if target has none.
		targetNode, err := target.resolveCommentTarget(path)
		if err != nil {
			return
		}
		if targetNode.Comment() == "" {
			_ = target.SetComment(path, srcInline)
		}
	}
}

// mergeTableComments merges comments from a source table header to target.
func mergeTableComments(target *Document, path string, srcTbl *TableNode) {
	srcLeading := srcTbl.LeadingComments()
	srcInline := srcTbl.Comment()

	if len(srcLeading) > 0 {
		targetNode, err := target.resolveCommentTarget(path)
		if err != nil {
			return
		}
		combined := append(targetNode.LeadingComments(), srcLeading...)
		if len(combined) > 0 {
			_ = target.SetLeadingComments(path, combined)
		}
	}

	if srcInline != "" {
		targetNode, err := target.resolveCommentTarget(path)
		if err != nil {
			return
		}
		if targetNode.Comment() == "" {
			_ = target.SetComment(path, srcInline)
		}
	}
}
