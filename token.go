package tomledit

// tokenType identifies the kind of lexical token.
type tokenType int

const (
	tokenBareKey                tokenType = iota // tokenBareKey is an unquoted key (e.g. host).
	tokenBasicString                             // tokenBasicString is a double-quoted string ("...").
	tokenLiteralString                           // tokenLiteralString is a single-quoted string ('...').
	tokenMultiLineBasicString                    // tokenMultiLineBasicString is a triple-double-quoted string.
	tokenMultiLineLiteralString                  // tokenMultiLineLiteralString is a triple-single-quoted string.
	tokenInteger                                 // tokenInteger is an integer literal.
	tokenFloat                                   // tokenFloat is a float literal.
	tokenBoolean                                 // tokenBoolean is true or false.
	tokenOffsetDateTime                          // tokenOffsetDateTime is a date-time with timezone offset.
	tokenLocalDateTime                           // tokenLocalDateTime is a local date-time (no timezone).
	tokenLocalDate                               // tokenLocalDate is a local date (YYYY-MM-DD).
	tokenLocalTime                               // tokenLocalTime is a local time (HH:MM:SS).
	tokenEquals                                  // tokenEquals is the = sign.
	tokenDot                                     // tokenDot is the . separator in dotted keys.
	tokenComma                                   // tokenComma is a , separator.
	tokenLeftBracket                             // tokenLeftBracket is [.
	tokenRightBracket                            // tokenRightBracket is ].
	tokenDoubleLeftBracket                       // tokenDoubleLeftBracket is [[.
	tokenDoubleRightBracket                      // tokenDoubleRightBracket is ]].
	tokenLeftBrace                               // tokenLeftBrace is {.
	tokenRightBrace                              // tokenRightBrace is }.
	tokenComment                                 // tokenComment is a # comment.
	tokenWhitespace                              // tokenWhitespace is spaces or tabs.
	tokenNewline                                 // tokenNewline is a line break.
	tokenEOF                                     // tokenEOF marks end of input.
)

var tokenTypeNames = [...]string{
	tokenBareKey:                "BareKey",
	tokenBasicString:            "BasicString",
	tokenLiteralString:          "LiteralString",
	tokenMultiLineBasicString:   "MultiLineBasicString",
	tokenMultiLineLiteralString: "MultiLineLiteralString",
	tokenInteger:                "Integer",
	tokenFloat:                  "Float",
	tokenBoolean:                "Boolean",
	tokenOffsetDateTime:         "OffsetDateTime",
	tokenLocalDateTime:          "LocalDateTime",
	tokenLocalDate:              "LocalDate",
	tokenLocalTime:              "LocalTime",
	tokenEquals:                 "Equals",
	tokenDot:                    "Dot",
	tokenComma:                  "Comma",
	tokenLeftBracket:            "LeftBracket",
	tokenRightBracket:           "RightBracket",
	tokenDoubleLeftBracket:      "DoubleLeftBracket",
	tokenDoubleRightBracket:     "DoubleRightBracket",
	tokenLeftBrace:              "LeftBrace",
	tokenRightBrace:             "RightBrace",
	tokenComment:                "Comment",
	tokenWhitespace:             "Whitespace",
	tokenNewline:                "Newline",
	tokenEOF:                    "EOF",
}

// String returns the internal name of the token type. It is the lexer's own
// vocabulary, for test failures and debugging output; a diagnostic a caller
// reads renders describe instead.
func (t tokenType) String() string {
	if int(t) >= 0 && int(t) < len(tokenTypeNames) {
		return tokenTypeNames[t]
	}
	return "Unknown"
}

// tokenTypeDescriptions spells each token type the way a diagnostic names it:
// the single place that decides, so every message reads the same. A token that
// is one fixed piece of punctuation is quoted as the glyph the writer typed,
// since naming it any other way just asks the reader to translate; everything
// else is plain words, with the article that makes "expected X, got Y" read as
// a sentence. Two spellings of the same thing (a basic and a literal string)
// describe alike: which quoting style was used is not what the message is
// about.
var tokenTypeDescriptions = [...]string{
	tokenBareKey:                "a bare key",
	tokenBasicString:            "a string",
	tokenLiteralString:          "a string",
	tokenMultiLineBasicString:   "a multi-line string",
	tokenMultiLineLiteralString: "a multi-line string",
	tokenInteger:                "an integer",
	tokenFloat:                  "a float",
	tokenBoolean:                "a boolean",
	tokenOffsetDateTime:         "an offset date-time",
	tokenLocalDateTime:          "a local date-time",
	tokenLocalDate:              "a local date",
	tokenLocalTime:              "a local time",
	tokenEquals:                 "'='",
	tokenDot:                    "'.'",
	tokenComma:                  "','",
	tokenLeftBracket:            "'['",
	tokenRightBracket:           "']'",
	tokenDoubleLeftBracket:      "'[['",
	tokenDoubleRightBracket:     "']]'",
	tokenLeftBrace:              "'{'",
	tokenRightBrace:             "'}'",
	tokenComment:                "a comment",
	tokenWhitespace:             "whitespace",
	tokenNewline:                "newline",
	tokenEOF:                    "end of input",
}

// describe returns the token type's user-facing spelling: what a diagnostic
// calls it. Every message that names a token routes through here, so a caller
// never reads the lexer's internal vocabulary.
func (t tokenType) describe() string {
	if int(t) >= 0 && int(t) < len(tokenTypeDescriptions) {
		return tokenTypeDescriptions[t]
	}
	return "an unrecognized token"
}

// token represents a single lexical token from TOML source.
type token struct {
	Type   tokenType
	Raw    []byte // exact bytes from source
	Line   int    // 1-based
	Column int    // 1-based
	Offset int    // 0-based byte offset of Raw within the source
}
