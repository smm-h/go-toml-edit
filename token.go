package tomledit

// TokenType identifies the kind of lexical token.
type TokenType int

const (
	TokenBareKey                TokenType = iota // TokenBareKey is an unquoted key (e.g. host).
	TokenBasicString                             // TokenBasicString is a double-quoted string ("...").
	TokenLiteralString                           // TokenLiteralString is a single-quoted string ('...').
	TokenMultiLineBasicString                    // TokenMultiLineBasicString is a triple-double-quoted string.
	TokenMultiLineLiteralString                  // TokenMultiLineLiteralString is a triple-single-quoted string.
	TokenInteger                                 // TokenInteger is an integer literal.
	TokenFloat                                   // TokenFloat is a float literal.
	TokenBoolean                                 // TokenBoolean is true or false.
	TokenOffsetDateTime                          // TokenOffsetDateTime is a date-time with timezone offset.
	TokenLocalDateTime                           // TokenLocalDateTime is a local date-time (no timezone).
	TokenLocalDate                               // TokenLocalDate is a local date (YYYY-MM-DD).
	TokenLocalTime                               // TokenLocalTime is a local time (HH:MM:SS).
	TokenEquals                                  // TokenEquals is the = sign.
	TokenDot                                     // TokenDot is the . separator in dotted keys.
	TokenComma                                   // TokenComma is a , separator.
	TokenLeftBracket                             // TokenLeftBracket is [.
	TokenRightBracket                            // TokenRightBracket is ].
	TokenDoubleLeftBracket                       // TokenDoubleLeftBracket is [[.
	TokenDoubleRightBracket                      // TokenDoubleRightBracket is ]].
	TokenLeftBrace                               // TokenLeftBrace is {.
	TokenRightBrace                              // TokenRightBrace is }.
	TokenComment                                 // TokenComment is a # comment.
	TokenWhitespace                              // TokenWhitespace is spaces or tabs.
	TokenNewline                                 // TokenNewline is a line break.
	TokenEOF                                     // TokenEOF marks end of input.
)

var tokenTypeNames = [...]string{
	TokenBareKey:                "BareKey",
	TokenBasicString:            "BasicString",
	TokenLiteralString:          "LiteralString",
	TokenMultiLineBasicString:   "MultiLineBasicString",
	TokenMultiLineLiteralString: "MultiLineLiteralString",
	TokenInteger:                "Integer",
	TokenFloat:                  "Float",
	TokenBoolean:                "Boolean",
	TokenOffsetDateTime:         "OffsetDateTime",
	TokenLocalDateTime:          "LocalDateTime",
	TokenLocalDate:              "LocalDate",
	TokenLocalTime:              "LocalTime",
	TokenEquals:                 "Equals",
	TokenDot:                    "Dot",
	TokenComma:                  "Comma",
	TokenLeftBracket:            "LeftBracket",
	TokenRightBracket:           "RightBracket",
	TokenDoubleLeftBracket:      "DoubleLeftBracket",
	TokenDoubleRightBracket:     "DoubleRightBracket",
	TokenLeftBrace:              "LeftBrace",
	TokenRightBrace:             "RightBrace",
	TokenComment:                "Comment",
	TokenWhitespace:             "Whitespace",
	TokenNewline:                "Newline",
	TokenEOF:                    "EOF",
}

// String returns the human-readable name of the token type.
func (t TokenType) String() string {
	if int(t) >= 0 && int(t) < len(tokenTypeNames) {
		return tokenTypeNames[t]
	}
	return "Unknown"
}

// Token represents a single lexical token from TOML source.
type Token struct {
	Type   TokenType
	Raw    []byte // exact bytes from source
	Line   int    // 1-based
	Column int    // 1-based
	Offset int    // 0-based byte offset of Raw within the source
}
