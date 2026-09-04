package tomledit

import "time"

// The write funnels.
//
// Every change to a parsed node goes through this file, and two source-walking
// tests hold that true: nothing outside it assigns to a scalar's payload or
// lexeme, and nothing outside it assigns to a container's contents or a key's
// parts. The reason is that each of those writes has a consequence the writer
// must not be able to forget:
//
//   - a payload write invalidates the lexeme, because the bytes the value was
//     written as no longer describe it;
//   - a structure write invalidates the read-layer, because the logical view of
//     the document is no longer the one that was folded;
//   - both mark the node, and every node above it, as no longer splicing its
//     original bytes.
//
// Construction is not mutation: a node built here or by the parser is fresh,
// and its payload and lexeme agree by construction. The guards therefore
// forbid assignment, not composite literals.

// scalarValue pairs a scalar's payload with its lexeme -- the exact source
// bytes the value was written as. They live in one struct because they must
// never disagree: set, the only thing that writes the payload, drops the lexeme
// in the same statement, so the bytes the serializer splices can never describe
// a value the node no longer holds. A scalar built programmatically carries no
// lexeme and renders canonically.
type scalarValue[T any] struct {
	payload T
	lexeme  []byte
}

// scalarOf returns the value of a scalar built rather than parsed: a payload
// with no lexeme, which renders canonically.
func scalarOf[T any](payload T) scalarValue[T] {
	return scalarValue[T]{payload: payload}
}

// lexed returns the value of a scalar read from source: a payload together with
// the bytes it was written as.
func lexed[T any](payload T, lexeme []byte) scalarValue[T] {
	return scalarValue[T]{payload: payload, lexeme: lexeme}
}

// get returns the payload.
func (s *scalarValue[T]) get() T { return s.payload }

// raw returns the lexeme, which is nil once the payload has been replaced.
func (s *scalarValue[T]) raw() []byte { return s.lexeme }

// set replaces the payload and drops the lexeme with it.
func (s *scalarValue[T]) set(v T) {
	s.payload = v
	s.lexeme = nil
}

// setLexeme records the bytes a parsed value was written as.
func (s *scalarValue[T]) setLexeme(b []byte) { s.lexeme = b }

// --- scalar payload writes ---
//
// One per scalar kind, each the only way to change that kind's payload: it
// replaces the value, drops the lexeme, and marks the node so the serializer
// re-renders it instead of splicing bytes that no longer apply.

func (n *StringNode) setValue(v string, style StringStyle) {
	n.val.set(v)
	n.style = style
	noteValueWrite(n)
}

func (n *IntegerNode) setValue(v int64, base IntegerBase) {
	n.val.set(v)
	n.base = base
	noteValueWrite(n)
}

func (n *FloatNode) setValue(v float64) {
	n.val.set(v)
	noteValueWrite(n)
}

func (n *BooleanNode) setValue(v bool) {
	n.val.set(v)
	noteValueWrite(n)
}

func (n *DateTimeNode) setValue(v time.Time) {
	n.val.set(v)
	noteValueWrite(n)
}

func (n *LocalDateTimeNode) setValue(v LocalDateTime) {
	n.val.set(v)
	noteValueWrite(n)
}

func (n *LocalDateNode) setValue(v LocalDate) {
	n.val.set(v)
	noteValueWrite(n)
}

func (n *LocalTimeNode) setValue(v LocalTime) {
	n.val.set(v)
	noteValueWrite(n)
}

// noteValueWrite records a payload replacement: the node's own bytes stop being
// valid, and the read-layer folded from the document's old values is dropped
// with them, so a record handed out before the write is never handed out again
// after it.
func noteValueWrite(n Node) {
	n.markDirty()
	invalidateLayer(n)
}

// --- the lexeme, at parse time ---
//
// A scalar's Raw is its lexeme, so the shared setRaw of nodeBase would write a
// field these kinds never read. Each of them records the bytes it was written
// as in the one place that holds them.

func (n *StringNode) Raw() []byte        { return copyBytes(n.val.raw()) }
func (n *IntegerNode) Raw() []byte       { return copyBytes(n.val.raw()) }
func (n *FloatNode) Raw() []byte         { return copyBytes(n.val.raw()) }
func (n *BooleanNode) Raw() []byte       { return copyBytes(n.val.raw()) }
func (n *DateTimeNode) Raw() []byte      { return copyBytes(n.val.raw()) }
func (n *LocalDateTimeNode) Raw() []byte { return copyBytes(n.val.raw()) }
func (n *LocalDateNode) Raw() []byte     { return copyBytes(n.val.raw()) }
func (n *LocalTimeNode) Raw() []byte     { return copyBytes(n.val.raw()) }

func (n *StringNode) rawBytes() []byte        { return n.val.raw() }
func (n *IntegerNode) rawBytes() []byte       { return n.val.raw() }
func (n *FloatNode) rawBytes() []byte         { return n.val.raw() }
func (n *BooleanNode) rawBytes() []byte       { return n.val.raw() }
func (n *DateTimeNode) rawBytes() []byte      { return n.val.raw() }
func (n *LocalDateTimeNode) rawBytes() []byte { return n.val.raw() }
func (n *LocalDateNode) rawBytes() []byte     { return n.val.raw() }
func (n *LocalTimeNode) rawBytes() []byte     { return n.val.raw() }

func (n *StringNode) setRaw(b []byte)        { n.val.setLexeme(b) }
func (n *IntegerNode) setRaw(b []byte)       { n.val.setLexeme(b) }
func (n *FloatNode) setRaw(b []byte)         { n.val.setLexeme(b) }
func (n *BooleanNode) setRaw(b []byte)       { n.val.setLexeme(b) }
func (n *DateTimeNode) setRaw(b []byte)      { n.val.setLexeme(b) }
func (n *LocalDateTimeNode) setRaw(b []byte) { n.val.setLexeme(b) }
func (n *LocalDateNode) setRaw(b []byte)     { n.val.setLexeme(b) }
func (n *LocalTimeNode) setRaw(b []byte)     { n.val.setLexeme(b) }

// --- parent links and dirtiness ---
//
// Every node knows the container it was put into, and the structure funnel is
// what puts it there. The link exists so that "does anything at or below this
// node still splice its original bytes?" is a single field read rather than a
// walk of the subtree: a node that goes dirty says so upward, once, at the
// moment it happens.

// setParent records the container a node was put into.
func (n *nodeBase) setParent(p Node) { n.parent = p }

// parentNode returns the container the node was put into, or nil for a root
// and for a node not yet attached to one.
func (n *nodeBase) parentNode() Node { return n.parent }

// setSubtreeDirty records that this node, or something under it, no longer
// splices its original bytes.
func (n *nodeBase) setSubtreeDirty() { n.subtree = true }

// markDirty records that this node's own bytes are no longer valid, and says so
// upward.
func (n *nodeBase) markDirty() {
	n.dirty = true
	n.subtree = true
	markSubtree(n.parent)
}

// markSubtree carries dirtiness up to the document. It stops at the first
// ancestor that already knows, whose own ancestors therefore know too.
func markSubtree(n Node) {
	for cur := n; cur != nil; cur = cur.parentNode() {
		if cur.subtreeDirty() {
			return
		}
		cur.setSubtreeDirty()
	}
}

// adopt records container as child's parent, and carries a child that arrives
// already dirty -- a node built for a write -- upward from its new position.
func adopt(container Node, child Node) {
	if child == nil {
		return
	}
	child.setParent(container)
	if child.subtreeDirty() {
		markSubtree(container)
	}
}

// --- container contents ---

// contents returns a concrete container's ordered contents. It is the reading
// half of the structure funnel: a caller changes the slice it is handed and
// gives it back to setContents, so nothing outside this file holds a pointer
// into a node.
func contents(n Node) ([]Node, error) {
	switch c := n.(type) {
	case *Document:
		return c.children, nil
	case *TableNode:
		return c.children, nil
	case *ArrayTableNode:
		return c.children, nil
	case *ArrayNode:
		return c.elements, nil
	case *InlineTableNode:
		return c.children, nil
	default:
		return nil, newError(KindWrongContainer,
			"%s nodes hold no children to reorder", n.Type())
	}
}

// rendersAsValue reports whether a container renders as ONE fragment, so that
// changing what it holds invalidates its own bytes. An array and an inline
// table do; a document, a table and an array-of-tables entry render their
// children in place, and their own bytes still stand.
func rendersAsValue(n Node) bool {
	switch n.(type) {
	case *ArrayNode, *InlineTableNode:
		return true
	}
	return false
}

// setContents replaces what a container holds. It is the only way a parsed
// container's contents change: it adopts every node it is given, invalidates
// the container's own bytes when the container renders as one fragment, says so
// upward, and drops the read-layer built from the shape it just changed.
func setContents(container Node, items []Node) error {
	if err := storeContents(container, items); err != nil {
		return err
	}
	for _, item := range items {
		adopt(container, item)
	}
	if rendersAsValue(container) {
		container.markDirty()
	} else {
		markSubtree(container)
	}
	invalidateLayer(container)
	return nil
}

// storeContents writes the contents into whichever field the container keeps
// them in.
func storeContents(container Node, items []Node) error {
	switch c := container.(type) {
	case *Document:
		c.children = items
	case *TableNode:
		c.children = items
	case *ArrayTableNode:
		c.children = items
	case *ArrayNode:
		c.elements = items
	case *InlineTableNode:
		c.children = items
	default:
		return newError(KindWrongContainer,
			"%s nodes hold no children to reorder", container.Type())
	}
	return nil
}

// appendContent adds one node to the end of a container's contents.
func appendContent(container Node, child Node) error {
	items, err := contents(container)
	if err != nil {
		return err
	}
	return setContents(container, append(append([]Node(nil), items...), child))
}

// insertContent adds one node at position at, which must be within the
// contents or just past their end.
func insertContent(container Node, at int, child Node) error {
	items, err := contents(container)
	if err != nil {
		return err
	}
	if at < 0 || at > len(items) {
		return newError(KindBadInput, "position %d is outside the container's %d children", at, len(items))
	}
	out := make([]Node, 0, len(items)+1)
	out = append(out, items[:at]...)
	out = append(out, child)
	out = append(out, items[at:]...)
	return setContents(container, out)
}

// removeContent removes the node at position at.
func removeContent(container Node, at int) error {
	items, err := contents(container)
	if err != nil {
		return err
	}
	if at < 0 || at >= len(items) {
		return newError(KindBadInput, "position %d is outside the container's %d children", at, len(items))
	}
	out := make([]Node, 0, len(items)-1)
	out = append(out, items[:at]...)
	out = append(out, items[at+1:]...)
	return setContents(container, out)
}

// --- pairs and keys ---

// setVal replaces the value a pair binds.
func (kv *KeyValueNode) setVal(v Node) {
	kv.val = v
	adopt(kv, v)
	kv.markDirty()
	invalidateLayer(kv)
}

// renamePart changes one part of a key in place. Only that part's bytes stop
// being valid; the rest of the key, and the construct around it, still splice.
func (k *KeyNode) renamePart(i int, name string) {
	if i < 0 || i >= len(k.parts) {
		return
	}
	k.parts[i] = name
	if i < len(k.rawParts) {
		k.rawParts[i] = []byte(name)
	}
	if i < len(k.styles) {
		k.styles[i] = StringBasic
	}
	k.markDirty()
	invalidateLayer(k)
}

// --- building, at parse time ---
//
// A node the parser is assembling has nothing to invalidate: its contents and
// its bytes are being written for the first time, and they agree. These are the
// funnel's construction half -- they record the parent link and nothing else.

// buildAppend adds a child to a container being built.
func buildAppend(container Node, child Node) {
	switch c := container.(type) {
	case *Document:
		c.children = append(c.children, child)
	case *TableNode:
		c.children = append(c.children, child)
	case *ArrayTableNode:
		c.children = append(c.children, child)
	case *ArrayNode:
		c.elements = append(c.elements, child)
	case *InlineTableNode:
		c.children = append(c.children, child)
	default:
		panic("tomledit: " + container.Type().String() + " nodes hold no children")
	}
	adopt(container, child)
}

// buildPart adds one part to a key being built.
func (k *KeyNode) buildPart(part string, raw []byte, style StringStyle) {
	k.parts = append(k.parts, part)
	k.rawParts = append(k.rawParts, raw)
	k.styles = append(k.styles, style)
}

// buildTrailingComments records the comments written after an array's last
// element, before its closing bracket.
func (a *ArrayNode) buildTrailingComments(lines [][]byte) {
	a.trailingComments = lines
}

// newPair returns a pair binding key to val, with both adopted.
func newPair(key *KeyNode, val Node) *KeyValueNode {
	kv := &KeyValueNode{key: key, val: val}
	adopt(kv, key)
	adopt(kv, val)
	return kv
}

// --- the read-layer's generation ---

// invalidateLayer tells the document a structural change happened, so the
// logical view folded from the old shape is no longer the document's view.
// Every structure write ends here; a node not attached to a document has no
// layer to drop.
func invalidateLayer(n Node) {
	if doc := documentOf(n); doc != nil {
		doc.bumpGeneration()
	}
}

// documentOf walks the parent links to the document a node belongs to, or nil
// when the node is not attached to one.
func documentOf(n Node) *Document {
	for cur := n; cur != nil; cur = cur.parentNode() {
		if doc, ok := cur.(*Document); ok {
			return doc
		}
	}
	return nil
}
