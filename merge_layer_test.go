package tomledit

import (
	"strings"
	"testing"
)

// Merge reads the source document through the read-layer. These pin what that
// buys: the source's spelling stops steering the traversal, so a construct is
// merged where it belongs and the comments of every new construct travel with
// it.

func mergeInto(t *testing.T, targetSrc, sourceSrc string) *Document {
	t.Helper()
	target, err := Parse([]byte(targetSrc))
	if err != nil {
		t.Fatalf("parsing the target: %v", err)
	}
	source, err := Parse([]byte(sourceSrc))
	if err != nil {
		t.Fatalf("parsing the source: %v", err)
	}
	if err := target.Merge(source); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	return target
}

// Fails if merging into an inline table starts pulling the source's unrelated
// top-level tables in with it: a scope whose children are the inline table's
// pairs must not be asked what the DOCUMENT's tables belong under.
func TestMergeLayer_InlineTableDoesNotAbsorbTopLevelTables(t *testing.T) {
	target := mergeInto(t,
		"cfg = {a = 1}\n",
		"cfg = {b = 2}\n\n[t]\nx = 1\n")

	if got, err := target.GetInt("cfg.a"); err != nil || got != 1 {
		t.Errorf("cfg.a = %d (%v), want 1", got, err)
	}
	if got, err := target.GetInt("cfg.b"); err != nil || got != 2 {
		t.Errorf("cfg.b = %d (%v), want 2", got, err)
	}
	if got, err := target.GetInt("t.x"); err != nil || got != 1 {
		t.Errorf("t.x = %d (%v), want 1", got, err)
	}
	if target.Has("cfg.t.x") {
		t.Errorf("the source's [t] was merged inside the inline table too:\n%s", target.Bytes())
	}
	if _, err := Parse(target.Bytes()); err != nil {
		t.Fatalf("the merged document does not re-parse: %v\n%s", err, target.Bytes())
	}
}

// Fails if a table that is new in the target arrives without the comments
// written above its header: comments come along with what is new, and a
// header is what carries a table's own.
func TestMergeLayer_NewTableBringsItsHeaderComments(t *testing.T) {
	target := mergeInto(t,
		"name = \"app\"\n",
		"# the logging section\n[logging]\n# how loud\nlevel = \"debug\"\n")

	out := string(target.Bytes())
	if !strings.Contains(out, "the logging section") {
		t.Errorf("the table's header comment was dropped:\n%s", out)
	}
	if !strings.Contains(out, "how loud") {
		t.Errorf("the key's comment was dropped:\n%s", out)
	}
}

// Fails if the entries of an array-of-tables that is new in the target arrive
// stripped of their comments -- the header's and their keys' alike.
func TestMergeLayer_NewArrayOfTablesBringsItsComments(t *testing.T) {
	target := mergeInto(t,
		"shop = \"corner\"\n",
		"# the first product\n[[products]]\n# what it is called\nname = \"Hammer\"\n\n# the second product\n[[products]]\nname = \"Nail\"\n")

	if got, err := target.GetString("products[0].name"); err != nil || got != "Hammer" {
		t.Errorf("products[0].name = %q (%v), want \"Hammer\"", got, err)
	}
	if got, err := target.GetString("products[1].name"); err != nil || got != "Nail" {
		t.Errorf("products[1].name = %q (%v), want \"Nail\"", got, err)
	}

	out := string(target.Bytes())
	for _, want := range []string{"the first product", "what it is called", "the second product"} {
		if !strings.Contains(out, want) {
			t.Errorf("comment %q was dropped:\n%s", want, out)
		}
	}
	if _, err := Parse(target.Bytes()); err != nil {
		t.Fatalf("the merged document does not re-parse: %v\n%s", err, out)
	}
}

// Fails if the source's spelling starts changing what merges: three documents
// saying the same thing in three forms must seed one target identically.
func TestMergeLayer_SourceSpellingDoesNotChangeWhatMerges(t *testing.T) {
	spellings := []string{
		"[a]\nb = 1\nc = 2\n",
		"a.b = 1\na.c = 2\n",
		"a = {b = 1, c = 2}\n",
	}
	for _, src := range spellings {
		target := mergeInto(t, "name = \"app\"\n", src)
		if got, err := target.GetInt("a.b"); err != nil || got != 1 {
			t.Errorf("source %q: a.b = %d (%v), want 1", src, got, err)
		}
		if got, err := target.GetInt("a.c"); err != nil || got != 2 {
			t.Errorf("source %q: a.c = %d (%v), want 2", src, got, err)
		}
		if _, err := Parse(target.Bytes()); err != nil {
			t.Errorf("source %q: the merged document does not re-parse: %v\n%s", src, err, target.Bytes())
		}
	}
}

// Fails if an inline table that is new in the target arrives with its keys
// reordered: an inline table merges as one construct, and the order the source
// wrote its keys in comes along with it, at every depth and inside an array.
func TestMergeLayer_NewInlineTableKeepsSourceKeyOrder(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{
			"flat",
			"t = {z = 1, a = 2, m = 3}\n",
			"name = \"app\"\nt = {z = 1, a = 2, m = 3}\n",
		},
		{
			"nested",
			"t = {z = 1, inner = {q = 1, b = 2}, a = 3}\n",
			"name = \"app\"\nt = {z = 1, inner = {q = 1, b = 2}, a = 3}\n",
		},
		{
			"inside an array",
			"t = [{z = 1, a = 2}, {y = 3, b = 4}]\n",
			"name = \"app\"\nt = [{z = 1, a = 2}, {y = 3, b = 4}]\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := mergeInto(t, "name = \"app\"\n", tc.source)
			if got := string(target.Bytes()); got != tc.want {
				t.Errorf("merged document is\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

// Fails if a dotted key inside a merged inline table stops naming the table it
// names: {a.b = 1} binds b inside a, and a merge that turned it into a single
// key spelled "a.b" would say something the source never said.
func TestMergeLayer_NewInlineTableKeepsDottedKeyMeaning(t *testing.T) {
	target := mergeInto(t, "name = \"app\"\n", "t = {a.b = 1, z = 2, a.c = 3}\n")

	for path, want := range map[string]int64{"t.a.b": 1, "t.z": 2, "t.a.c": 3} {
		got, err := target.GetInt(path)
		if err != nil || got != want {
			t.Errorf("%s = %d (%v), want %d", path, got, err, want)
		}
	}
	if _, err := Parse(target.Bytes()); err != nil {
		t.Fatalf("the merged document does not re-parse: %v\n%s", err, target.Bytes())
	}
}

// Fails if a quoted key inside a merged inline table arrives with its quotes
// baked into the key text: the key "x.y" is one key whose text has a dot in
// it, not a key whose text has quotation marks in it.
func TestMergeLayer_NewInlineTableKeepsQuotedKeyText(t *testing.T) {
	target := mergeInto(t, "name = \"app\"\n", "t = {\"x.y\" = 1}\n")

	if got, err := target.GetInt("t.\"x.y\""); err != nil || got != 1 {
		t.Errorf("t.\"x.y\" = %d (%v), want 1\n%s", got, err, target.Bytes())
	}
	if _, err := Parse(target.Bytes()); err != nil {
		t.Fatalf("the merged document does not re-parse: %v\n%s", err, target.Bytes())
	}
}

// Fails if a sub-table under an array-of-tables entry stops folding into the
// entry it was written under: the fold addresses the LAST entry, and a merge
// that walked the raw children by key-path prefix could put it under any.
func TestMergeLayer_SubTableUnderArrayEntryMergesIntoThatEntry(t *testing.T) {
	target := mergeInto(t,
		"name = \"shop\"\n",
		"[[s]]\nid = 1\n\n[[s]]\nid = 2\n\n[s.t]\nx = 9\n")

	if got, err := target.GetInt("s[0].id"); err != nil || got != 1 {
		t.Errorf("s[0].id = %d (%v), want 1", got, err)
	}
	if got, err := target.GetInt("s[1].id"); err != nil || got != 2 {
		t.Errorf("s[1].id = %d (%v), want 2", got, err)
	}
	if got, err := target.GetInt("s[1].t.x"); err != nil || got != 9 {
		t.Errorf("s[1].t.x = %d (%v), want 9 -- the sub-table belongs to the last entry", got, err)
	}
	if target.Has("s[0].t.x") {
		t.Errorf("the sub-table was merged into the first entry too:\n%s", target.Bytes())
	}
	if _, err := Parse(target.Bytes()); err != nil {
		t.Fatalf("the merged document does not re-parse: %v\n%s", err, target.Bytes())
	}
}
