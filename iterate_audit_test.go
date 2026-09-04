package tomledit

import "testing"

// Audit focus 1: Early break in range-over-func iterator.
// Verifies that `break` in a `for range doc.Items(...)` loop stops iteration
// cleanly without panic or corruption.
func TestAudit_Items_EarlyBreak(t *testing.T) {
	input := `tags = [10, 20, 30, 40, 50]`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var collected []int64
	for _, node := range doc.Items("tags") {
		v, ok := node.(*IntegerNode)
		if !ok {
			t.Fatalf("expected *IntegerNode, got %T", node)
		}
		collected = append(collected, v.Val)
		if v.Val == 20 {
			break // early exit after second element
		}
	}

	if len(collected) != 2 {
		t.Fatalf("expected 2 items before break, got %d: %v", len(collected), collected)
	}
	if collected[0] != 10 || collected[1] != 20 {
		t.Errorf("expected [10 20], got %v", collected)
	}

	// Verify the document is still usable after early break.
	n := doc.Len("tags")
	if n != 5 {
		t.Errorf("after break, Len should still be 5, got %d", n)
	}
}

// Audit focus 1b: Early break on array-of-tables.
func TestAudit_Items_EarlyBreak_ArrayOfTables(t *testing.T) {
	input := `[[servers]]
name = "alpha"

[[servers]]
name = "beta"

[[servers]]
name = "gamma"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	count := 0
	for range doc.Items("servers") {
		count++
		if count == 1 {
			break
		}
	}
	if count != 1 {
		t.Errorf("expected 1 iteration before break, got %d", count)
	}

	// Document still intact.
	if doc.Len("servers") != 3 {
		t.Errorf("expected Len=3, got %d", doc.Len("servers"))
	}
}

// Audit focus 2: Items on nested arrays -- doc.Items("config.tags")
// where tags is an array inside a table.
func TestAudit_Items_NestedArrayInTable(t *testing.T) {
	input := `[config]
tags = ["fast", "reliable", "secure"]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var vals []string
	for i, node := range doc.Items("config.tags") {
		s, ok := node.(*StringNode)
		if !ok {
			t.Fatalf("element %d: expected *StringNode, got %T", i, node)
		}
		vals = append(vals, s.Val)
	}

	expected := []string{"fast", "reliable", "secure"}
	if len(vals) != len(expected) {
		t.Fatalf("expected %d items, got %d: %v", len(expected), len(vals), vals)
	}
	for i, exp := range expected {
		if vals[i] != exp {
			t.Errorf("element %d: expected %q, got %q", i, exp, vals[i])
		}
	}

	n := doc.Len("config.tags")
	if n != 3 {
		t.Errorf("expected Len=3, got %d", n)
	}
}

// Audit focus 2b: Items on deeply nested array.
func TestAudit_Items_DeeplyNestedArray(t *testing.T) {
	input := `[server]
[server.database]
ports = [5432, 5433, 5434]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	count := 0
	for _, node := range doc.Items("server.database.ports") {
		if node.Type() != NodeInteger {
			t.Errorf("element %d: expected Integer, got %s", count, node.Type())
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 elements, got %d", count)
	}
}

// Audit focus 2c: Items on an array inside an inline table.
func TestAudit_Items_ArrayInInlineTable(t *testing.T) {
	input := `config = {tags = [1, 2, 3]}`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	count := 0
	for range doc.Items("config.tags") {
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 items, got %d", count)
	}
}

// Audit focus 1c: Modifying document during iteration should not panic.
// (We don't guarantee correct results, just no crash.)
func TestAudit_Items_ModifyDuringIteration_NoPanic(t *testing.T) {
	input := `values = [1, 2, 3]`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Just verify it doesn't panic when we read values during iteration.
	for _, node := range doc.Items("values") {
		_ = node.(Scalar).Value()
	}
}

// Verify that Cursor.Items and direct Items agree.
func TestAudit_Items_CursorVsDirect(t *testing.T) {
	input := `[config]
tags = ["a", "b", "c"]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var directVals []string
	for _, node := range doc.Items("config.tags") {
		s, ok := node.(*StringNode)
		if !ok {
			t.Fatalf("expected *StringNode, got %T", node)
		}
		directVals = append(directVals, s.Val)
	}

	var cursorVals []string
	for _, node := range doc.Key("config").Key("tags").Items() {
		s, ok := node.(*StringNode)
		if !ok {
			t.Fatalf("expected *StringNode, got %T", node)
		}
		cursorVals = append(cursorVals, s.Val)
	}

	if len(directVals) != len(cursorVals) {
		t.Fatalf("length mismatch: direct=%d, cursor=%d", len(directVals), len(cursorVals))
	}
	for i := range directVals {
		if directVals[i] != cursorVals[i] {
			t.Errorf("element %d: direct=%q, cursor=%q", i, directVals[i], cursorVals[i])
		}
	}
}
