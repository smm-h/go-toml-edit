package tomledit

import "testing"

// --- Issue 1: Compound table paths not fully resolvable ---
// A table declared as [a.b.c.d] should make a.b.c.d.key resolvable via
// GetString("a.b.c.d.key"). Resolution must synthesize implicit intermediate
// tables for compound key paths.

func TestCompoundTablePath_DeepGet(t *testing.T) {
	input := []byte("[a.b.c.d]\nkey = \"deep\"\n")
	doc, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	val, err := doc.GetString("a.b.c.d.key")
	if err != nil {
		t.Fatalf("GetString(\"a.b.c.d.key\") returned not-ok; expected \"deep\"")
	}
	if val != "deep" {
		t.Fatalf("GetString(\"a.b.c.d.key\") = %q; want \"deep\"", val)
	}
}

func TestCompoundTablePath_IntermediateAccess(t *testing.T) {
	// Accessing a.b should resolve to an implicit table that eventually
	// leads to [a.b.c.d].
	input := []byte("[a.b.c.d]\nkey = \"deep\"\n")
	doc, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if _, ok := doc.Lookup("a.b.c.d"); !ok {
		t.Fatalf("Lookup(\"a.b.c.d\") found nothing; expected the table node")
	}
}

func TestCompoundTablePath_TwoLevels(t *testing.T) {
	input := []byte("[x.y]\nval = 1\n")
	doc, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	v, err := doc.GetInt("x.y.val")
	if err != nil {
		t.Fatalf("GetInt(\"x.y.val\") returned not-ok; expected 1")
	}
	if v != 1 {
		t.Fatalf("GetInt(\"x.y.val\") = %d; want 1", v)
	}
}

// --- Issue 2: Multiple dotted keys sharing a prefix ---
// When multiple dotted keys share a prefix (e.g., database.host and
// database.port), the resolver must be able to resolve all of them,
// not just the first one encountered.

func TestDottedKeys_MultipleWithSharedPrefix_TopLevel(t *testing.T) {
	input := []byte("database.host = \"localhost\"\ndatabase.port = 5432\n")
	doc, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	host, err := doc.GetString("database.host")
	if err != nil {
		t.Fatalf("GetString(\"database.host\") returned not-ok; expected \"localhost\"")
	}
	if host != "localhost" {
		t.Fatalf("GetString(\"database.host\") = %q; want \"localhost\"", host)
	}

	port, err := doc.GetInt("database.port")
	if err != nil {
		t.Fatalf("GetInt(\"database.port\") returned not-ok; expected 5432")
	}
	if port != 5432 {
		t.Fatalf("GetInt(\"database.port\") = %d; want 5432", port)
	}
}

func TestDottedKeys_MultipleWithSharedPrefix_InsideTable(t *testing.T) {
	input := []byte("[server]\ndatabase.host = \"localhost\"\ndatabase.port = 5432\n")
	doc, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	host, err := doc.GetString("server.database.host")
	if err != nil {
		t.Fatalf("GetString(\"server.database.host\") returned not-ok; expected \"localhost\"")
	}
	if host != "localhost" {
		t.Fatalf("GetString(\"server.database.host\") = %q; want \"localhost\"", host)
	}

	port, err := doc.GetInt("server.database.port")
	if err != nil {
		t.Fatalf("GetInt(\"server.database.port\") returned not-ok; expected 5432")
	}
	if port != 5432 {
		t.Fatalf("GetInt(\"server.database.port\") = %d; want 5432", port)
	}
}

func TestDottedKeys_ThreeKeysSharedPrefix(t *testing.T) {
	input := []byte("db.host = \"localhost\"\ndb.port = 5432\ndb.name = \"mydb\"\n")
	doc, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	host, err := doc.GetString("db.host")
	if err != nil {
		t.Fatalf("GetString(\"db.host\") returned not-ok")
	}
	if host != "localhost" {
		t.Fatalf("got %q, want \"localhost\"", host)
	}

	port, err := doc.GetInt("db.port")
	if err != nil {
		t.Fatalf("GetInt(\"db.port\") returned not-ok")
	}
	if port != 5432 {
		t.Fatalf("got %d, want 5432", port)
	}

	name, err := doc.GetString("db.name")
	if err != nil {
		t.Fatalf("GetString(\"db.name\") returned not-ok")
	}
	if name != "mydb" {
		t.Fatalf("got %q, want \"mydb\"", name)
	}
}

func TestDottedKeys_DeeperSharedPrefix(t *testing.T) {
	input := []byte("a.b.x = 1\na.b.y = 2\n")
	doc, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	x, err := doc.GetInt("a.b.x")
	if err != nil {
		t.Fatalf("GetInt(\"a.b.x\") returned not-ok")
	}
	if x != 1 {
		t.Fatalf("got %d, want 1", x)
	}

	y, err := doc.GetInt("a.b.y")
	if err != nil {
		t.Fatalf("GetInt(\"a.b.y\") returned not-ok")
	}
	if y != 2 {
		t.Fatalf("got %d, want 2", y)
	}
}

// Combination: compound table path with dotted keys inside
func TestCompoundTableWithDottedKeys(t *testing.T) {
	input := []byte("[a.b]\nc.x = 10\nc.y = 20\n")
	doc, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	x, err := doc.GetInt("a.b.c.x")
	if err != nil {
		t.Fatalf("GetInt(\"a.b.c.x\") returned not-ok; expected 10")
	}
	if x != 10 {
		t.Fatalf("got %d, want 10", x)
	}

	y, err := doc.GetInt("a.b.c.y")
	if err != nil {
		t.Fatalf("GetInt(\"a.b.c.y\") returned not-ok; expected 20")
	}
	if y != 20 {
		t.Fatalf("got %d, want 20", y)
	}
}
