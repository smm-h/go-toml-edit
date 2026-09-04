package tomledit

import (
	"errors"
	"math"
	"testing"
)

// The Set-equality contract: Set and SetCreate are a no-op if and only if the
// bytes they would write for the value are exactly the value fragment's bytes
// already stored. One rule, no special cases -- NaN spellings, infinities,
// signed zeros, integer-versus-float, date-time offsets, string quoting and
// integer bases alike.

// valueNodeAt returns the node the path names, for the identity comparison that
// tells a no-op from a write: a write replaces the node, a no-op leaves the one
// the parse produced in place.
func valueNodeAt(t *testing.T, doc *Document, path string) Node {
	t.Helper()
	n, ok := doc.Lookup(path)
	if !ok {
		t.Fatalf("nothing resolves at %q", path)
	}
	return n
}

// Fails if Set stops recognising that it would write the bytes already there --
// the node would be replaced, its lexeme and span thrown away, and the document
// marked as edited for a write that changes nothing.
func TestSetSameBytesIsANoOp(t *testing.T) {
	cases := []struct {
		src   string
		path  string
		value any
	}{
		{"x = 1\n", "x", 1},
		{"x = -1\n", "x", -1},
		{"x = 1.5\n", "x", 1.5},
		{"x = -0.0\n", "x", math.Copysign(0, -1)},
		{"x = inf\n", "x", math.Inf(1)},
		{"x = -inf\n", "x", math.Inf(-1)},
		{"x = nan\n", "x", math.NaN()},
		{"x = true\n", "x", true},
		{"x = \"hi\"\n", "x", "hi"},
		{"x = 1979-05-27T07:32:00Z\n", "x", mustTime(t, "1979-05-27T07:32:00Z")},
		{"x = 1979-05-27\n", "x", LocalDate{Year: 1979, Month: 5, Day: 27}},
		{"a = [1, 2]\n", "a[0]", 1},
		{"t = {a = 1}\n", "t", map[string]any{"a": 1}},
	}
	for _, tc := range cases {
		doc, err := Parse([]byte(tc.src))
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		before := valueNodeAt(t, doc, tc.path)
		if err := doc.Set(tc.path, tc.value); err != nil {
			t.Fatalf("Set(%q, %v) on %q: %v", tc.path, tc.value, tc.src, err)
		}
		if after := valueNodeAt(t, doc, tc.path); after != before {
			t.Errorf("Set(%q, %v) on %q replaced the node: the write was not recognised as a no-op",
				tc.path, tc.value, tc.src)
		}
		if doc.subtreeDirty() {
			t.Errorf("Set(%q, %v) on %q marked the document as edited", tc.path, tc.value, tc.src)
		}
		if got := string(doc.Bytes()); got != tc.src {
			t.Errorf("Set(%q, %v) on %q rendered %q", tc.path, tc.value, tc.src, got)
		}
	}
}

// Fails if Set stops writing when the bytes differ -- a value spelled another
// way is normalised on first touch, and only then.
func TestSetDifferentBytesWritesAndThenSettles(t *testing.T) {
	cases := []struct {
		src   string
		value any
		want  string
	}{
		{"x = 0x2A\n", 42, "x = 42\n"},
		{"x = 1_000\n", 1000, "x = 1000\n"},
		{"x = 'lit'\n", "lit", "x = \"lit\"\n"},
		{"x = +1.0\n", 1.0, "x = 1.0\n"},
		{"x = 1\n", 1.0, "x = 1.0\n"},
		{"x = 1.0\n", 1, "x = 1\n"},
		{"x = 0.0\n", math.Copysign(0, -1), "x = -0.0\n"},
		{"x = 1979-05-27T00:32:00-07:00\n", mustTime(t, "1979-05-27T07:32:00Z"), "x = 1979-05-27T07:32:00Z\n"},
	}
	for _, tc := range cases {
		doc, err := Parse([]byte(tc.src))
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		if err := doc.Set("x", tc.value); err != nil {
			t.Fatalf("Set on %q: %v", tc.src, err)
		}
		got := string(doc.Bytes())
		if got != tc.want {
			t.Errorf("Set(%v) on %q rendered %q, want %q", tc.value, tc.src, got, tc.want)
		}
		// Idempotent from here: the same write again is a no-op.
		before := valueNodeAt(t, doc, "x")
		if err := doc.Set("x", tc.value); err != nil {
			t.Fatalf("second Set on %q: %v", tc.src, err)
		}
		if after := valueNodeAt(t, doc, "x"); after != before {
			t.Errorf("the second Set(%v) on %q wrote again instead of settling", tc.value, tc.src)
		}
		if again := string(doc.Bytes()); again != got {
			t.Errorf("the second Set(%v) on %q rendered %q, want %q", tc.value, tc.src, again, got)
		}
	}
}

// Fails if a container-valued Set stops replacing the container wholesale, or
// stops recognising the case where it would write what is already there.
func TestSetContainerReplacesWholesale(t *testing.T) {
	doc, err := Parse([]byte("t = { a = 1, b = 2 } # keep\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := doc.Set("t", []Pair{{Key: "a", Value: 1}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	const want = "t = {a = 1} # keep\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
	before := valueNodeAt(t, doc, "t")
	if err := doc.Set("t", []Pair{{Key: "a", Value: 1}}); err != nil {
		t.Fatalf("second Set: %v", err)
	}
	if after := valueNodeAt(t, doc, "t"); after != before {
		t.Errorf("the identical container write replaced the container again")
	}
}

// Fails if a NaN carrying a sign bit stops being refused: TOML has one NaN
// spelling and the sign the caller passed would be dropped in silence.
func TestSetRefusesSignBitNaN(t *testing.T) {
	negNaN := math.Float64frombits(math.Float64bits(math.NaN()) | (1 << 63))
	doc, err := Parse([]byte("x = 1\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var diag *Error
	if err := doc.Set("x", negNaN); !errors.As(err, &diag) || diag.Kind != KindBadInput {
		t.Fatalf("Set returned %v, want a KindBadInput diagnostic", err)
	}
	if got := string(doc.Bytes()); got != "x = 1\n" {
		t.Errorf("the refused write changed the document: %q", got)
	}
	// The accepted NaN writes the one spelling.
	if err := doc.Set("x", math.NaN()); err != nil {
		t.Fatalf("Set(NaN): %v", err)
	}
	if got := string(doc.Bytes()); got != "x = nan\n" {
		t.Errorf("Set(NaN) rendered %q, want \"x = nan\\n\"", got)
	}
}

// Fails if a no-op Set starts clearing dirtiness an earlier edit recorded: the
// no-op decides not to write, which is not the same as undoing a write.
func TestNoOpSetKeepsEarlierEdits(t *testing.T) {
	doc, err := Parse([]byte("x = 1 # old\ny = 2\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := doc.SetComment("x", "new"); err != nil {
		t.Fatalf("SetComment: %v", err)
	}
	if err := doc.Set("x", 1); err != nil {
		t.Fatalf("Set: %v", err)
	}
	const want = "x = 1 # new\ny = 2\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("rendered %q, want %q", got, want)
	}
}

// Fails if Delete stops being a silent no-op on a path the document does not
// carry -- an ensure-absent loop calls it unconditionally.
func TestDeleteMissingPathStaysASilentNoOp(t *testing.T) {
	const src = "x = 1\n[t]\ny = 2\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, path := range []string{"nope", "t.nope", "nope.deeper", "x.nope"} {
		if err := doc.Delete(path); err != nil {
			t.Errorf("Delete(%q) reported %v, want a silent no-op", path, err)
		}
	}
	if got := string(doc.Bytes()); got != src {
		t.Errorf("the no-op deletes changed the document: %q", got)
	}
}
