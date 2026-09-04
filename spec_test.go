package tomledit

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The descriptor surface: a document shape described as data, checked by the
// same engine the reflection front end runs.

// specTestDoc is a document a hand-built descriptor describes exactly.
const specTestDoc = `
title = "example"
port = 8080

[server]
host = "localhost"
tags = ["a", "b"]

[labels]
env = "prod"
team = "backend"

[[items]]
name = "first"

[[items]]
name = "second"
`

// specTestSpec describes specTestDoc without a single line of reflection: a
// nested table, an array of scalars, a table of arbitrary keys, and an
// array-of-tables.
func specTestSpec() *Spec {
	return &Spec{Fields: map[string]Field{
		"title": {Kind: FieldKindString, Required: true},
		"port":  {Kind: FieldKindInteger},
		"server": {Kind: FieldKindTable, Table: &Spec{Fields: map[string]Field{
			"host": {Kind: FieldKindString, Required: true},
			"tags": {Kind: FieldKindArray, Elem: &Field{Kind: FieldKindString}},
		}}},
		"labels": {Kind: FieldKindTable, Table: &Spec{Dynamic: &Field{Kind: FieldKindString}}},
		"items": {Kind: FieldKindArray, Elem: &Field{Kind: FieldKindTable, Table: &Spec{Fields: map[string]Field{
			"name": {Kind: FieldKindString},
		}}}},
	}}
}

// Fails if a descriptor built by hand -- no reflection anywhere -- stops being
// able to validate a document. This is the surface a consumer with a
// runtime-known schema uses.
func TestValidate_HandBuiltDescriptor(t *testing.T) {
	doc, err := Parse([]byte(specTestDoc))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := doc.Validate(specTestSpec()); err != nil {
		t.Fatalf("Validate refused a document the descriptor describes: %v", err)
	}
}

// Fails if a key no field names, and no Dynamic covers, stops being refused --
// or if a table WITH a Dynamic starts refusing keys it covers.
func TestValidate_DynamicIsTheOnlyWayToExtraKeys(t *testing.T) {
	doc, err := Parse([]byte("[labels]\nanything = \"goes\"\n[strict]\nnope = 1\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	spec := &Spec{Fields: map[string]Field{
		"labels": {Kind: FieldKindTable, Table: &Spec{Dynamic: &Field{Kind: FieldKindString}}},
		"strict": {Kind: FieldKindTable, Table: &Spec{}},
	}}
	err = doc.Validate(spec)
	if err == nil {
		t.Fatal("Validate accepted a key no field names in a table with no Dynamic")
	}
	diags := diagnosticsOf(t, err)
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want only the one about strict.nope: %v", len(diags), err)
	}
	if diags[0].Kind != KindUnknownKey || diags[0].Path != "strict.nope" {
		t.Errorf("diagnostic = %s at %q, want an unknown key at strict.nope", diags[0].Kind, diags[0].Path)
	}
}

// Fails if FieldAny stops being total: it is the explicit spelling of "I am
// not describing this", so nothing inside it is ever a violation.
func TestValidate_FieldAnyDescribesNothing(t *testing.T) {
	doc, err := Parse([]byte("[opaque]\nwhatever = [1, {a = 2}]\nmore = 1979-05-27\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	spec := &Spec{Fields: map[string]Field{"opaque": FieldAny()}}
	if err := doc.Validate(spec); err != nil {
		t.Fatalf("Validate refused something FieldAny covers: %v", err)
	}
}

// Fails if missing required keys stop being reported in lexicographic order.
// A map has no order of its own, so the report needs one that does not depend
// on iteration.
func TestValidate_MissingKeysAreLexicographic(t *testing.T) {
	doc, err := Parse([]byte("present = 1\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	spec := &Spec{Fields: map[string]Field{
		"present": {Kind: FieldKindInteger},
		"zebra":   {Kind: FieldKindInteger, Required: true},
		"alpha":   {Kind: FieldKindInteger, Required: true},
		"middle":  {Kind: FieldKindInteger, Required: true},
	}}
	err = doc.Validate(spec)
	if err == nil {
		t.Fatal("Validate accepted a document missing three required keys")
	}
	var paths []string
	for _, d := range diagnosticsOf(t, err) {
		if d.Kind != KindMissingKey {
			t.Errorf("diagnostic %s at %q, want a missing key", d.Kind, d.Path)
		}
		paths = append(paths, d.Path)
	}
	want := []string{"alpha", "middle", "zebra"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Errorf("missing keys reported as %v, want %v", paths, want)
	}
}

// Fails if a descriptor that describes nothing is accepted: an array with no
// element descriptor, a table with no field set, or a sub-descriptor on a kind
// that has no use for one. Each is refused before the document is read, and
// the error names the field it is about.
func TestValidate_DescriptorConstructionRefusals(t *testing.T) {
	doc, err := Parse([]byte("x = 1\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	tests := []struct {
		name  string
		spec  *Spec
		names []string // fragments the error must carry
	}{
		{
			name:  "nil descriptor",
			spec:  nil,
			names: []string{"nil"},
		},
		{
			name:  "array without an element descriptor",
			spec:  &Spec{Fields: map[string]Field{"ports": {Kind: FieldKindArray}}},
			names: []string{"ports", "Elem"},
		},
		{
			name:  "table without a field set",
			spec:  &Spec{Fields: map[string]Field{"server": {Kind: FieldKindTable}}},
			names: []string{"server", "Table"},
		},
		{
			name:  "element descriptor on a scalar",
			spec:  &Spec{Fields: map[string]Field{"name": {Kind: FieldKindString, Elem: &Field{Kind: FieldKindString}}}},
			names: []string{"name", "Elem"},
		},
		{
			name:  "field set on an array",
			spec:  &Spec{Fields: map[string]Field{"items": {Kind: FieldKindArray, Elem: &Field{Kind: FieldKindString}, Table: &Spec{}}}},
			names: []string{"items", "Table"},
		},
		{
			name:  "nested refusal",
			spec:  &Spec{Fields: map[string]Field{"server": {Kind: FieldKindTable, Table: &Spec{Fields: map[string]Field{"ports": {Kind: FieldKindArray}}}}}},
			names: []string{"ports", "Elem"},
		},
		{
			name:  "unknown kind",
			spec:  &Spec{Fields: map[string]Field{"weird": {Kind: FieldKind(99)}}},
			names: []string{"weird", "unknown field kind"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := doc.Validate(tc.spec)
			if err == nil {
				t.Fatal("Validate accepted a descriptor that describes nothing")
			}
			for _, fragment := range tc.names {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("error %q does not name %q", err, fragment)
				}
			}
		})
	}
}

// Fails if a recursive descriptor stops terminating: a Spec that reaches
// itself describes a document of unbounded depth, which is legal.
func TestValidate_RecursiveDescriptor(t *testing.T) {
	node := &Spec{Fields: map[string]Field{"name": {Kind: FieldKindString}}}
	child := Field{Kind: FieldKindTable, Table: node}
	node.Fields["child"] = child

	doc, err := Parse([]byte("name = \"a\"\n[child]\nname = \"b\"\n[child.child]\nname = \"c\"\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := doc.Validate(node); err != nil {
		t.Fatalf("Validate refused a document a recursive descriptor describes: %v", err)
	}
}

// Fails if Validate starts writing anything: it answers about the document and
// holds no target at all, which is the whole reason it exists beside Decode.
func TestValidate_ReportsWithoutDecoding(t *testing.T) {
	doc, err := Parse([]byte("port = 9000\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	spec := &Spec{Fields: map[string]Field{"port": {Kind: FieldKindInteger}}}
	if err := doc.Validate(spec); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, err := doc.GetInt("port"); err != nil || got != 9000 {
		t.Errorf("port = %d (%v) after Validate, want the document untouched", got, err)
	}
}

// Fails if the descriptor's date-time kinds are collapsed into one: the four
// flavors are four members, and each accepts only its own.
func TestValidate_DateTimeKindsAreFour(t *testing.T) {
	doc, err := Parse([]byte("odt = 1979-05-27T07:32:00Z\nldt = 1979-05-27T07:32:00\nld = 1979-05-27\nlt = 07:32:00\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	exact := &Spec{Fields: map[string]Field{
		"odt": {Kind: FieldKindOffsetDateTime},
		"ldt": {Kind: FieldKindLocalDateTime},
		"ld":  {Kind: FieldKindLocalDate},
		"lt":  {Kind: FieldKindLocalTime},
	}}
	if err := doc.Validate(exact); err != nil {
		t.Fatalf("Validate refused each date-time in its own kind: %v", err)
	}

	crossed := &Spec{Fields: map[string]Field{
		"odt": {Kind: FieldKindLocalDateTime},
		"ldt": {Kind: FieldKindOffsetDateTime},
		"ld":  {Kind: FieldKindLocalTime},
		"lt":  {Kind: FieldKindLocalDate},
	}}
	err = doc.Validate(crossed)
	if err == nil {
		t.Fatal("Validate accepted every date-time in the wrong kind")
	}
	diags := diagnosticsOf(t, err)
	if len(diags) != 4 {
		t.Fatalf("got %d diagnostics, want one per crossed kind: %v", len(diags), err)
	}
	for _, d := range diags {
		if !errors.Is(d, ErrTypeMismatch) {
			t.Errorf("diagnostic at %q is %s, want a type mismatch", d.Path, d.Kind)
		}
	}
}

// --- the descriptor path's decode ---

// decodeSpecDoc carries every shape DecodeSpec has to build: scalars of each
// flavor, a header table, a table implied by a longer header, an inline table,
// an array, and an array-of-tables.
const decodeSpecDoc = `title = "app"
count = 7
ratio = 1.5
on = true
when = 1979-05-27T07:32:00Z
ports = [1, 2]

[server]
host = "localhost"
opts = { debug = true }

[deep.nested]
x = 1

[[items]]
name = "a"

[[items]]
name = "b"
`

// decodeSpecSpec describes decodeSpecDoc exactly.
func decodeSpecSpec() *Spec {
	return &Spec{Fields: map[string]Field{
		"title": {Kind: FieldKindString},
		"count": {Kind: FieldKindInteger},
		"ratio": {Kind: FieldKindFloat},
		"on":    {Kind: FieldKindBoolean},
		"when":  {Kind: FieldKindOffsetDateTime},
		"ports": {Kind: FieldKindArray, Elem: &Field{Kind: FieldKindInteger}},
		"server": {Kind: FieldKindTable, Table: &Spec{Fields: map[string]Field{
			"host": {Kind: FieldKindString},
			"opts": {Kind: FieldKindTable, Table: &Spec{Fields: map[string]Field{
				"debug": {Kind: FieldKindBoolean},
			}}},
		}}},
		"deep": {Kind: FieldKindTable, Table: &Spec{Fields: map[string]Field{
			"nested": {Kind: FieldKindTable, Table: &Spec{Fields: map[string]Field{
				"x": {Kind: FieldKindInteger},
			}}},
		}}},
		"items": {Kind: FieldKindArray, Elem: &Field{Kind: FieldKindTable, Table: &Spec{Fields: map[string]Field{
			"name": {Kind: FieldKindString},
		}}}},
	}}
}

// Fails if DecodeSpec stops answering with the document's own values, in the
// native forms the conversion table's last row names. Every table spelling
// arrives as a map, both array spellings as a slice, and a scalar as the Go
// type of its TOML type.
func TestDecodeSpec_BuildsTheDocumentsValues(t *testing.T) {
	doc, err := Parse([]byte(decodeSpecDoc))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got, err := doc.DecodeSpec(decodeSpecSpec())
	if err != nil {
		t.Fatalf("DecodeSpec: %v", err)
	}

	if got["title"] != "app" || got["count"] != int64(7) || got["ratio"] != 1.5 || got["on"] != true {
		t.Errorf("scalars decoded as %#v", got)
	}
	if when, ok := got["when"].(time.Time); !ok || !when.Equal(time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC)) {
		t.Errorf("when = %#v, want the instant the document carries", got["when"])
	}
	if !reflect.DeepEqual(got["ports"], []any{int64(1), int64(2)}) {
		t.Errorf("ports = %#v, want []any{1, 2}", got["ports"])
	}

	server, ok := got["server"].(map[string]any)
	if !ok {
		t.Fatalf("server is %T, want map[string]any", got["server"])
	}
	if server["host"] != "localhost" {
		t.Errorf("server.host = %v, want localhost", server["host"])
	}
	if opts, ok := server["opts"].(map[string]any); !ok || opts["debug"] != true {
		t.Errorf("server.opts = %#v, want the inline table as a map", server["opts"])
	}
	// A table implied by a longer header is a table like any other.
	deep, ok := got["deep"].(map[string]any)
	if !ok {
		t.Fatalf("deep is %T, want map[string]any", got["deep"])
	}
	if nested, ok := deep["nested"].(map[string]any); !ok || nested["x"] != int64(1) {
		t.Errorf("deep.nested = %#v, want the implied table", deep["nested"])
	}

	items, ok := got["items"].([]any)
	if !ok {
		t.Fatalf("items is %T, want []any", got["items"])
	}
	if len(items) != 2 {
		t.Fatalf("items has %d entries, want 2", len(items))
	}
	for i, want := range []string{"a", "b"} {
		entry, ok := items[i].(map[string]any)
		if !ok || entry["name"] != want {
			t.Errorf("items[%d] = %#v, want a map naming %q", i, items[i], want)
		}
	}
}

// Fails if DecodeSpec ever returns a partial map. Its atomicity has no
// exception clause: the document below violates the descriptor in the middle,
// with clean keys before and after, and none of them may come back.
func TestDecodeSpec_IsAtomic(t *testing.T) {
	doc, err := Parse([]byte(`first = "clean"
bad = "not an integer"
last = "clean too"

[unknown_table]
x = 1
`))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	spec := &Spec{Fields: map[string]Field{
		"first": {Kind: FieldKindString},
		"bad":   {Kind: FieldKindInteger},
		"last":  {Kind: FieldKindString},
	}}

	got, err := doc.DecodeSpec(spec)
	if err == nil {
		t.Fatalf("DecodeSpec accepted a document that violates the descriptor: %#v", got)
	}
	if got != nil {
		t.Errorf("a failed DecodeSpec returned %#v, want no map at all", got)
	}
	// The violations are the whole answer, and they are all of them.
	diags := diagnosticsOf(t, err)
	if len(diags) != 2 {
		t.Fatalf("got %d diagnostics, want the type mismatch and the unknown table: %v", len(diags), err)
	}
	if diags[0].Kind != KindInexact && diags[0].Kind != KindTypeMismatch {
		t.Errorf("first diagnostic is %s at %q, want the refused value", diags[0].Kind, diags[0].Path)
	}
	if diags[1].Kind != KindUnknownTable {
		t.Errorf("second diagnostic is %s at %q, want the unknown table", diags[1].Kind, diags[1].Path)
	}
}

// Fails if a descriptor that describes nothing reaches the document: the
// construction error is the same one Validate reports, and it comes back with
// no map.
func TestDecodeSpec_RefusesADescriptorThatDescribesNothing(t *testing.T) {
	doc, err := Parse([]byte("vals = [1]\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got, err := doc.DecodeSpec(&Spec{Fields: map[string]Field{
		"vals": {Kind: FieldKindArray}, // no Elem
	}})
	if err == nil {
		t.Fatal("DecodeSpec accepted an array field with no element descriptor")
	}
	if got != nil {
		t.Errorf("a refused DecodeSpec returned %#v, want no map at all", got)
	}
	if !strings.Contains(err.Error(), "Elem") {
		t.Errorf("err = %v, want the missing sub-descriptor named", err)
	}
}

// Fails if DecodeSpec and Validate stop agreeing about what a document is: they
// run one engine, so the descriptor path reports the same diagnostics whether
// or not the caller wants the values.
func TestDecodeSpec_ReportsWhatValidateReports(t *testing.T) {
	doc, err := Parse([]byte("port = \"eighty\"\nextra = 1\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	spec := &Spec{Fields: map[string]Field{"port": {Kind: FieldKindInteger}}}

	_, decodeErr := doc.DecodeSpec(spec)
	validateErr := doc.Validate(spec)
	from := diagnosticsOf(t, decodeErr)
	against := diagnosticsOf(t, validateErr)
	if len(from) != len(against) {
		t.Fatalf("DecodeSpec reported %d diagnostics, Validate %d", len(from), len(against))
	}
	for i := range from {
		if from[i].Kind != against[i].Kind || from[i].Path != against[i].Path || from[i].Pos != against[i].Pos {
			t.Errorf("diagnostic %d: DecodeSpec says %s at %q %v, Validate says %s at %q %v",
				i, from[i].Kind, from[i].Path, from[i].Pos, against[i].Kind, against[i].Path, against[i].Pos)
		}
	}
}

// Fails if DecodeSpec starts writing into the document it reads: it builds a
// value of its own out of the read-layer, and the document is untouched.
func TestDecodeSpec_LeavesTheDocumentAlone(t *testing.T) {
	const src = "port = 9000\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got, err := doc.DecodeSpec(&Spec{Fields: map[string]Field{"port": {Kind: FieldKindInteger}}})
	if err != nil {
		t.Fatalf("DecodeSpec: %v", err)
	}
	got["port"] = "mutated"
	if string(doc.Bytes()) != src {
		t.Errorf("document renders %q after DecodeSpec, want %q", doc.Bytes(), src)
	}
	again, err := doc.DecodeSpec(&Spec{Fields: map[string]Field{"port": {Kind: FieldKindInteger}}})
	if err != nil {
		t.Fatalf("DecodeSpec: %v", err)
	}
	if again["port"] != int64(9000) {
		t.Errorf("port = %#v on a second read, want the returned map to be the caller's own", again["port"])
	}
}
