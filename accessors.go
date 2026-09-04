package tomledit

// The accessor block: how a node's contents are read now that its fields are
// unexported. Each accessor mirrors the field it replaced, and each one that
// answers with a slice answers with a copy -- sorting or truncating what a
// caller was handed changes nothing in the document.

// Children returns the document's top-level constructs in document order, as a
// copy.
func (n *Document) Children() []Node { return copyNodes(n.children) }

// KeyPath returns the parts of the header's key, decoded, as a copy.
func (n *TableNode) KeyPath() []string { return copyStrings(n.keyPath) }

// Children returns the constructs written under this header, in document
// order, as a copy.
func (n *TableNode) Children() []Node { return copyNodes(n.children) }

// KeyPath returns the parts of the header's key, decoded, as a copy.
func (n *ArrayTableNode) KeyPath() []string { return copyStrings(n.keyPath) }

// Children returns the constructs written under this header, in document
// order, as a copy.
func (n *ArrayTableNode) Children() []Node { return copyNodes(n.children) }

// Key returns the pair's key node.
func (n *KeyValueNode) Key() *KeyNode { return n.key }

// Val returns the pair's value node.
func (n *KeyValueNode) Val() Node { return n.val }

// Parts returns the key's parts, decoded, as a copy: "a.b" reads as
// ["a", "b"], and a quoted part carries no quotes.
func (n *KeyNode) Parts() []string { return copyStrings(n.parts) }

// RawParts returns the source bytes of each part, as written, as a copy: the
// outer slice and every part in it. It is empty for a key created
// programmatically.
func (n *KeyNode) RawParts() [][]byte { return copyByteSlices(n.frag.rawParts) }

// Styles returns the quoting style of each part, as a copy.
func (n *KeyNode) Styles() []StringStyle {
	out := make([]StringStyle, len(n.styles))
	copy(out, n.styles)
	return out
}

// Style returns the quoting style the string was written in.
func (n *StringNode) Style() StringStyle { return n.style }

// Base returns the numeric base the integer was written in.
func (n *IntegerNode) Base() IntegerBase { return n.base }

// Elements returns the array's elements in order, as a copy.
func (n *ArrayNode) Elements() []Node { return copyNodes(n.elements) }

// Children returns the inline table's pairs in order, as a copy.
func (n *InlineTableNode) Children() []Node { return copyNodes(n.children) }

// Text returns the comment line as it was written, including its "#" and its
// trailing newline. A comment node standing for a run of blank lines has no
// text of its own; Raw carries the blank bytes.
func (n *CommentNode) Text() string { return n.text }
