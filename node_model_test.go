package tomledit

import (
	"reflect"
	"testing"
	"time"
)

// The node model: what a node exposes, what it copies, and what happens when
// its payload is replaced.

// nodeImplementers is one value of every concrete node kind. A kind added to
// the package without an entry here is missed by the tests below, which is what
// the count assertion in TestNodeModel_EveryKindIsCovered guards.
func nodeImplementers() []Node {
	return []Node{
		&Document{},
		&TableNode{},
		&ArrayTableNode{},
		&KeyValueNode{},
		&KeyNode{},
		&StringNode{},
		&IntegerNode{},
		&FloatNode{},
		&BooleanNode{},
		&DateTimeNode{},
		&LocalDateTimeNode{},
		&LocalDateNode{},
		&LocalTimeNode{},
		&ArrayNode{},
		&InlineTableNode{},
		&CommentNode{},
	}
}

// Fails if a node kind is added without an entry in nodeImplementers, which
// would leave it out of every test in this file.
func TestNodeModel_EveryKindIsCovered(t *testing.T) {
	if got, want := len(nodeImplementers()), len(nodeTypeNames); got != want {
		t.Errorf("nodeImplementers lists %d kinds, the package names %d", got, want)
	}
	seen := map[NodeType]bool{}
	for _, n := range nodeImplementers() {
		if seen[n.Type()] {
			t.Errorf("two entries of nodeImplementers report %s", n.Type())
		}
		seen[n.Type()] = true
	}
}

// Fails if any node kind exposes a field: a node's contents are read through
// its accessors and written through the funnels of mutate.go, and an exported
// field is a way around both.
func TestNodeModel_NoExportedFields(t *testing.T) {
	for _, n := range nodeImplementers() {
		typ := reflect.TypeOf(n).Elem()
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.IsExported() {
				t.Errorf("%s exposes the field %s; unexport it and add an accessor",
					typ.Name(), f.Name)
			}
		}
	}
}

// Fails if an accessor hands out the node's own slice: a caller that sorts,
// truncates or overwrites what it was given would be editing the document
// through a read.
func TestNodeModel_AccessorsAnswerWithCopies(t *testing.T) {
	const src = "a.b = 1\n[t]\nx = [1, 2, 3]\ny = { p = 1, q = 2 }\n[[items]]\nn = 1\n"
	doc := parseOrFail(t, src)

	children := doc.Children()
	for i := range children {
		children[i] = nil
	}

	tbl := doc.Children()[1].(*TableNode)
	tblChildren := tbl.Children()
	for i := range tblChildren {
		tblChildren[i] = nil
	}
	keyPath := tbl.KeyPath()
	for i := range keyPath {
		keyPath[i] = "clobbered"
	}

	kv := tbl.Children()[0].(*KeyValueNode)
	arr := kv.Val().(*ArrayNode)
	elements := arr.Elements()
	for i := range elements {
		elements[i] = nil
	}

	inline := tbl.Children()[1].(*KeyValueNode).Val().(*InlineTableNode)
	inlineChildren := inline.Children()
	for i := range inlineChildren {
		inlineChildren[i] = nil
	}

	dotted := doc.Children()[0].(*KeyValueNode).Key()
	parts := dotted.Parts()
	for i := range parts {
		parts[i] = "clobbered"
	}
	raws := dotted.RawParts()
	for i := range raws {
		raws[i] = nil
	}
	styles := dotted.Styles()
	for i := range styles {
		styles[i] = StringMultiLineLiteral
	}

	atbl := doc.Children()[2].(*ArrayTableNode)
	atblChildren := atbl.Children()
	for i := range atblChildren {
		atblChildren[i] = nil
	}
	atblPath := atbl.KeyPath()
	for i := range atblPath {
		atblPath[i] = "clobbered"
	}

	if got := string(doc.Bytes()); got != src {
		t.Errorf("writing into what an accessor returned changed the document:\n  got:  %q\n  want: %q", got, src)
	}
}

// Fails if replacing a scalar's payload leaves its lexeme behind: the bytes the
// value was written as would then be spliced for a value the node no longer
// holds.
func TestNodeModel_PayloadWriteDropsTheLexeme(t *testing.T) {
	cases := []struct {
		name string
		src  string
		set  func(Node)
		want string
	}{
		{"string", "v = 'literal'\n", func(n Node) { n.(*StringNode).setValue("basic", StringBasic) }, "v = \"basic\"\n"},
		{"integer", "v = 0xFF\n", func(n Node) { n.(*IntegerNode).setValue(9, IntegerDecimal) }, "v = 9\n"},
		{"float", "v = 1e3\n", func(n Node) { n.(*FloatNode).setValue(2.5) }, "v = 2.5\n"},
		{"boolean", "v = true\n", func(n Node) { n.(*BooleanNode).setValue(false) }, "v = false\n"},
		{
			"offset date-time", "v = 1979-05-27T07:32:00Z\n",
			func(n Node) {
				n.(*DateTimeNode).setValue(time.Date(1980, 6, 28, 8, 33, 1, 0, time.UTC))
			},
			"v = 1980-06-28T08:33:01Z\n",
		},
		{
			"local date-time", "v = 1979-05-27T07:32:00\n",
			func(n Node) {
				n.(*LocalDateTimeNode).setValue(LocalDateTime{Year: 1980, Month: 6, Day: 28, Hour: 8, Minute: 33, Second: 1})
			},
			"v = 1980-06-28T08:33:01\n",
		},
		{
			"local date", "v = 1979-05-27\n",
			func(n Node) { n.(*LocalDateNode).setValue(LocalDate{Year: 1980, Month: 6, Day: 28}) },
			"v = 1980-06-28\n",
		},
		{
			"local time", "v = 07:32:00\n",
			func(n Node) { n.(*LocalTimeNode).setValue(LocalTime{Hour: 8, Minute: 33, Second: 1}) },
			"v = 08:33:01\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseOrFail(t, tc.src)
			node := doc.Children()[0].(*KeyValueNode).Val()
			if len(node.Raw()) == 0 {
				t.Fatalf("the parsed value carries no lexeme")
			}
			tc.set(node)
			if len(node.Raw()) != 0 {
				t.Errorf("the lexeme survived the payload write: %q", node.Raw())
			}
			if got := string(doc.Bytes()); got != tc.want {
				t.Errorf("the document reads %q, want %q", got, tc.want)
			}
		})
	}
}

// Fails if the dirtiness check stops being constant-time: the serializer asks
// every node whether anything at or below it still splices its original bytes,
// and the answer has to be one field read rather than a walk of the subtree.
// The two documents below hold the same number of nodes, one flat and one
// nested eight deep, and both are edited at their innermost value; a check that
// walks reads far more of the deep one than of the flat one.
func TestNodeModel_DirtinessCheckDoesNotWalkTheSubtree(t *testing.T) {
	probes := func(src string, edit func(*Document)) (int, int) {
		doc := parseOrFail(t, src)
		edit(doc)
		nodes := countNodes(doc)
		countingDirtyProbes = true
		dirtyProbes = 0
		doc.Bytes()
		countingDirtyProbes = false
		return dirtyProbes, nodes
	}

	innermost := func(n Node) *IntegerNode {
		for {
			arr, ok := n.(*ArrayNode)
			if !ok {
				return n.(*IntegerNode)
			}
			n = arr.Elements()[0]
		}
	}
	edit := func(d *Document) {
		kv := d.Children()[0].(*KeyValueNode)
		innermost(kv.Val()).setValue(99, IntegerDecimal)
	}

	flatProbes, flatNodes := probes("v = [1, 2, 3, 4, 5, 6, 7, 8]\n", edit)
	deepProbes, deepNodes := probes("v = [[[[[[[[1]]]]]]]]\n", edit)

	if flatNodes != deepNodes {
		t.Fatalf("the two documents hold %d and %d nodes; the comparison needs them equal",
			flatNodes, deepNodes)
	}
	if flatProbes != deepProbes {
		t.Errorf("the check reads %d nodes of the flat document and %d of the equally large nested one: it is walking the subtree",
			flatProbes, deepProbes)
	}
	if deepProbes > deepNodes {
		t.Errorf("the check reads %d nodes for a document of %d: it is reading more than one per node",
			deepProbes, deepNodes)
	}
}

// countNodes counts every node in a document, so the probe rate above is
// measured per node rather than per document.
func countNodes(n Node) int {
	total := 1
	switch c := n.(type) {
	case *Document:
		for _, child := range c.children {
			total += countNodes(child)
		}
	case *TableNode:
		for _, child := range c.children {
			total += countNodes(child)
		}
	case *ArrayTableNode:
		for _, child := range c.children {
			total += countNodes(child)
		}
	case *ArrayNode:
		for _, elem := range c.elements {
			total += countNodes(elem)
		}
	case *InlineTableNode:
		for _, child := range c.children {
			total += countNodes(child)
		}
	case *KeyValueNode:
		total += countNodes(c.key)
		total += countNodes(c.val)
	}
	return total
}

// Fails if a node that goes dirty stops saying so upward: the serializer reads
// one field per node, so an edit deep inside a container has to have reached
// every node above it by the time the render asks.
func TestNodeModel_DirtinessReachesEveryAncestor(t *testing.T) {
	doc := parseOrFail(t, "[t]\nv = [1, [2, 3]]\n")
	tbl := doc.Children()[0].(*TableNode)
	kv := tbl.Children()[0].(*KeyValueNode)
	outer := kv.Val().(*ArrayNode)
	inner := outer.Elements()[1].(*ArrayNode)
	deepest := inner.Elements()[0].(*IntegerNode)

	deepest.setValue(9, IntegerDecimal)

	for _, n := range []Node{inner, outer, kv, tbl, doc} {
		if !n.subtreeDirty() {
			t.Errorf("%s did not learn that a node under it went dirty", n.Type())
		}
	}
	// The table's own header bytes are untouched by a write to a value inside
	// it, which is what keeps the header spliced rather than re-rendered.
	if tbl.isDirty() {
		t.Errorf("the table header was invalidated by a write to a value under it")
	}
	if got, want := string(doc.Bytes()), "[t]\nv = [1, [9, 3]]\n"; got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
}

// Fails if a structural write stops invalidating the read-layer: the logical
// view folded from the old shape does not describe the document afterwards.
func TestNodeModel_StructuralWritesBumpTheGeneration(t *testing.T) {
	doc := parseOrFail(t, "a = 1\n")
	before := doc.generation
	if err := doc.Set("a", 2); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if doc.generation == before {
		t.Errorf("a write left the read-layer's generation at %d", before)
	}
}
