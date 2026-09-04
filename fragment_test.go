package tomledit

import "testing"

// The fragment contract: a rendered construct decomposes into byte ranges --
// leading trivia, key bytes, separator bytes, value bytes, inline comment,
// trailing newline for a key-value line; brackets, per-part key bytes and the
// dots between them for a header; the brackets, per-element bytes and the
// separators between them for a container. A write invalidates the fragment it
// touched and nothing else, so every other fragment still splices the bytes it
// was written as.

// editedBytes parses src, runs edit against the document and returns what it
// renders.
func editedBytes(t *testing.T, src string, edit func(*Document) error) string {
	t.Helper()
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if err := edit(doc); err != nil {
		t.Fatalf("edit of %q: %v", src, err)
	}
	return string(doc.Bytes())
}

// Fails if a value write stops leaving the rest of its line byte-identical --
// the separator spacing and the inline comment are fragments the write never
// touched.
func TestFragmentValueWriteKeepsLineSpacing(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"x   =    1    # keep me\n", "x   =    7    # keep me\n"},
		{"x=1\n", "x=7\n"},
		{"x\t=\t1\n", "x\t=\t7\n"},
		{"x = 1  \n", "x = 7  \n"},
		{"x = 1", "x = 7"},
	}
	for _, tc := range cases {
		got := editedBytes(t, tc.src, func(d *Document) error { return d.Set("x", 7) })
		if got != tc.want {
			t.Errorf("Set on %q rendered %q, want %q", tc.src, got, tc.want)
		}
	}
}

// Fails if a value write stops preserving how the KEY was written: its quoting,
// and the whitespace around the dots of a dotted key.
func TestFragmentValueWriteKeepsKeySpelling(t *testing.T) {
	cases := []struct {
		src  string
		path string
		want string
	}{
		{"'lit' = 1\n", "lit", "'lit' = 7\n"},
		{"\"quoted\" = 1\n", "quoted", "\"quoted\" = 7\n"},
		{"a . b  =  1\n", "a.b", "a . b  =  7\n"},
		{"a.'b' = 1\n", "a.b", "a.'b' = 7\n"},
	}
	for _, tc := range cases {
		got := editedBytes(t, tc.src, func(d *Document) error { return d.Set(tc.path, 7) })
		if got != tc.want {
			t.Errorf("Set(%q) on %q rendered %q, want %q", tc.path, tc.src, got, tc.want)
		}
	}
}

// Fails if writing a comment starts rewriting the value beside it: a trivia
// write invalidates the trivia fragment and nothing else, so the value keeps
// the base, quoting and spacing it was written with.
func TestFragmentCommentWriteKeepsValueSpelling(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"x = 0x2A\n", "x = 0x2A # note\n"},
		{"x   =   0x2A\n", "x   =   0x2A # note\n"},
		{"x = 'lit'   # old\n", "x = 'lit'   # note\n"},
		{"x = 1_000\n", "x = 1_000 # note\n"},
		{"x = +1.0e2\n", "x = +1.0e2 # note\n"},
	}
	for _, tc := range cases {
		got := editedBytes(t, tc.src, func(d *Document) error { return d.SetComment("x", "note") })
		if got != tc.want {
			t.Errorf("SetComment on %q rendered %q, want %q", tc.src, got, tc.want)
		}
	}
}

// Fails if writing one array element stops leaving every other element's bytes,
// and the separators around them, exactly as written.
func TestFragmentArrayElementIsolation(t *testing.T) {
	cases := []struct {
		src  string
		path string
		want string
	}{
		{"arr = [ 1,2,   3 ] # c\n", "arr[1]", "arr = [ 1,9,   3 ] # c\n"},
		{"arr = [0x1, 0x2]\n", "arr[0]", "arr = [9, 0x2]\n"},
		{"arr = [\n  1,\n  # about two\n  2,\n]\n", "arr[0]", "arr = [\n  9,\n  # about two\n  2,\n]\n"},
	}
	for _, tc := range cases {
		got := editedBytes(t, tc.src, func(d *Document) error { return d.Set(tc.path, 9) })
		if got != tc.want {
			t.Errorf("Set(%q) on %q rendered %q, want %q", tc.path, tc.src, got, tc.want)
		}
	}
}

// Fails if writing one inline-table value stops leaving the braces, the commas
// and every other pair exactly as written.
func TestFragmentInlineTablePairIsolation(t *testing.T) {
	cases := []struct {
		src  string
		path string
		want string
	}{
		{"t = { a = 1,  b = 'x' }\n", "t.a", "t = { a = 9,  b = 'x' }\n"},
		{"t = {a=1,b=2}\n", "t.b", "t = {a=1,b=9}\n"},
		{"t = { 'q' . r = 1 }\n", "t.q.r", "t = { 'q' . r = 9 }\n"},
	}
	for _, tc := range cases {
		got := editedBytes(t, tc.src, func(d *Document) error { return d.Set(tc.path, 9) })
		if got != tc.want {
			t.Errorf("Set(%q) on %q rendered %q, want %q", tc.path, tc.src, got, tc.want)
		}
	}
}

// Fails if a header that re-renders stops splicing the bytes its key parts were
// written as -- the quoting of each part and the whitespace around the dots and
// inside the brackets.
func TestFragmentHeaderKeySplices(t *testing.T) {
	cases := []struct {
		src  string
		path string
		want string
	}{
		{"[ \"a\" . b ]\nx = 1\n", "a.b", "# note\n[ \"a\" . b ]\nx = 1\n"},
		{"['a']\nx = 1\n", "a", "# note\n['a']\nx = 1\n"},
		{"[[ 'a' ]]\nx = 1\n", "a[0]", "# note\n[[ 'a' ]]\nx = 1\n"},
	}
	for _, tc := range cases {
		got := editedBytes(t, tc.src, func(d *Document) error {
			return d.SetLeadingComments(tc.path, []string{"note"})
		})
		if got != tc.want {
			t.Errorf("SetLeadingComments(%q) on %q rendered %q, want %q", tc.path, tc.src, got, tc.want)
		}
	}
}

// Fails if a write inside a nested container stops leaving the containers
// around it spliced -- fragment dirtiness recurses, so a clean sub-fragment
// inside a dirty container still writes its original bytes.
func TestFragmentNestedContainerIsolation(t *testing.T) {
	src := "a = [ {x = 1},  {y = 0x2} ]\n"
	want := "a = [ {x = 9},  {y = 0x2} ]\n"
	got := editedBytes(t, src, func(d *Document) error { return d.Set("a[0].x", 9) })
	if got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}
