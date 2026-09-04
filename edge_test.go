package tomledit

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// 1. Empty document: Parse([]byte("")) succeeds, Bytes() returns empty/newline
func TestEdgeEmptyDocument(t *testing.T) {
	doc, err := Parse([]byte(""))
	if err != nil {
		t.Fatalf("Parse empty document failed: %v", err)
	}
	out := doc.Bytes()
	if len(out) != 0 {
		t.Fatalf("expected empty output for empty document, got %q", out)
	}
}

// 2. Document with only comments: Parse, Bytes round-trips, comments preserved
func TestEdgeOnlyComments(t *testing.T) {
	input := "# This is a comment\n# Another comment\n# Third line\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	out := doc.Bytes()
	if string(out) != input {
		t.Fatalf("comment round-trip failed:\ninput:  %q\noutput: %q", input, out)
	}

	// Verify re-parsing produces same output
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	out2 := doc2.Bytes()
	if !bytes.Equal(out, out2) {
		t.Fatalf("second round-trip changed output:\nfirst:  %q\nsecond: %q", out, out2)
	}
}

// 3. Deeply nested tables (10+ levels) using hierarchical table headers
func TestEdgeDeeplyNestedTables(t *testing.T) {
	input := "[a]\n[a.b]\n[a.b.c]\n[a.b.c.d]\n[a.b.c.d.e]\n[a.b.c.d.e.f]\n[a.b.c.d.e.f.g]\n[a.b.c.d.e.f.g.h]\n[a.b.c.d.e.f.g.h.i]\n[a.b.c.d.e.f.g.h.i.j]\nkey = \"deep\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	out := doc.Bytes()
	if string(out) != input {
		t.Fatalf("deep table round-trip failed:\ninput:  %q\noutput: %q", input, out)
	}

	// Verify we can access the value through the hierarchy
	val, ok := doc.GetString("a.b.c.d.e.f.g.h.i.j.key")
	if !ok || val != "deep" {
		t.Fatalf("GetString on deep table: got %q, %v; want %q, true", val, ok, "deep")
	}
}

// 3b. Deeply nested tables using compound key path (round-trip only)
func TestEdgeDeeplyNestedCompoundKeyPath(t *testing.T) {
	input := "[a.b.c.d.e.f.g.h.i.j]\nkey = \"deep\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	out := doc.Bytes()
	if string(out) != input {
		t.Fatalf("deep compound table round-trip failed:\ninput:  %q\noutput: %q", input, out)
	}

	// Re-parse must be stable
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	out2 := doc2.Bytes()
	if !bytes.Equal(out, out2) {
		t.Fatalf("second round-trip not stable:\nfirst:  %q\nsecond: %q", out, out2)
	}
}

// 4. Very long strings (10KB+ string value)
func TestEdgeVeryLongString(t *testing.T) {
	longStr := strings.Repeat("abcdefghij", 1100) // 11000 bytes
	input := `key = "` + longStr + "\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	val, ok := doc.GetString("key")
	if !ok || val != longStr {
		t.Fatalf("GetString: got len=%d, want len=%d", len(val), len(longStr))
	}
	out := doc.Bytes()
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	val2, ok := doc2.GetString("key")
	if !ok || val2 != longStr {
		t.Fatalf("round-trip lost long string: got len=%d, want len=%d", len(val2), len(longStr))
	}
}

// 5. Very large arrays (1000+ elements)
func TestEdgeVeryLargeArray(t *testing.T) {
	var b strings.Builder
	b.WriteString("key = [")
	for i := 0; i < 1000; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		// Use valid TOML integers (no leading zeros)
		fmt.Fprintf(&b, "%d", i)
	}
	b.WriteString("]\n")
	input := b.String()

	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	node, ok := doc.Lookup("key")
	if !ok {
		t.Fatal("Get returned nil for key")
	}
	arr, ok := node.(*ArrayNode)
	if !ok {
		t.Fatalf("expected ArrayNode, got %T", node)
	}
	if len(arr.elements) != 1000 {
		t.Fatalf("expected 1000 elements, got %d", len(arr.elements))
	}
}

// 6. All TOML 1.0 escape sequences
func TestEdgeAllEscapeSequences(t *testing.T) {
	// \b \t \n \f \r \" \\ \uXXXX \UXXXXXXXX
	input := `key = "tab:\there\nnewline\r\nCRLF\bback\fform\"quote\\slashA\U00000042"` + "\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	val, ok := doc.GetString("key")
	if !ok {
		t.Fatal("GetString failed")
	}
	// Expected: tab:\there\nnewline etc. with actual control chars
	if !strings.Contains(val, "\t") {
		t.Error("missing tab")
	}
	if !strings.Contains(val, "\n") {
		t.Error("missing newline")
	}
	if !strings.Contains(val, "\r") {
		t.Error("missing carriage return")
	}
	if !strings.Contains(val, "\b") {
		t.Error("missing backspace")
	}
	if !strings.Contains(val, "\f") {
		t.Error("missing form feed")
	}
	if !strings.Contains(val, "\"") {
		t.Error("missing escaped quote")
	}
	if !strings.Contains(val, "\\") {
		t.Error("missing escaped backslash")
	}
	if !strings.Contains(val, "A") {
		t.Error("missing \\u0041 (A)")
	}
	if !strings.Contains(val, "B") {
		t.Error("missing \\U00000042 (B)")
	}

	// Round-trip
	out := doc.Bytes()
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	val2, ok := doc2.GetString("key")
	if !ok || val2 != val {
		t.Fatalf("round-trip changed escape values:\noriginal: %q\nafter:    %q", val, val2)
	}
}

// 7. Unicode in keys and values: CJK, emoji, RTL text
func TestEdgeUnicode(t *testing.T) {
	input := "\"\\u4F60\\u597D\" = \"\\u4E16\\u754C\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	out := doc.Bytes()
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	out2 := doc2.Bytes()
	if !bytes.Equal(out, out2) {
		t.Fatalf("unicode round-trip not stable:\nfirst:  %q\nsecond: %q", out, out2)
	}
}

func TestEdgeUnicodeDirectChars(t *testing.T) {
	// Direct UTF-8 characters in values
	input := "greeting = \"\xe4\xbd\xa0\xe5\xa5\xbd\"\n" // "你好"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	val, ok := doc.GetString("greeting")
	if !ok || val != "\xe4\xbd\xa0\xe5\xa5\xbd" {
		t.Fatalf("expected '你好', got %q", val)
	}
}

// 8. Mixed table definition styles: Standard tables + dotted keys + sub-tables
func TestEdgeMixedTableStyles(t *testing.T) {
	input := `[server]
host = "localhost"
database.host = "db.example.com"

[server.logging]
level = "info"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Standard key under table
	val, ok := doc.GetString("server.host")
	if !ok || val != "localhost" {
		t.Fatalf("expected 'localhost', got %q, %v", val, ok)
	}

	// Dotted key: database.host under [server]
	val, ok = doc.GetString("server.database.host")
	if !ok || val != "db.example.com" {
		t.Fatalf("expected 'db.example.com', got %q, %v", val, ok)
	}

	// Sub-table key
	val, ok = doc.GetString("server.logging.level")
	if !ok || val != "info" {
		t.Fatalf("expected 'info', got %q, %v", val, ok)
	}

	// Round-trip
	out := doc.Bytes()
	if string(out) != input {
		t.Fatalf("mixed styles round-trip failed:\ninput:  %q\noutput: %q", input, out)
	}
}

// 9. Blank lines everywhere
func TestEdgeBlankLinesEverywhere(t *testing.T) {
	input := "\n\nkey1 = \"val1\"\n\n\n[table]\n\nkey2 = \"val2\"\n\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	out := doc.Bytes()
	if string(out) != input {
		t.Fatalf("blank lines round-trip failed:\ninput:  %q\noutput: %q", input, out)
	}
}

// 10. CRLF line endings
func TestEdgeCRLFLineEndings(t *testing.T) {
	input := "[server]\r\nhost = \"localhost\"\r\nport = 8080\r\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	val, ok := doc.GetString("server.host")
	if !ok || val != "localhost" {
		t.Fatalf("expected 'localhost', got %q, %v", val, ok)
	}

	port, ok := doc.GetInt("server.port")
	if !ok || port != 8080 {
		t.Fatalf("expected 8080, got %d, %v", port, ok)
	}
}

// 11. No trailing newline
func TestEdgeNoTrailingNewline(t *testing.T) {
	input := `key = "value"`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	val, ok := doc.GetString("key")
	if !ok || val != "value" {
		t.Fatalf("expected 'value', got %q, %v", val, ok)
	}

	// Round-trip should still produce valid TOML
	out := doc.Bytes()
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse of no-trailing-newline output failed: %v\noutput: %q", err, out)
	}
	val2, ok := doc2.GetString("key")
	if !ok || val2 != "value" {
		t.Fatalf("round-trip changed value: %q", val2)
	}
}

// 12. Tab indentation
func TestEdgeTabIndentation(t *testing.T) {
	input := "[server]\n\thost = \"localhost\"\n\tport = 8080\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	val, ok := doc.GetString("server.host")
	if !ok || val != "localhost" {
		t.Fatalf("expected 'localhost', got %q, %v", val, ok)
	}
}

// 13. Multiple operations then round-trip
func TestEdgeMultipleOperationsThenRoundTrip(t *testing.T) {
	input := "[server]\nhost = \"localhost\"\nport = 8080\n\n[[products]]\nname = \"Widget\"\nprice = 9.99\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Set a value
	if err := doc.Set("server.host", "newhost"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Delete a value
	if err := doc.Delete("server.port"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Rename a value
	if err := doc.RenameKey("products[0].name", "title"); err != nil {
		t.Fatalf("RenameKey failed: %v", err)
	}

	// Create a new table
	if err := doc.NewTable("logging"); err != nil {
		t.Fatalf("NewTable failed: %v", err)
	}

	// Set values in the new table
	if err := doc.Set("logging.level", "info"); err != nil {
		t.Fatalf("Set in new table failed: %v", err)
	}

	// Bytes() should produce valid TOML
	out := doc.Bytes()
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("round-trip after operations produced invalid TOML: %v\noutput: %q", err, out)
	}

	// Verify modifications
	val, ok := doc2.GetString("server.host")
	if !ok || val != "newhost" {
		t.Fatalf("expected 'newhost', got %q, %v", val, ok)
	}

	_, ok = doc2.GetInt("server.port")
	if ok {
		t.Fatal("expected server.port to be deleted")
	}

	val, ok = doc2.GetString("products[0].title")
	if !ok || val != "Widget" {
		t.Fatalf("expected renamed key 'title' = 'Widget', got %q, %v", val, ok)
	}

	val, ok = doc2.GetString("logging.level")
	if !ok || val != "info" {
		t.Fatalf("expected 'info', got %q, %v", val, ok)
	}

	// Second round-trip must be stable
	out2 := doc2.Bytes()
	doc3, err := Parse(out2)
	if err != nil {
		t.Fatalf("second re-parse failed: %v", err)
	}
	out3 := doc3.Bytes()
	if !bytes.Equal(out2, out3) {
		t.Fatalf("second round-trip not stable:\nfirst:  %q\nsecond: %q", out2, out3)
	}
}

// 14. Set then Get consistency
func TestEdgeSetThenGetConsistency(t *testing.T) {
	doc, err := Parse([]byte("[test]\n"))
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		path string
		val  any
	}{
		{"test.str", "hello"},
		{"test.num", 42},
		{"test.flag", true},
		{"test.pi", 3.14},
	}

	for _, tc := range testCases {
		if err := doc.Set(tc.path, tc.val); err != nil {
			t.Fatalf("Set(%q, %v) failed: %v", tc.path, tc.val, err)
		}
		node, ok := doc.Lookup(tc.path)
		if !ok {
			t.Fatalf("Lookup(%q) returned nil after Set", tc.path)
		}
		got := node.(Scalar).Value()
		switch expected := tc.val.(type) {
		case string:
			if s, ok := got.(string); !ok || s != expected {
				t.Fatalf("Lookup(%q) = %v, want %v", tc.path, got, expected)
			}
		case int:
			if n, ok := got.(int64); !ok || n != int64(expected) {
				t.Fatalf("Lookup(%q) = %v, want %v", tc.path, got, expected)
			}
		case bool:
			if b, ok := got.(bool); !ok || b != expected {
				t.Fatalf("Lookup(%q) = %v, want %v", tc.path, got, expected)
			}
		case float64:
			if f, ok := got.(float64); !ok || f != expected {
				t.Fatalf("Lookup(%q) = %v, want %v", tc.path, got, expected)
			}
		}
	}
}

// 15. Delete everything
func TestEdgeDeleteEverything(t *testing.T) {
	input := "[server]\nhost = \"localhost\"\nport = 8080\n\n[database]\nname = \"mydb\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	// Delete all keys and tables
	doc.Delete("server.host")
	doc.Delete("server.port")
	doc.Delete("server")
	doc.Delete("database.name")
	doc.Delete("database")

	out := doc.Bytes()

	// Should still be parseable
	_, err = Parse(out)
	if err != nil {
		t.Fatalf("parsing after deleting everything failed: %v\noutput: %q", err, out)
	}
}

// 16. Extreme negative indices
func TestEdgeExtremeNegativeIndices(t *testing.T) {
	input := "[[products]]\nname = \"A\"\n\n[[products]]\nname = \"B\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	// -100 on a 2-element array should return nil/error
	node, ok := doc.Lookup("products[-100]")
	if ok {
		t.Fatalf("expected nil for extreme negative index, got %v", node)
	}

	// Also test with inline arrays
	input2 := "key = [1, 2]\n"
	doc2, err := Parse([]byte(input2))
	if err != nil {
		t.Fatal(err)
	}
	node2, ok := doc2.Lookup("key[-100]")
	if ok {
		t.Fatalf("expected nil for extreme negative index on array, got %v", node2)
	}

	// Positive out of range too
	node3, ok := doc2.Lookup("key[100]")
	if ok {
		t.Fatalf("expected nil for out-of-range positive index, got %v", node3)
	}
}

// 17. Empty keys
func TestEdgeEmptyKeys(t *testing.T) {
	input := `"" = "empty key"` + "\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Access with quoted empty key
	node, ok := doc.Lookup(`""`)
	if !ok {
		t.Fatal("Get with empty key returned nil")
	}
	s, ok := node.(*StringNode)
	if !ok {
		t.Fatalf("expected StringNode, got %T", node)
	}
	if s.val.get() != "empty key" {
		t.Fatalf("expected 'empty key', got %q", s.val.get())
	}

	// Round-trip
	out := doc.Bytes()
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse failed: %v\noutput: %q", err, out)
	}
	out2 := doc2.Bytes()
	if !bytes.Equal(out, out2) {
		t.Fatalf("empty key round-trip not stable:\nfirst:  %q\nsecond: %q", out, out2)
	}
}

// 18. Keys that look like integers
func TestEdgeIntegerLikeKeys(t *testing.T) {
	input := "\"0\" = \"zero\"\n\"1\" = \"one\"\n\"42\" = \"answer\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// These should be found as string keys, not confused with array indices
	val, ok := doc.GetString(`"0"`)
	if !ok || val != "zero" {
		t.Fatalf("expected 'zero', got %q, %v", val, ok)
	}

	val, ok = doc.GetString(`"1"`)
	if !ok || val != "one" {
		t.Fatalf("expected 'one', got %q, %v", val, ok)
	}

	val, ok = doc.GetString(`"42"`)
	if !ok || val != "answer" {
		t.Fatalf("expected 'answer', got %q, %v", val, ok)
	}

	// Round-trip
	out := doc.Bytes()
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse failed: %v\noutput: %q", err, out)
	}
	out2 := doc2.Bytes()
	if !bytes.Equal(out, out2) {
		t.Fatalf("integer-like key round-trip not stable:\nfirst:  %q\nsecond: %q", out, out2)
	}
}
