package tomledit

import (
	"testing"
	"time"
)

func TestNodeTypes(t *testing.T) {
	tests := []struct {
		name     string
		node     Node
		wantType NodeType
		checkVal func(t *testing.T, n Node)
	}{
		{
			name:     "Document",
			node:     &Document{children: []Node{&CommentNode{text: "hi"}}},
			wantType: NodeDocument,
			checkVal: func(t *testing.T, n Node) {
				children := n.(*Document).children
				if len(children) != 1 {
					t.Errorf("expected 1 child, got %d", len(children))
				}
			},
		},
		{
			name:     "TableNode",
			node:     &TableNode{keyPath: []string{"server"}, children: nil},
			wantType: NodeTable,
			checkVal: func(t *testing.T, n Node) {
				children := n.(*TableNode).children
				if len(children) != 0 {
					t.Errorf("expected 0 children, got %d", len(children))
				}
			},
		},
		{
			name:     "ArrayTableNode",
			node:     &ArrayTableNode{keyPath: []string{"products"}},
			wantType: NodeArrayTable,
		},
		{
			name: "KeyValueNode",
			node: &KeyValueNode{
				key: &KeyNode{parts: []string{"name"}},
				val: &StringNode{val: scalarOf("hello")},
			},
			wantType: NodeKeyValue,
			checkVal: func(t *testing.T, n Node) {
				valNode := n.(*KeyValueNode).val
				if valNode.Type() != NodeString {
					t.Errorf("expected String value node, got %v", valNode.Type())
				}
			},
		},
		{
			name:     "KeyNode",
			node:     &KeyNode{parts: []string{"server", "host"}, rawParts: [][]byte{[]byte("server"), []byte("host")}},
			wantType: NodeKey,
			checkVal: func(t *testing.T, n Node) {
				parts := n.(*KeyNode).parts
				if len(parts) != 2 || parts[0] != "server" || parts[1] != "host" {
					t.Errorf("unexpected parts: %v", parts)
				}
			},
		},
		{
			name:     "StringNode/Basic",
			node:     &StringNode{val: scalarOf("hello"), style: StringBasic},
			wantType: NodeString,
			checkVal: func(t *testing.T, n Node) {
				v := n.(Scalar).Value()
				if v.(string) != "hello" {
					t.Errorf("expected hello, got %v", v)
				}
			},
		},
		{
			name:     "StringNode/Literal",
			node:     &StringNode{val: scalarOf(`C:\path`), style: StringLiteral},
			wantType: NodeString,
			checkVal: func(t *testing.T, n Node) {
				v := n.(Scalar).Value()
				if v.(string) != `C:\path` {
					t.Errorf("expected C:\\path, got %v", v)
				}
			},
		},
		{
			name:     "IntegerNode/Decimal",
			node:     &IntegerNode{val: scalarOf[int64](42), base: IntegerDecimal},
			wantType: NodeInteger,
			checkVal: func(t *testing.T, n Node) {
				v := n.(Scalar).Value()
				if v.(int64) != 42 {
					t.Errorf("expected 42, got %v", v)
				}
			},
		},
		{
			name:     "IntegerNode/Hex",
			node:     &IntegerNode{val: scalarOf[int64](0xDEAD), base: IntegerHex},
			wantType: NodeInteger,
			checkVal: func(t *testing.T, n Node) {
				v := n.(Scalar).Value()
				if v.(int64) != 0xDEAD {
					t.Errorf("expected 0xDEAD, got %v", v)
				}
			},
		},
		{
			name:     "FloatNode",
			node:     &FloatNode{val: scalarOf(3.14)},
			wantType: NodeFloat,
			checkVal: func(t *testing.T, n Node) {
				v := n.(Scalar).Value()
				if v.(float64) != 3.14 {
					t.Errorf("expected 3.14, got %v", v)
				}
			},
		},
		{
			name:     "BooleanNode",
			node:     &BooleanNode{val: scalarOf(true)},
			wantType: NodeBoolean,
			checkVal: func(t *testing.T, n Node) {
				v := n.(Scalar).Value()
				if v.(bool) != true {
					t.Errorf("expected true, got %v", v)
				}
			},
		},
		{
			name:     "DateTimeNode",
			node:     &DateTimeNode{val: scalarOf(time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC))},
			wantType: NodeDateTime,
			checkVal: func(t *testing.T, n Node) {
				v := n.(Scalar).Value()
				dt := v.(time.Time)
				if dt.Year() != 1979 {
					t.Errorf("expected year 1979, got %d", dt.Year())
				}
			},
		},
		{
			name:     "LocalDateTimeNode",
			node:     &LocalDateTimeNode{val: scalarOf(LocalDateTime{Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32})},
			wantType: NodeLocalDateTime,
			checkVal: func(t *testing.T, n Node) {
				v := n.(Scalar).Value()
				ldt := v.(LocalDateTime)
				if ldt.Year != 1979 || ldt.Month != 5 {
					t.Errorf("unexpected local datetime: %+v", ldt)
				}
			},
		},
		{
			name:     "LocalDateNode",
			node:     &LocalDateNode{val: scalarOf(LocalDate{Year: 1979, Month: 5, Day: 27})},
			wantType: NodeLocalDate,
			checkVal: func(t *testing.T, n Node) {
				v := n.(Scalar).Value()
				ld := v.(LocalDate)
				if ld.Year != 1979 {
					t.Errorf("expected year 1979, got %d", ld.Year)
				}
			},
		},
		{
			name:     "LocalTimeNode",
			node:     &LocalTimeNode{val: scalarOf(LocalTime{Hour: 7, Minute: 32, Second: 0})},
			wantType: NodeLocalTime,
			checkVal: func(t *testing.T, n Node) {
				v := n.(Scalar).Value()
				lt := v.(LocalTime)
				if lt.Hour != 7 || lt.Minute != 32 {
					t.Errorf("unexpected local time: %+v", lt)
				}
			},
		},
		{
			name:     "ArrayNode",
			node:     &ArrayNode{elements: []Node{&IntegerNode{val: scalarOf[int64](1)}, &IntegerNode{val: scalarOf[int64](2)}}},
			wantType: NodeArray,
			checkVal: func(t *testing.T, n Node) {
				elems := n.(*ArrayNode).elements
				if len(elems) != 2 {
					t.Errorf("expected 2 elements, got %d", len(elems))
				}
			},
		},
		{
			name:     "InlineTableNode",
			node:     &InlineTableNode{children: []Node{&KeyValueNode{key: &KeyNode{parts: []string{"a"}}, val: &IntegerNode{val: scalarOf[int64](1)}}}},
			wantType: NodeInlineTable,
			checkVal: func(t *testing.T, n Node) {
				children := n.(*InlineTableNode).children
				if len(children) != 1 {
					t.Errorf("expected 1 child, got %d", len(children))
				}
			},
		},
		{
			name:     "CommentNode",
			node:     &CommentNode{text: "# this is a comment"},
			wantType: NodeComment,
			checkVal: func(t *testing.T, n Node) {
				if v := n.(*CommentNode).text; v != "# this is a comment" {
					t.Errorf("unexpected comment text: %v", v)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.Type(); got != tt.wantType {
				t.Errorf("Type() = %v, want %v", got, tt.wantType)
			}
			if tt.checkVal != nil {
				tt.checkVal(t, tt.node)
			}
		})
	}
}

func TestTrivia(t *testing.T) {
	n := &StringNode{val: scalarOf("hello")}

	// Initially empty
	if got := n.Comment(); got != "" {
		t.Errorf("Comment() = %q, want empty", got)
	}
	if got := n.LeadingComments(); len(got) != 0 {
		t.Errorf("LeadingComments() = %v, want empty", got)
	}

	// Set inline comment. The getter answers the comment's text, not its
	// bytes: the "#" and the whitespace around it are the spelling.
	n.setComment("# inline")
	if got := n.Comment(); got != "inline" {
		t.Errorf("Comment() = %q, want %q", got, "inline")
	}

	// Set leading comments
	n.setLeadingComments([]string{"# line 1\n", "# line 2\n"})
	lc := n.LeadingComments()
	if len(lc) != 2 || lc[0] != "line 1" || lc[1] != "line 2" {
		t.Errorf("LeadingComments() = %v, unexpected", lc)
	}
}

func TestDirtyFlag(t *testing.T) {
	n := &IntegerNode{val: scalarOf[int64](42)}

	// Initially clean
	if n.isDirty() {
		t.Error("new node should not be dirty")
	}

	// Setting raw does not make dirty
	n.setRaw([]byte("42"))
	if n.isDirty() {
		t.Error("setRaw should not make dirty")
	}
	if string(n.Raw()) != "42" {
		t.Errorf("Raw() = %q, want %q", n.Raw(), "42")
	}

	// setComment marks dirty
	n2 := &IntegerNode{val: scalarOf[int64](10)}
	n2.setComment("# ten")
	if !n2.isDirty() {
		t.Error("setComment should mark dirty")
	}

	// setLeadingComments marks dirty
	n3 := &IntegerNode{val: scalarOf[int64](20)}
	n3.setLeadingComments([]string{"# twenty"})
	if !n3.isDirty() {
		t.Error("setLeadingComments should mark dirty")
	}

	// markDirty directly
	n4 := &IntegerNode{val: scalarOf[int64](30)}
	n4.markDirty()
	if !n4.isDirty() {
		t.Error("markDirty should mark dirty")
	}
}

func TestTriviaAccess(t *testing.T) {
	n := &StringNode{val: scalarOf("test")}
	tr := n.trivia()
	if tr == nil {
		t.Fatal("trivia() returned nil")
	}
	tr.LeadingWhitespace = []byte("  ")
	tr.TrailingNewline = []byte("\n")

	// Verify mutation through pointer
	if string(n.trivia().LeadingWhitespace) != "  " {
		t.Error("trivia pointer should allow mutation")
	}
	if string(n.trivia().TrailingNewline) != "\n" {
		t.Error("trailing newline not set correctly")
	}
}

func TestNodeTypeString(t *testing.T) {
	tests := []struct {
		nt   NodeType
		want string
	}{
		{NodeDocument, "Document"},
		{NodeTable, "Table"},
		{NodeArrayTable, "ArrayTable"},
		{NodeKeyValue, "KeyValue"},
		{NodeKey, "Key"},
		{NodeString, "String"},
		{NodeInteger, "Integer"},
		{NodeFloat, "Float"},
		{NodeBoolean, "Boolean"},
		{NodeDateTime, "DateTime"},
		{NodeLocalDateTime, "LocalDateTime"},
		{NodeLocalDate, "LocalDate"},
		{NodeLocalTime, "LocalTime"},
		{NodeArray, "Array"},
		{NodeInlineTable, "InlineTable"},
		{NodeComment, "Comment"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.nt.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}

	// Unknown type
	if got := NodeType(999).String(); got != "Unknown" {
		t.Errorf("unknown String() = %q, want %q", got, "Unknown")
	}
}
