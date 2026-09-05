package tomledit

import "testing"

const iterateTestDocument = `title = "Test"
tags = [1, 2, 3]

[server]
host = "localhost"
port = 8080

[[products]]
name = "Widget"
price = 9.99

[[products]]
name = "Gadget"
price = 19.99

[[products]]
name = "Doohickey"
price = 29.99
`

func parseIterateTestDoc(t *testing.T) *Document {
	t.Helper()
	doc, err := Parse([]byte(iterateTestDocument))
	if err != nil {
		t.Fatalf("failed to parse test document: %v", err)
	}
	return doc
}

// cursorAt walks a path with the Cursor, one segment at a time. The document
// carries no path-based Items or Len of its own; a caller who wants to iterate
// what a path names walks to it and asks the position, which is what this does
// once so the tests below read as they did.
func cursorAt(t *testing.T, doc *Document, path string) *Cursor {
	t.Helper()
	segments, err := ParsePath(path)
	if err != nil {
		t.Fatalf("parsing the path %q: %v", path, err)
	}
	if len(segments) == 0 {
		t.Fatalf("the path %q names nothing to walk to", path)
	}
	var c *Cursor
	for i, seg := range segments {
		switch {
		case seg.Kind == SegmentIndex && i == 0:
			t.Fatalf("the path %q starts with an index; a document is not indexable", path)
		case seg.Kind == SegmentIndex:
			c = c.At(seg.Index)
		case i == 0:
			c = doc.Key(seg.Key)
		default:
			c = c.Key(seg.Key)
		}
	}
	return c
}

// Fails if an array-of-tables stops being countable through the read-layer:
// the entry holds the records, and how many there are is how long that slice
// is. This is the replacement for the deleted path-based Len over a collection.
func TestReadLayer_CollectionLength(t *testing.T) {
	doc := parseIterateTestDoc(t)
	entry, ok := doc.Root().Get("products")
	if !ok {
		t.Fatal("the root record does not carry \"products\"")
	}
	if entry.Kind() != EntryRecords {
		t.Fatalf("\"products\" is a %v, want an array-of-tables", entry.Kind())
	}
	records, ok := entry.Records()
	if !ok {
		t.Fatal("the entry did not hand out its records")
	}
	if len(records) != 3 {
		t.Errorf("the collection holds %d entries, want 3", len(records))
	}
	for i, rec := range records {
		node, ok := rec.Node()
		if !ok {
			t.Fatalf("entry %d has no concrete node", i)
		}
		if node.Type() != NodeArrayTable {
			t.Errorf("entry %d is a %s, want an ArrayTable", i, node.Type())
		}
	}
}

// Fails if a plain array stops being countable through its own node: the array
// node hands out its elements, and how many there are is how long that slice
// is. This is the replacement for the deleted path-based Len over an array.
func TestReadLayer_ArrayLength(t *testing.T) {
	doc := parseIterateTestDoc(t)
	node, ok := doc.Lookup("tags")
	if !ok {
		t.Fatal("\"tags\" names no node")
	}
	arr, ok := node.(*ArrayNode)
	if !ok {
		t.Fatalf("\"tags\" is a %T, want an *ArrayNode", node)
	}
	if got := len(arr.Elements()); got != 3 {
		t.Errorf("the array holds %d elements, want 3", got)
	}
}

// Fails if the entries of a table stop being enumerable in first-appearance
// order through the read-layer. This is the replacement for the deleted
// path-based Items over a table.
func TestReadLayer_EntryEnumeration(t *testing.T) {
	doc := parseIterateTestDoc(t)
	entry, ok := doc.Root().Get("server")
	if !ok {
		t.Fatal("the root record does not carry \"server\"")
	}
	rec, ok := entry.Record()
	if !ok {
		t.Fatal("\"server\" is not a table")
	}
	var keys []string
	for e := range rec.Entries() {
		keys = append(keys, e.Key())
	}
	want := []string{"host", "port"}
	if len(keys) != len(want) {
		t.Fatalf("the table enumerated %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("the table enumerated %v, want %v", keys, want)
		}
	}
}

// Test 1: Items("products") iterates over [[products]] entries
func TestItems_ArrayOfTables(t *testing.T) {
	doc := parseIterateTestDoc(t)
	count := 0
	for i, node := range cursorAt(t, doc, "products").Items() {
		if i != count {
			t.Errorf("expected index %d, got %d", count, i)
		}
		if node == nil {
			t.Errorf("node at index %d is nil", i)
		}
		if node.Type() != NodeArrayTable {
			t.Errorf("expected ArrayTable node, got %s", node.Type())
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 products, got %d", count)
	}
}

// Test 2: Items("tags") iterates over inline array [1, 2, 3]
func TestItems_InlineArray(t *testing.T) {
	doc := parseIterateTestDoc(t)
	count := 0
	for i, node := range cursorAt(t, doc, "tags").Items() {
		if i != count {
			t.Errorf("expected index %d, got %d", count, i)
		}
		if node == nil {
			t.Errorf("node at index %d is nil", i)
		}
		if node.Type() != NodeInteger {
			t.Errorf("expected Integer node, got %s", node.Type())
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 tags, got %d", count)
	}
}

// Test 3: Items("nonexistent") yields nothing
func TestItems_Nonexistent(t *testing.T) {
	doc := parseIterateTestDoc(t)
	count := 0
	for range cursorAt(t, doc, "nonexistent").Items() {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 items for nonexistent path, got %d", count)
	}
}

// Test 4: Items("server.host") on a scalar yields nothing
func TestItems_Scalar(t *testing.T) {
	doc := parseIterateTestDoc(t)
	count := 0
	for range cursorAt(t, doc, "server.host").Items() {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 items for scalar path, got %d", count)
	}
}

// Test 5: Len("products") returns correct count
func TestLen_ArrayOfTables(t *testing.T) {
	doc := parseIterateTestDoc(t)
	n := cursorAt(t, doc, "products").Len()
	if n != 3 {
		t.Errorf("expected Len 3, got %d", n)
	}
}

// Test 6: Len("tags") returns correct count
func TestLen_InlineArray(t *testing.T) {
	doc := parseIterateTestDoc(t)
	n := cursorAt(t, doc, "tags").Len()
	if n != 3 {
		t.Errorf("expected Len 3, got %d", n)
	}
}

// Test 7: Len("nonexistent") returns -1
func TestLen_Nonexistent(t *testing.T) {
	doc := parseIterateTestDoc(t)
	n := cursorAt(t, doc, "nonexistent").Len()
	if n != -1 {
		t.Errorf("expected Len -1, got %d", n)
	}
}

// Test 8: Len("server.host") returns -1
func TestLen_Scalar(t *testing.T) {
	doc := parseIterateTestDoc(t)
	n := cursorAt(t, doc, "server.host").Len()
	if n != -1 {
		t.Errorf("expected Len -1, got %d", n)
	}
}

// Test 9: Cursor Items works
func TestCursor_Items(t *testing.T) {
	doc := parseIterateTestDoc(t)
	count := 0
	for i, node := range doc.Key("products").Items() {
		if i != count {
			t.Errorf("expected index %d, got %d", count, i)
		}
		if node == nil {
			t.Errorf("node at index %d is nil", i)
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 products via cursor, got %d", count)
	}
}

// Test 10: Cursor Len works
func TestCursor_Len(t *testing.T) {
	doc := parseIterateTestDoc(t)
	n := doc.Key("products").Len()
	if n != 3 {
		t.Errorf("expected Len 3, got %d", n)
	}
}

// Test 11: Error cursor Items yields nothing
func TestCursor_ErrorItems(t *testing.T) {
	doc := parseIterateTestDoc(t)
	count := 0
	for range doc.Key("missing").Items() {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 items for error cursor, got %d", count)
	}
}

// Test 12: Negative index within iteration
func TestItems_NegativeIndex(t *testing.T) {
	doc := parseIterateTestDoc(t)
	// Access the last product via negative index after iterating
	last := doc.Key("products").At(-1)
	name, err := last.Key("name").String()
	if err != nil {
		t.Fatal("could not get name from last product")
	}
	if name != "Doohickey" {
		t.Errorf("expected last product name \"Doohickey\", got %q", name)
	}
}

// Test 13: Collect all items into slice to verify count and values
func TestItems_CollectAll(t *testing.T) {
	doc := parseIterateTestDoc(t)
	var names []string
	for _, node := range cursorAt(t, doc, "products").Items() {
		at, ok := node.(*ArrayTableNode)
		if !ok {
			t.Fatalf("expected *ArrayTableNode, got %T", node)
		}
		// Find the "name" KV in the array table children
		for _, child := range at.children {
			if kv, ok := child.(*KeyValueNode); ok {
				if len(kv.key.parts) == 1 && kv.key.parts[0] == "name" {
					if s, ok := kv.val.(*StringNode); ok {
						names = append(names, s.val.get())
					}
				}
			}
		}
	}
	expected := []string{"Widget", "Gadget", "Doohickey"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("name[%d]: expected %q, got %q", i, expected[i], name)
		}
	}
}
