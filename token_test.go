package tomledit

import "testing"

func TestTokenTypes(t *testing.T) {
	tests := []struct {
		name    string
		typ     tokenType
		raw     string
		line    int
		col     int
		wantStr string
	}{
		{"BareKey", tokenBareKey, "name", 1, 1, "BareKey"},
		{"BasicString", tokenBasicString, `"hello"`, 1, 5, "BasicString"},
		{"LiteralString", tokenLiteralString, `'hello'`, 2, 1, "LiteralString"},
		{"MultiLineBasicString", tokenMultiLineBasicString, `"""hello"""`, 3, 1, "MultiLineBasicString"},
		{"MultiLineLiteralString", tokenMultiLineLiteralString, `'''hello'''`, 4, 1, "MultiLineLiteralString"},
		{"Integer", tokenInteger, "42", 1, 8, "Integer"},
		{"Float", tokenFloat, "3.14", 1, 8, "Float"},
		{"Boolean", tokenBoolean, "true", 1, 8, "Boolean"},
		{"OffsetDateTime", tokenOffsetDateTime, "1979-05-27T07:32:00Z", 1, 8, "OffsetDateTime"},
		{"LocalDateTime", tokenLocalDateTime, "1979-05-27T07:32:00", 1, 8, "LocalDateTime"},
		{"LocalDate", tokenLocalDate, "1979-05-27", 1, 8, "LocalDate"},
		{"LocalTime", tokenLocalTime, "07:32:00", 1, 8, "LocalTime"},
		{"Equals", tokenEquals, "=", 1, 5, "Equals"},
		{"Dot", tokenDot, ".", 1, 6, "Dot"},
		{"Comma", tokenComma, ",", 1, 10, "Comma"},
		{"LeftBracket", tokenLeftBracket, "[", 1, 1, "LeftBracket"},
		{"RightBracket", tokenRightBracket, "]", 1, 8, "RightBracket"},
		{"DoubleLeftBracket", tokenDoubleLeftBracket, "[[", 1, 1, "DoubleLeftBracket"},
		{"DoubleRightBracket", tokenDoubleRightBracket, "]]", 1, 8, "DoubleRightBracket"},
		{"LeftBrace", tokenLeftBrace, "{", 1, 8, "LeftBrace"},
		{"RightBrace", tokenRightBrace, "}", 1, 15, "RightBrace"},
		{"Comment", tokenComment, "# a comment", 1, 1, "Comment"},
		{"Whitespace", tokenWhitespace, "  ", 1, 1, "Whitespace"},
		{"Newline", tokenNewline, "\n", 1, 12, "Newline"},
		{"EOF", tokenEOF, "", 5, 1, "EOF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := token{
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
	if last.Type != tokenEOF || last.Offset != len(src) {
		t.Errorf("last token = %s at offset %d, want EOF at %d", last.Type, last.Offset, len(src))
	}
}

func TestTokenTypeStringUnknown(t *testing.T) {
	unknown := tokenType(999)
	if got := unknown.String(); got != "Unknown" {
		t.Errorf("String() = %q, want %q", got, "Unknown")
	}
}
