package tomledit

import (
	"errors"
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

// --- table creation, continued ---

// Fails if a sub-table under an existing header stops being creatable.
func TestNewTable_AllowsASubTable(t *testing.T) {
	doc := parseOrFail(t, "[a]\nx = 1\n")
	if err := doc.NewTable("a.b"); err != nil {
		t.Fatalf(`NewTable("a.b") was refused: %v`, err)
	}
	mustFold(t, doc)
}
