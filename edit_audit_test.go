package tomledit

import (
	"strings"
	"testing"
)

// --- Audit Focus 1: Comment/trivia preservation on Set ---

func TestAudit_SetPreservesLeadingAndInlineComments(t *testing.T) {
	src := `# Important setting
host = "localhost"  # server address
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("host", "newhost")
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	out := string(doc.Bytes())
	if !strings.Contains(out, "# Important setting") {
		t.Errorf("leading comment lost after Set. Output:\n%s", out)
	}
	if !strings.Contains(out, "# server address") {
		t.Errorf("inline comment lost after Set. Output:\n%s", out)
	}
	if !strings.Contains(out, `"newhost"`) {
		t.Errorf("value not updated. Output:\n%s", out)
	}

	// Round-trip
	doc2, err := Parse([]byte(out))
	if err != nil {
		t.Fatalf("round-trip parse error: %v\nOutput:\n%s", err, out)
	}
	val, ok := doc2.GetString("host")
	if !ok || val != "newhost" {
		t.Errorf("round-trip: expected 'newhost', got %q (ok=%v)", val, ok)
	}
}

func TestAudit_SetPreservesCommentsInTable(t *testing.T) {
	src := `[server]
# The hostname
host = "localhost"  # main host
port = 8080
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("server.host", "example.com")
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	out := string(doc.Bytes())
	if !strings.Contains(out, "# The hostname") {
		t.Errorf("leading comment lost. Output:\n%s", out)
	}
	if !strings.Contains(out, "# main host") {
		t.Errorf("inline comment lost. Output:\n%s", out)
	}

	roundTripAudit(t, doc)
}

// --- Audit Focus 2: Set with compound types ---

func TestAudit_SetSliceSerializesToValidTOML(t *testing.T) {
	src := `[config]
name = "test"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("config.tags", []any{"alpha", "beta", "gamma"})
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	out := string(doc.Bytes())
	// Verify output contains array syntax
	if !strings.Contains(out, `"alpha"`) || !strings.Contains(out, `"beta"`) {
		t.Errorf("array values not in output:\n%s", out)
	}

	// Must parse back
	doc2 := roundTripAudit(t, doc)
	node, ok := doc2.Lookup("config.tags")
	if !ok {
		t.Fatal("tags not found after round-trip")
	}
	arr, ok := node.(*ArrayNode)
	if !ok {
		t.Fatalf("expected *ArrayNode, got %T", node)
	}
	if len(arr.elements) != 3 {
		t.Errorf("expected 3 elements, got %d", len(arr.elements))
	}
}

func TestAudit_SetMapSerializesToValidTOML(t *testing.T) {
	src := `[config]
name = "test"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("config.metadata", map[string]any{
		"version": "1.0",
		"author":  "someone",
	})
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	out := string(doc.Bytes())
	// Must parse back as valid TOML
	doc2 := roundTripAudit(t, doc)
	if _, ok := doc2.Lookup("config.metadata"); !ok {
		t.Fatalf("metadata not found after round-trip. Output:\n%s", out)
	}
}

func TestAudit_SetTypedSliceIntSerializesToValidTOML(t *testing.T) {
	src := `[data]
label = "numbers"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("data.values", []int{1, 2, 3})
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	doc2 := roundTripAudit(t, doc)
	node, ok := doc2.Lookup("data.values")
	if !ok {
		t.Fatal("values not found after round-trip")
	}
	arr, ok := node.(*ArrayNode)
	if !ok {
		t.Fatalf("expected *ArrayNode, got %T", node)
	}
	if len(arr.elements) != 3 {
		t.Errorf("expected 3 elements, got %d", len(arr.elements))
	}
}

// --- Audit Focus 3: SetCreate table placement ---

func TestAudit_SetCreateTablePlacement(t *testing.T) {
	src := `[existing]
key = "val"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.SetCreate("new.section.key", "hello")
	if err != nil {
		t.Fatalf("SetCreate error: %v", err)
	}

	out := string(doc.Bytes())
	// The new tables should appear in the output
	if !strings.Contains(out, "[new]") && !strings.Contains(out, "[new.section]") {
		t.Errorf("auto-created tables not visible in output:\n%s", out)
	}
	// Existing table should still be present
	if !strings.Contains(out, "[existing]") {
		t.Errorf("existing table lost:\n%s", out)
	}

	doc2 := roundTripAudit(t, doc)
	val, ok := doc2.GetString("new.section.key")
	if !ok || val != "hello" {
		t.Errorf("round-trip: expected 'hello', got %q (ok=%v)", val, ok)
	}
}

// --- Audit Focus 4: Delete edge cases ---

func TestAudit_DeleteOnlyKeyInTable(t *testing.T) {
	src := `[server]
host = "localhost"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Delete("server.host")
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	out := string(doc.Bytes())
	// The table header should remain (empty table)
	if !strings.Contains(out, "[server]") {
		t.Errorf("empty table header was removed. Output:\n%s", out)
	}
	// The key should be gone
	if strings.Contains(out, "host") {
		t.Errorf("key not deleted. Output:\n%s", out)
	}

	roundTripAudit(t, doc)
}

func TestAudit_DeleteArrayOfTablesEntry(t *testing.T) {
	src := `[[products]]
name = "A"

[[products]]
name = "B"

[[products]]
name = "C"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Delete the middle entry
	err = doc.Delete("products[1]")
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	// Index should have shifted: products[0]="A", products[1]="C"
	val0, ok := doc.GetString("products[0].name")
	if !ok || val0 != "A" {
		t.Errorf("expected products[0].name='A', got %q (ok=%v)", val0, ok)
	}
	val1, ok := doc.GetString("products[1].name")
	if !ok || val1 != "C" {
		t.Errorf("expected products[1].name='C', got %q (ok=%v)", val1, ok)
	}

	out := string(doc.Bytes())
	if strings.Count(out, "[[products]]") != 2 {
		t.Errorf("expected 2 [[products]] sections, got %d. Output:\n%s",
			strings.Count(out, "[[products]]"), out)
	}

	roundTripAudit(t, doc)
}

func TestAudit_DeleteFromInlineTable(t *testing.T) {
	src := `config = {a = 1, b = 2, c = 3}
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Delete("config.b")
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	out := string(doc.Bytes())
	// b should be gone, a and c should remain
	doc2 := roundTripAudit(t, doc)
	if doc2.Has("config.b") {
		t.Errorf("config.b should be deleted. Output:\n%s", out)
	}
	va, ok := doc2.GetInt("config.a")
	if !ok || va != 1 {
		t.Errorf("config.a lost after delete. Output:\n%s", out)
	}
	vc, ok := doc2.GetInt("config.c")
	if !ok || vc != 3 {
		t.Errorf("config.c lost after delete. Output:\n%s", out)
	}
}

func TestAudit_DeleteTableItself(t *testing.T) {
	src := `[server]
host = "localhost"

[database]
name = "mydb"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Delete("server")
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	out := string(doc.Bytes())
	if strings.Contains(out, "[server]") {
		t.Errorf("server table should be deleted. Output:\n%s", out)
	}
	if !strings.Contains(out, "[database]") {
		t.Errorf("database table should remain. Output:\n%s", out)
	}

	doc2 := roundTripAudit(t, doc)
	if doc2.Has("server") {
		t.Error("server should not resolve after deletion")
	}
	val, ok := doc2.GetString("database.name")
	if !ok || val != "mydb" {
		t.Errorf("database.name should be preserved, got %q (ok=%v)", val, ok)
	}
}

// --- Audit Focus 5: RenameKey edge cases ---

func TestAudit_RenameQuotedKey(t *testing.T) {
	src := `[server]
"host.name" = "localhost"
port = 8080
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Rename a key that was originally quoted
	err = doc.RenameKey(`server."host.name"`, "hostname")
	if err != nil {
		t.Fatalf("RenameKey error: %v", err)
	}

	val, ok := doc.GetString("server.hostname")
	if !ok || val != "localhost" {
		t.Errorf("expected 'localhost', got %q (ok=%v)", val, ok)
	}

	roundTripAudit(t, doc)
}

func TestAudit_RenameToKeyNeedingQuotes(t *testing.T) {
	src := `[server]
host = "localhost"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Rename to a key that contains a dot (needs quoting)
	err = doc.RenameKey("server.host", "host.name")
	if err != nil {
		t.Fatalf("RenameKey error: %v", err)
	}

	out := string(doc.Bytes())
	// The new key should be quoted in the output since it contains a dot
	if !strings.Contains(out, `"host.name"`) {
		t.Errorf("key with dot should be quoted. Output:\n%s", out)
	}

	// Round-trip: parse the output and verify the key resolves
	doc2 := roundTripAudit(t, doc)
	val, ok := doc2.GetString(`server."host.name"`)
	if !ok || val != "localhost" {
		t.Errorf("round-trip: expected 'localhost', got %q (ok=%v). Output:\n%s", val, ok, out)
	}
}

func TestAudit_RenamePreservesTrivia(t *testing.T) {
	src := `[server]
# The main host
host = "localhost"  # server address
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.RenameKey("server.host", "address")
	if err != nil {
		t.Fatalf("RenameKey error: %v", err)
	}

	out := string(doc.Bytes())
	if !strings.Contains(out, "# The main host") {
		t.Errorf("leading comment lost after rename. Output:\n%s", out)
	}
	if !strings.Contains(out, "# server address") {
		t.Errorf("inline comment lost after rename. Output:\n%s", out)
	}
	if !strings.Contains(out, "address") {
		t.Errorf("new key name not in output. Output:\n%s", out)
	}

	roundTripAudit(t, doc)
}

// --- Audit Focus 6: Interaction between operations ---

func TestAudit_SetDeleteSetSamePath(t *testing.T) {
	src := `[config]
key = "original"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Set
	err = doc.Set("config.key", "first")
	if err != nil {
		t.Fatalf("first Set error: %v", err)
	}
	v, ok := doc.GetString("config.key")
	if !ok || v != "first" {
		t.Fatalf("after first Set: expected 'first', got %q", v)
	}

	// Delete
	err = doc.Delete("config.key")
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if doc.Has("config.key") {
		t.Fatal("key should be deleted")
	}

	// Set again (re-create)
	err = doc.Set("config.key", "second")
	if err != nil {
		t.Fatalf("second Set error: %v", err)
	}
	v, ok = doc.GetString("config.key")
	if !ok || v != "second" {
		t.Fatalf("after second Set: expected 'second', got %q", v)
	}

	roundTripAudit(t, doc)
}

func TestAudit_NewTableThenSetThenRename(t *testing.T) {
	src := `[existing]
a = 1
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.NewTable("newtable")
	if err != nil {
		t.Fatalf("NewTable error: %v", err)
	}

	err = doc.Set("newtable.key1", "value1")
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	err = doc.RenameKey("newtable.key1", "renamed_key")
	if err != nil {
		t.Fatalf("RenameKey error: %v", err)
	}

	val, ok := doc.GetString("newtable.renamed_key")
	if !ok || val != "value1" {
		t.Errorf("expected 'value1', got %q (ok=%v)", val, ok)
	}

	out := string(doc.Bytes())
	if !strings.Contains(out, "[newtable]") {
		t.Errorf("newtable missing from output:\n%s", out)
	}

	roundTripAudit(t, doc)
}

// --- Audit Focus 7: No surprising shared state (basic check) ---

func TestAudit_TwoDocumentsIndependent(t *testing.T) {
	src := `[server]
host = "localhost"
`
	doc1, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	doc2, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Modify doc1
	err = doc1.Set("server.host", "modified")
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	// doc2 should be unaffected
	val, ok := doc2.GetString("server.host")
	if !ok || val != "localhost" {
		t.Errorf("doc2 was affected by doc1 mutation: got %q", val)
	}
}

// --- Audit Focus 8: Setting values in array-of-tables entries ---

func TestAudit_SetInArrayOfTablesEntry(t *testing.T) {
	src := `[[products]]
name = "Widget"
price = 9.99

[[products]]
name = "Gadget"
price = 19.99
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("products[0].name", "NewName")
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	val, ok := doc.GetString("products[0].name")
	if !ok || val != "NewName" {
		t.Errorf("expected 'NewName', got %q (ok=%v)", val, ok)
	}

	// Verify the other entry is unchanged
	val2, ok := doc.GetString("products[1].name")
	if !ok || val2 != "Gadget" {
		t.Errorf("products[1] should be unchanged, got %q (ok=%v)", val2, ok)
	}

	roundTripAudit(t, doc)
}

func TestAudit_AddNewKeyToArrayOfTablesEntry(t *testing.T) {
	src := `[[products]]
name = "Widget"

[[products]]
name = "Gadget"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("products[1].color", "blue")
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	val, ok := doc.GetString("products[1].color")
	if !ok || val != "blue" {
		t.Errorf("expected 'blue', got %q (ok=%v)", val, ok)
	}

	// Verify first entry is unchanged
	if doc.Has("products[0].color") {
		t.Error("products[0].color should not exist")
	}

	roundTripAudit(t, doc)
}

// --- Additional edge case tests ---

func TestAudit_SetNilReturnsError(t *testing.T) {
	src := `key = "val"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("key", nil)
	if err == nil {
		t.Error("Set with nil should return error")
	}
}

func TestAudit_SetUnsupportedTypeReturnsError(t *testing.T) {
	src := `key = "val"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("key", struct{ X int }{42})
	if err == nil {
		t.Error("Set with unsupported type should return error")
	}
}

func TestAudit_DeleteFromEmptyArray(t *testing.T) {
	src := `arr = []
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Should be silent no-op
	err = doc.Delete("arr[0]")
	if err != nil {
		t.Fatalf("Delete from empty array should be silent no-op, got: %v", err)
	}
}

func TestAudit_DeleteNonExistentPath(t *testing.T) {
	src := `[server]
host = "localhost"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Completely non-existent path
	err = doc.Delete("nonexistent.deep.path")
	if err != nil {
		t.Fatalf("Delete non-existent path should be silent no-op, got: %v", err)
	}
}

func TestAudit_RenameArrayIndexReturnsError(t *testing.T) {
	src := `arr = [1, 2, 3]
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.RenameKey("arr[0]", "newname")
	if err == nil {
		t.Error("RenameKey with index segment should return error")
	}
}

func TestAudit_NewTableWithIndexReturnsError(t *testing.T) {
	src := ``
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.NewTable("a[0]")
	if err == nil {
		t.Error("NewTable with index segment should return error")
	}
}

func TestAudit_NewArrayTableWithIndexReturnsError(t *testing.T) {
	src := ``
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.NewArrayTable("a[0]")
	if err == nil {
		t.Error("NewArrayTable with index segment should return error")
	}
}

func TestAudit_SetEmptyPathReturnsError(t *testing.T) {
	src := `key = "val"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("", "value")
	if err == nil {
		t.Error("Set with empty path should return error")
	}
}

func TestAudit_DeleteEmptyPathReturnsError(t *testing.T) {
	src := `key = "val"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Delete("")
	if err == nil {
		t.Error("Delete with empty path should return error")
	}
}

func TestAudit_SetAllIntUintTypes(t *testing.T) {
	src := `[types]
dummy = 0
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	intVals := []struct {
		key string
		val any
	}{
		{"types.i8", int8(8)},
		{"types.i16", int16(16)},
		{"types.i32", int32(32)},
		{"types.i64", int64(64)},
		{"types.u8", uint8(8)},
		{"types.u16", uint16(16)},
		{"types.u32", uint32(32)},
		{"types.u64", uint64(0)}, // use 0 to avoid overflow
		{"types.u", uint(42)},
		{"types.f32", float32(1.5)},
	}

	for _, iv := range intVals {
		err = doc.Set(iv.key, iv.val)
		if err != nil {
			t.Fatalf("Set(%q, %v) error: %v", iv.key, iv.val, err)
		}
	}

	roundTripAudit(t, doc)
}

func TestAudit_ValueToNodeNodePassthrough(t *testing.T) {
	n := &IntegerNode{val: scalarOf[int64](42)}
	n.markDirty()

	result, err := valueToNode(n)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != n {
		t.Error("Node passthrough should return the same node")
	}
}

func TestAudit_SetOverwritePreservesOtherKeys(t *testing.T) {
	src := `[server]
host = "localhost"
port = 8080
debug = true
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("server.host", "example.com")
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	// Other keys should be unchanged
	port, ok := doc.GetInt("server.port")
	if !ok || port != 8080 {
		t.Errorf("port should be unchanged, got %d (ok=%v)", port, ok)
	}
	debug, ok := doc.GetBool("server.debug")
	if !ok || !debug {
		t.Errorf("debug should be unchanged, got %v (ok=%v)", debug, ok)
	}

	roundTripAudit(t, doc)
}

func TestAudit_DeleteAllArrayOfTablesEntries(t *testing.T) {
	src := `[[items]]
name = "A"

[[items]]
name = "B"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Delete both entries
	err = doc.Delete("items[0]")
	if err != nil {
		t.Fatalf("Delete [0] error: %v", err)
	}
	err = doc.Delete("items[0]") // was index 1, now index 0
	if err != nil {
		t.Fatalf("Delete [0] again error: %v", err)
	}

	out := string(doc.Bytes())
	if strings.Contains(out, "[[items]]") {
		t.Errorf("all items should be deleted. Output:\n%s", out)
	}

	roundTripAudit(t, doc)
}

func TestAudit_NewTableDuplicateDetection(t *testing.T) {
	src := `[server]
host = "localhost"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.NewTable("server")
	if err == nil {
		t.Error("NewTable should error on duplicate")
	}
}

func TestAudit_NewArrayTableMultipleSameKey(t *testing.T) {
	doc, err := Parse([]byte(""))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Multiple NewArrayTable with same key is valid
	err = doc.NewArrayTable("items")
	if err != nil {
		t.Fatalf("first NewArrayTable error: %v", err)
	}
	err = doc.NewArrayTable("items")
	if err != nil {
		t.Fatalf("second NewArrayTable error: %v", err)
	}

	out := string(doc.Bytes())
	if strings.Count(out, "[[items]]") != 2 {
		t.Errorf("expected 2 [[items]] sections. Output:\n%s", out)
	}

	roundTripAudit(t, doc)
}

func TestAudit_SetCreateNestedInExistingTable(t *testing.T) {
	src := `[server]
host = "localhost"
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// SetCreate a path that partially exists
	err = doc.SetCreate("server.logging.file", "/var/log/app.log")
	if err != nil {
		t.Fatalf("SetCreate error: %v", err)
	}

	val, ok := doc.GetString("server.logging.file")
	if !ok || val != "/var/log/app.log" {
		t.Errorf("expected '/var/log/app.log', got %q (ok=%v)", val, ok)
	}

	roundTripAudit(t, doc)
}

func TestAudit_SetInlineArrayElement(t *testing.T) {
	src := `arr = [1, 2, 3]
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("arr[1]", 42)
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	doc2 := roundTripAudit(t, doc)
	node, ok := doc2.Lookup("arr[1]")
	if !ok {
		t.Fatal("arr[1] not found after round-trip")
	}
	if node.(Scalar).Value() != int64(42) {
		t.Errorf("expected 42, got %v", node.(Scalar).Value())
	}
}

func TestAudit_DeleteInlineArrayElement(t *testing.T) {
	src := `arr = [10, 20, 30]
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Delete("arr[1]")
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	doc2 := roundTripAudit(t, doc)
	node, ok := doc2.Lookup("arr")
	if !ok {
		t.Fatal("arr not found")
	}
	arr, ok := node.(*ArrayNode)
	if !ok {
		t.Fatalf("expected *ArrayNode, got %T", node)
	}
	if len(arr.elements) != 2 {
		t.Errorf("expected 2 elements, got %d", len(arr.elements))
	}
	if arr.elements[0].(Scalar).Value() != int64(10) {
		t.Errorf("expected arr[0]=10, got %v", arr.elements[0].(Scalar).Value())
	}
	if arr.elements[1].(Scalar).Value() != int64(30) {
		t.Errorf("expected arr[1]=30, got %v", arr.elements[1].(Scalar).Value())
	}
}

// --- Bug: inline value dirty propagation ---

func TestAudit_SetExistingKeyInInlineTableRoundTrip(t *testing.T) {
	// This tests the known bug: modifying an existing key in an inline table
	// does not propagate dirty to the wrapping KV node, so Bytes() emits
	// original raw bytes.
	src := `config = {x = 1, y = 2}
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("config.x", 99)
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	doc2 := roundTripAudit(t, doc)
	val, ok := doc2.GetInt("config.x")
	if !ok || val != 99 {
		t.Errorf("expected config.x=99 after round-trip, got %d (ok=%v)", val, ok)
	}
}

func TestAudit_SetNewKeyInInlineTableRoundTrip(t *testing.T) {
	// Same bug as above but for adding a NEW key to an inline table.
	src := `[nested]
inline = {x = 1, y = 2}
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = doc.Set("nested.inline.z", 3)
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	doc2 := roundTripAudit(t, doc)
	val, ok := doc2.GetInt("nested.inline.z")
	if !ok || val != 3 {
		t.Errorf("expected nested.inline.z=3 after round-trip, got %d (ok=%v)", val, ok)
	}
}

// --- Helper ---

func roundTripAudit(t *testing.T, doc *Document) *Document {
	t.Helper()
	out := doc.Bytes()
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("round-trip failed: output is not valid TOML:\n%s\nparse error: %v", string(out), err)
	}
	return doc2
}
