package tomledit

import (
	"strings"
	"testing"
)

// What a comment may carry is the lexer's rule, and a comment write mirrors it:
// the text must be valid UTF-8 and may hold no control character other than a
// tab -- which makes a newline, a carriage return, U+0000 and U+007F all
// refusals. Text the library would write verbatim otherwise leaves a document
// whose bytes do not parse: a newline ends the comment and turns its tail into
// a line of TOML nobody wrote, and an invalid byte is read back as U+FFFD.

// commentControlCases enumerates the control characters a comment cannot carry.
var commentControlCases = []struct {
	name string
	text string
}{
	{"newline", "first\nsecond = 2"},
	{"carriage return", "first\rsecond"},
	{"nul", "a\x00b"},
	{"unit separator", "a\x1fb"},
	{"delete", "a\x7fb"},
	{"bell", "a\x07b"},
}

// Fails if a comment write ever again puts a byte in the document that the
// lexer would refuse to read back.
func TestSetComment_RefusesTextTheLexerCannotRead(t *testing.T) {
	const src = "x = 1\n"
	t.Run("invalid UTF-8", func(t *testing.T) {
		doc := parseOrFail(t, src)
		refuseWrite(t, doc, src, "SetComment", doc.SetComment("x", "note "+badUTF8))
	})
	for _, tc := range commentControlCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseOrFail(t, src)
			refuseWrite(t, doc, src, "SetComment", doc.SetComment("x", tc.text))
		})
	}
}

// The same rule on the leading-comment setter, where each element is one
// comment line: a newline inside an element is a second line the element does
// not get to open.
//
// Fails if SetLeadingComments ever again writes a line the lexer cannot read.
func TestSetLeadingComments_RefusesTextTheLexerCannotRead(t *testing.T) {
	const src = "x = 1\n"
	t.Run("invalid UTF-8", func(t *testing.T) {
		doc := parseOrFail(t, src)
		refuseWrite(t, doc, src, "SetLeadingComments", doc.SetLeadingComments("x", []string{"note " + badUTF8}))
	})
	for _, tc := range commentControlCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseOrFail(t, src)
			refuseWrite(t, doc, src, "SetLeadingComments", doc.SetLeadingComments("x", []string{tc.text}))
		})
	}
}

// A refused element refuses the whole call: the caller asked for one block of
// comment lines, and a document carrying the good half of it is a document the
// caller never described.
//
// Fails if SetLeadingComments starts writing the elements it accepted before
// reaching one it refuses.
func TestSetLeadingComments_RefusesTheWholeBlockOnOneBadLine(t *testing.T) {
	const src = "x = 1\n"
	doc := parseOrFail(t, src)
	err := doc.SetLeadingComments("x", []string{"fine", "also fine", "bad\nline"})
	refuseWrite(t, doc, src, "SetLeadingComments", err)
	if strings.Contains(string(doc.Bytes()), "fine") {
		t.Errorf("the refused call wrote the lines it had already accepted: %q", doc.Bytes())
	}
}

// A refusal leaves the comments the node already carries alone.
//
// Fails if a refused comment write clears or rewrites what was there.
func TestSetComment_RefusalLeavesTheExistingCommentAlone(t *testing.T) {
	const src = "# above\nx = 1 # beside\n"
	doc := parseOrFail(t, src)
	refuseWrite(t, doc, src, "SetComment", doc.SetComment("x", "bad\nline"))
	refuseWrite(t, doc, src, "SetLeadingComments", doc.SetLeadingComments("x", []string{"bad\nline"}))
	// refuseWrite already compared the whole document; these name the two
	// comments the refusals must not have touched.
	out := string(doc.Bytes())
	if !strings.Contains(out, "# above") || !strings.Contains(out, "# beside") {
		t.Errorf("a refused comment write disturbed the comments already there: %q", out)
	}
}

// The rule is the lexer's, so everything the lexer accepts in a comment is
// still writable: a tab, and any non-ASCII text.
//
// Fails if the refusal ever grows past what TOML forbids.
func TestSetComment_AcceptsWhatACommentMayCarry(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"tab", "before\tafter"},
		{"non-ASCII", "café — naïve 日本語"},
		{"emoji", "shipped 🚀"},
		{"a hash inside", "see # 12"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseOrFail(t, "x = 1\n")
			if err := doc.SetComment("x", tc.text); err != nil {
				t.Fatalf("SetComment(%q) refused: %v", tc.text, err)
			}
			if err := doc.SetLeadingComments("x", []string{tc.text}); err != nil {
				t.Fatalf("SetLeadingComments(%q) refused: %v", tc.text, err)
			}
			out := doc.Bytes()
			if _, err := Parse(out); err != nil {
				t.Fatalf("the document no longer parses: %v\n%s", err, out)
			}
		})
	}
}

// Whatever a comment write accepts, the document it produces must parse -- the
// property the refusal exists to hold.
//
// Fails if an accepted comment write ever produces bytes the parser refuses.
func TestCommentWritesKeepTheDocumentParseable(t *testing.T) {
	texts := []string{"note", "", "with\ttab", "unicode ✓", "note " + badUTF8, "two\nlines", "a\x00b"}
	for _, text := range texts {
		doc := parseOrFail(t, "x = 1\n[t]\ny = 2\n")
		errInline := doc.SetComment("x", text)
		errLead := doc.SetLeadingComments("t", []string{text})
		out := doc.Bytes()
		if _, err := Parse(out); err != nil {
			t.Errorf("text %q: inline err=%v, leading err=%v, and the document does not parse: %v\n%s",
				text, errInline, errLead, err, out)
		}
		if errInline == nil && strings.Contains(text, "\n") {
			t.Errorf("text %q: a newline was accepted into an inline comment", text)
		}
	}
}
