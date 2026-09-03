package tomledit

import (
	"fmt"
	"testing"
)

func parseDiffDoc(t *testing.T, input string) *Document {
	t.Helper()
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	return doc
}

func requireChanges(t *testing.T, got []Change, want []Change) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d changes, got %d:\n%s", len(want), len(got), formatChanges(got))
	}
	for i := range want {
		if got[i].Kind != want[i].Kind {
			t.Errorf("change[%d] kind: got %v, want %v", i, got[i].Kind, want[i].Kind)
		}
		if got[i].Path != want[i].Path {
			t.Errorf("change[%d] path: got %q, want %q", i, got[i].Path, want[i].Path)
		}
		if !valuesEqual(got[i].OldValue, want[i].OldValue) {
			t.Errorf("change[%d] old value: got %v (%T), want %v (%T)",
				i, got[i].OldValue, got[i].OldValue, want[i].OldValue, want[i].OldValue)
		}
		if !valuesEqual(got[i].NewValue, want[i].NewValue) {
			t.Errorf("change[%d] new value: got %v (%T), want %v (%T)",
				i, got[i].NewValue, got[i].NewValue, want[i].NewValue, want[i].NewValue)
		}
	}
}

func formatChanges(changes []Change) string {
	var s string
	for i, c := range changes {
		s += fmt.Sprintf("  [%d] %v %q old=%v new=%v\n", i, c.Kind, c.Path, c.OldValue, c.NewValue)
	}
	return s
}

// Test 1: Identical documents produce empty result.
func TestDiff_Identical(t *testing.T) {
	input := `
name = "test"
version = 42

[server]
host = "localhost"
port = 8080
`
	a := parseDiffDoc(t, input)
	b := parseDiffDoc(t, input)
	changes := Diff(a, b)
	requireChanges(t, changes, nil)
}

// Test 2: Added keys -- b has keys a doesn't.
func TestDiff_Added(t *testing.T) {
	a := parseDiffDoc(t, `name = "test"`)
	b := parseDiffDoc(t, `
name = "test"
version = 1
debug = true
`)
	changes := Diff(a, b)
	requireChanges(t, changes, []Change{
		{Kind: Added, Path: "debug", NewValue: true},
		{Kind: Added, Path: "version", NewValue: int64(1)},
	})
}

// Test 3: Removed keys -- a has keys b doesn't.
func TestDiff_Removed(t *testing.T) {
	a := parseDiffDoc(t, `
name = "test"
version = 1
debug = true
`)
	b := parseDiffDoc(t, `name = "test"`)
	changes := Diff(a, b)
	requireChanges(t, changes, []Change{
		{Kind: Removed, Path: "debug", OldValue: true},
		{Kind: Removed, Path: "version", OldValue: int64(1)},
	})
}

// Test 4: Modified values -- same keys, different values.
func TestDiff_Modified(t *testing.T) {
	a := parseDiffDoc(t, `
name = "alpha"
port = 8080
enabled = true
`)
	b := parseDiffDoc(t, `
name = "beta"
port = 9090
enabled = false
`)
	changes := Diff(a, b)
	requireChanges(t, changes, []Change{
		{Kind: Modified, Path: "enabled", OldValue: true, NewValue: false},
		{Kind: Modified, Path: "name", OldValue: "alpha", NewValue: "beta"},
		{Kind: Modified, Path: "port", OldValue: int64(8080), NewValue: int64(9090)},
	})
}

// Test 5: Mixed changes -- adds, removes, and modifications together.
func TestDiff_Mixed(t *testing.T) {
	a := parseDiffDoc(t, `
name = "test"
version = 1
old_key = "gone"
`)
	b := parseDiffDoc(t, `
name = "prod"
version = 1
new_key = "here"
`)
	changes := Diff(a, b)
	requireChanges(t, changes, []Change{
		{Kind: Modified, Path: "name", OldValue: "test", NewValue: "prod"},
		{Kind: Added, Path: "new_key", NewValue: "here"},
		{Kind: Removed, Path: "old_key", OldValue: "gone"},
	})
}

// Test 6: Nested tables -- changes within [server.database].
func TestDiff_NestedTables(t *testing.T) {
	a := parseDiffDoc(t, `
[server]
host = "localhost"

[server.database]
name = "mydb"
port = 5432
`)
	b := parseDiffDoc(t, `
[server]
host = "remotehost"

[server.database]
name = "mydb"
port = 3306
pool = 10
`)
	changes := Diff(a, b)
	requireChanges(t, changes, []Change{
		{Kind: Added, Path: "server.database.pool", NewValue: int64(10)},
		{Kind: Modified, Path: "server.database.port", OldValue: int64(5432), NewValue: int64(3306)},
		{Kind: Modified, Path: "server.host", OldValue: "localhost", NewValue: "remotehost"},
	})
}

// Test 7: Array-of-tables -- different number of entries.
func TestDiff_ArrayOfTables(t *testing.T) {
	a := parseDiffDoc(t, `
[[products]]
name = "Hammer"

[[products]]
name = "Nail"
`)
	b := parseDiffDoc(t, `
[[products]]
name = "Hammer"

[[products]]
name = "Screw"

[[products]]
name = "Bolt"
`)
	changes := Diff(a, b)
	requireChanges(t, changes, []Change{
		{Kind: Modified, Path: "products[1].name", OldValue: "Nail", NewValue: "Screw"},
		{Kind: Added, Path: "products[2].name", NewValue: "Bolt"},
	})
}

// Test 8: Value type changes -- string to int at same path.
func TestDiff_TypeChange(t *testing.T) {
	a := parseDiffDoc(t, `port = "8080"`)
	b := parseDiffDoc(t, `port = 8080`)
	changes := Diff(a, b)
	requireChanges(t, changes, []Change{
		{Kind: Modified, Path: "port", OldValue: "8080", NewValue: int64(8080)},
	})
}

// Test 9: Empty documents -- both empty, no changes.
func TestDiff_BothEmpty(t *testing.T) {
	a := parseDiffDoc(t, ``)
	b := parseDiffDoc(t, ``)
	changes := Diff(a, b)
	requireChanges(t, changes, nil)
}

// Test 10: One empty, one populated.
func TestDiff_OneEmpty(t *testing.T) {
	empty := parseDiffDoc(t, ``)
	populated := parseDiffDoc(t, `
name = "test"
count = 42
`)
	// Empty -> populated: all added
	added := Diff(empty, populated)
	requireChanges(t, added, []Change{
		{Kind: Added, Path: "count", NewValue: int64(42)},
		{Kind: Added, Path: "name", NewValue: "test"},
	})

	// Populated -> empty: all removed
	removed := Diff(populated, empty)
	requireChanges(t, removed, []Change{
		{Kind: Removed, Path: "count", OldValue: int64(42)},
		{Kind: Removed, Path: "name", OldValue: "test"},
	})
}

// Test 11: Changes sorted by path.
func TestDiff_SortedByPath(t *testing.T) {
	a := parseDiffDoc(t, `
z = 1
a = 2
m = 3
`)
	b := parseDiffDoc(t, `
z = 10
a = 20
m = 30
`)
	changes := Diff(a, b)
	if len(changes) != 3 {
		t.Fatalf("expected 3 changes, got %d", len(changes))
	}
	// Must be sorted: a, m, z
	expectedPaths := []string{"a", "m", "z"}
	for i, exp := range expectedPaths {
		if changes[i].Path != exp {
			t.Errorf("change[%d] path: got %q, want %q", i, changes[i].Path, exp)
		}
	}
}

// Test: ChangeKind.String() returns correct names.
func TestChangeKind_String(t *testing.T) {
	tests := []struct {
		kind ChangeKind
		want string
	}{
		{Added, "added"},
		{Removed, "removed"},
		{Modified, "modified"},
		{ChangeKind(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("ChangeKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}
