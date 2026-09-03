package tomledit

import "fmt"

// SetComment sets the inline comment on the node at the given path.
// The comment string should NOT include the "# " prefix -- it will be added
// automatically. An empty string removes the comment.
// For table paths, the comment is set on the table header line.
// Returns an error if the path does not exist or targets a member of an
// inline table (TOML forbids comments inside inline tables).
func (d *Document) SetComment(path string, comment string) error {
	node, err := d.resolveCommentTarget(path)
	if err != nil {
		return err
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
		return err
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
	segments, err := parsePath(path)
	if err != nil {
		return nil, fmt.Errorf("path syntax error: %w", err)
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	// First try resolving the full path normally to see what we get.
	node, err := resolveNode(d, segments)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}

	// If the resolved node is a table or array-table, return it directly.
	switch node.(type) {
	case *TableNode, *ArrayTableNode:
		return node, nil
	}

	// For value nodes (string, int, etc.), we need the parent KV node.
	// Resolve the parent, then find the KV with the matching key.
	lastSeg := segments[len(segments)-1]
	if lastSeg.Type != keySegment {
		// Index segments: the resolved node is what we have.
		return node, nil
	}

	parentSegs := segments[:len(segments)-1]
	parent, err := resolveNode(d, parentSegs)
	if err != nil {
		return nil, fmt.Errorf("parent path not found: %w", err)
	}

	// Check if the parent is an inline table. TOML does not allow comments
	// inside inline tables, so setting a comment would produce invalid TOML.
	if isInsideInlineTable(parent) {
		return nil, fmt.Errorf("cannot set comment on inline table member: TOML does not allow comments inside inline tables")
	}

	kv := findKVInParent(parent, d, lastSeg.Key)
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
func findKVInParent(parent Node, doc *Document, key string) *KeyValueNode {
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
