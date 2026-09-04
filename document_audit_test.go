package tomledit

import (
	"errors"
	"strings"
	"testing"
)

// --- Cross-table resolution ---

func TestAudit_DeepCrossTableResolution(t *testing.T) {
	// [a] then [a.b] then [a.b.c] -- resolve a.b.c.key
	input := `
[a]
x = 1

[a.b]
y = 2

[a.b.c]
z = 3
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// a.b.c.z should resolve
	val, err := doc.GetInt("a.b.c.z")
	if err != nil {
		t.Fatalf("GetInt(\"a.b.c.z\"): %v", err)
	}
	if val != 3 {
		t.Errorf("expected 3, got %d", val)
	}

	// a.b.y should resolve
	val, err = doc.GetInt("a.b.y")
	if err != nil {
		t.Fatalf("GetInt(\"a.b.y\"): %v", err)
	}
	if val != 2 {
		t.Errorf("expected 2, got %d", val)
	}

	// a.x should resolve
	val, err = doc.GetInt("a.x")
	if err != nil {
		t.Fatalf("GetInt(\"a.x\"): %v", err)
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}
}

func TestAudit_MixedDottedKeysAndTables(t *testing.T) {
	// [a] with b.c = val -- resolve a.b.c
	input := `
[a]
b.c = "hello"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	val, err := doc.GetString("a.b.c")
	if err != nil {
		t.Fatalf("GetString(\"a.b.c\"): %v", err)
	}
	if val != "hello" {
		t.Errorf("expected \"hello\", got %q", val)
	}
}

func TestAudit_ArrayOfTablesWithSubTables(t *testing.T) {
	// [[products]] with [products.details]
	input := `
[[products]]
name = "Widget"

[products.details]
color = "red"

[[products]]
name = "Gadget"

[products.details]
color = "blue"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// products[0].name
	val, err := doc.GetString("products[0].name")
	if err != nil {
		t.Fatalf("GetString(\"products[0].name\"): %v", err)
	}
	if val != "Widget" {
		t.Errorf("expected \"Widget\", got %q", val)
	}

	// products[0].details.color
	val, err = doc.GetString("products[0].details.color")
	if err != nil {
		t.Fatalf("GetString(\"products[0].details.color\"): %v", err)
	}
	if val != "red" {
		t.Errorf("expected \"red\", got %q", val)
	}

	// products[1].details.color
	val, err = doc.GetString("products[1].details.color")
	if err != nil {
		t.Fatalf("GetString(\"products[1].details.color\"): %v", err)
	}
	if val != "blue" {
		t.Errorf("expected \"blue\", got %q", val)
	}
}

// --- Edge cases ---

func TestAudit_EmptyPathGet(t *testing.T) {
	doc, err := Parse([]byte(`key = "val"`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Get with empty path should return nil (ParsePath returns error for "")
	node, ok := doc.Lookup("")
	if ok {
		t.Errorf("Lookup(\"\") should return nil, got %T", node)
	}
}

func TestAudit_EmptyPathResolve(t *testing.T) {
	doc, err := Parse([]byte(`key = "val"`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Resolve with empty path should return a bad-path diagnostic
	_, resolveErr := doc.Resolve("")
	if resolveErr == nil {
		t.Fatal("Resolve(\"\") should return error")
	}
	if !errors.Is(resolveErr, ErrBadPath) {
		t.Errorf("expected a bad-path diagnostic, got: %v", resolveErr)
	}
}

func TestAudit_PathToTable(t *testing.T) {
	// What does Get return for a path pointing to a table (not a value)?
	input := `
[server]
host = "localhost"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Lookup("server") should answer with the TableNode
	node, ok := doc.Lookup("server")
	if !ok {
		t.Fatal("Lookup(\"server\") returned nil")
	}
	if _, ok := node.(*TableNode); !ok {
		t.Errorf("expected *TableNode, got %T", node)
	}
}

func TestAudit_PathToDocumentRoot(t *testing.T) {
	// ParsePath("") returns error, so there's no way to request the root via Get
	// But Resolve with empty string should error cleanly
	doc, err := Parse([]byte(`key = "val"`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	_, resolveErr := doc.Resolve("")
	if resolveErr == nil {
		t.Fatal("Resolve(\"\") should return error for empty path")
	}
}

func TestAudit_IndexOutOfBounds(t *testing.T) {
	input := `
[[items]]
name = "a"

[[items]]
name = "b"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Index 5 out of bounds
	node, ok := doc.Lookup("items[5]")
	if ok {
		t.Errorf("Lookup(\"items[5]\") should return nil for out-of-bounds, got %T", node)
	}

	// Resolve should give proper error
	_, resolveErr := doc.Resolve("items[5]")
	if resolveErr == nil {
		t.Fatal("Resolve(\"items[5]\") should return error")
	}
}

func TestAudit_NegativeIndexOutOfBounds(t *testing.T) {
	input := `
[[items]]
name = "a"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// -2 on length-1 array
	node, ok := doc.Lookup("items[-2]")
	if ok {
		t.Errorf("Lookup(\"items[-2]\") should return nil for out-of-bounds, got %T", node)
	}
}

func TestAudit_NegativeIndexOnEmptyArray(t *testing.T) {
	input := `
[section]
arr = []
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	node, ok := doc.Lookup("section.arr[-1]")
	if ok {
		t.Errorf("Lookup(\"section.arr[-1]\") should return nil for empty array, got %T", node)
	}

	_, resolveErr := doc.Resolve("section.arr[-1]")
	if resolveErr == nil {
		t.Fatal("Resolve(\"section.arr[-1]\") should return error for empty array")
	}
	if !strings.Contains(resolveErr.Error(), "empty collection") {
		t.Errorf("expected 'empty collection' in error, got: %v", resolveErr)
	}
}

func TestAudit_DeeplyNested(t *testing.T) {
	input := `
[a]
[a.b]
[a.b.c]
[a.b.c.d]
[a.b.c.d.e]
[a.b.c.d.e.f]
val = 42
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	val, err := doc.GetInt("a.b.c.d.e.f.val")
	if err != nil {
		t.Fatalf("GetInt(\"a.b.c.d.e.f.val\"): %v", err)
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
}

// --- Type mismatches in getters ---

func TestAudit_GetStringOnInt(t *testing.T) {
	doc, err := Parse([]byte(`val = 42`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	val, err := doc.GetString("val")
	if err == nil {
		t.Errorf("GetString on an integer reported no error, and read %q", val)
	}
	if val != "" {
		t.Errorf("expected empty string, got %q", val)
	}
}

func TestAudit_GetIntOnString(t *testing.T) {
	doc, err := Parse([]byte(`val = "hello"`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	val, err := doc.GetInt("val")
	if err == nil {
		t.Errorf("GetInt on a string reported no error, and read %d", val)
	}
	if val != 0 {
		t.Errorf("expected 0, got %d", val)
	}
}

func TestAudit_GetBoolOnString(t *testing.T) {
	doc, err := Parse([]byte(`val = "true"`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	val, err := doc.GetBool("val")
	if err == nil {
		t.Errorf("GetBool on a string reported no error")
	}
	if val != false {
		t.Errorf("expected false, got %v", val)
	}
}

func TestAudit_GetFloatOnBool(t *testing.T) {
	doc, err := Parse([]byte(`val = true`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	val, err := doc.GetFloat("val")
	if err == nil {
		t.Errorf("GetFloat on a boolean reported no error")
	}
	if val != 0 {
		t.Errorf("expected 0, got %f", val)
	}
}

func TestAudit_GetTimeOnString(t *testing.T) {
	doc, err := Parse([]byte(`val = "2024-01-01"`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = doc.GetTime("val")
	if err == nil {
		t.Error("GetTime on a string reported no error")
	}
}

func TestAudit_GetIntOnFloat(t *testing.T) {
	doc, err := Parse([]byte(`val = 3.14`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	val, err := doc.GetInt("val")
	if err == nil {
		t.Errorf("GetInt on a float reported no error, and read %d", val)
	}
}

// Fails if GetFloat stops reading the conversion table's integer-into-float
// row: an integer a float64 holds exactly is a float64, and one it does not is
// an inexact-value diagnostic rather than a silent answer.
func TestAudit_GetFloatOnInt(t *testing.T) {
	doc, err := Parse([]byte("exact = 42\ninexact = 9007199254740993\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	val, err := doc.GetFloat("exact")
	if err != nil || val != 42 {
		t.Errorf("GetFloat on an exactly representable integer: (%f, %v)", val, err)
	}
	if _, err := doc.GetFloat("inexact"); !errors.Is(err, ErrInexact) {
		t.Errorf("GetFloat on an integer no float64 holds exactly = %v, want an inexact-value diagnostic", err)
	}
}

// --- Resolve error details ---

func TestAudit_ResolveDistinguishesPathSyntaxVsResolution(t *testing.T) {
	doc, err := Parse([]byte(`key = "val"`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Bad syntax: a path that does not parse.
	_, syntaxErr := doc.Resolve("a[")
	if syntaxErr == nil {
		t.Fatal("expected error for bad syntax")
	}
	if !errors.Is(syntaxErr, ErrBadPath) {
		t.Errorf("expected a bad-path diagnostic, got: %v", syntaxErr)
	}

	// Valid syntax, missing key: a resolution failure, a different kind.
	_, resErr := doc.Resolve("nonexistent")
	if resErr == nil {
		t.Fatal("expected error for missing key")
	}
	if !errors.Is(resErr, ErrNotFound) {
		t.Errorf("expected a not-found diagnostic, got: %v", resErr)
	}
	if errors.Is(resErr, ErrBadPath) {
		t.Errorf("a resolution failure must not match the bad-path kind: %v", resErr)
	}
}

// --- Get on array-of-tables without index ---

func TestAudit_GetArrayOfTablesWithoutIndex(t *testing.T) {
	input := `
[[products]]
name = "Widget"

[[products]]
name = "Gadget"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// An array-of-tables is not a single node: the concrete-node surfaces
	// refuse it, and the read-layer is where its entries are read.
	if node, ok := doc.Lookup("products"); ok {
		t.Errorf("Lookup(\"products\") answered with %T; an array-of-tables is no single node", node)
	}
	if doc.Has("products") {
		t.Error("Has(\"products\") reported an array-of-tables as a concrete node")
	}
	_, err = doc.Resolve("products")
	if !errors.Is(err, ErrWrongContainer) {
		t.Errorf("Resolve(\"products\") = %v, want a wrong-container diagnostic", err)
	}

	// A key on a collection needs an index first.
	_, err = doc.Resolve("products.name")
	if !errors.Is(err, ErrWrongContainer) {
		t.Errorf("Resolve(\"products.name\") = %v, want a wrong-container diagnostic", err)
	}

	// Indexing reaches the entry, which is a node.
	node, ok := doc.Lookup("products[0]")
	if !ok {
		t.Fatal("Lookup(\"products[0]\") found nothing")
	}
	if _, isEntry := node.(*ArrayTableNode); !isEntry {
		t.Errorf("products[0] is a %T, want an *ArrayTableNode", node)
	}

	// The read-layer carries the collection itself.
	entry, ok := doc.Root().Get("products")
	if !ok {
		t.Fatal("the read-layer has no entry products")
	}
	records, ok := entry.Records()
	if !ok || len(records) != 2 {
		t.Errorf("the read-layer holds %d product entries, want 2", len(records))
	}
}

// --- Dotted key intermediate access ---

func TestAudit_DottedKeyIntermediateAccess(t *testing.T) {
	// a.b.c = val -- the tables the dotted key implies have no node of their own
	input := `a.b.c = "deep"`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Full path should work
	val, err := doc.GetString("a.b.c")
	if err != nil {
		t.Fatalf("GetString(\"a.b.c\"): %v", err)
	}
	if val != "deep" {
		t.Errorf("expected \"deep\", got %q", val)
	}

	// The tables a dotted key implies have no node of their own, so the
	// concrete-node surfaces refuse them.
	for _, path := range []string{"a", "a.b"} {
		if node, ok := doc.Lookup(path); ok {
			t.Errorf("Lookup(%q) answered with %T; an implied table is no single node", path, node)
		}
		if _, err := doc.Resolve(path); !errors.Is(err, ErrWrongContainer) {
			t.Errorf("Resolve(%q) = %v, want a wrong-container diagnostic", path, err)
		}
	}

	// The read-layer carries them.
	a, ok := doc.Root().Get("a")
	if !ok {
		t.Fatal("the read-layer has no entry a")
	}
	rec, ok := a.Record()
	if !ok {
		t.Fatalf("entry a is a %s, want a record", a.Kind())
	}
	if _, ok := rec.Get("b"); !ok {
		t.Error("the record a does not hold b")
	}
}

// --- Inline table traversal ---

func TestAudit_InlineTableDeepTraversal(t *testing.T) {
	input := `
[section]
nested = {a = {b = {c = 42}}}
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	val, err := doc.GetInt("section.nested.a.b.c")
	if err != nil {
		t.Fatalf("GetInt(\"section.nested.a.b.c\"): %v", err)
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
}

// --- Array value indexing ---

func TestAudit_ArrayValueIndexing(t *testing.T) {
	input := `
arr = ["a", "b", "c"]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// arr[0]
	val, err := doc.GetString("arr[0]")
	if err != nil {
		t.Fatalf("GetString(\"arr[0]\"): %v", err)
	}
	if val != "a" {
		t.Errorf("expected \"a\", got %q", val)
	}

	// arr[-1]
	val, err = doc.GetString("arr[-1]")
	if err != nil {
		t.Fatalf("GetString(\"arr[-1]\"): %v", err)
	}
	if val != "c" {
		t.Errorf("expected \"c\", got %q", val)
	}
}

// --- Concurrent read safety ---

func TestAudit_ConcurrentReads(t *testing.T) {
	input := `
[server]
host = "localhost"
port = 8080

[[items]]
name = "a"

[[items]]
name = "b"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Run multiple goroutines reading concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			for j := 0; j < 100; j++ {
				doc.GetString("server.host")
				doc.GetInt("server.port")
				doc.GetString("items[0].name")
				doc.GetString("items[-1].name")
				doc.Key("server").Key("host").String()
				doc.Key("items").At(0).Key("name").String()
				doc.Resolve("server.host")
			}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	// If we get here without race detector complaints, it passes
}

// --- Test that a getter refuses a path that does not parse ---

func TestAudit_GettersRefuseUnparseablePath(t *testing.T) {
	doc, err := Parse([]byte(`key = "val"`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Each getter reports the bad path and reads the zero value.
	if s, err := doc.GetString("bad["); err == nil || s != "" {
		t.Errorf("GetString on bad path: (%q, %v)", s, err)
	}
	if n, err := doc.GetInt("bad["); err == nil || n != 0 {
		t.Errorf("GetInt on bad path: (%d, %v)", n, err)
	}
	if b, err := doc.GetBool("bad["); err == nil || b != false {
		t.Errorf("GetBool on bad path: (%v, %v)", b, err)
	}
	if f, err := doc.GetFloat("bad["); err == nil || f != 0 {
		t.Errorf("GetFloat on bad path: (%f, %v)", f, err)
	}
	if _, err := doc.GetTime("bad["); err == nil {
		t.Error("GetTime on a path that does not parse reported no error")
	}
	if node, ok := doc.Lookup("bad["); ok {
		t.Errorf("Lookup on bad path should report false, got %T", node)
	}
}

// --- Test top-level key access ---

func TestAudit_TopLevelKey(t *testing.T) {
	input := `title = "My TOML"`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	val, err := doc.GetString("title")
	if err != nil {
		t.Fatalf("GetString(\"title\"): %v", err)
	}
	if val != "My TOML" {
		t.Errorf("expected \"My TOML\", got %q", val)
	}
}

// --- Test indexing into non-array ---

func TestAudit_IndexIntoNonArray(t *testing.T) {
	input := `val = "hello"`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	node, ok := doc.Lookup("val[0]")
	if ok {
		t.Errorf("Lookup(\"val[0]\") on a string should return nil, got %T", node)
	}

	_, resolveErr := doc.Resolve("val[0]")
	if resolveErr == nil {
		t.Fatal("Resolve(\"val[0]\") on a string should return error")
	}
}

// --- Test key lookup in non-table ---

func TestAudit_KeyLookupInNonTable(t *testing.T) {
	input := `val = 42`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	node, ok := doc.Lookup("val.sub")
	if ok {
		t.Errorf("Lookup(\"val.sub\") on an integer should return nil, got %T", node)
	}
}

// --- Test Get on array-of-tables entry returns ArrayTableNode ---

func TestAudit_GetIndexedArrayTable(t *testing.T) {
	input := `
[[entries]]
x = 1

[[entries]]
x = 2
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	node, ok := doc.Lookup("entries[0]")
	if !ok {
		t.Fatal("Lookup(\"entries[0]\") returned nil")
	}
	if _, ok := node.(*ArrayTableNode); !ok {
		t.Errorf("expected *ArrayTableNode, got %T", node)
	}
}
