package tomledit

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ==========================================================================
// Audit Focus Area 1: Struct embedding edge cases
// ==========================================================================

// Two embedded structs with the same field name at the same depth.
// Go convention: both are shadowed (neither accessible). Current code: first wins.
func TestAudit_EmbeddedConflictingFields(t *testing.T) {
	type A struct {
		Name string `toml:"name"`
	}
	type B struct {
		Name string `toml:"name"`
	}
	type Config struct {
		A
		B
	}
	input := `name = "hello"`
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	// Documenting current behavior: first embedded struct wins (A.Name gets set).
	// This differs from encoding/json which makes conflicting fields invisible.
	// Not necessarily a bug, but worth noting.
	if err != nil {
		t.Logf("INFO: conflicting embedded fields returned error: %v", err)
		return
	}
	// If no error, one of them should be set (current impl: first wins)
	if cfg.A.Name == "" && cfg.B.Name == "" {
		t.Error("neither embedded field was set -- both shadowed (Go convention, acceptable)")
	}
	t.Logf("INFO: A.Name=%q B.Name=%q (first-wins behavior)", cfg.A.Name, cfg.B.Name)
}

// Unexported embedded struct -- its fields are not promoted, so the document
// universe does not carry them.
//
// Fails if a key naming a field of an unexported embedded struct is accepted
// again: nothing can write it, so it is unknown, not ignored.
func TestAudit_UnexportedEmbeddedStruct(t *testing.T) {
	type inner struct {
		Val string `toml:"val"`
	}
	type Config struct {
		inner        // unexported embedding -- this field itself is unexported
		Name  string `toml:"name"`
	}
	input := `
name = "test"
val = "should_not_appear"
`
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatalf("Unmarshal accepted a key naming an unexported embedding: %+v", cfg)
	}
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("err = %v, want an unknown-key diagnostic", err)
	}
	if cfg.Name != "test" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test")
	}
	if cfg.inner.Val != "" {
		t.Errorf("unexported embedded field should not be set, got Val=%q", cfg.inner.Val)
	}
}

// Embedded pointer to struct (nil by default, should be allocated).
func TestAudit_EmbeddedPointerStruct(t *testing.T) {
	type Base struct {
		Host string `toml:"host"`
	}
	type Config struct {
		*Base
		Name string `toml:"name"`
	}
	input := `
name = "test"
host = "localhost"
`
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Name != "test" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test")
	}
	if cfg.Base == nil {
		t.Fatal("embedded *Base not allocated")
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host = %q, want %q", cfg.Host, "localhost")
	}
}

// ==========================================================================
// Audit Focus Area 2: Array-of-tables with sub-tables
// ==========================================================================

// Sub-table should scope into the LAST array entry.
func TestAudit_ArrayTableSubTableScopesIntoLastEntry(t *testing.T) {
	input := `
[[products]]
name = "First"

[[products]]
name = "Second"

[products.details]
weight = 2.5
`
	type Details struct {
		Weight float64 `toml:"weight"`
	}
	type Product struct {
		Name    string  `toml:"name"`
		Details Details `toml:"details"`
	}
	type Config struct {
		Products []Product `toml:"products"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(cfg.Products) != 2 {
		t.Fatalf("len(Products) = %d, want 2", len(cfg.Products))
	}
	// The sub-table [products.details] should apply to the LAST entry (Second)
	if cfg.Products[1].Details.Weight != 2.5 {
		t.Errorf("Products[1].Details.Weight = %f, want 2.5", cfg.Products[1].Details.Weight)
	}
	// First entry should have zero details
	if cfg.Products[0].Details.Weight != 0 {
		t.Errorf("Products[0].Details.Weight = %f, want 0", cfg.Products[0].Details.Weight)
	}
}

// Nested array-of-tables: [[products]] with [[products.tags]]
func TestAudit_NestedArrayOfTables(t *testing.T) {
	input := `
[[products]]
name = "Widget"

[[products.tags]]
label = "sale"

[[products.tags]]
label = "new"

[[products]]
name = "Gadget"

[[products.tags]]
label = "premium"
`
	type Tag struct {
		Label string `toml:"label"`
	}
	type Product struct {
		Name string `toml:"name"`
		Tags []Tag  `toml:"tags"`
	}
	type Config struct {
		Products []Product `toml:"products"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(cfg.Products) != 2 {
		t.Fatalf("len(Products) = %d, want 2", len(cfg.Products))
	}
	if cfg.Products[0].Name != "Widget" {
		t.Errorf("Products[0].Name = %q, want Widget", cfg.Products[0].Name)
	}
	if len(cfg.Products[0].Tags) != 2 {
		t.Fatalf("len(Products[0].Tags) = %d, want 2", len(cfg.Products[0].Tags))
	}
	if cfg.Products[0].Tags[0].Label != "sale" {
		t.Errorf("Products[0].Tags[0].Label = %q, want sale", cfg.Products[0].Tags[0].Label)
	}
	if cfg.Products[0].Tags[1].Label != "new" {
		t.Errorf("Products[0].Tags[1].Label = %q, want new", cfg.Products[0].Tags[1].Label)
	}
	if cfg.Products[1].Name != "Gadget" {
		t.Errorf("Products[1].Name = %q, want Gadget", cfg.Products[1].Name)
	}
	if len(cfg.Products[1].Tags) != 1 {
		t.Fatalf("len(Products[1].Tags) = %d, want 1", len(cfg.Products[1].Tags))
	}
	if cfg.Products[1].Tags[0].Label != "premium" {
		t.Errorf("Products[1].Tags[0].Label = %q, want premium", cfg.Products[1].Tags[0].Label)
	}
}

// ==========================================================================
// Audit Focus Area 3: Dotted key decoding
// ==========================================================================

func TestAudit_DottedKeyIntoNestedStruct(t *testing.T) {
	input := `a.b.c = 1`
	type C struct {
		C int `toml:"c"`
	}
	type B struct {
		B C `toml:"b"`
	}
	type Config struct {
		A B `toml:"a"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.A.B.C != 1 {
		t.Errorf("A.B.C = %d, want 1", cfg.A.B.C)
	}
}

// Dotted key where intermediate target is a map.
func TestAudit_DottedKeyIntoMapField(t *testing.T) {
	input := `meta.version = "1.0"`
	type Config struct {
		Meta map[string]any `toml:"meta"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Meta == nil {
		t.Fatal("Meta is nil")
	}
	if cfg.Meta["version"] != "1.0" {
		t.Errorf("Meta[version] = %v, want 1.0", cfg.Meta["version"])
	}
}

// Dotted key into top-level map[string]any
func TestAudit_DottedKeyTopLevelMap(t *testing.T) {
	input := `a.b.c = "deep"`
	m := map[string]any{}
	if err := Unmarshal([]byte(input), &m); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	a, ok := m["a"].(map[string]any)
	if !ok {
		t.Fatalf("m[a] is %T, want map[string]any", m["a"])
	}
	b, ok := a["b"].(map[string]any)
	if !ok {
		t.Fatalf("m[a][b] is %T, want map[string]any", a["b"])
	}
	if b["c"] != "deep" {
		t.Errorf("m[a][b][c] = %v, want deep", b["c"])
	}
}

// ==========================================================================
// Audit Focus Area 4: Map with non-string keys
// ==========================================================================

// Fails if a map target whose keys are not strings reaches reflect: TOML keys
// are strings, so the target is refused when its descriptor is derived, with
// an error naming the type -- never a panic.
func TestAudit_MapNonStringKeys(t *testing.T) {
	input := `
[data]
foo = "bar"
`
	type Config struct {
		Data map[int]string `toml:"data"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatal("Unmarshal accepted a map[int]string target")
	}
	if !strings.Contains(err.Error(), "map[int]string") {
		t.Errorf("err = %v, want the offending target type named", err)
	}
}

// ==========================================================================
// Audit Focus Area 5: Nil pointer to struct
// ==========================================================================

func TestAudit_NilPointerTarget(t *testing.T) {
	type MyStruct struct {
		Name string `toml:"name"`
	}
	var s *MyStruct
	err := Unmarshal([]byte(`name = "test"`), s)
	if err == nil {
		t.Fatal("expected error for nil pointer target, got nil")
	}
	if !strings.Contains(err.Error(), "non-nil pointer") {
		t.Errorf("error message should mention non-nil pointer, got: %v", err)
	}
}

// ==========================================================================
// Audit Focus Area 6: Decode into map[string]any
// ==========================================================================

func TestAudit_FullDocumentIntoMap(t *testing.T) {
	input := `
title = "example"
count = 10

[server]
host = "localhost"
port = 8080

[server.tls]
enabled = true

[[items]]
name = "a"

[[items]]
name = "b"
`
	m := map[string]any{}
	if err := Unmarshal([]byte(input), &m); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Top-level primitives
	if m["title"] != "example" {
		t.Errorf("title = %v, want example", m["title"])
	}
	if m["count"] != int64(10) {
		t.Errorf("count = %v (%T), want int64(10)", m["count"], m["count"])
	}

	// Nested table -> sub-map
	server, ok := m["server"].(map[string]any)
	if !ok {
		t.Fatalf("server is %T, want map[string]any", m["server"])
	}
	if server["host"] != "localhost" {
		t.Errorf("server.host = %v, want localhost", server["host"])
	}
	if server["port"] != int64(8080) {
		t.Errorf("server.port = %v, want 8080", server["port"])
	}

	// Nested sub-table
	tls, ok := server["tls"].(map[string]any)
	if !ok {
		t.Fatalf("server.tls is %T, want map[string]any", server["tls"])
	}
	if tls["enabled"] != true {
		t.Errorf("server.tls.enabled = %v, want true", tls["enabled"])
	}

	// Array of tables -> []any of map[string]any
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("items is %T, want []any", m["items"])
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	item0, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("items[0] is %T, want map[string]any", items[0])
	}
	if item0["name"] != "a" {
		t.Errorf("items[0].name = %v, want a", item0["name"])
	}
	item1, ok := items[1].(map[string]any)
	if !ok {
		t.Fatalf("items[1] is %T, want map[string]any", items[1])
	}
	if item1["name"] != "b" {
		t.Errorf("items[1].name = %v, want b", item1["name"])
	}
}

// ==========================================================================
// Audit Focus Area 7: Integer edge cases
// ==========================================================================

func TestAudit_MaxInt64IntoInt64(t *testing.T) {
	input := `val = 9223372036854775807` // math.MaxInt64
	type Config struct {
		Val int64 `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Val != math.MaxInt64 {
		t.Errorf("Val = %d, want %d", cfg.Val, int64(math.MaxInt64))
	}
}

func TestAudit_MaxInt64IntoInt32Overflow(t *testing.T) {
	input := `val = 9223372036854775807`
	type Config struct {
		Val int32 `toml:"val"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatal("expected overflow error for int64 max into int32, got nil")
	}
}

func TestAudit_NegativeIntoUint(t *testing.T) {
	input := `val = -1`
	type Config struct {
		Val uint `toml:"val"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatal("expected error for negative into uint, got nil")
	}
}

func TestAudit_MinInt64IntoInt64(t *testing.T) {
	// TOML spec allows min int64 = -9223372036854775808
	// But the parser stores as int64 -- Go's parser may not handle this.
	// Let's at least test -9223372036854775807
	input := `val = -9223372036854775807`
	type Config struct {
		Val int64 `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Val != -9223372036854775807 {
		t.Errorf("Val = %d, want -9223372036854775807", cfg.Val)
	}
}

func TestAudit_Int8Boundary(t *testing.T) {
	// int8 range: -128 to 127
	input128 := `val = 128`
	type Config struct {
		Val int8 `toml:"val"`
	}
	var cfg Config
	err := Unmarshal([]byte(input128), &cfg)
	if err == nil {
		t.Error("expected overflow error for 128 into int8")
	}

	inputNeg129 := `val = -129`
	err = Unmarshal([]byte(inputNeg129), &cfg)
	if err == nil {
		t.Error("expected overflow error for -129 into int8")
	}

	// Boundary values that should work
	input127 := `val = 127`
	if err := Unmarshal([]byte(input127), &cfg); err != nil {
		t.Errorf("127 into int8 should succeed: %v", err)
	}
	if cfg.Val != 127 {
		t.Errorf("Val = %d, want 127", cfg.Val)
	}

	inputNeg128 := `val = -128`
	if err := Unmarshal([]byte(inputNeg128), &cfg); err != nil {
		t.Errorf("-128 into int8 should succeed: %v", err)
	}
	if cfg.Val != -128 {
		t.Errorf("Val = %d, want -128", cfg.Val)
	}
}

// ==========================================================================
// Audit Focus Area 8: Float precision
// ==========================================================================

func TestAudit_FloatIntoFloat32(t *testing.T) {
	// A large float value that doesn't overflow float32 but loses precision
	input := `val = 3.141592653589793`
	type Config struct {
		Val float32 `toml:"val"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	// Float precision loss should be silent (not an error), similar to encoding/json.
	if err != nil {
		t.Fatalf("Unmarshal failed: %v (precision loss should be silent)", err)
	}
	// float32 has ~7 digits of precision
	if cfg.Val < 3.14 || cfg.Val > 3.15 {
		t.Errorf("Val = %f, expected approximately 3.14159", cfg.Val)
	}
}

func TestAudit_FloatOverflowFloat32(t *testing.T) {
	// float64 value that overflows float32
	input := `val = 3.5e+39` // float32 max is ~3.4e38
	type Config struct {
		Val float32 `toml:"val"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Error("expected overflow error for float64 value that exceeds float32 range")
	}
}

func TestAudit_FloatSpecialValues(t *testing.T) {
	// TOML supports inf and nan
	inputInf := `val = inf`
	type Config struct {
		Val float64 `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(inputInf), &cfg); err != nil {
		t.Fatalf("inf unmarshal failed: %v", err)
	}
	if !math.IsInf(cfg.Val, 1) {
		t.Errorf("val = %f, want +inf", cfg.Val)
	}

	inputNegInf := `val = -inf`
	if err := Unmarshal([]byte(inputNegInf), &cfg); err != nil {
		t.Fatalf("-inf unmarshal failed: %v", err)
	}
	if !math.IsInf(cfg.Val, -1) {
		t.Errorf("val = %f, want -inf", cfg.Val)
	}

	inputNan := `val = nan`
	if err := Unmarshal([]byte(inputNan), &cfg); err != nil {
		t.Fatalf("nan unmarshal failed: %v", err)
	}
	if !math.IsNaN(cfg.Val) {
		t.Errorf("val = %f, want NaN", cfg.Val)
	}
}

// ==========================================================================
// Audit Focus Area 9: Heterogeneous arrays
// ==========================================================================

func TestAudit_HeterogeneousArrayIntoSliceAny(t *testing.T) {
	input := `val = [1, "two", true, 3.14]`
	type Config struct {
		Val []any `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(cfg.Val) != 4 {
		t.Fatalf("len(Val) = %d, want 4", len(cfg.Val))
	}
	if cfg.Val[0] != int64(1) {
		t.Errorf("Val[0] = %v (%T), want int64(1)", cfg.Val[0], cfg.Val[0])
	}
	if cfg.Val[1] != "two" {
		t.Errorf("Val[1] = %v, want two", cfg.Val[1])
	}
	if cfg.Val[2] != true {
		t.Errorf("Val[2] = %v, want true", cfg.Val[2])
	}
	if cfg.Val[3] != 3.14 {
		t.Errorf("Val[3] = %v, want 3.14", cfg.Val[3])
	}
}

func TestAudit_HeterogeneousArrayIntoInterface(t *testing.T) {
	input := `val = [1, "two", true]`
	type Config struct {
		Val any `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	arr, ok := cfg.Val.([]any)
	if !ok {
		t.Fatalf("Val is %T, want []any", cfg.Val)
	}
	if len(arr) != 3 {
		t.Fatalf("len(Val) = %d, want 3", len(arr))
	}
	if arr[0] != int64(1) {
		t.Errorf("Val[0] = %v (%T), want int64(1)", arr[0], arr[0])
	}
	if arr[1] != "two" {
		t.Errorf("Val[1] = %v, want two", arr[1])
	}
	if arr[2] != true {
		t.Errorf("Val[2] = %v, want true", arr[2])
	}
}

// ==========================================================================
// Audit Focus Area 10: Empty inline table
// ==========================================================================

func TestAudit_EmptyInlineTableIntoStruct(t *testing.T) {
	input := `val = {}`
	type Inner struct {
		X int `toml:"x"`
	}
	type Config struct {
		Val Inner `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	// All fields should be zero values
	if cfg.Val.X != 0 {
		t.Errorf("Val.X = %d, want 0", cfg.Val.X)
	}
}

func TestAudit_EmptyInlineTableIntoMap(t *testing.T) {
	input := `val = {}`
	type Config struct {
		Val map[string]any `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Val == nil {
		t.Fatal("Val should be non-nil empty map")
	}
	if len(cfg.Val) != 0 {
		t.Errorf("len(Val) = %d, want 0", len(cfg.Val))
	}
}

func TestAudit_EmptyInlineTableIntoInterface(t *testing.T) {
	input := `val = {}`
	type Config struct {
		Val any `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	m, ok := cfg.Val.(map[string]any)
	if !ok {
		t.Fatalf("Val is %T, want map[string]any", cfg.Val)
	}
	if len(m) != 0 {
		t.Errorf("len(Val) = %d, want 0", len(m))
	}
}

// ==========================================================================
// Additional edge cases: Case-insensitive matching details
// ==========================================================================

func TestAudit_CaseInsensitive_AllCaps(t *testing.T) {
	// TOML key "HOST" should match struct field "Host" case-insensitively
	input := `HOST = "example.com"`
	type Config struct {
		Host string // no tag, field name is "Host"
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	// BUG check: lookup("HOST") should try strings.ToLower("HOST") = "host"
	// and find the fallback for "Host". If this fails, lookup is incomplete.
	if cfg.Host != "example.com" {
		t.Errorf("Host = %q, want %q (case-insensitive match for HOST -> Host failed)", cfg.Host, "example.com")
	}
}

func TestAudit_CaseInsensitive_ExactFirst(t *testing.T) {
	// When there's an exact match AND a case-insensitive match, exact wins.
	input := `Name = "exact"`
	type Config struct {
		Name string `toml:"Name"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Name != "exact" {
		t.Errorf("Name = %q, want %q", cfg.Name, "exact")
	}
}

// ==========================================================================
// Additional edge cases: Pointer handling
// ==========================================================================

func TestAudit_PointerToStruct(t *testing.T) {
	input := `
[server]
host = "localhost"
`
	type Server struct {
		Host string `toml:"host"`
	}
	type Config struct {
		Server *Server `toml:"server"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Server == nil {
		t.Fatal("Server pointer not allocated")
	}
	if cfg.Server.Host != "localhost" {
		t.Errorf("Server.Host = %q, want localhost", cfg.Server.Host)
	}
}

func TestAudit_PointerSliceElements(t *testing.T) {
	input := `
[[items]]
name = "first"

[[items]]
name = "second"
`
	type Item struct {
		Name string `toml:"name"`
	}
	type Config struct {
		Items []*Item `toml:"items"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(cfg.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(cfg.Items))
	}
	if cfg.Items[0] == nil || cfg.Items[0].Name != "first" {
		t.Errorf("Items[0] = %v, want &{first}", cfg.Items[0])
	}
	if cfg.Items[1] == nil || cfg.Items[1].Name != "second" {
		t.Errorf("Items[1] = %v, want &{second}", cfg.Items[1])
	}
}

// ==========================================================================
// Additional edge cases: Decode method (Document.Decode)
// ==========================================================================

func TestAudit_DecodeNonPointer(t *testing.T) {
	doc := &Document{}
	type Config struct{}
	var cfg Config
	err := doc.Decode(cfg) // not &cfg
	if err == nil {
		t.Fatal("expected error for non-pointer Decode target")
	}
}

func TestAudit_DecodeNilPointer(t *testing.T) {
	doc := &Document{}
	var cfg *struct{ Name string }
	err := doc.Decode(cfg)
	if err == nil {
		t.Fatal("expected error for nil pointer Decode target")
	}
}

// ==========================================================================
// Additional edge cases: Error messages contain path and type
// ==========================================================================

func TestAudit_ErrorMessageContainsPath(t *testing.T) {
	input := `
[server]
port = "not_a_number"
`
	type Server struct {
		Port int `toml:"port"`
	}
	type Config struct {
		Server Server `toml:"server"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatal("expected error for string into int")
	}
	errStr := err.Error()
	// Should contain path info like "server.port"
	if !strings.Contains(errStr, "server.port") {
		t.Errorf("error should contain path 'server.port', got: %s", errStr)
	}
	// Should contain type info
	if !strings.Contains(errStr, "int") {
		t.Errorf("error should contain type info 'int', got: %s", errStr)
	}
}

func TestAudit_ErrorMessageOverflow(t *testing.T) {
	input := `val = 999`
	type Config struct {
		Val int8 `toml:"val"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatal("expected overflow error")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "overflow") {
		t.Errorf("error should mention 'overflow', got: %s", errStr)
	}
	if !strings.Contains(errStr, "int8") {
		t.Errorf("error should mention target type 'int8', got: %s", errStr)
	}
}

// ==========================================================================
// Additional edge cases: unknown keys inside a nested table
// ==========================================================================

// Fails if an unknown key inside a nested table is ignored, or if its
// diagnostic stops naming the full path to it.
func TestAudit_ExtraKeysInNestedTable(t *testing.T) {
	input := `
[server]
host = "localhost"
unknown_key = "should be refused"
another_unknown = 999
`
	type Server struct {
		Host string `toml:"host"`
	}
	type Config struct {
		Server Server `toml:"server"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatalf("Unmarshal accepted unknown keys in a nested table: %+v", cfg)
	}
	var all *Errors
	if !errors.As(err, &all) {
		t.Fatalf("err = %v (%T), want an aggregate", err, err)
	}
	var paths []string
	for _, d := range all.Unwrap() {
		var diag *Error
		if errors.As(d, &diag) {
			paths = append(paths, diag.Path)
		}
	}
	if !reflect.DeepEqual(paths, []string{"server.unknown_key", "server.another_unknown"}) {
		t.Errorf("diagnostics name %v, want both nested keys in document order", paths)
	}
	if cfg.Server.Host != "localhost" {
		t.Errorf("Host = %q, want localhost", cfg.Server.Host)
	}
}

// ==========================================================================
// Additional edge cases: Array fixed size mismatch
// ==========================================================================

func TestAudit_ArrayTooManyElements(t *testing.T) {
	input := `vals = [1, 2, 3, 4, 5]`
	type Config struct {
		Vals [3]int `toml:"vals"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Error("expected error when array has more elements than fixed-size target")
	}
}

// Fails if an under-filled fixed-size array is accepted again: zero-padding
// invents values the document never carried. The any-length spelling is a
// slice.
func TestAudit_ArrayFewerElements(t *testing.T) {
	input := `vals = [1, 2]`
	type Config struct {
		Vals [5]int `toml:"vals"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatalf("Unmarshal accepted two elements into [5]int: %v", cfg.Vals)
	}
	if !errors.Is(err, ErrInexact) {
		t.Fatalf("err = %v, want an inexact diagnostic", err)
	}
	if cfg.Vals != [5]int{} {
		t.Errorf("Vals = %v, want the target untouched", cfg.Vals)
	}
}

// ==========================================================================
// Additional: integer to float coercion
// ==========================================================================

func TestAudit_IntegerToFloat32(t *testing.T) {
	input := `val = 42`
	type Config struct {
		Val float32 `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("integer to float32 should succeed: %v", err)
	}
	if cfg.Val != 42.0 {
		t.Errorf("Val = %f, want 42.0", cfg.Val)
	}
}

// ==========================================================================
// Additional: Unmarshaler takes precedence over default
// ==========================================================================

func TestAudit_UnmarshalerPrecedenceOverTextUnmarshaler(t *testing.T) {
	// A type that implements both Unmarshaler and TextUnmarshaler.
	// Unmarshaler should take precedence.
	input := `val = "hello"`
	type Config struct {
		Val dualUnmarshaler `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Val.Via != "toml" {
		t.Errorf("expected Unmarshaler (toml) to take precedence, got Via=%q", cfg.Val.Via)
	}
}

// dualUnmarshaler implements both Unmarshaler and encoding.TextUnmarshaler.
type dualUnmarshaler struct {
	Via   string
	Value string
}

func (d *dualUnmarshaler) UnmarshalTOML(node Node) error {
	d.Via = "toml"
	if node.Type() == NodeString {
		d.Value = node.(*StringNode).Val
	}
	return nil
}

func (d *dualUnmarshaler) UnmarshalText(text []byte) error {
	d.Via = "text"
	d.Value = string(text)
	return nil
}

// ==========================================================================
// Additional: local types -> time.Time conversion
// ==========================================================================

func TestAudit_LocalDateTimeToTimeTime(t *testing.T) {
	input := `val = 2024-03-15T09:30:00`
	type Config struct {
		Val time.Time `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	expected := time.Date(2024, 3, 15, 9, 30, 0, 0, time.UTC)
	if !cfg.Val.Equal(expected) {
		t.Errorf("Val = %v, want %v", cfg.Val, expected)
	}
}

func TestAudit_LocalDateToTimeTime(t *testing.T) {
	input := `val = 2024-03-15`
	type Config struct {
		Val time.Time `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	expected := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !cfg.Val.Equal(expected) {
		t.Errorf("Val = %v, want %v", cfg.Val, expected)
	}
}

// ==========================================================================
// Additional: Decode into any (top-level interface)
// ==========================================================================

func TestAudit_DecodeIntoAny(t *testing.T) {
	input := `
name = "test"
value = 42
`
	var v any
	if err := Unmarshal([]byte(input), &v); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("v is %T, want map[string]any", v)
	}
	if m["name"] != "test" {
		t.Errorf("name = %v, want test", m["name"])
	}
	if m["value"] != int64(42) {
		t.Errorf("value = %v, want 42", m["value"])
	}
}

// ==========================================================================
// Additional: Array of tables into map[string]any with sub-tables
// ==========================================================================

func TestAudit_ArrayTableSubTableIntoMap(t *testing.T) {
	input := `
[[products]]
name = "Hammer"

[products.details]
weight = 1.5

[[products]]
name = "Nail"

[products.details]
weight = 0.01
`
	m := map[string]any{}
	if err := Unmarshal([]byte(input), &m); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	products, ok := m["products"].([]any)
	if !ok {
		t.Fatalf("products is %T, want []any", m["products"])
	}
	if len(products) != 2 {
		t.Fatalf("len(products) = %d, want 2", len(products))
	}

	p0, ok := products[0].(map[string]any)
	if !ok {
		t.Fatalf("products[0] is %T, want map[string]any", products[0])
	}
	if p0["name"] != "Hammer" {
		t.Errorf("products[0].name = %v, want Hammer", p0["name"])
	}
	details0, ok := p0["details"].(map[string]any)
	if !ok {
		t.Fatalf("products[0].details is %T, want map[string]any", p0["details"])
	}
	if details0["weight"] != 1.5 {
		t.Errorf("products[0].details.weight = %v, want 1.5", details0["weight"])
	}

	p1, ok := products[1].(map[string]any)
	if !ok {
		t.Fatalf("products[1] is %T, want map[string]any", products[1])
	}
	if p1["name"] != "Nail" {
		t.Errorf("products[1].name = %v, want Nail", p1["name"])
	}
	details1, ok := p1["details"].(map[string]any)
	if !ok {
		t.Fatalf("products[1].details is %T, want map[string]any", p1["details"])
	}
	if details1["weight"] != 0.01 {
		t.Errorf("products[1].details.weight = %v, want 0.01", details1["weight"])
	}
}

// ==========================================================================
// Additional: Inline table with dotted keys
// ==========================================================================

func TestAudit_InlineTableWithDottedKeys(t *testing.T) {
	input := `val = {a.b = "nested"}`
	type Config struct {
		Val map[string]any `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	a, ok := cfg.Val["a"].(map[string]any)
	if !ok {
		t.Fatalf("Val[a] is %T, want map[string]any", cfg.Val["a"])
	}
	if a["b"] != "nested" {
		t.Errorf("Val[a][b] = %v, want nested", a["b"])
	}
}

// ==========================================================================
// Verify DeepEqual on full decode
// ==========================================================================

// Fails if Unmarshal and Parse+Decode stop producing the same values, or stop
// producing the RIGHT ones: the two entry points agreeing on nothing is also
// agreement.
func TestAudit_UnmarshalEqualsDecodeForComplexDoc(t *testing.T) {
	input := `
title = "audit"

[db]
host = "pg"
port = 5432

[[db.replicas]]
host = "r1"

[[db.replicas]]
host = "r2"
`
	type Replica struct {
		Host string `toml:"host"`
	}
	type DB struct {
		Host     string    `toml:"host"`
		Port     int       `toml:"port"`
		Replicas []Replica `toml:"replicas"`
	}
	type Config struct {
		Title string `toml:"title"`
		DB    DB     `toml:"db"`
	}

	// What the document says, spelled out: comparing the two decodes to each
	// other alone would pass just as happily if both of them decoded nothing.
	want := Config{
		Title: "audit",
		DB: DB{
			Host:     "pg",
			Port:     5432,
			Replicas: []Replica{{Host: "r1"}, {Host: "r2"}},
		},
	}

	var cfg1 Config
	if err := Unmarshal([]byte(input), &cfg1); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(cfg1, want) {
		t.Errorf("Unmarshal decoded %+v, want %+v", cfg1, want)
	}

	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	var cfg2 Config
	if err := doc.Decode(&cfg2); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !reflect.DeepEqual(cfg2, want) {
		t.Errorf("Decode decoded %+v, want %+v", cfg2, want)
	}

	if !reflect.DeepEqual(cfg1, cfg2) {
		t.Errorf("Unmarshal != Decode:\n  Unmarshal: %+v\n  Decode:    %+v", cfg1, cfg2)
	}
}
