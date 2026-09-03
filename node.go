package tomledit

// NodeType identifies the kind of AST node.
type NodeType int

const (
	NodeDocument      NodeType = iota // NodeDocument is the root document node.
	NodeTable                         // NodeTable is a [table] header node.
	NodeArrayTable                    // NodeArrayTable is an [[array-table]] header node.
	NodeKeyValue                      // NodeKeyValue is a key = value pair.
	NodeKey                           // NodeKey is a (possibly dotted) key.
	NodeString                        // NodeString is a string value.
	NodeInteger                       // NodeInteger is an integer value.
	NodeFloat                         // NodeFloat is a float value.
	NodeBoolean                       // NodeBoolean is a boolean value.
	NodeDateTime                      // NodeDateTime is an offset date-time value.
	NodeLocalDateTime                 // NodeLocalDateTime is a local date-time value (no timezone).
	NodeLocalDate                     // NodeLocalDate is a local date value.
	NodeLocalTime                     // NodeLocalTime is a local time value.
	NodeArray                         // NodeArray is an array value.
	NodeInlineTable                   // NodeInlineTable is an inline table value.
	NodeComment                       // NodeComment is a standalone comment line.
)

var nodeTypeNames = [...]string{
	NodeDocument:      "Document",
	NodeTable:         "Table",
	NodeArrayTable:    "ArrayTable",
	NodeKeyValue:      "KeyValue",
	NodeKey:           "Key",
	NodeString:        "String",
	NodeInteger:       "Integer",
	NodeFloat:         "Float",
	NodeBoolean:       "Boolean",
	NodeDateTime:      "DateTime",
	NodeLocalDateTime: "LocalDateTime",
	NodeLocalDate:     "LocalDate",
	NodeLocalTime:     "LocalTime",
	NodeArray:         "Array",
	NodeInlineTable:   "InlineTable",
	NodeComment:       "Comment",
}

// String returns the human-readable name of the node type.
func (n NodeType) String() string {
	if int(n) >= 0 && int(n) < len(nodeTypeNames) {
		return nodeTypeNames[n]
	}
	return "Unknown"
}

// Trivia holds formatting and comment data attached to a node.
type Trivia struct {
	LeadingWhitespace []byte
	LeadingComments   [][]byte // each is a full "# ...\n" line
	InlineComment     []byte   // "# ..." after value on same line
	TrailingNewline   []byte
}

// Node is the interface implemented by all AST nodes.
// Every node carries its original raw bytes, trivia (whitespace and comments),
// and a semantic value accessible via Value. Implementation is restricted to
// this package.
type Node interface {
	Type() NodeType
	Value() any
	Comment() string
	LeadingComments() []string
	Raw() []byte

	// Span returns the source range the node covered in the most recent
	// Parse. Nodes created programmatically (edits, Marshal) return the
	// zero Span (IsValid reports false). Edits do not update spans; see
	// the Span type documentation for the exact policy.
	Span() Span

	// unexported methods restrict implementation to this package
	setComment(comment string)
	setLeadingComments(comments []string)
	setRaw([]byte)
	setSpan(Span)
	isDirty() bool
	markDirty()
	trivia() *Trivia
}

// nodeBase provides the shared implementation for all concrete node types.
type nodeBase struct {
	raw        []byte
	dirty      bool
	nodeTrivia Trivia
	span       Span
}

func (n *nodeBase) Raw() []byte {
	return n.raw
}

func (n *nodeBase) setRaw(b []byte) {
	n.raw = b
}

func (n *nodeBase) Span() Span {
	return n.span
}

func (n *nodeBase) setSpan(s Span) {
	n.span = s
}

func (n *nodeBase) isDirty() bool {
	return n.dirty
}

func (n *nodeBase) markDirty() {
	n.dirty = true
}

func (n *nodeBase) trivia() *Trivia {
	return &n.nodeTrivia
}

func (n *nodeBase) Comment() string {
	return string(n.nodeTrivia.InlineComment)
}

func (n *nodeBase) SetComment(comment string) {
	n.nodeTrivia.InlineComment = []byte(comment)
	n.dirty = true
}

func (n *nodeBase) setComment(comment string) {
	n.SetComment(comment)
}

func (n *nodeBase) LeadingComments() []string {
	result := make([]string, len(n.nodeTrivia.LeadingComments))
	for i, c := range n.nodeTrivia.LeadingComments {
		result[i] = string(c)
	}
	return result
}

func (n *nodeBase) SetLeadingComments(comments []string) {
	n.nodeTrivia.LeadingComments = make([][]byte, len(comments))
	for i, c := range comments {
		n.nodeTrivia.LeadingComments[i] = []byte(c)
	}
	n.dirty = true
}

func (n *nodeBase) setLeadingComments(comments []string) {
	n.SetLeadingComments(comments)
}

// nullNode provides no-op implementations of all Node interface methods.
// Internal virtual node types (dottedKeyView, dottedKeyGroup,
// compoundTableView, arrayTableCollection) embed nullNode and override
// only Type() and Value().
type nullNode struct{}

func (nullNode) Type() NodeType              { return NodeType(-1) }
func (nullNode) Value() any                  { return nil }
func (nullNode) Comment() string             { return "" }
func (nullNode) SetComment(string)           {}
func (nullNode) setComment(string)           {}
func (nullNode) LeadingComments() []string   { return nil }
func (nullNode) SetLeadingComments([]string) {}
func (nullNode) setLeadingComments([]string) {}
func (nullNode) Raw() []byte                 { return nil }
func (nullNode) setRaw([]byte)               {}
func (nullNode) Span() Span                  { return Span{} }
func (nullNode) setSpan(Span)                {}
func (nullNode) isDirty() bool               { return false }
func (nullNode) markDirty()                  {}
func (nullNode) trivia() *Trivia             { return &Trivia{} }

// LocalDateTime represents a TOML local date-time (no timezone).
type LocalDateTime struct {
	Year, Month, Day     int
	Hour, Minute, Second int
	Nanosecond           int
}

// LocalDate represents a TOML local date.
type LocalDate struct {
	Year, Month, Day int
}

// LocalTime represents a TOML local time.
type LocalTime struct {
	Hour, Minute, Second int
	Nanosecond           int
}
