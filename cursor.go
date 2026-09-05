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
	root, err := d.readLayer()
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

// The terminals. Each reports the navigation failure the chain already carries,
// then reads the node the cursor stands on through the accessor stages of
// access.go -- so a terminal answers exactly what the path-level getter of the
// same type answers, for the same document position.

// String reads the value at the cursor as a string. It is deliberately not a
// fmt.Stringer: a cursor is a position, not a rendering of one.
//
// The error is the navigation failure that ended the chain, or
// KindTypeMismatch when the value is not a string.
func (c *Cursor) String() (string, error) { return cursorAs[string](c) }

// Int reads the value at the cursor as an int64.
//
// The error is the navigation failure that ended the chain, or
// KindTypeMismatch when the value is not an integer.
func (c *Cursor) Int() (int64, error) { return cursorAs[int64](c) }

// Bool reads the value at the cursor as a bool.
//
// The error is the navigation failure that ended the chain, or
// KindTypeMismatch when the value is not a boolean.
func (c *Cursor) Bool() (bool, error) { return cursorAs[bool](c) }

// Float reads the value at the cursor as a float64: a float verbatim, and an
// integer the target holds exactly.
//
// The error is the navigation failure that ended the chain, KindTypeMismatch
// when the value is neither a float nor an integer, or KindInexact for an
// integer no float64 holds exactly.
func (c *Cursor) Float() (float64, error) { return cursorAs[float64](c) }

// Time reads the value at the cursor as a time.Time: an offset date-time
// verbatim, and a local date-time or local date read as UTC.
//
// The error is the navigation failure that ended the chain, or
// KindTypeMismatch when the value is not one of those three flavors -- a string
// among them, even one spelling a valid RFC 3339 timestamp, which a time.Time
// decode target refuses too.
func (c *Cursor) Time() (time.Time, error) { return cursorAs[time.Time](c) }
