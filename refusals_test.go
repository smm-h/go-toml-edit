package tomledit

import (
	"errors"
	"strings"
	"testing"
)

// The fold-aware edit refusals. Every editing operation that could bind a name
// already bound by another construct refuses instead, so no sequence of public
// edits can build a document the read-layer cannot fold. TestEditRefusals_
// NoSequenceCanBreakTheFold below drives the audited sequences through the
// whole surface at once; the tests here pin each refusal on its own.

// refuse runs an edit expected to be refused and returns its diagnostic.
func refuse(t *testing.T, doc *Document, op string, err error, want error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s was accepted; the document now reads %q", op, doc.Bytes())
	}
	if !errors.Is(err, want) {
		t.Errorf("%s reported %v, want %v", op, err, want)
	}
	mustFold(t, doc)
}

// mustFold asserts the document still folds -- the property every refusal in
// this file protects.
func mustFold(t *testing.T, doc *Document) {
	t.Helper()
	if _, err := foldDocument(doc); err != nil {
		t.Fatalf("the document no longer folds: %v\n%s", err, doc.Bytes())
	}
}

// parseOrFail parses source, failing the test when it is not valid TOML.
func parseOrFail(t *testing.T, src string) *Document {
	t.Helper()
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return doc
}

// --- the audited sequences, and the property they protect ---

// Fails if any of the editing sequences audited during the read-layer work
// stops being refused, or is refused with a different kind than the one ruled
// for it. Each built a document the read-layer could not fold, leaving Root()
// to panic and the document unrepairable through the public surface.
func TestEditRefusals_TheAuditedSequences(t *testing.T) {
	tests := []struct {
		name string
		src  string
		run  func(*Document) error
		want error
	}{
		{
			"NewArrayTable over a table",
			"[a]\nx = 1\n",
			func(d *Document) error { return d.NewArrayTable("a") },
			ErrConflict,
		},
		{
			"NewTable over an array-of-tables",
			"[[a]]\nx = 1\n",
			func(d *Document) error { return d.NewTable("a") },
			ErrConflict,
		},
		{
			"NewTable over a value",
			"a = 1\n",
			func(d *Document) error { return d.NewTable("a") },
			ErrConflict,
		},
		{
			"NewArrayTable over a value",
			"a = 1\n",
			func(d *Document) error { return d.NewArrayTable("a") },
			ErrConflict,
		},
		{
			"Set over a header table",
			"[a]\nx = 1\n",
			func(d *Document) error { return d.Set("a", 1) },
			ErrWrongContainer,
		},
		{
			"Set over a dotted prefix",
			"a.b = 1\n",
			func(d *Document) error { return d.Set("a", 1) },
			ErrWrongContainer,
		},
		{
			"SetCreate over an array-of-tables",
			"[[s]]\nx = 1\n",
			func(d *Document) error { return d.SetCreate("s", 1) },
			ErrWrongContainer,
		},
		{
			"RenameKey onto a table-bound name",
			"x = 1\n[a]\ny = 2\n",
			func(d *Document) error { return d.RenameKey("x", "a") },
			ErrConflict,
		},
		{
			"NewTable under a value",
			"a = 1\n",
			func(d *Document) error { return d.NewTable("a.b") },
			ErrConflict,
		},
		{
			"Set beside a dotted key inside a table",
			"[t]\na.b = 1\n",
			func(d *Document) error { return d.Set("t.a", 1) },
			ErrWrongContainer,
		},
		{
			"NewTable then NewArrayTable of one name",
			"x = 1\n",
			func(d *Document) error {
				if err := d.NewTable("z"); err != nil {
					return err
				}
				return d.NewArrayTable("z")
			},
			ErrConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := parseOrFail(t, tt.src)
			err := tt.run(doc)
			if err == nil {
				t.Fatalf("the sequence was accepted; the document now reads %q", doc.Bytes())
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("the refusal is %v, want %v", err, tt.want)
			}
			assertRootDoesNotPanic(t, doc)
		})
	}
}

// Fails if any edit through the public surface leaves the document unfoldable
// -- the property the refusals exist for, checked over a grid of documents and
// operations rather than over the audited sequences alone. An operation may
// refuse or succeed; what it may not do is leave a document whose Root()
// panics.
func TestEditRefusals_NoSequenceCanBreakTheFold(t *testing.T) {
	sources := []string{
		"",
		"a = 1\n",
		"a.b = 1\n",
		"a = { b = 1 }\n",
		"[a]\nb = 1\n",
		"[a.b]\nc = 1\n",
		"[[a]]\nb = 1\n",
		"[[a]]\nb = 1\n[[a]]\nb = 2\n",
		"[[a]]\nb = 1\n[a.c]\nd = 2\n",
		"x = 1\n[a]\nb = 2\n[[c]]\nd = 3\n",
	}
	paths := []string{"a", "b", "a.b", "a.b.c", "a[0]", "a[0].b", "x"}

	for _, src := range sources {
		for _, path := range paths {
			operations := []struct {
				name string
				run  func(*Document) error
			}{
				{"Set", func(d *Document) error { return d.Set(path, 7) }},
				{"SetCreate", func(d *Document) error { return d.SetCreate(path, 7) }},
				{"SetCreate table", func(d *Document) error { return d.SetCreate(path, map[string]any{"k": 1}) }},
				{"Delete", func(d *Document) error { return d.Delete(path) }},
				{"NewTable", func(d *Document) error { return d.NewTable(path) }},
				{"NewArrayTable", func(d *Document) error { return d.NewArrayTable(path) }},
				{"RenameKey to a", func(d *Document) error { return d.RenameKey(path, "a") }},
				{"RenameKey to fresh", func(d *Document) error { return d.RenameKey(path, "fresh") }},
				{"AppendToArray", func(d *Document) error { return d.AppendToArray(path, 7) }},
				{"RemoveFromArray", func(d *Document) error { return d.RemoveFromArray(path, 0) }},
				{"PermuteChildren", func(d *Document) error { return d.PermuteChildren(path, []int{0}) }},
				{"SetComment", func(d *Document) error { return d.SetComment(path, "note") }},
			}
			for _, op := range operations {
				name := op.name + " " + path + " on " + strings.ReplaceAll(src, "\n", "|")
				t.Run(name, func(t *testing.T) {
					doc := parseOrFail(t, src)
					_ = op.run(doc)
					assertRootDoesNotPanic(t, doc)
					// A document that still folds must also still render as
					// TOML that parses back.
					if _, err := Parse(doc.Bytes()); err != nil {
						t.Fatalf("the document no longer parses: %v\n%q", err, doc.Bytes())
					}
				})
			}
		}
	}
}

// assertRootDoesNotPanic fails the test when reading the document's read-layer
// panics -- the failure mode every refusal in this file prevents.
func assertRootDoesNotPanic(t *testing.T, doc *Document) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Root() panicked: %v\nthe document reads %q", r, doc.Bytes())
		}
	}()
	doc.Root()
}

// --- the structural creators ---

// Fails if NewArrayTable stops refusing a name a [header] table already binds:
// the two constructs would claim one key and the document would stop folding.
func TestNewArrayTable_RefusesATableOfTheSameName(t *testing.T) {
	doc := parseOrFail(t, "[a]\nx = 1\n")
	refuse(t, doc, `NewArrayTable("a")`, doc.NewArrayTable("a"), ErrConflict)
}

// Fails if NewTable stops refusing a name an array-of-tables already binds.
func TestNewTable_RefusesAnArrayOfTablesOfTheSameName(t *testing.T) {
	doc := parseOrFail(t, "[[a]]\nx = 1\n")
	refuse(t, doc, `NewTable("a")`, doc.NewTable("a"), ErrConflict)
}

// Fails if NewTable stops refusing a name bound to a value.
func TestNewTable_RefusesAValueOfTheSameName(t *testing.T) {
	doc := parseOrFail(t, "a = 1\n")
	refuse(t, doc, `NewTable("a")`, doc.NewTable("a"), ErrConflict)
}

// Fails if NewArrayTable stops refusing a name bound to a value.
func TestNewArrayTable_RefusesAValueOfTheSameName(t *testing.T) {
	doc := parseOrFail(t, "a = 1\n")
	refuse(t, doc, `NewArrayTable("a")`, doc.NewArrayTable("a"), ErrConflict)
}

// Fails if a header is created under a name bound to a value: "a" holds an
// integer, so nothing can be nested inside it.
func TestNewTable_RefusesAPrefixBoundToAValue(t *testing.T) {
	doc := parseOrFail(t, "a = 1\n")
	refuse(t, doc, `NewTable("a.b")`, doc.NewTable("a.b"), ErrConflict)
}

// Fails if the two creators stop seeing each other's work: a table and an
// array-of-tables of one name, both created through the public surface.
func TestNewArrayTable_RefusesATableThisSessionCreated(t *testing.T) {
	doc := parseOrFail(t, "x = 1\n")
	if err := doc.NewTable("z"); err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	refuse(t, doc, `NewArrayTable("z")`, doc.NewArrayTable("z"), ErrConflict)
}

// Fails if NewTable stops refusing a table it already created -- the
// pre-existing duplicate refusal, restated through the read-layer.
func TestNewTable_RefusesADuplicateTable(t *testing.T) {
	doc := parseOrFail(t, "[a]\nx = 1\n")
	refuse(t, doc, `NewTable("a")`, doc.NewTable("a"), ErrConflict)
}

// Fails if NewTable stops refusing a name an inline table binds: a header for
// a key already written as an inline table is not valid TOML.
func TestNewTable_RefusesAnInlineTableOfTheSameName(t *testing.T) {
	doc := parseOrFail(t, "a = { x = 1 }\n")
	refuse(t, doc, `NewTable("a")`, doc.NewTable("a"), ErrConflict)
}

// Fails if NewTable stops refusing a table a dotted key implied: TOML forbids
// extending a dotted-key table with a header of its own.
func TestNewTable_RefusesATableImpliedByADottedKey(t *testing.T) {
	doc := parseOrFail(t, "a.b = 1\n")
	refuse(t, doc, `NewTable("a")`, doc.NewTable("a"), ErrConflict)
}

// Fails if the refusals grow into an over-refusal: a table implied by a LONGER
// header may be given a header of its own, which is valid TOML and folds.
func TestNewTable_AllowsAnchoringATableImpliedByALongerHeader(t *testing.T) {
	doc := parseOrFail(t, "[a.b]\nx = 1\n")
	if err := doc.NewTable("a"); err != nil {
		t.Fatalf(`NewTable("a") on a table implied by [a.b] was refused: %v`, err)
	}
	mustFold(t, doc)
	if _, err := Parse(doc.Bytes()); err != nil {
		t.Fatalf("the result is not valid TOML: %v\n%s", err, doc.Bytes())
	}
}

// Fails if a header is created under a prefix bound to an INLINE table. TOML
// gives an inline table no way to be extended, so a [a.c] header beside
// "a = { b = 1 }" is not valid TOML: the creators must refuse the name rather
// than write bytes the parser rejects.
func TestNewTable_RefusesAPrefixBoundToAnInlineTable(t *testing.T) {
	doc := parseOrFail(t, "a = { b = 1 }\n")
	refuse(t, doc, `NewTable("a.c")`, doc.NewTable("a.c"), ErrConflict)

	arr := parseOrFail(t, "a = { b = 1 }\n")
	refuse(t, arr, `NewArrayTable("a.c")`, arr.NewArrayTable("a.c"), ErrConflict)
}

// Fails if a header UNDER a table a dotted key spelled out stops being
// creatable. TOML forbids giving such a table a header of its OWN; a header for
// a table nested under it is explicitly permitted, and the compliance corpus
// this package passes carries the document as valid/spec-1.0.0/table-9.toml.
func TestNewTable_AllowsASubTableUnderADottedImpliedTable(t *testing.T) {
	doc := parseOrFail(t, "[fruit]\napple.color = \"red\"\napple.taste.sweet = true\n")
	if err := doc.NewTable("fruit.apple.texture"); err != nil {
		t.Fatalf(`NewTable("fruit.apple.texture") was refused: %v`, err)
	}
	mustFold(t, doc)
	if err := doc.Set("fruit.apple.texture.smooth", true); err != nil {
		t.Fatalf("writing into the new table was refused: %v", err)
	}
	again, err := Parse(doc.Bytes())
	if err != nil {
		t.Fatalf("the result is not valid TOML: %v\n%s", err, doc.Bytes())
	}
	if got, ok := again.GetBool("fruit.apple.texture.smooth"); !ok || !got {
		t.Errorf("after the round-trip the document reads\n%s", doc.Bytes())
	}
}

// Fails if an array-of-tables entry UNDER a table a dotted key spelled out
// stops being creatable -- the corpus's valid/table/array-within-dotted.toml.
func TestNewArrayTable_AllowsAnEntryUnderADottedImpliedTable(t *testing.T) {
	doc := parseOrFail(t, "[fruit]\napple.color = \"red\"\n")
	if err := doc.NewArrayTable("fruit.apple.seeds"); err != nil {
		t.Fatalf(`NewArrayTable("fruit.apple.seeds") was refused: %v`, err)
	}
	mustFold(t, doc)
	if err := doc.Set("fruit.apple.seeds[0].size", 2); err != nil {
		t.Fatalf("writing into the new entry was refused: %v", err)
	}
	again, err := Parse(doc.Bytes())
	if err != nil {
		t.Fatalf("the result is not valid TOML: %v\n%s", err, doc.Bytes())
	}
	if got, ok := again.GetInt("fruit.apple.seeds[0].size"); !ok || got != 2 {
		t.Errorf("after the round-trip the document reads\n%s", doc.Bytes())
	}
}

// Fails if the refusal at the FINAL key weakens or misnames its subject: a
// header may not redefine a table a dotted key spelled out, whether that table
// is the path's own last key or one a longer dotted key implied deeper, and the
// diagnostic must name the path whose final key violates the rule.
func TestNewTable_RefusesRedefiningADottedImpliedTable(t *testing.T) {
	const src = "[fruit]\napple.color = \"red\"\napple.taste.sweet = true\n"

	doc := parseOrFail(t, src)
	refuse(t, doc, `NewTable("fruit.apple")`, doc.NewTable("fruit.apple"), ErrConflict)

	deeper := parseOrFail(t, src)
	err := deeper.NewTable("fruit.apple.taste")
	refuse(t, deeper, `NewTable("fruit.apple.taste")`, err, ErrConflict)
	if err != nil && !strings.Contains(err.Error(), `"fruit.apple.taste"`) {
		t.Errorf("the refusal reads %v; it must name fruit.apple.taste, whose final key redefines the dotted table", err)
	}
}

// Fails if NewArrayTable stops appending entries: a name already bound to an
// array-of-tables is exactly what a new entry extends.
func TestNewArrayTable_AllowsAnotherEntry(t *testing.T) {
	doc := parseOrFail(t, "[[s]]\nx = 1\n")
	if err := doc.NewArrayTable("s"); err != nil {
		t.Fatalf(`NewArrayTable("s") on an existing array-of-tables was refused: %v`, err)
	}
	mustFold(t, doc)
	root := doc.Root()
	e, _ := root.Get("s")
	records, _ := e.Records()
	if len(records) != 2 {
		t.Errorf("the array-of-tables holds %d entries, want 2", len(records))
	}
}

// Fails if a sub-table under an array-of-tables stops being creatable: it
// addresses the last entry, which is valid TOML.
func TestNewTable_AllowsASubTableUnderAnArrayOfTables(t *testing.T) {
	doc := parseOrFail(t, "[[s]]\nx = 1\n")
	if err := doc.NewTable("s.t"); err != nil {
		t.Fatalf(`NewTable("s.t") was refused: %v`, err)
	}
	mustFold(t, doc)
	if _, err := Parse(doc.Bytes()); err != nil {
		t.Fatalf("the result is not valid TOML: %v\n%s", err, doc.Bytes())
	}
}

// --- RenameKey ---

// Fails if RenameKey only looks at sibling key-value pairs: a [header] table
// binds its name just as much, and renaming onto it builds a document with two
// constructs on one key.
func TestRenameKey_RefusesANameATableBinds(t *testing.T) {
	doc := parseOrFail(t, "x = 1\n[a]\ny = 2\n")
	refuse(t, doc, `RenameKey("x", "a")`, doc.RenameKey("x", "a"), ErrConflict)
}

// Fails if RenameKey stops seeing an array-of-tables as a binding of its name.
func TestRenameKey_RefusesANameAnArrayOfTablesBinds(t *testing.T) {
	doc := parseOrFail(t, "x = 1\n[[a]]\ny = 2\n")
	refuse(t, doc, `RenameKey("x", "a")`, doc.RenameKey("x", "a"), ErrConflict)
}

// Fails if RenameKey stops seeing the table a dotted key implies as a binding.
func TestRenameKey_RefusesANameADottedKeyImplies(t *testing.T) {
	doc := parseOrFail(t, "[t]\nx = 1\na.b = 2\n")
	refuse(t, doc, `RenameKey("t.x", "a")`, doc.RenameKey("t.x", "a"), ErrConflict)
}

// Fails if the refusal grows into an over-refusal: renaming onto a free name
// is the operation's whole purpose.
func TestRenameKey_AllowsAFreeName(t *testing.T) {
	doc := parseOrFail(t, "x = 1\n[a]\ny = 2\n")
	if err := doc.RenameKey("x", "z"); err != nil {
		t.Fatalf(`RenameKey("x", "z") was refused: %v`, err)
	}
	mustFold(t, doc)
	if !doc.Has("z") {
		t.Errorf("after the rename the document reads %q", doc.Bytes())
	}
}

// --- value writes ---

// Fails if a value write starts replacing a structural construct: a [header]
// table changes through a structural operation or an explicit Delete, never as
// a side effect of writing a value at its name.
func TestSet_RefusesANameBoundByAHeaderTable(t *testing.T) {
	doc := parseOrFail(t, "[a]\nx = 1\n")
	refuse(t, doc, `Set("a", 5)`, doc.Set("a", 5), ErrWrongContainer)
}

// Fails if a value write starts binding a name a dotted key already spelled
// out: "a" is a table there, and writing a value at it would define the key
// twice.
func TestSet_RefusesANameADottedKeyImplies(t *testing.T) {
	doc := parseOrFail(t, "a.b = 1\n")
	refuse(t, doc, `Set("a", 5)`, doc.Set("a", 5), ErrWrongContainer)
}

// Fails if a value write starts binding a name a dotted key implied inside a
// table -- the same rule one level down.
func TestSet_RefusesADottedPrefixInsideATable(t *testing.T) {
	doc := parseOrFail(t, "[t]\na.b = 1\n")
	refuse(t, doc, `Set("t.a", 5)`, doc.Set("t.a", 5), ErrWrongContainer)
}

// Fails if a value write starts binding the name of an array-of-tables, which
// no single node stands for.
func TestSetCreate_RefusesAnArrayOfTablesName(t *testing.T) {
	doc := parseOrFail(t, "[[s]]\nx = 1\n")
	refuse(t, doc, `SetCreate("s", 5)`, doc.SetCreate("s", 5), ErrWrongContainer)
}

// Fails if a key written under an array-of-tables collection stops being
// refused: the collection is not a table, so it holds no keys of its own.
func TestSetCreate_RefusesAKeyUnderAnArrayOfTables(t *testing.T) {
	doc := parseOrFail(t, "[[s]]\nx = 1\n")
	refuse(t, doc, `SetCreate("s.k", 5)`, doc.SetCreate("s.k", 5), ErrWrongContainer)
}

// Fails if a value write starts binding the name of a table only implied by a
// longer header.
func TestSet_RefusesANameALongerHeaderImplies(t *testing.T) {
	doc := parseOrFail(t, "[a.b]\nx = 1\n")
	refuse(t, doc, `Set("a", 5)`, doc.Set("a", 5), ErrWrongContainer)
}

// Fails if the refusals reach an inline table: a key holding one is a value
// fragment, and setting it replaces that value wholesale.
func TestSet_ReplacesAnInlineTableWholesale(t *testing.T) {
	doc := parseOrFail(t, "a = { x = 1 }\n")
	if err := doc.Set("a", map[string]any{"y": 2}); err != nil {
		t.Fatalf(`Set over an inline table was refused: %v`, err)
	}
	mustFold(t, doc)
	if got := string(doc.Bytes()); got != "a = {y = 2}\n" {
		t.Errorf("the document reads %q, want the replaced inline table", got)
	}
}

// Fails if the refusals reach an ordinary value write or a new key.
func TestSet_AllowsValuesAndNewKeys(t *testing.T) {
	doc := parseOrFail(t, "[a]\nx = 1\n")
	if err := doc.Set("a.x", 2); err != nil {
		t.Fatalf(`Set("a.x", 2) was refused: %v`, err)
	}
	if err := doc.Set("a.y", 3); err != nil {
		t.Fatalf(`Set("a.y", 3) was refused: %v`, err)
	}
	mustFold(t, doc)
	if v, ok := doc.GetInt("a.y"); !ok || v != 3 {
		t.Errorf("after the writes the document reads %q", doc.Bytes())
	}
}

// --- comments ---

// Fails if a comment write into an inline table reports anything but a
// wrong-container refusal. TOML gives an inline table no place to put a
// comment, so the operation has nowhere to be written at all -- the same
// family as renaming through an array index, which the second half asserts
// beside it. A conflict would say the edit produces an invalid document,
// which is a different claim.
func TestSetComment_InsideAnInlineTableIsAWrongContainer(t *testing.T) {
	doc := parseOrFail(t, "point = { x = 1, y = 2 }\n")
	err := doc.SetComment("point.x", "no")
	if !errors.Is(err, ErrWrongContainer) {
		t.Errorf("SetComment inside an inline table reported %v, want a wrong-container diagnostic", err)
	}

	arr := parseOrFail(t, "items = [1, 2]\n")
	err = arr.RenameKey("items[0]", "x")
	if !errors.Is(err, ErrWrongContainer) {
		t.Errorf("RenameKey through an array index reported %v, want a wrong-container diagnostic", err)
	}
}

// --- Delete ---

// Fails if Delete swallows a fold failure: a document the read-layer cannot
// make sense of must say so, not answer as though the path were simply absent.
func TestDelete_SurfacesAFoldFailure(t *testing.T) {
	doc := unfoldableDoc(t)
	err := doc.Delete("a.x")
	if err == nil {
		t.Fatal("Delete on an unfoldable document reported success")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("Delete reported %v, want a conflict diagnostic", err)
	}
}

// Fails if Delete stops being idempotent on a document that folds: a path
// naming nothing is a silent no-op, which is what makes ensure-absent loops
// writable.
func TestDelete_MissingPathStaysASilentNoOp(t *testing.T) {
	doc := parseOrFail(t, "[a]\nx = 1\n")
	if err := doc.Delete("a.missing"); err != nil {
		t.Errorf("Delete of a missing key reported %v, want nil", err)
	}
	if err := doc.Delete("nowhere.at.all"); err != nil {
		t.Errorf("Delete of a missing path reported %v, want nil", err)
	}
	if got := string(doc.Bytes()); got != "[a]\nx = 1\n" {
		t.Errorf("the document reads %q after two no-op deletes", got)
	}
}

// --- table creation, continued ---

// Fails if a sub-table under an existing header stops being creatable.
func TestNewTable_AllowsASubTable(t *testing.T) {
	doc := parseOrFail(t, "[a]\nx = 1\n")
	if err := doc.NewTable("a.b"); err != nil {
		t.Fatalf(`NewTable("a.b") was refused: %v`, err)
	}
	mustFold(t, doc)
}
