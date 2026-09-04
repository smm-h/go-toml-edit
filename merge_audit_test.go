package tomledit

import (
	"strings"
	"testing"
)

// Audit 1: defaults three levels deep, verify no overwrite at any level.
func TestAudit_EnsureDefaults_ThreeLevelsDeep(t *testing.T) {
	doc, err := Parse([]byte(`[a]
x = 1

[a.b]
y = 2

[a.b.c]
z = 3
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if _, err := doc.EnsureDefaults([]Default{
		{Path: "a.x", Value: 999},
		{Path: "a.w", Value: 10},
		{Path: "a.b.y", Value: 999},
		{Path: "a.b.v", Value: 20},
		{Path: "a.b.c.z", Value: 999},
		{Path: "a.b.c.u", Value: 30},
	}); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	// Level 1: existing key preserved, new key added
	x, err := doc.GetInt("a.x")
	if err != nil || x != 1 {
		t.Errorf("a.x: got %d (ok=%v), want 1", x, err)
	}
	w, err := doc.GetInt("a.w")
	if err != nil || w != 10 {
		t.Errorf("a.w: got %d (ok=%v), want 10", w, err)
	}

	// Level 2: existing key preserved, new key added
	y, err := doc.GetInt("a.b.y")
	if err != nil || y != 2 {
		t.Errorf("a.b.y: got %d (ok=%v), want 2", y, err)
	}
	v, err := doc.GetInt("a.b.v")
	if err != nil || v != 20 {
		t.Errorf("a.b.v: got %d (ok=%v), want 20", v, err)
	}

	// Level 3: existing key preserved, new key added
	z, err := doc.GetInt("a.b.c.z")
	if err != nil || z != 3 {
		t.Errorf("a.b.c.z: got %d (ok=%v), want 3", z, err)
	}
	u, err := doc.GetInt("a.b.c.u")
	if err != nil || u != 30 {
		t.Errorf("a.b.c.u: got %d (ok=%v), want 30", u, err)
	}

	// Round-trip
	out := doc.Bytes()
	if _, err := Parse(out); err != nil {
		t.Fatalf("round-trip parse failed: %v\noutput:\n%s", err, string(out))
	}
}

// Audit 2: defaults against an existing nested structure -- the document has
// [server.database] with some keys, the defaults add new keys to that table.
func TestAudit_EnsureDefaults_ExistingNestedStructure(t *testing.T) {
	doc, err := Parse([]byte(`[server]
host = "prod.example.com"
port = 443

[server.database]
name = "proddb"
host = "db.example.com"
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if _, err := doc.EnsureDefaults([]Default{
		{Path: "server.host", Value: "localhost"},
		{Path: "server.port", Value: 8080},
		{Path: "server.workers", Value: 4},
		{Path: "server.database.name", Value: "devdb"},
		{Path: "server.database.host", Value: "localhost"},
		{Path: "server.database.port", Value: 5432},
		{Path: "server.database.pool_size", Value: 10},
		{Path: "server.database.ssl", Value: false},
	}); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	// Existing keys unchanged
	host, _ := doc.GetString("server.host")
	if host != "prod.example.com" {
		t.Errorf("server.host overwritten: %q", host)
	}
	port, _ := doc.GetInt("server.port")
	if port != 443 {
		t.Errorf("server.port overwritten: %d", port)
	}
	dbName, _ := doc.GetString("server.database.name")
	if dbName != "proddb" {
		t.Errorf("server.database.name overwritten: %q", dbName)
	}
	dbHost, _ := doc.GetString("server.database.host")
	if dbHost != "db.example.com" {
		t.Errorf("server.database.host overwritten: %q", dbHost)
	}

	// New keys added
	workers, err := doc.GetInt("server.workers")
	if err != nil || workers != 4 {
		t.Errorf("server.workers: got %d (ok=%v), want 4", workers, err)
	}
	dbPort, err := doc.GetInt("server.database.port")
	if err != nil || dbPort != 5432 {
		t.Errorf("server.database.port: got %d (ok=%v), want 5432", dbPort, err)
	}
	poolSize, err := doc.GetInt("server.database.pool_size")
	if err != nil || poolSize != 10 {
		t.Errorf("server.database.pool_size: got %d (ok=%v), want 10", poolSize, err)
	}
	ssl, err := doc.GetBool("server.database.ssl")
	if err != nil || ssl != false {
		t.Errorf("server.database.ssl: got %v (ok=%v), want false", ssl, err)
	}

	// Round-trip
	out := doc.Bytes()
	if _, err := Parse(out); err != nil {
		t.Fatalf("round-trip parse failed: %v\noutput:\n%s", err, string(out))
	}
}

// Audit 3: Merge comments -- source leading comments are appended (not replaced)
// for existing keys.
func TestAudit_Merge_CommentsAppended(t *testing.T) {
	target, err := Parse([]byte(`# Target leading comment 1
# Target leading comment 2
host = "myhost"
`))
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}

	source, err := Parse([]byte(`# Source leading comment A
# Source leading comment B
host = "default"
`))
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	if err := target.Merge(source); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// Value must be unchanged
	host, _ := target.GetString("host")
	if host != "myhost" {
		t.Errorf("host overwritten: %q", host)
	}

	// Both target and source comments must be present
	out := string(target.Bytes())
	if !strings.Contains(out, "Target leading comment 1") {
		t.Errorf("target comment 1 was lost:\n%s", out)
	}
	if !strings.Contains(out, "Target leading comment 2") {
		t.Errorf("target comment 2 was lost:\n%s", out)
	}
	if !strings.Contains(out, "Source leading comment A") {
		t.Errorf("source comment A was not appended:\n%s", out)
	}
	if !strings.Contains(out, "Source leading comment B") {
		t.Errorf("source comment B was not appended:\n%s", out)
	}

	// Target comments should appear before source comments
	idx1 := strings.Index(out, "Target leading comment 2")
	idxA := strings.Index(out, "Source leading comment A")
	if idx1 >= idxA {
		t.Errorf("target comments should appear before source comments:\n%s", out)
	}
}

// Audit 4: Merge new inline table -- source has config = {x = 1}, target doesn't.
func TestAudit_Merge_NewInlineTable(t *testing.T) {
	target, err := Parse([]byte(`name = "app"
`))
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}

	source, err := Parse([]byte(`config = {x = 1, y = "hello"}
`))
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	if err := target.Merge(source); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// The inline table should exist and be queryable
	x, err := target.GetInt("config.x")
	if err != nil || x != 1 {
		t.Errorf("config.x: got %d (ok=%v), want 1", x, err)
	}
	y, err := target.GetString("config.y")
	if err != nil || y != "hello" {
		t.Errorf("config.y: got %q (ok=%v), want %q", y, err, "hello")
	}

	// Round-trip
	out := target.Bytes()
	if _, err := Parse(out); err != nil {
		t.Fatalf("round-trip parse failed: %v\noutput:\n%s", err, string(out))
	}
}

// Audit 5: Merge idempotent -- merging twice produces same result as merging once.
func TestAudit_Merge_Idempotent(t *testing.T) {
	targetInput := `[server]
host = "myhost"
port = 9090
`
	sourceInput := `[server]
host = "default"
workers = 4

[logging]
level = "debug"
`
	target1, err := Parse([]byte(targetInput))
	if err != nil {
		t.Fatalf("parse target1: %v", err)
	}
	source1, err := Parse([]byte(sourceInput))
	if err != nil {
		t.Fatalf("parse source1: %v", err)
	}
	if err := target1.Merge(source1); err != nil {
		t.Fatalf("first Merge: %v", err)
	}
	afterFirst := string(target1.Bytes())

	// Parse the result and merge again
	target2, err := Parse([]byte(afterFirst))
	if err != nil {
		t.Fatalf("parse after first merge: %v", err)
	}
	source2, err := Parse([]byte(sourceInput))
	if err != nil {
		t.Fatalf("parse source2: %v", err)
	}
	if err := target2.Merge(source2); err != nil {
		t.Fatalf("second Merge: %v", err)
	}
	afterSecond := string(target2.Bytes())

	// The results should have the same key-value content.
	// We check by re-parsing and comparing values, since comment merging
	// may accumulate (appending source comments again).
	doc1, _ := Parse([]byte(afterFirst))
	doc2, _ := Parse([]byte(afterSecond))

	changes := Diff(doc1, doc2)
	if len(changes) > 0 {
		t.Errorf("merging twice changed values: %v", changes)
	}

	// All expected values present
	host, _ := target2.GetString("server.host")
	if host != "myhost" {
		t.Errorf("server.host: got %q, want %q", host, "myhost")
	}
	workers, err := target2.GetInt("server.workers")
	if err != nil || workers != 4 {
		t.Errorf("server.workers: got %d (ok=%v), want 4", workers, err)
	}
	level, err := target2.GetString("logging.level")
	if err != nil || level != "debug" {
		t.Errorf("logging.level: got %q (ok=%v), want %q", level, err, "debug")
	}
}

// Audit 6: Merge with array-of-tables -- does it handle [[products]] in the source?
func TestAudit_Merge_ArrayOfTables(t *testing.T) {
	target, err := Parse([]byte(`name = "shop"
`))
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}

	source, err := Parse([]byte(`[[products]]
name = "Hammer"
price = 9.99

[[products]]
name = "Nail"
price = 0.05
`))
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	if err := target.Merge(source); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// The array-of-tables entries should be queryable
	name0, err := target.GetString("products[0].name")
	if err != nil || name0 != "Hammer" {
		t.Errorf("products[0].name: got %q (ok=%v), want %q", name0, err, "Hammer")
	}
	price0, err := target.GetFloat("products[0].price")
	if err != nil || price0 != 9.99 {
		t.Errorf("products[0].price: got %v (ok=%v), want 9.99", price0, err)
	}

	name1, err := target.GetString("products[1].name")
	if err != nil || name1 != "Nail" {
		t.Errorf("products[1].name: got %q (ok=%v), want %q", name1, err, "Nail")
	}

	// Round-trip
	out := target.Bytes()
	if _, err := Parse(out); err != nil {
		t.Fatalf("round-trip parse failed: %v\noutput:\n%s", err, string(out))
	}
}

// Audit 6b: Merge with array-of-tables -- target already has [[products]], skip.
func TestAudit_Merge_ArrayOfTables_ExistingSkipped(t *testing.T) {
	target, err := Parse([]byte(`[[products]]
name = "Existing"
`))
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}

	source, err := Parse([]byte(`[[products]]
name = "Default"
price = 1.00
`))
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	if err := target.Merge(source); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// Target's existing entry should be preserved
	name0, err := target.GetString("products[0].name")
	if err != nil || name0 != "Existing" {
		t.Errorf("products[0].name: got %q (ok=%v), want %q", name0, err, "Existing")
	}

	// Round-trip
	out := target.Bytes()
	if _, err := Parse(out); err != nil {
		t.Fatalf("round-trip parse failed: %v\noutput:\n%s", err, string(out))
	}
}
