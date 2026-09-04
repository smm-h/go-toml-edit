package tomledit

import (
	"strings"
	"testing"
)

// TestArrayComments_RoundTripInlineComments verifies that parsing an array with
// inline comments on elements and round-tripping via Bytes() preserves them
// exactly (clean path via raw bytes).
func TestArrayComments_RoundTripInlineComments(t *testing.T) {
	input := `arr = [
    1, # first
    2, # second
    3,
]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := string(doc.Bytes())
	if got != input {
		t.Errorf("round-trip mismatch\ngot:\n%s\nwant:\n%s", got, input)
	}
}

// TestArrayComments_RoundTripLeadingComments verifies round-trip of arrays
// with standalone comment lines between elements.
func TestArrayComments_RoundTripLeadingComments(t *testing.T) {
	input := `arr = [
    1,
    # between
    2,
    3,
]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := string(doc.Bytes())
	if got != input {
		t.Errorf("round-trip mismatch\ngot:\n%s\nwant:\n%s", got, input)
	}
}

// TestArrayComments_SetPreservesOtherComments sets an array element and
// verifies that comments on other elements survive re-rendering.
func TestArrayComments_SetPreservesOtherComments(t *testing.T) {
	input := `arr = [
    1, # first
    # between
    2, # second
    3,
]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Set arr[1] = 42 (the element with "# second" inline comment)
	if err := doc.Set("arr[1]", 42); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got := string(doc.Bytes())

	// The inline comment "# first" on the first element should survive.
	if !strings.Contains(got, "# first") {
		t.Errorf("lost '# first' comment\ngot:\n%s", got)
	}

	// The standalone "# between" comment should survive.
	if !strings.Contains(got, "# between") {
		t.Errorf("lost '# between' comment\ngot:\n%s", got)
	}

	// The replaced element value should be 42.
	if !strings.Contains(got, "42") {
		t.Errorf("missing new value 42\ngot:\n%s", got)
	}

	// The output must be valid TOML.
	doc2, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("re-parse failed: %v\noutput:\n%s", err, got)
	}

	// Verify the semantic value is correct.
	node, ok := doc2.Lookup("arr[1]")
	if !ok {
		t.Fatalf("Get arr[1]: not found")
	}
	if node.(Scalar).Value() != int64(42) {
		t.Errorf("arr[1] = %v, want 42", node.(Scalar).Value())
	}
}

// TestArrayComments_FormatPreservesComments parses a multi-line array with
// comments, formats it, and verifies comments appear in the output.
func TestArrayComments_FormatPreservesComments(t *testing.T) {
	input := `arr = [
    1, # first
    # a comment
    2, # second
    3,
]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Force array dirty by setting element 2 (third element, index 2).
	if err := doc.Set("arr[2]", 99); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got := string(doc.Bytes())

	if !strings.Contains(got, "# first") {
		t.Errorf("lost '# first' comment\ngot:\n%s", got)
	}
	if !strings.Contains(got, "# a comment") {
		t.Errorf("lost '# a comment' comment\ngot:\n%s", got)
	}
	if !strings.Contains(got, "# second") {
		t.Errorf("lost '# second' comment\ngot:\n%s", got)
	}
	if !strings.Contains(got, "99") {
		t.Errorf("missing new value 99\ngot:\n%s", got)
	}

	// Must be valid TOML.
	if _, err := Parse([]byte(got)); err != nil {
		t.Fatalf("re-parse failed: %v\noutput:\n%s", err, got)
	}
}

// TestArrayComments_SetPreservesInlineOnUnchanged sets one element and verifies
// that inline comments on the unchanged elements are preserved.
func TestArrayComments_SetPreservesInlineOnUnchanged(t *testing.T) {
	input := `arr = [
    "a", # alpha
    "b", # beta
    "c", # gamma
]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Change the middle element only.
	if err := doc.Set("arr[1]", "B"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got := string(doc.Bytes())

	// Comments on unchanged elements must survive.
	if !strings.Contains(got, "# alpha") {
		t.Errorf("lost '# alpha' comment\ngot:\n%s", got)
	}
	if !strings.Contains(got, "# gamma") {
		t.Errorf("lost '# gamma' comment\ngot:\n%s", got)
	}

	// The changed element's value should be "B".
	if !strings.Contains(got, `"B"`) {
		t.Errorf("missing new value \"B\"\ngot:\n%s", got)
	}

	// Must be valid TOML.
	doc2, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("re-parse failed: %v\noutput:\n%s", err, got)
	}
	node, ok := doc2.Lookup("arr[1]")
	if !ok {
		t.Fatalf("Get arr[1]: not found")
	}
	if node.(Scalar).Value() != "B" {
		t.Errorf("arr[1] = %v, want \"B\"", node.(Scalar).Value())
	}
}

// TestArrayComments_TrailingCommentBeforeClose verifies that a comment after the
// last element but before ']' is preserved.
func TestArrayComments_TrailingCommentBeforeClose(t *testing.T) {
	input := `arr = [
    1,
    2,
    # trailing note
]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Clean round-trip.
	got := string(doc.Bytes())
	if got != input {
		t.Errorf("clean round-trip mismatch\ngot:\n%s\nwant:\n%s", got, input)
	}

	// Dirty round-trip: modify an element.
	if err := doc.Set("arr[0]", 10); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got = string(doc.Bytes())
	if !strings.Contains(got, "# trailing note") {
		t.Errorf("lost '# trailing note' comment\ngot:\n%s", got)
	}

	// Must be valid TOML.
	if _, err := Parse([]byte(got)); err != nil {
		t.Fatalf("re-parse failed: %v\noutput:\n%s", err, got)
	}
}

// TestArrayComments_InlineOnlyNoLeading tests an array where elements only have
// inline comments and no leading standalone comments.
func TestArrayComments_InlineOnlyNoLeading(t *testing.T) {
	input := `arr = [
    10, # ten
    20, # twenty
]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Set the first element.
	if err := doc.Set("arr[0]", 100); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got := string(doc.Bytes())

	// Second element's inline comment should survive.
	if !strings.Contains(got, "# twenty") {
		t.Errorf("lost '# twenty' comment\ngot:\n%s", got)
	}

	if !strings.Contains(got, "100") {
		t.Errorf("missing new value 100\ngot:\n%s", got)
	}

	// Must be valid TOML.
	if _, err := Parse([]byte(got)); err != nil {
		t.Fatalf("re-parse failed: %v\noutput:\n%s", err, got)
	}
}

// TestArrayComments_SimpleArrayNoComments ensures that arrays without comments
// still render in compact form.
func TestArrayComments_SimpleArrayNoComments(t *testing.T) {
	input := "arr = [1, 2, 3]\n"

	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Set an element to force dirty.
	if err := doc.Set("arr[0]", 10); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got := string(doc.Bytes())

	// Should still be compact (single line), not multiline.
	if strings.Count(got, "\n") > 1 {
		t.Errorf("expected compact array, got multiline:\n%s", got)
	}

	if !strings.Contains(got, "[10, 2, 3]") {
		t.Errorf("unexpected output:\n%s", got)
	}
}
