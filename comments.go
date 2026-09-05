package tomledit

// SetComment sets the inline comment on the node at the given path.
// The comment string should NOT include the "# " prefix -- it will be added
// automatically. An empty string removes the comment.
// For table paths, the comment is set on the table header line.
// Returns an error if the path does not exist or targets a member of an
// inline table (TOML forbids comments inside inline tables).
//
// The text has to be something a comment can carry: valid UTF-8, and no
// control character other than a tab -- so a newline, a carriage return and
// U+0000 are each refused with KindBadInput, and the document is left as it
// was. These are the lexer's rules for reading a comment; text breaking them
// would render bytes the parser cannot read back.
func (d *Document) SetComment(path string, comment string) error {
	if err := checkWritableComment(comment); err != nil {
		return d.diag(err, path)
	}
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
//
// Each element is ONE comment line, and each is held to what SetComment holds
// its text to: valid UTF-8, no control character other than a tab. An element
// carrying a newline would be a second line it does not get to open, so it is
// refused with KindBadInput -- and one refused element refuses the whole call,
// leaving the node's comments as they were.
//
// A nil or empty slice removes the leading comments. An empty STRING element
// is not removal: it writes a "# " comment line with no content.
func (d *Document) SetLeadingComments(path string, comments []string) error {
	// Every line is checked before the first is written: the caller asked for
	// one block of comments, so a document carrying part of it is one the
	// caller never described.
	for _, c := range comments {
		if err := checkWritableComment(c); err != nil {
			return d.diag(err, path)
		}
	}
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

// GetComment returns the inline comment on the node at the given path, as
// text: without the "#" and the whitespace around it, so a line written
// `x = 1 # note` answers "note". A node with no inline comment answers the
// empty string.
//
// The path resolves to the same node SetComment writes to -- for a key-value
// path the pair, not the unwrapped value; for a table path the header line --
// so the two round-trip: what SetComment writes, GetComment reads back. It
// answers the NORMALIZED text, which is what SetComment takes, and a caller
// that needs the bytes as written reads Raw on the node instead.
//
// The navigation errors are SetComment's: a path naming nothing is
// KindNotFound, a path naming a member of an inline table is
// KindWrongContainer (TOML gives an inline table nowhere to put a comment,
// so nothing there can carry one to read), and a malformed path is
// KindBadPath.
func (d *Document) GetComment(path string) (string, error) {
	node, err := d.resolveCommentTarget(path)
	if err != nil {
		return "", d.diag(err, path)
	}
	return node.Comment(), nil
}

// GetLeadingComments returns the comment lines written above the node at the
// given path, in order, each as text: without its "#", its trailing newline
// and the whitespace around them. A node with no leading comments answers nil.
//
// The path resolves to the same node SetLeadingComments writes to, so the two
// round-trip, and the navigation errors are the same ones -- see GetComment.
func (d *Document) GetLeadingComments(path string) ([]string, error) {
	node, err := d.resolveCommentTarget(path)
	if err != nil {
		return nil, d.diag(err, path)
	}
	comments := node.LeadingComments()
	if len(comments) == 0 {
		return nil, nil
	}
	return comments, nil
}

// checkWritableComment refuses comment text a comment cannot carry. What one
// may hold is the lexer's rule, mirrored here: valid UTF-8, and no control
// character other than a tab (U+0000-U+0008, U+000A-U+001F and U+007F are all
// out, which covers the newline and the carriage return that would end the
// comment and turn its tail into a line of TOML nobody wrote). Checking at the
// write keeps the document renderable back into itself.
func checkWritableComment(text string) error {
	if err := checkWritableText(text, "comment"); err != nil {
		return err
	}
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if ch < 0x80 && isControlChar(ch) {
			return newError(KindBadInput,
				"the comment %q carries the control character U+%04X, which TOML does not allow in a comment",
				text, ch).withValue(text)
		}
	}
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

	// The comment goes on the line that binds the key, so what hosts it is the
	// container that WRITES the pair: for a table no single node stands for,
	// that is the region a dotted key spelled the table out in.
	host := parent.node
	if host == nil && parent.rec != nil {
		host, _, _ = parent.rec.impliedRegion()
	}

	// TOML gives an inline table no place to put a comment, so the container
	// structurally cannot host the operation -- the same refusal as renaming
	// through an array index, and not a conflict, which would say the edit
	// produces an invalid document.
	if isInsideInlineTable(host) {
		return nil, newError(KindWrongContainer, "an inline table has nowhere to put a comment: TOML does not allow one inside")
	}

	if kv := bindingKV(parent, lastSeg.Key); kv != nil {
		return kv, nil
	}

	// Fallback: return the resolved node itself.
	return node, nil
}

// bindingKV returns the pair that binds key at the position: the one written in
// a concrete container, or -- for a table no single node stands for -- the
// dotted pair in the region that spells that table out.
func bindingKV(pos layerPos, key string) *KeyValueNode {
	if pos.node != nil {
		return findKVInParent(pos.node, key)
	}
	if pos.rec != nil {
		return pos.rec.dottedKV(key)
	}
	return nil
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
		children = p.children
	case *TableNode:
		children = p.children
	case *ArrayTableNode:
		children = p.children
	case *InlineTableNode:
		children = p.children
	default:
		return nil
	}
	for _, child := range children {
		if kv, ok := child.(*KeyValueNode); ok {
			if len(kv.key.parts) > 0 && kv.key.parts[0] == key {
				return kv
			}
		}
	}
	return nil
}
