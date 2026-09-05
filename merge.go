package tomledit

// Merge merges all values from the other document into d. Only keys that do
// not exist in d are set; existing values are never overwritten. Seeding a
// document from a list of defaults rather than from another document is
// EnsureDefaults.
//
// The other document is read through the read-layer, so what merges is what it
// MEANS, not how it is written: a key bound by a dotted key, by a header table
// or by an inline table arrives the same way, and a table the source only
// implies is merged as the table it implies. The target is asked the same
// question -- a key it spells with a longer header counts as present just as
// much as one with a header of its own.
//
// Comment handling:
//   - For existing keys: other's leading comments are appended to d's leading
//     comments; if d has no inline comment and other does, it is copied.
//   - For new keys: comments from other are brought along with the value.
//
// Array-of-tables are treated atomically: if d already has anything at a given
// path, all of other's entries for that path are skipped.
func (d *Document) Merge(other *Document) error {
	src, err := other.readLayer()
	if err != nil {
		return d.diag(err, "")
	}
	return d.diag(mergeRecord(d, src, ""), "")
}

// mergeRecord merges every entry of a source record into the target document
// under the given path prefix.
func mergeRecord(target *Document, src *Record, prefix string) error {
	for e := range src.Entries() {
		path := buildPathFromParts(prefix, []string{e.Key()})
		var err error
		switch e.Kind() {
		case EntryValue:
			err = mergeEntryValue(target, path, e)
		case EntryRecord:
			err = mergeEntryRecord(target, path, e)
		case EntryRecords:
			err = mergeEntryRecords(target, path, e)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// mergeEntryValue merges a scalar or plain-array entry.
func mergeEntryValue(target *Document, path string, e Entry) error {
	node, ok := e.Node()
	if !ok {
		return nil
	}
	if _, present := target.probe(path); present {
		mergeCommentsOnExisting(target, path, commentHost(node))
		return nil
	}
	if err := target.SetCreate(path, nodeToValue(node)); err != nil {
		return wrapError(err, "merging key %q", path)
	}
	copyComments(target, path, commentHost(node))
	return nil
}

// mergeEntryRecord merges a table entry, in whatever form the source spells it.
//
// An inline table is a VALUE: the source wrote it as one construct, and the
// target receives it as one, keeping the spelling. Every other table form is
// merged key by key, so a target that already carries some of its keys keeps
// them.
func mergeEntryRecord(target *Document, path string, e Entry) error {
	src, ok := e.Record()
	if !ok {
		return nil
	}
	srcNode, hasNode := src.Node()

	pos, present := target.probe(path)
	if present {
		if hasNode {
			mergeCommentsOnExisting(target, path, commentHost(srcNode))
		}
		if pos.rec == nil {
			// The target binds this name to something that is not a table --
			// a value, or an array-of-tables. Merging into it would overwrite.
			return nil
		}
		return mergeRecord(target, src, path)
	}

	if inline, isInline := srcNode.(*InlineTableNode); hasNode && isInline {
		if err := target.SetCreate(path, nodeToValue(inline)); err != nil {
			return wrapError(err, "merging inline table %q", path)
		}
		copyComments(target, path, commentHost(inline))
		return nil
	}

	if err := mergeRecord(target, src, path); err != nil {
		return err
	}
	if hasNode {
		// The keys wrote the table into existence; its header can carry the
		// comments the source header carried. A source table with no keys
		// wrote nothing, and the copy quietly finds nothing to write to.
		copyComments(target, path, commentHost(srcNode))
	}
	return nil
}

// mergeEntryRecords merges an array-of-tables entry. It is atomic: a target
// that already has anything at the path keeps what it has, entries and all.
func mergeEntryRecords(target *Document, path string, e Entry) error {
	if _, present := target.probe(path); present {
		return nil
	}
	records, ok := e.Records()
	if !ok {
		return nil
	}
	for _, src := range records {
		if err := target.NewArrayTable(path); err != nil {
			return wrapError(err, "creating array table %q", path)
		}
		// Every write below addresses the entry just created, which is the
		// last one until the next iteration creates another.
		entryPath := path + "[-1]"
		if srcNode, hasNode := src.Node(); hasNode {
			copyComments(target, entryPath, commentHost(srcNode))
		}
		if err := mergeRecord(target, src, entryPath); err != nil {
			return err
		}
	}
	return nil
}

// commentHost returns the node whose trivia carries a construct's comments. A
// header opens with its own line and holds them itself; a value holds them on
// the pair that binds it, which is the line they were written above.
func commentHost(n Node) Node {
	switch n.(type) {
	case *TableNode, *ArrayTableNode:
		return n
	}
	if kv, ok := n.parentNode().(*KeyValueNode); ok {
		return kv
	}
	return n
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
//
// An inline table becomes the ordered []Pair form, not a map: the write path
// renders a map with its keys sorted, which would alphabetize a table the
// source wrote in its own order.
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
		return inlineTableToPairs(v)
	default:
		return nil
	}
}

// inlineTableToPairs converts an inline table to the ordered []Pair form, in
// the order its keys were written. A dotted key inside the table names a
// nested table, so it folds under its first part into a nested []Pair, which
// is what the read-layer reads it as.
func inlineTableToPairs(t *InlineTableNode) []Pair {
	pairs := make([]Pair, 0, len(t.children))
	for _, child := range t.children {
		kv, ok := child.(*KeyValueNode)
		if !ok {
			continue
		}
		pairs = insertOrderedPair(pairs, kv.key.parts, nodeToValue(kv.val))
	}
	return pairs
}

// insertOrderedPair binds a value under a dotted key inside an ordered pair
// list, descending into the nested list each leading part names and creating
// one where the part is new. A leading part already bound to something that is
// not a nested table cannot come from a document that parsed; the binding is
// appended anyway, so the write path refuses the duplicate key rather than
// dropping a value in silence.
func insertOrderedPair(pairs []Pair, parts []string, val any) []Pair {
	if len(parts) == 0 {
		return pairs
	}
	if len(parts) == 1 {
		return append(pairs, Pair{Key: parts[0], Value: val})
	}
	for i := range pairs {
		if pairs[i].Key != parts[0] {
			continue
		}
		if nested, ok := pairs[i].Value.([]Pair); ok {
			pairs[i].Value = insertOrderedPair(nested, parts[1:], val)
			return pairs
		}
		break
	}
	return append(pairs, Pair{Key: parts[0], Value: insertOrderedPair(nil, parts[1:], val)})
}

// copyComments copies the leading and inline comments of a source construct to
// the target document at the given path.
func copyComments(target *Document, path string, srcNode Node) {
	// The getters answer in the same normalized text the setters take, so a
	// comment moves from one document to another without being reformatted.
	srcLeading := srcNode.LeadingComments()
	srcInline := srcNode.Comment()

	if len(srcLeading) > 0 {
		_ = target.SetLeadingComments(path, srcLeading)
	}
	if srcInline != "" {
		_ = target.SetComment(path, srcInline)
	}
}

// mergeCommentsOnExisting appends source comments to existing target comments.
func mergeCommentsOnExisting(target *Document, path string, srcNode Node) {
	srcLeading := srcNode.LeadingComments()
	srcInline := srcNode.Comment()

	if len(srcLeading) > 0 {
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
		// Only copy the inline comment if the target has none.
		targetNode, err := target.resolveCommentTarget(path)
		if err != nil {
			return
		}
		if targetNode.Comment() == "" {
			_ = target.SetComment(path, srcInline)
		}
	}
}
