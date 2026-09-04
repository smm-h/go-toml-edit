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

	// blankRuns holds the blank lines around the leading comments: entry i the
	// run written before LeadingComments[i], and the entry after the last
	// comment the run between that comment and the node itself. A blank line is
	// what separates one construct from the next, so a node that re-renders
	// emits these runs again rather than pulling itself up against whatever
	// stands above it. Runs are held as bytes, not as a count, so a blank line
	// carrying whitespace of its own survives verbatim. A missing entry is an
	// empty run.
	blankRuns [][]byte
}

// blankRun returns the bytes of blank-line run i, or nil when there is none.
func (t *Trivia) blankRun(i int) []byte {
	if i < 0 || i >= len(t.blankRuns) {
		return nil
	}
	return t.blankRuns[i]
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
	t := &n.nodeTrivia
	// The blank lines that separated this node from the one above it, and those
	// that stood between the last comment and the node, survive replacing the
	// comments in between. With no comments left the two runs meet, so they are
	// joined into the one run that remains.
	lead := t.blankRun(0)
	tail := t.blankRun(len(t.LeadingComments))
	t.LeadingComments = make([][]byte, len(comments))
	for i, c := range comments {
		t.LeadingComments[i] = []byte(c)
	}
	if len(comments) == 0 {
		joined := make([]byte, 0, len(lead)+len(tail))
		joined = append(joined, lead...)
		joined = append(joined, tail...)
		t.blankRuns = [][]byte{joined}
	} else {
		t.blankRuns = make([][]byte, len(comments)+1)
		t.blankRuns[0] = lead
		t.blankRuns[len(comments)] = tail
	}
	n.dirty = true
}

func (n *nodeBase) setLeadingComments(comments []string) {
	n.SetLeadingComments(comments)
}

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
