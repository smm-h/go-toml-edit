package tomledit

import (
	"testing"
)

// The Diff semantics ruled by the design record, pinned here so a later change
// to leaf collection or value comparison cannot move them silently:
//
//   - an integer and a float never compare equal, whatever their magnitudes;
//   - two spellings of one value are not a difference -- integer bases and
//     underscores, float spellings, equivalent date-time offsets, string
//     quoting styles, and the structural spellings of one logical shape.

func diffDocs(t *testing.T, a, b string) []Change {
	t.Helper()
	da, err := Parse([]byte(a))
	if err != nil {
		t.Fatalf("parsing the first document: %v", err)
	}
	db, err := Parse([]byte(b))
	if err != nil {
		t.Fatalf("parsing the second document: %v", err)
	}
	return Diff(da, db)
}

// Fails if Diff starts treating an integer and a float as one value: a
// document holding 1 and a document holding 1.0 must report a modification.
func TestDiffPin_IntegerAndFloatNeverCompareEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b string
	}{
		{"one", "x = 1\n", "x = 1.0\n"},
		{"zero", "x = 0\n", "x = 0.0\n"},
		{"negative", "x = -3\n", "x = -3.0\n"},
		{"large", "x = 1000\n", "x = 1e3\n"},
		{"float first", "x = 2.0\n", "x = 2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changes := diffDocs(t, tc.a, tc.b)
			if len(changes) != 1 {
				t.Fatalf("Diff reported %d changes, want 1: %v", len(changes), changes)
			}
			if changes[0].Kind != Modified || changes[0].Path != "x" {
				t.Fatalf("Diff reported %v at %q, want a modification at \"x\"",
					changes[0].Kind, changes[0].Path)
			}
		})
	}
}

// Fails if Diff starts reporting a difference between two spellings of the
// same value: the leaf comparison reads semantic values, never lexemes.
func TestDiffPin_SpellingPairsAreNotDifferences(t *testing.T) {
	cases := []struct {
		name string
		a, b string
	}{
		{"integer hex", "x = 0x2A\n", "x = 42\n"},
		{"integer octal", "x = 0o52\n", "x = 42\n"},
		{"integer binary", "x = 0b101010\n", "x = 42\n"},
		{"integer underscores", "x = 1_000\n", "x = 1000\n"},
		{"integer plus sign", "x = +7\n", "x = 7\n"},
		{"float exponent", "x = 1e3\n", "x = 1000.0\n"},
		{"float exponent case", "x = 1E3\n", "x = 1.0e+3\n"},
		{"float underscores", "x = 1_000.5\n", "x = 1000.5\n"},
		{"offset date-time zone", "x = 1979-05-27T00:32:00-07:00\n", "x = 1979-05-27T07:32:00Z\n"},
		{"offset date-time case", "x = 1979-05-27t07:32:00z\n", "x = 1979-05-27T07:32:00Z\n"},
		{"offset date-time fraction", "x = 1979-05-27T07:32:00.000Z\n", "x = 1979-05-27T07:32:00Z\n"},
		{"string quoting", "x = \"a\"\n", "x = 'a'\n"},
		{"string multi-line", "x = \"\"\"a\"\"\"\n", "x = \"a\"\n"},
		{"string escape", "x = \"\\u0041\"\n", "x = \"A\"\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if changes := diffDocs(t, tc.a, tc.b); len(changes) != 0 {
				t.Fatalf("Diff reported %v, want no differences", changes)
			}
		})
	}
}

// Fails if Diff starts distinguishing the structural spellings of one logical
// shape: an array-of-tables and an inline array of inline tables hold the same
// values at the same paths, and so do a header table, an inline table and a
// dotted key.
func TestDiffPin_StructuralSpellingsAreNotDifferences(t *testing.T) {
	cases := []struct {
		name string
		a, b string
	}{
		{
			"array-of-tables against an inline array",
			"[[p]]\nname = \"one\"\n\n[[p]]\nname = \"two\"\n",
			"p = [{name = \"one\"}, {name = \"two\"}]\n",
		},
		{
			"header table against an inline table",
			"[a]\nb = 1\n",
			"a = {b = 1}\n",
		},
		{
			"header table against a dotted key",
			"[a]\nb = 1\n",
			"a.b = 1\n",
		},
		{
			"nested header against a dotted key",
			"[a.b]\nc = 1\n",
			"[a]\nb.c = 1\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if changes := diffDocs(t, tc.a, tc.b); len(changes) != 0 {
				t.Fatalf("Diff reported %v, want no differences", changes)
			}
		})
	}
}

// Fails if a document stops comparing equal to itself -- the property every
// other pin rests on, and the one a value comparison that refuses to call a
// value equal to itself (a not-a-number, for instance) breaks first.
func TestDiffPin_EveryCorpusShapeEqualsItself(t *testing.T) {
	sources := []string{
		"x = 1\ny = 2.5\nz = true\n",
		"x = nan\ny = inf\nz = -inf\n",
		"t = 1979-05-27T07:32:00Z\nd = 1979-05-27\nlt = 07:32:00\nldt = 1979-05-27T07:32:00\n",
		"a = [1, 2, 3]\nb = [\"x\", \"y\"]\n",
		"[[p]]\nname = \"one\"\n\n[[p]]\nname = \"two\"\n",
		"[a]\nb = {c = 1, d = [2, 3]}\n",
	}
	for _, src := range sources {
		if changes := diffDocs(t, src, src); len(changes) != 0 {
			t.Errorf("a document differs from itself: %v\nsource:\n%s", changes, src)
		}
	}
}
