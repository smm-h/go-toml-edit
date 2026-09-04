package tomledit

// SetComment sets the inline comment on the node at the given path.
// The comment string should NOT include the "# " prefix -- it will be added
// automatically. An empty string removes the comment.
// For table paths, the comment is set on the table header line.
// Returns an error if the path does not exist or targets a member of an
// inline table (TOML forbids comments inside inline tables).
func (d *Document) SetComment(path string, comment string) error {
	node, err := d.resolveCommentTarget(path)
	if err != nil {
		return d.diag(err, path)
	}
	if comment == "" {
		node.setComment("")
	} else {
		node.setComment("# " + comment)
	}
	node.markDirty()
	return nil
}

// SetLeadingComments sets the leading comment lines on the node at the given
// path. Each string should NOT include the "# " prefix -- it will be added
// automatically. For table paths, the comments are set on the table header.
// Returns an error if the path does not exist.
func (d *Document) SetLeadingComments(path string, comments []string) error {
	node, err := d.resolveCommentTarget(path)
	if err != nil {
		return d.diag(err, path)
	}
	formatted := make([]string, len(comments))
	for i, c := range comments {
		formatted[i] = "# " + c + "\n"
	}
	node.setLeadingComments(formatted)
	node.markDirty()
	return nil
}

// resolveCommentTarget resolves a path to the node that should receive
// comments. For key-value paths this is the KeyValueNode (not the unwrapped
// value). For table paths it's the TableNode or ArrayTableNode.
func (d *Document) resolveCommentTarget(path string) (Node, error) {
	segments, err := ParsePath(path)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		return nil, newError(KindBadPath, "empty path")
	}

	// Resolve the full path first to see what it names.
	pos, err := d.walkPath(segments)
	if err != nil {
		return nil, wrapError(err, "path not found")
	}
	node, ok := pos.concrete()
	if !ok {
		// Nothing carries the comment: the path names an array-of-tables, or a
		// table only implied by a longer header or a dotted key.
		return nil, pos.noNodeError()
	}

	// A table or array-table header carries its own comments.
	switch node.(type) {
	case *TableNode, *ArrayTableNode:
		return node, nil
	}

	// For value nodes (string, int, etc.), we need the parent KV node.
	// Resolve the parent, then find the KV with the matching key.
	lastSeg := segments[len(segments)-1]
	if lastSeg.Kind != SegmentKey {
		// Index segments: the resolved node is what we have.
		return node, nil
	}

	parent, err := d.walkPath(segments[:len(segments)-1])
	if err != nil {
		return nil, wrapError(err, "parent path not found")
	}

	// TOML gives an inline table no place to put a comment, so the container
	// structurally cannot host the operation -- the same refusal as renaming
	// through an array index, and not a conflict, which would say the edit
	// produces an invalid document.
	if isInsideInlineTable(parent.node) {
		return nil, newError(KindWrongContainer, "an inline table has nowhere to put a comment: TOML does not allow one inside")
	}

	kv := findKVInParent(parent.node, lastSeg.Key)
	if kv != nil {
		return kv, nil
	}

	// Fallback: return the resolved node itself.
	return node, nil
}

// isInsideInlineTable returns true if the node is an InlineTableNode or
// wraps one (e.g. a KeyValueNode whose value is inside an inline table).
func isInsideInlineTable(node Node) bool {
	switch node.(type) {
	case *InlineTableNode:
		return true
	}
	return false
}

// findKVInParent searches for a KeyValueNode with the given key in a parent.
func findKVInParent(parent Node, key string) *KeyValueNode {
	var children []Node
	switch p := parent.(type) {
	case *Document:
		children = p.Children
	case *TableNode:
		children = p.Children
	case *ArrayTableNode:
		children = p.Children
	case *InlineTableNode:
		children = p.Children
	default:
		return nil
	}
	for _, child := range children {
		if kv, ok := child.(*KeyValueNode); ok {
			if len(kv.Key.Parts) > 0 && kv.Key.Parts[0] == key {
				return kv
			}
		}
	}
	return nil
}
