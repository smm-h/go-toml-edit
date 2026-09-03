package tomledit

// MergeDefaults recursively walks defaults and sets keys that do not exist in
// the document at the given path. If path is empty, merges at the document root.
//
// Maps (map[string]any) merge recursively: only missing keys are set. Scalars
// and arrays are atomic: existing keys are never overwritten. This is useful
// for applying default configuration values to a user-provided TOML file.
func (d *Document) MergeDefaults(path string, defaults map[string]any) error {
	return d.diag(d.mergeMap(path, defaults), "")
}

// mergeMap recursively merges a map[string]any into the document at the given
// path prefix. Keys that already exist in the document are skipped unless the
// value is itself a map, in which case it recurses.
func (d *Document) mergeMap(prefix string, m map[string]any) error {
	for key, val := range m {
		fullPath := key
		if prefix != "" {
			fullPath = prefix + "." + key
		}

		subMap, isMap := val.(map[string]any)
		existing := d.Get(fullPath)

		if existing == nil {
			// Key doesn't exist: set it. For maps, use SetCreate so the whole
			// inline table is created atomically.
			if err := d.SetCreate(fullPath, val); err != nil {
				return wrapError(err, "setting default %q", fullPath)
			}
			continue
		}

		// Key exists. If both sides are maps/tables, recurse.
		if isMap {
			// Check if existing is a table-like node we can merge into.
			switch existing.(type) {
			case *TableNode, *InlineTableNode, *ArrayTableNode,
				*compoundTableView, *dottedKeyView, *dottedKeyGroup:
				if err := d.mergeMap(fullPath, subMap); err != nil {
					return err
				}
			default:
				// Existing is a scalar or array -- keep it (atomic).
			}
			continue
		}

		// Existing key with non-map default: keep existing value (atomic).
	}
	return nil
}

// Merge merges all values from the other document into d. Same recursive
// semantics as MergeDefaults: only keys that do not exist in d are set;
// existing values are never overwritten.
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

		if len(kv.Key.Parts) > 1 {
			continue // handled below
		}

		part := kv.Key.Parts[0]
		fullPath := part
		if prefix != "" {
			fullPath = prefix + "." + part
		}

		existing := target.Get(fullPath)
		if existing == nil {
			// New key: copy value and comments.
			if err := target.SetCreate(fullPath, nodeToValue(kv.Val)); err != nil {
				return wrapError(err, "merging key %q", fullPath)
			}
			// Copy comments to the newly created node.
			copyComments(target, fullPath, kv)
		} else {
			// Existing key: check if both are tables for recursive merge.
			if isTableLike(existing) && isTableLike(kv.Val) {
				if err := mergeChildren(target, source, kv.Val, fullPath); err != nil {
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
		if len(kv.Key.Parts) <= 1 {
			continue
		}
		// Multi-part dotted key like a.b.c = val.
		// Build the full path and merge as a leaf.
		fullPath := buildPathFromParts(prefix, kv.Key.Parts)
		existing := target.Get(fullPath)
		if existing == nil {
			if err := target.SetCreate(fullPath, nodeToValue(kv.Val)); err != nil {
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

	for _, child := range source.Children {
		switch n := child.(type) {
		case *TableNode:
			if !isDirectChild(n.KeyPath, scopePath) {
				continue
			}
			suffix := n.KeyPath[len(scopePath):]
			subPrefix := buildPathFromParts(prefix, suffix)

			existing := target.Get(subPrefix)
			if existing == nil {
				// Entire sub-table is new. Create it and copy all children.
				if err := mergeSubTable(target, source, n, subPrefix); err != nil {
					return err
				}
			} else if isTableLike(existing) {
				// Both sides have this table: recurse.
				if err := mergeChildren(target, source, n, subPrefix); err != nil {
					return err
				}
				// Merge comments on the table header.
				mergeTableComments(target, subPrefix, n)
			}

		case *ArrayTableNode:
			if !isDirectChild(n.KeyPath, scopePath) {
				continue
			}
			suffix := n.KeyPath[len(scopePath):]
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
		existing := target.Get(group.subPrefix)
		if existing != nil {
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
	for _, child := range tbl.Children {
		kv, ok := child.(*KeyValueNode)
		if !ok {
			continue
		}
		fullPath := buildPathFromParts(subPrefix, kv.Key.Parts)
		if err := target.SetCreate(fullPath, nodeToValue(kv.Val)); err != nil {
			return wrapError(err, "merging sub-table key %q", fullPath)
		}
		copyComments(target, fullPath, kv)
	}

	// Also copy sub-sub-tables.
	for _, child := range source.Children {
		if sub, ok := child.(*TableNode); ok {
			if hasPrefix(sub.KeyPath, tbl.KeyPath) && len(sub.KeyPath) > len(tbl.KeyPath) {
				suffix := sub.KeyPath[len(tbl.KeyPath):]
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
	for _, child := range atn.Children {
		kv, ok := child.(*KeyValueNode)
		if !ok {
			continue
		}
		// For array-of-tables, we need to target the last entry.
		// Since we just created it with NewArrayTable, the path with [-1]
		// should work.
		fullPath := subPrefix + "[-1]." + buildPathFromParts("", kv.Key.Parts)
		if err := target.SetCreate(fullPath, nodeToValue(kv.Val)); err != nil {
			return wrapError(err, "merging array table entry key %q", fullPath)
		}
	}
	return nil
}

// scopeChildren returns the children of a scope node.
func scopeChildren(scope Node) []Node {
	switch s := scope.(type) {
	case *Document:
		return s.Children
	case *TableNode:
		return s.Children
	case *ArrayTableNode:
		return s.Children
	case *InlineTableNode:
		return s.Children
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
		return s.KeyPath
	case *ArrayTableNode:
		return s.KeyPath
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
	case *TableNode, *InlineTableNode, *ArrayTableNode,
		*compoundTableView, *dottedKeyView, *dottedKeyGroup:
		return true
	default:
		return false
	}
}

// nodeToValue converts an AST node back to a Go value suitable for SetCreate.
func nodeToValue(n Node) any {
	switch v := n.(type) {
	case *StringNode:
		return v.Val
	case *IntegerNode:
		return v.Val
	case *FloatNode:
		return v.Val
	case *BooleanNode:
		return v.Val
	case *DateTimeNode:
		return v.Val
	case *LocalDateTimeNode:
		return v.Val
	case *LocalDateNode:
		return v.Val
	case *LocalTimeNode:
		return v.Val
	case *ArrayNode:
		items := make([]any, len(v.Elements))
		for i, elem := range v.Elements {
			items[i] = nodeToValue(elem)
		}
		return items
	case *InlineTableNode:
		m := make(map[string]any)
		for _, child := range v.Children {
			if kv, ok := child.(*KeyValueNode); ok {
				key := buildPathFromParts("", kv.Key.Parts)
				m[key] = nodeToValue(kv.Val)
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
	srcLeading := srcKV.LeadingComments()
	srcInline := srcKV.Comment()

	if len(srcLeading) > 0 {
		// SetLeadingComments expects comment text without "# " prefix and "\n" suffix.
		cleaned := cleanLeadingComments(srcLeading)
		if len(cleaned) > 0 {
			_ = target.SetLeadingComments(path, cleaned)
		}
	}
	if srcInline != "" {
		// SetComment expects text without "# " prefix.
		cleaned := cleanInlineComment(srcInline)
		if cleaned != "" {
			_ = target.SetComment(path, cleaned)
		}
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
		existingLeading := targetNode.LeadingComments()

		// Append source comments to existing.
		cleanedSrc := cleanLeadingComments(srcLeading)
		cleanedExisting := cleanLeadingComments(existingLeading)
		combined := append(cleanedExisting, cleanedSrc...)
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
			cleaned := cleanInlineComment(srcInline)
			if cleaned != "" {
				_ = target.SetComment(path, cleaned)
			}
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
		existingLeading := targetNode.LeadingComments()
		cleanedSrc := cleanLeadingComments(srcLeading)
		cleanedExisting := cleanLeadingComments(existingLeading)
		combined := append(cleanedExisting, cleanedSrc...)
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
			cleaned := cleanInlineComment(srcInline)
			if cleaned != "" {
				_ = target.SetComment(path, cleaned)
			}
		}
	}
}

// cleanLeadingComments strips "# " prefix and trailing "\n" from leading
// comment strings, returning the bare text suitable for SetLeadingComments.
func cleanLeadingComments(comments []string) []string {
	var result []string
	for _, c := range comments {
		text := c
		// Strip leading "# "
		if len(text) >= 2 && text[0] == '#' && text[1] == ' ' {
			text = text[2:]
		} else if len(text) >= 1 && text[0] == '#' {
			text = text[1:]
		}
		// Strip trailing newline
		if len(text) > 0 && text[len(text)-1] == '\n' {
			text = text[:len(text)-1]
		}
		result = append(result, text)
	}
	return result
}

// cleanInlineComment strips the "# " prefix from an inline comment string.
func cleanInlineComment(comment string) string {
	text := comment
	if len(text) >= 2 && text[0] == '#' && text[1] == ' ' {
		return text[2:]
	}
	if len(text) >= 1 && text[0] == '#' {
		return text[1:]
	}
	return text
}
