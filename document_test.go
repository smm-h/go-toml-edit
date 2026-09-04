package tomledit

import (
	"testing"
)

const testDocument = `# Server configuration
[server]
host = "localhost"
port = 8080
debug = true
rate = 3.14

[server.database]
name = "mydb"
timeout = 30

[[products]]
name = "Widget"
price = 9.99

[[products]]
name = "Gadget"
price = 19.99

[dates]
created = 1979-05-27T07:32:00Z
updated = 1979-05-27T07:32:00
birthday = 1979-05-27
alarm = 07:32:00

[nested]
inline = {x = 1, y = 2}
array = [1, 2, 3]
`

func parseTestDoc(t *testing.T) *Document {
	t.Helper()
	doc, err := Parse([]byte(testDocument))
	if err != nil {
		t.Fatalf("failed to parse test document: %v", err)
	}
	return doc
}

func TestGet_ServerHost(t *testing.T) {
	doc := parseTestDoc(t)
	node, ok := doc.Lookup("server.host")
	if !ok {
		t.Fatal("Lookup(\"server.host\") returned nil")
	}
	s, ok := node.(*StringNode)
	if !ok {
		t.Fatalf("expected *StringNode, got %T", node)
	}
	if s.val.get() != "localhost" {
		t.Errorf("expected \"localhost\", got %q", s.val.get())
	}
}

func TestGetString_ServerHost(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetString("server.host")
	if err != nil {
		t.Fatalf("GetString(\"server.host\"): %v", err)
	}
	if val != "localhost" {
		t.Errorf("expected \"localhost\", got %q", val)
	}
}

func TestGetInt_ServerPort(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetInt("server.port")
	if err != nil {
		t.Fatalf("GetInt(\"server.port\"): %v", err)
	}
	if val != 8080 {
		t.Errorf("expected 8080, got %d", val)
	}
}

func TestGetBool_ServerDebug(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetBool("server.debug")
	if err != nil {
		t.Fatalf("GetBool(\"server.debug\"): %v", err)
	}
	if val != true {
		t.Errorf("expected true, got %v", val)
	}
}

func TestGetFloat_ServerRate(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetFloat("server.rate")
	if err != nil {
		t.Fatalf("GetFloat(\"server.rate\"): %v", err)
	}
	if val != 3.14 {
		t.Errorf("expected 3.14, got %f", val)
	}
}

func TestGetString_ServerDatabaseName(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetString("server.database.name")
	if err != nil {
		t.Fatalf("GetString(\"server.database.name\"): %v", err)
	}
	if val != "mydb" {
		t.Errorf("expected \"mydb\", got %q", val)
	}
}

func TestGetString_Products0Name(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetString("products[0].name")
	if err != nil {
		t.Fatalf("GetString(\"products[0].name\"): %v", err)
	}
	if val != "Widget" {
		t.Errorf("expected \"Widget\", got %q", val)
	}
}

func TestGetString_Products1Name(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetString("products[1].name")
	if err != nil {
		t.Fatalf("GetString(\"products[1].name\"): %v", err)
	}
	if val != "Gadget" {
		t.Errorf("expected \"Gadget\", got %q", val)
	}
}

func TestGetString_ProductsNeg1Name(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetString("products[-1].name")
	if err != nil {
		t.Fatalf("GetString(\"products[-1].name\"): %v", err)
	}
	if val != "Gadget" {
		t.Errorf("expected \"Gadget\", got %q", val)
	}
}

func TestGetFloat_Products0Price(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetFloat("products[0].price")
	if err != nil {
		t.Fatalf("GetFloat(\"products[0].price\"): %v", err)
	}
	if val != 9.99 {
		t.Errorf("expected 9.99, got %f", val)
	}
}

func TestGetTime_DatesCreated(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetTime("dates.created")
	if err != nil {
		t.Fatalf("GetTime(\"dates.created\"): %v", err)
	}
	if val.Year() != 1979 || val.Month() != 5 || val.Day() != 27 {
		t.Errorf("unexpected time: %v", val)
	}
}

func TestGet_DatesUpdated(t *testing.T) {
	doc := parseTestDoc(t)
	node, ok := doc.Lookup("dates.updated")
	if !ok {
		t.Fatal("Lookup(\"dates.updated\") returned nil")
	}
	if _, ok := node.(*LocalDateTimeNode); !ok {
		t.Errorf("expected *LocalDateTimeNode, got %T", node)
	}
}

func TestGet_DatesBirthday(t *testing.T) {
	doc := parseTestDoc(t)
	node, ok := doc.Lookup("dates.birthday")
	if !ok {
		t.Fatal("Lookup(\"dates.birthday\") returned nil")
	}
	if _, ok := node.(*LocalDateNode); !ok {
		t.Errorf("expected *LocalDateNode, got %T", node)
	}
}

func TestGet_DatesAlarm(t *testing.T) {
	doc := parseTestDoc(t)
	node, ok := doc.Lookup("dates.alarm")
	if !ok {
		t.Fatal("Lookup(\"dates.alarm\") returned nil")
	}
	if _, ok := node.(*LocalTimeNode); !ok {
		t.Errorf("expected *LocalTimeNode, got %T", node)
	}
}

func TestGetInt_NestedInlineX(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetInt("nested.inline.x")
	if err != nil {
		t.Fatalf("GetInt(\"nested.inline.x\"): %v", err)
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}
}

func TestGetInt_NestedArray0(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetInt("nested.array[0]")
	if err != nil {
		t.Fatalf("GetInt(\"nested.array[0]\"): %v", err)
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}
}

func TestGetInt_NestedArrayNeg1(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetInt("nested.array[-1]")
	if err != nil {
		t.Fatalf("GetInt(\"nested.array[-1]\"): %v", err)
	}
	if val != 3 {
		t.Errorf("expected 3, got %d", val)
	}
}

func TestGet_MissingPath(t *testing.T) {
	doc := parseTestDoc(t)
	node, ok := doc.Lookup("nonexistent")
	if ok {
		t.Errorf("expected nil for missing path, got %T", node)
	}
}

func TestGetString_WrongType(t *testing.T) {
	doc := parseTestDoc(t)
	val, err := doc.GetString("server.port")
	if err == nil {
		t.Errorf("GetString on an integer reported no error, and read %q", val)
	}
	if val != "" {
		t.Errorf("expected empty string for wrong type, got %q", val)
	}
}

func TestResolve_Success(t *testing.T) {
	doc := parseTestDoc(t)
	node, err := doc.Resolve("server.host")
	if err != nil {
		t.Fatalf("Resolve(\"server.host\") returned error: %v", err)
	}
	if node == nil {
		t.Fatal("Resolve(\"server.host\") returned nil node")
	}
	s, ok := node.(*StringNode)
	if !ok {
		t.Fatalf("expected *StringNode, got %T", node)
	}
	if s.val.get() != "localhost" {
		t.Errorf("expected \"localhost\", got %q", s.val.get())
	}
}

func TestResolve_MissingPath(t *testing.T) {
	doc := parseTestDoc(t)
	node, err := doc.Resolve("nonexistent")
	if err == nil {
		t.Fatal("Resolve(\"nonexistent\") should return error")
	}
	if node != nil {
		t.Errorf("expected nil node for missing path, got %T", node)
	}
}

func TestResolve_BadPathSyntax(t *testing.T) {
	doc := parseTestDoc(t)
	node, err := doc.Resolve("bad[path")
	if err == nil {
		t.Fatal("Resolve(\"bad[path\") should return error for bad syntax")
	}
	if node != nil {
		t.Errorf("expected nil node for bad path, got %T", node)
	}
}
