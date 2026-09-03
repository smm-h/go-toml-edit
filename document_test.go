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
	node := doc.Get("server.host")
	if node == nil {
		t.Fatal("Get(\"server.host\") returned nil")
	}
	s, ok := node.(*StringNode)
	if !ok {
		t.Fatalf("expected *StringNode, got %T", node)
	}
	if s.Val != "localhost" {
		t.Errorf("expected \"localhost\", got %q", s.Val)
	}
}

func TestGetString_ServerHost(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetString("server.host")
	if !ok {
		t.Fatal("GetString(\"server.host\") returned false")
	}
	if val != "localhost" {
		t.Errorf("expected \"localhost\", got %q", val)
	}
}

func TestGetInt_ServerPort(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetInt("server.port")
	if !ok {
		t.Fatal("GetInt(\"server.port\") returned false")
	}
	if val != 8080 {
		t.Errorf("expected 8080, got %d", val)
	}
}

func TestGetBool_ServerDebug(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetBool("server.debug")
	if !ok {
		t.Fatal("GetBool(\"server.debug\") returned false")
	}
	if val != true {
		t.Errorf("expected true, got %v", val)
	}
}

func TestGetFloat_ServerRate(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetFloat("server.rate")
	if !ok {
		t.Fatal("GetFloat(\"server.rate\") returned false")
	}
	if val != 3.14 {
		t.Errorf("expected 3.14, got %f", val)
	}
}

func TestGetString_ServerDatabaseName(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetString("server.database.name")
	if !ok {
		t.Fatal("GetString(\"server.database.name\") returned false")
	}
	if val != "mydb" {
		t.Errorf("expected \"mydb\", got %q", val)
	}
}

func TestGetString_Products0Name(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetString("products[0].name")
	if !ok {
		t.Fatal("GetString(\"products[0].name\") returned false")
	}
	if val != "Widget" {
		t.Errorf("expected \"Widget\", got %q", val)
	}
}

func TestGetString_Products1Name(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetString("products[1].name")
	if !ok {
		t.Fatal("GetString(\"products[1].name\") returned false")
	}
	if val != "Gadget" {
		t.Errorf("expected \"Gadget\", got %q", val)
	}
}

func TestGetString_ProductsNeg1Name(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetString("products[-1].name")
	if !ok {
		t.Fatal("GetString(\"products[-1].name\") returned false")
	}
	if val != "Gadget" {
		t.Errorf("expected \"Gadget\", got %q", val)
	}
}

func TestGetFloat_Products0Price(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetFloat("products[0].price")
	if !ok {
		t.Fatal("GetFloat(\"products[0].price\") returned false")
	}
	if val != 9.99 {
		t.Errorf("expected 9.99, got %f", val)
	}
}

func TestGetTime_DatesCreated(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetTime("dates.created")
	if !ok {
		t.Fatal("GetTime(\"dates.created\") returned false")
	}
	if val.Year() != 1979 || val.Month() != 5 || val.Day() != 27 {
		t.Errorf("unexpected time: %v", val)
	}
}

func TestGet_DatesUpdated(t *testing.T) {
	doc := parseTestDoc(t)
	node := doc.Get("dates.updated")
	if node == nil {
		t.Fatal("Get(\"dates.updated\") returned nil")
	}
	if _, ok := node.(*LocalDateTimeNode); !ok {
		t.Errorf("expected *LocalDateTimeNode, got %T", node)
	}
}

func TestGet_DatesBirthday(t *testing.T) {
	doc := parseTestDoc(t)
	node := doc.Get("dates.birthday")
	if node == nil {
		t.Fatal("Get(\"dates.birthday\") returned nil")
	}
	if _, ok := node.(*LocalDateNode); !ok {
		t.Errorf("expected *LocalDateNode, got %T", node)
	}
}

func TestGet_DatesAlarm(t *testing.T) {
	doc := parseTestDoc(t)
	node := doc.Get("dates.alarm")
	if node == nil {
		t.Fatal("Get(\"dates.alarm\") returned nil")
	}
	if _, ok := node.(*LocalTimeNode); !ok {
		t.Errorf("expected *LocalTimeNode, got %T", node)
	}
}

func TestGetInt_NestedInlineX(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetInt("nested.inline.x")
	if !ok {
		t.Fatal("GetInt(\"nested.inline.x\") returned false")
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}
}

func TestGetInt_NestedArray0(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetInt("nested.array[0]")
	if !ok {
		t.Fatal("GetInt(\"nested.array[0]\") returned false")
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}
}

func TestGetInt_NestedArrayNeg1(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetInt("nested.array[-1]")
	if !ok {
		t.Fatal("GetInt(\"nested.array[-1]\") returned false")
	}
	if val != 3 {
		t.Errorf("expected 3, got %d", val)
	}
}

func TestGet_MissingPath(t *testing.T) {
	doc := parseTestDoc(t)
	node := doc.Get("nonexistent")
	if node != nil {
		t.Errorf("expected nil for missing path, got %T", node)
	}
}

func TestGetString_WrongType(t *testing.T) {
	doc := parseTestDoc(t)
	val, ok := doc.GetString("server.port")
	if ok {
		t.Errorf("expected false for wrong type, got (%q, true)", val)
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
	if s.Val != "localhost" {
		t.Errorf("expected \"localhost\", got %q", s.Val)
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
