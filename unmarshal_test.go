package tomledit

import (
	"encoding"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// --- test helpers ---

func mustParse(t *testing.T, input string) *Document {
	t.Helper()
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	return doc
}

// --- custom types for testing ---

// customUnmarshaler implements Unmarshaler.
type customUnmarshaler struct {
	Data string
}

func (c *customUnmarshaler) UnmarshalTOML(node Node) error {
	if node.Type() == NodeString {
		c.Data = "custom:" + node.(*StringNode).Val
		return nil
	}
	return fmt.Errorf("customUnmarshaler: expected string, got %s", node.Type())
}

// Verify interface compliance.
var _ Unmarshaler = (*customUnmarshaler)(nil)

// textUnmarshalerType implements encoding.TextUnmarshaler.
type textUnmarshalerType struct {
	Value string
}

func (t *textUnmarshalerType) UnmarshalText(text []byte) error {
	t.Value = "text:" + string(text)
	return nil
}

var _ encoding.TextUnmarshaler = (*textUnmarshalerType)(nil)

// --- 1. Basic types ---

func TestUnmarshal_BasicTypes(t *testing.T) {
	input := `
name = "Alice"
age = 30
height = 1.75
active = true
score_i8 = 127
score_i16 = 32000
score_i32 = 2000000
score_i64 = 9000000000
score_u = 42
score_u8 = 255
score_u16 = 65535
score_u32 = 4000000000
score_u64 = 18000000000
`
	type Config struct {
		Name     string  `toml:"name"`
		Age      int     `toml:"age"`
		Height   float64 `toml:"height"`
		Active   bool    `toml:"active"`
		ScoreI8  int8    `toml:"score_i8"`
		ScoreI16 int16   `toml:"score_i16"`
		ScoreI32 int32   `toml:"score_i32"`
		ScoreI64 int64   `toml:"score_i64"`
		ScoreU   uint    `toml:"score_u"`
		ScoreU8  uint8   `toml:"score_u8"`
		ScoreU16 uint16  `toml:"score_u16"`
		ScoreU32 uint32  `toml:"score_u32"`
		ScoreU64 uint64  `toml:"score_u64"`
	}

	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Name != "Alice" {
		t.Errorf("Name = %q, want %q", cfg.Name, "Alice")
	}
	if cfg.Age != 30 {
		t.Errorf("Age = %d, want %d", cfg.Age, 30)
	}
	if cfg.Height != 1.75 {
		t.Errorf("Height = %f, want %f", cfg.Height, 1.75)
	}
	if !cfg.Active {
		t.Error("Active = false, want true")
	}
	if cfg.ScoreI8 != 127 {
		t.Errorf("ScoreI8 = %d, want 127", cfg.ScoreI8)
	}
	if cfg.ScoreI16 != 32000 {
		t.Errorf("ScoreI16 = %d, want 32000", cfg.ScoreI16)
	}
	if cfg.ScoreI32 != 2000000 {
		t.Errorf("ScoreI32 = %d, want 2000000", cfg.ScoreI32)
	}
	if cfg.ScoreI64 != 9000000000 {
		t.Errorf("ScoreI64 = %d, want 9000000000", cfg.ScoreI64)
	}
	if cfg.ScoreU != 42 {
		t.Errorf("ScoreU = %d, want 42", cfg.ScoreU)
	}
	if cfg.ScoreU8 != 255 {
		t.Errorf("ScoreU8 = %d, want 255", cfg.ScoreU8)
	}
	if cfg.ScoreU16 != 65535 {
		t.Errorf("ScoreU16 = %d, want 65535", cfg.ScoreU16)
	}
	if cfg.ScoreU32 != 4000000000 {
		t.Errorf("ScoreU32 = %d, want 4000000000", cfg.ScoreU32)
	}
	if cfg.ScoreU64 != 18000000000 {
		t.Errorf("ScoreU64 = %d, want 18000000000", cfg.ScoreU64)
	}
}

// --- 2. Time types ---

func TestUnmarshal_TimeTypes(t *testing.T) {
	input := `
odt = 2024-01-15T10:30:00Z
ldt = 2024-01-15T10:30:00
ld = 2024-01-15
lt = 10:30:00
`
	type Config struct {
		Odt time.Time     `toml:"odt"`
		Ldt LocalDateTime `toml:"ldt"`
		Ld  LocalDate     `toml:"ld"`
		Lt  LocalTime     `toml:"lt"`
	}

	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	expectedODT := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if !cfg.Odt.Equal(expectedODT) {
		t.Errorf("Odt = %v, want %v", cfg.Odt, expectedODT)
	}
	if cfg.Ldt.Year != 2024 || cfg.Ldt.Month != 1 || cfg.Ldt.Day != 15 {
		t.Errorf("Ldt date = %d-%d-%d, want 2024-1-15", cfg.Ldt.Year, cfg.Ldt.Month, cfg.Ldt.Day)
	}
	if cfg.Ldt.Hour != 10 || cfg.Ldt.Minute != 30 {
		t.Errorf("Ldt time = %d:%d, want 10:30", cfg.Ldt.Hour, cfg.Ldt.Minute)
	}
	if cfg.Ld.Year != 2024 || cfg.Ld.Month != 1 || cfg.Ld.Day != 15 {
		t.Errorf("Ld = %d-%d-%d, want 2024-1-15", cfg.Ld.Year, cfg.Ld.Month, cfg.Ld.Day)
	}
	if cfg.Lt.Hour != 10 || cfg.Lt.Minute != 30 {
		t.Errorf("Lt = %d:%d, want 10:30", cfg.Lt.Hour, cfg.Lt.Minute)
	}
}

func TestUnmarshal_LocalDateTimeAsTime(t *testing.T) {
	input := `ldt = 2024-06-15T14:30:00`
	type Config struct {
		Ldt time.Time `toml:"ldt"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	expected := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)
	if !cfg.Ldt.Equal(expected) {
		t.Errorf("Ldt = %v, want %v", cfg.Ldt, expected)
	}
}

func TestUnmarshal_LocalDateAsTime(t *testing.T) {
	input := `ld = 2024-06-15`
	type Config struct {
		Ld time.Time `toml:"ld"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	expected := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	if !cfg.Ld.Equal(expected) {
		t.Errorf("Ld = %v, want %v", cfg.Ld, expected)
	}
}

// --- 3. Nested structs ---

func TestUnmarshal_NestedStructs(t *testing.T) {
	input := `
[server]
host = "localhost"
port = 8080
`
	type Server struct {
		Host string `toml:"host"`
		Port int    `toml:"port"`
	}
	type Config struct {
		Server Server `toml:"server"`
	}

	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Server.Host != "localhost" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "localhost")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 8080)
	}
}

// --- 4. Slices ---

func TestUnmarshal_Slices(t *testing.T) {
	input := `
ints = [1, 2, 3]
strings = ["a", "b", "c"]
`
	type Config struct {
		Ints    []int    `toml:"ints"`
		Strings []string `toml:"strings"`
	}

	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(cfg.Ints, []int{1, 2, 3}) {
		t.Errorf("Ints = %v, want [1 2 3]", cfg.Ints)
	}
	if !reflect.DeepEqual(cfg.Strings, []string{"a", "b", "c"}) {
		t.Errorf("Strings = %v, want [a b c]", cfg.Strings)
	}
}

func TestUnmarshal_FixedArray(t *testing.T) {
	input := `vals = [10, 20, 30]`
	type Config struct {
		Vals [3]int `toml:"vals"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Vals != [3]int{10, 20, 30} {
		t.Errorf("Vals = %v, want [10 20 30]", cfg.Vals)
	}
}

// --- 5. Array of tables ---

func TestUnmarshal_ArrayOfTables(t *testing.T) {
	input := `
[[products]]
name = "Hammer"
price = 9.99

[[products]]
name = "Nail"
price = 0.05
`
	type Product struct {
		Name  string  `toml:"name"`
		Price float64 `toml:"price"`
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
	if cfg.Products[0].Name != "Hammer" {
		t.Errorf("Products[0].Name = %q, want %q", cfg.Products[0].Name, "Hammer")
	}
	if cfg.Products[0].Price != 9.99 {
		t.Errorf("Products[0].Price = %f, want 9.99", cfg.Products[0].Price)
	}
	if cfg.Products[1].Name != "Nail" {
		t.Errorf("Products[1].Name = %q, want %q", cfg.Products[1].Name, "Nail")
	}
}

// --- 6. Maps ---

func TestUnmarshal_MapStringAny(t *testing.T) {
	input := `
[config]
key1 = "value1"
key2 = 42
`
	type Config struct {
		Config map[string]any `toml:"config"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Config["key1"] != "value1" {
		t.Errorf("Config[key1] = %v, want value1", cfg.Config["key1"])
	}
	if cfg.Config["key2"] != int64(42) {
		t.Errorf("Config[key2] = %v (%T), want 42 (int64)", cfg.Config["key2"], cfg.Config["key2"])
	}
}

func TestUnmarshal_MapStringString(t *testing.T) {
	input := `
[labels]
env = "prod"
team = "backend"
`
	type Config struct {
		Labels map[string]string `toml:"labels"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Labels["env"] != "prod" {
		t.Errorf("Labels[env] = %q, want %q", cfg.Labels["env"], "prod")
	}
	if cfg.Labels["team"] != "backend" {
		t.Errorf("Labels[team] = %q, want %q", cfg.Labels["team"], "backend")
	}
}

func TestUnmarshal_TopLevelMap(t *testing.T) {
	input := `
name = "test"
value = 42
`
	m := map[string]any{}
	if err := Unmarshal([]byte(input), &m); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if m["name"] != "test" {
		t.Errorf("m[name] = %v, want test", m["name"])
	}
	if m["value"] != int64(42) {
		t.Errorf("m[value] = %v, want 42", m["value"])
	}
}

// --- 7. Inline tables ---

func TestUnmarshal_InlineTable(t *testing.T) {
	input := `point = {x = 1, y = 2}`
	type Point struct {
		X int `toml:"x"`
		Y int `toml:"y"`
	}
	type Config struct {
		Point Point `toml:"point"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Point.X != 1 || cfg.Point.Y != 2 {
		t.Errorf("Point = {%d, %d}, want {1, 2}", cfg.Point.X, cfg.Point.Y)
	}
}

func TestUnmarshal_InlineTableToMap(t *testing.T) {
	input := `point = {x = 1, y = 2}`
	type Config struct {
		Point map[string]int64 `toml:"point"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Point["x"] != 1 || cfg.Point["y"] != 2 {
		t.Errorf("Point = %v, want {x:1, y:2}", cfg.Point)
	}
}

// --- 8. Struct tags ---

func TestUnmarshal_StructTags(t *testing.T) {
	input := `
custom_name = "hello"
skipped = "should not appear"
with_empty = "value"
`
	type Config struct {
		CustomField string `toml:"custom_name"`
		Skipped     string `toml:"-"`
		WithEmpty   string `toml:"with_empty,omitempty"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.CustomField != "hello" {
		t.Errorf("CustomField = %q, want %q", cfg.CustomField, "hello")
	}
	if cfg.Skipped != "" {
		t.Errorf("Skipped = %q, want empty (should be skipped)", cfg.Skipped)
	}
	if cfg.WithEmpty != "value" {
		t.Errorf("WithEmpty = %q, want %q", cfg.WithEmpty, "value")
	}
}

func TestUnmarshal_EmbeddedStruct(t *testing.T) {
	input := `
name = "test"
host = "localhost"
port = 8080
`
	type Base struct {
		Host string `toml:"host"`
		Port int    `toml:"port"`
	}
	type Config struct {
		Base
		Name string `toml:"name"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Name != "test" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test")
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host = %q, want %q", cfg.Host, "localhost")
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want %d", cfg.Port, 8080)
	}
}

func TestUnmarshal_EmbeddedStructWithTag(t *testing.T) {
	input := `
[base]
host = "localhost"
port = 8080
`
	type Base struct {
		Host string `toml:"host"`
		Port int    `toml:"port"`
	}
	type Config struct {
		Base Base `toml:"base"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Base.Host != "localhost" {
		t.Errorf("Base.Host = %q, want %q", cfg.Base.Host, "localhost")
	}
}

// --- 9. Pointer fields ---

func TestUnmarshal_PointerFields(t *testing.T) {
	input := `
name = "test"
count = 42
`
	type Config struct {
		Name  *string `toml:"name"`
		Count *int    `toml:"count"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Name == nil || *cfg.Name != "test" {
		t.Errorf("Name = %v, want pointer to 'test'", cfg.Name)
	}
	if cfg.Count == nil || *cfg.Count != 42 {
		t.Errorf("Count = %v, want pointer to 42", cfg.Count)
	}
}

func TestUnmarshal_DoublePointer(t *testing.T) {
	input := `name = "deep"`
	type Config struct {
		Name **string `toml:"name"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Name == nil || *cfg.Name == nil || **cfg.Name != "deep" {
		t.Error("double pointer not allocated correctly")
	}
}

func TestUnmarshal_NilPointerUntouched(t *testing.T) {
	input := `other = "x"`
	type Config struct {
		Name  *string `toml:"name"`
		Other string  `toml:"other"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Name != nil {
		t.Errorf("Name should be nil but got %v", cfg.Name)
	}
	if cfg.Other != "x" {
		t.Errorf("Other = %q, want %q", cfg.Other, "x")
	}
}

// --- 10. Interface fields ---

func TestUnmarshal_InterfaceFields(t *testing.T) {
	input := `
str = "hello"
num = 42
flt = 3.14
boo = true
arr = [1, 2, 3]
`
	type Config struct {
		Str any `toml:"str"`
		Num any `toml:"num"`
		Flt any `toml:"flt"`
		Boo any `toml:"boo"`
		Arr any `toml:"arr"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Str != "hello" {
		t.Errorf("Str = %v, want hello", cfg.Str)
	}
	if cfg.Num != int64(42) {
		t.Errorf("Num = %v (%T), want 42 (int64)", cfg.Num, cfg.Num)
	}
	if cfg.Flt != 3.14 {
		t.Errorf("Flt = %v, want 3.14", cfg.Flt)
	}
	if cfg.Boo != true {
		t.Errorf("Boo = %v, want true", cfg.Boo)
	}
	arr, ok := cfg.Arr.([]any)
	if !ok || len(arr) != 3 {
		t.Errorf("Arr = %v, want []any{1,2,3}", cfg.Arr)
	}
}

// --- 11. Case-insensitive matching ---

func TestUnmarshal_CaseInsensitive(t *testing.T) {
	input := `host = "localhost"`
	type Config struct {
		Host string
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host = %q, want %q", cfg.Host, "localhost")
	}
}

func TestUnmarshal_CaseInsensitiveLower(t *testing.T) {
	// TOML key "host" matches struct field "Host" case-insensitively.
	input := `host = "example.com"`
	type Config struct {
		Host string // no tag, field name is "Host"
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Host != "example.com" {
		t.Errorf("Host = %q, want %q", cfg.Host, "example.com")
	}
}

// --- 12. Custom Unmarshaler ---

func TestUnmarshal_CustomUnmarshaler(t *testing.T) {
	input := `val = "world"`
	type Config struct {
		Val customUnmarshaler `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Val.Data != "custom:world" {
		t.Errorf("Val.Data = %q, want %q", cfg.Val.Data, "custom:world")
	}
}

// --- 13. TextUnmarshaler ---

func TestUnmarshal_TextUnmarshaler(t *testing.T) {
	input := `val = "hello"`
	type Config struct {
		Val textUnmarshalerType `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Val.Value != "text:hello" {
		t.Errorf("Val.Value = %q, want %q", cfg.Val.Value, "text:hello")
	}
}

// --- 14. Overflow ---

func TestUnmarshal_IntOverflow(t *testing.T) {
	input := `val = 256`
	type Config struct {
		Val int8 `toml:"val"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatal("expected overflow error, got nil")
	}
}

func TestUnmarshal_UintOverflow(t *testing.T) {
	input := `val = 300`
	type Config struct {
		Val uint8 `toml:"val"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatal("expected overflow error, got nil")
	}
}

func TestUnmarshal_NegativeUint(t *testing.T) {
	input := `val = -1`
	type Config struct {
		Val uint `toml:"val"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatal("expected negative-to-uint error, got nil")
	}
}

// --- 15. Type mismatch ---

func TestUnmarshal_TypeMismatch(t *testing.T) {
	input := `val = 42`
	type Config struct {
		Val string `toml:"val"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatal("expected type mismatch error, got nil")
	}
}

func TestUnmarshal_TypeMismatchBoolToInt(t *testing.T) {
	input := `val = true`
	type Config struct {
		Val int `toml:"val"`
	}
	var cfg Config
	err := Unmarshal([]byte(input), &cfg)
	if err == nil {
		t.Fatal("expected type mismatch error, got nil")
	}
}

// --- 16. Non-pointer target ---

func TestUnmarshal_NonPointerTarget(t *testing.T) {
	type Config struct {
		Name string `toml:"name"`
	}
	var cfg Config
	err := Unmarshal([]byte(`name = "x"`), cfg) // not &cfg
	if err == nil {
		t.Fatal("expected error for non-pointer target, got nil")
	}
}

func TestUnmarshal_NilPointerTarget(t *testing.T) {
	var cfg *struct{ Name string }
	err := Unmarshal([]byte(`name = "x"`), cfg) // nil pointer
	if err == nil {
		t.Fatal("expected error for nil pointer target, got nil")
	}
}

// --- 17. Empty document ---

func TestUnmarshal_EmptyDocument(t *testing.T) {
	type Config struct {
		Name  string `toml:"name"`
		Count int    `toml:"count"`
	}
	var cfg Config
	if err := Unmarshal([]byte(""), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Name != "" {
		t.Errorf("Name = %q, want empty", cfg.Name)
	}
	if cfg.Count != 0 {
		t.Errorf("Count = %d, want 0", cfg.Count)
	}
}

// --- 18. Extra keys ---

func TestUnmarshal_ExtraKeys(t *testing.T) {
	input := `
name = "test"
unknown = "ignored"
also_unknown = 999
`
	type Config struct {
		Name string `toml:"name"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Name != "test" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test")
	}
}

// --- 19. Deeply nested ---

func TestUnmarshal_DeeplyNested(t *testing.T) {
	input := `
[a]
[a.b]
[a.b.c]
[a.b.c.d]
[a.b.c.d.e]
val = "deep"
`
	type E struct {
		Val string `toml:"val"`
	}
	type D struct {
		E E `toml:"e"`
	}
	type C struct {
		D D `toml:"d"`
	}
	type B struct {
		C C `toml:"c"`
	}
	type A struct {
		B B `toml:"b"`
	}
	type Config struct {
		A A `toml:"a"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.A.B.C.D.E.Val != "deep" {
		t.Errorf("deeply nested val = %q, want %q", cfg.A.B.C.D.E.Val, "deep")
	}
}

// --- 20. Array of tables with sub-tables ---

func TestUnmarshal_ArrayTablesWithSubTables(t *testing.T) {
	input := `
[[products]]
name = "Hammer"

[products.details]
weight = 1.5
color = "red"

[[products]]
name = "Nail"

[products.details]
weight = 0.01
color = "silver"
`
	type Details struct {
		Weight float64 `toml:"weight"`
		Color  string  `toml:"color"`
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
	if cfg.Products[0].Name != "Hammer" {
		t.Errorf("Products[0].Name = %q, want Hammer", cfg.Products[0].Name)
	}
	if cfg.Products[0].Details.Weight != 1.5 {
		t.Errorf("Products[0].Details.Weight = %f, want 1.5", cfg.Products[0].Details.Weight)
	}
	if cfg.Products[0].Details.Color != "red" {
		t.Errorf("Products[0].Details.Color = %q, want red", cfg.Products[0].Details.Color)
	}
	if cfg.Products[1].Name != "Nail" {
		t.Errorf("Products[1].Name = %q, want Nail", cfg.Products[1].Name)
	}
	if cfg.Products[1].Details.Weight != 0.01 {
		t.Errorf("Products[1].Details.Weight = %f, want 0.01", cfg.Products[1].Details.Weight)
	}
}

// --- 21. Dotted keys ---

func TestUnmarshal_DottedKeys(t *testing.T) {
	input := `
a.b.c = "nested"
`
	type C struct {
		C string `toml:"c"`
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
	if cfg.A.B.C != "nested" {
		t.Errorf("A.B.C = %q, want %q", cfg.A.B.C, "nested")
	}
}

// --- 22. Unmarshal convenience ---

func TestUnmarshal_ConvenienceEquality(t *testing.T) {
	input := `
name = "test"
value = 42
`
	type Config struct {
		Name  string `toml:"name"`
		Value int    `toml:"value"`
	}

	// Via Unmarshal
	var cfg1 Config
	if err := Unmarshal([]byte(input), &cfg1); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Via Parse + Decode
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	var cfg2 Config
	if err := doc.Decode(&cfg2); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if !reflect.DeepEqual(cfg1, cfg2) {
		t.Errorf("Unmarshal and Parse+Decode produced different results:\n  Unmarshal: %+v\n  Decode:    %+v", cfg1, cfg2)
	}
}

// --- 23. Full round-trip ---

func TestUnmarshal_FullRoundTrip(t *testing.T) {
	input := `
title = "TOML Example"
debug = false

[owner]
name = "Tom Preston-Werner"
dob = 1979-05-27T07:32:00Z

[database]
enabled = true
ports = [8001, 8001, 8002]
data = [["delta", "phi"], [3.14]]
temp_targets = {cpu = 79.5, case = 72.0}

[[servers]]
name = "alpha"
ip = "10.0.0.1"

[[servers]]
name = "beta"
ip = "10.0.0.2"
`
	type Owner struct {
		Name string    `toml:"name"`
		Dob  time.Time `toml:"dob"`
	}
	type Database struct {
		Enabled     bool               `toml:"enabled"`
		Ports       []int              `toml:"ports"`
		Data        []any              `toml:"data"`
		TempTargets map[string]float64 `toml:"temp_targets"`
	}
	type Server struct {
		Name string `toml:"name"`
		IP   string `toml:"ip"`
	}
	type Config struct {
		Title    string   `toml:"title"`
		Debug    bool     `toml:"debug"`
		Owner    Owner    `toml:"owner"`
		Database Database `toml:"database"`
		Servers  []Server `toml:"servers"`
	}

	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Title != "TOML Example" {
		t.Errorf("Title = %q", cfg.Title)
	}
	if cfg.Debug != false {
		t.Error("Debug should be false")
	}
	if cfg.Owner.Name != "Tom Preston-Werner" {
		t.Errorf("Owner.Name = %q", cfg.Owner.Name)
	}
	expectedDob := time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC)
	if !cfg.Owner.Dob.Equal(expectedDob) {
		t.Errorf("Owner.Dob = %v, want %v", cfg.Owner.Dob, expectedDob)
	}
	if !cfg.Database.Enabled {
		t.Error("Database.Enabled should be true")
	}
	if len(cfg.Database.Ports) != 3 {
		t.Errorf("Database.Ports length = %d, want 3", len(cfg.Database.Ports))
	}
	if cfg.Database.Ports[0] != 8001 {
		t.Errorf("Database.Ports[0] = %d, want 8001", cfg.Database.Ports[0])
	}
	if len(cfg.Database.Data) != 2 {
		t.Errorf("Database.Data length = %d, want 2", len(cfg.Database.Data))
	}
	if cfg.Database.TempTargets["cpu"] != 79.5 {
		t.Errorf("Database.TempTargets[cpu] = %f, want 79.5", cfg.Database.TempTargets["cpu"])
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("Servers length = %d, want 2", len(cfg.Servers))
	}
	if cfg.Servers[0].Name != "alpha" || cfg.Servers[0].IP != "10.0.0.1" {
		t.Errorf("Servers[0] = %+v", cfg.Servers[0])
	}
	if cfg.Servers[1].Name != "beta" || cfg.Servers[1].IP != "10.0.0.2" {
		t.Errorf("Servers[1] = %+v", cfg.Servers[1])
	}
}

// --- Additional edge cases ---

func TestUnmarshal_Float32(t *testing.T) {
	input := `val = 3.14`
	type Config struct {
		Val float32 `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Val < 3.13 || cfg.Val > 3.15 {
		t.Errorf("Val = %f, want ~3.14", cfg.Val)
	}
}

func TestUnmarshal_IntegerToFloat(t *testing.T) {
	input := `val = 42`
	type Config struct {
		Val float64 `toml:"val"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Val != 42.0 {
		t.Errorf("Val = %f, want 42.0", cfg.Val)
	}
}

func TestDecode_TopLevelMapStringAny(t *testing.T) {
	input := `
name = "test"

[server]
host = "localhost"
`
	doc := mustParse(t, input)
	m := map[string]any{}
	if err := doc.Decode(&m); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if m["name"] != "test" {
		t.Errorf("m[name] = %v, want test", m["name"])
	}
	server, ok := m["server"]
	if !ok {
		t.Fatal("m[server] not found")
	}
	serverMap, ok := server.(map[string]any)
	if !ok {
		t.Fatalf("m[server] is %T, want map[string]any", server)
	}
	if serverMap["host"] != "localhost" {
		t.Errorf("m[server][host] = %v, want localhost", serverMap["host"])
	}
}

func TestUnmarshal_InlineTableToInterface(t *testing.T) {
	input := `val = {a = 1, b = "two"}`
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
	if m["a"] != int64(1) {
		t.Errorf("Val.a = %v (%T), want 1 (int64)", m["a"], m["a"])
	}
	if m["b"] != "two" {
		t.Errorf("Val.b = %v, want two", m["b"])
	}
}

func TestUnmarshal_NestedDottedKeysInTable(t *testing.T) {
	input := `
[server]
database.host = "db.example.com"
database.port = 5432
`
	type Database struct {
		Host string `toml:"host"`
		Port int    `toml:"port"`
	}
	type Server struct {
		Database Database `toml:"database"`
	}
	type Config struct {
		Server Server `toml:"server"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Server.Database.Host != "db.example.com" {
		t.Errorf("Server.Database.Host = %q, want db.example.com", cfg.Server.Database.Host)
	}
	if cfg.Server.Database.Port != 5432 {
		t.Errorf("Server.Database.Port = %d, want 5432", cfg.Server.Database.Port)
	}
}

func TestUnmarshal_ArrayTableWithMapTarget(t *testing.T) {
	input := `
[[items]]
name = "a"
val = 1

[[items]]
name = "b"
val = 2
`
	m := map[string]any{}
	if err := Unmarshal([]byte(input), &m); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	items, ok := m["items"]
	if !ok {
		t.Fatal("items not found in map")
	}
	arr, ok := items.([]any)
	if !ok {
		t.Fatalf("items is %T, want []any", items)
	}
	if len(arr) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(arr))
	}
	item0, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("items[0] is %T, want map[string]any", arr[0])
	}
	if item0["name"] != "a" {
		t.Errorf("items[0].name = %v, want a", item0["name"])
	}
}

func TestUnmarshal_MultiLevelKeyPath(t *testing.T) {
	input := `
[a.b]
c = "hello"
`
	type B struct {
		C string `toml:"c"`
	}
	type A struct {
		B B `toml:"b"`
	}
	type Config struct {
		A A `toml:"a"`
	}
	var cfg Config
	if err := Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.A.B.C != "hello" {
		t.Errorf("A.B.C = %q, want hello", cfg.A.B.C)
	}
}
