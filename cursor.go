package tomledit

import "time"

// Cursor provides a fluent, nil-safe API for navigating a TOML document.
// A Cursor is never nil. If navigation fails at any point, the cursor captures
// the error and all subsequent operations (Key, At, String, etc.) become no-ops
// that propagate the original error. Check Err after a chain of calls to see
// whether the traversal succeeded.
//
// A cursor navigates the read-layer, so Key and At step through compound tables
// and array-of-tables the same way a path does: Key crosses into a table
// however the document spells it, and At addresses an entry of an
// array-of-tables or an element of an array.
type Cursor struct {
	pos layerPos
	doc *Document
	err error
}

// Key returns a Cursor navigated to the named child of the document root.
// This is the entry point for the fluent cursor API. Chain additional Key or
// At calls to traverse deeper, then extract the value with String, Int, etc.
func (d *Document) Key(name string) *Cursor {
	root, err := foldDocument(d)
	if err != nil {
		return &Cursor{doc: d, err: err}
	}
	c := &Cursor{pos: posFromRecord(root), doc: d}
	return c.Key(name)
}

// Key navigates to a named child within the current scope.
func (c *Cursor) Key(name string) *Cursor {
	if c.err != nil {
		return c
	}
	pos, err := c.pos.key(name)
	if err != nil {
		return &Cursor{doc: c.doc, err: wrapError(err, "key %q", name)}
	}
	return &Cursor{pos: pos, doc: c.doc}
}

// At navigates to an array index. Supports negative indices.
func (c *Cursor) At(index int) *Cursor {
	if c.err != nil {
		return c
	}
	pos, err := c.pos.at(index)
	if err != nil {
		return &Cursor{doc: c.doc, err: wrapError(err, "index [%d]", index)}
	}
	return &Cursor{pos: pos, doc: c.doc}
}

// Node returns the node at the cursor's position, or nil if the cursor has an
// error. A position no single node stands for -- an array-of-tables, or a table
// implied by a longer header or a dotted key -- has no node to return: Node
// reports that through Err as a KindWrongContainer diagnostic and returns nil.
func (c *Cursor) Node() Node {
	if c.err != nil {
		return nil
	}
	node, ok := c.pos.concrete()
	if !ok {
		c.err = c.pos.noNodeError()
		return nil
	}
	return node
}

// Err returns the first error encountered during navigation, as an *Error
// naming the document's file when it has one.
func (c *Cursor) Err() error {
	return c.doc.diag(c.err, "")
}

// String extracts a string value from the current node.
// Returns ("", false) if the cursor has an error or the node is not a string.
func (c *Cursor) String() (string, bool) {
	if c.err != nil {
		return "", false
	}
	if s, ok := c.pos.node.(*StringNode); ok {
		return s.val.get(), true
	}
	return "", false
}

// Int extracts an integer value from the current node.
// Returns (0, false) if the cursor has an error or the node is not an integer.
func (c *Cursor) Int() (int64, bool) {
	if c.err != nil {
		return 0, false
	}
	if n, ok := c.pos.node.(*IntegerNode); ok {
		return n.val.get(), true
	}
	return 0, false
}

// Bool extracts a boolean value from the current node.
// Returns (false, false) if the cursor has an error or the node is not a boolean.
func (c *Cursor) Bool() (bool, bool) {
	if c.err != nil {
		return false, false
	}
	if b, ok := c.pos.node.(*BooleanNode); ok {
		return b.val.get(), true
	}
	return false, false
}

// Float extracts a float64 value from the current node.
// Returns (0, false) if the cursor has an error or the node is not a float.
func (c *Cursor) Float() (float64, bool) {
	if c.err != nil {
		return 0, false
	}
	if f, ok := c.pos.node.(*FloatNode); ok {
		return f.val.get(), true
	}
	return 0, false
}

// Time extracts a time.Time value from the current node.
// Returns (time.Time{}, false) if the cursor has an error or the node is not an offset date-time.
func (c *Cursor) Time() (time.Time, bool) {
	if c.err != nil {
		return time.Time{}, false
	}
	if dt, ok := c.pos.node.(*DateTimeNode); ok {
		return dt.val.get(), true
	}
	return time.Time{}, false
}
