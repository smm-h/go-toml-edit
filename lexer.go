package tomledit

import (
	"fmt"
	"unicode/utf8"
)

// lexContext tracks whether the lexer expects a key or a value next.
type lexContext int

const (
	ctxKey   lexContext = iota // expecting a key (after newline, after [, after {, after , in table)
	ctxValue                   // expecting a value (after =)
)

// scopeKind identifies whether a nesting level is an array or inline table.
type scopeKind int

const (
	scopeArray       scopeKind = iota // inside [...]
	scopeInlineTable                  // inside {...}
)

// lexer tokenizes TOML source into a stream of tokens.
type lexer struct {
	src    []byte
	pos    int // current byte offset
	line   int // current line (1-based)
	col    int // current column (1-based)
	tokens []Token
	ctx    lexContext

	// bracket depth tracking to distinguish table headers from array values
	// and to know when , means "next key" vs "next value"
	arrayDepth       int  // nesting depth of [ in value context
	inlineTableDepth int  // nesting depth of { in any context
	afterNewline     bool // true at start or after a newline (for [[ detection)

	// scopeStack tracks the nesting of arrays and inline tables so that
	// commas can be attributed to the correct innermost scope.
	scopeStack []scopeKind
}

// lex tokenizes the given TOML source bytes into a slice of tokens.
func lex(src []byte) ([]Token, error) {
	l := &lexer{
		src:          src,
		line:         1,
		col:          1,
		ctx:          ctxKey,
		afterNewline: true,
	}
	if err := l.run(); err != nil {
		return nil, err
	}
	return l.tokens, nil
}

func (l *lexer) run() error {
	for l.pos < len(l.src) {
		if err := l.next(); err != nil {
			return err
		}
	}
	l.emitAt(TokenEOF, l.pos, l.pos, l.line, l.col)
	return nil
}

func (l *lexer) peekAt(offset int) byte {
	p := l.pos + offset
	if p < len(l.src) {
		return l.src[p]
	}
	return 0
}

func (l *lexer) emitAt(typ TokenType, start, end, line, col int) {
	l.tokens = append(l.tokens, Token{
		Type:   typ,
		Raw:    l.src[start:end],
		Line:   line,
		Column: col,
		Offset: start,
	})
}

func (l *lexer) errorf(msg string) error {
	snippet := l.extractSnippet()
	return &ParseError{
		Line:    l.line,
		Column:  l.col,
		Offset:  l.pos,
		Snippet: snippet,
		Message: msg,
	}
}

func (l *lexer) errorfAt(line, col, offset int, msg string) error {
	// extract snippet around the given offset
	start := offset
	end := offset
	for start > 0 && l.src[start-1] != '\n' {
		start--
	}
	for end < len(l.src) && l.src[end] != '\n' {
		end++
	}
	snippet := string(l.src[start:end])
	if len(snippet) > 60 {
		snippet = snippet[:60]
	}
	return &ParseError{
		Line:    line,
		Column:  col,
		Offset:  offset,
		Snippet: snippet,
		Message: msg,
	}
}

func (l *lexer) extractSnippet() string {
	start := l.pos
	end := l.pos
	for start > 0 && l.src[start-1] != '\n' {
		start--
	}
	for end < len(l.src) && l.src[end] != '\n' {
		end++
	}
	snippet := string(l.src[start:end])
	if len(snippet) > 60 {
		snippet = snippet[:60]
	}
	return snippet
}

func (l *lexer) advance(n int) {
	for i := 0; i < n && l.pos < len(l.src); i++ {
		if l.src[l.pos] == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		l.pos++
	}
}

func (l *lexer) next() error {
	ch := l.src[l.pos]

	// Newlines
	if ch == '\n' || (ch == '\r' && l.peekAt(1) == '\n') {
		return l.lexNewline()
	}

	// Whitespace (spaces and tabs, not newlines)
	if ch == ' ' || ch == '\t' {
		return l.lexWhitespace()
	}

	// Comments
	if ch == '#' {
		return l.lexComment()
	}

	// Equals
	if ch == '=' {
		l.emitAt(TokenEquals, l.pos, l.pos+1, l.line, l.col)
		l.advance(1)
		l.ctx = ctxValue
		l.afterNewline = false
		return nil
	}

	// Dot (key separator)
	if ch == '.' && l.ctx == ctxKey {
		l.emitAt(TokenDot, l.pos, l.pos+1, l.line, l.col)
		l.advance(1)
		l.afterNewline = false
		return nil
	}

	// Comma
	if ch == ',' {
		l.emitAt(TokenComma, l.pos, l.pos+1, l.line, l.col)
		l.advance(1)
		l.afterNewline = false
		// Determine context based on innermost scope
		if len(l.scopeStack) > 0 {
			switch l.scopeStack[len(l.scopeStack)-1] {
			case scopeArray:
				l.ctx = ctxValue
			case scopeInlineTable:
				l.ctx = ctxKey
			}
		}
		return nil
	}

	// Left bracket
	if ch == '[' {
		return l.lexLeftBracket()
	}

	// Right bracket
	if ch == ']' {
		return l.lexRightBracket()
	}

	// Left brace
	if ch == '{' {
		l.emitAt(TokenLeftBrace, l.pos, l.pos+1, l.line, l.col)
		l.advance(1)
		l.inlineTableDepth++
		l.scopeStack = append(l.scopeStack, scopeInlineTable)
		l.ctx = ctxKey
		l.afterNewline = false
		return nil
	}

	// Right brace
	if ch == '}' {
		l.emitAt(TokenRightBrace, l.pos, l.pos+1, l.line, l.col)
		l.advance(1)
		if l.inlineTableDepth > 0 {
			l.inlineTableDepth--
		}
		if len(l.scopeStack) > 0 {
			l.scopeStack = l.scopeStack[:len(l.scopeStack)-1]
		}
		l.afterNewline = false
		// After }, context depends on what we're nested in
		return nil
	}

	// Strings
	if ch == '"' {
		return l.lexBasicStringOrMultiLine()
	}
	if ch == '\'' {
		return l.lexLiteralStringOrMultiLine()
	}

	// Value context: numbers, booleans, dates/times, special floats
	if l.ctx == ctxValue {
		return l.lexValue()
	}

	// Key context: bare keys
	if isBareKeyChar(ch) {
		return l.lexBareKey()
	}

	// Check for invalid UTF-8
	if ch >= 0x80 {
		_, size := utf8.DecodeRune(l.src[l.pos:])
		if size == 1 {
			// invalid UTF-8 (RuneError with size 1)
			return l.errorf("invalid UTF-8 byte")
		}
		// valid multi-byte UTF-8 but not a recognized token start
		return l.errorf("unexpected character")
	}

	return l.errorf("unexpected character")
}

func (l *lexer) lexNewline() error {
	start := l.pos
	line := l.line
	col := l.col
	if l.src[l.pos] == '\r' && l.peekAt(1) == '\n' {
		l.advance(2)
	} else {
		l.advance(1)
	}
	l.emitAt(TokenNewline, start, l.pos, line, col)
	l.afterNewline = true
	// After a newline at root level (not in array/inline table), expect key
	if l.arrayDepth == 0 && l.inlineTableDepth == 0 {
		l.ctx = ctxKey
	}
	return nil
}

func (l *lexer) lexWhitespace() error {
	start := l.pos
	line := l.line
	col := l.col
	for l.pos < len(l.src) && (l.src[l.pos] == ' ' || l.src[l.pos] == '\t') {
		l.advance(1)
	}
	l.emitAt(TokenWhitespace, start, l.pos, line, col)
	return nil
}

func (l *lexer) lexComment() error {
	start := l.pos
	line := l.line
	col := l.col
	// consume # and everything until newline (but not the newline itself)
	for l.pos < len(l.src) && l.src[l.pos] != '\n' && !(l.src[l.pos] == '\r' && l.peekAt(1) == '\n') {
		ch := l.src[l.pos]
		// Reject control characters in comments (tab is allowed)
		if ch < 0x80 && isControlChar(ch) {
			return l.errorf(fmt.Sprintf("control character U+%04X not allowed in comment", ch))
		}
		// Check for invalid UTF-8 within the comment
		if ch >= 0x80 {
			_, size := utf8.DecodeRune(l.src[l.pos:])
			if size == 1 {
				return l.errorf("invalid UTF-8 byte in comment")
			}
			l.advance(size)
		} else {
			l.advance(1)
		}
	}
	l.emitAt(TokenComment, start, l.pos, line, col)
	return nil
}

func (l *lexer) lexLeftBracket() error {
	if l.ctx == ctxValue {
		// In value context, [ starts an array. Each [ is separate.
		l.emitAt(TokenLeftBracket, l.pos, l.pos+1, l.line, l.col)
		l.advance(1)
		l.arrayDepth++
		l.scopeStack = append(l.scopeStack, scopeArray)
		l.afterNewline = false
		// stay in value context for array elements
		return nil
	}

	// In key context: could be [ for table or [[ for array table
	// [[ only at start of line (after optional whitespace after newline)
	if l.afterNewline && l.peekAt(1) == '[' {
		l.emitAt(TokenDoubleLeftBracket, l.pos, l.pos+2, l.line, l.col)
		l.advance(2)
		l.afterNewline = false
		return nil
	}

	l.emitAt(TokenLeftBracket, l.pos, l.pos+1, l.line, l.col)
	l.advance(1)
	l.afterNewline = false
	return nil
}

func (l *lexer) lexRightBracket() error {
	if l.arrayDepth > 0 {
		// In an array value context, ] closes the array
		l.emitAt(TokenRightBracket, l.pos, l.pos+1, l.line, l.col)
		l.advance(1)
		l.arrayDepth--
		if len(l.scopeStack) > 0 {
			l.scopeStack = l.scopeStack[:len(l.scopeStack)-1]
		}
		l.afterNewline = false
		return nil
	}

	// In key context (table header): could be ]] for array table
	if l.peekAt(1) == ']' {
		l.emitAt(TokenDoubleRightBracket, l.pos, l.pos+2, l.line, l.col)
		l.advance(2)
		l.afterNewline = false
		l.ctx = ctxKey // after table header, expect key on next line
		return nil
	}

	l.emitAt(TokenRightBracket, l.pos, l.pos+1, l.line, l.col)
	l.advance(1)
	l.afterNewline = false
	l.ctx = ctxKey // after table header close, expect key
	return nil
}

func (l *lexer) lexBasicStringOrMultiLine() error {
	// Check for multi-line: """
	if l.peekAt(1) == '"' && l.peekAt(2) == '"' {
		return l.lexMultiLineBasicString()
	}
	return l.lexBasicString()
}

func (l *lexer) lexBasicString() error {
	start := l.pos
	line := l.line
	col := l.col
	l.advance(1) // skip opening "

	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '"' {
			l.advance(1) // skip closing "
			l.emitAt(TokenBasicString, start, l.pos, line, col)
			l.afterNewline = false
			return nil
		}
		if ch == '\\' {
			if err := l.consumeEscape(); err != nil {
				return err
			}
			continue
		}
		if ch == '\n' || (ch == '\r' && l.peekAt(1) == '\n') {
			return l.errorfAt(line, col, start, "unterminated basic string")
		}
		// Reject control characters (U+0000-U+0008, U+000A-U+001F, U+007F)
		if ch < 0x80 && isStringControlChar(ch) {
			return l.errorf(fmt.Sprintf("control character U+%04X not allowed in basic string", ch))
		}
		// Check for invalid UTF-8
		if ch >= 0x80 {
			_, size := utf8.DecodeRune(l.src[l.pos:])
			if size == 1 {
				return l.errorf("invalid UTF-8 byte in string")
			}
			l.advance(size)
		} else {
			l.advance(1)
		}
	}
	return l.errorfAt(line, col, start, "unterminated basic string")
}

func (l *lexer) consumeEscape() error {
	l.advance(1) // skip backslash
	if l.pos >= len(l.src) {
		return l.errorf("unterminated escape sequence")
	}
	ch := l.src[l.pos]
	switch ch {
	case 'b', 't', 'n', 'f', 'r', '"', '\\':
		l.advance(1)
		return nil
	case 'u':
		l.advance(1)
		return l.consumeUnicodeEscape(4)
	case 'U':
		l.advance(1)
		return l.consumeUnicodeEscape(8)
	default:
		return l.errorf("invalid escape sequence: \\" + string(ch))
	}
}

func (l *lexer) consumeUnicodeEscape(n int) error {
	for i := 0; i < n; i++ {
		if l.pos >= len(l.src) {
			return l.errorf("incomplete unicode escape sequence")
		}
		if !isHexDigit(l.src[l.pos]) {
			return l.errorf("invalid unicode escape: non-hex digit")
		}
		l.advance(1)
	}
	return nil
}

func (l *lexer) lexMultiLineBasicString() error {
	start := l.pos
	line := l.line
	col := l.col
	l.advance(3) // skip opening """

	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '"' && l.peekAt(1) == '"' && l.peekAt(2) == '"' {
			// Could be """ followed by more quotes (up to 2 extra allowed)
			l.advance(3) // skip closing """
			// Allow up to 2 trailing quotes as part of the string content
			for i := 0; i < 2 && l.pos < len(l.src) && l.src[l.pos] == '"'; i++ {
				l.advance(1)
			}
			l.emitAt(TokenMultiLineBasicString, start, l.pos, line, col)
			l.afterNewline = false
			return nil
		}
		if ch == '\\' {
			// In multi-line basic strings, \ at end of line trims whitespace.
			// Per TOML spec, \ may be followed by optional spaces/tabs before
			// the newline (all of which are trimmed).
			l.advance(1) // skip backslash
			if l.pos < len(l.src) {
				// Save position to restore if this is not a line-ending backslash
				savedPos := l.pos
				savedCol := l.col
				// Skip optional whitespace between \ and newline
				for l.pos < len(l.src) && (l.src[l.pos] == ' ' || l.src[l.pos] == '\t') {
					l.advance(1)
				}
				if l.pos < len(l.src) {
					next := l.src[l.pos]
					if next == '\n' || (next == '\r' && l.peekAt(1) == '\n') {
						// line-ending backslash: skip newline and following whitespace
						if next == '\r' {
							l.advance(2)
						} else {
							l.advance(1)
						}
						for l.pos < len(l.src) && (l.src[l.pos] == ' ' || l.src[l.pos] == '\t' || l.src[l.pos] == '\n' || (l.src[l.pos] == '\r' && l.peekAt(1) == '\n')) {
							if l.src[l.pos] == '\r' {
								l.advance(2)
							} else {
								l.advance(1)
							}
						}
						continue
					}
				}
				// Not a line-ending backslash: restore position and handle
				// as a regular escape sequence. We only skipped spaces/tabs
				// (no newlines), so line is unchanged.
				l.pos = savedPos
				l.col = savedCol
				l.pos--
				l.col--
				if err := l.consumeEscape(); err != nil {
					return err
				}
			}
			continue
		}
		// Reject control characters (but allow tab and newlines in multi-line)
		if ch < 0x80 && isMultiLineStringControlChar(ch) {
			return l.errorf(fmt.Sprintf("control character U+%04X not allowed in multi-line basic string", ch))
		}
		if ch >= 0x80 {
			_, size := utf8.DecodeRune(l.src[l.pos:])
			if size == 1 {
				return l.errorf("invalid UTF-8 byte in multi-line basic string")
			}
			l.advance(size)
		} else {
			l.advance(1)
		}
	}
	return l.errorfAt(line, col, start, "unterminated multi-line basic string")
}

func (l *lexer) lexLiteralStringOrMultiLine() error {
	if l.peekAt(1) == '\'' && l.peekAt(2) == '\'' {
		return l.lexMultiLineLiteralString()
	}
	return l.lexLiteralString()
}

func (l *lexer) lexLiteralString() error {
	start := l.pos
	line := l.line
	col := l.col
	l.advance(1) // skip opening '

	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '\'' {
			l.advance(1) // skip closing '
			l.emitAt(TokenLiteralString, start, l.pos, line, col)
			l.afterNewline = false
			return nil
		}
		if ch == '\n' || (ch == '\r' && l.peekAt(1) == '\n') {
			return l.errorfAt(line, col, start, "unterminated literal string")
		}
		// Reject control characters
		if ch < 0x80 && isStringControlChar(ch) {
			return l.errorf(fmt.Sprintf("control character U+%04X not allowed in literal string", ch))
		}
		if ch >= 0x80 {
			_, size := utf8.DecodeRune(l.src[l.pos:])
			if size == 1 {
				return l.errorf("invalid UTF-8 byte in literal string")
			}
			l.advance(size)
		} else {
			l.advance(1)
		}
	}
	return l.errorfAt(line, col, start, "unterminated literal string")
}

func (l *lexer) lexMultiLineLiteralString() error {
	start := l.pos
	line := l.line
	col := l.col
	l.advance(3) // skip opening '''

	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '\'' && l.peekAt(1) == '\'' && l.peekAt(2) == '\'' {
			l.advance(3) // skip closing '''
			// Allow up to 2 trailing quotes as part of the string content
			for i := 0; i < 2 && l.pos < len(l.src) && l.src[l.pos] == '\''; i++ {
				l.advance(1)
			}
			l.emitAt(TokenMultiLineLiteralString, start, l.pos, line, col)
			l.afterNewline = false
			return nil
		}
		// Reject control characters (but allow tab and newlines in multi-line)
		if ch < 0x80 && isMultiLineStringControlChar(ch) {
			return l.errorf(fmt.Sprintf("control character U+%04X not allowed in multi-line literal string", ch))
		}
		if ch >= 0x80 {
			_, size := utf8.DecodeRune(l.src[l.pos:])
			if size == 1 {
				return l.errorf("invalid UTF-8 byte in multi-line literal string")
			}
			l.advance(size)
		} else {
			l.advance(1)
		}
	}
	return l.errorfAt(line, col, start, "unterminated multi-line literal string")
}

func (l *lexer) lexBareKey() error {
	start := l.pos
	line := l.line
	col := l.col
	for l.pos < len(l.src) && isBareKeyChar(l.src[l.pos]) {
		l.advance(1)
	}
	l.emitAt(TokenBareKey, start, l.pos, line, col)
	l.afterNewline = false
	return nil
}

func (l *lexer) lexValue() error {
	ch := l.src[l.pos]

	// Special float values: inf, nan (and +inf, -inf, +nan, -nan)
	if ch == 'i' || ch == 'n' {
		return l.lexSpecialFloat()
	}
	if (ch == '+' || ch == '-') && l.pos+1 < len(l.src) {
		next := l.src[l.pos+1]
		if next == 'i' || next == 'n' {
			return l.lexSpecialFloat()
		}
	}

	// Booleans
	if ch == 't' && l.matchWord("true") {
		start := l.pos
		line := l.line
		col := l.col
		l.advance(4)
		l.emitAt(TokenBoolean, start, l.pos, line, col)
		l.afterNewline = false
		return nil
	}
	if ch == 'f' && l.matchWord("false") {
		start := l.pos
		line := l.line
		col := l.col
		l.advance(5)
		l.emitAt(TokenBoolean, start, l.pos, line, col)
		l.afterNewline = false
		return nil
	}

	// Numbers or dates
	if ch == '+' || ch == '-' || isDigit(ch) {
		return l.lexNumberOrDate()
	}

	// Bare key characters that didn't match true/false/inf/nan in value context:
	// These are still bare-key-shaped tokens that the parser will sort out.
	// Actually, in value context we shouldn't see bare keys except for the
	// special values above. If we get here with a letter, it's an error.
	if isBareKeyChar(ch) {
		return l.errorf("unexpected character in value context")
	}

	// Dot in value context (e.g., not handled at top level for non-key context)
	if ch == '.' {
		return l.errorf("unexpected '.' in value context")
	}

	// Check for invalid UTF-8
	if ch >= 0x80 {
		_, size := utf8.DecodeRune(l.src[l.pos:])
		if size == 1 {
			return l.errorf("invalid UTF-8 byte")
		}
		return l.errorf("unexpected character")
	}

	return l.errorf("unexpected character")
}

func (l *lexer) matchWord(word string) bool {
	if l.pos+len(word) > len(l.src) {
		return false
	}
	for i := 0; i < len(word); i++ {
		if l.src[l.pos+i] != word[i] {
			return false
		}
	}
	// Make sure the word is not followed by a bare-key character
	end := l.pos + len(word)
	if end < len(l.src) && isBareKeyChar(l.src[end]) {
		return false
	}
	return true
}

func (l *lexer) lexSpecialFloat() error {
	start := l.pos
	line := l.line
	col := l.col

	// optional sign
	if l.src[l.pos] == '+' || l.src[l.pos] == '-' {
		l.advance(1)
	}
	if l.pos >= len(l.src) {
		return l.errorf("unexpected end of input")
	}

	if l.src[l.pos] == 'i' && l.matchWordAt(l.pos, "inf") {
		l.advance(3)
		l.emitAt(TokenFloat, start, l.pos, line, col)
		l.afterNewline = false
		return nil
	}
	if l.src[l.pos] == 'n' && l.matchWordAt(l.pos, "nan") {
		l.advance(3)
		l.emitAt(TokenFloat, start, l.pos, line, col)
		l.afterNewline = false
		return nil
	}

	return l.errorf("unexpected character")
}

func (l *lexer) matchWordAt(pos int, word string) bool {
	if pos+len(word) > len(l.src) {
		return false
	}
	for i := 0; i < len(word); i++ {
		if l.src[pos+i] != word[i] {
			return false
		}
	}
	end := pos + len(word)
	if end < len(l.src) && isBareKeyChar(l.src[end]) {
		return false
	}
	return true
}

func (l *lexer) lexNumberOrDate() error {
	start := l.pos
	line := l.line
	col := l.col
	ch := l.src[l.pos]

	// Handle sign prefix
	hasSign := false
	if ch == '+' || ch == '-' {
		hasSign = true
		l.advance(1)
		if l.pos >= len(l.src) || !isDigit(l.src[l.pos]) {
			return l.errorfAt(line, col, start, "invalid number: sign not followed by digit")
		}
		ch = l.src[l.pos]
	}

	// Check for 0x, 0o, 0b prefixes. TOML only allows lowercase prefixes
	// and no sign prefix.
	if ch == '0' && l.pos+1 < len(l.src) {
		next := l.src[l.pos+1]
		if next == 'X' || next == 'O' || next == 'B' {
			return l.errorfAt(line, col, start, fmt.Sprintf("invalid number: uppercase prefix 0%c not allowed (use lowercase)", next))
		}
		if next == 'x' || next == 'o' || next == 'b' {
			if hasSign {
				return l.errorfAt(line, col, start, fmt.Sprintf("invalid number: sign not allowed with 0%c prefix", next))
			}
			switch next {
			case 'x':
				return l.lexHexInteger(start, line, col)
			case 'o':
				return l.lexOctInteger(start, line, col)
			case 'b':
				return l.lexBinInteger(start, line, col)
			}
		}
	}

	// Check for date: 4 digits followed by '-'
	if !hasSign && isDigit(ch) {
		digitCount := l.countDigitsFrom(l.pos)
		if digitCount == 4 && l.pos+4 < len(l.src) && l.src[l.pos+4] == '-' {
			return l.lexDateOrDateTime(start, line, col)
		}
		// Check for local time: 2 digits followed by ':'
		if digitCount == 2 && l.pos+2 < len(l.src) && l.src[l.pos+2] == ':' {
			return l.lexLocalTime(start, line, col)
		}
	}

	// Decimal number (integer or float)
	return l.lexDecimalNumber(start, line, col, hasSign)
}

func (l *lexer) countDigitsFrom(pos int) int {
	count := 0
	for pos+count < len(l.src) && isDigit(l.src[pos+count]) {
		count++
	}
	return count
}

func (l *lexer) lexHexInteger(start, line, col int) error {
	l.advance(2) // skip 0x
	if l.pos >= len(l.src) || !isHexDigit(l.src[l.pos]) {
		return l.errorfAt(line, col, start, "invalid hex integer: no digits after prefix")
	}
	l.advance(1)
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if isHexDigit(ch) {
			l.advance(1)
		} else if ch == '_' {
			// underscore must be between digits
			if l.pos+1 >= len(l.src) || !isHexDigit(l.src[l.pos+1]) {
				return l.errorfAt(line, col, start, "invalid hex integer: trailing or consecutive underscore")
			}
			l.advance(1)
		} else {
			break
		}
	}
	l.emitAt(TokenInteger, start, l.pos, line, col)
	l.afterNewline = false
	return nil
}

func (l *lexer) lexOctInteger(start, line, col int) error {
	l.advance(2) // skip 0o
	if l.pos >= len(l.src) || !isOctDigit(l.src[l.pos]) {
		return l.errorfAt(line, col, start, "invalid octal integer: no digits after prefix")
	}
	l.advance(1)
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if isOctDigit(ch) {
			l.advance(1)
		} else if ch == '_' {
			if l.pos+1 >= len(l.src) || !isOctDigit(l.src[l.pos+1]) {
				return l.errorfAt(line, col, start, "invalid octal integer: trailing or consecutive underscore")
			}
			l.advance(1)
		} else {
			break
		}
	}
	l.emitAt(TokenInteger, start, l.pos, line, col)
	l.afterNewline = false
	return nil
}

func (l *lexer) lexBinInteger(start, line, col int) error {
	l.advance(2) // skip 0b
	if l.pos >= len(l.src) || !isBinDigit(l.src[l.pos]) {
		return l.errorfAt(line, col, start, "invalid binary integer: no digits after prefix")
	}
	l.advance(1)
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if isBinDigit(ch) {
			l.advance(1)
		} else if ch == '_' {
			if l.pos+1 >= len(l.src) || !isBinDigit(l.src[l.pos+1]) {
				return l.errorfAt(line, col, start, "invalid binary integer: trailing or consecutive underscore")
			}
			l.advance(1)
		} else {
			break
		}
	}
	l.emitAt(TokenInteger, start, l.pos, line, col)
	l.afterNewline = false
	return nil
}

func (l *lexer) lexDecimalNumber(start, line, col int, hasSign bool) error {
	// We're positioned at the first digit
	firstDigit := l.src[l.pos]

	// Consume integer part
	l.advance(1)

	// Leading zeros check: if first digit is 0, the next char must not be a
	// digit or underscore (0 by itself is fine, 0.5 is fine, 0e1 is fine,
	// but 00, 01, 0_0, 0_1 are all invalid)
	if firstDigit == '0' && l.pos < len(l.src) && (isDigit(l.src[l.pos]) || l.src[l.pos] == '_') {
		return l.errorfAt(line, col, start, "invalid number: leading zeros not allowed")
	}

	// Consume remaining integer digits (with underscores)
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if isDigit(ch) {
			l.advance(1)
		} else if ch == '_' {
			if l.pos+1 >= len(l.src) || !isDigit(l.src[l.pos+1]) {
				return l.errorfAt(line, col, start, "invalid number: trailing or consecutive underscore")
			}
			l.advance(1)
		} else {
			break
		}
	}

	isFloat := false

	// Check for fractional part
	if l.pos < len(l.src) && l.src[l.pos] == '.' {
		// Make sure it's followed by a digit (not just a bare dot)
		if l.pos+1 < len(l.src) && isDigit(l.src[l.pos+1]) {
			isFloat = true
			l.advance(1) // skip '.'
			l.advance(1) // skip first fractional digit
			for l.pos < len(l.src) {
				ch := l.src[l.pos]
				if isDigit(ch) {
					l.advance(1)
				} else if ch == '_' {
					if l.pos+1 >= len(l.src) || !isDigit(l.src[l.pos+1]) {
						return l.errorfAt(line, col, start, "invalid number: trailing or consecutive underscore in fractional part")
					}
					l.advance(1)
				} else {
					break
				}
			}
		}
	}

	// Check for exponent
	if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		isFloat = true
		l.advance(1) // skip e/E
		if l.pos < len(l.src) && (l.src[l.pos] == '+' || l.src[l.pos] == '-') {
			l.advance(1) // skip sign
		}
		if l.pos >= len(l.src) || !isDigit(l.src[l.pos]) {
			return l.errorfAt(line, col, start, "invalid number: exponent has no digits")
		}
		l.advance(1)
		for l.pos < len(l.src) {
			ch := l.src[l.pos]
			if isDigit(ch) {
				l.advance(1)
			} else if ch == '_' {
				if l.pos+1 >= len(l.src) || !isDigit(l.src[l.pos+1]) {
					return l.errorfAt(line, col, start, "invalid number: trailing or consecutive underscore in exponent")
				}
				l.advance(1)
			} else {
				break
			}
		}
	}

	if isFloat {
		l.emitAt(TokenFloat, start, l.pos, line, col)
	} else {
		l.emitAt(TokenInteger, start, l.pos, line, col)
	}
	l.afterNewline = false
	return nil
}

func (l *lexer) lexDateOrDateTime(start, line, col int) error {
	// We're positioned at the first digit of YYYY
	// Consume YYYY-MM-DD
	l.advance(4) // YYYY
	l.advance(1) // -
	if !l.consumeNDigits(2) {
		return l.errorfAt(line, col, start, "invalid date: expected 2-digit month")
	}
	if l.pos >= len(l.src) || l.src[l.pos] != '-' {
		return l.errorfAt(line, col, start, "invalid date: expected '-' after month")
	}
	l.advance(1) // -
	if !l.consumeNDigits(2) {
		return l.errorfAt(line, col, start, "invalid date: expected 2-digit day")
	}

	// Check if this is just a date, or if there's a time part
	if l.pos < len(l.src) && (l.src[l.pos] == 'T' || l.src[l.pos] == 't' || l.src[l.pos] == ' ') {
		// Peek ahead: if it's a space, need to check the next char is a digit
		// to distinguish "date = 1979-05-27\n" from "date = 1979-05-27T07:32:00"
		sep := l.src[l.pos]
		if sep == ' ' {
			// Space separator: must be followed by digit to be a datetime
			if l.pos+1 < len(l.src) && isDigit(l.src[l.pos+1]) {
				l.advance(1) // skip space
				return l.lexTimePart(start, line, col, true)
			}
			// Just a local date
			l.emitAt(TokenLocalDate, start, l.pos, line, col)
			l.afterNewline = false
			return nil
		}
		// T or t separator
		l.advance(1) // skip T
		return l.lexTimePart(start, line, col, true)
	}

	// Just a local date
	l.emitAt(TokenLocalDate, start, l.pos, line, col)
	l.afterNewline = false
	return nil
}

func (l *lexer) lexTimePart(start, line, col int, canHaveOffset bool) error {
	// Consume HH:MM:SS
	if !l.consumeNDigits(2) {
		return l.errorfAt(line, col, start, "invalid time: expected 2-digit hour")
	}
	if l.pos >= len(l.src) || l.src[l.pos] != ':' {
		return l.errorfAt(line, col, start, "invalid time: expected ':' after hour")
	}
	l.advance(1) // :
	if !l.consumeNDigits(2) {
		return l.errorfAt(line, col, start, "invalid time: expected 2-digit minute")
	}
	if l.pos >= len(l.src) || l.src[l.pos] != ':' {
		return l.errorfAt(line, col, start, "invalid time: expected ':' after minute")
	}
	l.advance(1) // :
	if !l.consumeNDigits(2) {
		return l.errorfAt(line, col, start, "invalid time: expected 2-digit second")
	}

	// Optional fractional seconds
	if l.pos < len(l.src) && l.src[l.pos] == '.' {
		l.advance(1) // skip .
		if l.pos >= len(l.src) || !isDigit(l.src[l.pos]) {
			return l.errorfAt(line, col, start, "invalid time: expected digits after decimal point")
		}
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			l.advance(1)
		}
	}

	if !canHaveOffset {
		l.emitAt(TokenLocalTime, start, l.pos, line, col)
		l.afterNewline = false
		return nil
	}

	// Check for offset
	if l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == 'Z' || ch == 'z' {
			l.advance(1)
			l.emitAt(TokenOffsetDateTime, start, l.pos, line, col)
			l.afterNewline = false
			return nil
		}
		if ch == '+' || ch == '-' {
			l.advance(1) // skip +/-
			if !l.consumeNDigits(2) {
				return l.errorfAt(line, col, start, "invalid time offset: expected 2-digit hour")
			}
			if l.pos >= len(l.src) || l.src[l.pos] != ':' {
				return l.errorfAt(line, col, start, "invalid time offset: expected ':' after hour")
			}
			l.advance(1) // :
			if !l.consumeNDigits(2) {
				return l.errorfAt(line, col, start, "invalid time offset: expected 2-digit minute")
			}
			l.emitAt(TokenOffsetDateTime, start, l.pos, line, col)
			l.afterNewline = false
			return nil
		}
	}

	// No offset: local date-time
	l.emitAt(TokenLocalDateTime, start, l.pos, line, col)
	l.afterNewline = false
	return nil
}

func (l *lexer) lexLocalTime(start, line, col int) error {
	return l.lexTimePart(start, line, col, false)
}

func (l *lexer) consumeNDigits(n int) bool {
	for i := 0; i < n; i++ {
		if l.pos >= len(l.src) || !isDigit(l.src[l.pos]) {
			return false
		}
		l.advance(1)
	}
	return true
}

// Character classification helpers

// isControlChar returns true for control characters that are forbidden in
// TOML strings and comments: U+0000-U+0008, U+000A-U+001F, U+007F.
// Tab (U+0009) is allowed.
func isControlChar(ch byte) bool {
	return (ch <= 0x08) || (ch >= 0x0A && ch <= 0x1F) || ch == 0x7F
}

// isStringControlChar returns true for control characters forbidden in
// single-line strings: U+0000-U+0008, U+000A-U+001F, U+007F.
// Tab (U+0009) is allowed in strings.
func isStringControlChar(ch byte) bool {
	return isControlChar(ch)
}

// isMultiLineStringControlChar returns true for control characters forbidden
// in multi-line strings. Newlines (U+000A, U+000D) are allowed.
func isMultiLineStringControlChar(ch byte) bool {
	return (ch <= 0x08) || (ch >= 0x0B && ch <= 0x0C) || (ch >= 0x0E && ch <= 0x1F) || ch == 0x7F
}

func isBareKeyChar(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func isOctDigit(ch byte) bool {
	return ch >= '0' && ch <= '7'
}

func isBinDigit(ch byte) bool {
	return ch == '0' || ch == '1'
}
