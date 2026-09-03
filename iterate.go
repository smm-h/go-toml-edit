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
		segments, err := parsePath(path)
		if err != nil {
			return
		}
		node, err := resolveNode(d, segments)
		if err != nil {
			return
		}
		iterateNode(node, yield)
	}
}

// Len returns the number of elements at the path. Returns -1 if the path is
// invalid, does not exist, or does not point to an array or array-of-tables.
func (d *Document) Len(path string) int {
	segments, err := parsePath(path)
	if err != nil {
		return -1
	}
	node, err := resolveNode(d, segments)
	if err != nil {
		return -1
	}
	return nodeLen(node)
}

// Items returns a range-over-func iterator over elements of the current node.
// Works with ArrayNode elements and array-of-tables entries.
// An inert cursor (with error) yields nothing.
func (c *Cursor) Items() iter.Seq2[int, Node] {
	return func(yield func(int, Node) bool) {
		if c.err != nil || c.node == nil {
			return
		}
		iterateNode(c.node, yield)
	}
}

// Len returns the number of elements at the current cursor position.
// Returns -1 if the cursor has an error or the node isn't an array/array-of-tables.
func (c *Cursor) Len() int {
	if c.err != nil || c.node == nil {
		return -1
	}
	return nodeLen(c.node)
}

// iterateNode yields elements from an array or array-of-tables node.
func iterateNode(node Node, yield func(int, Node) bool) {
	switch n := node.(type) {
	case *ArrayNode:
		for i, elem := range n.Elements {
			if !yield(i, elem) {
				return
			}
		}
	case *arrayTableCollection:
		for i, entry := range n.entries {
			if !yield(i, entry) {
				return
			}
		}
	}
}

// nodeLen returns the length of an array or array-of-tables node, or -1.
func nodeLen(node Node) int {
	switch n := node.(type) {
	case *ArrayNode:
		return len(n.Elements)
	case *arrayTableCollection:
		return len(n.entries)
	default:
		return -1
	}
}
