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
	TableBlankLine bool // insert blank line before tables (default: true)
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

// WithTableBlankLine controls whether a blank line is inserted before
// table headers.
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
	// Track whether we've emitted any content, to know whether to insert
	// blank lines before table headers.
	emittedContent := false

	for _, child := range doc.Children {
		switch node := child.(type) {
		case *TableNode:
			formatTableNode(buf, node, cfg, &emittedContent, 0)
		case *ArrayTableNode:
			formatArrayTableNode(buf, node, cfg, &emittedContent, 0)
		case *KeyValueNode:
			formatLeadingComments(buf, node, cfg)
			formatKeyValueLine(buf, node, cfg, 0)
			emittedContent = true
		case *CommentNode:
			formatStandaloneComment(buf, node, cfg)
			emittedContent = true
		}
	}

	// Ensure trailing newline: the output must end with exactly one \n.
	ensureTrailingNewline(buf)
}

// formatTableNode formats a [table] header and its children.
func formatTableNode(buf *bytes.Buffer, node *TableNode, cfg *FormatConfig, emittedContent *bool, depth int) {
	// Blank line before table header (unless it's the first content).
	if *emittedContent && cfg.TableBlankLine {
		buf.WriteByte('\n')
	}

	// Leading comments on the table header.
	formatLeadingComments(buf, node, cfg)

	// Table header: [key.path]
	buf.WriteByte('[')
	buf.Write(renderKeyPath(node.KeyPath))
	buf.WriteByte(']')

	// Inline comment on table header.
	formatInlineComment(buf, node)

	buf.WriteByte('\n')
	*emittedContent = true

	// Children.
	indent := depth + 1
	for _, child := range node.Children {
		switch c := child.(type) {
		case *KeyValueNode:
			formatLeadingComments(buf, c, cfg)
			formatKeyValueLine(buf, c, cfg, indent)
		case *CommentNode:
			formatStandaloneComment(buf, c, cfg)
		}
	}
}

// formatArrayTableNode formats an [[array-table]] header and its children.
func formatArrayTableNode(buf *bytes.Buffer, node *ArrayTableNode, cfg *FormatConfig, emittedContent *bool, depth int) {
	// Blank line before table header (unless it's the first content).
	if *emittedContent && cfg.TableBlankLine {
		buf.WriteByte('\n')
	}

	// Leading comments on the table header.
	formatLeadingComments(buf, node, cfg)

	// Array table header: [[key.path]]
	buf.WriteString("[[")
	buf.Write(renderKeyPath(node.KeyPath))
	buf.WriteString("]]")

	// Inline comment on table header.
	formatInlineComment(buf, node)

	buf.WriteByte('\n')
	*emittedContent = true

	// Children.
	indent := depth + 1
	for _, child := range node.Children {
		switch c := child.(type) {
		case *KeyValueNode:
			formatLeadingComments(buf, c, cfg)
			formatKeyValueLine(buf, c, cfg, indent)
		case *CommentNode:
			formatStandaloneComment(buf, c, cfg)
		}
	}
}

// formatKeyValueLine formats a key = value line with optional indentation.
func formatKeyValueLine(buf *bytes.Buffer, kv *KeyValueNode, cfg *FormatConfig, indentLevel int) {
	// Indentation.
	writeIndent(buf, cfg, indentLevel)

	// Key.
	buf.Write(renderKeyFromParts(kv.Key))
	buf.WriteString(" = ")

	// Compute the prefix length so far on this line (for multi-line array decisions).
	prefixLen := indentLevel*cfg.IndentWidth + len(renderKeyFromParts(kv.Key)) + 3 // " = "

	// Value.
	formatValue(buf, kv.Val, cfg, prefixLen, indentLevel)

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
	if len(arr.TrailingComments) > 0 {
		return true
	}
	for _, elem := range arr.Elements {
		t := elem.trivia()
		if len(t.LeadingComments) > 0 || len(t.InlineComment) > 0 {
			return true
		}
	}
	return false
}

// formatArray formats an array, choosing inline or multi-line based on LineWidth.
func formatArray(buf *bytes.Buffer, arr *ArrayNode, cfg *FormatConfig, prefixLen int, indentLevel int) {
	if len(arr.Elements) == 0 {
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
	for i, elem := range arr.Elements {
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
	for _, elem := range arr.Elements {
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
	for _, c := range arr.TrailingComments {
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
	if len(it.Children) == 0 {
		buf.WriteString("{}")
		return
	}
	buf.WriteByte('{')
	for i, child := range it.Children {
		if i > 0 {
			buf.WriteString(", ")
		}
		kv, ok := child.(*KeyValueNode)
		if !ok {
			continue
		}
		buf.Write(renderKeyFromParts(kv.Key))
		buf.WriteString(" = ")
		formatValue(buf, kv.Val, cfg, 0, 0)
	}
	buf.WriteByte('}')
}

// formatLeadingComments emits leading comments attached to a node.
// Each comment is rendered as a clean "# text\n" line.
func formatLeadingComments(buf *bytes.Buffer, n Node, cfg *FormatConfig) {
	t := n.trivia()
	for _, c := range t.LeadingComments {
		line := strings.TrimRight(string(c), " \t\r\n")
		if line == "" {
			// Blank comment line -- skip or preserve as empty line.
			// We keep it as a blank line only if it's truly empty.
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
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

// formatStandaloneComment formats a standalone CommentNode.
func formatStandaloneComment(buf *bytes.Buffer, cn *CommentNode, cfg *FormatConfig) {
	text := strings.TrimRight(cn.Text, " \t\r\n")
	if text == "" {
		// Empty comment node -- might represent blank lines. Skip it.
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
