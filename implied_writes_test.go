package tomledit

import "testing"

// Writes whose PARENT is a table no single node stands for -- one a dotted key
// spelled out, or one only a longer header implies. The parent's own spelling
// decides where the write goes, not whether it happens: an existing key is
// reached through the pair that already binds it, a new key under a dotted
// table joins that table's own region as another dotted pair, and a new key
// under a table a longer header implies gets the anchoring header TOML allows.

// Fails if writing over a key a dotted pair binds is refused: the pair is a
// concrete node, and the write has somewhere to go whatever the layer says
// about the table above it.
func TestSet_ExistingKeyUnderADottedTable(t *testing.T) {
	doc := parseOrFail(t, "a.b = 1\n")
	if err := doc.Set("a.b", 2); err != nil {
		t.Fatalf(`Set("a.b", 2) was refused: %v`, err)
	}
	if got := string(doc.Bytes()); got != "a.b = 2\n" {
		t.Errorf("the document reads %q, want %q", got, "a.b = 2\n")
	}
	mustFold(t, doc)
}

// Fails if writing over a dotted key inside a [header] table is refused -- the
// same rule one level down.
func TestSet_ExistingKeyUnderADottedTableInsideATable(t *testing.T) {
	doc := parseOrFail(t, "[t]\na.b = 1\n")
	if err := doc.Set("t.a.b", 2); err != nil {
		t.Fatalf(`Set("t.a.b", 2) was refused: %v`, err)
	}
	if got := string(doc.Bytes()); got != "[t]\na.b = 2\n" {
		t.Errorf("the document reads %q, want %q", got, "[t]\na.b = 2\n")
	}
	mustFold(t, doc)
}

// Fails if a new key under a table a dotted key spelled out is refused, or is
// written as anything but another dotted pair in the same region: a [a] header
// there would not parse, since TOML refuses to redefine such a table.
func TestSet_NewKeyUnderADottedTableExtendsTheDottedRegion(t *testing.T) {
	doc := parseOrFail(t, "a.b = 1\n")
	if err := doc.Set("a.new", 5); err != nil {
		t.Fatalf(`Set("a.new", 5) was refused: %v`, err)
	}
	const want = "a.b = 1\na.new = 5\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
	again, err := Parse(doc.Bytes())
	if err != nil {
		t.Fatalf("the result is not valid TOML: %v\n%s", err, doc.Bytes())
	}
	if got, ok := again.GetInt("a.new"); !ok || got != 5 {
		t.Errorf("after the round-trip a.new reads %d (ok=%v), want 5", got, ok)
	}
}

// Fails if SetCreate refuses the same write Set makes: the parent is there, so
// there is nothing to create and the two must agree.
func TestSetCreate_NewKeyUnderADottedTable(t *testing.T) {
	doc := parseOrFail(t, "a.b = 1\n")
	if err := doc.SetCreate("a.new", 5); err != nil {
		t.Fatalf(`SetCreate("a.new", 5) was refused: %v`, err)
	}
	const want = "a.b = 1\na.new = 5\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
}

// Fails if a new key under a dotted table inside a [header] table is refused or
// escapes that table's region.
func TestSet_NewKeyUnderADottedTableInsideATable(t *testing.T) {
	doc := parseOrFail(t, "[t]\na.b = 1\n")
	if err := doc.Set("t.a.new", 5); err != nil {
		t.Fatalf(`Set("t.a.new", 5) was refused: %v`, err)
	}
	const want = "[t]\na.b = 1\na.new = 5\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
	again, err := Parse(doc.Bytes())
	if err != nil {
		t.Fatalf("the result is not valid TOML: %v\n%s", err, doc.Bytes())
	}
	if got, ok := again.GetInt("t.a.new"); !ok || got != 5 {
		t.Errorf("after the round-trip t.a.new reads %d (ok=%v), want 5", got, ok)
	}
}

// Fails if a new key under a table only a LONGER header implies is refused:
// that table has no header of its own yet, and giving it one is valid TOML --
// the same anchoring NewTable already performs.
func TestSet_NewKeyUnderAHeaderImpliedTableMaterializesTheHeader(t *testing.T) {
	doc := parseOrFail(t, "[a.b]\nx = 1\n")
	if err := doc.Set("a.new", 5); err != nil {
		t.Fatalf(`Set("a.new", 5) was refused: %v`, err)
	}
	const want = "[a.b]\nx = 1\n[a]\nnew = 5\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
	again, err := Parse(doc.Bytes())
	if err != nil {
		t.Fatalf("the result is not valid TOML: %v\n%s", err, doc.Bytes())
	}
	if got, ok := again.GetInt("a.new"); !ok || got != 5 {
		t.Errorf("after the round-trip a.new reads %d (ok=%v), want 5", got, ok)
	}
	if got, ok := again.GetInt("a.b.x"); !ok || got != 1 {
		t.Errorf("after the round-trip a.b.x reads %d (ok=%v), want 1", got, ok)
	}
}

// Fails if seeding a default beside a dotted key is refused. This is the
// scenario the read-layer work was driven by: a [server] table whose logging
// options are written as dotted keys, and a default list that fills in the one
// option the document does not carry.
func TestEnsureDefaults_SeedsBesideADottedKey(t *testing.T) {
	doc := parseOrFail(t, "[server]\nlog.level = \"info\"\n")
	added, err := doc.EnsureDefaults([]Default{
		{"server.log.level", "debug"},
		{"server.log.file", "/var/log/app.log"},
	})
	if err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	if len(added) != 1 || added[0] != "server.log.file" {
		t.Errorf("added = %v, want only server.log.file", added)
	}
	const want = "[server]\nlog.level = \"info\"\nlog.file = \"/var/log/app.log\"\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads %q, want %q", got, want)
	}
	again, err := Parse(doc.Bytes())
	if err != nil {
		t.Fatalf("the result is not valid TOML: %v\n%s", err, doc.Bytes())
	}
	if got, ok := again.GetString("server.log.level"); !ok || got != "info" {
		t.Errorf("the document's own value was overwritten: %q (ok=%v)", got, ok)
	}
	if got, ok := again.GetString("server.log.file"); !ok || got != "/var/log/app.log" {
		t.Errorf("the seeded value reads %q (ok=%v)", got, ok)
	}
}

// Fails if the region a dotted key spells out inside an INLINE table stops
// being writable: the pair belongs to the inline table's own children there,
// and the table re-renders as one value fragment.
func TestSet_NewKeyUnderADottedTableInsideAnInlineTable(t *testing.T) {
	doc := parseOrFail(t, "t = { a.b = 1 }\n")
	if err := doc.Set("t.a.new", 5); err != nil {
		t.Fatalf(`Set("t.a.new", 5) was refused: %v`, err)
	}
	again, err := Parse(doc.Bytes())
	if err != nil {
		t.Fatalf("the result is not valid TOML: %v\n%s", err, doc.Bytes())
	}
	if got, ok := again.GetInt("t.a.new"); !ok || got != 5 {
		t.Errorf("t.a.new reads %d (ok=%v) in %q", got, ok, doc.Bytes())
	}
	if got, ok := again.GetInt("t.a.b"); !ok || got != 1 {
		t.Errorf("t.a.b reads %d (ok=%v) in %q", got, ok, doc.Bytes())
	}
}

// --- removal ---

// Fails if deleting a key a dotted pair binds removes nothing: the pair is
// there, so the removal is not the missing-path no-op the contract allows.
func TestDelete_KeyUnderADottedTable(t *testing.T) {
	doc := parseOrFail(t, "a.b = 1\n")
	if err := doc.Delete("a.b"); err != nil {
		t.Fatalf(`Delete("a.b") reported %v`, err)
	}
	if got := string(doc.Bytes()); got != "" {
		t.Errorf("the document reads %q, want the pair gone", got)
	}
	mustFold(t, doc)
}

// Fails if deleting one pair of a dotted region takes the others with it, or
// leaves the named one behind.
func TestDelete_OnePairOfADottedRegion(t *testing.T) {
	doc := parseOrFail(t, "a.b = 1\na.c = 2\n")
	if err := doc.Delete("a.b"); err != nil {
		t.Fatalf(`Delete("a.b") reported %v`, err)
	}
	if got := string(doc.Bytes()); got != "a.c = 2\n" {
		t.Errorf("the document reads %q, want %q", got, "a.c = 2\n")
	}
	mustFold(t, doc)
}

// Fails if the removal stops at the document level: a dotted pair inside a
// [header] table is bound the same way.
func TestDelete_KeyUnderADottedTableInsideATable(t *testing.T) {
	doc := parseOrFail(t, "[t]\na.b = 1\n")
	if err := doc.Delete("t.a.b"); err != nil {
		t.Fatalf(`Delete("t.a.b") reported %v`, err)
	}
	if got := string(doc.Bytes()); got != "[t]\n" {
		t.Errorf("the document reads %q, want %q", got, "[t]\n")
	}
	mustFold(t, doc)
}

// Fails if a table under a table only a longer header implies survives its own
// removal: [a.b] is what "a.b" names, and the headers written under it go with
// it.
func TestDelete_TableUnderAHeaderImpliedTable(t *testing.T) {
	doc := parseOrFail(t, "[a.b]\nx = 1\n")
	if err := doc.Delete("a.b"); err != nil {
		t.Fatalf(`Delete("a.b") reported %v`, err)
	}
	if got := string(doc.Bytes()); got != "" {
		t.Errorf("the document reads %q, want the table gone", got)
	}

	nested := parseOrFail(t, "[a.b.c]\nx = 1\n")
	if err := nested.Delete("a.b"); err != nil {
		t.Fatalf(`Delete("a.b") reported %v`, err)
	}
	if got := string(nested.Bytes()); got != "" {
		t.Errorf("the document reads %q, want the nested table gone with it", got)
	}
}

// Fails if an array-of-tables under a table only a longer header implies
// survives its own removal.
func TestDelete_ArrayOfTablesUnderAHeaderImpliedTable(t *testing.T) {
	doc := parseOrFail(t, "[a.b]\nx = 1\n[[a.c]]\ny = 2\n[[a.c]]\ny = 3\n")
	if err := doc.Delete("a.c"); err != nil {
		t.Fatalf(`Delete("a.c") reported %v`, err)
	}
	if got := string(doc.Bytes()); got != "[a.b]\nx = 1\n" {
		t.Errorf("the document reads %q, want both entries gone", got)
	}
	mustFold(t, doc)
}
