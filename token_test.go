package tomledit

import "testing"

func TestTokenTypes(t *testing.T) {
	tests := []struct {
		name    string
		typ     TokenType
		raw     string
		line    int
		col     int
		wantStr string
	}{
		{"BareKey", TokenBareKey, "name", 1, 1, "BareKey"},
		{"BasicString", TokenBasicString, `"hello"`, 1, 5, "BasicString"},
		{"LiteralString", TokenLiteralString, `'hello'`, 2, 1, "LiteralString"},
		{"MultiLineBasicString", TokenMultiLineBasicString, `"""hello"""`, 3, 1, "MultiLineBasicString"},
		{"MultiLineLiteralString", TokenMultiLineLiteralString, `'''hello'''`, 4, 1, "MultiLineLiteralString"},
		{"Integer", TokenInteger, "42", 1, 8, "Integer"},
		{"Float", TokenFloat, "3.14", 1, 8, "Float"},
		{"Boolean", TokenBoolean, "true", 1, 8, "Boolean"},
		{"OffsetDateTime", TokenOffsetDateTime, "1979-05-27T07:32:00Z", 1, 8, "OffsetDateTime"},
		{"LocalDateTime", TokenLocalDateTime, "1979-05-27T07:32:00", 1, 8, "LocalDateTime"},
		{"LocalDate", TokenLocalDate, "1979-05-27", 1, 8, "LocalDate"},
		{"LocalTime", TokenLocalTime, "07:32:00", 1, 8, "LocalTime"},
		{"Equals", TokenEquals, "=", 1, 5, "Equals"},
		{"Dot", TokenDot, ".", 1, 6, "Dot"},
		{"Comma", TokenComma, ",", 1, 10, "Comma"},
		{"LeftBracket", TokenLeftBracket, "[", 1, 1, "LeftBracket"},
		{"RightBracket", TokenRightBracket, "]", 1, 8, "RightBracket"},
		{"DoubleLeftBracket", TokenDoubleLeftBracket, "[[", 1, 1, "DoubleLeftBracket"},
		{"DoubleRightBracket", TokenDoubleRightBracket, "]]", 1, 8, "DoubleRightBracket"},
		{"LeftBrace", TokenLeftBrace, "{", 1, 8, "LeftBrace"},
		{"RightBrace", TokenRightBrace, "}", 1, 15, "RightBrace"},
		{"Comment", TokenComment, "# a comment", 1, 1, "Comment"},
		{"Whitespace", TokenWhitespace, "  ", 1, 1, "Whitespace"},
		{"Newline", TokenNewline, "\n", 1, 12, "Newline"},
		{"EOF", TokenEOF, "", 5, 1, "EOF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := Token{
				Type:   tt.typ,
				Raw:    []byte(tt.raw),
				Line:   tt.line,
				Column: tt.col,
			}

			if tok.Type != tt.typ {
				t.Errorf("Type = %v, want %v", tok.Type, tt.typ)
			}
			if string(tok.Raw) != tt.raw {
				t.Errorf("Raw = %q, want %q", tok.Raw, tt.raw)
			}
			if tok.Line != tt.line {
				t.Errorf("Line = %d, want %d", tok.Line, tt.line)
			}
			if tok.Column != tt.col {
				t.Errorf("Column = %d, want %d", tok.Column, tt.col)
			}
			if got := tt.typ.String(); got != tt.wantStr {
				t.Errorf("String() = %q, want %q", got, tt.wantStr)
			}
		})
	}
}

// Fails if the lexer stops recording each token's byte offset, or records one
// that does not point at the token's own bytes -- the offsets diagnostics and
// spans are built from.
func TestTokenOffsets(t *testing.T) {
	src := []byte("# c\r\ntitle = \"hello\"\n[a.b]\nx = [1, 2]\ny = { z = '''q''' }\n")
	tokens, err := lex(src)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}
	prevEnd := 0
	for i, tok := range tokens {
		end := tok.Offset + len(tok.Raw)
		if tok.Offset < 0 || end > len(src) {
			t.Fatalf("token %d (%s): offset range %d..%d outside the %d-byte source", i, tok.Type, tok.Offset, end, len(src))
		}
		if got := src[tok.Offset:end]; string(got) != string(tok.Raw) {
			t.Errorf("token %d (%s): src[%d:%d] = %q, want %q", i, tok.Type, tok.Offset, end, got, tok.Raw)
		}
		if tok.Offset != prevEnd {
			t.Errorf("token %d (%s): offset = %d, want %d (tokens must tile the source without gaps)", i, tok.Type, tok.Offset, prevEnd)
		}
		prevEnd = end
	}
	if prevEnd != len(src) {
		t.Errorf("tokens cover %d bytes, want %d", prevEnd, len(src))
	}
	last := tokens[len(tokens)-1]
	if last.Type != TokenEOF || last.Offset != len(src) {
		t.Errorf("last token = %s at offset %d, want EOF at %d", last.Type, last.Offset, len(src))
	}
}

func TestTokenTypeStringUnknown(t *testing.T) {
	unknown := TokenType(999)
	if got := unknown.String(); got != "Unknown" {
		t.Errorf("String() = %q, want %q", got, "Unknown")
	}
}
