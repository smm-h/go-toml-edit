package tomledit

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const editTestDocument = `# Config file
[server]
host = "localhost"
port = 8080

[server.database]
name = "mydb"

[[products]]
name = "Widget"
price = 9.99

[[products]]
name = "Gadget"
price = 19.99
`

func parseEditTestDoc(t *testing.T) *Document {
	t.Helper()
	doc, err := Parse([]byte(editTestDocument))
	if err != nil {
		t.Fatalf("failed to parse test document: %v", err)
	}
	return doc
}

// roundTrip serializes the document, re-parses it, and returns the new doc.
// It also verifies the output is valid TOML.
func roundTrip(t *testing.T, doc *Document) *Document {
	t.Helper()
	out := doc.Bytes()
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("round-trip failed: output is not valid TOML:\n%s\nparse error: %v", string(out), err)
	}
	return doc2
}

// --- Set tests ---

func TestSet_ExistingValue(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.Set("server.host", "newhost")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	val, err := doc.GetString("server.host")
	if err != nil {
		t.Fatalf("GetString after Set: %v", err)
	}
	if val != "newhost" {
		t.Errorf("expected \"newhost\", got %q", val)
	}

	// Verify the comment at the top is preserved.
	out := string(doc.Bytes())
	if !strings.Contains(out, "# Config file") {
		t.Error("leading comment was lost after Set")
	}

	// Round-trip.
	doc2 := roundTrip(t, doc)
	val2, err := doc2.GetString("server.host")
	if err != nil || val2 != "newhost" {
		t.Errorf("round-trip: expected \"newhost\", got %q (ok=%v)", val2, err)
	}
}

func TestSet_DifferentType(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.Set("server.port", "not-a-number")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	val, err := doc.GetString("server.port")
	if err != nil {
		t.Fatalf("GetString after type change: %v", err)
	}
	if val != "not-a-number" {
		t.Errorf("expected \"not-a-number\", got %q", val)
	}
	roundTrip(t, doc)
}

func TestSet_AddNewKeyToExistingTable(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.Set("server.workers", 4)
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	val, err := doc.GetInt("server.workers")
	if err != nil {
		t.Fatalf("GetInt for new key: %v", err)
	}
	if val != 4 {
		t.Errorf("expected 4, got %d", val)
	}

	out := string(doc.Bytes())
	if !strings.Contains(out, "workers = 4") {
		t.Errorf("output does not contain new key:\n%s", out)
	}
	roundTrip(t, doc)
}

func TestSet_ErrorOnMissingIntermediate(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.Set("new.section.key", "val")
	if err == nil {
		t.Fatal("Set should return error when intermediates are missing")
	}
}

func TestSetCreate_AutoCreates(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.SetCreate("new.section.key", "val")
	if err != nil {
		t.Fatalf("SetCreate returned error: %v", err)
	}
	val, err := doc.GetString("new.section.key")
	if err != nil {
		t.Fatalf("GetString after SetCreate: %v", err)
	}
	if val != "val" {
		t.Errorf("expected \"val\", got %q", val)
	}

	out := string(doc.Bytes())
	if !strings.Contains(out, "[new.section]") && !strings.Contains(out, "[new]") {
		t.Errorf("output does not contain created table:\n%s", out)
	}
	roundTrip(t, doc)
}

func TestSet_AllPrimitiveTypes(t *testing.T) {
	doc := parseEditTestDoc(t)

	tests := []struct {
		key   string
		value any
		check func(t *testing.T, doc *Document)
	}{
		{
			key: "server.str", value: "hello",
			check: func(t *testing.T, doc *Document) {
				v, err := doc.GetString("server.str")
				if err != nil || v != "hello" {
					t.Errorf("string: expected \"hello\", got %q (ok=%v)", v, err)
				}
			},
		},
		{
			key: "server.num", value: 42,
			check: func(t *testing.T, doc *Document) {
				v, err := doc.GetInt("server.num")
				if err != nil || v != 42 {
					t.Errorf("int: expected 42, got %d (ok=%v)", v, err)
				}
			},
		},
		{
			key: "server.pi", value: 3.14,
			check: func(t *testing.T, doc *Document) {
				v, err := doc.GetFloat("server.pi")
				if err != nil || v != 3.14 {
					t.Errorf("float: expected 3.14, got %f (ok=%v)", v, err)
				}
			},
		},
		{
			key: "server.flag", value: true,
			check: func(t *testing.T, doc *Document) {
				v, err := doc.GetBool("server.flag")
				if err != nil || v != true {
					t.Errorf("bool: expected true, got %v (ok=%v)", v, err)
				}
			},
		},
		{
			key: "server.created", value: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			check: func(t *testing.T, doc *Document) {
				v, err := doc.GetTime("server.created")
				if err != nil {
					t.Errorf("time: GetTime: %v", err)
					return
				}
				if v.Year() != 2024 || v.Month() != 1 || v.Day() != 15 {
					t.Errorf("time: unexpected value %v", v)
				}
			},
		},
	}

	for _, tt := range tests {
		err := doc.Set(tt.key, tt.value)
		if err != nil {
			t.Fatalf("Set(%q) returned error: %v", tt.key, err)
		}
		tt.check(t, doc)
	}

	roundTrip(t, doc)
}

func TestSet_Array(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.Set("server.tags", []any{"web", "prod"})
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	node, ok := doc.Lookup("server.tags")
	if !ok {
		t.Fatal("Get returned nil for tags")
	}
	arr, ok := node.(*ArrayNode)
	if !ok {
		t.Fatalf("expected *ArrayNode, got %T", node)
	}
	if len(arr.elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(arr.elements))
	}

	out := string(doc.Bytes())
	if !strings.Contains(out, `"web"`) || !strings.Contains(out, `"prod"`) {
		t.Errorf("output does not contain array values:\n%s", out)
	}
	roundTrip(t, doc)
}

func TestSet_Map(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.Set("server.metadata", map[string]any{"version": "1.0"})
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	node, ok := doc.Lookup("server.metadata")
	if !ok {
		t.Fatal("Get returned nil for metadata")
	}
	tbl, ok := node.(*InlineTableNode)
	if !ok {
		t.Fatalf("expected *InlineTableNode, got %T", node)
	}
	if len(tbl.children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tbl.children))
	}

	out := string(doc.Bytes())
	if !strings.Contains(out, "version") || !strings.Contains(out, `"1.0"`) {
		t.Errorf("output does not contain map values:\n%s", out)
	}
	roundTrip(t, doc)
}

// Fails if a node resolved out of a document can be written back in as a value.
// Copying a value from one place to another is a read followed by a write of
// the VALUE; handing the node itself over would put one node in two places.
func TestSet_RefusesANodeAsAValue(t *testing.T) {
	doc := parseEditTestDoc(t)

	srcNode, ok := doc.Lookup("server.host")
	if !ok {
		t.Fatal("source node is nil")
	}
	if err := doc.Set("server.backup_host", srcNode); !errors.Is(err, ErrBadInput) {
		t.Errorf("Set accepted a node as a value: %v", err)
	}

	// The supported spelling: read the value, write the value.
	val, err := doc.GetString("server.host")
	if err != nil {
		t.Fatalf("GetString on the source key: %v", err)
	}
	if err := doc.Set("server.backup_host", val); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	copied, err := doc.GetString("server.backup_host")
	if err != nil || copied != "localhost" {
		t.Errorf("expected \"localhost\", got %q (err=%v)", copied, err)
	}
	roundTrip(t, doc)
}

func TestSet_InArrayElement(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.Set("products[0].name", "SuperWidget")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	val, err := doc.GetString("products[0].name")
	if err != nil {
		t.Fatalf("GetString: %v", err)
	}
	if val != "SuperWidget" {
		t.Errorf("expected \"SuperWidget\", got %q", val)
	}
	roundTrip(t, doc)
}

func TestSet_NegativeIndex(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.Set("products[-1].name", "MegaGadget")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	val, err := doc.GetString("products[-1].name")
	if err != nil {
		t.Fatalf("GetString: %v", err)
	}
	if val != "MegaGadget" {
		t.Errorf("expected \"MegaGadget\", got %q", val)
	}
	roundTrip(t, doc)
}

func TestSet_InInlineTable(t *testing.T) {
	src := `[nested]
inline = {x = 1, y = 2}
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = doc.Set("nested.inline.z", 3)
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	val, err := doc.GetInt("nested.inline.z")
	if err != nil {
		t.Fatalf("GetInt for new key in inline table: %v", err)
	}
	if val != 3 {
		t.Errorf("expected 3, got %d", val)
	}
	roundTrip(t, doc)
}

// --- Delete tests ---

func TestDelete_ExistingKey(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.Delete("server.port")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	// Port should be gone.
	_, err = doc.GetInt("server.port")
	if err == nil {
		t.Error("server.port should be deleted")
	}

	// Host should still be there.
	val, err := doc.GetString("server.host")
	if err != nil || val != "localhost" {
		t.Error("server.host should be preserved")
	}

	roundTrip(t, doc)
}

func TestDelete_NonExistent(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.Delete("server.nonexistent")
	if err != nil {
		t.Fatalf("Delete of non-existent key should not error, got: %v", err)
	}
	// Document should be unchanged.
	val, err := doc.GetString("server.host")
	if err != nil || val != "localhost" {
		t.Error("document should be unchanged")
	}
}

func TestDelete_ArrayElement(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.Delete("products[0]")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	// The first product should now be Gadget (formerly at index 1).
	val, err := doc.GetString("products[0].name")
	if err != nil {
		t.Fatalf("GetString after delete: %v", err)
	}
	if val != "Gadget" {
		t.Errorf("expected \"Gadget\" at index 0, got %q", val)
	}

	roundTrip(t, doc)
}

func TestDelete_NegativeIndex(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.Delete("products[-1]")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	// Only Widget should remain.
	val, err := doc.GetString("products[0].name")
	if err != nil {
		t.Fatalf("GetString: %v", err)
	}
	if val != "Widget" {
		t.Errorf("expected \"Widget\", got %q", val)
	}

	// Index 1 should not exist.
	if _, ok := doc.Lookup("products[1].name"); ok {
		t.Error("products[1] should not exist after deleting [-1]")
	}

	roundTrip(t, doc)
}

func TestDelete_RoundTrip(t *testing.T) {
	doc := parseEditTestDoc(t)
	_ = doc.Delete("server.port")
	doc2 := roundTrip(t, doc)

	_, err := doc2.GetInt("server.port")
	if err == nil {
		t.Error("port should be absent after round-trip")
	}

	val, err := doc2.GetString("server.host")
	if err != nil || val != "localhost" {
		t.Error("host should be preserved after round-trip")
	}
}

// --- RenameKey tests ---

func TestRename_ExistingKey(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.RenameKey("server.host", "address")
	if err != nil {
		t.Fatalf("RenameKey returned error: %v", err)
	}

	// Old key should be gone.
	_, err = doc.GetString("server.host")
	if err == nil {
		t.Error("server.host should no longer exist")
	}

	// New key should have the value.
	val, err := doc.GetString("server.address")
	if err != nil {
		t.Fatalf("GetString for renamed key: %v", err)
	}
	if val != "localhost" {
		t.Errorf("expected \"localhost\", got %q", val)
	}

	roundTrip(t, doc)
}

func TestRename_ToExistingKey(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.RenameKey("server.host", "port")
	if err == nil {
		t.Fatal("RenameKey to existing key should return error")
	}
}

func TestRename_NonExistent(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.RenameKey("server.nonexistent", "x")
	if err == nil {
		t.Fatal("RenameKey of non-existent key should return error")
	}
}

func TestRename_RoundTrip(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.RenameKey("server.host", "address")
	if err != nil {
		t.Fatalf("RenameKey returned error: %v", err)
	}
	doc2 := roundTrip(t, doc)

	val, err := doc2.GetString("server.address")
	if err != nil || val != "localhost" {
		t.Errorf("round-trip: expected \"localhost\", got %q (ok=%v)", val, err)
	}

	_, err = doc2.GetString("server.host")
	if err == nil {
		t.Error("round-trip: server.host should not exist")
	}
}

// --- NewTable tests ---

func TestNewTable_Create(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.NewTable("logging")
	if err != nil {
		t.Fatalf("NewTable returned error: %v", err)
	}

	out := string(doc.Bytes())
	if !strings.Contains(out, "[logging]") {
		t.Errorf("output does not contain [logging]:\n%s", out)
	}
	roundTrip(t, doc)
}

func TestNewTable_Duplicate(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.NewTable("server")
	if err == nil {
		t.Fatal("NewTable should return error for duplicate table")
	}
}

func TestNewTable_ThenSet(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.NewTable("logging")
	if err != nil {
		t.Fatalf("NewTable returned error: %v", err)
	}
	err = doc.Set("logging.level", "debug")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	val, err := doc.GetString("logging.level")
	if err != nil || val != "debug" {
		t.Errorf("expected \"debug\", got %q (ok=%v)", val, err)
	}

	out := string(doc.Bytes())
	if !strings.Contains(out, "[logging]") {
		t.Errorf("output does not contain [logging]:\n%s", out)
	}
	if !strings.Contains(out, `level = "debug"`) {
		t.Errorf("output does not contain level:\n%s", out)
	}
	roundTrip(t, doc)
}

// --- NewArrayTable tests ---

func TestNewArrayTable_Append(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.NewArrayTable("products")
	if err != nil {
		t.Fatalf("NewArrayTable returned error: %v", err)
	}

	out := string(doc.Bytes())
	// Should have three [[products]] sections.
	count := strings.Count(out, "[[products]]")
	if count != 3 {
		t.Errorf("expected 3 [[products]] sections, got %d in:\n%s", count, out)
	}
	roundTrip(t, doc)
}

func TestNewArrayTable_ThenSet(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.NewArrayTable("products")
	if err != nil {
		t.Fatalf("NewArrayTable returned error: %v", err)
	}
	err = doc.Set("products[-1].name", "NewProduct")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	val, err := doc.GetString("products[-1].name")
	if err != nil {
		t.Fatalf("GetString: %v", err)
	}
	if val != "NewProduct" {
		t.Errorf("expected \"NewProduct\", got %q", val)
	}
	roundTrip(t, doc)
}

func TestNewArrayTable_MultipleAppends(t *testing.T) {
	doc := parseEditTestDoc(t)
	for i := 0; i < 3; i++ {
		err := doc.NewArrayTable("products")
		if err != nil {
			t.Fatalf("NewArrayTable #%d returned error: %v", i, err)
		}
	}

	out := string(doc.Bytes())
	count := strings.Count(out, "[[products]]")
	if count != 5 { // 2 original + 3 new
		t.Errorf("expected 5 [[products]] sections, got %d", count)
	}
	roundTrip(t, doc)
}

// --- valueToNode tests ---

func TestValueToNode_UnsupportedType(t *testing.T) {
	_, err := valueToNode(struct{}{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestValueToNode_Nil(t *testing.T) {
	_, err := valueToNode(nil)
	if err == nil {
		t.Fatal("expected error for nil")
	}
}

func TestValueToNode_TypedSlice(t *testing.T) {
	n, err := valueToNode([]string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	arr, ok := n.(*ArrayNode)
	if !ok {
		t.Fatalf("expected *ArrayNode, got %T", n)
	}
	if len(arr.elements) != 3 {
		t.Errorf("expected 3 elements, got %d", len(arr.elements))
	}
}

func TestValueToNode_LocalDate(t *testing.T) {
	ld := LocalDate{Year: 2024, Month: 6, Day: 15}
	n, err := valueToNode(ld)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if _, ok := n.(*LocalDateNode); !ok {
		t.Fatalf("expected *LocalDateNode, got %T", n)
	}
}

func TestValueToNode_LocalTime(t *testing.T) {
	lt := LocalTime{Hour: 14, Minute: 30, Second: 0}
	n, err := valueToNode(lt)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if _, ok := n.(*LocalTimeNode); !ok {
		t.Fatalf("expected *LocalTimeNode, got %T", n)
	}
}

func TestValueToNode_LocalDateTime(t *testing.T) {
	ldt := LocalDateTime{Year: 2024, Month: 6, Day: 15, Hour: 14, Minute: 30}
	n, err := valueToNode(ldt)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if _, ok := n.(*LocalDateTimeNode); !ok {
		t.Fatalf("expected *LocalDateTimeNode, got %T", n)
	}
}

// Fails if a caller-built node is accepted as a value again. Values enter as Go
// values and the library renders them; a node handed in from outside carries a
// span, a lexeme and a parent from wherever it came from, none of which
// describe the document it would be written into.
func TestValueToNode_RefusesACallerBuiltNode(t *testing.T) {
	original := &StringNode{val: scalarOf("test")}
	original.markDirty()
	if _, err := valueToNode(original); !errors.Is(err, ErrBadInput) {
		t.Errorf("a caller-built node was accepted as a value: %v", err)
	}
}

// --- Integration / comprehensive round-trip tests ---

func TestSet_RoundTrip_Comprehensive(t *testing.T) {
	doc := parseEditTestDoc(t)

	// Set multiple values.
	_ = doc.Set("server.host", "example.com")
	_ = doc.Set("server.port", 9090)
	_ = doc.Set("server.workers", 8)
	_ = doc.Set("products[0].name", "SuperWidget")

	doc2 := roundTrip(t, doc)

	checks := []struct {
		path string
		want any
	}{
		{"server.host", "example.com"},
		{"server.port", int64(9090)},
		{"server.workers", int64(8)},
		{"products[0].name", "SuperWidget"},
		// Unchanged values should survive.
		{"server.database.name", "mydb"},
		{"products[1].name", "Gadget"},
	}

	for _, c := range checks {
		node, ok := doc2.Lookup(c.path)
		if !ok {
			t.Errorf("Lookup(%q) returned nil after round-trip", c.path)
			continue
		}
		got := node.(Scalar).Value()
		if got != c.want {
			t.Errorf("Lookup(%q) = %v (%T), want %v (%T)", c.path, got, got, c.want, c.want)
		}
	}
}

func TestSetCreate_DeepPath(t *testing.T) {
	doc := parseEditTestDoc(t)
	err := doc.SetCreate("a.b.c.d", "deep")
	if err != nil {
		t.Fatalf("SetCreate returned error: %v", err)
	}
	val, err := doc.GetString("a.b.c.d")
	if err != nil || val != "deep" {
		t.Errorf("expected \"deep\", got %q (ok=%v)", val, err)
	}

	doc2 := roundTrip(t, doc)
	val2, err := doc2.GetString("a.b.c.d")
	if err != nil || val2 != "deep" {
		t.Errorf("round-trip: expected \"deep\", got %q (ok=%v)", val2, err)
	}
}
