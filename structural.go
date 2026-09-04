package tomledit

// The structural operations: edits that change what a container holds and in
// what order, rather than what a value says. They address CONCRETE containers
// -- the document, a [header] table, an array-of-tables entry, an array, an
// inline table -- because a position no single node stands for has no children
// of its own to reorder or resize; such a path is refused with
// KindWrongContainer, the same answer every concrete-node surface gives.

// PermuteChildren reorders the children of the container the path names. The
// empty path addresses the document's own children.
//
// The order is a GATHER: order[i] is the index of the child that moves to
// position i, so children [A, B] with order [1, 0] end up [B, A]. It must be
// total -- a permutation of every index the container has, each exactly once.
// A wrong length, an out-of-range index and a repeated one are each refused
// with KindBadInput, and nothing is reordered. The two index violations name
// the offending index; a length mismatch is about the order as a whole and
// names both counts instead.
//
// The indices address the container's children as they stand right now. Read
// them, compute the order and permute in one editing sequence: an edit in
// between that adds or removes a child shifts every index after it, and the
// permutation would then move the wrong nodes.
//
// A child's own trivia travels with it -- its leading comments, its inline
// comment and the blank lines before it are part of the child, not of the
// position it used to hold.
func (d *Document) PermuteChildren(path string, order []int) error {
	return d.diag(d.permuteChildren(path, order), path)
}

func (d *Document) permuteChildren(path string, order []int) error {
	node, err := d.containerNodeAt(path)
	if err != nil {
		return err
	}
	children, err := contents(node)
	if err != nil {
		return err
	}
	permuted, err := permuteNodes(children, order)
	if err != nil {
		return err
	}
	return setContents(node, permuted)
}

// AppendToArray appends a value to the array the path names.
//
// The value is converted the way Set converts one: the Go types listed there,
// including []Pair for an ordered inline table. An array-of-tables is not an
// array value -- add an entry to one with NewArrayTable.
func (d *Document) AppendToArray(path string, value any) error {
	return d.diag(d.appendToArray(path, value), path)
}

func (d *Document) appendToArray(path string, value any) error {
	arr, err := d.arrayAt(path)
	if err != nil {
		return err
	}
	valNode, err := valueToNode(value)
	if err != nil {
		return err
	}
	return appendContent(arr, valNode)
}

// RemoveFromArray removes the element at index from the array the path names.
// A negative index counts from the end, so -1 removes the last element.
//
// Unlike Delete, which is idempotent by contract, this operation names a
// position that must exist: an index outside the array is refused with
// KindNotFound.
func (d *Document) RemoveFromArray(path string, index int) error {
	return d.diag(d.removeFromArray(path, index), path)
}

func (d *Document) removeFromArray(path string, index int) error {
	arr, err := d.arrayAt(path)
	if err != nil {
		return err
	}
	idx, err := normalizeIndex(index, len(arr.elements))
	if err != nil {
		return err
	}
	return removeContent(arr, idx)
}

// containerNodeAt resolves a path to the concrete node that stands for it. The
// empty path names the document itself, which is the container of its own
// top-level children.
func (d *Document) containerNodeAt(path string) (Node, error) {
	if path == "" {
		return d, nil
	}
	pos, err := d.resolvePos(path)
	if err != nil {
		return nil, err
	}
	node, ok := pos.concrete()
	if !ok {
		return nil, pos.noNodeError()
	}
	return node, nil
}

// arrayAt resolves a path to the array node it names, refusing everything else
// -- including an array-of-tables, which is a collection of table entries and
// not an array value.
func (d *Document) arrayAt(path string) (*ArrayNode, error) {
	pos, err := d.resolvePos(path)
	if err != nil {
		return nil, err
	}
	if pos.records != nil {
		return nil, newError(KindWrongContainer,
			"an array-of-tables is not an array value: add an entry with NewArrayTable, or remove one with Delete")
	}
	if arr, ok := pos.node.(*ArrayNode); ok {
		return arr, nil
	}
	if pos.node == nil {
		return nil, pos.noNodeError()
	}
	return nil, newError(KindWrongContainer, "%s is not an array", pos.describe())
}

// permuteNodes applies a gather permutation, returning the reordered children.
// It builds a new slice, so a permutation it refuses leaves the container's
// own children untouched.
func permuteNodes(children []Node, order []int) ([]Node, error) {
	if len(order) != len(children) {
		return nil, newError(KindBadInput,
			"the permutation names %d children, the container holds %d: every child must appear exactly once",
			len(order), len(children))
	}
	out := make([]Node, len(children))
	seen := make([]bool, len(children))
	for i, from := range order {
		if from < 0 || from >= len(children) {
			return nil, newError(KindBadInput,
				"index %d at position %d is out of range: the container holds %d children",
				from, i, len(children)).withValue(from)
		}
		if seen[from] {
			return nil, newError(KindBadInput,
				"index %d appears more than once in the permutation: every child must appear exactly once",
				from).withValue(from)
		}
		seen[from] = true
		out[i] = children[from]
	}
	return out, nil
}
