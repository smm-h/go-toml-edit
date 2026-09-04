package tomledit

import (
	"errors"
	"testing"
)

func TestCursor_ServerHost(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.Key("server").Key("host").String()
	if err != nil {
		t.Fatal("cursor server.host String() returned false")
	}
	if val != "localhost" {
		t.Errorf("expected \"localhost\", got %q", val)
	}
}

func TestCursor_ServerPort(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.Key("server").Key("port").Int()
	if err != nil {
		t.Fatal("cursor server.port Int() returned false")
	}
	if val != 8080 {
		t.Errorf("expected 8080, got %d", val)
	}
}

func TestCursor_Products0Name(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.Key("products").At(0).Key("name").String()
	if err != nil {
		t.Fatal("cursor products[0].name String() returned false")
	}
	if val != "Widget" {
		t.Errorf("expected \"Widget\", got %q", val)
	}
}

func TestCursor_ProductsNeg1Name(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.Key("products").At(-1).Key("name").String()
	if err != nil {
		t.Fatal("cursor products[-1].name String() returned false")
	}
	if val != "Gadget" {
		t.Errorf("expected \"Gadget\", got %q", val)
	}
}

func TestCursor_NestedInlineX(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.Key("nested").Key("inline").Key("x").Int()
	if err != nil {
		t.Fatal("cursor nested.inline.x Int() returned false")
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}
}

func TestCursor_NestedArray2(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.Key("nested").Key("array").At(2).Int()
	if err != nil {
		t.Fatal("cursor nested.array[2] Int() returned false")
	}
	if val != 3 {
		t.Errorf("expected 3, got %d", val)
	}
}

func TestCursor_MissingKey(t *testing.T) {
	doc := parseTestDoc(t)
	cursor := doc.Key("nonexistent")
	val, err := cursor.Key("x").String()
	if err == nil {
		t.Errorf("expected false for missing key, got (%q, true)", val)
	}
	if cursor.Err() == nil {
		t.Error("expected non-nil error for missing key")
	}
}

func TestCursor_ChainAfterError(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.Key("nonexistent").Key("x").Key("y").String()
	if err == nil {
		t.Errorf("expected false after chained error, got (%q, true)", val)
	}
	// Should not panic
}

func TestCursor_CrossTableResolution(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.Key("server").Key("database").Key("name").String()
	if err != nil {
		t.Fatal("cursor server.database.name String() returned false")
	}
	if val != "mydb" {
		t.Errorf("expected \"mydb\", got %q", val)
	}
}

func TestCursor_Node(t *testing.T) {
	doc := parseTestDoc(t)
	cursor := doc.Key("server").Key("host")
	node := cursor.Node()
	if node == nil {
		t.Fatal("Node() returned nil")
	}
	s, ok := node.(*StringNode)
	if !ok {
		t.Fatalf("expected *StringNode, got %T", node)
	}
	if s.val.get() != "localhost" {
		t.Errorf("expected \"localhost\", got %q", s.val.get())
	}
}

func TestCursor_ErrorMessage(t *testing.T) {
	doc := parseTestDoc(t)
	cursor := doc.Key("nonexistent")
	err := cursor.Err()
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("error message should not be empty")
	}
}

// The Cursor navigates the read-layer, so a chain crosses whatever spelling the
// document used -- and stops at a position no single node stands for.

// Fails if a cursor chain stops crossing a table the document spells with a
// longer header, or an array-of-tables entry, or a dotted key: the layer makes
// all three ordinary Key steps.
func TestCursor_NavigatesEverySpelling(t *testing.T) {
	doc, err := Parse([]byte(resolveTestDoc))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got, err := doc.Key("compound").Key("leaf").Key("m").Int(); err != nil || got != 4 {
		t.Errorf("compound.leaf.m = (%d, %v), want 4", got, err)
	}
	if got, err := doc.Key("dotted").Key("deep").Int(); err != nil || got != 2 {
		t.Errorf("dotted.deep = (%d, %v), want 2", got, err)
	}
	if got, err := doc.Key("coll").At(1).Key("name").String(); err != nil || got != "second" {
		t.Errorf("coll[1].name = (%q, %v), want %q", got, err, "second")
	}
	if got, err := doc.Key("inline").Key("x").Int(); err != nil || got != 3 {
		t.Errorf("inline.x = (%d, %v), want 3", got, err)
	}
}

// Fails if a cursor terminal at a position with no concrete node -- an
// array-of-tables, a table implied by a longer header -- starts answering with
// a node instead of reporting the refusal through Err.
func TestCursor_NodeAtALogicalOnlyPosition(t *testing.T) {
	doc, err := Parse([]byte(resolveTestDoc))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	for _, key := range []string{"coll", "compound", "dotted"} {
		c := doc.Key(key)
		if c.Err() != nil {
			t.Errorf("navigating to %q failed: %v", key, c.Err())
			continue
		}
		if node := c.Node(); node != nil {
			t.Errorf("%s Node() = %T, want nil", key, node)
		}
		if !errors.Is(c.Err(), ErrWrongContainer) {
			t.Errorf("%s Err() = %v, want a wrong-container diagnostic", key, c.Err())
		}
	}
}

// Fails if the cursor's own iteration stops reading an array-of-tables through
// the layer.
func TestCursor_ItemsOverACollection(t *testing.T) {
	doc, err := Parse([]byte(resolveTestDoc))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	c := doc.Key("coll")
	if got := c.Len(); got != 2 {
		t.Errorf("Len over the collection = %d, want 2", got)
	}
	seen := 0
	for _, node := range c.Items() {
		if _, ok := node.(*ArrayTableNode); !ok {
			t.Errorf("entry is a %T, want an *ArrayTableNode", node)
		}
		seen++
	}
	if seen != 2 {
		t.Errorf("Items yielded %d entries, want 2", seen)
	}
}

// Fails if a cursor stops refusing a step that does not apply, or stops
// carrying the first failure to the end of the chain.
func TestCursor_RefusalsCarryTheirKind(t *testing.T) {
	doc, err := Parse([]byte(resolveTestDoc))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := doc.Key("coll").Key("name").Err(); !errors.Is(err, ErrWrongContainer) {
		t.Errorf("a key on a collection = %v, want a wrong-container diagnostic", err)
	}
	if err := doc.Key("top").At(0).Err(); !errors.Is(err, ErrWrongContainer) {
		t.Errorf("an index into a scalar = %v, want a wrong-container diagnostic", err)
	}
	if err := doc.Key("missing").Key("deeper").At(3).Err(); !errors.Is(err, ErrNotFound) {
		t.Errorf("a chain after a missing key = %v, want the first failure", err)
	}
}
