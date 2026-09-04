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

// Fails if a diagnostic about a KEY points at the key but ranges over
// something else. Pos and Span are two descriptions of one construct: a
// consumer that underlines Span while quoting Pos must underline what the
// message complains about, and an unknown-key diagnostic complains about the
// key, never about the value it happens to bind.
func TestDecode_KeyAddressedDiagnosticsSpanTheKey(t *testing.T) {
	type Config struct {
		Known int `toml:"known"`
	}
	src := "known = 0\nunknown = 1\n\n[tbl]\nx = 1\n\n[[arr]]\ny = 2\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var cfg Config
	diags := diagnosticsOf(t, doc.Decode(&cfg))

	want := map[string]struct {
		kind ErrorKind
		text string // the source the span must cover
	}{
		"unknown": {KindUnknownKey, "unknown"},
		"tbl":     {KindUnknownTable, "tbl"},
		"arr":     {KindUnknownTable, "arr"},
	}
	if len(diags) != len(want) {
		for _, d := range diags {
			t.Logf("diagnostic: %s at %q", d.Kind, d.Path)
		}
		t.Fatalf("got %d diagnostics, want %d", len(diags), len(want))
	}
	for _, d := range diags {
		w, ok := want[d.Path]
		if !ok {
			t.Errorf("unexpected diagnostic at %q: %v", d.Path, d)
			continue
		}
		if d.Kind != w.kind {
			t.Errorf("%s: kind = %s, want %s", d.Path, d.Kind, w.kind)
		}
		if !d.Span.IsValid() {
			t.Errorf("%s: carries no span", d.Path)
			continue
		}
		if got := src[d.Span.Start.Offset:d.Span.End.Offset]; got != w.text {
			t.Errorf("%s: span covers %q, want the key %q", d.Path, got, w.text)
		}
		if d.Pos != d.Span.Start {
			t.Errorf("%s: Pos %+v is not the start of Span %+v: they describe different constructs",
				d.Path, d.Pos, d.Span)
		}
	}
}

// --- eager tag validation ---
//
// The types below sit behind a key the test document never carries, so a
// derivation that only inspects the types a document reaches never looks at
// them at all.

// eagerBadOption carries a toml tag option the package reads nothing from.
type eagerBadOption struct {
	Bad string `toml:"bad,nonsense"`
}

// eagerTaggedUnexported carries a toml tag on a field no document key can ever
// reach, which makes the tag inert text rather than a defect.
type eagerTaggedUnexported struct {
	secret string `toml:"secret"`
}

// The field exists to be ignored, never to be read or written; the read is
// what tells a linter that.
var _ = eagerTaggedUnexported{}.secret

type eagerStructField struct {
	Top    int            `toml:"top"`
	Nested eagerBadOption `toml:"nested"`
}

type eagerSliceElement struct {
	Top   int              `toml:"top"`
	Items []eagerBadOption `toml:"items"`
}

type eagerMapElement struct {
	Top   int                       `toml:"top"`
	Items map[string]eagerBadOption `toml:"items"`
}

type eagerPointerField struct {
	Top    int             `toml:"top"`
	Nested *eagerBadOption `toml:"nested"`
}

type eagerUnexportedTagField struct {
	Top    int                   `toml:"top"`
	Nested eagerTaggedUnexported `toml:"nested"`
}

// eagerRecursiveBad is recursive through three constructors at once, so a walk
// that does not remember where it has been never returns.
type eagerRecursiveBad struct {
	Name  string                       `toml:"name"`
	Child *eagerRecursiveBad           `toml:"child"`
	Peers []eagerRecursiveBad          `toml:"peers"`
	Bag   map[string]eagerRecursiveBad `toml:"bag"`
	Bad   eagerBadOption               `toml:"bad"`
}

// eagerRecursiveClean is the same shape with no tag defect anywhere: it must
// decode, which is what proves the walk terminates rather than merely erroring
// out early.
type eagerRecursiveClean struct {
	Name  string                         `toml:"name"`
	Child *eagerRecursiveClean           `toml:"child"`
	Peers []eagerRecursiveClean          `toml:"peers"`
	Bag   map[string]eagerRecursiveClean `toml:"bag"`
}

// Fails if a toml tag defect stays invisible because the document never
// reaches the type carrying it. A tag rule is a property of the TARGET, not of
// the input: a meaningless tag option is refused for every document a target
// is used with, or it is refused only by the inputs that happen to descend far
// enough -- which is silence for every other input.
func TestDecode_TagErrorsAreEagerAcrossTheTypeGraph(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		target  func() any
		wantIn  string
		alsoSee string // a document that DOES reach the type, for contrast
	}{
		{
			name:    "a nested struct field",
			input:   "top = 1\n",
			target:  func() any { return new(eagerStructField) },
			wantIn:  "nonsense",
			alsoSee: "top = 1\n[nested]\nbad = \"x\"\n",
		},
		{
			name:    "a slice element",
			input:   "top = 1\n",
			target:  func() any { return new(eagerSliceElement) },
			wantIn:  "nonsense",
			alsoSee: "top = 1\n[[items]]\nbad = \"x\"\n",
		},
		{
			name:    "a map element",
			input:   "top = 1\n",
			target:  func() any { return new(eagerMapElement) },
			wantIn:  "nonsense",
			alsoSee: "top = 1\n[items.a]\nbad = \"x\"\n",
		},
		{
			name:    "a pointer field",
			input:   "top = 1\n",
			target:  func() any { return new(eagerPointerField) },
			wantIn:  "nonsense",
			alsoSee: "top = 1\n[nested]\nbad = \"x\"\n",
		},
		{
			name:   "a recursive type",
			input:  "name = \"x\"\n",
			target: func() any { return new(eagerRecursiveBad) },
			wantIn: "nonsense",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Unmarshal([]byte(tt.input), tt.target())
			if err == nil {
				t.Fatalf("Unmarshal(%q) accepted a target whose type graph carries a tag defect", tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("err = %v, want the offending name %q", err, tt.wantIn)
			}
			if tt.alsoSee == "" {
				return
			}
			// The same defect on a document that does reach the type: the two
			// inputs must agree, which is the whole point.
			reached := Unmarshal([]byte(tt.alsoSee), tt.target())
			if reached == nil || !strings.Contains(reached.Error(), tt.wantIn) {
				t.Errorf("a document reaching the type reported %v, want the same tag error", reached)
			}
		})
	}
}

// Fails if the eager tag walk starts refusing a toml tag on an unexported
// field again. The tag names a key nothing can bind, which is not a defect to
// report but text with no effect: the type graph carrying it constructs, and a
// document key spelling the tag is unknown like any other undeclared key.
func TestDecode_TagOnUnexportedFieldIsInertAcrossTheTypeGraph(t *testing.T) {
	var cfg eagerUnexportedTagField
	if err := Unmarshal([]byte("top = 1\n"), &cfg); err != nil {
		t.Fatalf("Unmarshal refused a type graph carrying a tag on an unexported field: %v", err)
	}
	if cfg.Top != 1 {
		t.Errorf("Top = %d, want 1", cfg.Top)
	}

	err := Unmarshal([]byte("top = 1\n[nested]\nsecret = \"x\"\n"), &cfg)
	if err == nil {
		t.Fatal("Unmarshal bound a document key to an unexported field through its tag")
	}
	if !errors.Is(err, ErrUnknownKey) {
		t.Errorf("err = %v, want an unknown-key diagnostic", err)
	}
	if cfg.Nested.secret != "" {
		t.Errorf("secret = %q, want the tag to bind nothing", cfg.Nested.secret)
	}
}

// Fails if the eager tag walk stops terminating on a type that refers to
// itself: the clean recursive shape must decode like any other.
func TestDecode_EagerTagWalkTerminatesOnRecursiveTypes(t *testing.T) {
	var cfg eagerRecursiveClean
	if err := Unmarshal([]byte("name = \"root\"\n\n[child]\nname = \"kid\"\n"), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if cfg.Name != "root" || cfg.Child == nil || cfg.Child.Name != "kid" {
		t.Errorf("decoded %+v, want the recursive shape written through", cfg)
	}
}

// Fails if making TAG validation eager also makes type-SHAPE validation eager.
// A field whose type no conversion accepts is refused when a document key
// reaches it and at no other time, so a struct carrying one still decodes the
// documents that leave it alone.
func TestDecode_UndecodableFieldTypeStaysLazy(t *testing.T) {
	type Config struct {
		Top      int `toml:"top"`
		Callback func()
		Ch       chan int
	}
	var cfg Config
	if err := Unmarshal([]byte("top = 1\n"), &cfg); err != nil {
		t.Fatalf("Unmarshal failed on an untouched undecodable field: %v", err)
	}
	if cfg.Top != 1 {
		t.Errorf("Top = %d, want 1", cfg.Top)
	}
}
