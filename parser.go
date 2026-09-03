package tomledit

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Parse lexes and parses TOML source bytes into a Document AST.
//
// The returned Document preserves all whitespace, comments, and formatting
// from the original source. Serializing it back with Bytes produces the exact
// original bytes (round-trip fidelity).
//
// Returns a *ParseError on any lexing or parsing error, including duplicate
// key detection and invalid TOML syntax.
func Parse(src []byte) (*Document, error) {
	tokens, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{
		tokens: tokens,
		pos:    0,
		src:    src,
	}
	return p.parseDocument()
}

// parser is the internal recursive-descent parser.
type parser struct {
	tokens []Token
	pos    int
	src    []byte
}

// --- token helpers ---

func (p *parser) peek() Token {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return Token{Type: TokenEOF}
}

func (p *parser) peekType() TokenType {
	return p.peek().Type
}

func (p *parser) advance() Token {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

func (p *parser) expect(typ TokenType) (Token, error) {
	tok := p.advance()
	if tok.Type != typ {
		return tok, p.errorfAt(tok, "expected %s, got %s", typ, tok.Type)
	}
	return tok, nil
}

func (p *parser) errorfAt(tok Token, format string, args ...any) error {
	return &ParseError{
		Line:    tok.Line,
		Column:  tok.Column,
		Message: fmt.Sprintf(format, args...),
	}
}

func (p *parser) errorf(format string, args ...any) error {
	tok := p.peek()
	return p.errorfAt(tok, format, args...)
}

// --- trivia collection ---

// collectLeadingTrivia consumes whitespace, comments, and newlines that precede
// a content node. It returns:
//   - leadingWS: whitespace on the same line as the coming content
//   - leadingComments: full comment lines (comment + newline)
//   - raw: all consumed bytes concatenated
func (p *parser) collectLeadingTrivia() (leadingWS []byte, leadingComments [][]byte, raw []byte, orphan orphanTrivia) {
	// Accumulate lines. A "line" may be: whitespace, comment+newline, blank newline.
	// The last whitespace run (if not followed by newline or comment) is the
	// inline leading whitespace for the coming node.

	var accumulated []byte // all bytes consumed
	var pendingWS []byte   // whitespace not yet assigned

	// track records the span of every consumed trivia token so orphan
	// CommentNodes (emitted at end of input) can carry positions.
	track := func(tok Token) {
		sp := spanFromToken(tok)
		if !orphan.span.IsValid() {
			orphan.span.Start = sp.Start
		}
		orphan.span.End = sp.End
	}

	for {
		tt := p.peekType()
		switch tt {
		case TokenWhitespace:
			tok := p.advance()
			track(tok)
			pendingWS = append(pendingWS, tok.Raw...)
			continue

		case TokenNewline:
			tok := p.advance()
			track(tok)
			// The pending whitespace + this newline is a blank line
			accumulated = append(accumulated, pendingWS...)
			accumulated = append(accumulated, tok.Raw...)
			pendingWS = nil
			continue

		case TokenComment:
			tok := p.advance()
			track(tok)
			// A comment line: pendingWS is its leading whitespace
			commentLine := make([]byte, 0, len(pendingWS)+len(tok.Raw))
			commentLine = append(commentLine, pendingWS...)
			commentLine = append(commentLine, tok.Raw...)
			pendingWS = nil
			orphan.commentSpans = append(orphan.commentSpans, spanFromToken(tok))

			// Consume the newline after the comment if present
			if p.peekType() == TokenNewline {
				nl := p.advance()
				track(nl)
				commentLine = append(commentLine, nl.Raw...)
			}
			accumulated = append(accumulated, commentLine...)
			leadingComments = append(leadingComments, commentLine)
			continue

		default:
			// Not trivia. pendingWS is the leading whitespace for the content node.
			leadingWS = pendingWS
			raw = append(accumulated, pendingWS...)
			return
		}
	}
}

// orphanTrivia carries position data for trivia consumed by
// collectLeadingTrivia, used when the trivia becomes orphan CommentNodes at
// end of input.
type orphanTrivia struct {
	commentSpans []Span // parallel to leadingComments; each covers only the '#...' token
	span         Span   // the entire consumed trivia region; zero if nothing was consumed
}

// consumeInlineTrivia consumes optional whitespace + comment on the same line
// after a value (before newline/EOF).
func (p *parser) consumeInlineTrivia() (ws []byte, comment []byte) {
	if p.peekType() == TokenWhitespace {
		tok := p.advance()
		ws = tok.Raw
	}
	if p.peekType() == TokenComment {
		tok := p.advance()
		comment = tok.Raw
	}
	return
}

// consumeTrailingNewline consumes a newline or checks for EOF.
func (p *parser) consumeTrailingNewline() []byte {
	if p.peekType() == TokenNewline {
		tok := p.advance()
		return tok.Raw
	}
	return nil
}

// --- document parsing ---

func (p *parser) parseDocument() (*Document, error) {
	doc := &Document{}
	tracker := newDefinitionTracker()

	for p.peekType() != TokenEOF {
		leadingWS, leadingComments, triviaRaw, orphan := p.collectLeadingTrivia()

		tt := p.peekType()
		switch {
		case tt == TokenEOF:
			// Any remaining trivia becomes orphan comment nodes
			p.emitOrphanTrivia(doc, leadingComments, triviaRaw, orphan)

		case tt == TokenLeftBracket:
			tbl, err := p.parseTable(leadingWS, leadingComments, triviaRaw, tracker)
			if err != nil {
				return nil, err
			}
			doc.Children = append(doc.Children, tbl)

		case tt == TokenDoubleLeftBracket:
			atbl, err := p.parseArrayTable(leadingWS, leadingComments, triviaRaw, tracker)
			if err != nil {
				return nil, err
			}
			doc.Children = append(doc.Children, atbl)

		case isKeyToken(tt):
			kv, err := p.parseKeyValue(leadingWS, leadingComments, triviaRaw, tracker, nil)
			if err != nil {
				return nil, err
			}
			doc.Children = append(doc.Children, kv)

		default:
			return nil, p.errorf("unexpected token %s", tt)
		}
	}

	// The document spans from the first byte to the EOF position.
	eof := p.peek()
	doc.setSpan(Span{
		Start: Position{Line: 1, Column: 1},
		End:   Position{Line: eof.Line, Column: eof.Column},
	})

	return doc, nil
}

func (p *parser) emitOrphanTrivia(parent interface{ addChild(Node) }, comments [][]byte, raw []byte, orphan orphanTrivia) {
	if len(comments) == 0 && len(raw) == 0 {
		return
	}
	// If we have comment lines, emit them as CommentNodes
	if len(comments) > 0 {
		for i, c := range comments {
			cn := &CommentNode{Text: string(c)}
			cn.setRaw(c)
			if i < len(orphan.commentSpans) {
				cn.setSpan(orphan.commentSpans[i])
			}
			parent.addChild(cn)
		}
		return
	}
	// If only blank lines (raw without comments), attach as a comment node with empty text
	if len(raw) > 0 {
		cn := &CommentNode{Text: ""}
		cn.setRaw(raw)
		cn.setSpan(orphan.span)
		parent.addChild(cn)
	}
}

// childAdder is implemented by Document, TableNode, ArrayTableNode.
type childAdder interface {
	addChild(Node)
}

func (n *Document) addChild(c Node)       { n.Children = append(n.Children, c) }
func (n *TableNode) addChild(c Node)      { n.Children = append(n.Children, c) }
func (n *ArrayTableNode) addChild(c Node) { n.Children = append(n.Children, c) }

func isKeyToken(tt TokenType) bool {
	return tt == TokenBareKey || tt == TokenBasicString || tt == TokenLiteralString
}

// --- table parsing ---

func (p *parser) parseTable(leadingWS []byte, leadingComments [][]byte, triviaRaw []byte, tracker *definitionTracker) (*TableNode, error) {
	startPos := p.pos

	// consume [
	openTok, err := p.expect(TokenLeftBracket)
	if err != nil {
		return nil, err
	}

	// optional whitespace inside bracket
	p.skipWhitespace()

	// parse key path
	keyPath, _, keyLine, keyCol, err := p.parseKeyPath()
	if err != nil {
		return nil, err
	}

	p.skipWhitespace()

	// consume ]
	closeTok, err := p.expect(TokenRightBracket)
	if err != nil {
		return nil, err
	}

	// inline comment
	_, inlineComment := p.consumeInlineTrivia()

	// After the table header, only a newline or EOF is allowed
	if tt := p.peekType(); tt != TokenNewline && tt != TokenEOF {
		return nil, p.errorf("unexpected content after table header")
	}

	// trailing newline
	trailingNL := p.consumeTrailingNewline()

	// Build raw bytes for the header line
	headerRaw := p.rawFromTokenRange(startPos, p.pos)
	headerRaw = append(triviaRaw, headerRaw...)

	tbl := &TableNode{
		KeyPath: keyPath,
	}
	tbl.setRaw(headerRaw)
	// The table span covers only the [header], not the children.
	tbl.setSpan(Span{Start: tokenStart(openTok), End: spanFromToken(closeTok).End})
	tbl.nodeTrivia.LeadingWhitespace = leadingWS
	tbl.nodeTrivia.LeadingComments = leadingComments
	tbl.nodeTrivia.InlineComment = inlineComment
	tbl.nodeTrivia.TrailingNewline = trailingNL

	// Check for redefinition
	if err := tracker.defineTable(keyPath, tbl, keyLine, keyCol); err != nil {
		return nil, err
	}

	// Parse children (key-value pairs and orphan comments) until next table or EOF
	childTracker := tracker.tableScope(keyPath)
	if err := p.parseTableChildren(tbl, childTracker, tracker); err != nil {
		return nil, err
	}

	return tbl, nil
}

func (p *parser) parseArrayTable(leadingWS []byte, leadingComments [][]byte, triviaRaw []byte, tracker *definitionTracker) (*ArrayTableNode, error) {
	startPos := p.pos

	// consume [[
	openTok, err := p.expect(TokenDoubleLeftBracket)
	if err != nil {
		return nil, err
	}

	p.skipWhitespace()

	// parse key path
	keyPath, _, keyLine, keyCol, err := p.parseKeyPath()
	if err != nil {
		return nil, err
	}

	p.skipWhitespace()

	// consume ]]
	closeTok, err := p.expect(TokenDoubleRightBracket)
	if err != nil {
		return nil, err
	}

	// inline comment
	_, inlineComment := p.consumeInlineTrivia()

	// After the array table header, only a newline or EOF is allowed
	if tt := p.peekType(); tt != TokenNewline && tt != TokenEOF {
		return nil, p.errorf("unexpected content after array table header")
	}

	// trailing newline
	trailingNL := p.consumeTrailingNewline()

	// Build raw bytes for the header line
	headerRaw := p.rawFromTokenRange(startPos, p.pos)
	headerRaw = append(triviaRaw, headerRaw...)

	atbl := &ArrayTableNode{
		KeyPath: keyPath,
	}
	atbl.setRaw(headerRaw)
	// The array-table span covers only the [[header]], not the children.
	atbl.setSpan(Span{Start: tokenStart(openTok), End: spanFromToken(closeTok).End})
	atbl.nodeTrivia.LeadingWhitespace = leadingWS
	atbl.nodeTrivia.LeadingComments = leadingComments
	atbl.nodeTrivia.InlineComment = inlineComment
	atbl.nodeTrivia.TrailingNewline = trailingNL

	// Check for redefinition
	if err := tracker.defineArrayTable(keyPath, atbl, keyLine, keyCol); err != nil {
		return nil, err
	}

	// Parse children
	childTracker := tracker.arrayTableScope(keyPath)
	if err := p.parseTableChildren(atbl, childTracker, tracker); err != nil {
		return nil, err
	}

	return atbl, nil
}

func (p *parser) parseTableChildren(parent childAdder, childTracker *definitionTracker, rootTracker *definitionTracker) error {
	for {
		// Peek ahead past trivia to see what's next
		savedPos := p.pos
		leadingWS, leadingComments, triviaRaw, orphan := p.collectLeadingTrivia()

		tt := p.peekType()
		switch {
		case tt == TokenEOF:
			p.emitOrphanTrivia(parent, leadingComments, triviaRaw, orphan)
			return nil

		case tt == TokenLeftBracket || tt == TokenDoubleLeftBracket:
			// End of this table's children. Restore position so the outer loop handles it.
			p.pos = savedPos
			return nil

		case isKeyToken(tt):
			kv, err := p.parseKeyValue(leadingWS, leadingComments, triviaRaw, childTracker, nil)
			if err != nil {
				return err
			}
			parent.addChild(kv)

		default:
			return p.errorf("unexpected token %s in table body", tt)
		}
	}
}

// --- key-value parsing ---

func (p *parser) parseKeyValue(leadingWS []byte, leadingComments [][]byte, triviaRaw []byte, tracker *definitionTracker, parentPath []string) (*KeyValueNode, error) {
	startPos := p.pos

	// Parse key (possibly dotted)
	keyNode, keyLine, keyCol, err := p.parseKey()
	if err != nil {
		return nil, err
	}

	p.skipWhitespace()

	// Expect =
	_, err = p.expect(TokenEquals)
	if err != nil {
		return nil, err
	}

	p.skipWhitespace()

	// Parse value
	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	// Inline trivia
	_, inlineComment := p.consumeInlineTrivia()

	// Trailing newline
	trailingNL := p.consumeTrailingNewline()

	// Build raw bytes
	kvRaw := p.rawFromTokenRange(startPos, p.pos)
	kvRaw = append(triviaRaw, kvRaw...)

	kv := &KeyValueNode{
		Key: keyNode,
		Val: val,
	}
	kv.setRaw(kvRaw)
	// The key-value span runs from the key's first byte to the value's last
	// byte, excluding leading trivia, inline comment, and trailing newline.
	kv.setSpan(Span{Start: keyNode.Span().Start, End: val.Span().End})
	kv.nodeTrivia.LeadingWhitespace = leadingWS
	kv.nodeTrivia.LeadingComments = leadingComments
	kv.nodeTrivia.InlineComment = inlineComment
	kv.nodeTrivia.TrailingNewline = trailingNL

	// Track definition
	fullPath := append(parentPath, keyNode.Parts...)
	if err := tracker.defineKey(fullPath, kv, keyLine, keyCol); err != nil {
		return nil, err
	}

	return kv, nil
}

// --- key parsing ---

// parseKey parses a (possibly dotted) key, returning the KeyNode and the
// position (line, col) of the first key token.
func (p *parser) parseKey() (*KeyNode, int, int, error) {
	key := &KeyNode{}
	startPos := p.pos

	// Capture position of the first key token
	firstTok := p.peek()

	// First part
	part, rawPart, style, err := p.parseSimpleKey()
	if err != nil {
		return nil, 0, 0, err
	}
	key.Parts = append(key.Parts, part)
	key.RawParts = append(key.RawParts, rawPart)
	key.Styles = append(key.Styles, style)
	// parseSimpleKey consumes exactly one token; track the last key token so
	// the span ends at the final key part (excluding trailing whitespace).
	lastTok := p.tokens[p.pos-1]

	// Additional dotted parts
	for {
		// Optional whitespace before dot
		p.skipWhitespace()
		if p.peekType() != TokenDot {
			break
		}
		p.advance() // consume dot
		p.skipWhitespace()

		part, rawPart, style, err = p.parseSimpleKey()
		if err != nil {
			return nil, 0, 0, err
		}
		key.Parts = append(key.Parts, part)
		key.RawParts = append(key.RawParts, rawPart)
		key.Styles = append(key.Styles, style)
		lastTok = p.tokens[p.pos-1]
	}

	key.setRaw(p.rawFromTokenRange(startPos, p.pos))
	key.setSpan(Span{Start: tokenStart(firstTok), End: spanFromToken(lastTok).End})
	return key, firstTok.Line, firstTok.Column, nil
}

func (p *parser) parseSimpleKey() (decoded string, raw []byte, style StringStyle, err error) {
	tok := p.peek()
	switch tok.Type {
	case TokenBareKey:
		p.advance()
		return string(tok.Raw), tok.Raw, StringBasic, nil
	case TokenBasicString:
		p.advance()
		decoded, err = decodeBasicString(tok.Raw)
		if err != nil {
			return "", nil, 0, &ParseError{Line: tok.Line, Column: tok.Column, Message: err.Error()}
		}
		return decoded, tok.Raw, StringBasic, nil
	case TokenLiteralString:
		p.advance()
		decoded = decodeLiteralString(tok.Raw)
		return decoded, tok.Raw, StringLiteral, nil
	default:
		return "", nil, 0, p.errorf("expected key, got %s", tok.Type)
	}
}

// parseKeyPath parses a dotted key for table headers (without consuming =).
// Returns the decoded parts, raw parts, the position (line, col) of the first
// key token, and any error.
func (p *parser) parseKeyPath() ([]string, [][]byte, int, int, error) {
	var parts []string
	var rawParts [][]byte

	firstTok := p.peek()
	part, rawPart, _, err := p.parseSimpleKey()
	if err != nil {
		return nil, nil, 0, 0, err
	}
	parts = append(parts, part)
	rawParts = append(rawParts, rawPart)

	for {
		p.skipWhitespace()
		if p.peekType() != TokenDot {
			break
		}
		p.advance() // consume dot
		p.skipWhitespace()

		part, rawPart, _, err = p.parseSimpleKey()
		if err != nil {
			return nil, nil, 0, 0, err
		}
		parts = append(parts, part)
		rawParts = append(rawParts, rawPart)
	}

	return parts, rawParts, firstTok.Line, firstTok.Column, nil
}

// --- value parsing ---

func (p *parser) parseValue() (Node, error) {
	tok := p.peek()
	switch tok.Type {
	case TokenBasicString:
		return p.parseStringValue()
	case TokenLiteralString:
		return p.parseLiteralStringValue()
	case TokenMultiLineBasicString:
		return p.parseMultiLineBasicStringValue()
	case TokenMultiLineLiteralString:
		return p.parseMultiLineLiteralStringValue()
	case TokenInteger:
		return p.parseIntegerValue()
	case TokenFloat:
		return p.parseFloatValue()
	case TokenBoolean:
		return p.parseBooleanValue()
	case TokenOffsetDateTime:
		return p.parseDateTimeValue()
	case TokenLocalDateTime:
		return p.parseLocalDateTimeValue()
	case TokenLocalDate:
		return p.parseLocalDateValue()
	case TokenLocalTime:
		return p.parseLocalTimeValue()
	case TokenLeftBracket:
		return p.parseArrayValue()
	case TokenLeftBrace:
		return p.parseInlineTableValue()
	default:
		return nil, p.errorf("expected value, got %s", tok.Type)
	}
}

func (p *parser) parseStringValue() (Node, error) {
	tok := p.advance()
	decoded, err := decodeBasicString(tok.Raw)
	if err != nil {
		return nil, &ParseError{Line: tok.Line, Column: tok.Column, Message: err.Error()}
	}
	n := &StringNode{Val: decoded, Style: StringBasic}
	n.setRaw(tok.Raw)
	n.setSpan(spanFromToken(tok))
	return n, nil
}

func (p *parser) parseLiteralStringValue() (Node, error) {
	tok := p.advance()
	decoded := decodeLiteralString(tok.Raw)
	n := &StringNode{Val: decoded, Style: StringLiteral}
	n.setRaw(tok.Raw)
	n.setSpan(spanFromToken(tok))
	return n, nil
}

func (p *parser) parseMultiLineBasicStringValue() (Node, error) {
	tok := p.advance()
	decoded, err := decodeMultiLineBasicString(tok.Raw)
	if err != nil {
		return nil, &ParseError{Line: tok.Line, Column: tok.Column, Message: err.Error()}
	}
	n := &StringNode{Val: decoded, Style: StringMultiLineBasic}
	n.setRaw(tok.Raw)
	n.setSpan(spanFromToken(tok))
	return n, nil
}

func (p *parser) parseMultiLineLiteralStringValue() (Node, error) {
	tok := p.advance()
	decoded := decodeMultiLineLiteralString(tok.Raw)
	n := &StringNode{Val: decoded, Style: StringMultiLineLiteral}
	n.setRaw(tok.Raw)
	n.setSpan(spanFromToken(tok))
	return n, nil
}

func (p *parser) parseIntegerValue() (Node, error) {
	tok := p.advance()
	val, base, err := parseInteger(tok.Raw)
	if err != nil {
		return nil, &ParseError{Line: tok.Line, Column: tok.Column, Message: err.Error()}
	}
	n := &IntegerNode{Val: val, Base: base}
	n.setRaw(tok.Raw)
	n.setSpan(spanFromToken(tok))
	return n, nil
}

func (p *parser) parseFloatValue() (Node, error) {
	tok := p.advance()
	val, err := parseFloat(tok.Raw)
	if err != nil {
		return nil, &ParseError{Line: tok.Line, Column: tok.Column, Message: err.Error()}
	}
	n := &FloatNode{Val: val}
	n.setRaw(tok.Raw)
	n.setSpan(spanFromToken(tok))
	return n, nil
}

func (p *parser) parseBooleanValue() (Node, error) {
	tok := p.advance()
	val := string(tok.Raw) == "true"
	n := &BooleanNode{Val: val}
	n.setRaw(tok.Raw)
	n.setSpan(spanFromToken(tok))
	return n, nil
}

func (p *parser) parseDateTimeValue() (Node, error) {
	tok := p.advance()
	val, err := parseOffsetDateTime(string(tok.Raw))
	if err != nil {
		return nil, &ParseError{Line: tok.Line, Column: tok.Column, Message: err.Error()}
	}
	n := &DateTimeNode{Val: val}
	n.setRaw(tok.Raw)
	n.setSpan(spanFromToken(tok))
	return n, nil
}

func (p *parser) parseLocalDateTimeValue() (Node, error) {
	tok := p.advance()
	val, err := parseLocalDateTime(string(tok.Raw))
	if err != nil {
		return nil, &ParseError{Line: tok.Line, Column: tok.Column, Message: err.Error()}
	}
	n := &LocalDateTimeNode{Val: val}
	n.setRaw(tok.Raw)
	n.setSpan(spanFromToken(tok))
	return n, nil
}

func (p *parser) parseLocalDateValue() (Node, error) {
	tok := p.advance()
	val, err := parseLocalDate(string(tok.Raw))
	if err != nil {
		return nil, &ParseError{Line: tok.Line, Column: tok.Column, Message: err.Error()}
	}
	n := &LocalDateNode{Val: val}
	n.setRaw(tok.Raw)
	n.setSpan(spanFromToken(tok))
	return n, nil
}

func (p *parser) parseLocalTimeValue() (Node, error) {
	tok := p.advance()
	val, err := parseLocalTime(string(tok.Raw))
	if err != nil {
		return nil, &ParseError{Line: tok.Line, Column: tok.Column, Message: err.Error()}
	}
	n := &LocalTimeNode{Val: val}
	n.setRaw(tok.Raw)
	n.setSpan(spanFromToken(tok))
	return n, nil
}

func (p *parser) parseArrayValue() (Node, error) {
	startPos := p.pos
	openTok := p.advance() // consume [

	arr := &ArrayNode{}

	for {
		// Collect leading trivia (whitespace, newlines, comments) before the
		// next element or closing bracket.
		leadingWS, leadingComments := p.collectArrayTrivia()

		if p.peekType() == TokenRightBracket {
			// Comments collected here belong after the last element (or are the
			// only content in an empty array). Store them as trailing comments.
			arr.TrailingComments = leadingComments
			closeTok := p.advance() // consume ]
			arr.setRaw(p.rawFromTokenRange(startPos, p.pos))
			arr.setSpan(Span{Start: tokenStart(openTok), End: spanFromToken(closeTok).End})
			return arr, nil
		}

		elem, err := p.parseValue()
		if err != nil {
			return nil, err
		}

		// Attach leading trivia to this element.
		t := elem.trivia()
		t.LeadingWhitespace = leadingWS
		t.LeadingComments = leadingComments

		arr.Elements = append(arr.Elements, elem)

		// After the value, there may be:
		//   value, # comment    (comma first, then inline comment)
		//   value # comment \n, (inline comment first, then comma on next line)
		//   value,              (just comma)
		//   value ]             (last element, no comma)
		//   value # comment ]   (last element with comment)
		//
		// First, try to find comma or inline comment or ] by skipping whitespace.
		p.skipArrayWhitespace()

		switch p.peekType() {
		case TokenComma:
			p.advance() // consume comma
			// Inline comment after the comma on the same line.
			_, inlineComment := p.collectArrayInlineTrivia()
			if len(inlineComment) > 0 {
				t.InlineComment = inlineComment
			}
			continue

		case TokenComment:
			// Inline comment without a preceding comma (e.g., `2#,9\n,`).
			_, inlineComment := p.collectArrayInlineTrivia()
			if len(inlineComment) > 0 {
				t.InlineComment = inlineComment
			}
			// After the comment, skip whitespace/newlines and look for comma or ].
			p.skipArrayTrivia()
			if p.peekType() == TokenComma {
				p.advance()
				continue
			}
			if p.peekType() == TokenRightBracket {
				continue // will be consumed at top of loop
			}
			return nil, p.errorf("expected ',' or ']' in array, got %s", p.peekType())

		case TokenRightBracket:
			continue // will be consumed at top of loop

		default:
			return nil, p.errorf("expected ',' or ']' in array, got %s", p.peekType())
		}
	}
}

func (p *parser) parseInlineTableValue() (Node, error) {
	startPos := p.pos
	openTok := p.advance() // consume {

	tbl := &InlineTableNode{}
	tracker := newDefinitionTracker()

	p.skipWhitespace()

	if p.peekType() == TokenRightBrace {
		closeTok := p.advance() // consume }
		tbl.setRaw(p.rawFromTokenRange(startPos, p.pos))
		tbl.setSpan(Span{Start: tokenStart(openTok), End: spanFromToken(closeTok).End})
		return tbl, nil
	}

	for {
		p.skipWhitespace()

		keyNode, keyLine, keyCol, err := p.parseKey()
		if err != nil {
			return nil, err
		}

		p.skipWhitespace()

		_, err = p.expect(TokenEquals)
		if err != nil {
			return nil, err
		}

		p.skipWhitespace()

		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}

		kv := &KeyValueNode{
			Key: keyNode,
			Val: val,
		}
		kv.setSpan(Span{Start: keyNode.Span().Start, End: val.Span().End})

		// Check for duplicate keys within the inline table
		if err := tracker.defineKey(keyNode.Parts, kv, keyLine, keyCol); err != nil {
			return nil, err
		}

		// raw for inline table kv is set at inline table level
		tbl.Children = append(tbl.Children, kv)

		p.skipWhitespace()

		if p.peekType() == TokenComma {
			p.advance()
			continue
		}

		break
	}

	p.skipWhitespace()

	closeTok, err := p.expect(TokenRightBrace)
	if err != nil {
		return nil, err
	}

	tbl.setRaw(p.rawFromTokenRange(startPos, p.pos))
	tbl.setSpan(Span{Start: tokenStart(openTok), End: spanFromToken(closeTok).End})
	return tbl, nil
}

// --- helpers ---

func (p *parser) skipWhitespace() {
	for p.peekType() == TokenWhitespace {
		p.advance()
	}
}

func (p *parser) skipArrayTrivia() {
	for {
		tt := p.peekType()
		if tt == TokenWhitespace || tt == TokenNewline || tt == TokenComment {
			p.advance()
			continue
		}
		break
	}
}

// skipArrayWhitespace consumes whitespace and newlines (but NOT comments)
// inside an array.
func (p *parser) skipArrayWhitespace() {
	for {
		tt := p.peekType()
		if tt == TokenWhitespace || tt == TokenNewline {
			p.advance()
			continue
		}
		break
	}
}

// collectArrayTrivia collects whitespace, newlines, and comments before an
// array element or closing bracket. It returns the final leading whitespace
// (indentation before the element) and any full comment lines collected.
func (p *parser) collectArrayTrivia() (leadingWS []byte, leadingComments [][]byte) {
	var pendingWS []byte

	for {
		tt := p.peekType()
		switch tt {
		case TokenWhitespace:
			tok := p.advance()
			pendingWS = append(pendingWS, tok.Raw...)
			continue

		case TokenNewline:
			tok := p.advance()
			// Pending whitespace + newline is just a blank line; discard
			// the whitespace (it's not comment content).
			_ = tok
			pendingWS = nil
			continue

		case TokenComment:
			tok := p.advance()
			// Build a comment line: indentation + comment text
			commentLine := make([]byte, 0, len(pendingWS)+len(tok.Raw))
			commentLine = append(commentLine, pendingWS...)
			commentLine = append(commentLine, tok.Raw...)
			pendingWS = nil

			// Consume the newline after the comment if present.
			if p.peekType() == TokenNewline {
				nl := p.advance()
				commentLine = append(commentLine, nl.Raw...)
			}
			leadingComments = append(leadingComments, commentLine)
			continue

		default:
			// Not trivia. pendingWS is the leading whitespace (indentation)
			// for the coming element.
			leadingWS = pendingWS
			return
		}
	}
}

// collectArrayInlineTrivia collects optional whitespace and a comment on the
// same line after an array element value (before a comma or newline).
func (p *parser) collectArrayInlineTrivia() (ws []byte, comment []byte) {
	if p.peekType() == TokenWhitespace {
		tok := p.advance()
		ws = tok.Raw
	}
	if p.peekType() == TokenComment {
		tok := p.advance()
		comment = tok.Raw
	}
	return
}

func (p *parser) rawFromTokenRange(startIdx, endIdx int) []byte {
	if startIdx >= endIdx {
		return nil
	}
	var raw []byte
	for i := startIdx; i < endIdx && i < len(p.tokens); i++ {
		raw = append(raw, p.tokens[i].Raw...)
	}
	return raw
}

// --- string decoding ---

// decodeBasicString decodes a TOML basic string (including quotes).
func decodeBasicString(raw []byte) (string, error) {
	s := string(raw)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return "", fmt.Errorf("invalid basic string: %q", s)
	}
	inner := s[1 : len(s)-1]
	return decodeBasicStringEscapes(inner)
}

func decodeBasicStringEscapes(s string) (string, error) {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\\' {
			i++
			if i >= len(s) {
				return "", fmt.Errorf("trailing backslash in string")
			}
			switch s[i] {
			case 'b':
				b.WriteByte('\b')
				i++
			case 't':
				b.WriteByte('\t')
				i++
			case 'n':
				b.WriteByte('\n')
				i++
			case 'f':
				b.WriteByte('\f')
				i++
			case 'r':
				b.WriteByte('\r')
				i++
			case '"':
				b.WriteByte('"')
				i++
			case '\\':
				b.WriteByte('\\')
				i++
			case 'u':
				i++
				if i+4 > len(s) {
					return "", fmt.Errorf("incomplete \\u escape")
				}
				r, err := strconv.ParseUint(s[i:i+4], 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid \\u escape: %s", s[i:i+4])
				}
				if r >= 0xD800 && r <= 0xDFFF {
					return "", fmt.Errorf("surrogate code point not allowed in unicode escape: U+%04X", r)
				}
				if r > 0x10FFFF {
					return "", fmt.Errorf("unicode code point out of range: U+%08X", r)
				}
				b.WriteRune(rune(r))
				i += 4
			case 'U':
				i++
				if i+8 > len(s) {
					return "", fmt.Errorf("incomplete \\U escape")
				}
				r, err := strconv.ParseUint(s[i:i+8], 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid \\U escape: %s", s[i:i+8])
				}
				if r >= 0xD800 && r <= 0xDFFF {
					return "", fmt.Errorf("surrogate code point not allowed in unicode escape: U+%04X", r)
				}
				if r > 0x10FFFF {
					return "", fmt.Errorf("unicode code point out of range: U+%08X", r)
				}
				b.WriteRune(rune(r))
				i += 8
			default:
				return "", fmt.Errorf("invalid escape: \\%c", s[i])
			}
		} else {
			r, size := utf8.DecodeRuneInString(s[i:])
			b.WriteRune(r)
			i += size
		}
	}
	return b.String(), nil
}

func decodeLiteralString(raw []byte) string {
	s := string(raw)
	if len(s) < 2 {
		return ""
	}
	// strip quotes
	return s[1 : len(s)-1]
}

func decodeMultiLineBasicString(raw []byte) (string, error) {
	s := string(raw)
	if len(s) < 6 {
		return "", fmt.Errorf("invalid multi-line basic string")
	}
	// Strip """ from both ends. But closing may have up to 5 quotes total.
	inner := s[3:]
	// find the last """ and strip from there
	// The raw includes everything from """ to """
	// Count trailing quotes
	endQuotes := 0
	for i := len(inner) - 1; i >= 0 && inner[i] == '"'; i-- {
		endQuotes++
	}
	if endQuotes < 3 {
		return "", fmt.Errorf("invalid multi-line basic string: missing closing quotes")
	}
	// The extra quotes (beyond 3) are part of the content
	extra := endQuotes - 3
	inner = inner[:len(inner)-3] // remove closing """
	// But the 'extra' quotes at the end are content, which are already in inner
	_ = extra

	// Strip the first newline if present (TOML spec: newline immediately after opening """ is trimmed)
	if len(inner) > 0 && inner[0] == '\n' {
		inner = inner[1:]
	} else if len(inner) > 1 && inner[0] == '\r' && inner[1] == '\n' {
		inner = inner[2:]
	}

	// Process escape sequences and line-ending backslashes
	return decodeMultiLineBasicEscapes(inner)
}

func decodeMultiLineBasicEscapes(s string) (string, error) {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\\' {
			i++
			if i >= len(s) {
				return "", fmt.Errorf("trailing backslash")
			}
			// Line-ending backslash: \ may be followed by optional
			// spaces/tabs before the newline (all trimmed per TOML spec).
			j := i
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
			if j < len(s) && (s[j] == '\n' || (s[j] == '\r' && j+1 < len(s) && s[j+1] == '\n')) {
				i = j
				// skip newline
				if s[i] == '\r' {
					i += 2
				} else {
					i++
				}
				// skip following whitespace and newlines
				for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || (s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n')) {
					if s[i] == '\r' {
						i += 2
					} else {
						i++
					}
				}
				continue
			}
			switch s[i] {
			case 'b':
				b.WriteByte('\b')
				i++
			case 't':
				b.WriteByte('\t')
				i++
			case 'n':
				b.WriteByte('\n')
				i++
			case 'f':
				b.WriteByte('\f')
				i++
			case 'r':
				b.WriteByte('\r')
				i++
			case '"':
				b.WriteByte('"')
				i++
			case '\\':
				b.WriteByte('\\')
				i++
			case 'u':
				i++
				if i+4 > len(s) {
					return "", fmt.Errorf("incomplete \\u escape")
				}
				r, err := strconv.ParseUint(s[i:i+4], 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid \\u escape")
				}
				if r >= 0xD800 && r <= 0xDFFF {
					return "", fmt.Errorf("surrogate code point not allowed in unicode escape: U+%04X", r)
				}
				if r > 0x10FFFF {
					return "", fmt.Errorf("unicode code point out of range: U+%08X", r)
				}
				b.WriteRune(rune(r))
				i += 4
			case 'U':
				i++
				if i+8 > len(s) {
					return "", fmt.Errorf("incomplete \\U escape")
				}
				r, err := strconv.ParseUint(s[i:i+8], 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid \\U escape")
				}
				if r >= 0xD800 && r <= 0xDFFF {
					return "", fmt.Errorf("surrogate code point not allowed in unicode escape: U+%04X", r)
				}
				if r > 0x10FFFF {
					return "", fmt.Errorf("unicode code point out of range: U+%08X", r)
				}
				b.WriteRune(rune(r))
				i += 8
			default:
				return "", fmt.Errorf("invalid escape: \\%c", s[i])
			}
		} else {
			r, size := utf8.DecodeRuneInString(s[i:])
			b.WriteRune(r)
			i += size
		}
	}
	return b.String(), nil
}

func decodeMultiLineLiteralString(raw []byte) string {
	s := string(raw)
	if len(s) < 6 {
		return ""
	}
	inner := s[3:]
	// Count trailing quotes
	endQuotes := 0
	for i := len(inner) - 1; i >= 0 && inner[i] == '\''; i-- {
		endQuotes++
	}
	if endQuotes < 3 {
		return ""
	}
	inner = inner[:len(inner)-3]

	// Strip the first newline if present
	if len(inner) > 0 && inner[0] == '\n' {
		inner = inner[1:]
	} else if len(inner) > 1 && inner[0] == '\r' && inner[1] == '\n' {
		inner = inner[2:]
	}

	return inner
}

// --- number parsing ---

func parseInteger(raw []byte) (int64, IntegerBase, error) {
	s := string(raw)
	// strip underscores
	s = strings.ReplaceAll(s, "_", "")

	if len(s) == 0 {
		return 0, IntegerDecimal, fmt.Errorf("empty integer")
	}

	// Check for sign
	sign := ""
	rest := s
	if s[0] == '+' || s[0] == '-' {
		sign = s[:1]
		rest = s[1:]
	}

	// Check for prefix
	if len(rest) > 2 {
		prefix := rest[:2]
		switch prefix {
		case "0x", "0X":
			v, err := strconv.ParseInt(sign+rest[2:], 16, 64)
			return v, IntegerHex, err
		case "0o", "0O":
			v, err := strconv.ParseInt(sign+rest[2:], 8, 64)
			return v, IntegerOctal, err
		case "0b", "0B":
			v, err := strconv.ParseInt(sign+rest[2:], 2, 64)
			return v, IntegerBinary, err
		}
	}

	v, err := strconv.ParseInt(s, 10, 64)
	return v, IntegerDecimal, err
}

func parseFloat(raw []byte) (float64, error) {
	s := string(raw)
	s = strings.ReplaceAll(s, "_", "")

	// Special values
	switch s {
	case "inf", "+inf":
		return math.Inf(1), nil
	case "-inf":
		return math.Inf(-1), nil
	case "nan", "+nan":
		return math.NaN(), nil
	case "-nan":
		return math.NaN(), nil
	}

	return strconv.ParseFloat(s, 64)
}

// --- date/time parsing ---

func parseOffsetDateTime(s string) (time.Time, error) {
	// TOML allows 'T', 't', or ' ' as separator, and 'z'/'Z' for UTC
	normalized := strings.Replace(s, "t", "T", 1)
	normalized = strings.Replace(normalized, " ", "T", 1)
	// Normalize lowercase 'z' to 'Z' for Go's time.Parse
	if strings.HasSuffix(normalized, "z") {
		normalized = normalized[:len(normalized)-1] + "Z"
	}
	// Validate timezone offset range (Go's time.Parse silently accepts invalid offsets)
	if err := validateDateTimeOffset(normalized); err != nil {
		return time.Time{}, fmt.Errorf("invalid offset date-time %q: %v", s, err)
	}

	// Try various formats
	layouts := []string{
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.999999999Z",
	}

	var lastErr error
	for _, layout := range layouts {
		t, err := time.Parse(layout, normalized)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, fmt.Errorf("invalid offset date-time %q: %v", s, lastErr)
}

func parseLocalDateTime(s string) (LocalDateTime, error) {
	// normalize separator
	normalized := strings.Replace(s, "t", "T", 1)
	normalized = strings.Replace(normalized, " ", "T", 1)

	layouts := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.999999999",
	}

	for _, layout := range layouts {
		t, err := time.Parse(layout, normalized)
		if err == nil {
			return LocalDateTime{
				Year: t.Year(), Month: int(t.Month()), Day: t.Day(),
				Hour: t.Hour(), Minute: t.Minute(), Second: t.Second(),
				Nanosecond: t.Nanosecond(),
			}, nil
		}
	}
	return LocalDateTime{}, fmt.Errorf("invalid local date-time %q", s)
}

func parseLocalDate(s string) (LocalDate, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return LocalDate{}, fmt.Errorf("invalid local date %q: %v", s, err)
	}
	return LocalDate{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}, nil
}

func parseLocalTime(s string) (LocalTime, error) {
	layouts := []string{
		"15:04:05",
		"15:04:05.999999999",
	}
	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return LocalTime{
				Hour: t.Hour(), Minute: t.Minute(), Second: t.Second(),
				Nanosecond: t.Nanosecond(),
			}, nil
		}
	}
	return LocalTime{}, fmt.Errorf("invalid local time %q", s)
}

// validateDateTimeOffset checks that a timezone offset (e.g. +12:30) has valid
// hour and minute values. Go's time.Parse silently accepts invalid offsets
// like +12:60.
func validateDateTimeOffset(s string) error {
	// Find the offset part: look for +/- after the time portion
	// The format is YYYY-MM-DDTHH:MM:SS[.frac]+HH:MM or Z
	// Find last + or - that's part of the offset (not sign of year or exponent)
	idx := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '+' || s[i] == '-' {
			idx = i
			break
		}
		if s[i] == 'Z' {
			return nil // Z offset is always valid
		}
	}
	if idx == -1 {
		return nil // no offset found
	}
	// Verify it looks like an offset (has :)
	rest := s[idx+1:]
	if len(rest) != 5 || rest[2] != ':' {
		return nil // not a standard offset format
	}
	hours, err := strconv.Atoi(rest[0:2])
	if err != nil {
		return fmt.Errorf("invalid offset hours: %s", rest[0:2])
	}
	minutes, err := strconv.Atoi(rest[3:5])
	if err != nil {
		return fmt.Errorf("invalid offset minutes: %s", rest[3:5])
	}
	if hours > 23 {
		return fmt.Errorf("offset hours out of range: %d", hours)
	}
	if minutes > 59 {
		return fmt.Errorf("offset minutes out of range: %d", minutes)
	}
	return nil
}

// --- definition tracking (duplicate/conflict detection) ---

// tableEntry tracks what has been defined at a given key path.
type tableEntry struct {
	kind     entryKind
	node     Node
	children map[string]*tableEntry
}

type entryKind int

const (
	entryImplicit       entryKind = iota // implicitly created by sub-table header path
	entryDottedImplicit                  // implicitly created by dotted key (cannot be reopened)
	entryExplicit                        // explicitly defined via [table]
	entryValue                           // defined as a key = value
	entryArrayTable                      // defined via [[array-table]]
)

type definitionTracker struct {
	root *tableEntry
}

func newDefinitionTracker() *definitionTracker {
	return &definitionTracker{
		root: &tableEntry{kind: entryImplicit, children: make(map[string]*tableEntry)},
	}
}

// ensurePath creates implicit table entries along the path, returning the
// final entry. When fromDottedKey is true, new entries are marked as
// entryDottedImplicit (cannot be reopened by a [table] header).
// line and col are the source position of the key being defined, used for
// error reporting.
func (dt *definitionTracker) ensurePath(path []string, fromDottedKey bool, line, col int) (*tableEntry, error) {
	cur := dt.root
	for _, key := range path {
		if cur.children == nil {
			cur.children = make(map[string]*tableEntry)
		}
		child, ok := cur.children[key]
		if !ok {
			kind := entryImplicit
			if fromDottedKey {
				kind = entryDottedImplicit
			}
			child = &tableEntry{kind: kind, children: make(map[string]*tableEntry)}
			cur.children[key] = child
			cur = child
			continue
		}
		if child.kind == entryValue {
			return nil, &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("key %q is already defined as a value", key),
			}
		}
		// Dotted keys cannot traverse through explicitly defined tables
		// or array tables (you can't add to a table that was already
		// defined with a [table] or [[array-table]] header)
		if fromDottedKey && (child.kind == entryExplicit || child.kind == entryArrayTable) {
			return nil, &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("cannot extend %q via dotted key (already explicitly defined)", key),
			}
		}
		cur = child
	}
	return cur, nil
}

func (dt *definitionTracker) defineTable(path []string, node Node, line, col int) error {
	if len(path) == 0 {
		return nil
	}
	parent, err := dt.ensurePath(path[:len(path)-1], false, line, col)
	if err != nil {
		return err
	}
	key := path[len(path)-1]
	if parent.children == nil {
		parent.children = make(map[string]*tableEntry)
	}
	existing, ok := parent.children[key]
	if ok {
		if existing.kind == entryExplicit {
			return &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("table [%s] already defined", strings.Join(path, ".")),
			}
		}
		if existing.kind == entryValue {
			return &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("key %q is already defined as a value", key),
			}
		}
		if existing.kind == entryArrayTable {
			return &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("cannot define [%s] as a table, already defined as array table", strings.Join(path, ".")),
			}
		}
		if existing.kind == entryDottedImplicit {
			return &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("cannot define [%s] as a table, already implicitly defined via dotted key", strings.Join(path, ".")),
			}
		}
		// Was implicit (from sub-table path), promote to explicit
		existing.kind = entryExplicit
		existing.node = node
		return nil
	}
	parent.children[key] = &tableEntry{kind: entryExplicit, node: node, children: make(map[string]*tableEntry)}
	return nil
}

func (dt *definitionTracker) defineArrayTable(path []string, node Node, line, col int) error {
	if len(path) == 0 {
		return nil
	}
	parent, err := dt.ensurePath(path[:len(path)-1], false, line, col)
	if err != nil {
		return err
	}
	key := path[len(path)-1]
	if parent.children == nil {
		parent.children = make(map[string]*tableEntry)
	}
	existing, ok := parent.children[key]
	if ok {
		if existing.kind == entryExplicit {
			return &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("cannot define [[%s]] as array table, already defined as table", strings.Join(path, ".")),
			}
		}
		if existing.kind == entryValue {
			return &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("key %q is already defined as a value", key),
			}
		}
		if existing.kind == entryArrayTable {
			// Multiple [[array-table]] entries are fine -- reset children for the new element
			existing.node = node
			existing.children = make(map[string]*tableEntry)
			return nil
		}
		if existing.kind == entryDottedImplicit {
			return &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("cannot define [[%s]] as array table, already implicitly defined via dotted key", strings.Join(path, ".")),
			}
		}
		// Was implicit (from sub-table path). Can only promote if no
		// sub-tables were already defined under it (which would conflict
		// with array-table semantics that reset children per element).
		if len(existing.children) > 0 {
			return &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("cannot define [[%s]] as array table, already used as table with sub-entries", strings.Join(path, ".")),
			}
		}
		existing.kind = entryArrayTable
		existing.node = node
		existing.children = make(map[string]*tableEntry)
		return nil
	}
	parent.children[key] = &tableEntry{kind: entryArrayTable, node: node, children: make(map[string]*tableEntry)}
	return nil
}

func (dt *definitionTracker) defineKey(path []string, node Node, line, col int) error {
	if len(path) == 0 {
		return nil
	}
	// Navigate/create intermediate entries. Dotted keys create
	// entryDottedImplicit entries that cannot be reopened by [table] headers.
	parent, err := dt.ensurePath(path[:len(path)-1], true, line, col)
	if err != nil {
		return err
	}
	key := path[len(path)-1]
	if parent.children == nil {
		parent.children = make(map[string]*tableEntry)
	}
	existing, ok := parent.children[key]
	if ok {
		if existing.kind == entryValue {
			return &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("duplicate key %q", strings.Join(path, ".")),
			}
		}
		if existing.kind == entryExplicit || existing.kind == entryArrayTable {
			return &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("key %q conflicts with table definition", strings.Join(path, ".")),
			}
		}
		// Implicit: promote to value only if it has no children (sub-tables)
		if len(existing.children) > 0 {
			return &ParseError{
				Line:    line,
				Column:  col,
				Message: fmt.Sprintf("key %q is already defined as a table", strings.Join(path, ".")),
			}
		}
		existing.kind = entryValue
		existing.node = node
		return nil
	}
	parent.children[key] = &tableEntry{kind: entryValue, node: node}
	return nil
}

// tableScope returns a tracker that scopes key definitions to the given table path.
func (dt *definitionTracker) tableScope(path []string) *definitionTracker {
	cur := dt.root
	for _, key := range path {
		if cur.children == nil {
			cur.children = make(map[string]*tableEntry)
		}
		child, ok := cur.children[key]
		if !ok {
			child = &tableEntry{kind: entryImplicit, children: make(map[string]*tableEntry)}
			cur.children[key] = child
		}
		cur = child
	}
	return &definitionTracker{root: cur}
}

// arrayTableScope returns a tracker scoped to the latest element of the array table.
func (dt *definitionTracker) arrayTableScope(path []string) *definitionTracker {
	// For array tables, each occurrence gets fresh children, so the existing
	// entry's children map is already reset in defineArrayTable.
	return dt.tableScope(path)
}
