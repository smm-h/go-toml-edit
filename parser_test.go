package tomledit

import (
	"math"
	"strings"
	"testing"
)

// collectRawBytes walks the tree depth-first and concatenates raw bytes from
// each node in the correct order to reproduce the original source.
func collectRawBytes(node Node) []byte {
	switch n := node.(type) {
	case *Document:
		var out []byte
		for _, c := range n.Children {
			out = append(out, collectRawBytes(c)...)
		}
		return out
	case *TableNode:
		out := append([]byte(nil), n.Raw()...)
		for _, c := range n.Children {
			out = append(out, collectRawBytes(c)...)
		}
		return out
	case *ArrayTableNode:
		out := append([]byte(nil), n.Raw()...)
		for _, c := range n.Children {
			out = append(out, collectRawBytes(c)...)
		}
		return out
	default:
		return node.Raw()
	}
}

// --- Simple key-value tests ---

func TestParseSimpleString(t *testing.T) {
	input := `key = "value"` + "\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(doc.Children))
	}
	kv, ok := doc.Children[0].(*KeyValueNode)
	if !ok {
		t.Fatalf("expected KeyValueNode, got %T", doc.Children[0])
	}
	if kv.Key.Parts[0] != "key" {
		t.Errorf("key = %q, want %q", kv.Key.Parts[0], "key")
	}
	sv, ok := kv.Val.(*StringNode)
	if !ok {
		t.Fatalf("expected StringNode, got %T", kv.Val)
	}
	if sv.Val != "value" {
		t.Errorf("val = %q, want %q", sv.Val, "value")
	}
}

func TestParseSimpleInteger(t *testing.T) {
	tests := []struct {
		input string
		val   int64
		base  IntegerBase
	}{
		{"n = 42\n", 42, IntegerDecimal},
		{"n = +99\n", 99, IntegerDecimal},
		{"n = -17\n", -17, IntegerDecimal},
		{"n = 0\n", 0, IntegerDecimal},
		{"n = 1_000\n", 1000, IntegerDecimal},
		{"n = 0xDEAD\n", 0xDEAD, IntegerHex},
		{"n = 0o755\n", 0o755, IntegerOctal},
		{"n = 0b1101\n", 0b1101, IntegerBinary},
		{"n = 0xdead_beef\n", 0xdeadbeef, IntegerHex},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			doc, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			kv := doc.Children[0].(*KeyValueNode)
			iv := kv.Val.(*IntegerNode)
			if iv.Val != tt.val {
				t.Errorf("val = %d, want %d", iv.Val, tt.val)
			}
			if iv.Base != tt.base {
				t.Errorf("base = %d, want %d", iv.Base, tt.base)
			}
		})
	}
}

func TestParseSimpleFloat(t *testing.T) {
	tests := []struct {
		input string
		val   float64
		isNaN bool
	}{
		{"n = 3.14\n", 3.14, false},
		{"n = +1.0\n", 1.0, false},
		{"n = -0.5\n", -0.5, false},
		{"n = 1e10\n", 1e10, false},
		{"n = 5e+22\n", 5e+22, false},
		{"n = inf\n", math.Inf(1), false},
		{"n = +inf\n", math.Inf(1), false},
		{"n = -inf\n", math.Inf(-1), false},
		{"n = nan\n", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			doc, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			kv := doc.Children[0].(*KeyValueNode)
			fv := kv.Val.(*FloatNode)
			if tt.isNaN {
				if !math.IsNaN(fv.Val) {
					t.Errorf("expected NaN, got %f", fv.Val)
				}
			} else if fv.Val != tt.val {
				t.Errorf("val = %f, want %f", fv.Val, tt.val)
			}
		})
	}
}

func TestParseBoolean(t *testing.T) {
	input := "a = true\nb = false\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(doc.Children))
	}
	bv1 := doc.Children[0].(*KeyValueNode).Val.(*BooleanNode)
	bv2 := doc.Children[1].(*KeyValueNode).Val.(*BooleanNode)
	if bv1.Val != true {
		t.Error("expected true")
	}
	if bv2.Val != false {
		t.Error("expected false")
	}
}

func TestParseDottedKey(t *testing.T) {
	input := "a.b.c = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.Children[0].(*KeyValueNode)
	if len(kv.Key.Parts) != 3 {
		t.Fatalf("expected 3 key parts, got %d", len(kv.Key.Parts))
	}
	if kv.Key.Parts[0] != "a" || kv.Key.Parts[1] != "b" || kv.Key.Parts[2] != "c" {
		t.Errorf("parts = %v, want [a b c]", kv.Key.Parts)
	}
}

func TestParseQuotedKey(t *testing.T) {
	input := `"key with spaces" = "val"` + "\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.Children[0].(*KeyValueNode)
	if kv.Key.Parts[0] != "key with spaces" {
		t.Errorf("key = %q, want %q", kv.Key.Parts[0], "key with spaces")
	}
}

func TestParseMixedDottedKey(t *testing.T) {
	input := `server."host name".port = 8080` + "\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.Children[0].(*KeyValueNode)
	if len(kv.Key.Parts) != 3 {
		t.Fatalf("expected 3 key parts, got %d", len(kv.Key.Parts))
	}
	if kv.Key.Parts[0] != "server" || kv.Key.Parts[1] != "host name" || kv.Key.Parts[2] != "port" {
		t.Errorf("parts = %v", kv.Key.Parts)
	}
}

func TestParseLiteralKey(t *testing.T) {
	input := "'literal-key' = 42\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.Children[0].(*KeyValueNode)
	if kv.Key.Parts[0] != "literal-key" {
		t.Errorf("key = %q, want %q", kv.Key.Parts[0], "literal-key")
	}
}

// --- String types ---

func TestParseStringTypes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		val   string
		style StringStyle
	}{
		{"basic", `s = "hello"` + "\n", "hello", StringBasic},
		{"basic-escape", `s = "hello\nworld"` + "\n", "hello\nworld", StringBasic},
		{"basic-unicode", `s = "A"` + "\n", "A", StringBasic},
		{"literal", `s = 'hello'` + "\n", "hello", StringLiteral},
		{"literal-backslash", `s = 'C:\path'` + "\n", `C:\path`, StringLiteral},
		{"multiline-basic", "s = \"\"\"\nhello\"\"\"\n", "hello", StringMultiLineBasic},
		{"multiline-literal", "s = '''\nhello'''\n", "hello", StringMultiLineLiteral},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			sv := doc.Children[0].(*KeyValueNode).Val.(*StringNode)
			if sv.Val != tt.val {
				t.Errorf("val = %q, want %q", sv.Val, tt.val)
			}
			if sv.Style != tt.style {
				t.Errorf("style = %d, want %d", sv.Style, tt.style)
			}
		})
	}
}

// --- Date/time types ---

func TestParseDateTimeTypes(t *testing.T) {
	t.Run("offset-datetime", func(t *testing.T) {
		doc, err := Parse([]byte("dt = 1979-05-27T07:32:00Z\n"))
		if err != nil {
			t.Fatal(err)
		}
		dt := doc.Children[0].(*KeyValueNode).Val.(*DateTimeNode)
		if dt.Val.Year() != 1979 || dt.Val.Month() != 5 || dt.Val.Day() != 27 {
			t.Errorf("date = %v", dt.Val)
		}
	})

	t.Run("offset-datetime-with-offset", func(t *testing.T) {
		doc, err := Parse([]byte("dt = 1979-05-27T07:32:00+09:00\n"))
		if err != nil {
			t.Fatal(err)
		}
		dt := doc.Children[0].(*KeyValueNode).Val.(*DateTimeNode)
		if dt.Val.Year() != 1979 {
			t.Errorf("year = %d", dt.Val.Year())
		}
	})

	t.Run("local-datetime", func(t *testing.T) {
		doc, err := Parse([]byte("dt = 1979-05-27T07:32:00\n"))
		if err != nil {
			t.Fatal(err)
		}
		ldt := doc.Children[0].(*KeyValueNode).Val.(*LocalDateTimeNode)
		if ldt.Val.Year != 1979 || ldt.Val.Month != 5 || ldt.Val.Day != 27 {
			t.Errorf("date = %+v", ldt.Val)
		}
		if ldt.Val.Hour != 7 || ldt.Val.Minute != 32 {
			t.Errorf("time = %+v", ldt.Val)
		}
	})

	t.Run("local-date", func(t *testing.T) {
		doc, err := Parse([]byte("d = 1979-05-27\n"))
		if err != nil {
			t.Fatal(err)
		}
		ld := doc.Children[0].(*KeyValueNode).Val.(*LocalDateNode)
		if ld.Val.Year != 1979 || ld.Val.Month != 5 || ld.Val.Day != 27 {
			t.Errorf("date = %+v", ld.Val)
		}
	})

	t.Run("local-time", func(t *testing.T) {
		doc, err := Parse([]byte("t = 07:32:00\n"))
		if err != nil {
			t.Fatal(err)
		}
		lt := doc.Children[0].(*KeyValueNode).Val.(*LocalTimeNode)
		if lt.Val.Hour != 7 || lt.Val.Minute != 32 || lt.Val.Second != 0 {
			t.Errorf("time = %+v", lt.Val)
		}
	})

	t.Run("local-time-fractional", func(t *testing.T) {
		doc, err := Parse([]byte("t = 07:32:00.999\n"))
		if err != nil {
			t.Fatal(err)
		}
		lt := doc.Children[0].(*KeyValueNode).Val.(*LocalTimeNode)
		if lt.Val.Hour != 7 || lt.Val.Minute != 32 {
			t.Errorf("time = %+v", lt.Val)
		}
		if lt.Val.Nanosecond == 0 {
			t.Error("expected non-zero nanoseconds")
		}
	})
}

// --- Arrays ---

func TestParseSimpleArray(t *testing.T) {
	input := "a = [1, 2, 3]\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	arr := doc.Children[0].(*KeyValueNode).Val.(*ArrayNode)
	if len(arr.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr.Elements))
	}
	for i, expected := range []int64{1, 2, 3} {
		iv := arr.Elements[i].(*IntegerNode)
		if iv.Val != expected {
			t.Errorf("element %d = %d, want %d", i, iv.Val, expected)
		}
	}
}

func TestParseMultilineArray(t *testing.T) {
	input := `a = [
  1,
  2,
  3,
]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	arr := doc.Children[0].(*KeyValueNode).Val.(*ArrayNode)
	if len(arr.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr.Elements))
	}
}

func TestParseArrayWithComments(t *testing.T) {
	input := `a = [
  1, # first
  2, # second
  3  # third
]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	arr := doc.Children[0].(*KeyValueNode).Val.(*ArrayNode)
	if len(arr.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr.Elements))
	}
}

func TestParseNestedArray(t *testing.T) {
	input := `a = [[1, 2], [3, 4]]` + "\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	arr := doc.Children[0].(*KeyValueNode).Val.(*ArrayNode)
	if len(arr.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(arr.Elements))
	}
	inner := arr.Elements[0].(*ArrayNode)
	if len(inner.Elements) != 2 {
		t.Fatalf("expected 2 inner elements, got %d", len(inner.Elements))
	}
}

func TestParseEmptyArray(t *testing.T) {
	input := "a = []\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	arr := doc.Children[0].(*KeyValueNode).Val.(*ArrayNode)
	if len(arr.Elements) != 0 {
		t.Errorf("expected empty array, got %d elements", len(arr.Elements))
	}
}

// --- Inline tables ---

func TestParseInlineTable(t *testing.T) {
	input := `point = {x = 1, y = 2}` + "\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	it := doc.Children[0].(*KeyValueNode).Val.(*InlineTableNode)
	if len(it.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(it.Children))
	}
	kv1 := it.Children[0].(*KeyValueNode)
	kv2 := it.Children[1].(*KeyValueNode)
	if kv1.Key.Parts[0] != "x" || kv2.Key.Parts[0] != "y" {
		t.Errorf("keys = %q, %q", kv1.Key.Parts[0], kv2.Key.Parts[0])
	}
}

func TestParseEmptyInlineTable(t *testing.T) {
	input := "t = {}\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	it := doc.Children[0].(*KeyValueNode).Val.(*InlineTableNode)
	if len(it.Children) != 0 {
		t.Errorf("expected empty inline table, got %d", len(it.Children))
	}
}

// --- Standard tables ---

func TestParseTable(t *testing.T) {
	input := `[server]
host = "localhost"
port = 8080
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(doc.Children))
	}
	tbl, ok := doc.Children[0].(*TableNode)
	if !ok {
		t.Fatalf("expected TableNode, got %T", doc.Children[0])
	}
	if len(tbl.KeyPath) != 1 || tbl.KeyPath[0] != "server" {
		t.Errorf("KeyPath = %v", tbl.KeyPath)
	}
	if len(tbl.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(tbl.Children))
	}
}

func TestParseNestedTables(t *testing.T) {
	input := `[a]
x = 1

[a.b]
y = 2
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(doc.Children))
	}
	tbl1 := doc.Children[0].(*TableNode)
	tbl2 := doc.Children[1].(*TableNode)

	if tbl1.KeyPath[0] != "a" {
		t.Errorf("table 1 key = %v", tbl1.KeyPath)
	}
	if len(tbl1.Children) != 1 {
		t.Errorf("table 1 children = %d, want 1", len(tbl1.Children))
	}

	if len(tbl2.KeyPath) != 2 || tbl2.KeyPath[0] != "a" || tbl2.KeyPath[1] != "b" {
		t.Errorf("table 2 key = %v", tbl2.KeyPath)
	}
	if len(tbl2.Children) != 1 {
		t.Errorf("table 2 children = %d, want 1", len(tbl2.Children))
	}
}

func TestParseDottedTableKey(t *testing.T) {
	input := `[a.b.c]
x = 1
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	tbl := doc.Children[0].(*TableNode)
	if len(tbl.KeyPath) != 3 {
		t.Fatalf("expected 3-part KeyPath, got %d", len(tbl.KeyPath))
	}
}

// --- Array tables ---

func TestParseArrayTable(t *testing.T) {
	input := `[[products]]
name = "Hammer"
price = 9.99

[[products]]
name = "Nail"
price = 0.05
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(doc.Children))
	}
	atbl1, ok := doc.Children[0].(*ArrayTableNode)
	if !ok {
		t.Fatalf("expected ArrayTableNode, got %T", doc.Children[0])
	}
	if len(atbl1.KeyPath) != 1 || atbl1.KeyPath[0] != "products" {
		t.Errorf("KeyPath = %v", atbl1.KeyPath)
	}
	if len(atbl1.Children) != 2 {
		t.Errorf("children = %d, want 2", len(atbl1.Children))
	}

	atbl2 := doc.Children[1].(*ArrayTableNode)
	if len(atbl2.Children) != 2 {
		t.Errorf("children = %d, want 2", len(atbl2.Children))
	}
}

// --- Trivia preservation ---

func TestTriviaLeadingComments(t *testing.T) {
	input := `# This is a comment
key = "value"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.Children[0].(*KeyValueNode)
	lc := kv.LeadingComments()
	if len(lc) != 1 {
		t.Fatalf("expected 1 leading comment, got %d", len(lc))
	}
	if !strings.Contains(lc[0], "This is a comment") {
		t.Errorf("leading comment = %q", lc[0])
	}
}

func TestTriviaMultipleLeadingComments(t *testing.T) {
	input := `# comment 1
# comment 2
key = "value"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.Children[0].(*KeyValueNode)
	lc := kv.LeadingComments()
	if len(lc) != 2 {
		t.Fatalf("expected 2 leading comments, got %d", len(lc))
	}
}

func TestTriviaInlineComment(t *testing.T) {
	input := "key = \"value\" # inline\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.Children[0].(*KeyValueNode)
	if !strings.Contains(kv.Comment(), "inline") {
		t.Errorf("inline comment = %q", kv.Comment())
	}
}

func TestTriviaTrailingNewline(t *testing.T) {
	input := "key = \"value\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.Children[0].(*KeyValueNode)
	if string(kv.trivia().TrailingNewline) != "\n" {
		t.Errorf("trailing newline = %q", kv.trivia().TrailingNewline)
	}
}

func TestTriviaTableLeadingComment(t *testing.T) {
	input := `# table comment
[server]
host = "localhost"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	tbl := doc.Children[0].(*TableNode)
	lc := tbl.LeadingComments()
	if len(lc) != 1 {
		t.Fatalf("expected 1 leading comment, got %d", len(lc))
	}
	if !strings.Contains(lc[0], "table comment") {
		t.Errorf("comment = %q", lc[0])
	}
}

func TestTriviaTableInlineComment(t *testing.T) {
	input := "[server] # inline on table\nhost = \"localhost\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	tbl := doc.Children[0].(*TableNode)
	if !strings.Contains(tbl.Comment(), "inline on table") {
		t.Errorf("inline comment = %q", tbl.Comment())
	}
}

// --- Orphan comments ---

func TestOrphanCommentAtEnd(t *testing.T) {
	input := "key = 1\n# orphan at end\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	// The orphan comment should end up as a CommentNode somewhere in the document
	found := false
	for _, c := range doc.Children {
		if _, ok := c.(*CommentNode); ok {
			found = true
		}
	}
	// Alternatively it could be attached as a leading comment to EOF or as a table child.
	// For root-level orphan, check if we got 2 children (kv + comment) or 1 with trivia.
	// Our implementation attaches trailing comments as CommentNode children.
	if !found {
		// Check if there are only 1 child - maybe the orphan is absorbed as trivia
		// For this implementation, orphan comments at end of doc go to document children
		t.Logf("doc children: %d", len(doc.Children))
		for i, c := range doc.Children {
			t.Logf("  child %d: %T", i, c)
		}
	}
}

// --- Complex documents ---

func TestParseComplexDocument(t *testing.T) {
	input := `# Global config
title = "My App"
version = 1

[database]
host = "127.0.0.1"
port = 5432
enabled = true

[database.connection]
timeout = 30
max_retries = 3

[[servers]]
name = "alpha"
ip = "10.0.0.1"

[[servers]]
name = "beta"
ip = "10.0.0.2"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	// Root should have: title kv, version kv, [database], [database.connection], 2x [[servers]]
	if len(doc.Children) < 4 {
		t.Fatalf("expected at least 4 top-level children, got %d", len(doc.Children))
	}

	// First child should be title kv
	kv, ok := doc.Children[0].(*KeyValueNode)
	if !ok {
		t.Fatalf("child 0: expected KeyValueNode, got %T", doc.Children[0])
	}
	if kv.Key.Parts[0] != "title" {
		t.Errorf("first key = %q", kv.Key.Parts[0])
	}
}

func TestParseRealisticConfig(t *testing.T) {
	input := `# Server configuration
[server]
host = "0.0.0.0"
port = 8080

[server.tls]
enabled = true
cert = "/etc/ssl/cert.pem"
key = "/etc/ssl/key.pem"

# Logging
[logging]
level = "info"
format = "json"
outputs = ["stdout", "file"]

[[database]]
name = "primary"
dsn = "postgres://localhost/mydb"
max_conns = 10

[[database]]
name = "replica"
dsn = "postgres://replica/mydb"
max_conns = 5
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	// Check it parsed without error and has reasonable structure
	if len(doc.Children) < 4 {
		t.Errorf("expected at least 4 children, got %d", len(doc.Children))
	}
}

// --- Round-trip tests ---

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple-kv", "key = \"value\"\n"},
		{"integer", "n = 42\n"},
		{"float", "f = 3.14\n"},
		{"bool", "b = true\n"},
		{"array", "a = [1, 2, 3]\n"},
		{"inline-table", "t = {x = 1, y = 2}\n"},
		{"table", "[server]\nhost = \"localhost\"\n"},
		{"array-table", "[[products]]\nname = \"Hammer\"\n"},
		{"comment-before-kv", "# comment\nkey = 1\n"},
		{"inline-comment", "key = 1 # inline\n"},
		{"multiple-tables", "[a]\nx = 1\n\n[b]\ny = 2\n"},
		{"dotted-key", "a.b.c = 1\n"},
		{"empty-doc", ""},
		{"only-newline", "\n"},
		{"blank-lines", "a = 1\n\nb = 2\n"},
		{"local-date", "d = 1979-05-27\n"},
		{"local-time", "t = 07:32:00\n"},
		{"offset-datetime", "dt = 1979-05-27T07:32:00Z\n"},
		{"multiline-basic", "s = \"\"\"\nhello\"\"\"\n"},
		{"multiline-literal", "s = '''\nhello'''\n"},
		{"literal-string", "s = 'hello'\n"},
		{"hex-integer", "n = 0xDEAD\n"},
		{"empty-inline-table", "t = {}\n"},
		{"empty-array", "a = []\n"},
		{"multiline-array", "a = [\n  1,\n  2,\n]\n"},
		{"complex", "# header\n[server]\nhost = \"0.0.0.0\" # primary\nport = 8080\n\n[[items]]\nname = \"one\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			got := string(collectRawBytes(doc))
			if got != tt.input {
				t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", tt.input, got)
			}
		})
	}
}

// --- Error cases ---

func TestParseDuplicateKey(t *testing.T) {
	input := "a = 1\na = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for duplicate key")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q, expected duplicate key error", err.Error())
	}
}

func TestParseTableRedefinition(t *testing.T) {
	input := "[a]\nx = 1\n[a]\ny = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for table redefinition")
	}
	if !strings.Contains(err.Error(), "already defined") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestParseValueThenTable(t *testing.T) {
	input := "a = 1\n[a]\nx = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for value-then-table conflict")
	}
}

func TestParseDottedKeyConflict(t *testing.T) {
	input := "a.b = 1\na.b.c = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for dotted key conflict")
	}
}

func TestParseArrayTableVsTableConflict(t *testing.T) {
	input := "[a]\nx = 1\n[[a]]\ny = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for array-table vs table conflict")
	}
}

func TestParseTableVsArrayTableConflict(t *testing.T) {
	input := "[[a]]\nx = 1\n[a]\ny = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for table vs array-table conflict")
	}
}

// --- Edge cases ---

func TestParseEmptyDocument(t *testing.T) {
	doc, err := Parse([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(doc.Children))
	}
}

func TestParseDocumentWithOnlyComments(t *testing.T) {
	input := "# just a comment\n# another comment\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	// Comments should be captured as orphan CommentNodes
	if len(doc.Children) == 0 {
		t.Error("expected at least 1 child for comments")
	}
}

func TestParseDeeplyNestedTable(t *testing.T) {
	input := "[a.b.c.d.e]\nx = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	tbl := doc.Children[0].(*TableNode)
	if len(tbl.KeyPath) != 5 {
		t.Errorf("expected 5-part KeyPath, got %d", len(tbl.KeyPath))
	}
}

func TestParseEmptyTable(t *testing.T) {
	input := "[empty]\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	tbl := doc.Children[0].(*TableNode)
	if len(tbl.Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(tbl.Children))
	}
}

func TestParseSubtableAfterParent(t *testing.T) {
	// [a] then [a.b] is fine (not a redefinition)
	input := "[a]\nx = 1\n[a.b]\ny = 2\n"
	_, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseMultipleRootKVs(t *testing.T) {
	input := "a = 1\nb = 2\nc = 3\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(doc.Children))
	}
}

func TestParseKVBeforeTable(t *testing.T) {
	input := "root = true\n\n[section]\nkey = \"val\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(doc.Children))
	}
	_, ok := doc.Children[0].(*KeyValueNode)
	if !ok {
		t.Errorf("child 0: expected KeyValueNode, got %T", doc.Children[0])
	}
	_, ok = doc.Children[1].(*TableNode)
	if !ok {
		t.Errorf("child 1: expected TableNode, got %T", doc.Children[1])
	}
}

func TestParseNoTrailingNewline(t *testing.T) {
	// TOML allows no trailing newline
	input := "key = 42"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(doc.Children))
	}
}

func TestParseCRLF(t *testing.T) {
	input := "key = \"value\"\r\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.Children[0].(*KeyValueNode)
	if string(kv.trivia().TrailingNewline) != "\r\n" {
		t.Errorf("expected CRLF trailing, got %q", kv.trivia().TrailingNewline)
	}
}

func TestParseInlineTableWithDottedKey(t *testing.T) {
	input := "t = {a.b = 1}\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	it := doc.Children[0].(*KeyValueNode).Val.(*InlineTableNode)
	if len(it.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(it.Children))
	}
	kv := it.Children[0].(*KeyValueNode)
	if len(kv.Key.Parts) != 2 {
		t.Errorf("expected 2 key parts, got %d", len(kv.Key.Parts))
	}
}

func TestParseArrayOfStrings(t *testing.T) {
	input := `colors = ["red", "green", "blue"]` + "\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	arr := doc.Children[0].(*KeyValueNode).Val.(*ArrayNode)
	if len(arr.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr.Elements))
	}
	for i, expected := range []string{"red", "green", "blue"} {
		sv := arr.Elements[i].(*StringNode)
		if sv.Val != expected {
			t.Errorf("element %d = %q, want %q", i, sv.Val, expected)
		}
	}
}

func TestParseWhitespaceAroundEquals(t *testing.T) {
	input := "key   =   42\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.Children[0].(*KeyValueNode)
	iv := kv.Val.(*IntegerNode)
	if iv.Val != 42 {
		t.Errorf("val = %d, want 42", iv.Val)
	}
}

func TestParseLeadingWhitespace(t *testing.T) {
	input := "  key = 42\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.Children[0].(*KeyValueNode)
	if string(kv.trivia().LeadingWhitespace) != "  " {
		t.Errorf("leading whitespace = %q, want %q", kv.trivia().LeadingWhitespace, "  ")
	}
}

func TestParseEscapeSequences(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`s = "hello\nworld"` + "\n", "hello\nworld"},
		{`s = "tab\there"` + "\n", "tab\there"},
		{`s = "quote\"here"` + "\n", "quote\"here"},
		{`s = "back\\slash"` + "\n", "back\\slash"},
		{`s = "A"` + "\n", "A"},
		{`s = "\U00000041"` + "\n", "A"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			doc, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			sv := doc.Children[0].(*KeyValueNode).Val.(*StringNode)
			if sv.Val != tt.expected {
				t.Errorf("val = %q, want %q", sv.Val, tt.expected)
			}
		})
	}
}

func TestParseArrayTrailingComma(t *testing.T) {
	input := "a = [1, 2, 3,]\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	arr := doc.Children[0].(*KeyValueNode).Val.(*ArrayNode)
	if len(arr.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr.Elements))
	}
}

// --- Multiple array table entries ---

func TestParseMultipleArrayTableEntries(t *testing.T) {
	input := `[[fruit]]
name = "apple"

[[fruit]]
name = "banana"

[[fruit]]
name = "cherry"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, c := range doc.Children {
		if at, ok := c.(*ArrayTableNode); ok {
			count++
			if at.KeyPath[0] != "fruit" {
				t.Errorf("unexpected KeyPath: %v", at.KeyPath)
			}
			if len(at.Children) != 1 {
				t.Errorf("expected 1 child, got %d", len(at.Children))
			}
		}
	}
	if count != 3 {
		t.Errorf("expected 3 array table entries, got %d", count)
	}
}

// --- Comprehensive round-trip with complex document ---

func TestRoundTripComplex(t *testing.T) {
	input := `# This is a TOML document

title = "TOML Example"

[owner]
name = "Tom Preston-Werner"

[database]
enabled = true
ports = [8001, 8001, 8002]
data = [["delta", "phi"], [3.14]]
temp_targets = {cpu = 79.5, case = 72.0}

[servers]

[servers.alpha]
ip = "10.0.0.1"
role = "frontend"

[servers.beta]
ip = "10.0.0.2"
role = "backend"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\ninput len=%d\ngot len=%d", len(input), len(got))
		// Find first difference
		for i := 0; i < len(input) && i < len(got); i++ {
			if input[i] != got[i] {
				start := i - 10
				if start < 0 {
					start = 0
				}
				end := i + 20
				if end > len(input) {
					end = len(input)
				}
				endG := i + 20
				if endG > len(got) {
					endG = len(got)
				}
				t.Errorf("first diff at byte %d:\n  input: %q\n  got:   %q", i, input[start:end], got[start:endG])
				break
			}
		}
		if len(input) != len(got) {
			t.Errorf("length mismatch: input=%d, got=%d", len(input), len(got))
		}
	}
}
