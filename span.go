package tomledit

// Position is a 1-based line/column location in TOML source. Columns count
// bytes (not runes), matching the convention used by ParseError and Token.
type Position struct {
	Line   int // 1-based line number
	Column int // 1-based byte column within the line
}

// IsValid reports whether the position was populated from source (Line >= 1).
// The zero Position is invalid.
func (p Position) IsValid() bool { return p.Line >= 1 }

// Span is the half-open source range [Start, End) covered by a node: Start is
// the position of the node's first byte, End is the position immediately
// after its last byte.
//
// Spans reflect the most recent Parse. Edit operations (Set, Delete, RenameKey,
// Merge, ...) do not update spans: nodes created programmatically carry the
// zero Span (IsValid reports false), and nodes whose content was edited keep
// the span from the last parse. To obtain fresh spans after editing,
// serialize with Bytes and re-Parse the result.
type Span struct {
	Start Position
	End   Position
}

// IsValid reports whether the span was populated by Parse. The zero Span is
// invalid; it is returned by nodes created programmatically (edits, Marshal)
// and by internal virtual view nodes.
func (s Span) IsValid() bool { return s.Start.IsValid() }

// advancePos returns the position after consuming raw, using the same
// line/column rules as the lexer: a newline byte moves to the next line at
// column 1, every other byte advances the column by one.
func advancePos(p Position, raw []byte) Position {
	for _, b := range raw {
		if b == '\n' {
			p.Line++
			p.Column = 1
		} else {
			p.Column++
		}
	}
	return p
}

// tokenStart returns the start position of a token.
func tokenStart(tok Token) Position {
	return Position{Line: tok.Line, Column: tok.Column}
}

// spanFromToken returns the span covering exactly one token.
func spanFromToken(tok Token) Span {
	start := tokenStart(tok)
	return Span{Start: start, End: advancePos(start, tok.Raw)}
}
