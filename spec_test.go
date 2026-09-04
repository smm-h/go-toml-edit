package tomledit

import (
	"errors"
	"strings"
	"testing"
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
