package tomledit

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The exported literal renderers: the bytes this library writes for a value it
// was handed, available on their own so that a consumer building TOML text
// outside a document writes the same spellings the library does.
//
// They are the canonical forms of the design record, and they are what a
// constructed or value-mutated node renders as. A value the library merely READ
// is not rendered at all -- it splices the bytes it was written as, so every
// TOML-valid spelling survives untouched.

// QuoteString returns s as a TOML basic string: double-quoted, with the
// backslash, the double quote and the control characters escaped and everything
// else written verbatim.
//
// The escapes are exactly TOML's own set. Backspace, tab, newline, form feed
// and carriage return take their short forms; every other control character,
// U+007F included, takes the four-digit "\u" form with LOWERCASE hex digits.
// Non-ASCII text is written as itself: a basic string carries it directly, and
// escaping it would only make the result harder to read.
//
// It is TOTAL: every Go string has an output. For a string that is valid UTF-8
// -- which is every string a write can carry, since the value-writing
// operations refuse anything else -- the result parses back to s, so QuoteString
// is the inverse of reading a string value. A string that is NOT valid UTF-8 is
// rendered with each invalid byte as U+FFFD, and the inverse property does not
// hold for it: no TOML spelling carries such a byte, and this renderer has no
// input to refuse.
func QuoteString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
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
				b.WriteString(`\u`)
				const hex = "0123456789abcdef"
				b.WriteByte('0')
				b.WriteByte('0')
				b.WriteByte(hex[(r>>4)&0xF])
				b.WriteByte(hex[r&0xF])
			} else {
				b.WriteRune(r)
			}
		}
		i += size
	}
	b.WriteByte('"')
	return b.String()
}

// QuoteKey returns s as a TOML key: bare when TOML's bare-key rule allows it
// (ASCII letters, digits, hyphens and underscores, and at least one character),
// and the basic-string form of QuoteString otherwise. The empty key is
// therefore written as a pair of quotes.
//
// It quotes ONE key. A dotted path is a sequence of keys, each quoted on its
// own and joined with dots; JoinPath does that for the library's path syntax.
//
// It is total on the same terms as QuoteString, and inherits its treatment of a
// key that is not valid UTF-8: the invalid bytes render as U+FFFD, and the
// key-writing operations refuse such a key before it ever reaches here.
func QuoteKey(s string) string {
	if isBareKey(s) {
		return s
	}
	return QuoteString(s)
}

// FormatFloat returns f as a TOML float. It is TOTAL: every float64 has an
// output, including the ones TOML has no numeric spelling for.
//
// A finite value is written in the shortest form that reads back as the same
// float64, with a float marker always present -- a ".0" is appended when the
// shortest form carries neither a fractional part nor an exponent, so the
// result never reads back as an integer. Negative zero keeps its sign.
//
// The three non-finite values are written "nan", "inf" and "-inf". A NaN with
// its sign bit set is written "nan" like any other: the library never writes
// "+nan", "-nan" or "+inf", which no reader needs and which say nothing about
// the value. (The value-writing operations refuse a sign-bit NaN as input
// rather than silently dropping the sign; this renderer, having no input to
// refuse, renders it.)
func FormatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".e") {
		s += ".0"
	}
	return s
}
