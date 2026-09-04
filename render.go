package tomledit

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Bytes serializes the document back to TOML bytes.
//
// A document parsed and never edited renders as the exact bytes it was parsed
// from. An edited one renders per FRAGMENT: every byte range an edit did not
// touch is written back as it was read, and only the ranges the edits
// invalidated are rendered anew, in the library's canonical spellings.
//
// So a value write leaves the line's spacing, its inline comment and the
// quoting of its key exactly as they were; a comment write leaves the value's
// base and quoting; a write inside an array or an inline table leaves every
// sibling, separator and interior comment; and a rename leaves everything but
// the renamed key part. What the library writes -- and only that -- is written
// in the forms of QuoteString, QuoteKey and FormatFloat.
func (d *Document) Bytes() []byte {
	return serializeChildren(nil, d.children)
}

// serializeChildren appends each child's bytes to buf, keeping one construct
// from continuing another's line. A file whose last line carries no newline
// gives that construct none either -- which is exactly right until something is
// written after it, and then the two would run together into a document that no
// longer parses. The separator belongs to neither construct's fragments, so the
// serializer writes it where the join happens.
func serializeChildren(buf []byte, children []Node) []byte {
	for _, child := range children {
		if n := len(buf); n > 0 && buf[n-1] != '\n' {
			buf = append(buf, '\n')
		}
		buf = append(buf, serializeNode(child)...)
	}
	return buf
}

// spliceOriginalBytes governs whether the serializer may write the bytes a
// construct was read with. It is true in every ordinary use -- splicing is what
// fidelity IS -- and the package's own corpus battery turns it off to render
// every document from its semantic content alone, which is the only way to put
// the renderers themselves under the whole corpus. Nothing exported reads or
// writes it.
var spliceOriginalBytes = true

// serializeNode dispatches serialization for a single node. A node nothing
// under it changed emits the bytes it was read with; one that did is rendered
// fragment by fragment, so the parts of it that were not written still emit
// theirs.
func serializeNode(n Node) []byte {
	switch node := n.(type) {
	case *TableNode:
		var buf []byte
		if spliceOriginalBytes && !node.isDirty() {
			buf = append(buf, node.rawBytes()...)
		} else {
			buf = append(buf, renderTableHeader(node)...)
		}
		return serializeChildren(buf, node.children)

	case *ArrayTableNode:
		var buf []byte
		if spliceOriginalBytes && !node.isDirty() {
			buf = append(buf, node.rawBytes()...)
		} else {
			buf = append(buf, renderArrayTableHeader(node)...)
		}
		return serializeChildren(buf, node.children)

	case *KeyValueNode:
		if spliceOriginalBytes && !node.subtreeDirty() {
			return node.rawBytes()
		}
		return renderKeyValue(node)

	case *CommentNode:
		if spliceOriginalBytes && !node.isDirty() {
			return node.rawBytes()
		}
		return renderComment(node)

	case *ArrayNode, *InlineTableNode:
		// A container that nothing under it changed splices its whole range;
		// one that did re-renders per fragment, splicing the brackets, the
		// separators and every clean element inside it.
		if spliceOriginalBytes && !n.subtreeDirty() {
			return n.rawBytes()
		}
		return renderValue(n)

	default:
		// Scalars. A clean one splices, like everything else; a marked one
		// splices too when it still has a lexeme, because the lexeme IS the
		// value fragment's clean bytes and the one thing that drops it is a
		// write to the payload. That is what keeps a comment written beside
		// 0x2A from rewriting it as 42.
		if !spliceOriginalBytes {
			return renderValue(n)
		}
		if !n.subtreeDirty() {
			return n.rawBytes()
		}
		if lexeme := n.rawBytes(); lexeme != nil {
			return lexeme
		}
		return renderValue(n)
	}
}

// renderValue renders a value node from its semantic content, ignoring whatever
// bytes it may have been read with. It is the canonical rendering of the
// design record for the scalar kinds, and the fragment-wise rendering of the
// two container kinds.
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

// renderBasicString renders a value as a TOML basic string with proper
// escaping. It is QuoteString, the exported form of the same rule.
func renderBasicString(s string) []byte {
	return []byte(QuoteString(s))
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
				b.WriteString(fmt.Sprintf(`\u%04x`, r))
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

// renderFloat renders a FloatNode in the canonical form of FormatFloat.
func renderFloat(n *FloatNode) []byte {
	return []byte(FormatFloat(n.val.get()))
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

// renderArray renders an ArrayNode. When the gap fragments still describe it --
// nothing was added, removed, or given a comment of its own -- every byte but
// the elements themselves is spliced, and each element answers for itself.
// Otherwise the array is rebuilt from its elements and their trivia.
func renderArray(n *ArrayNode) []byte {
	if gaps, ok := containerGaps(n); ok {
		return spliceContainer(gaps, n.elements, serializeNode)
	}
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

// renderInlineTable renders an InlineTableNode, splicing its gap fragments on
// the same terms as an array's.
func renderInlineTable(n *InlineTableNode) []byte {
	if gaps, ok := containerGaps(n); ok {
		return spliceContainer(gaps, n.children, renderInlinePair)
	}
	if len(n.children) == 0 {
		return []byte("{}")
	}
	var buf []byte
	buf = append(buf, '{')
	for i, child := range n.children {
		if i > 0 {
			buf = append(buf, ", "...)
		}
		buf = append(buf, renderInlinePair(child)...)
	}
	buf = append(buf, '}')
	return buf
}

// renderInlinePair renders one pair of an inline table: key, separator, value.
// A pair inside an inline table carries no trivia -- TOML gives it nowhere to
// put a comment, and it ends at its value rather than at a newline.
func renderInlinePair(child Node) []byte {
	kv, ok := child.(*KeyValueNode)
	if !ok {
		return nil
	}
	var buf []byte
	buf = append(buf, renderKey(kv.key)...)
	buf = append(buf, renderSeparator(kv)...)
	return append(buf, serializeNode(kv.val)...)
}

// spliceContainer writes a container from its gap fragments and its elements:
// gap, element, gap, element, ... , gap.
func spliceContainer(gaps [][]byte, items []Node, render func(Node) []byte) []byte {
	var buf []byte
	for i, item := range items {
		buf = append(buf, gaps[i]...)
		buf = append(buf, render(item)...)
	}
	return append(buf, gaps[len(items)]...)
}

// renderKeyValue renders a KeyValueNode fragment by fragment: leading trivia,
// key, separator, value, inline comment, trailing newline. Each one splices the
// bytes it was written as unless the write invalidated it.
func renderKeyValue(n *KeyValueNode) []byte {
	var buf []byte
	buf = append(buf, renderTrivia(n)...)
	buf = append(buf, renderKey(n.key)...)
	buf = append(buf, renderSeparator(n)...)
	buf = append(buf, serializeNode(n.val)...)
	return append(buf, renderLineTail(n)...)
}

// renderSeparator writes the bytes between a pair's key and its value: the ones
// it was written with, or the canonical " = " for a pair written from scratch.
func renderSeparator(n *KeyValueNode) []byte {
	if spliceOriginalBytes && n.sep != nil {
		return n.sep
	}
	return []byte(" = ")
}

// renderLineTail writes what follows a construct on its line: the whitespace it
// was written with, its inline comment, and its trailing newline. A construct
// that ended the file without a newline keeps ending without one.
func renderLineTail(n Node) []byte {
	t := n.trivia()
	// The gap is written whatever follows it: before a comment it is the
	// spacing the line carried (or the one the comment's writer chose), and
	// with no comment it is the trailing whitespace the line ended with.
	buf := append([]byte(nil), t.inlineGap...)
	buf = append(buf, t.InlineComment...)
	if len(t.TrailingNewline) > 0 {
		return append(buf, t.TrailingNewline...)
	}
	if n.rawBytes() == nil {
		// A construct built rather than parsed ends its own line; one parsed at
		// end of file without a newline keeps ending without one.
		return append(buf, '\n')
	}
	return buf
}

// renderKey renders a KeyNode from its fragments: every part splices the bytes
// it was written as (or, for a renamed part, its canonical spelling) and the
// dots and whitespace between them splice too. A key whose fragments were never
// captured renders from its parts.
func renderKey(k *KeyNode) []byte {
	if spliceOriginalBytes && k.frag.describes(len(k.parts)) {
		return k.frag.splice()
	}
	return renderKeyFromParts(k)
}

// renderKeyFromParts always renders a KeyNode from its semantic parts,
// ignoring raw bytes. Used when the parent KV is dirty to avoid trailing
// whitespace from the key's raw bytes.
func renderKeyFromParts(k *KeyNode) []byte {
	parts := make([]string, len(k.parts))
	for i, part := range k.parts {
		parts[i] = QuoteKey(part)
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

// renderTableHeader renders a TableNode header fragment by fragment.
func renderTableHeader(n *TableNode) []byte {
	var buf []byte
	buf = append(buf, renderTrivia(n)...)
	buf = append(buf, renderHeaderKey(n.header, n.keyPath, "[", "]")...)
	return append(buf, renderLineTail(n)...)
}

// renderArrayTableHeader renders an ArrayTableNode header fragment by fragment.
func renderArrayTableHeader(n *ArrayTableNode) []byte {
	var buf []byte
	buf = append(buf, renderTrivia(n)...)
	buf = append(buf, renderHeaderKey(n.header, n.keyPath, "[[", "]]")...)
	return append(buf, renderLineTail(n)...)
}

// renderHeaderKey writes a header's brackets and key. The captured fragments
// carry the brackets themselves, so a header written "[ 'a' . b ]" comes back
// spelled exactly that way; a header created programmatically has none, and is
// written canonically between the brackets it is given.
func renderHeaderKey(frag keyFragments, keyPath []string, open, close string) []byte {
	if spliceOriginalBytes && frag.describes(len(keyPath)) {
		return frag.splice()
	}
	var buf []byte
	buf = append(buf, open...)
	buf = append(buf, renderKeyPath(keyPath)...)
	return append(buf, close...)
}

// renderKeyPath renders a dotted key path (for table headers).
func renderKeyPath(parts []string) []byte {
	rendered := make([]string, len(parts))
	for i, part := range parts {
		rendered[i] = QuoteKey(part)
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
