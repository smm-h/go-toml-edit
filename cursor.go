package tomledit

import "time"

// Cursor provides a fluent, nil-safe API for navigating a TOML document's AST.
// A Cursor is never nil. If navigation fails at any point, the cursor captures
// the error and all subsequent operations (Key, At, String, etc.) become no-ops
// that propagate the original error. Check Err after a chain of calls to see
// whether the traversal succeeded.
type Cursor struct {
	node Node
	doc  *Document
	err  error
	// tablePath tracks the logical table path for sub-table resolution.
	tablePath []string
}

// Key returns a Cursor navigated to the named child of the document root.
// This is the entry point for the fluent cursor API. Chain additional Key or
// At calls to traverse deeper, then extract the value with String, Int, etc.
func (d *Document) Key(name string) *Cursor {
	c := &Cursor{node: d, doc: d}
	return c.Key(name)
}

// Key navigates to a named child within the current scope.
func (c *Cursor) Key(name string) *Cursor {
	if c.err != nil {
		return c
	}
	if c.node == nil {
		return &Cursor{doc: c.doc, err: newError(KindNotFound, "cannot look up key %q: current node is nil", name)}
	}

	node, tablePath, err := resolveKeySegment(c.doc, c.node, c.tablePath, name, nil, 0)
	if err != nil {
		return &Cursor{doc: c.doc, err: wrapError(err, "key %q", name)}
	}
	return &Cursor{node: node, doc: c.doc, tablePath: tablePath}
}

// At navigates to an array index. Supports negative indices.
func (c *Cursor) At(index int) *Cursor {
	if c.err != nil {
		return c
	}
	if c.node == nil {
		return &Cursor{doc: c.doc, err: newError(KindNotFound, "cannot index [%d]: current node is nil", index)}
	}

	node, err := resolveIndexSegment(c.doc, c.node, c.tablePath, index)
	if err != nil {
		return &Cursor{doc: c.doc, err: wrapError(err, "index [%d]", index)}
	}
	return &Cursor{node: node, doc: c.doc, tablePath: c.tablePath}
}

// Node returns the current node, or nil if the cursor has an error.
func (c *Cursor) Node() Node {
	if c.err != nil {
		return nil
	}
	return c.node
}

// Err returns the first error encountered during navigation, as an *Error
// naming the document's file when it has one.
func (c *Cursor) Err() error {
	return c.doc.diag(c.err, "")
}

// String extracts a string value from the current node.
// Returns ("", false) if the cursor has an error or the node is not a string.
func (c *Cursor) String() (string, bool) {
	if c.err != nil || c.node == nil {
		return "", false
	}
	if s, ok := c.node.(*StringNode); ok {
		return s.Val, true
	}
	return "", false
}

// Int extracts an integer value from the current node.
// Returns (0, false) if the cursor has an error or the node is not an integer.
func (c *Cursor) Int() (int64, bool) {
	if c.err != nil || c.node == nil {
		return 0, false
	}
	if n, ok := c.node.(*IntegerNode); ok {
		return n.Val, true
	}
	return 0, false
}

// Bool extracts a boolean value from the current node.
// Returns (false, false) if the cursor has an error or the node is not a boolean.
func (c *Cursor) Bool() (bool, bool) {
	if c.err != nil || c.node == nil {
		return false, false
	}
	if b, ok := c.node.(*BooleanNode); ok {
		return b.Val, true
	}
	return false, false
}

// Float extracts a float64 value from the current node.
// Returns (0, false) if the cursor has an error or the node is not a float.
func (c *Cursor) Float() (float64, bool) {
	if c.err != nil || c.node == nil {
		return 0, false
	}
	if f, ok := c.node.(*FloatNode); ok {
		return f.Val, true
	}
	return 0, false
}

// Time extracts a time.Time value from the current node.
// Returns (time.Time{}, false) if the cursor has an error or the node is not an offset date-time.
func (c *Cursor) Time() (time.Time, bool) {
	if c.err != nil || c.node == nil {
		return time.Time{}, false
	}
	if dt, ok := c.node.(*DateTimeNode); ok {
		return dt.Val, true
	}
	return time.Time{}, false
}
