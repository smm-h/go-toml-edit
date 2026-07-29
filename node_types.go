package tomledit

import "time"

// StringStyle indicates the quoting style for a string node.
type StringStyle int

const (
	StringBasic            StringStyle = iota // StringBasic is a double-quoted string ("...").
	StringLiteral                             // StringLiteral is a single-quoted string ('...').
	StringMultiLineBasic                      // StringMultiLineBasic is a triple-double-quoted string ("""...""").
	StringMultiLineLiteral                    // StringMultiLineLiteral is a triple-single-quoted string ('''...''').
)

// IntegerBase indicates the numeric base for an integer node.
type IntegerBase int

const (
	IntegerDecimal IntegerBase = iota // IntegerDecimal is base-10 (e.g. 42).
	IntegerHex                        // IntegerHex is base-16 (e.g. 0xFF).
	IntegerOctal                      // IntegerOctal is base-8 (e.g. 0o77).
	IntegerBinary                     // IntegerBinary is base-2 (e.g. 0b1010).
)

// DocumentNode is the root node of a TOML document.
type DocumentNode struct {
	nodeBase
	Children []Node
}

// Type returns NodeDocument.
func (n *DocumentNode) Type() NodeType { return NodeDocument }

// Value returns the document's child nodes.
func (n *DocumentNode) Value() any { return n.Children }

// TableNode represents a [table] header and its children.
type TableNode struct {
	nodeBase
	KeyPath  []string
	Children []Node
}

// Type returns NodeTable.
func (n *TableNode) Type() NodeType { return NodeTable }

// Value returns the table's child nodes.
func (n *TableNode) Value() any { return n.Children }

// ArrayTableNode represents an [[array-table]] header and its children.
type ArrayTableNode struct {
	nodeBase
	KeyPath  []string
	Children []Node
}

// Type returns NodeArrayTable.
func (n *ArrayTableNode) Type() NodeType { return NodeArrayTable }

// Value returns the array table's child nodes.
func (n *ArrayTableNode) Value() any { return n.Children }

// KeyValueNode represents a key = value pair.
type KeyValueNode struct {
	nodeBase
	Key   *KeyNode
	Val   Node
}

// Type returns NodeKeyValue.
func (n *KeyValueNode) Type() NodeType { return NodeKeyValue }

// Value returns the value node of this key-value pair.
func (n *KeyValueNode) Value() any { return n.Val }

// KeyNode represents a (possibly dotted) key.
type KeyNode struct {
	nodeBase
	Parts    []string   // semantic parts (e.g. ["server", "host"])
	RawParts [][]byte   // original bytes for each part
	Styles   []StringStyle // quoting style per part
}

// Type returns NodeKey.
func (n *KeyNode) Type() NodeType { return NodeKey }

// Value returns the key's decoded parts.
func (n *KeyNode) Value() any { return n.Parts }

// StringNode represents a string value.
type StringNode struct {
	nodeBase
	Val   string
	Style StringStyle
}

// Type returns NodeString.
func (n *StringNode) Type() NodeType { return NodeString }

// Value returns the decoded string.
func (n *StringNode) Value() any { return n.Val }

// IntegerNode represents an integer value.
type IntegerNode struct {
	nodeBase
	Val  int64
	Base IntegerBase
}

// Type returns NodeInteger.
func (n *IntegerNode) Type() NodeType { return NodeInteger }

// Value returns the integer value as int64.
func (n *IntegerNode) Value() any { return n.Val }

// FloatNode represents a float value.
type FloatNode struct {
	nodeBase
	Val float64
}

// Type returns NodeFloat.
func (n *FloatNode) Type() NodeType { return NodeFloat }

// Value returns the float value as float64.
func (n *FloatNode) Value() any { return n.Val }

// BooleanNode represents a boolean value.
type BooleanNode struct {
	nodeBase
	Val bool
}

// Type returns NodeBoolean.
func (n *BooleanNode) Type() NodeType { return NodeBoolean }

// Value returns the boolean value.
func (n *BooleanNode) Value() any { return n.Val }

// DateTimeNode represents an offset date-time value.
type DateTimeNode struct {
	nodeBase
	Val time.Time
}

// Type returns NodeDateTime.
func (n *DateTimeNode) Type() NodeType { return NodeDateTime }

// Value returns the time.Time value.
func (n *DateTimeNode) Value() any { return n.Val }

// LocalDateTimeNode represents a local date-time value (no timezone).
type LocalDateTimeNode struct {
	nodeBase
	Val LocalDateTime
}

// Type returns NodeLocalDateTime.
func (n *LocalDateTimeNode) Type() NodeType { return NodeLocalDateTime }

// Value returns the LocalDateTime value.
func (n *LocalDateTimeNode) Value() any { return n.Val }

// LocalDateNode represents a local date value.
type LocalDateNode struct {
	nodeBase
	Val LocalDate
}

// Type returns NodeLocalDate.
func (n *LocalDateNode) Type() NodeType { return NodeLocalDate }

// Value returns the LocalDate value.
func (n *LocalDateNode) Value() any { return n.Val }

// LocalTimeNode represents a local time value.
type LocalTimeNode struct {
	nodeBase
	Val LocalTime
}

// Type returns NodeLocalTime.
func (n *LocalTimeNode) Type() NodeType { return NodeLocalTime }

// Value returns the LocalTime value.
func (n *LocalTimeNode) Value() any { return n.Val }

// ArrayNode represents an array value.
type ArrayNode struct {
	nodeBase
	Elements         []Node
	TrailingComments [][]byte // comments after the last element, before ']'
}

// Type returns NodeArray.
func (n *ArrayNode) Type() NodeType { return NodeArray }

// Value returns the array's elements.
func (n *ArrayNode) Value() any { return n.Elements }

// InlineTableNode represents an inline table value.
type InlineTableNode struct {
	nodeBase
	Children []Node // KeyValueNode entries
}

// Type returns NodeInlineTable.
func (n *InlineTableNode) Type() NodeType { return NodeInlineTable }

// Value returns the inline table's child nodes.
func (n *InlineTableNode) Value() any { return n.Children }

// CommentNode represents a standalone comment line.
type CommentNode struct {
	nodeBase
	Text string
}

// Type returns NodeComment.
func (n *CommentNode) Type() NodeType { return NodeComment }

// Value returns the comment text.
func (n *CommentNode) Value() any { return n.Text }
