package tomledit

import (
	"strings"
	"testing"
)

// tok is a helper for concise test token definitions.
type tok struct {
	typ TokenType
	raw string
}

func assertTokens(t *testing.T, input string, expected []tok) {
	t.Helper()
	tokens, err := lex([]byte(input))
	if err != nil {
		t.Fatalf("lex(%q) returned error: %v", input, err)
	}
	// Filter out EOF for comparison (we always append it)
	var got []Token
	for _, tk := range tokens {
		if tk.Type == TokenEOF {
			continue
		}
		got = append(got, tk)
	}
	if len(got) != len(expected) {
		t.Errorf("lex(%q): got %d tokens, want %d", input, len(got), len(expected))
		for i, tk := range got {
			t.Logf("  got[%d]: %s %q (line=%d col=%d)", i, tk.Type, string(tk.Raw), tk.Line, tk.Column)
		}
		for i, tk := range expected {
			t.Logf("  want[%d]: %s %q", i, tk.typ, tk.raw)
		}
		return
	}
	for i, tk := range got {
		exp := expected[i]
		if tk.Type != exp.typ || string(tk.Raw) != exp.raw {
			t.Errorf("lex(%q) token[%d]: got (%s, %q), want (%s, %q)",
				input, i, tk.Type, string(tk.Raw), exp.typ, exp.raw)
		}
	}
}

func assertError(t *testing.T, input string, msgSubstr string) {
	t.Helper()
	_, err := lex([]byte(input))
	if err == nil {
		t.Fatalf("lex(%q): expected error containing %q, got nil", input, msgSubstr)
	}
	if !strings.Contains(err.Error(), msgSubstr) {
		t.Errorf("lex(%q): error %q does not contain %q", input, err.Error(), msgSubstr)
	}
}

// --- Simple key-value tests ---

func TestLexSimpleKeyValue(t *testing.T) {
	assertTokens(t, "key = \"value\"\n", []tok{
		{TokenBareKey, "key"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenBasicString, "\"value\""},
		{TokenNewline, "\n"},
	})
}

func TestLexKeyValueNoTrailingNewline(t *testing.T) {
	assertTokens(t, "key = \"value\"", []tok{
		{TokenBareKey, "key"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenBasicString, "\"value\""},
	})
}

func TestLexMultipleKeyValues(t *testing.T) {
	assertTokens(t, "a = 1\nb = 2\n", []tok{
		{TokenBareKey, "a"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenInteger, "1"},
		{TokenNewline, "\n"},
		{TokenBareKey, "b"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenInteger, "2"},
		{TokenNewline, "\n"},
	})
}

func TestLexDottedKey(t *testing.T) {
	assertTokens(t, "a.b.c = 1\n", []tok{
		{TokenBareKey, "a"},
		{TokenDot, "."},
		{TokenBareKey, "b"},
		{TokenDot, "."},
		{TokenBareKey, "c"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenInteger, "1"},
		{TokenNewline, "\n"},
	})
}

func TestLexDottedKeyWithSpaces(t *testing.T) {
	assertTokens(t, "a . b = 1\n", []tok{
		{TokenBareKey, "a"},
		{TokenWhitespace, " "},
		{TokenDot, "."},
		{TokenWhitespace, " "},
		{TokenBareKey, "b"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenInteger, "1"},
		{TokenNewline, "\n"},
	})
}

// --- String tests ---

func TestLexBasicString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "simple",
			input: `key = "hello"` + "\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenBasicString, `"hello"`},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "empty",
			input: `key = ""` + "\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenBasicString, `""`},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "escapes",
			input: `key = "tab\there\nnewline"` + "\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenBasicString, `"tab\there\nnewline"`},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "unicode_escape_4",
			input: `key = "A"` + "\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenBasicString, `"A"`},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "unicode_escape_8",
			input: `key = "\U0001F600"` + "\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenBasicString, `"\U0001F600"`},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "escaped_quotes",
			input: `key = "say \"hello\""` + "\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenBasicString, `"say \"hello\""`},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "escaped_backslash",
			input: `key = "C:\\path\\to"` + "\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenBasicString, `"C:\\path\\to"`},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

func TestLexLiteralString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "simple",
			input: "key = 'hello'\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLiteralString, "'hello'"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "empty",
			input: "key = ''\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLiteralString, "''"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "windows_path",
			input: "key = 'C:\\path\\to\\file'\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLiteralString, "'C:\\path\\to\\file'"},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

func TestLexMultiLineBasicString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "simple",
			input: "key = \"\"\"hello\nworld\"\"\"\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenMultiLineBasicString, "\"\"\"hello\nworld\"\"\""},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "empty",
			input: "key = \"\"\"\"\"\"\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenMultiLineBasicString, "\"\"\"\"\"\""},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "with_escape",
			input: "key = \"\"\"hello\\nworld\"\"\"\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenMultiLineBasicString, "\"\"\"hello\\nworld\"\"\""},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "line_ending_backslash",
			input: "key = \"\"\"hello \\\n  world\"\"\"\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenMultiLineBasicString, "\"\"\"hello \\\n  world\"\"\""},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

func TestLexMultiLineLiteralString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "simple",
			input: "key = '''hello\nworld'''\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenMultiLineLiteralString, "'''hello\nworld'''"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "with_single_quotes",
			input: "key = '''it's ok'''\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenMultiLineLiteralString, "'''it's ok'''"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "with_double_quotes_inside",
			input: "key = '''two ''quotes'' here'''\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenMultiLineLiteralString, "'''two ''quotes'' here'''"},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

func TestLexQuotedKey(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "basic_string_key",
			input: "\"key with spaces\" = 1\n",
			want: []tok{
				{TokenBasicString, "\"key with spaces\""},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "1"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "literal_string_key",
			input: "'key' = 1\n",
			want: []tok{
				{TokenLiteralString, "'key'"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "1"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "dotted_with_quoted_parts",
			input: "a.\"b c\".d = 1\n",
			want: []tok{
				{TokenBareKey, "a"},
				{TokenDot, "."},
				{TokenBasicString, "\"b c\""},
				{TokenDot, "."},
				{TokenBareKey, "d"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "1"},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

// --- Number tests ---

func TestLexIntegers(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "simple",
			input: "key = 42\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "42"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "positive",
			input: "key = +42\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "+42"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "negative",
			input: "key = -42\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "-42"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "zero",
			input: "key = 0\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "0"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "underscores",
			input: "key = 1_000_000\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "1_000_000"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "hex",
			input: "key = 0xDEAD_BEEF\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "0xDEAD_BEEF"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "octal",
			input: "key = 0o755\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "0o755"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "binary",
			input: "key = 0b1010\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "0b1010"},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

func TestLexFloats(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "simple",
			input: "key = 3.14\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "3.14"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "positive",
			input: "key = +1.0\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "+1.0"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "negative",
			input: "key = -0.5\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "-0.5"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "exponent",
			input: "key = 5e+22\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "5e+22"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "exponent_no_sign",
			input: "key = 1e06\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "1e06"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "float_with_exponent",
			input: "key = 6.626e-34\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "6.626e-34"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "inf",
			input: "key = inf\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "inf"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "positive_inf",
			input: "key = +inf\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "+inf"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "negative_inf",
			input: "key = -inf\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "-inf"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "nan",
			input: "key = nan\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "nan"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "positive_nan",
			input: "key = +nan\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "+nan"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "negative_nan",
			input: "key = -nan\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "-nan"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "underscore_in_float",
			input: "key = 1_000.5\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenFloat, "1_000.5"},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

// --- Boolean tests ---

func TestLexBooleans(t *testing.T) {
	assertTokens(t, "key = true\n", []tok{
		{TokenBareKey, "key"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenBoolean, "true"},
		{TokenNewline, "\n"},
	})
	assertTokens(t, "key = false\n", []tok{
		{TokenBareKey, "key"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenBoolean, "false"},
		{TokenNewline, "\n"},
	})
}

// --- Date/time tests ---

func TestLexDateTimes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "offset_datetime_Z",
			input: "key = 1979-05-27T07:32:00Z\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenOffsetDateTime, "1979-05-27T07:32:00Z"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "offset_datetime_offset",
			input: "key = 1979-05-27T07:32:00+00:00\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenOffsetDateTime, "1979-05-27T07:32:00+00:00"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "offset_datetime_negative_offset",
			input: "key = 1979-05-27T07:32:00-05:00\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenOffsetDateTime, "1979-05-27T07:32:00-05:00"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "offset_datetime_with_fractional",
			input: "key = 1979-05-27T07:32:00.999999Z\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenOffsetDateTime, "1979-05-27T07:32:00.999999Z"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "offset_datetime_space_separator",
			input: "key = 1979-05-27 07:32:00Z\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenOffsetDateTime, "1979-05-27 07:32:00Z"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "local_datetime",
			input: "key = 1979-05-27T07:32:00\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLocalDateTime, "1979-05-27T07:32:00"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "local_datetime_with_fractional",
			input: "key = 1979-05-27T07:32:00.123\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLocalDateTime, "1979-05-27T07:32:00.123"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "local_date",
			input: "key = 1979-05-27\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLocalDate, "1979-05-27"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "local_time",
			input: "key = 07:32:00\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLocalTime, "07:32:00"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "local_time_with_fractional",
			input: "key = 07:32:00.999999\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLocalTime, "07:32:00.999999"},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

// --- Comment tests ---

func TestLexComments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "full_line_comment",
			input: "# this is a comment\n",
			want: []tok{
				{TokenComment, "# this is a comment"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "inline_comment",
			input: "key = 1 # inline\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "1"},
				{TokenWhitespace, " "},
				{TokenComment, "# inline"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "comment_only",
			input: "# comment",
			want: []tok{
				{TokenComment, "# comment"},
			},
		},
		{
			name:  "empty_comment",
			input: "#\n",
			want: []tok{
				{TokenComment, "#"},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

// --- Table header tests ---

func TestLexTableHeaders(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "simple_table",
			input: "[table]\n",
			want: []tok{
				{TokenLeftBracket, "["},
				{TokenBareKey, "table"},
				{TokenRightBracket, "]"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "dotted_table",
			input: "[a.b.c]\n",
			want: []tok{
				{TokenLeftBracket, "["},
				{TokenBareKey, "a"},
				{TokenDot, "."},
				{TokenBareKey, "b"},
				{TokenDot, "."},
				{TokenBareKey, "c"},
				{TokenRightBracket, "]"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "table_with_spaces",
			input: "[ table ]\n",
			want: []tok{
				{TokenLeftBracket, "["},
				{TokenWhitespace, " "},
				{TokenBareKey, "table"},
				{TokenWhitespace, " "},
				{TokenRightBracket, "]"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "array_table",
			input: "[[products]]\n",
			want: []tok{
				{TokenDoubleLeftBracket, "[["},
				{TokenBareKey, "products"},
				{TokenDoubleRightBracket, "]]"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "array_table_dotted",
			input: "[[a.b]]\n",
			want: []tok{
				{TokenDoubleLeftBracket, "[["},
				{TokenBareKey, "a"},
				{TokenDot, "."},
				{TokenBareKey, "b"},
				{TokenDoubleRightBracket, "]]"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "table_with_quoted_key",
			input: "[\"a b\"]\n",
			want: []tok{
				{TokenLeftBracket, "["},
				{TokenBasicString, "\"a b\""},
				{TokenRightBracket, "]"},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

// --- Inline table tests ---

func TestLexInlineTables(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "simple",
			input: "key = {a = 1, b = 2}\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLeftBrace, "{"},
				{TokenBareKey, "a"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "1"},
				{TokenComma, ","},
				{TokenWhitespace, " "},
				{TokenBareKey, "b"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "2"},
				{TokenRightBrace, "}"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "empty",
			input: "key = {}\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLeftBrace, "{"},
				{TokenRightBrace, "}"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "string_values",
			input: "key = {a = \"hello\"}\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLeftBrace, "{"},
				{TokenBareKey, "a"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenBasicString, "\"hello\""},
				{TokenRightBrace, "}"},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

// --- Array tests ---

func TestLexArrays(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "simple",
			input: "key = [1, 2, 3]\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLeftBracket, "["},
				{TokenInteger, "1"},
				{TokenComma, ","},
				{TokenWhitespace, " "},
				{TokenInteger, "2"},
				{TokenComma, ","},
				{TokenWhitespace, " "},
				{TokenInteger, "3"},
				{TokenRightBracket, "]"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "multi_line",
			input: "key = [\n  1,\n  2,\n]\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLeftBracket, "["},
				{TokenNewline, "\n"},
				{TokenWhitespace, "  "},
				{TokenInteger, "1"},
				{TokenComma, ","},
				{TokenNewline, "\n"},
				{TokenWhitespace, "  "},
				{TokenInteger, "2"},
				{TokenComma, ","},
				{TokenNewline, "\n"},
				{TokenRightBracket, "]"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "nested_arrays",
			input: "key = [[1, 2], [3, 4]]\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLeftBracket, "["},
				{TokenLeftBracket, "["},
				{TokenInteger, "1"},
				{TokenComma, ","},
				{TokenWhitespace, " "},
				{TokenInteger, "2"},
				{TokenRightBracket, "]"},
				{TokenComma, ","},
				{TokenWhitespace, " "},
				{TokenLeftBracket, "["},
				{TokenInteger, "3"},
				{TokenComma, ","},
				{TokenWhitespace, " "},
				{TokenInteger, "4"},
				{TokenRightBracket, "]"},
				{TokenRightBracket, "]"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "string_array",
			input: "key = [\"a\", \"b\"]\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLeftBracket, "["},
				{TokenBasicString, "\"a\""},
				{TokenComma, ","},
				{TokenWhitespace, " "},
				{TokenBasicString, "\"b\""},
				{TokenRightBracket, "]"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "empty_array",
			input: "key = []\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenLeftBracket, "["},
				{TokenRightBracket, "]"},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

// --- Complex document tests ---

func TestLexComplexDocument(t *testing.T) {
	input := `# Configuration file
title = "TOML Example"

[owner]
name = "Tom Preston-Werner"
dob = 1979-05-27T07:32:00-08:00

[database]
enabled = true
ports = [8001, 8001, 8002]
data = [["delta", "phi"], [3.14]]
temp_targets = {cpu = 79.5, case = 72.0}

[servers]

[servers.alpha]
ip = "10.0.0.1"
role = "frontend"

[servers.beta]
ip = "10.0.0.2"
role = "backend"
`

	tokens, err := lex([]byte(input))
	if err != nil {
		t.Fatalf("lex complex document: %v", err)
	}

	// Verify key properties rather than every single token.
	// Check that the document starts with a comment
	if tokens[0].Type != TokenComment {
		t.Errorf("expected first token to be Comment, got %s", tokens[0].Type)
	}
	if string(tokens[0].Raw) != "# Configuration file" {
		t.Errorf("expected first token raw to be '# Configuration file', got %q", string(tokens[0].Raw))
	}

	// Check that we find table headers
	foundTable := false
	foundArrayBracket := false
	for _, tk := range tokens {
		if tk.Type == TokenLeftBracket {
			foundTable = true
		}
		if tk.Type == TokenLeftBracket && foundTable {
			foundArrayBracket = true
		}
	}
	if !foundTable {
		t.Error("expected to find table headers")
	}
	if !foundArrayBracket {
		t.Error("expected to find array brackets")
	}

	// Check that we have booleans
	foundBool := false
	for _, tk := range tokens {
		if tk.Type == TokenBoolean && string(tk.Raw) == "true" {
			foundBool = true
			break
		}
	}
	if !foundBool {
		t.Error("expected to find boolean 'true'")
	}

	// Check we have integers
	foundInt := false
	for _, tk := range tokens {
		if tk.Type == TokenInteger && string(tk.Raw) == "8001" {
			foundInt = true
			break
		}
	}
	if !foundInt {
		t.Error("expected to find integer '8001'")
	}

	// Check we have floats
	foundFloat := false
	for _, tk := range tokens {
		if tk.Type == TokenFloat && string(tk.Raw) == "3.14" {
			foundFloat = true
			break
		}
	}
	if !foundFloat {
		t.Error("expected to find float '3.14'")
	}

	// Check we have offset datetime
	foundDateTime := false
	for _, tk := range tokens {
		if tk.Type == TokenOffsetDateTime {
			foundDateTime = true
			break
		}
	}
	if !foundDateTime {
		t.Error("expected to find offset datetime")
	}

	// Check inline table
	foundBrace := false
	for _, tk := range tokens {
		if tk.Type == TokenLeftBrace {
			foundBrace = true
			break
		}
	}
	if !foundBrace {
		t.Error("expected to find inline table '{'")
	}

	// Last token should be EOF
	last := tokens[len(tokens)-1]
	if last.Type != TokenEOF {
		t.Errorf("expected last token to be EOF, got %s", last.Type)
	}

	// Verify round-trip: concatenating all Raw fields should reproduce input
	var reconstructed []byte
	for _, tk := range tokens {
		reconstructed = append(reconstructed, tk.Raw...)
	}
	if string(reconstructed) != input {
		t.Error("round-trip failed: concatenated Raw fields do not match input")
	}
}

// --- Whitespace tests ---

func TestLexWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "tabs",
			input: "\tkey = 1\n",
			want: []tok{
				{TokenWhitespace, "\t"},
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "1"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "crlf",
			input: "key = 1\r\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "1"},
				{TokenNewline, "\r\n"},
			},
		},
		{
			name:  "multiple_spaces",
			input: "key   =   1\n",
			want: []tok{
				{TokenBareKey, "key"},
				{TokenWhitespace, "   "},
				{TokenEquals, "="},
				{TokenWhitespace, "   "},
				{TokenInteger, "1"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "blank_lines",
			input: "\n\n",
			want: []tok{
				{TokenNewline, "\n"},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

// --- Position tracking tests ---

func TestLexPositionTracking(t *testing.T) {
	input := "a = 1\nb = 2\n"
	tokens, err := lex([]byte(input))
	if err != nil {
		t.Fatalf("lex: %v", err)
	}

	// "a" should be at line 1, col 1
	if tokens[0].Line != 1 || tokens[0].Column != 1 {
		t.Errorf("token 'a': got line=%d col=%d, want line=1 col=1", tokens[0].Line, tokens[0].Column)
	}

	// "=" should be at line 1, col 3
	if tokens[2].Line != 1 || tokens[2].Column != 3 {
		t.Errorf("token '=': got line=%d col=%d, want line=1 col=3", tokens[2].Line, tokens[2].Column)
	}

	// "1" should be at line 1, col 5
	if tokens[4].Line != 1 || tokens[4].Column != 5 {
		t.Errorf("token '1': got line=%d col=%d, want line=1 col=5", tokens[4].Line, tokens[4].Column)
	}

	// "\n" should be at line 1, col 6
	if tokens[5].Line != 1 || tokens[5].Column != 6 {
		t.Errorf("token '\\n': got line=%d col=%d, want line=1 col=6", tokens[5].Line, tokens[5].Column)
	}

	// "b" should be at line 2, col 1
	if tokens[6].Line != 2 || tokens[6].Column != 1 {
		t.Errorf("token 'b': got line=%d col=%d, want line=2 col=1", tokens[6].Line, tokens[6].Column)
	}
}

// --- Error tests ---

func TestLexErrors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "unterminated_basic_string",
			input:    "key = \"hello\n",
			contains: "unterminated basic string",
		},
		{
			name:     "unterminated_basic_string_eof",
			input:    "key = \"hello",
			contains: "unterminated basic string",
		},
		{
			name:     "unterminated_literal_string",
			input:    "key = 'hello\n",
			contains: "unterminated literal string",
		},
		{
			name:     "unterminated_literal_string_eof",
			input:    "key = 'hello",
			contains: "unterminated literal string",
		},
		{
			name:     "unterminated_multiline_basic_string",
			input:    "key = \"\"\"hello",
			contains: "unterminated multi-line basic string",
		},
		{
			name:     "unterminated_multiline_literal_string",
			input:    "key = '''hello",
			contains: "unterminated multi-line literal string",
		},
		{
			name:     "invalid_escape",
			input:    "key = \"\\x\"",
			contains: "invalid escape sequence",
		},
		{
			name:     "invalid_unicode_escape",
			input:    "key = \"\\uZZZZ\"",
			contains: "invalid unicode escape",
		},
		{
			name:     "leading_zeros",
			input:    "key = 07\n",
			contains: "leading zeros",
		},
		{
			name:     "trailing_underscore",
			input:    "key = 1_\n",
			contains: "underscore",
		},
		{
			name:     "unexpected_character",
			input:    "key = @\n",
			contains: "unexpected character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertError(t, tt.input, tt.contains)
		})
	}
}

// --- Bare key with special words tests ---

func TestLexBareKeySpecialWords(t *testing.T) {
	// In key context, true/false/inf/nan are bare keys
	tests := []struct {
		name  string
		input string
		want  []tok
	}{
		{
			name:  "true_as_key",
			input: "true = 1\n",
			want: []tok{
				{TokenBareKey, "true"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "1"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "false_as_key",
			input: "false = 1\n",
			want: []tok{
				{TokenBareKey, "false"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "1"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "inf_as_key",
			input: "inf = 1\n",
			want: []tok{
				{TokenBareKey, "inf"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "1"},
				{TokenNewline, "\n"},
			},
		},
		{
			name:  "nan_as_key",
			input: "nan = 1\n",
			want: []tok{
				{TokenBareKey, "nan"},
				{TokenWhitespace, " "},
				{TokenEquals, "="},
				{TokenWhitespace, " "},
				{TokenInteger, "1"},
				{TokenNewline, "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTokens(t, tt.input, tt.want)
		})
	}
}

// --- Round-trip tests ---

func TestLexRoundTrip(t *testing.T) {
	inputs := []string{
		"key = \"value\"\n",
		"[table]\nkey = 42\n",
		"[[array]]\nname = \"test\"\n",
		"key = {a = 1, b = \"hello\"}\n",
		"key = [1, 2, 3]\n",
		"# comment\nkey = true\n",
		"key = 1979-05-27T07:32:00Z\n",
		"key = 07:32:00\n",
		"key = inf\nk2 = nan\n",
		"key = 0xDEAD\nk2 = 0o777\nk3 = 0b1010\n",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			tokens, err := lex([]byte(input))
			if err != nil {
				t.Fatalf("lex(%q): %v", input, err)
			}
			var reconstructed []byte
			for _, tk := range tokens {
				reconstructed = append(reconstructed, tk.Raw...)
			}
			if string(reconstructed) != input {
				t.Errorf("round-trip failed for %q: got %q", input, string(reconstructed))
			}
		})
	}
}

// --- Empty input ---

func TestLexEmpty(t *testing.T) {
	tokens, err := lex([]byte(""))
	if err != nil {
		t.Fatalf("lex empty: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Type != TokenEOF {
		t.Errorf("lex empty: expected [EOF], got %v", tokens)
	}
}

// --- ParseError type test ---

func TestParseErrorFormat(t *testing.T) {
	e := &ParseError{
		Line:    3,
		Column:  10,
		Message: "unexpected character",
	}
	got := e.Error()
	expected := "line 3, column 10: unexpected character"
	if got != expected {
		t.Errorf("ParseError.Error() = %q, want %q", got, expected)
	}
}

// --- Array of inline tables ---

func TestLexArrayOfInlineTables(t *testing.T) {
	assertTokens(t, "key = [{a = 1}, {b = 2}]\n", []tok{
		{TokenBareKey, "key"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenLeftBracket, "["},
		{TokenLeftBrace, "{"},
		{TokenBareKey, "a"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenInteger, "1"},
		{TokenRightBrace, "}"},
		{TokenComma, ","},
		{TokenWhitespace, " "},
		{TokenLeftBrace, "{"},
		{TokenBareKey, "b"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenInteger, "2"},
		{TokenRightBrace, "}"},
		{TokenRightBracket, "]"},
		{TokenNewline, "\n"},
	})
}

// --- Boolean in array ---

func TestLexBooleanInArray(t *testing.T) {
	assertTokens(t, "key = [true, false]\n", []tok{
		{TokenBareKey, "key"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenLeftBracket, "["},
		{TokenBoolean, "true"},
		{TokenComma, ","},
		{TokenWhitespace, " "},
		{TokenBoolean, "false"},
		{TokenRightBracket, "]"},
		{TokenNewline, "\n"},
	})
}

// --- Mixed array with comment ---

func TestLexArrayWithComment(t *testing.T) {
	assertTokens(t, "key = [\n  1, # first\n  2,\n]\n", []tok{
		{TokenBareKey, "key"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenLeftBracket, "["},
		{TokenNewline, "\n"},
		{TokenWhitespace, "  "},
		{TokenInteger, "1"},
		{TokenComma, ","},
		{TokenWhitespace, " "},
		{TokenComment, "# first"},
		{TokenNewline, "\n"},
		{TokenWhitespace, "  "},
		{TokenInteger, "2"},
		{TokenComma, ","},
		{TokenNewline, "\n"},
		{TokenRightBracket, "]"},
		{TokenNewline, "\n"},
	})
}

// --- Table header after key-value ---

func TestLexTableAfterKeyValue(t *testing.T) {
	assertTokens(t, "a = 1\n[b]\nc = 2\n", []tok{
		{TokenBareKey, "a"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenInteger, "1"},
		{TokenNewline, "\n"},
		{TokenLeftBracket, "["},
		{TokenBareKey, "b"},
		{TokenRightBracket, "]"},
		{TokenNewline, "\n"},
		{TokenBareKey, "c"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenInteger, "2"},
		{TokenNewline, "\n"},
	})
}

// --- Indented table header ---

func TestLexIndentedTableHeader(t *testing.T) {
	// Whitespace before a table header
	assertTokens(t, "  [table]\n", []tok{
		{TokenWhitespace, "  "},
		{TokenLeftBracket, "["},
		{TokenBareKey, "table"},
		{TokenRightBracket, "]"},
		{TokenNewline, "\n"},
	})
}

// --- Multiple array tables ---

func TestLexMultipleArrayTables(t *testing.T) {
	assertTokens(t, "[[products]]\nname = \"a\"\n[[products]]\nname = \"b\"\n", []tok{
		{TokenDoubleLeftBracket, "[["},
		{TokenBareKey, "products"},
		{TokenDoubleRightBracket, "]]"},
		{TokenNewline, "\n"},
		{TokenBareKey, "name"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenBasicString, "\"a\""},
		{TokenNewline, "\n"},
		{TokenDoubleLeftBracket, "[["},
		{TokenBareKey, "products"},
		{TokenDoubleRightBracket, "]]"},
		{TokenNewline, "\n"},
		{TokenBareKey, "name"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenBasicString, "\"b\""},
		{TokenNewline, "\n"},
	})
}

// --- Negative float ---

func TestLexNegativeFloat(t *testing.T) {
	assertTokens(t, "key = -0.5\n", []tok{
		{TokenBareKey, "key"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenFloat, "-0.5"},
		{TokenNewline, "\n"},
	})
}

// --- Table header then array table ---

func TestLexTableThenArrayTable(t *testing.T) {
	assertTokens(t, "[a]\nb = 1\n[[c]]\nd = 2\n", []tok{
		{TokenLeftBracket, "["},
		{TokenBareKey, "a"},
		{TokenRightBracket, "]"},
		{TokenNewline, "\n"},
		{TokenBareKey, "b"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenInteger, "1"},
		{TokenNewline, "\n"},
		{TokenDoubleLeftBracket, "[["},
		{TokenBareKey, "c"},
		{TokenDoubleRightBracket, "]]"},
		{TokenNewline, "\n"},
		{TokenBareKey, "d"},
		{TokenWhitespace, " "},
		{TokenEquals, "="},
		{TokenWhitespace, " "},
		{TokenInteger, "2"},
		{TokenNewline, "\n"},
	})
}
