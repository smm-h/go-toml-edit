package tomledit

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Bytes serializes the document back to TOML bytes.
//
// For clean documents (parsed and never modified), it returns the exact
// original source bytes (round-trip fidelity). For nodes that have been
// modified via Set, Delete, or other editing operations, only the affected
// nodes are re-rendered from their semantic values; unmodified nodes retain
// their original formatting.
func (d *Document) Bytes() []byte {
	var buf []byte
	for _, child := range d.children {
		buf = append(buf, serializeNode(child)...)
	}
	return buf
}

// serializeNode dispatches serialization for a single node. Clean nodes emit
// their raw bytes; dirty nodes are re-rendered from semantic values.
func serializeNode(n Node) []byte {
	switch node := n.(type) {
	case *TableNode:
		var buf []byte
		if !node.isDirty() {
			buf = append(buf, node.rawBytes()...)
		} else {
			buf = append(buf, renderTableHeader(node)...)
		}
		for _, child := range node.children {
			buf = append(buf, serializeNode(child)...)
		}
		return buf

	case *ArrayTableNode:
		var buf []byte
		if !node.isDirty() {
			buf = append(buf, node.rawBytes()...)
		} else {
			buf = append(buf, renderArrayTableHeader(node)...)
		}
		for _, child := range node.children {
			buf = append(buf, serializeNode(child)...)
		}
		return buf

	case *KeyValueNode:
		if !node.subtreeDirty() {
			return node.rawBytes()
		}
		return renderKeyValue(node)

	case *CommentNode:
		if !node.isDirty() {
			return node.rawBytes()
		}
		return renderComment(node)

	default:
		// Leaf value nodes (string, int, float, bool, datetime, array, inline
		// table). A container asks the same question as a scalar: the answer
		// is one field read, because a node that went dirty said so upward at
		// the moment it happened.
		if !n.subtreeDirty() {
			return n.rawBytes()
		}
		return renderValue(n)
	}
}

// renderValue renders a dirty value node from its semantic value.
func renderValue(n Node) []byte {
	switch node := n.(type) {
	case *StringNode:
		return renderString(node)
	case *IntegerNode:
		return renderInteger(node)
	case *FloatNode:
		return renderFloat(node)
	case *BooleanNode:
		return renderBoolean(node)
	case *DateTimeNode:
		return renderDateTime(node)
	case *LocalDateTimeNode:
		return renderLocalDateTime(node)
	case *LocalDateNode:
		return renderLocalDate(node)
	case *LocalTimeNode:
		return renderLocalTime(node)
	case *ArrayNode:
		return renderArray(node)
	case *InlineTableNode:
		return renderInlineTable(node)
	default:
		return nil
	}
}

// renderString renders a StringNode based on its Style.
func renderString(n *StringNode) []byte {
	switch n.style {
	case StringLiteral:
		// Literal strings cannot contain single quotes or newlines.
		// Fall back to basic if the value contains them.
		if !strings.Contains(n.val.get(), "'") && !strings.Contains(n.val.get(), "\n") {
			return []byte("'" + n.val.get() + "'")
		}
		return renderBasicString(n.val.get())
	case StringMultiLineBasic:
		return renderMultiLineBasicString(n.val.get())
	case StringMultiLineLiteral:
		// Multi-line literal strings cannot contain '''.
		// Fall back to multi-line basic if needed.
		if !strings.Contains(n.val.get(), "'''") {
			return []byte("'''\n" + n.val.get() + "'''")
		}
		return renderMultiLineBasicString(n.val.get())
	default: // StringBasic
		return renderBasicString(n.val.get())
	}
}

// renderBasicString renders a value as a TOML basic string with proper escaping.
func renderBasicString(s string) []byte {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7F {
				// Control character: use \u escape
				b.WriteString(fmt.Sprintf(`\u%04X`, r))
			} else {
				b.WriteRune(r)
			}
		}
		i += size
	}
	b.WriteByte('"')
	return []byte(b.String())
}

// renderMultiLineBasicString renders a value as a TOML multi-line basic string.
func renderMultiLineBasicString(s string) []byte {
	var b strings.Builder
	b.WriteString("\"\"\"\n")
	consecutiveQuotes := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch r {
		case '"':
			consecutiveQuotes++
			if consecutiveQuotes >= 3 && consecutiveQuotes%3 == 0 {
				// Every 3rd consecutive quote must be escaped to avoid
				// forming a closing """ delimiter inside the content.
				b.WriteString(`\"`)
			} else {
				b.WriteByte('"')
			}
		case '\\':
			consecutiveQuotes = 0
			b.WriteString(`\\`)
		case '\b':
			consecutiveQuotes = 0
			b.WriteString(`\b`)
		case '\t':
			consecutiveQuotes = 0
			b.WriteRune('\t') // tabs are allowed in multi-line
		case '\n':
			consecutiveQuotes = 0
			b.WriteByte('\n') // newlines are allowed in multi-line
		case '\f':
			consecutiveQuotes = 0
			b.WriteString(`\f`)
		case '\r':
			consecutiveQuotes = 0
			b.WriteString(`\r`)
		default:
			consecutiveQuotes = 0
			if r < 0x20 || r == 0x7F {
				b.WriteString(fmt.Sprintf(`\u%04X`, r))
			} else {
				b.WriteRune(r)
			}
		}
		i += size
	}
	// Before writing the closing """, ensure we don't form 3+ consecutive
	// quotes with any trailing quotes in the content.
	// If the content ends with N unescaped quotes, the closing """ would
	// create N+3 consecutive quotes. We need to escape the last content
	// quote(s) that would cause a problem.
	// However, since we already escaped every 3rd quote above, the content
	// can end with at most 2 consecutive unescaped quotes.
	// With 2 trailing quotes + closing """, that's 5 quotes total = """""
	// which TOML allows (parsed as 2 content quotes + closing delimiter).
	// With 1 trailing quote + closing """, that's 4 = """" which is also fine.
	// So no additional handling needed at the boundary.
	b.WriteString(`"""`)
	return []byte(b.String())
}

// renderInteger renders an IntegerNode based on its Base.
func renderInteger(n *IntegerNode) []byte {
	switch n.base {
	case IntegerHex:
		if n.val.get() < 0 {
			return []byte("-0x" + strconv.FormatInt(-n.val.get(), 16))
		}
		return []byte("0x" + strconv.FormatInt(n.val.get(), 16))
	case IntegerOctal:
		if n.val.get() < 0 {
			return []byte("-0o" + strconv.FormatInt(-n.val.get(), 8))
		}
		return []byte("0o" + strconv.FormatInt(n.val.get(), 8))
	case IntegerBinary:
		if n.val.get() < 0 {
			return []byte("-0b" + strconv.FormatInt(-n.val.get(), 2))
		}
		return []byte("0b" + strconv.FormatInt(n.val.get(), 2))
	default: // IntegerDecimal
		return []byte(strconv.FormatInt(n.val.get(), 10))
	}
}

// renderFloat renders a FloatNode.
func renderFloat(n *FloatNode) []byte {
	if math.IsInf(n.val.get(), 1) {
		return []byte("inf")
	}
	if math.IsInf(n.val.get(), -1) {
		return []byte("-inf")
	}
	if math.IsNaN(n.val.get()) {
		return []byte("nan")
	}
	s := strconv.FormatFloat(n.val.get(), 'f', -1, 64)
	// Ensure there's a decimal point so it's recognized as a float
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return []byte(s)
}

// renderBoolean renders a BooleanNode.
func renderBoolean(n *BooleanNode) []byte {
	if n.val.get() {
		return []byte("true")
	}
	return []byte("false")
}

// renderDateTime renders a DateTimeNode.
func renderDateTime(n *DateTimeNode) []byte {
	return []byte(n.val.get().Format("2006-01-02T15:04:05.999999999Z07:00"))
}

// renderLocalDateTime renders a LocalDateTimeNode.
func renderLocalDateTime(n *LocalDateTimeNode) []byte {
	v := n.val.get()
	s := fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d", v.Year, v.Month, v.Day, v.Hour, v.Minute, v.Second)
	if v.Nanosecond > 0 {
		frac := formatFractional(v.Nanosecond)
		s += "." + frac
	}
	return []byte(s)
}

// renderLocalDate renders a LocalDateNode.
func renderLocalDate(n *LocalDateNode) []byte {
	return []byte(fmt.Sprintf("%04d-%02d-%02d", n.val.get().Year, n.val.get().Month, n.val.get().Day))
}

// renderLocalTime renders a LocalTimeNode.
func renderLocalTime(n *LocalTimeNode) []byte {
	v := n.val.get()
	s := fmt.Sprintf("%02d:%02d:%02d", v.Hour, v.Minute, v.Second)
	if v.Nanosecond > 0 {
		frac := formatFractional(v.Nanosecond)
		s += "." + frac
	}
	return []byte(s)
}

// formatFractional formats nanoseconds as a fractional string, trimming trailing zeros.
func formatFractional(ns int) string {
	s := fmt.Sprintf("%09d", ns)
	// Trim trailing zeros
	s = strings.TrimRight(s, "0")
	return s
}

// renderArray renders an ArrayNode.
func renderArray(n *ArrayNode) []byte {
	if len(n.elements) == 0 {
		if len(n.trailingComments) > 0 {
			// Empty array with trailing comments -- render multiline.
			var buf []byte
			buf = append(buf, '[')
			buf = append(buf, '\n')
			for _, c := range n.trailingComments {
				buf = append(buf, c...)
			}
			buf = append(buf, ']')
			return buf
		}
		return []byte("[]")
	}

	// Check whether any element carries trivia (leading comments or inline
	// comment). If so, render in multiline format to preserve them.
	multiline := len(n.trailingComments) > 0
	if !multiline {
		for _, elem := range n.elements {
			t := elem.trivia()
			if len(t.LeadingComments) > 0 || len(t.InlineComment) > 0 {
				multiline = true
				break
			}
		}
	}

	if !multiline {
		var buf []byte
		buf = append(buf, '[')
		for i, elem := range n.elements {
			if i > 0 {
				buf = append(buf, ", "...)
			}
			buf = append(buf, serializeNode(elem)...)
		}
		buf = append(buf, ']')
		return buf
	}

	// Multiline format with comment preservation.
	var buf []byte
	buf = append(buf, '[')
	buf = append(buf, '\n')
	for i, elem := range n.elements {
		t := elem.trivia()

		// Leading comments for this element.
		for _, c := range t.LeadingComments {
			buf = append(buf, c...)
		}

		// Leading whitespace (indentation).
		if len(t.LeadingWhitespace) > 0 {
			buf = append(buf, t.LeadingWhitespace...)
		} else {
			buf = append(buf, "    "...)
		}

		// The value itself.
		buf = append(buf, serializeNode(elem)...)

		// Trailing comma.
		buf = append(buf, ',')

		// Inline comment.
		if len(t.InlineComment) > 0 {
			buf = append(buf, ' ')
			buf = append(buf, t.InlineComment...)
		}

		_ = i
		buf = append(buf, '\n')
	}

	// Trailing comments (after last element, before ']').
	for _, c := range n.trailingComments {
		buf = append(buf, c...)
	}

	buf = append(buf, ']')
	return buf
}

// renderInlineTable renders an InlineTableNode.
func renderInlineTable(n *InlineTableNode) []byte {
	if len(n.children) == 0 {
		return []byte("{}")
	}
	var buf []byte
	buf = append(buf, '{')
	for i, child := range n.children {
		if i > 0 {
			buf = append(buf, ", "...)
		}
		kv, ok := child.(*KeyValueNode)
		if !ok {
			continue
		}
		buf = append(buf, renderKeyFromParts(kv.key)...)
		buf = append(buf, " = "...)
		buf = append(buf, serializeNode(kv.val)...)
	}
	buf = append(buf, '}')
	return buf
}

// renderKeyValue renders a dirty KeyValueNode.
func renderKeyValue(n *KeyValueNode) []byte {
	var buf []byte

	// Leading trivia
	buf = append(buf, renderTrivia(n)...)

	// Key: if the key itself is clean, use renderKeyParts to preserve the
	// original key formatting (e.g., literal-quoted 'host' stays literal).
	// Only re-render from parts when the key is dirty.
	if n.key.isDirty() {
		buf = append(buf, renderKeyFromParts(n.key)...)
	} else {
		buf = append(buf, renderKeyParts(n.key)...)
	}
	buf = append(buf, " = "...)

	// Value
	buf = append(buf, serializeNode(n.val)...)

	// Inline comment
	t := n.trivia()
	if len(t.InlineComment) > 0 {
		buf = append(buf, ' ')
		buf = append(buf, t.InlineComment...)
	}

	// Trailing newline
	if len(t.TrailingNewline) > 0 {
		buf = append(buf, t.TrailingNewline...)
	} else {
		buf = append(buf, '\n')
	}
	return buf
}

// renderKeyParts renders a KeyNode, preferring its raw bytes if clean.
func renderKeyParts(k *KeyNode) []byte {
	if !k.isDirty() && len(k.rawBytes()) > 0 {
		raw := k.rawBytes()
		// Trim trailing whitespace that may have been captured during parsing.
		for len(raw) > 0 {
			last := raw[len(raw)-1]
			if last == ' ' || last == '\t' {
				raw = raw[:len(raw)-1]
			} else {
				break
			}
		}
		return raw
	}
	return renderKeyFromParts(k)
}

// renderKeyFromParts always renders a KeyNode from its semantic parts,
// ignoring raw bytes. Used when the parent KV is dirty to avoid trailing
// whitespace from the key's raw bytes.
func renderKeyFromParts(k *KeyNode) []byte {
	var parts []string
	for _, part := range k.parts {
		if isBareKey(part) {
			parts = append(parts, part)
		} else {
			parts = append(parts, string(renderBasicString(part)))
		}
	}
	return []byte(strings.Join(parts, "."))
}

// isBareKey returns true if s can be used as a bare key (A-Za-z0-9, -, _).
// Per TOML 1.0 spec, bare keys only allow ASCII letters, digits, hyphens, and underscores.
func isBareKey(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

// renderTableHeader renders a dirty TableNode header.
func renderTableHeader(n *TableNode) []byte {
	var buf []byte
	buf = append(buf, renderTrivia(n)...)
	buf = append(buf, '[')
	buf = append(buf, renderKeyPath(n.keyPath)...)
	buf = append(buf, ']')

	t := n.trivia()
	if len(t.InlineComment) > 0 {
		buf = append(buf, ' ')
		buf = append(buf, t.InlineComment...)
	}

	if len(t.TrailingNewline) > 0 {
		buf = append(buf, t.TrailingNewline...)
	} else {
		buf = append(buf, '\n')
	}
	return buf
}

// renderArrayTableHeader renders a dirty ArrayTableNode header.
func renderArrayTableHeader(n *ArrayTableNode) []byte {
	var buf []byte
	buf = append(buf, renderTrivia(n)...)
	buf = append(buf, '[', '[')
	buf = append(buf, renderKeyPath(n.keyPath)...)
	buf = append(buf, ']', ']')

	t := n.trivia()
	if len(t.InlineComment) > 0 {
		buf = append(buf, ' ')
		buf = append(buf, t.InlineComment...)
	}

	if len(t.TrailingNewline) > 0 {
		buf = append(buf, t.TrailingNewline...)
	} else {
		buf = append(buf, '\n')
	}
	return buf
}

// renderKeyPath renders a dotted key path (for table headers).
func renderKeyPath(parts []string) []byte {
	var rendered []string
	for _, part := range parts {
		if isBareKey(part) {
			rendered = append(rendered, part)
		} else {
			rendered = append(rendered, string(renderBasicString(part)))
		}
	}
	return []byte(strings.Join(rendered, "."))
}

// renderComment renders a dirty CommentNode.
func renderComment(n *CommentNode) []byte {
	if n.text == "" {
		return []byte("\n")
	}
	text := n.text
	// If it already looks like a comment line (starts with #), use as-is
	if len(text) > 0 && text[0] == '#' {
		if !strings.HasSuffix(text, "\n") {
			return []byte(text + "\n")
		}
		return []byte(text)
	}
	return []byte("# " + text + "\n")
}

// renderTrivia emits the leading trivia (blank lines, comments and whitespace)
// for a dirty node.
func renderTrivia(n Node) []byte {
	t := n.trivia()
	var buf []byte
	for i, c := range t.LeadingComments {
		buf = append(buf, t.blankRun(i)...)
		buf = append(buf, c...)
	}
	buf = append(buf, t.blankRun(len(t.LeadingComments))...)
	buf = append(buf, t.LeadingWhitespace...)
	return buf
}
