package tomledit

import "testing"

// spanTestDoc is a representative document exercising every node type:
// scalars of all kinds, arrays, inline tables, multiline strings, nested
// tables, arrays of tables, and comments.
const spanTestDoc = `# header comment
title = "hello"
count = 42
pi = 3.14
ok = true
date = 2024-01-15T10:30:00Z
ldt = 2024-01-15T10:30:00
ld = 2024-01-15
lt = 10:30:00
tags = [1, 2, 3]
point = { x = 1, y = 2 }
desc = """
multi
line"""

[server]
host = "localhost"

[server.tls]
enabled = false

[[products]]
name = "a"

[[products]]
name = "b"

# trailing comment
`

func mustParseSpanDoc(t *testing.T) *Document {
	t.Helper()
	doc, err := Parse([]byte(spanTestDoc))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	return doc
}

// assertSpan checks the line/column endpoints of a span. The byte offsets of
// the same endpoints are covered by the offset tests at the bottom of this
// file, which assert they agree with the line/column pair for every node.
func assertSpan(t *testing.T, label string, got Span, startLine, startCol, endLine, endCol int) {
	t.Helper()
	if got.Start.Line != startLine || got.Start.Column != startCol ||
		got.End.Line != endLine || got.End.Column != endCol {
		t.Errorf("%s: span = %d:%d-%d:%d, want %d:%d-%d:%d",
			label,
			got.Start.Line, got.Start.Column, got.End.Line, got.End.Column,
			startLine, startCol, endLine, endCol)
	}
}

// rootKV returns the i-th root-level KeyValueNode of the span test document.
func rootKV(t *testing.T, doc *Document, i int) *KeyValueNode {
	t.Helper()
	kv, ok := doc.Children[i].(*KeyValueNode)
	if !ok {
		t.Fatalf("doc.Children[%d] is %T, want *KeyValueNode", i, doc.Children[i])
	}
	return kv
}

func TestSpanScalarValues(t *testing.T) {
	doc := mustParseSpanDoc(t)

	tests := []struct {
		name     string
		index    int
		nodeType NodeType
		// key span
		kStartLine, kStartCol, kEndLine, kEndCol int
		// value span
		vStartLine, vStartCol, vEndLine, vEndCol int
		// key-value span
		kvStartLine, kvStartCol, kvEndLine, kvEndCol int
	}{
		{"title string", 0, NodeString, 2, 1, 2, 6, 2, 9, 2, 16, 2, 1, 2, 16},
		{"count integer", 1, NodeInteger, 3, 1, 3, 6, 3, 9, 3, 11, 3, 1, 3, 11},
		{"pi float", 2, NodeFloat, 4, 1, 4, 3, 4, 6, 4, 10, 4, 1, 4, 10},
		{"ok boolean", 3, NodeBoolean, 5, 1, 5, 3, 5, 6, 5, 10, 5, 1, 5, 10},
		{"date offset date-time", 4, NodeDateTime, 6, 1, 6, 5, 6, 8, 6, 28, 6, 1, 6, 28},
		{"ldt local date-time", 5, NodeLocalDateTime, 7, 1, 7, 4, 7, 7, 7, 26, 7, 1, 7, 26},
		{"ld local date", 6, NodeLocalDate, 8, 1, 8, 3, 8, 6, 8, 16, 8, 1, 8, 16},
		{"lt local time", 7, NodeLocalTime, 9, 1, 9, 3, 9, 6, 9, 14, 9, 1, 9, 14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := rootKV(t, doc, tt.index)
			if kv.Val.Type() != tt.nodeType {
				t.Fatalf("value type = %s, want %s", kv.Val.Type(), tt.nodeType)
			}
			assertSpan(t, "key", kv.Key.Span(), tt.kStartLine, tt.kStartCol, tt.kEndLine, tt.kEndCol)
			assertSpan(t, "value", kv.Val.Span(), tt.vStartLine, tt.vStartCol, tt.vEndLine, tt.vEndCol)
			assertSpan(t, "key-value", kv.Span(), tt.kvStartLine, tt.kvStartCol, tt.kvEndLine, tt.kvEndCol)
		})
	}
}

func TestSpanMultilineString(t *testing.T) {
	doc := mustParseSpanDoc(t)
	kv := rootKV(t, doc, 10) // desc = """ ... """
	if kv.Val.Type() != NodeString {
		t.Fatalf("value type = %s, want String", kv.Val.Type())
	}
	sn := kv.Val.(*StringNode)
	if sn.Style != StringMultiLineBasic {
		t.Fatalf("style = %v, want StringMultiLineBasic", sn.Style)
	}
	assertSpan(t, "key", kv.Key.Span(), 12, 1, 12, 5)
	assertSpan(t, "value", kv.Val.Span(), 12, 8, 14, 8)
	assertSpan(t, "key-value", kv.Span(), 12, 1, 14, 8)
}

func TestSpanArray(t *testing.T) {
	doc := mustParseSpanDoc(t)
	kv := rootKV(t, doc, 8) // tags = [1, 2, 3]
	arr, ok := kv.Val.(*ArrayNode)
	if !ok {
		t.Fatalf("value is %T, want *ArrayNode", kv.Val)
	}
	assertSpan(t, "array", arr.Span(), 10, 8, 10, 17)
	if len(arr.Elements) != 3 {
		t.Fatalf("len(Elements) = %d, want 3", len(arr.Elements))
	}
	assertSpan(t, "element 0", arr.Elements[0].Span(), 10, 9, 10, 10)
	assertSpan(t, "element 1", arr.Elements[1].Span(), 10, 12, 10, 13)
	assertSpan(t, "element 2", arr.Elements[2].Span(), 10, 15, 10, 16)
	assertSpan(t, "key-value", kv.Span(), 10, 1, 10, 17)
}

func TestSpanInlineTable(t *testing.T) {
	doc := mustParseSpanDoc(t)
	kv := rootKV(t, doc, 9) // point = { x = 1, y = 2 }
	it, ok := kv.Val.(*InlineTableNode)
	if !ok {
		t.Fatalf("value is %T, want *InlineTableNode", kv.Val)
	}
	assertSpan(t, "inline table", it.Span(), 11, 9, 11, 25)
	if len(it.Children) != 2 {
		t.Fatalf("len(Children) = %d, want 2", len(it.Children))
	}
	x := it.Children[0].(*KeyValueNode)
	assertSpan(t, "x key", x.Key.Span(), 11, 11, 11, 12)
	assertSpan(t, "x value", x.Val.Span(), 11, 15, 11, 16)
	assertSpan(t, "x key-value", x.Span(), 11, 11, 11, 16)
	y := it.Children[1].(*KeyValueNode)
	assertSpan(t, "y key-value", y.Span(), 11, 18, 11, 23)
}

func TestSpanTableHeaders(t *testing.T) {
	doc := mustParseSpanDoc(t)

	server, ok := doc.Children[11].(*TableNode)
	if !ok {
		t.Fatalf("doc.Children[11] is %T, want *TableNode", doc.Children[11])
	}
	assertSpan(t, "[server]", server.Span(), 16, 1, 16, 9)

	host := server.Children[0].(*KeyValueNode)
	assertSpan(t, "host key-value", host.Span(), 17, 1, 17, 19)
	assertSpan(t, "host value", host.Val.Span(), 17, 8, 17, 19)

	tls, ok := doc.Children[12].(*TableNode)
	if !ok {
		t.Fatalf("doc.Children[12] is %T, want *TableNode", doc.Children[12])
	}
	assertSpan(t, "[server.tls]", tls.Span(), 19, 1, 19, 13)

	enabled := tls.Children[0].(*KeyValueNode)
	assertSpan(t, "enabled key-value", enabled.Span(), 20, 1, 20, 16)
}

func TestSpanArrayTableHeaders(t *testing.T) {
	doc := mustParseSpanDoc(t)

	first, ok := doc.Children[13].(*ArrayTableNode)
	if !ok {
		t.Fatalf("doc.Children[13] is %T, want *ArrayTableNode", doc.Children[13])
	}
	assertSpan(t, "first [[products]]", first.Span(), 22, 1, 22, 13)
	name1 := first.Children[0].(*KeyValueNode)
	assertSpan(t, "first name", name1.Span(), 23, 1, 23, 11)

	second, ok := doc.Children[14].(*ArrayTableNode)
	if !ok {
		t.Fatalf("doc.Children[14] is %T, want *ArrayTableNode", doc.Children[14])
	}
	assertSpan(t, "second [[products]]", second.Span(), 25, 1, 25, 13)
	name2 := second.Children[0].(*KeyValueNode)
	assertSpan(t, "second name", name2.Span(), 26, 1, 26, 11)
}

func TestSpanTrailingComment(t *testing.T) {
	doc := mustParseSpanDoc(t)
	// The trailing comment is an orphan CommentNode attached to the last
	// array-table entry.
	second := doc.Children[14].(*ArrayTableNode)
	var comment *CommentNode
	for _, c := range second.Children {
		if cn, ok := c.(*CommentNode); ok {
			comment = cn
		}
	}
	if comment == nil {
		t.Fatalf("no CommentNode found in last array-table children")
	}
	assertSpan(t, "trailing comment", comment.Span(), 28, 1, 28, 19)
}

func TestSpanDocument(t *testing.T) {
	doc := mustParseSpanDoc(t)
	assertSpan(t, "document", doc.Span(), 1, 1, 29, 1)
}

func TestSpanDottedKey(t *testing.T) {
	doc, err := Parse([]byte("srv.host = 1\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	kv := doc.Children[0].(*KeyValueNode)
	assertSpan(t, "dotted key", kv.Key.Span(), 1, 1, 1, 9)
	assertSpan(t, "key-value", kv.Span(), 1, 1, 1, 13)
}

func TestSpanCRLF(t *testing.T) {
	doc, err := Parse([]byte("a = 1\r\nb = 2\r\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	b := doc.Children[1].(*KeyValueNode)
	assertSpan(t, "b key", b.Key.Span(), 2, 1, 2, 2)
	assertSpan(t, "b value", b.Val.Span(), 2, 5, 2, 6)
}

func TestSpanBlankTrailingTrivia(t *testing.T) {
	doc, err := Parse([]byte("a = 1\n\n\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	var comment *CommentNode
	for _, c := range doc.Children {
		if cn, ok := c.(*CommentNode); ok {
			comment = cn
		}
	}
	if comment == nil {
		t.Fatalf("no CommentNode found for trailing blank lines")
	}
	// Blank-line orphan trivia spans the consumed blank region.
	assertSpan(t, "blank trivia", comment.Span(), 2, 1, 4, 1)
}

// TestSpanWalkRetrieval proves that Walk callbacks can retrieve positions for
// every visited node.
func TestSpanWalkRetrieval(t *testing.T) {
	doc := mustParseSpanDoc(t)

	lines := map[string]int{}
	err := doc.Walk(func(path string, node Node) error {
		sp := node.Span()
		if !sp.IsValid() {
			t.Errorf("node at %q has invalid span", path)
		}
		lines[path] = sp.Start.Line
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	want := map[string]int{
		"title":              2,
		"tags":               10,
		"tags[0]":            10,
		"point":              11,
		"point.x":            11,
		"server.host":        17,
		"server.tls.enabled": 20,
		"products[0].name":   23,
		"products[1].name":   26,
	}
	for path, line := range want {
		got, ok := lines[path]
		if !ok {
			t.Errorf("path %q not visited by Walk", path)
			continue
		}
		if got != line {
			t.Errorf("path %q: start line = %d, want %d", path, got, line)
		}
	}
}

// TestSpanEditPolicy verifies the documented policy: spans reflect the most
// recent Parse. Nodes created by edits carry the zero (invalid) span, and
// untouched nodes keep their parse-time spans.
func TestSpanEditPolicy(t *testing.T) {
	doc, err := Parse([]byte("a = 1\nb = 2\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if err := doc.Set("a", 42); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// The replacement value node was created programmatically: invalid span.
	aVal, ok := doc.Lookup("a")
	if !ok {
		t.Fatalf("Lookup(a) returned nil")
	}
	if aVal.Span().IsValid() {
		t.Errorf("edited value span = %+v, want invalid (zero) span", aVal.Span())
	}

	// Untouched nodes keep their parse-time spans.
	bVal, ok := doc.Lookup("b")
	if !ok {
		t.Fatalf("Lookup(b) returned nil")
	}
	assertSpan(t, "untouched value", bVal.Span(), 2, 5, 2, 6)
}

// --- byte offsets ---

// offsetOfLineColumn converts a 1-based line/column pair into the 0-based byte
// offset of the same point in src, by the lexer's own advancement rules.
func offsetOfLineColumn(src []byte, line, column int) int {
	curLine, curCol := 1, 1
	for i := 0; i <= len(src); i++ {
		if curLine == line && curCol == column {
			return i
		}
		if i == len(src) {
			break
		}
		if src[i] == '\n' {
			curLine++
			curCol = 1
		} else {
			curCol++
		}
	}
	return -1
}

// Fails if any node's span offsets stop agreeing with its line/column
// endpoints -- the condition a span-construction site that forgets to carry
// the offset produces.
func TestSpanOffsetsAgreeWithLineColumn(t *testing.T) {
	src := []byte(spanTestDoc)
	doc := mustParseSpanDoc(t)

	check := func(label string, sp Span) {
		t.Helper()
		if !sp.IsValid() {
			return
		}
		if want := offsetOfLineColumn(src, sp.Start.Line, sp.Start.Column); sp.Start.Offset != want {
			t.Errorf("%s: start offset = %d, want %d (line %d, column %d)",
				label, sp.Start.Offset, want, sp.Start.Line, sp.Start.Column)
		}
		if want := offsetOfLineColumn(src, sp.End.Line, sp.End.Column); sp.End.Offset != want {
			t.Errorf("%s: end offset = %d, want %d (line %d, column %d)",
				label, sp.End.Offset, want, sp.End.Line, sp.End.Column)
		}
	}

	check("document", doc.Span())
	err := doc.Walk(func(path string, node Node) error {
		check(path, node.Span())
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
}

// Fails if a span stops slicing the exact source bytes of the construct it
// covers -- the offsets are only useful if src[start:end] is the construct.
func TestSpanOffsetsSliceTheSource(t *testing.T) {
	src := []byte(spanTestDoc)
	doc := mustParseSpanDoc(t)

	tests := []struct {
		label string
		span  Span
		want  string
	}{
		{"title key", rootKV(t, doc, 0).Key.Span(), "title"},
		{"title value", rootKV(t, doc, 0).Val.Span(), `"hello"`},
		{"count key-value", rootKV(t, doc, 1).Span(), "count = 42"},
		{"tags array", rootKV(t, doc, 8).Val.Span(), "[1, 2, 3]"},
		{"tags element 1", rootKV(t, doc, 8).Val.(*ArrayNode).Elements[1].Span(), "2"},
		{"point inline table", rootKV(t, doc, 9).Val.Span(), "{ x = 1, y = 2 }"},
		{"desc multiline value", rootKV(t, doc, 10).Val.Span(), "\"\"\"\nmulti\nline\"\"\""},
		{"[server] header", doc.Children[11].Span(), "[server]"},
		{"[server.tls] header", doc.Children[12].Span(), "[server.tls]"},
		{"[[products]] header", doc.Children[13].Span(), "[[products]]"},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			sp := tt.span
			if !sp.IsValid() {
				t.Fatalf("span is invalid")
			}
			if sp.Start.Offset < 0 || sp.End.Offset > len(src) || sp.Start.Offset > sp.End.Offset {
				t.Fatalf("offsets out of range: %d..%d (source is %d bytes)", sp.Start.Offset, sp.End.Offset, len(src))
			}
			if got := string(src[sp.Start.Offset:sp.End.Offset]); got != tt.want {
				t.Errorf("src[%d:%d] = %q, want %q", sp.Start.Offset, sp.End.Offset, got, tt.want)
			}
		})
	}
}

// Fails if CRLF line endings stop being counted in offsets (the \r is a byte
// of the source even though it never advances the line).
func TestSpanOffsetsCRLF(t *testing.T) {
	src := []byte("a = 1\r\nb = 2\r\n")
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	b := doc.Children[1].(*KeyValueNode)
	if got := string(src[b.Span().Start.Offset:b.Span().End.Offset]); got != "b = 2" {
		t.Errorf("second key-value slices %q, want %q", got, "b = 2")
	}
}

// Fails if the document span stops ending at the last byte of the source.
func TestSpanOffsetsDocumentCoversSource(t *testing.T) {
	src := []byte(spanTestDoc)
	doc := mustParseSpanDoc(t)
	sp := doc.Span()
	if sp.Start.Offset != 0 {
		t.Errorf("document start offset = %d, want 0", sp.Start.Offset)
	}
	if sp.End.Offset != len(src) {
		t.Errorf("document end offset = %d, want %d (the source length)", sp.End.Offset, len(src))
	}
}

func TestSpanZeroInvalid(t *testing.T) {
	var s Span
	if s.IsValid() {
		t.Errorf("zero Span must be invalid")
	}
	var p Position
	if p.IsValid() {
		t.Errorf("zero Position must be invalid")
	}
	doc, err := Parse([]byte("a = 1\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !doc.Span().IsValid() {
		t.Errorf("parsed document span must be valid")
	}
}
