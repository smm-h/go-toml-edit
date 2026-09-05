package tomledit

import (
	"strings"
	"time"
)

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

// trivia holds formatting and comment data attached to a node.
type trivia struct {
	LeadingWhitespace []byte
	LeadingComments   [][]byte // each is a full "# ...\n" line
	InlineComment     []byte   // "# ..." after value on same line
	TrailingNewline   []byte

	// inlineGap holds the bytes standing between the construct and its inline
	// comment -- the whitespace a writer put there, which is none at all in
	// "x = 1# note". It is a fragment of its own: a node that re-renders
	// because something else about it changed writes the gap back exactly. A
	// comment written where there was none gets one space, and removing a
	// comment clears the gap, so the line keeps no whitespace pointing at
	// nothing.
	inlineGap []byte

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
func (t *trivia) blankRun(i int) []byte {
	if i < 0 || i >= len(t.blankRuns) {
		return nil
	}
	return t.blankRuns[i]
}

// Node is the interface implemented by all AST nodes.
// Every node carries its original raw bytes, its trivia (whitespace and
// comments) and its source range. It carries no value: only the value-carrying
// kinds do, and those implement Scalar. Implementation is restricted to this
// package.
type Node interface {
	Type() NodeType

	// Comment returns the node's inline comment as text: without the "#" and
	// the whitespace around it, and empty when there is none.
	Comment() string

	// LeadingComments returns the comment lines written above the node as
	// text, each without its "#", its trailing newline and the whitespace
	// around them.
	LeadingComments() []string

	// Raw returns the node's bytes as written, comments and spacing included,
	// as a copy: writing into what it returns changes neither what a later
	// read answers nor what the document renders.
	Raw() []byte

	// Span returns the source range the node covered in the most recent
	// Parse. Nodes created programmatically by an edit return the
	// zero Span (IsValid reports false). Edits do not update spans; see
	// the Span type documentation for the exact policy.
	Span() Span

	// unexported methods restrict implementation to this package
	setComment(comment string)
	setLeadingComments(comments []string)
	rawBytes() []byte
	setRaw([]byte)
	setSpan(Span)
	isDirty() bool
	subtreeDirty() bool
	setSubtreeDirty()
	markDirty()
	parentNode() Node
	setParent(Node)
	trivia() *trivia
}

// Scalar is the sub-interface of Node the value-carrying node kinds implement:
// strings, integers, floats, booleans and the four date-time flavors. Every
// other kind holds structure rather than a value -- a document, a table, an
// array-of-tables, an array, an inline table, a key, a key-value pair, a
// comment -- and is read through its own accessors instead.
type Scalar interface {
	Node

	// Value returns the node's payload as the Go type the conversion table
	// gives that TOML type: string, int64, float64, bool, time.Time, or one of
	// LocalDateTime, LocalDate and LocalTime.
	Value() any

	// The typed accessor family. Each one reads this node through the
	// conversion table -- the same table and the same conversion the decode
	// engine runs -- so a value a decode target accepts is a value the matching
	// accessor accepts. A value the table refuses for the target is a
	// KindTypeMismatch diagnostic naming both sides; a value it accepts by kind
	// but cannot carry exactly is KindInexact.

	// AsString reads the node as a string. Only a string is one.
	AsString() (string, error)

	// AsInt reads the node as an int64. Only an integer is one: a float is
	// never an integer, however whole it is written.
	AsInt() (int64, error)

	// AsFloat reads the node as a float64: a float verbatim, and an integer
	// the target holds exactly.
	AsFloat() (float64, error)

	// AsBool reads the node as a bool. Only a boolean is one.
	AsBool() (bool, error)

	// AsTime reads the node as a time.Time: an offset date-time verbatim, and
	// a local date-time or local date read as UTC, because a time.Time target
	// declares that intent. A local time carries no date and is refused.
	AsTime() (time.Time, error)

	// AsLocalDateTime reads the node as a LocalDateTime. Only a local
	// date-time is one.
	AsLocalDateTime() (LocalDateTime, error)

	// AsLocalDate reads the node as a LocalDate. Only a local date is one.
	AsLocalDate() (LocalDate, error)

	// AsLocalTime reads the node as a LocalTime. Only a local time is one.
	AsLocalTime() (LocalTime, error)
}

// Every value-carrying node kind, and no other, implements Scalar.
var (
	_ Scalar = (*StringNode)(nil)
	_ Scalar = (*IntegerNode)(nil)
	_ Scalar = (*FloatNode)(nil)
	_ Scalar = (*BooleanNode)(nil)
	_ Scalar = (*DateTimeNode)(nil)
	_ Scalar = (*LocalDateTimeNode)(nil)
	_ Scalar = (*LocalDateNode)(nil)
	_ Scalar = (*LocalTimeNode)(nil)
)

// nodeBase provides the shared implementation for all concrete node types.
type nodeBase struct {
	raw   []byte
	dirty bool

	// subtree records that this node, or something under it, no longer splices
	// its original bytes. The structure funnel keeps it true by saying so
	// upward at the moment a node goes dirty, which is what makes the
	// serializer's question a single field read rather than a walk.
	subtree bool

	// parent is the container this node was put into, nil for a document and
	// for a node not yet attached to one. Only the funnels of mutate.go write
	// it.
	parent Node

	nodeTrivia trivia
	span       Span
}

// Raw answers with a copy. The serializer splices through rawBytes instead, so
// the copy is paid for only by a caller that asked for the bytes.
func (n *nodeBase) Raw() []byte {
	return copyBytes(n.raw)
}

// rawBytes returns the node's own bytes, for the splicing inside the package.
func (n *nodeBase) rawBytes() []byte {
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
	probeDirtyRead()
	return n.dirty
}

// subtreeDirty reports whether this node or anything under it stopped splicing
// its original bytes. It is a single field read whatever the document's depth,
// which dirtyProbes exists to pin.
func (n *nodeBase) subtreeDirty() bool {
	probeDirtyRead()
	// A node built dirty by a composite literal never went through markDirty,
	// so its own bit is the second half of the answer.
	return n.subtree || n.dirty
}

// dirtyProbes counts how many times the serializer asks a node about its
// dirtiness, for the test that pins that count as independent of how deeply the
// document nests. Counting is off unless a test turns it on.
var (
	dirtyProbes         int
	countingDirtyProbes bool
)

func probeDirtyRead() {
	if countingDirtyProbes {
		dirtyProbes++
	}
}

func (n *nodeBase) trivia() *trivia {
	return &n.nodeTrivia
}

// Comment returns the text of the node's inline comment, without the "#" and
// the whitespace around it: a node written `x = 1 # note` answers "note". A
// node with no inline comment answers the empty string. Raw carries the bytes
// as written, for a caller that needs them exactly.
func (n *nodeBase) Comment() string {
	return normalizeCommentText(n.nodeTrivia.InlineComment)
}

func (n *nodeBase) setComment(comment string) {
	switch {
	case comment == "":
		// The gap led to a comment that is no longer there; keeping it would
		// leave trailing whitespace behind the value.
		n.nodeTrivia.inlineGap = nil
	case len(n.nodeTrivia.InlineComment) == 0:
		// A comment written where there was none: the spacing is the writer's
		// to choose, and it chooses one space. A comment REPLACING one keeps
		// the spacing the line was written with, whatever it was -- including
		// none at all.
		n.nodeTrivia.inlineGap = []byte(" ")
	}
	n.nodeTrivia.InlineComment = []byte(comment)
	n.markDirty()
	dropGapsOf(n.parent)
}

// LeadingComments returns the text of each comment line written above the node,
// in order, each without its "#", its trailing newline and the whitespace
// around them. Raw carries the bytes as written, for a caller that needs them
// exactly.
func (n *nodeBase) LeadingComments() []string {
	result := make([]string, len(n.nodeTrivia.LeadingComments))
	for i, c := range n.nodeTrivia.LeadingComments {
		result[i] = normalizeCommentText(c)
	}
	return result
}

// normalizeCommentText returns a comment's content: the bytes with the leading
// "#" and the whitespace on either side of it removed. It is what the comment
// getters answer and what the path-based setters take, so content read from one
// document and written to another is written as it was read -- normalized, not
// verbatim: the marker and the spacing around it are the setter's to choose,
// and a comment written "#note" reads and rewrites as "# note".
//
// Two inputs do not survive that round trip, both because the content and its
// marker are not separable once the marker is gone:
//
//   - Content that itself begins with "#". "## section" reads as "# section",
//     and the setter puts its own marker in front of that, giving
//     "# # section".
//   - The empty comment. "#" reads as "", which is what the setters take to
//     mean removal, so writing it back removes the comment instead of
//     restoring it.
//
// A caller that needs the bytes exactly reads Raw instead.
func normalizeCommentText(b []byte) string {
	text := strings.TrimSpace(string(b))
	text = strings.TrimPrefix(text, "#")
	return strings.TrimSpace(text)
}

func (n *nodeBase) setLeadingComments(comments []string) {
	t := &n.nodeTrivia
	// The blank lines that separated this node from the one above it, and those
	// that stood between the last comment and the node, survive replacing the
	// comments in between. With no comments left the two runs meet, so they are
	// joined into the one run that remains -- and, symmetrically, a node that
	// had no comments to begin with carries ONE run, which is the one above:
	// reading it as both would put a blank line on either side of the comments
	// being written.
	lead := t.blankRun(0)
	var tail []byte
	if len(t.LeadingComments) > 0 {
		tail = t.blankRun(len(t.LeadingComments))
	}
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
	n.markDirty()
	dropGapsOf(n.parent)
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
