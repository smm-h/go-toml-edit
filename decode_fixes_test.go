package tomledit

import (
	"errors"
	"strings"
	"testing"
)

// Four decode defects, each phrased on the public decode surface. They are the
// regression tests for what the descriptor engine changed: a table under a map
// target losing its key, an array-of-tables under a plain table never being
// decoded, a reflect panic on a map-typed table path, and a fixed-size Go
// array quietly accepting the wrong number of elements.

// Fails if a table written under a map-typed field decodes into the map's own
// top level instead of into the element the key names: [items.a] binds the map
// element "a", never the map's "name" element.
func TestDecode_MapOfStructKeepsTheTableKey(t *testing.T) {
	input := `
[items.a]
name = "first"

[items.b]
name = "second"
`
	type Item struct {
		Name string `toml:"name"`
	}
	type Config struct {
		Items map[string]Item `toml:"items"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(cfg.Items) != 2 {
		t.Fatalf("Items = %v, want two elements", cfg.Items)
	}
	if cfg.Items["a"].Name != "first" {
		t.Errorf(`Items["a"].Name = %q, want "first"`, cfg.Items["a"].Name)
	}
	if cfg.Items["b"].Name != "second" {
		t.Errorf(`Items["b"].Name = %q, want "second"`, cfg.Items["b"].Name)
	}
}

// Fails if the same truncation survives for a map of any: the sub-table has to
// arrive as a nested map under its own key, not merged into the outer one.
func TestDecode_MapOfAnyKeepsTheTableKey(t *testing.T) {
	input := `
[items.a]
name = "first"
`
	type Config struct {
		Items map[string]any `toml:"items"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	inner, ok := cfg.Items["a"].(map[string]any)
	if !ok {
		t.Fatalf(`Items["a"] = %#v, want a map[string]any`, cfg.Items["a"])
	}
	if inner["name"] != "first" {
		t.Errorf(`Items["a"]["name"] = %v, want "first"`, inner["name"])
	}
}

// Fails if an array-of-tables nested under a plain table is skipped: [[a.b]]
// under [a] binds a.b, and a decode that reaches neither the header nor its
// contents silently loses data.
func TestDecode_ArrayTableUnderPlainTable(t *testing.T) {
	input := `
[a]
x = 1

[[a.b]]
y = 2

[[a.b]]
y = 3
`
	type B struct {
		Y int `toml:"y"`
	}
	type A struct {
		X int `toml:"x"`
		B []B `toml:"b"`
	}
	type Config struct {
		A A `toml:"a"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.A.X != 1 {
		t.Errorf("A.X = %d, want 1", cfg.A.X)
	}
	if len(cfg.A.B) != 2 {
		t.Fatalf("A.B = %v, want two entries", cfg.A.B)
	}
	if cfg.A.B[0].Y != 2 || cfg.A.B[1].Y != 3 {
		t.Errorf("A.B = %v, want [{2} {3}]", cfg.A.B)
	}
}

// Fails if a table reaching a map element that cannot hold one is handed to
// reflect anyway: the dotted key builds a table at data.a, the element type is
// a string, and the answer is a positioned type-mismatch diagnostic -- never a
// panic.
func TestDecode_TableIntoTypedMapElementIsRefused(t *testing.T) {
	input := `
[data]
a.b = "x"
`
	type Config struct {
		Data map[string]string `toml:"data"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatalf("Unmarshal accepted a table into a string map element: %v", cfg)
	}
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("err = %v, want a type-mismatch diagnostic", err)
	}
	var diag *Error
	if !errors.As(err, &diag) {
		t.Fatalf("err = %v (%T), want a *Error", err, err)
	}
	if diag.Path != "data.a" {
		t.Errorf("diagnostic names path %q, want %q", diag.Path, "data.a")
	}
	if diag.Pos.Line != 3 {
		t.Errorf("diagnostic at line %d, want the line the key is written on (3)", diag.Pos.Line)
	}
}

// Fails if a fixed-size Go array accepts an array of a different length. The
// arity is declared by the target: under-fill used to zero-pad, inventing
// values the document never carried.
func TestDecode_FixedArrayLengthIsExact(t *testing.T) {
	type Config struct {
		Vals [5]int `toml:"vals"`
	}
	var cfg Config
	err := Unmarshal([]byte("vals = [1, 2]\n"), &cfg)
	if err == nil {
		t.Fatalf("Unmarshal accepted two elements into [5]int: %v", cfg.Vals)
	}
	if !errors.Is(err, ErrInexact) {
		t.Fatalf("err = %v, want an inexact diagnostic", err)
	}
	var diag *Error
	if !errors.As(err, &diag) {
		t.Fatalf("err = %v (%T), want a *Error", err, err)
	}
	if diag.Path != "vals" {
		t.Errorf("diagnostic names path %q, want %q", diag.Path, "vals")
	}
	if !strings.Contains(diag.Message, "5") || !strings.Contains(diag.Message, "2") {
		t.Errorf("message %q names neither the expected length nor the one given", diag.Message)
	}
}
