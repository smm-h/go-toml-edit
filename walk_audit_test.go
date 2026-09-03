package tomledit

import (
	"errors"
	"testing"
)

// Audit focus 5: Walk path correctness on a complex document with mixed
// tables, array-of-tables, dotted keys, inline tables, and inline arrays.
func TestAudit_Walk_ComplexPathCorrectness(t *testing.T) {
	input := `
title = "My App"
debug = true

[owner]
name = "Alice"
contact.email = "alice@example.com"
contact.phone = "555-1234"

[server]
host = "0.0.0.0"
ports = [80, 443, 8080]

[server.tls]
enabled = true
cert = "/etc/ssl/cert.pem"

[[products]]
name = "Widget"
tags = ["fast", "cheap"]
meta = {color = "red", weight = 1.5}

[[products]]
name = "Gadget"
tags = ["fancy"]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var paths []string
	err = doc.Walk(func(path string, node Node) error {
		paths = append(paths, path)
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	expected := []string{
		"title",
		"debug",
		"owner.name",
		"owner.contact.email",
		"owner.contact.phone",
		"server.host",
		"server.ports", "server.ports[0]", "server.ports[1]", "server.ports[2]",
		"server.tls.enabled",
		"server.tls.cert",
		"products[0].name",
		"products[0].tags", "products[0].tags[0]", "products[0].tags[1]",
		"products[0].meta", "products[0].meta.color", "products[0].meta.weight",
		"products[1].name",
		"products[1].tags", "products[1].tags[0]",
	}

	if len(paths) != len(expected) {
		t.Logf("PATHS (%d):", len(paths))
		for i, p := range paths {
			t.Logf("  [%d] %s", i, p)
		}
		t.Logf("EXPECTED (%d):", len(expected))
		for i, p := range expected {
			t.Logf("  [%d] %s", i, p)
		}
		t.Fatalf("path count mismatch: got %d, expected %d", len(paths), len(expected))
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Audit focus 6: Walk + array-of-tables with sub-tables.
// [[products]] with [products.details] -- paths should include array index.
func TestAudit_Walk_ArrayOfTablesWithSubTables(t *testing.T) {
	input := `[[products]]
name = "Widget"

[products.details]
color = "red"
weight = 1.5

[[products]]
name = "Gadget"

[products.details]
color = "blue"
weight = 2.0
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var paths []string
	err = doc.Walk(func(path string, node Node) error {
		paths = append(paths, path)
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	// The expected paths: sub-tables of array-of-tables entries should
	// include the array index of their parent entry.
	expected := []string{
		"products[0].name",
		"products[0].details.color",
		"products[0].details.weight",
		"products[1].name",
		"products[1].details.color",
		"products[1].details.weight",
	}

	t.Logf("ACTUAL PATHS (%d):", len(paths))
	for i, p := range paths {
		t.Logf("  [%d] %s", i, p)
	}

	if len(paths) != len(expected) {
		t.Errorf("path count: got %d, expected %d", len(paths), len(expected))
	}

	// Check each expected path appears (order-sensitive).
	for i, exp := range expected {
		if i >= len(paths) {
			t.Errorf("missing path[%d]: expected %q", i, exp)
			continue
		}
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Audit focus 6b: Array-of-tables with nested array-of-tables.
func TestAudit_Walk_NestedArrayOfTables(t *testing.T) {
	input := `[[servers]]
name = "alpha"

[[servers.interfaces]]
ip = "10.0.0.1"

[[servers.interfaces]]
ip = "10.0.0.2"

[[servers]]
name = "beta"

[[servers.interfaces]]
ip = "10.0.1.1"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var paths []string
	err = doc.Walk(func(path string, node Node) error {
		paths = append(paths, path)
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	expected := []string{
		"servers[0].name",
		"servers[0].interfaces[0].ip",
		"servers[0].interfaces[1].ip",
		"servers[1].name",
		"servers[1].interfaces[0].ip",
	}

	t.Logf("ACTUAL PATHS (%d):", len(paths))
	for i, p := range paths {
		t.Logf("  [%d] %s", i, p)
	}

	if len(paths) != len(expected) {
		t.Errorf("path count: got %d, expected %d", len(paths), len(expected))
	}
	for i, exp := range expected {
		if i >= len(paths) {
			t.Errorf("missing path[%d]: expected %q", i, exp)
			continue
		}
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Audit focus 7: Walk order -- root KVs before tables.
func TestAudit_Walk_RootKVsBeforeTables(t *testing.T) {
	input := `name = "test"
version = 42

[section]
key = "value"

[[items]]
id = 1

[[items]]
id = 2
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var paths []string
	err = doc.Walk(func(path string, node Node) error {
		paths = append(paths, path)
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	expected := []string{
		"name",
		"version",
		"section.key",
		"items[0].id",
		"items[1].id",
	}

	if len(paths) != len(expected) {
		t.Fatalf("path count: got %d, expected %d\npaths: %v", len(paths), len(expected), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}

	// Specifically verify ordering: root KVs come first.
	if paths[0] != "name" || paths[1] != "version" {
		t.Errorf("root KVs should come first, got: %v", paths[:2])
	}
}

// Audit focus 8: ErrSkipTable on array-of-tables entry.
// Return ErrSkipTable for products[0] -- should it skip that entry but continue to products[1]?
//
// NOTE: Walk does not yield array-of-tables entries as standalone nodes --
// it walks directly into their children. So ErrSkipTable can only be returned
// for individual KV values within an entry. This test documents the actual
// behavior: ErrSkipTable on a scalar KV is a no-op (that KV is still "not collected"
// since ErrSkipTable replaces nil as the return value, but subsequent KVs in the
// same table ARE still visited).
func TestAudit_Walk_ErrSkipTable_ArrayOfTablesEntry(t *testing.T) {
	input := `[[products]]
name = "Widget"
price = 9.99

[[products]]
name = "Gadget"
price = 19.99
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var paths []string
	err = doc.Walk(func(path string, node Node) error {
		// Try to "skip" the first product entry by returning ErrSkipTable
		// when we encounter its first KV.
		if path == "products[0].name" {
			return ErrSkipTable
		}
		paths = append(paths, path)
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	t.Logf("Paths after ErrSkipTable on products[0].name: %v", paths)

	// Since ErrSkipTable on a scalar is a no-op (nothing to skip),
	// products[0].price should still be visited.
	// products[1].name and products[1].price should also be visited.
	foundPrice0 := false
	foundName1 := false
	foundPrice1 := false
	for _, p := range paths {
		switch p {
		case "products[0].price":
			foundPrice0 = true
		case "products[1].name":
			foundName1 = true
		case "products[1].price":
			foundPrice1 = true
		}
	}

	if !foundPrice0 {
		t.Logf("products[0].price was skipped -- ErrSkipTable on scalar DID skip remaining table children")
	} else {
		t.Logf("products[0].price was visited -- ErrSkipTable on scalar is a no-op as expected")
	}
	if !foundName1 {
		t.Errorf("products[1].name should be visited regardless")
	}
	if !foundPrice1 {
		t.Errorf("products[1].price should be visited regardless")
	}
}

// Audit focus 8b: ErrSkipTable on an inline table inside an array-of-tables.
func TestAudit_Walk_ErrSkipTable_InlineTableInArrayOfTables(t *testing.T) {
	input := `[[products]]
name = "Widget"
meta = {color = "red", weight = 1.5}

[[products]]
name = "Gadget"
meta = {color = "blue", weight = 2.0}
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var paths []string
	err = doc.Walk(func(path string, node Node) error {
		if path == "products[0].meta" {
			paths = append(paths, path)
			return ErrSkipTable
		}
		paths = append(paths, path)
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	t.Logf("Paths: %v", paths)

	// ErrSkipTable on products[0].meta should skip its children (color, weight)
	// but products[1] should still be fully visited.
	for _, p := range paths {
		if p == "products[0].meta.color" || p == "products[0].meta.weight" {
			t.Errorf("ErrSkipTable should have skipped %q", p)
		}
	}

	// products[1] should be fully visited.
	expected1 := map[string]bool{
		"products[1].name":        false,
		"products[1].meta":        false,
		"products[1].meta.color":  false,
		"products[1].meta.weight": false,
	}
	for _, p := range paths {
		if _, ok := expected1[p]; ok {
			expected1[p] = true
		}
	}
	for p, found := range expected1 {
		if !found {
			t.Errorf("expected to find %q in paths", p)
		}
	}
}

// Audit focus 7b: Walk with keys requiring quoting -- paths should use quotes.
func TestAudit_Walk_SpecialKeysQuoted(t *testing.T) {
	input := `[server]
"host.name" = "example.com"
"port number" = 8080
normal-key = true
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var paths []string
	err = doc.Walk(func(path string, node Node) error {
		paths = append(paths, path)
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	expected := []string{
		`server."host.name"`,
		`server."port number"`,
		`server.normal-key`,
	}

	if len(paths) != len(expected) {
		t.Fatalf("path count: got %d, expected %d\npaths: %v", len(paths), len(expected), paths)
	}
	for i, exp := range expected {
		if paths[i] != exp {
			t.Errorf("path[%d]: expected %q, got %q", i, exp, paths[i])
		}
	}
}

// Audit: Walk with error stops immediately and returns the error.
func TestAudit_Walk_ErrorStops(t *testing.T) {
	input := `a = 1
b = 2
c = 3
d = 4
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	sentinel := errors.New("stop at b")
	var visited []string
	err = doc.Walk(func(path string, node Node) error {
		visited = append(visited, path)
		if path == "b" {
			return sentinel
		}
		return nil
	}, WalkAll)

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
	if len(visited) != 2 || visited[0] != "a" || visited[1] != "b" {
		t.Errorf("expected [a b], got %v", visited)
	}
}

// Audit: Walk with empty table.
func TestAudit_Walk_EmptyTable(t *testing.T) {
	input := `[empty]

[notempty]
key = "val"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var paths []string
	err = doc.Walk(func(path string, node Node) error {
		paths = append(paths, path)
		return nil
	}, WalkAll)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	// Empty table produces no paths; notempty.key should appear.
	if len(paths) != 1 || paths[0] != "notempty.key" {
		t.Errorf("expected [notempty.key], got %v", paths)
	}
}
