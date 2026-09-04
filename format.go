package tomledit

import (
	"bytes"
	"strings"
)

// FormatConfig controls how the formatter normalizes TOML output.
// Use DefaultFormatConfig to get sensible defaults and WithIndentWidth,
// WithLineWidth, or WithTableBlankLine to override specific settings.
type FormatConfig struct {
	IndentWidth    int  // spaces per indent level (default: 0, no indentation of values)
	LineWidth      int  // max line width before arrays go multi-line (default: 80)
	TableBlankLine bool // insert a blank line before a table that has none (default: true)
}

// DefaultFormatConfig returns a FormatConfig with sensible defaults.
func DefaultFormatConfig() FormatConfig {
	return FormatConfig{
		IndentWidth:    0,
		LineWidth:      80,
		TableBlankLine: true,
	}
}

// FormatOption is a functional option for configuring the formatter.
type FormatOption func(*FormatConfig)

// WithIndentWidth sets the number of spaces per indent level for values
// under table headers.
func WithIndentWidth(n int) FormatOption {
	return func(cfg *FormatConfig) {
		cfg.IndentWidth = n
	}
}

// WithLineWidth sets the maximum line width before arrays are rendered
// in multi-line format.
func WithLineWidth(n int) FormatOption {
	return func(cfg *FormatConfig) {
		cfg.LineWidth = n
	}
}

// WithTableBlankLine controls whether a blank line is INSERTED before a table
// header that has none. It never removes one: blank lines the document already
// carries are preserved either way (each run collapsing to a single blank
// line), so turning it off only stops the formatter from adding separation
// where the writer left none.
func WithTableBlankLine(b bool) FormatOption {
	return func(cfg *FormatConfig) {
		cfg.TableBlankLine = b
	}
}

// Format returns normalized TOML bytes. It does NOT mutate the document --
// it produces a new byte slice by walking the AST and re-rendering every node
// with consistent formatting, ignoring all raw bytes. This is useful for
// enforcing a canonical style. Pass zero or more FormatOption values (e.g.
// WithIndentWidth, WithLineWidth) to customize the output.
//
// The writer's blank-line grouping survives at document and table-body
// level: a run of blank lines becomes exactly one, and where the writer left
// no gap the formatter opens none. The output never begins with a blank line
// and always ends with exactly one newline, so blank lines at either end of
// the document are dropped. Arrays are restructured wholesale (inline or
// multi-line from the configured line width), so blank lines between array
// elements do not survive formatting; Bytes preserves them.
func (d *Document) Format(opts ...FormatOption) []byte {
	cfg := DefaultFormatConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	var buf bytes.Buffer
	formatDocument(&buf, d, &cfg)
	return buf.Bytes()
}

// formatDocument walks all document children and formats them.
func formatDocument(buf *bytes.Buffer, doc *Document, cfg *FormatConfig) {
	for _, child := range doc.children {
		formatChild(buf, child, cfg, 0)
	}

	// Ensure trailing newline: the output must end with exactly one \n.
	ensureTrailingNewline(buf)
}

// formatChild formats one child of a document, table or array-table body.
func formatChild(buf *bytes.Buffer, child Node, cfg *FormatConfig, indent int) {
	switch node := child.(type) {
	case *TableNode:
		formatTableNode(buf, node, cfg, indent)
	case *ArrayTableNode:
		formatArrayTableNode(buf, node, cfg, indent)
	case *KeyValueNode:
		formatLeadingTrivia(buf, node)
		formatKeyValueLine(buf, node, cfg, indent)
	case *CommentNode:
		formatStandaloneComment(buf, node)
	}
}

// formatTableNode formats a [table] header and its children.
func formatTableNode(buf *bytes.Buffer, node *TableNode, cfg *FormatConfig, depth int) {
	formatTableSeparation(buf, node, cfg)

	// Table header: [key.path]
	buf.WriteByte('[')
	buf.Write(renderKeyPath(node.keyPath))
	buf.WriteByte(']')

	// Inline comment on table header.
	formatInlineComment(buf, node)

	buf.WriteByte('\n')

	// Children.
	indent := depth + 1
	for _, child := range node.children {
		formatChild(buf, child, cfg, indent)
	}
}

// formatArrayTableNode formats an [[array-table]] header and its children.
func formatArrayTableNode(buf *bytes.Buffer, node *ArrayTableNode, cfg *FormatConfig, depth int) {
	formatTableSeparation(buf, node, cfg)

	// Array table header: [[key.path]]
	buf.WriteString("[[")
	buf.Write(renderKeyPath(node.keyPath))
	buf.WriteString("]]")

	// Inline comment on table header.
	formatInlineComment(buf, node)

	buf.WriteByte('\n')

	// Children.
	indent := depth + 1
	for _, child := range node.children {
		formatChild(buf, child, cfg, indent)
	}
}

// formatTableSeparation writes what stands between the previous construct and
// a table header: the blank line the option asks for where none is written
// already, then the node's own leading trivia -- its blank-line runs and its
// comments. The option is insertion-only, and writeBlankLine collapses, so a
// document that already separates its tables is not double-spaced by it.
func formatTableSeparation(buf *bytes.Buffer, node Node, cfg *FormatConfig) {
	if cfg.TableBlankLine {
		writeBlankLine(buf)
	}
	formatLeadingTrivia(buf, node)
}

// formatKeyValueLine formats a key = value line with optional indentation.
func formatKeyValueLine(buf *bytes.Buffer, kv *KeyValueNode, cfg *FormatConfig, indentLevel int) {
	// Indentation.
	writeIndent(buf, cfg, indentLevel)

	// Key.
	buf.Write(renderKeyFromParts(kv.key))
	buf.WriteString(" = ")

	// Compute the prefix length so far on this line (for multi-line array decisions).
	prefixLen := indentLevel*cfg.IndentWidth + len(renderKeyFromParts(kv.key)) + 3 // " = "

	// Value.
	formatValue(buf, kv.val, cfg, prefixLen, indentLevel)

	// Inline comment.
	formatInlineComment(buf, kv)

	buf.WriteByte('\n')
}

// formatValue formats any value node, re-rendering from semantic data.
func formatValue(buf *bytes.Buffer, n Node, cfg *FormatConfig, prefixLen int, indentLevel int) {
	switch node := n.(type) {
	case *StringNode:
		buf.Write(renderString(node))
	case *IntegerNode:
		buf.Write(renderInteger(node))
	case *FloatNode:
		buf.Write(renderFloat(node))
	case *BooleanNode:
		buf.Write(renderBoolean(node))
	case *DateTimeNode:
		buf.Write(renderDateTime(node))
	case *LocalDateTimeNode:
		buf.Write(renderLocalDateTime(node))
	case *LocalDateNode:
		buf.Write(renderLocalDate(node))
	case *LocalTimeNode:
		buf.Write(renderLocalTime(node))
	case *ArrayNode:
		formatArray(buf, node, cfg, prefixLen, indentLevel)
	case *InlineTableNode:
		formatInlineTable(buf, node, cfg)
	}
}

// arrayHasComments returns true if any element has leading or inline comments,
// or if the array has trailing comments. Arrays with comments must be rendered
// in multi-line format since TOML inline arrays cannot contain comments.
func arrayHasComments(arr *ArrayNode) bool {
	if len(arr.trailingComments) > 0 {
		return true
	}
	for _, elem := range arr.elements {
		t := elem.trivia()
		if len(t.LeadingComments) > 0 || len(t.InlineComment) > 0 {
			return true
		}
	}
	return false
}

// formatArray formats an array, choosing inline or multi-line based on LineWidth.
func formatArray(buf *bytes.Buffer, arr *ArrayNode, cfg *FormatConfig, prefixLen int, indentLevel int) {
	if len(arr.elements) == 0 {
		buf.WriteString("[]")
		return
	}

	// Arrays with comments must be multi-line (TOML inline arrays cannot
	// contain comments).
	if arrayHasComments(arr) {
		formatArrayMultiLine(buf, arr, cfg, indentLevel)
		return
	}

	// Try inline first: render all elements to measure total width.
	inline := formatArrayInline(arr, cfg)
	totalLen := prefixLen + len(inline)
	if totalLen <= cfg.LineWidth {
		buf.Write(inline)
		return
	}

	// Multi-line format.
	formatArrayMultiLine(buf, arr, cfg, indentLevel)
}

// formatArrayInline renders an array in inline form: [elem1, elem2, elem3]
func formatArrayInline(arr *ArrayNode, cfg *FormatConfig) []byte {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, elem := range arr.elements {
		if i > 0 {
			b.WriteString(", ")
		}
		formatValue(&b, elem, cfg, 0, 0)
	}
	b.WriteByte(']')
	return b.Bytes()
}

// formatArrayMultiLine renders an array with one element per line, including
// a trailing comma on the last element. It preserves leading comments,
// inline comments on elements, and trailing comments on the array.
func formatArrayMultiLine(buf *bytes.Buffer, arr *ArrayNode, cfg *FormatConfig, indentLevel int) {
	buf.WriteString("[\n")
	elemIndent := indentLevel + 1
	for _, elem := range arr.elements {
		// Leading comments for this element.
		t := elem.trivia()
		for _, c := range t.LeadingComments {
			writeIndent(buf, cfg, elemIndent)
			line := strings.TrimRight(string(c), " \t\r\n")
			if line != "" {
				buf.WriteString(line)
			}
			buf.WriteByte('\n')
		}

		writeIndent(buf, cfg, elemIndent)
		formatValue(buf, elem, cfg, elemIndent*cfg.IndentWidth, elemIndent)
		buf.WriteByte(',')

		// Inline comment on this element.
		if len(t.InlineComment) > 0 {
			comment := strings.TrimRight(string(t.InlineComment), " \t\r\n")
			if comment != "" {
				if comment[0] == '#' {
					buf.WriteString("  ")
					buf.WriteString(comment)
				} else {
					buf.WriteString("  # ")
					buf.WriteString(comment)
				}
			}
		}

		buf.WriteByte('\n')
	}

	// Trailing comments (after last element, before ']').
	for _, c := range arr.trailingComments {
		writeIndent(buf, cfg, elemIndent)
		line := strings.TrimRight(string(c), " \t\r\n")
		if line != "" {
			buf.WriteString(line)
		}
		buf.WriteByte('\n')
	}

	writeIndent(buf, cfg, indentLevel)
	buf.WriteByte(']')
}

// formatInlineTable formats an inline table on one line with consistent spacing.
func formatInlineTable(buf *bytes.Buffer, it *InlineTableNode, cfg *FormatConfig) {
	if len(it.children) == 0 {
		buf.WriteString("{}")
		return
	}
	buf.WriteByte('{')
	for i, child := range it.children {
		if i > 0 {
			buf.WriteString(", ")
		}
		kv, ok := child.(*KeyValueNode)
		if !ok {
			continue
		}
		buf.Write(renderKeyFromParts(kv.key))
		buf.WriteString(" = ")
		formatValue(buf, kv.val, cfg, 0, 0)
	}
	buf.WriteByte('}')
}

// formatLeadingTrivia emits what a node carries above itself: its blank-line
// runs and its leading comments, in the order they were written. Each comment
// is rendered as a clean "# text\n" line and each run of blank lines as a
// single blank line.
func formatLeadingTrivia(buf *bytes.Buffer, n Node) {
	t := n.trivia()
	for i, c := range t.LeadingComments {
		if len(t.blankRun(i)) > 0 {
			writeBlankLine(buf)
		}
		line := strings.TrimRight(string(c), " \t\r\n")
		if line == "" {
			// Blank comment line -- skip or preserve as empty line.
			// We keep it as a blank line only if it's truly empty.
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	// The run between the last comment (or the construct above, when there are
	// no comments) and the node itself.
	if len(t.blankRun(len(t.LeadingComments))) > 0 {
		writeBlankLine(buf)
	}
}

// writeBlankLine writes the single blank line a run of them collapses to. It
// writes nothing when nothing stands above it -- the output never opens with a
// blank line -- and nothing where a blank line already stands, which is what
// makes runs collapse and the table-blank-line option insertion-only.
func writeBlankLine(buf *bytes.Buffer) {
	b := buf.Bytes()
	if len(b) == 0 || b[len(b)-1] != '\n' {
		return
	}
	if len(b) >= 2 && b[len(b)-2] == '\n' {
		return
	}
	buf.WriteByte('\n')
}

// formatInlineComment appends an inline comment if one exists.
// Uses the normalized format: two spaces, then "# text".
func formatInlineComment(buf *bytes.Buffer, n Node) {
	t := n.trivia()
	if len(t.InlineComment) == 0 {
		return
	}
	comment := strings.TrimRight(string(t.InlineComment), " \t\r\n")
	if comment == "" {
		return
	}
	// Normalize to "  # text" format.
	// The stored comment typically starts with "#".
	if comment[0] == '#' {
		buf.WriteString("  ")
		buf.WriteString(comment)
	} else {
		buf.WriteString("  # ")
		buf.WriteString(comment)
	}
}

// formatStandaloneComment formats a standalone CommentNode. A comment node
// with no text stands for a run of blank lines that no following construct
// could carry, and contributes the one blank line that run collapses to.
func formatStandaloneComment(buf *bytes.Buffer, cn *CommentNode) {
	formatLeadingTrivia(buf, cn)
	text := strings.TrimRight(cn.text, " \t\r\n")
	if text == "" {
		if bytes.ContainsRune(cn.rawBytes(), '\n') {
			writeBlankLine(buf)
		}
		return
	}
	// If it already starts with #, emit as-is.
	if text[0] == '#' {
		buf.WriteString(text)
		buf.WriteByte('\n')
	} else {
		buf.WriteString("# ")
		buf.WriteString(text)
		buf.WriteByte('\n')
	}
}

// writeIndent writes indentation spaces based on the indent level and config.
func writeIndent(buf *bytes.Buffer, cfg *FormatConfig, level int) {
	n := level * cfg.IndentWidth
	for i := 0; i < n; i++ {
		buf.WriteByte(' ')
	}
}

// ensureTrailingNewline makes sure the buffer ends with exactly one \n.
func ensureTrailingNewline(buf *bytes.Buffer) {
	b := buf.Bytes()
	if len(b) == 0 {
		buf.WriteByte('\n')
		return
	}
	// Strip trailing newlines, then add exactly one.
	for len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	buf.Reset()
	buf.Write(b)
	buf.WriteByte('\n')
}
