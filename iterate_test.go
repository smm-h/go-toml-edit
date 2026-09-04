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

// Test 1: Items("products") iterates over [[products]] entries
func TestItems_ArrayOfTables(t *testing.T) {
	doc := parseIterateTestDoc(t)
	count := 0
	for i, node := range doc.Items("products") {
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
	for i, node := range doc.Items("tags") {
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
	for range doc.Items("nonexistent") {
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
	for range doc.Items("server.host") {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 items for scalar path, got %d", count)
	}
}

// Test 5: Len("products") returns correct count
func TestLen_ArrayOfTables(t *testing.T) {
	doc := parseIterateTestDoc(t)
	n := doc.Len("products")
	if n != 3 {
		t.Errorf("expected Len 3, got %d", n)
	}
}

// Test 6: Len("tags") returns correct count
func TestLen_InlineArray(t *testing.T) {
	doc := parseIterateTestDoc(t)
	n := doc.Len("tags")
	if n != 3 {
		t.Errorf("expected Len 3, got %d", n)
	}
}

// Test 7: Len("nonexistent") returns -1
func TestLen_Nonexistent(t *testing.T) {
	doc := parseIterateTestDoc(t)
	n := doc.Len("nonexistent")
	if n != -1 {
		t.Errorf("expected Len -1, got %d", n)
	}
}

// Test 8: Len("server.host") returns -1
func TestLen_Scalar(t *testing.T) {
	doc := parseIterateTestDoc(t)
	n := doc.Len("server.host")
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
	for _, node := range doc.Items("products") {
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
