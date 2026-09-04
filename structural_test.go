package tomledit

import (
	"errors"
	"strings"
	"testing"
)

// The structural operations: reordering a container's children, and appending
// to or removing from an array.

// --- PermuteChildren ---

// Fails if the gather semantics change: order[i] names the child that moves to
// position i, so the document's own children come out in exactly that order.
func TestPermuteChildren_ReordersDocumentChildren(t *testing.T) {
	doc := parseOrFail(t, "[b]\nx = 1\n[a]\ny = 2\n[c]\nz = 3\n")
	if err := doc.PermuteChildren("", []int{1, 0, 2}); err != nil {
		t.Fatalf("PermuteChildren: %v", err)
	}
	const want = "[a]\ny = 2\n[b]\nx = 1\n[c]\nz = 3\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads\n%q\nwant\n%q", got, want)
	}
	mustFold(t, doc)
}

// Fails if a reordered child stops bringing its trivia along: the comments and
// the blank line written above a table belong to that table, not to the
// position it happened to hold.
func TestPermuteChildren_CommentsTravelWithTheirNodes(t *testing.T) {
	const src = "# about b\n[b]\nx = 1\n\n# about a\n[a]\ny = 2\n"
	doc := parseOrFail(t, src)
	children := doc.Children
	if len(children) != 2 {
		t.Fatalf("the document holds %d children, want 2: %#v", len(children), children)
	}
	if err := doc.PermuteChildren("", []int{1, 0}); err != nil {
		t.Fatalf("PermuteChildren: %v", err)
	}
	got := string(doc.Bytes())
	if !strings.Contains(got, "# about a\n[a]") || !strings.Contains(got, "# about b\n[b]") {
		t.Errorf("a comment lost its node:\n%s", got)
	}
	if strings.Index(got, "[a]") > strings.Index(got, "[b]") {
		t.Errorf("the children were not reordered:\n%s", got)
	}
	if _, err := Parse([]byte(got)); err != nil {
		t.Fatalf("the result is not valid TOML: %v\n%s", err, got)
	}
}

// Fails if reordering a table's own key-value pairs stops working, or starts
// rewriting the table header that still describes them.
func TestPermuteChildren_ReordersTableChildren(t *testing.T) {
	doc := parseOrFail(t, "[t]\nb = 2\na = 1\n")
	if err := doc.PermuteChildren("t", []int{1, 0}); err != nil {
		t.Fatalf("PermuteChildren: %v", err)
	}
	const want = "[t]\na = 1\nb = 2\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
}

// Fails if an array stops re-rendering after its elements move: an array is
// one value fragment, so its stored bytes no longer describe its contents.
func TestPermuteChildren_ReordersArrayElements(t *testing.T) {
	doc := parseOrFail(t, "items = [1, 2, 3]\n")
	if err := doc.PermuteChildren("items", []int{2, 0, 1}); err != nil {
		t.Fatalf("PermuteChildren: %v", err)
	}
	const want = "items = [3, 1, 2]\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
}

// Fails if an inline table stops re-rendering after its pairs move.
func TestPermuteChildren_ReordersInlineTablePairs(t *testing.T) {
	doc := parseOrFail(t, "t = { b = 2, a = 1 }\n")
	if err := doc.PermuteChildren("t", []int{1, 0}); err != nil {
		t.Fatalf("PermuteChildren: %v", err)
	}
	const want = "t = {a = 1, b = 2}\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
}

// Fails if the entries of an array-of-tables stop being reorderable: they are
// children of the document, and a document-level permutation moves them.
func TestPermuteChildren_ReordersArrayOfTablesEntries(t *testing.T) {
	doc := parseOrFail(t, "[[s]]\nn = 1\n[[s]]\nn = 2\n")
	if err := doc.PermuteChildren("", []int{1, 0}); err != nil {
		t.Fatalf("PermuteChildren: %v", err)
	}
	root := doc.Root()
	e, _ := root.Get("s")
	records, _ := e.Records()
	if len(records) != 2 {
		t.Fatalf("the collection holds %d entries, want 2", len(records))
	}
	first, _ := records[0].Get("n")
	node, _ := first.Node()
	if n, ok := node.(*IntegerNode); !ok || n.Val != 2 {
		t.Errorf("the first entry reads %q, want the one that was second", doc.Bytes())
	}
}

// Fails if a permutation stops being total: a shorter or longer one leaves the
// caller's intent for the unnamed children unstated, and is refused.
func TestPermuteChildren_RefusesAPartialPermutation(t *testing.T) {
	doc := parseOrFail(t, "a = 1\nb = 2\nc = 3\n")
	err := doc.PermuteChildren("", []int{1, 0})
	if !errors.Is(err, ErrBadInput) {
		t.Errorf("a two-entry permutation of three children reported %v, want a bad-input diagnostic", err)
	}
	if got := string(doc.Bytes()); got != "a = 1\nb = 2\nc = 3\n" {
		t.Errorf("a refused permutation reordered the document: %q", got)
	}
}

// Fails if an out-of-range index is applied, or reported without naming
// itself.
func TestPermuteChildren_RefusesAnOutOfRangeIndex(t *testing.T) {
	doc := parseOrFail(t, "a = 1\nb = 2\n")
	for _, order := range [][]int{{0, 2}, {-1, 0}} {
		err := doc.PermuteChildren("", order)
		if !errors.Is(err, ErrBadInput) {
			t.Fatalf("permutation %v reported %v, want a bad-input diagnostic", order, err)
		}
		var diag *Error
		if errors.As(err, &diag) && diag.Value == nil {
			t.Errorf("permutation %v reported a diagnostic naming no index: %v", order, diag)
		}
	}
	if got := string(doc.Bytes()); got != "a = 1\nb = 2\n" {
		t.Errorf("a refused permutation reordered the document: %q", got)
	}
}

// Fails if a repeated index is applied: it would drop one child and duplicate
// another, which is not a permutation.
func TestPermuteChildren_RefusesARepeatedIndex(t *testing.T) {
	doc := parseOrFail(t, "a = 1\nb = 2\n")
	err := doc.PermuteChildren("", []int{0, 0})
	if !errors.Is(err, ErrBadInput) {
		t.Errorf("a repeated index reported %v, want a bad-input diagnostic", err)
	}
	if got := string(doc.Bytes()); got != "a = 1\nb = 2\n" {
		t.Errorf("a refused permutation reordered the document: %q", got)
	}
}

// Fails if a path no single node stands for stops being refused: a collection
// and a table another construct only implied have no children of their own.
func TestPermuteChildren_RefusesALogicalOnlyPath(t *testing.T) {
	doc := parseOrFail(t, "[[s]]\nn = 1\n[[s]]\nn = 2\n")
	if err := doc.PermuteChildren("s", []int{1, 0}); !errors.Is(err, ErrWrongContainer) {
		t.Errorf("PermuteChildren on an array-of-tables reported %v, want a wrong-container diagnostic", err)
	}

	implied := parseOrFail(t, "[a.b]\nx = 1\n")
	if err := implied.PermuteChildren("a", []int{}); !errors.Is(err, ErrWrongContainer) {
		t.Errorf("PermuteChildren on an implied table reported %v, want a wrong-container diagnostic", err)
	}
}

// Fails if a scalar stops being refused as a container.
func TestPermuteChildren_RefusesAScalar(t *testing.T) {
	doc := parseOrFail(t, "x = 1\n")
	if err := doc.PermuteChildren("x", []int{}); !errors.Is(err, ErrWrongContainer) {
		t.Errorf("PermuteChildren on a scalar reported %v, want a wrong-container diagnostic", err)
	}
}

// Fails if an empty permutation of an empty container stops being the no-op it
// is: the identity of a container with no children is total by construction.
func TestPermuteChildren_EmptyContainerTakesTheEmptyOrder(t *testing.T) {
	doc := parseOrFail(t, "items = []\n")
	if err := doc.PermuteChildren("items", nil); err != nil {
		t.Errorf("PermuteChildren on an empty array reported %v, want nil", err)
	}
}

// --- AppendToArray ---

// Fails if appending stops converting its value the way Set does, or stops
// re-rendering the array.
func TestAppendToArray_AppendsAValue(t *testing.T) {
	doc := parseOrFail(t, "items = [1, 2]\n")
	if err := doc.AppendToArray("items", 3); err != nil {
		t.Fatalf("AppendToArray: %v", err)
	}
	if err := doc.AppendToArray("items", "four"); err != nil {
		t.Fatalf("AppendToArray: %v", err)
	}
	const want = "items = [1, 2, 3, \"four\"]\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
}

// Fails if appending to an empty array stops working.
func TestAppendToArray_AppendsToAnEmptyArray(t *testing.T) {
	doc := parseOrFail(t, "items = []\n")
	if err := doc.AppendToArray("items", true); err != nil {
		t.Fatalf("AppendToArray: %v", err)
	}
	if got := string(doc.Bytes()); got != "items = [true]\n" {
		t.Errorf("the document reads %q", got)
	}
}

// Fails if an unconvertible value stops being refused before it reaches the
// array.
func TestAppendToArray_RefusesAnUnconvertibleValue(t *testing.T) {
	doc := parseOrFail(t, "items = [1]\n")
	if err := doc.AppendToArray("items", struct{ X int }{1}); !errors.Is(err, ErrBadInput) {
		t.Errorf("AppendToArray of an unsupported type reported %v, want a bad-input diagnostic", err)
	}
	if got := string(doc.Bytes()); got != "items = [1]\n" {
		t.Errorf("a refused append changed the document: %q", got)
	}
}

// Fails if an array-of-tables starts answering as an array value: its entries
// are tables, added with NewArrayTable.
func TestAppendToArray_RefusesAnArrayOfTables(t *testing.T) {
	doc := parseOrFail(t, "[[s]]\nn = 1\n")
	err := doc.AppendToArray("s", 2)
	if !errors.Is(err, ErrWrongContainer) {
		t.Errorf("AppendToArray on an array-of-tables reported %v, want a wrong-container diagnostic", err)
	}
	if !strings.Contains(err.Error(), "NewArrayTable") {
		t.Errorf("the refusal does not name the operation that does add an entry: %v", err)
	}
}

// Fails if a path naming something that is not an array stops being refused.
func TestAppendToArray_RefusesANonArray(t *testing.T) {
	doc := parseOrFail(t, "x = 1\n[t]\ny = 2\n")
	if err := doc.AppendToArray("x", 2); !errors.Is(err, ErrWrongContainer) {
		t.Errorf("AppendToArray on a scalar reported %v, want a wrong-container diagnostic", err)
	}
	if err := doc.AppendToArray("t", 2); !errors.Is(err, ErrWrongContainer) {
		t.Errorf("AppendToArray on a table reported %v, want a wrong-container diagnostic", err)
	}
	if err := doc.AppendToArray("missing", 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("AppendToArray on a missing path reported %v, want a not-found diagnostic", err)
	}
}

// --- the ordered inline-table input ---

// Fails if a []Pair stops writing its keys in the order given -- the whole
// reason it exists beside a map, whose keys are written sorted.
func TestPair_WritesKeysInOrder(t *testing.T) {
	doc := parseOrFail(t, "x = 1\n")
	if err := doc.Set("x", []Pair{{"zulu", 1}, {"alpha", 2}, {"mike", 3}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	const want = "x = {zulu = 1, alpha = 2, mike = 3}\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
}

// Fails if a key repeated in the input reaches the document: an inline table
// carrying one key twice is not valid TOML.
func TestPair_RefusesADuplicateKey(t *testing.T) {
	doc := parseOrFail(t, "x = 1\n")
	err := doc.Set("x", []Pair{{"a", 1}, {"a", 2}})
	if !errors.Is(err, ErrBadInput) {
		t.Errorf("Set with a duplicate key reported %v, want a bad-input diagnostic", err)
	}
	if got := string(doc.Bytes()); got != "x = 1\n" {
		t.Errorf("a refused write changed the document: %q", got)
	}
}

// Fails if a key that cannot be written as TOML is accepted: no quoting makes
// invalid UTF-8 a key.
func TestPair_RefusesAKeyThatIsNotUTF8(t *testing.T) {
	doc := parseOrFail(t, "x = 1\n")
	err := doc.Set("x", []Pair{{string([]byte{0xff}), 1}})
	if !errors.Is(err, ErrBadInput) {
		t.Errorf("Set with an invalid key reported %v, want a bad-input diagnostic", err)
	}
}

// Fails if a Pair's key stops being taken verbatim: it is one key, quoted when
// it has to be, and never a path into a nested table.
func TestPair_KeyIsASingleKeyNotAPath(t *testing.T) {
	doc := parseOrFail(t, "x = 1\n")
	if err := doc.Set("x", []Pair{{"a.b", 1}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	const want = "x = {\"a.b\" = 1}\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
	if v, ok := doc.GetInt(`x."a.b"`); !ok || v != 1 {
		t.Errorf("the key is not readable as one key: %q", doc.Bytes())
	}
}

// Fails if []Pair stops nesting, or stops working wherever a value goes.
func TestPair_NestsAndTravels(t *testing.T) {
	doc := parseOrFail(t, "x = 1\nitems = []\n")
	if err := doc.Set("x", []Pair{{"inner", []Pair{{"b", 2}, {"a", 1}}}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := doc.AppendToArray("items", []Pair{{"n", 1}}); err != nil {
		t.Fatalf("AppendToArray: %v", err)
	}
	const want = "x = {inner = {b = 2, a = 1}}\nitems = [{n = 1}]\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
}

// Fails if an empty ordered input stops writing an empty inline table.
func TestPair_EmptyInputWritesAnEmptyTable(t *testing.T) {
	doc := parseOrFail(t, "x = 1\n")
	if err := doc.Set("x", []Pair{}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := string(doc.Bytes()); got != "x = {}\n" {
		t.Errorf("the document reads %q", got)
	}
}

// --- RemoveFromArray ---

// Fails if removing by index stops working, or stops counting a negative index
// from the end.
func TestRemoveFromArray_RemovesByIndex(t *testing.T) {
	doc := parseOrFail(t, "items = [1, 2, 3]\n")
	if err := doc.RemoveFromArray("items", 1); err != nil {
		t.Fatalf("RemoveFromArray: %v", err)
	}
	if got := string(doc.Bytes()); got != "items = [1, 3]\n" {
		t.Errorf("the document reads %q, want the middle element gone", got)
	}
	if err := doc.RemoveFromArray("items", -1); err != nil {
		t.Fatalf("RemoveFromArray(-1): %v", err)
	}
	if got := string(doc.Bytes()); got != "items = [1]\n" {
		t.Errorf("the document reads %q, want the last element gone", got)
	}
}

// Fails if an index outside the array stops being refused: unlike Delete, this
// operation names a position that must exist.
func TestRemoveFromArray_RefusesAnIndexOutsideTheArray(t *testing.T) {
	doc := parseOrFail(t, "items = [1, 2]\n")
	for _, index := range []int{2, -3} {
		if err := doc.RemoveFromArray("items", index); !errors.Is(err, ErrNotFound) {
			t.Errorf("RemoveFromArray(%d) reported %v, want a not-found diagnostic", index, err)
		}
	}
	if err := doc.RemoveFromArray("empty", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("RemoveFromArray on a missing path reported %v, want a not-found diagnostic", err)
	}
	if got := string(doc.Bytes()); got != "items = [1, 2]\n" {
		t.Errorf("a refused removal changed the document: %q", got)
	}
}

// Fails if an array-of-tables starts answering as an array value.
func TestRemoveFromArray_RefusesAnArrayOfTables(t *testing.T) {
	doc := parseOrFail(t, "[[s]]\nn = 1\n")
	if err := doc.RemoveFromArray("s", 0); !errors.Is(err, ErrWrongContainer) {
		t.Errorf("RemoveFromArray on an array-of-tables reported %v, want a wrong-container diagnostic", err)
	}
}
