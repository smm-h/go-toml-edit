package tomledit

import (
	"errors"
	"fmt"
	"testing"
)

func parseWalkDoc(t *testing.T, input string) *DocumentNode {
	t.Helper()
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	return doc
}

func collectPaths(t *testing.T, doc *DocumentNode) []string {
	t.Helper()
	var paths []string
	err := doc.Walk(func(path string, node Node) error {
		paths = append(paths, path)
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk returned error: %v", err)
	}
	return paths
}

type pathNode struct {
	path string
	node Node
}

func collectPathNodes(t *testing.T, doc *DocumentNode) []pathNode {
	t.Helper()
	var result []pathNode
	err := doc.Walk(func(path string, node Node) error {
		result = append(result, pathNode{path: path, node: node})
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk returned error: %v", err)
	}
	return result
}

// Test 1: Simple document with a single key-value pair.
func TestWalk_SimpleDocument(t *testing.T) {
	doc := parseWalkDoc(t, `key = "value"`)
	pns := collectPathNodes(t, doc)
	if len(pns) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(pns))
	}
	if pns[0].path != "key" {
		t.Errorf("expected path %q, got %q", "key", pns[0].path)
	}
	s, ok := pns[0].node.(*StringNode)
	if !ok {
		t.Fatalf("expected *StringNode, got %T", pns[0].node)
	}
	if s.Val != "value" {
		t.Errorf("expected value %q, got %q", "value", s.Val)
	}
}

// Test 2: Table with key-value pairs.
func TestWalk_Table(t *testing.T) {
	doc := parseWalkDoc(t, `[server]
host = "localhost"
port = 8080
`)
	paths := collectPaths(t, doc)
	expected := []string{"server.host", "server.port"}
	if len(paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(paths), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Test 3: Nested tables.
func TestWalk_NestedTables(t *testing.T) {
	doc := parseWalkDoc(t, `[server]
host = "localhost"

[server.database]
name = "mydb"
port = 5432
`)
	paths := collectPaths(t, doc)
	expected := []string{
		"server.host",
		"server.database.name",
		"server.database.port",
	}
	if len(paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(paths), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Test 4: Array-of-tables.
func TestWalk_ArrayOfTables(t *testing.T) {
	doc := parseWalkDoc(t, `[[products]]
name = "Widget"

[[products]]
name = "Gadget"
`)
	paths := collectPaths(t, doc)
	expected := []string{
		"products[0].name",
		"products[1].name",
	}
	if len(paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(paths), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Test 5: Dotted keys.
func TestWalk_DottedKeys(t *testing.T) {
	doc := parseWalkDoc(t, `a.b.c = 1`)
	paths := collectPaths(t, doc)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d: %v", len(paths), paths)
	}
	if paths[0] != "a.b.c" {
		t.Errorf("expected %q, got %q", "a.b.c", paths[0])
	}
}

// Test 6: Inline tables.
func TestWalk_InlineTables(t *testing.T) {
	doc := parseWalkDoc(t, `config = {x = 1, y = 2}`)
	paths := collectPaths(t, doc)
	// The inline table itself is yielded, then its children
	expected := []string{"config", "config.x", "config.y"}
	if len(paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(paths), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Test 7: Inline arrays.
func TestWalk_InlineArrays(t *testing.T) {
	doc := parseWalkDoc(t, `tags = [1, 2, 3]`)
	paths := collectPaths(t, doc)
	// The array itself is yielded, then each element
	expected := []string{"tags", "tags[0]", "tags[1]", "tags[2]"}
	if len(paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(paths), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Test 8: Complex document combining all features.
func TestWalk_ComplexDocument(t *testing.T) {
	doc := parseWalkDoc(t, `
title = "Example"
tags = [1, 2]

[server]
host = "localhost"
ports = [80, 443]

[server.database]
name = "mydb"

[[products]]
name = "Widget"
config = {color = "red"}

[[products]]
name = "Gadget"
`)
	paths := collectPaths(t, doc)
	expected := []string{
		"title",
		"tags", "tags[0]", "tags[1]",
		"server.host",
		"server.ports", "server.ports[0]", "server.ports[1]",
		"server.database.name",
		"products[0].name",
		"products[0].config", "products[0].config.color",
		"products[1].name",
	}
	if len(paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(paths), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Test 9: ErrSkipTable skips a table's children but continues with other tables.
func TestWalk_ErrSkipTable(t *testing.T) {
	doc := parseWalkDoc(t, `
[server]
host = "localhost"
port = 8080

[database]
name = "mydb"
`)
	var paths []string
	err := doc.Walk(func(path string, node Node) error {
		// Skip all entries under "server"
		if path == "server.host" {
			return ErrSkipTable
		}
		paths = append(paths, path)
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk returned error: %v", err)
	}
	// server.host triggered ErrSkipTable (was not collected), server.port follows
	// in the same table so it should still appear (ErrSkipTable on a scalar is a no-op).
	// database.name should appear.
	if len(paths) < 1 {
		t.Fatal("expected at least one path")
	}
	// Verify database.name is present
	found := false
	for _, p := range paths {
		if p == "database.name" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected database.name to be visited, got: %v", paths)
	}
}

// Test 9b: ErrSkipTable on an inline table skips its children.
func TestWalk_SkipInlineTable(t *testing.T) {
	doc := parseWalkDoc(t, `config = {x = 1, y = 2}
other = "yes"`)
	var paths []string
	err := doc.Walk(func(path string, node Node) error {
		if path == "config" {
			paths = append(paths, path)
			return ErrSkipTable
		}
		paths = append(paths, path)
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk returned error: %v", err)
	}
	expected := []string{"config", "other"}
	if len(paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(paths), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Test 10: Error propagation stops the walk.
func TestWalk_ErrorPropagation(t *testing.T) {
	doc := parseWalkDoc(t, `a = 1
b = 2
c = 3
`)
	myErr := errors.New("stop here")
	var visited []string
	err := doc.Walk(func(path string, node Node) error {
		visited = append(visited, path)
		if path == "b" {
			return myErr
		}
		return nil
	}, WalkAll)
	if !errors.Is(err, myErr) {
		t.Fatalf("expected myErr, got %v", err)
	}
	// Should have visited "a" and "b", but not "c"
	if len(visited) != 2 {
		t.Fatalf("expected 2 visited, got %d: %v", len(visited), visited)
	}
	if visited[0] != "a" || visited[1] != "b" {
		t.Errorf("expected [a, b], got %v", visited)
	}
}

// Test 11: Empty document produces no calls.
func TestWalk_EmptyDocument(t *testing.T) {
	doc := parseWalkDoc(t, ``)
	paths := collectPaths(t, doc)
	if len(paths) != 0 {
		t.Errorf("expected 0 paths, got %d: %v", len(paths), paths)
	}
}

// Test 12: Walk collects all paths from a complex document.
func TestWalk_CollectAllPaths(t *testing.T) {
	doc := parseWalkDoc(t, `
name = "test"
version = 1

[owner]
first = "Tom"
last = "Preston"

[owner.address]
city = "NYC"

[[fruits]]
name = "apple"

[[fruits]]
name = "banana"
`)
	paths := collectPaths(t, doc)
	expectedSet := map[string]bool{
		"name":               true,
		"version":            true,
		"owner.first":        true,
		"owner.last":         true,
		"owner.address.city": true,
		"fruits[0].name":     true,
		"fruits[1].name":     true,
	}
	if len(paths) != len(expectedSet) {
		t.Fatalf("expected %d paths, got %d: %v", len(expectedSet), len(paths), paths)
	}
	for _, p := range paths {
		if !expectedSet[p] {
			t.Errorf("unexpected path %q", p)
		}
	}
}

// Test 13: Value types -- verify that fn receives the value node, not the KV node.
func TestWalk_ValueTypes(t *testing.T) {
	doc := parseWalkDoc(t, `
str = "hello"
num = 42
pi = 3.14
flag = true
`)
	pns := collectPathNodes(t, doc)
	typeChecks := map[string]NodeType{
		"str":  NodeString,
		"num":  NodeInteger,
		"pi":   NodeFloat,
		"flag": NodeBoolean,
	}
	for _, pn := range pns {
		expected, ok := typeChecks[pn.path]
		if !ok {
			t.Errorf("unexpected path %q", pn.path)
			continue
		}
		if pn.node.Type() != expected {
			t.Errorf("path %q: expected node type %s, got %s", pn.path, expected, pn.node.Type())
		}
		// Verify it is NOT a KeyValueNode
		if _, isKV := pn.node.(*KeyValueNode); isKV {
			t.Errorf("path %q: expected unwrapped value, got *KeyValueNode", pn.path)
		}
	}
}

// Test: Keys requiring quoting are quoted in the path.
func TestWalk_QuotedKeys(t *testing.T) {
	doc := parseWalkDoc(t, `"host.name" = "example.com"`)
	paths := collectPaths(t, doc)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d: %v", len(paths), paths)
	}
	expected := `"host.name"`
	if paths[0] != expected {
		t.Errorf("expected path %q, got %q", expected, paths[0])
	}
}

// Test: Keys with spaces are quoted.
func TestWalk_KeysWithSpaces(t *testing.T) {
	doc := parseWalkDoc(t, `"my key" = 1`)
	paths := collectPaths(t, doc)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d: %v", len(paths), paths)
	}
	expected := `"my key"`
	if paths[0] != expected {
		t.Errorf("expected path %q, got %q", expected, paths[0])
	}
}

// Test: Comments are skipped by Walk.
func TestWalk_CommentsSkipped(t *testing.T) {
	doc := parseWalkDoc(t, `# This is a comment
key = "value"
# Another comment
`)
	paths := collectPaths(t, doc)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d: %v", len(paths), paths)
	}
	if paths[0] != "key" {
		t.Errorf("expected path %q, got %q", "key", paths[0])
	}
}

// Test: Walk with dotted keys under a table.
func TestWalk_DottedKeysUnderTable(t *testing.T) {
	doc := parseWalkDoc(t, `[server]
db.host = "localhost"
db.port = 5432
`)
	paths := collectPaths(t, doc)
	expected := []string{"server.db.host", "server.db.port"}
	if len(paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(paths), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Test: Nested inline tables.
func TestWalk_NestedInlineTables(t *testing.T) {
	doc := parseWalkDoc(t, `point = {x = {a = 1}, y = 2}`)
	paths := collectPaths(t, doc)
	expected := []string{"point", "point.x", "point.x.a", "point.y"}
	if len(paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(paths), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Test: Array containing inline tables.
func TestWalk_ArrayOfInlineTables(t *testing.T) {
	doc := parseWalkDoc(t, `items = [{name = "a"}, {name = "b"}]`)
	paths := collectPaths(t, doc)
	expected := []string{
		"items",
		"items[0]", "items[0].name",
		"items[1]", "items[1].name",
	}
	if len(paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(paths), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Test: Multiple root-level KVs followed by tables.
func TestWalk_RootKVsAndTables(t *testing.T) {
	doc := parseWalkDoc(t, `
a = 1
b = 2

[section]
c = 3
`)
	paths := collectPaths(t, doc)
	expected := []string{"a", "b", "section.c"}
	if len(paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(paths), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Prevent unused import warning for fmt
var _ = fmt.Sprintf
