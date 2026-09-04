package tomledit

// Fragments: the byte ranges a rendered construct decomposes into.
//
// A construct that re-renders does not rebuild itself whole. It rebuilds the
// fragment the write touched and splices the original bytes of every other one,
// which is what makes a value write leave the line's spacing and comment alone,
// a comment write leave the value's spelling alone, and a write inside a
// container leave every sibling and separator alone.
//
// Two kinds of fragment state are held here: the decomposition of a dotted key
// (the bytes of each part, and the dots, whitespace and brackets around them),
// and the decomposition of a container (the bytes standing before, between and
// after its elements). Both are captured at parse time and dropped exactly when
// they stop describing the node -- there is no third state where they are kept
// but wrong.

// keyFragments holds a dotted key's byte decomposition: the bytes of each part,
// and the gaps around them, of which there is always one more than there are
// parts. A key built rather than parsed carries the same shape, with the
// canonical bytes of each part and the dots as its gaps.
type keyFragments struct {
	rawParts [][]byte
	gaps     [][]byte
}

// canonicalKeyFragments returns the fragments of a key written from scratch:
// each part in its canonical spelling, joined by dots with no whitespace.
func canonicalKeyFragments(parts []string) keyFragments {
	k := keyFragments{
		rawParts: make([][]byte, len(parts)),
		gaps:     make([][]byte, len(parts)+1),
	}
	for i, part := range parts {
		k.rawParts[i] = []byte(QuoteKey(part))
		if i > 0 {
			k.gaps[i] = []byte(".")
		}
	}
	return k
}

// describes reports whether the fragments still describe a key of n parts. They
// stop describing one only by never having been captured: every operation that
// changes a part keeps them in step.
func (k keyFragments) describes(n int) bool {
	return n > 0 && len(k.rawParts) == n && len(k.gaps) == n+1
}

// splice writes the key from its fragments.
func (k keyFragments) splice() []byte {
	var buf []byte
	for i, raw := range k.rawParts {
		buf = append(buf, k.gaps[i]...)
		buf = append(buf, raw...)
	}
	return append(buf, k.gaps[len(k.rawParts)]...)
}

// rename replaces one part's bytes with the canonical spelling of a new name,
// leaving every other part and every gap as it was. It is what makes a rename
// invalidate one fragment and nothing else.
func (k *keyFragments) rename(i int, name string) {
	if i >= 0 && i < len(k.rawParts) {
		k.rawParts[i] = []byte(QuoteKey(name))
	}
}

// buildPart records the bytes of one more part, and the gap that preceded it.
// The trailing gap is closed by buildKeyEnd.
func (k *keyFragments) buildPart(raw, gap []byte) {
	k.gaps = append(k.gaps, gap)
	k.rawParts = append(k.rawParts, raw)
}

// buildKeyEnd records the bytes standing after the last part.
func (k *keyFragments) buildKeyEnd(gap []byte) {
	k.gaps = append(k.gaps, gap)
}

// extendEnd adds more bytes to the gap after the last part, for a caller that
// learns them after the key itself was read -- a header's closing bracket.
func (k *keyFragments) extendEnd(b []byte) {
	if len(k.gaps) == 0 {
		return
	}
	last := len(k.gaps) - 1
	k.gaps[last] = append(append([]byte(nil), k.gaps[last]...), b...)
}

// --- container gaps ---

// containerGaps returns a container's gap fragments and whether they still
// describe it: one gap before every element and one after the last, so that
// splicing gap, element, gap, element ... reproduces the container's bytes.
func containerGaps(n Node) ([][]byte, bool) {
	switch c := n.(type) {
	case *ArrayNode:
		return c.gaps, len(c.gaps) == len(c.elements)+1
	case *InlineTableNode:
		return c.gaps, len(c.gaps) == len(c.children)+1
	}
	return nil, false
}

// dropGapsOf drops a container's gap fragments, which is what a change they
// cannot describe has to do: an element added or removed (the gaps no longer
// line up) or an element's own trivia written (the comment the write asks for
// has no fragment of its own -- it stood inside the gaps). The container then
// renders from its elements and their trivia instead.
//
// It is called with the node's parent, which for anything but an array element
// or an inline-table pair holds no gaps and is left alone.
func dropGapsOf(n Node) {
	switch c := n.(type) {
	case *ArrayNode:
		c.gaps = nil
	case *InlineTableNode:
		c.gaps = nil
	}
}
