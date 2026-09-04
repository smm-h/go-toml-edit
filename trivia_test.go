package tomledit

import "testing"

// Blank lines are part of what a document says about itself: they separate one
// construct from the next, and a library that claims byte-exact round-trips
// cannot drop them. These tests pin both halves -- the untouched round-trip and
// the re-render of a node whose bytes are no longer valid.

// Fails if a blank line the parser cannot attach to a following construct is
// dropped: trivia at end of input reaches the document as comment nodes, and
// the blank lines between and before those comments have to arrive with them.
func TestTrivia_BlankLinesRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"blank-before-trailing-comment", "[a]\nx = 1\n\n# trailing\n"},
		{"blanks-between-trailing-comments", "x = 1\n\n#a\n\n#b\n"},
		{"blank-run-before-trailing-comment", "x = 1\n\n\n# trailing\n"},
		{"blank-after-trailing-comment", "x = 1\n# trailing\n\n"},
		{"blank-before-trailing-comment-in-table", "[a]\nx = 1\n\n# one\n# two\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := string(doc.Bytes()); got != tc.src {
				t.Errorf("round-trip lost bytes:\n  src: %q\n  got: %q", tc.src, got)
			}
		})
	}
}

// Fails if the blank-line runs in a node's leading trivia stop being emitted
// when the node re-renders: an edit that dirties a construct must not silently
// pull it up against the construct above it.
func TestTrivia_BlankLinesSurviveReRender(t *testing.T) {
	cases := []struct {
		name string
		src  string
		edit func(*Document) error
		want string
	}{
		{
			name: "blank-before-pair",
			src:  "x = 1\n\ny = 2\n",
			edit: func(d *Document) error { return d.Set("y", 3) },
			want: "x = 1\n\ny = 3\n",
		},
		{
			name: "blank-run-before-pair",
			src:  "x = 1\n\n\n\ny = 2\n",
			edit: func(d *Document) error { return d.Set("y", 3) },
			want: "x = 1\n\n\n\ny = 3\n",
		},
		{
			name: "blank-before-leading-comment",
			src:  "x = 1\n\n# c\ny = 2\n",
			edit: func(d *Document) error { return d.Set("y", 3) },
			want: "x = 1\n\n# c\ny = 3\n",
		},
		{
			name: "blank-after-leading-comment",
			src:  "x = 1\n# c\n\ny = 2\n",
			edit: func(d *Document) error { return d.Set("y", 3) },
			want: "x = 1\n# c\n\ny = 3\n",
		},
		{
			name: "blank-between-leading-comments",
			src:  "x = 1\n# one\n\n# two\ny = 2\n",
			edit: func(d *Document) error { return d.Set("y", 3) },
			want: "x = 1\n# one\n\n# two\ny = 3\n",
		},
		{
			name: "blank-before-table-header",
			src:  "[a]\nx = 1\n\n[b]\ny = 2\n",
			edit: func(d *Document) error { return d.SetComment("b", "hi") },
			want: "[a]\nx = 1\n\n[b] # hi\ny = 2\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if err := tc.edit(doc); err != nil {
				t.Fatalf("edit: %v", err)
			}
			if got := string(doc.Bytes()); got != tc.want {
				t.Errorf("re-render lost the blank-line separation:\n  got:  %q\n  want: %q", got, tc.want)
			}
		})
	}
}
