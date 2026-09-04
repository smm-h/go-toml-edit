package tomledit

// Path resolution walks the read-layer, not the raw AST: a path step means the
// same thing whatever spelling the document used for the construct it
// addresses. A position reached this way is one of three things:
//
//   - a record -- a table in any spelling. It carries a concrete node when one
//     stands for it (a header table, an inline table, the document itself) and
//     none when the table is only implied, by a longer header or a dotted key.
//   - a collection -- the entries of an array-of-tables. No single node stands
//     for one; indexing it reaches an entry, which is a record with a node.
//   - a value -- a scalar or an array, which is always a concrete node.
//
// Everything that answers with a Node -- Resolve, Lookup, the getters, the
// edit operations -- needs a position that carries one, and refuses a position
// that does not with KindWrongContainer. Everything that addresses structure --
// an index into a collection, a key inside a table -- works from the position
// itself and never needs one.
type layerPos struct {
	// rec is set when the position is a record.
	rec *Record
	// records is set when the position is an array-of-tables collection.
	records []*Record
	// node is the concrete node at this position, or nil when none stands
	// for it.
	node Node
}

// posFromRecord returns the position of a record.
func posFromRecord(r *Record) layerPos {
	return layerPos{rec: r, node: r.node}
}

// posFromEntry returns the position an entry addresses.
func posFromEntry(e Entry) layerPos {
	switch e.kind {
	case EntryRecord:
		return layerPos{rec: e.record, node: e.record.node}
	case EntryRecords:
		return layerPos{records: e.records}
	default:
		return layerPos{node: e.node}
	}
}

// posFromNode returns the position of a concrete node reached without the
// layer -- an array element, for instance.
func posFromNode(n Node) layerPos {
	return layerPos{node: n}
}

// concrete returns the node at the position, and whether one stands for it.
func (p layerPos) concrete() (Node, bool) {
	if p.node == nil {
		return nil, false
	}
	return p.node, true
}

// describe names what the position addresses, for diagnostics.
func (p layerPos) describe() string {
	switch {
	case p.records != nil:
		return "array-of-tables"
	case p.node != nil:
		return p.node.Type().String() + " node"
	case p.rec != nil:
		return "table"
	default:
		return "nothing"
	}
}

// noNodeError reports that the position names something no single node stands
// for. It is the refusal every concrete-node surface makes on a logical-only
// path.
func (p layerPos) noNodeError() *Error {
	if p.records != nil {
		return newError(KindWrongContainer,
			"an array-of-tables is not a single node: index it to address an entry, or read it through the read-layer")
	}
	return newError(KindWrongContainer,
		"this table is not a single node: it is implied by a longer header or a dotted key, so read it through the read-layer")
}

// notFound reports a key the position does not carry, naming the container the
// way the document spells it.
func (p layerPos) notFound(key string) *Error {
	switch n := p.node.(type) {
	case *TableNode:
		return newError(KindNotFound, "key %q not found in table [%s]", key, pathFromKeys(n.keyPath))
	case *ArrayTableNode:
		return newError(KindNotFound, "key %q not found in array table [[%s]]", key, pathFromKeys(n.keyPath))
	case *InlineTableNode:
		return newError(KindNotFound, "key %q not found in inline table", key)
	}
	return newError(KindNotFound, "key %q not found", key)
}

// key navigates to a named child of the position.
func (p layerPos) key(name string) (layerPos, error) {
	if p.records != nil {
		return layerPos{}, newError(KindWrongContainer,
			"cannot look up key %q on array-of-tables (use an index first)", name)
	}
	if p.rec != nil {
		e, ok := p.rec.Get(name)
		if !ok {
			return layerPos{}, p.notFound(name)
		}
		return posFromEntry(e), nil
	}
	// An inline table reached without the layer -- as an array element, say --
	// folds on demand, so a key inside it resolves like any other.
	if inline, ok := p.node.(*InlineTableNode); ok {
		rec, err := foldInlineTable(inline)
		if err != nil {
			return layerPos{}, err
		}
		e, ok := rec.Get(name)
		if !ok {
			return layerPos{}, p.notFound(name)
		}
		return posFromEntry(e), nil
	}
	return layerPos{}, newError(KindWrongContainer, "cannot look up key %q in %s", name, p.describe())
}

// binding reports what the position's logical view already binds the key to,
// and whether it binds it at all. It is what an edit asks before writing a key:
// the answer covers every spelling -- a value, a table in any form, a table
// another construct only implied, an array-of-tables -- where a scan of the
// container's own children would see only the pairs written there literally.
//
// A position with no record of its own -- an inline table reached as an array
// element, say -- folds on demand.
func (p layerPos) binding(key string) (Entry, bool, error) {
	if p.rec != nil {
		e, ok := p.rec.Get(key)
		return e, ok, nil
	}
	if inline, ok := p.node.(*InlineTableNode); ok {
		rec, err := foldInlineTable(inline)
		if err != nil {
			return Entry{}, false, err
		}
		e, ok := rec.Get(key)
		return e, ok, nil
	}
	return Entry{}, false, nil
}

// at navigates to an element of the position by index. A negative index counts
// from the end.
func (p layerPos) at(index int) (layerPos, error) {
	if p.records != nil {
		idx, err := normalizeIndex(index, len(p.records))
		if err != nil {
			return layerPos{}, err
		}
		return posFromRecord(p.records[idx]), nil
	}
	if arr, ok := p.node.(*ArrayNode); ok {
		idx, err := normalizeIndex(index, len(arr.elements))
		if err != nil {
			return layerPos{}, err
		}
		return posFromNode(arr.elements[idx]), nil
	}
	return layerPos{}, newError(KindWrongContainer, "cannot index into %s", p.describe())
}

// step follows one path segment.
func (p layerPos) step(seg PathSegment) (layerPos, error) {
	if seg.Kind == SegmentIndex {
		return p.at(seg.Index)
	}
	return p.key(seg.Key)
}

// walkFrom follows segments from a starting position.
func walkFrom(pos layerPos, segments []PathSegment) (layerPos, error) {
	var err error
	for _, seg := range segments {
		pos, err = pos.step(seg)
		if err != nil {
			return layerPos{}, err
		}
	}
	return pos, nil
}

// walkPath folds the document and follows segments from its root. No segments
// address the document itself.
func (d *Document) walkPath(segments []PathSegment) (layerPos, error) {
	root, err := foldDocument(d)
	if err != nil {
		return layerPos{}, err
	}
	return walkFrom(posFromRecord(root), segments)
}

// resolvePos parses a path and walks it.
func (d *Document) resolvePos(path string) (layerPos, error) {
	segments, err := ParsePath(path)
	if err != nil {
		return layerPos{}, err
	}
	return d.walkPath(segments)
}

// --- public read surface ---

// Resolve resolves the path against the document and returns the node it
// names. For a key-value pair the value node is returned, not the pair.
//
// Path syntax is ParsePath's: dots between keys ("server.host"), brackets for
// indices ("items[0]", "items[-1]" for the last), quotes around a key that
// would otherwise read as more than one segment.
//
// The returned *Error carries KindBadPath for a syntactically invalid path,
// KindNotFound for a path naming nothing, and KindWrongContainer for a step
// that does not apply to what it addresses -- including a path that names
// something no single node stands for: an array-of-tables (index it, or read
// its entries through Root) or a table implied by a longer header or a dotted
// key. Lookup and Has answer the same question without an error.
func (d *Document) Resolve(path string) (Node, error) {
	pos, err := d.resolvePos(path)
	if err != nil {
		return nil, d.diag(err, path)
	}
	node, ok := pos.concrete()
	if !ok {
		return nil, d.diag(pos.noNodeError(), path)
	}
	return node, nil
}

// Lookup returns the node the path names, and whether it names one. It is the
// comma-ok form of Resolve, and answers about CONCRETE nodes: a path naming
// something no single node stands for -- an array-of-tables, or a table implied
// by a longer header or a dotted key -- reports false, even though the
// read-layer carries it. Use Root to read those.
func (d *Document) Lookup(path string) (Node, bool) {
	pos, err := d.resolvePos(path)
	if err != nil {
		return nil, false
	}
	return pos.concrete()
}

// Has reports whether the path names a concrete node, on the same terms as
// Lookup.
func (d *Document) Has(path string) bool {
	_, ok := d.Lookup(path)
	return ok
}
