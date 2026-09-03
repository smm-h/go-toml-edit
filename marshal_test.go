package tomledit

import (
	"strings"
	"testing"
)

func TestMarshalEmptyMap(t *testing.T) {
	b, err := Marshal(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(b) != 0 {
		t.Fatalf("expected empty output, got %q", string(b))
	}
}

func TestMarshalFlatPrimitives(t *testing.T) {
	input := map[string]any{
		"name":    "test",
		"enabled": true,
		"count":   int64(42),
		"ratio":   float64(3.14),
		"num":     7,
	}
	b, err := Marshal(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Round-trip: unmarshal back and compare.
	var got map[string]any
	if err := Unmarshal(b, &got); err != nil {
		t.Fatalf("round-trip unmarshal failed: %v\nTOML:\n%s", err, string(b))
	}

	// Unmarshal decodes integers as int64.
	checks := map[string]any{
		"name":    "test",
		"enabled": true,
		"count":   int64(42),
		"ratio":   float64(3.14),
		"num":     int64(7),
	}
	for k, want := range checks {
		gotVal, ok := got[k]
		if !ok {
			t.Errorf("missing key %q after round-trip", k)
			continue
		}
		if gotVal != want {
			t.Errorf("key %q: got %v (%T), want %v (%T)", k, gotVal, gotVal, want, want)
		}
	}
}

func TestMarshalFloatFormatting(t *testing.T) {
	input := map[string]any{
		"val": float64(3.0),
	}
	b, err := Marshal(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "3.0") {
		t.Fatalf("float64(3.0) should render as 3.0, got:\n%s", s)
	}
	// Must not render as just "3" (no decimal point).
	if strings.Contains(s, "= 3\n") {
		t.Fatalf("float64(3.0) must not render without decimal point, got:\n%s", s)
	}
}

func TestMarshalNestedMap(t *testing.T) {
	input := map[string]any{
		"section": map[string]any{
			"key": "value",
		},
	}
	b, err := Marshal(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "[section]") {
		t.Fatalf("nested map should produce [section] header, got:\n%s", s)
	}
	if !strings.Contains(s, "key = \"value\"") {
		t.Fatalf("nested map should contain key = \"value\", got:\n%s", s)
	}
}

func TestMarshalSortedKeys(t *testing.T) {
	input := map[string]any{
		"zebra":  "z",
		"alpha":  "a",
		"middle": "m",
	}
	b, err := Marshal(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(b)

	alphaIdx := strings.Index(s, "alpha")
	middleIdx := strings.Index(s, "middle")
	zebraIdx := strings.Index(s, "zebra")

	if alphaIdx >= middleIdx || middleIdx >= zebraIdx {
		t.Fatalf("keys should be sorted alphabetically, got:\n%s", s)
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	input := map[string]any{
		"title": "example",
		"database": map[string]any{
			"server":  "192.168.1.1",
			"ports":   []any{int64(8001), int64(8001), int64(8002)},
			"enabled": true,
		},
	}
	b, err := Marshal(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]any
	if err := Unmarshal(b, &got); err != nil {
		t.Fatalf("round-trip unmarshal failed: %v\nTOML:\n%s", err, string(b))
	}

	// Check top-level.
	if got["title"] != "example" {
		t.Errorf("title: got %v, want %q", got["title"], "example")
	}
	db, ok := got["database"].(map[string]any)
	if !ok {
		t.Fatalf("database should be map[string]any, got %T", got["database"])
	}
	if db["server"] != "192.168.1.1" {
		t.Errorf("database.server: got %v, want %q", db["server"], "192.168.1.1")
	}
	if db["enabled"] != true {
		t.Errorf("database.enabled: got %v, want true", db["enabled"])
	}
	ports, ok := db["ports"].([]any)
	if !ok {
		t.Fatalf("database.ports should be []any, got %T", db["ports"])
	}
	if len(ports) != 3 {
		t.Fatalf("database.ports: got %d elements, want 3", len(ports))
	}
}

func TestMarshalErrorNonMapRoot(t *testing.T) {
	_, err := Marshal("string")
	if err == nil {
		t.Fatal("expected error for string root")
	}
	if !strings.Contains(err.Error(), "Marshal requires a map type") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestMarshalErrorNil(t *testing.T) {
	_, err := Marshal(nil)
	if err == nil {
		t.Fatal("expected error for nil root")
	}
	if !strings.Contains(err.Error(), "Marshal requires a map type") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestMarshalErrorStructRoot(t *testing.T) {
	type Foo struct{ X int }
	_, err := Marshal(Foo{X: 1})
	if err == nil {
		t.Fatal("expected error for struct root")
	}
	if !strings.Contains(err.Error(), "Marshal requires a map type") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestMarshalMixedFlatAndNested(t *testing.T) {
	input := map[string]any{
		"title": "example",
		"owner": map[string]any{
			"name": "Tom",
		},
		"debug": true,
	}
	b, err := Marshal(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(b)

	// Flat keys (debug, title) should come before sections ([owner]).
	titleIdx := strings.Index(s, "title")
	debugIdx := strings.Index(s, "debug")
	ownerIdx := strings.Index(s, "[owner]")

	if titleIdx == -1 || debugIdx == -1 || ownerIdx == -1 {
		t.Fatalf("missing expected content in output:\n%s", s)
	}
	if titleIdx >= ownerIdx {
		t.Fatalf("flat key 'title' should come before [owner] section, got:\n%s", s)
	}
	if debugIdx >= ownerIdx {
		t.Fatalf("flat key 'debug' should come before [owner] section, got:\n%s", s)
	}
}

func TestMarshalArrayValue(t *testing.T) {
	input := map[string]any{
		"tags": []any{"a", "b", "c"},
	}
	b, err := Marshal(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]any
	if err := Unmarshal(b, &got); err != nil {
		t.Fatalf("round-trip failed: %v\nTOML:\n%s", err, string(b))
	}
	tags, ok := got["tags"].([]any)
	if !ok {
		t.Fatalf("tags should be []any, got %T", got["tags"])
	}
	if len(tags) != 3 {
		t.Fatalf("tags: got %d elements, want 3", len(tags))
	}
	if tags[0] != "a" || tags[1] != "b" || tags[2] != "c" {
		t.Errorf("tags: got %v, want [a b c]", tags)
	}
}

func TestMarshalQuotedKeys(t *testing.T) {
	input := map[string]any{
		"simple":         "bare",
		"key with space": "quoted",
	}
	b, err := Marshal(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(b)
	// "key with space" must be quoted in the output.
	if !strings.Contains(s, "\"key with space\"") {
		t.Fatalf("key with spaces should be quoted, got:\n%s", s)
	}
	// "simple" should be bare (unquoted).
	if strings.Contains(s, "\"simple\"") {
		t.Fatalf("simple key should be bare (unquoted), got:\n%s", s)
	}
}

func TestMarshalDeeplyNestedMap(t *testing.T) {
	// Deeper nesting uses inline tables as fallback.
	input := map[string]any{
		"outer": map[string]any{
			"inner": map[string]any{
				"deep": "value",
			},
		},
	}
	b, err := Marshal(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should round-trip correctly regardless of representation.
	var got map[string]any
	if err := Unmarshal(b, &got); err != nil {
		t.Fatalf("round-trip failed: %v\nTOML:\n%s", err, string(b))
	}
	outer, ok := got["outer"].(map[string]any)
	if !ok {
		t.Fatalf("outer should be map, got %T", got["outer"])
	}
	inner, ok := outer["inner"].(map[string]any)
	if !ok {
		t.Fatalf("inner should be map, got %T", outer["inner"])
	}
	if inner["deep"] != "value" {
		t.Errorf("outer.inner.deep: got %v, want %q", inner["deep"], "value")
	}
}

func TestMarshalInlineTableDeterministicKeyOrder(t *testing.T) {
	// Maps at depth > 2 become inline tables via mapToInlineTableNode.
	// Keys must be sorted for deterministic output.
	input := map[string]any{
		"section": map[string]any{
			"nested": map[string]any{
				"zebra":  1,
				"alpha":  2,
				"mango":  3,
				"banana": 4,
				"cherry": 5,
			},
		},
	}

	// Marshal once to get the reference output.
	ref, err := Marshal(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Marshal many times and verify identical output each time.
	for i := 0; i < 100; i++ {
		b, err := Marshal(input)
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if string(b) != string(ref) {
			t.Fatalf("non-deterministic output on iteration %d:\nreference:\n%s\ngot:\n%s", i, string(ref), string(b))
		}
	}

	// Verify the inline table keys appear in sorted order.
	out := string(ref)
	alphaIdx := strings.Index(out, "alpha")
	bananaIdx := strings.Index(out, "banana")
	cherryIdx := strings.Index(out, "cherry")
	mangoIdx := strings.Index(out, "mango")
	zebraIdx := strings.Index(out, "zebra")

	if alphaIdx < 0 || bananaIdx < 0 || cherryIdx < 0 || mangoIdx < 0 || zebraIdx < 0 {
		t.Fatalf("missing keys in output:\n%s", out)
	}
	if !(alphaIdx < bananaIdx && bananaIdx < cherryIdx && cherryIdx < mangoIdx && mangoIdx < zebraIdx) {
		t.Errorf("inline table keys are not in sorted order:\n%s", out)
	}
}
