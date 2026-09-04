package tomledit

import "iter"

// Items returns a range-over-func iterator over elements at the given path.
// Works with ArrayNode elements (inline arrays) and array-of-tables entries.
// Returns an empty iterator if the path is invalid, not found, or points to a
// non-array node. Use with a range loop:
//
//	for i, node := range doc.Items("servers") { ... }
func (d *Document) Items(path string) iter.Seq2[int, Node] {
	return func(yield func(int, Node) bool) {
		pos, err := d.resolvePos(path)
		if err != nil {
			return
		}
		iteratePos(pos, yield)
	}
}

// Len returns the number of elements at the path. Returns -1 if the path is
// invalid, does not exist, or does not point to an array or array-of-tables.
func (d *Document) Len(path string) int {
	pos, err := d.resolvePos(path)
	if err != nil {
		return -1
	}
	return posLen(pos)
}

// Items returns a range-over-func iterator over elements of the current node.
// Works with ArrayNode elements and array-of-tables entries.
// An inert cursor (with error) yields nothing.
func (c *Cursor) Items() iter.Seq2[int, Node] {
	return func(yield func(int, Node) bool) {
		if c.err != nil {
			return
		}
		iteratePos(c.pos, yield)
	}
}

// Len returns the number of elements at the current cursor position.
// Returns -1 if the cursor has an error or the node isn't an array/array-of-tables.
func (c *Cursor) Len() int {
	if c.err != nil {
		return -1
	}
	return posLen(c.pos)
}

// iteratePos yields the elements at a position: the entries of an
// array-of-tables, or the elements of an array node.
func iteratePos(pos layerPos, yield func(int, Node) bool) {
	if pos.records != nil {
		for i, rec := range pos.records {
			if !yield(i, rec.node) {
				return
			}
		}
		return
	}
	if arr, ok := pos.node.(*ArrayNode); ok {
		for i, elem := range arr.elements {
			if !yield(i, elem) {
				return
			}
		}
	}
}

// posLen returns the number of elements at a position, or -1 when it holds
// neither an array-of-tables nor an array.
func posLen(pos layerPos) int {
	if pos.records != nil {
		return len(pos.records)
	}
	if arr, ok := pos.node.(*ArrayNode); ok {
		return len(arr.elements)
	}
	return -1
}
