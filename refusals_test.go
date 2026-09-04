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

// Fails if a sub-table under an existing header stops being creatable.
func TestNewTable_AllowsASubTable(t *testing.T) {
	doc := parseOrFail(t, "[a]\nx = 1\n")
	if err := doc.NewTable("a.b"); err != nil {
		t.Fatalf(`NewTable("a.b") was refused: %v`, err)
	}
	mustFold(t, doc)
}
