package tomledit

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The engine suite: what the decode contract reports, where it reports it, and
// that both front ends report it alike.

// diagnosticsOf returns the diagnostics an error carries, in order.
func diagnosticsOf(t *testing.T, err error) []*Error {
	t.Helper()
	var all *Errors
	if errors.As(err, &all) {
		out := make([]*Error, 0, len(all.Unwrap()))
		for _, e := range all.Unwrap() {
			var d *Error
			if !errors.As(e, &d) {
				t.Fatalf("aggregate carries %v (%T), want a *Error", e, e)
			}
			out = append(out, d)
		}
		return out
	}
	var d *Error
	if errors.As(err, &d) {
		return []*Error{d}
	}
	t.Fatalf("err = %v (%T), want diagnostics", err, err)
	return nil
}

// engineTestDoc carries one of every violation the decode contract can report,
// plus keys buried under a construct that is itself refused.
const engineTestDoc = `name = "app"
port = "8080"
extra = true

[server]
host = "h"
bogus = 1

[unknown_table]
a = 1
b = 2

[[items]]
label = 1
`

type engineItem struct {
	Label string `toml:"label"`
}

type engineServer struct {
	Host string `toml:"host"`
}

type engineConfig struct {
	Name    string       `toml:"name"`
	Port    int64        `toml:"port"`
	Timeout int64        `toml:"timeout,required"`
	Server  engineServer `toml:"server"`
	Items   []engineItem `toml:"items"`
}

// engineTestSpec describes engineConfig without reflection, in the terms a
// descriptor can express: presence, kind and required-ness.
func engineTestSpec() *Spec {
	return &Spec{Fields: map[string]Field{
		"name":    {Kind: FieldKindString},
		"port":    {Kind: FieldKindInteger},
		"timeout": {Kind: FieldKindInteger, Required: true},
		"server": {Kind: FieldKindTable, Table: &Spec{Fields: map[string]Field{
			"host": {Kind: FieldKindString},
		}}},
		"items": {Kind: FieldKindArray, Elem: &Field{Kind: FieldKindTable, Table: &Spec{Fields: map[string]Field{
			"label": {Kind: FieldKindString},
		}}}},
	}}
}

// Fails if a violation stops being reported, stops carrying the kind the
// contract gives it, or stops pointing at the construct it is about.
func TestEngine_ViolationKindsAndPositions(t *testing.T) {
	doc, err := Parse([]byte(engineTestDoc))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var cfg engineConfig
	err = doc.Decode(&cfg)
	if err == nil {
		t.Fatal("Decode accepted a document with five violations")
	}

	want := []struct {
		kind ErrorKind
		path string
		line int
		col  int
	}{
		{KindTypeMismatch, "port", 2, 1},
		{KindUnknownKey, "extra", 3, 1},
		{KindUnknownKey, "server.bogus", 7, 1},
		{KindUnknownTable, "unknown_table", 9, 2},
		{KindTypeMismatch, "items[0].label", 14, 1},
		{KindMissingKey, "timeout", 1, 1},
	}
	diags := diagnosticsOf(t, err)
	if len(diags) != len(want) {
		for _, d := range diags {
			t.Logf("diagnostic: %s at %q (line %d, column %d)", d.Kind, d.Path, d.Pos.Line, d.Pos.Column)
		}
		t.Fatalf("got %d diagnostics, want %d", len(diags), len(want))
	}
	for i, w := range want {
		d := diags[i]
		if d.Kind != w.kind || d.Path != w.path {
			t.Errorf("diagnostic %d = %s at %q, want %s at %q", i, d.Kind, d.Path, w.kind, w.path)
		}
		if d.Pos.Line != w.line || d.Pos.Column != w.col {
			t.Errorf("diagnostic %d (%s) at line %d, column %d, want line %d, column %d",
				i, d.Path, d.Pos.Line, d.Pos.Column, w.line, w.col)
		}
		if !d.Span.IsValid() && d.Kind != KindMissingKey {
			t.Errorf("diagnostic %d (%s) carries no span", i, d.Path)
		}
	}

	// Everything the document got right still decoded.
	if cfg.Name != "app" || cfg.Server.Host != "h" {
		t.Errorf("decoded %+v, want the valid keys written through", cfg)
	}
}

// Fails if the engine starts descending below a construct it refused: the keys
// inside an unknown table are not independent violations, they are that
// table's contents, and reporting them buries the one error that matters.
func TestEngine_NoDescentBelowARefusedConstruct(t *testing.T) {
	doc, err := Parse([]byte(engineTestDoc))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var cfg engineConfig
	for _, d := range diagnosticsOf(t, doc.Decode(&cfg)) {
		if strings.HasPrefix(d.Path, "unknown_table.") {
			t.Errorf("diagnostic %q comes from inside a refused table", d.Path)
		}
	}
}

// Fails if an unknown table stops carrying the inventory that makes it
// actionable: the direct child keys of the offending construct, and only the
// direct ones.
func TestEngine_UnknownTableCarriesItsKeys(t *testing.T) {
	doc, err := Parse([]byte("[nope]\na = 1\nb = 2\n[nope.deeper]\nc = 3\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = doc.Validate(&Spec{})
	diags := diagnosticsOf(t, err)
	if len(diags) != 1 || diags[0].Kind != KindUnknownTable {
		t.Fatalf("got %v, want one unknown-table diagnostic", err)
	}
	if !reflect.DeepEqual(diags[0].Keys, []string{"a", "b", "deeper"}) {
		t.Errorf("Keys = %v, want the direct child keys [a b deeper]", diags[0].Keys)
	}
}

// Fails if an unknown array-of-tables stops reporting as one table with the
// keys of its FIRST entry, and starts reporting once per entry or per key.
func TestEngine_UnknownArrayOfTablesCarriesFirstEntryKeys(t *testing.T) {
	doc, err := Parse([]byte("[[nope]]\na = 1\n[[nope]]\nb = 2\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	diags := diagnosticsOf(t, doc.Validate(&Spec{}))
	if len(diags) != 1 || diags[0].Kind != KindUnknownTable {
		t.Fatalf("got %d diagnostics, want one unknown-table diagnostic", len(diags))
	}
	if !reflect.DeepEqual(diags[0].Keys, []string{"a"}) {
		t.Errorf("Keys = %v, want the first entry's keys [a]", diags[0].Keys)
	}
}

// Fails if the two front ends stop agreeing. A descriptor cannot express a Go
// target's width, so the comparison is scoped to what both spell: presence,
// kind and required-ness -- the kinds, the paths, and their order.
func TestEngine_FrontEndsReportIdentically(t *testing.T) {
	doc, err := Parse([]byte(engineTestDoc))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var cfg engineConfig
	viaStruct := diagnosticsOf(t, doc.Decode(&cfg))
	viaSpec := diagnosticsOf(t, doc.Validate(engineTestSpec()))

	if len(viaStruct) != len(viaSpec) {
		t.Fatalf("the struct front end reported %d diagnostics, the descriptor %d", len(viaStruct), len(viaSpec))
	}
	for i := range viaStruct {
		a, b := viaStruct[i], viaSpec[i]
		if a.Kind != b.Kind || a.Path != b.Path {
			t.Errorf("diagnostic %d: struct says %s at %q, descriptor says %s at %q", i, a.Kind, a.Path, b.Kind, b.Path)
		}
		if a.Pos != b.Pos {
			t.Errorf("diagnostic %d (%s): struct points at %v, descriptor at %v", i, a.Path, a.Pos, b.Pos)
		}
	}
}

// Fails if the aggregate stops reading as its first diagnostic, or stops
// exposing the rest: a call site that prints the error sees one failure, a
// call site that inspects it sees them all.
func TestEngine_AggregateRendersItsFirstDiagnostic(t *testing.T) {
	doc, err := Parse([]byte(engineTestDoc))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var cfg engineConfig
	err = doc.Decode(&cfg)
	diags := diagnosticsOf(t, err)
	if err.Error() != diags[0].Error() {
		t.Errorf("aggregate renders %q, want its first diagnostic %q", err, diags[0])
	}
	if !errors.Is(err, ErrUnknownKey) || !errors.Is(err, ErrMissingKey) {
		t.Error("the aggregate does not match the kind sentinels of the diagnostics it carries")
	}
	var first *Error
	if !errors.As(err, &first) || first != diags[0] {
		t.Error("errors.As with a single-diagnostic target did not yield the first diagnostic")
	}
}

// Fails if a custom decoder on a table that no single node stands for -- one
// implied by a longer header or a dotted key -- is silently skipped instead of
// refused. A hook that cannot run is not a hook that did nothing.
func TestEngine_CustomDecoderNeedsANodeToDecode(t *testing.T) {
	type Config struct {
		Implied dualUnmarshaler `toml:"implied"`
	}
	doc, err := Parse([]byte("implied.inner = 1\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var cfg Config
	err = doc.Decode(&cfg)
	if err == nil {
		t.Fatalf("Decode ran a custom decoder with no node to hand it: %+v", cfg)
	}
	if !errors.Is(err, ErrWrongContainer) {
		t.Fatalf("err = %v, want a wrong-container diagnostic", err)
	}
}

// Fails if DecodeNode stops decoding one construct at a time, or starts
// accepting a node that carries no value of its own.
func TestDecodeNode(t *testing.T) {
	doc, err := Parse([]byte("[server]\nhost = \"h\"\nports = [1, 2]\ninline = {x = 1}\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	type Server struct {
		Host   string         `toml:"host"`
		Ports  []int          `toml:"ports"`
		Inline map[string]int `toml:"inline"`
	}
	table, err := doc.Resolve("server")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var server Server
	if err := DecodeNode(table, &server); err != nil {
		t.Fatalf("DecodeNode on a table: %v", err)
	}
	if server.Host != "h" || !reflect.DeepEqual(server.Ports, []int{1, 2}) || server.Inline["x"] != 1 {
		t.Errorf("decoded %+v, want the whole table", server)
	}

	node, err := doc.Resolve("server.host")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var host string
	if err := DecodeNode(node, &host); err != nil {
		t.Fatalf("DecodeNode on a scalar: %v", err)
	}
	if host != "h" {
		t.Errorf("host = %q, want %q", host, "h")
	}

	var ports [2]int
	arr, err := doc.Resolve("server.ports")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := DecodeNode(arr, &ports); err != nil {
		t.Fatalf("DecodeNode on an array: %v", err)
	}
	if ports != [2]int{1, 2} {
		t.Errorf("ports = %v, want [1 2]", ports)
	}

	// A key carries no value of its own.
	kv := &KeyNode{Parts: []string{"host"}}
	if err := DecodeNode(kv, &host); err == nil {
		t.Error("DecodeNode accepted a key node")
	}
	if err := DecodeNode(node, host); err == nil {
		t.Error("DecodeNode accepted a non-pointer target")
	}
}

// Fails if DecodeNode stops applying the same strictness the document-level
// entry points apply.
func TestDecodeNode_IsStrictToo(t *testing.T) {
	doc, err := Parse([]byte("[server]\nhost = \"h\"\nbogus = 1\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	type Server struct {
		Host string `toml:"host"`
	}
	table, err := doc.Resolve("server")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var server Server
	err = DecodeNode(table, &server)
	if err == nil {
		t.Fatal("DecodeNode accepted an unknown key")
	}
	if !errors.Is(err, ErrUnknownKey) {
		t.Errorf("err = %v, want an unknown-key diagnostic", err)
	}
}

// --- the conversion table, through both of its consumers ---

// convStruct is the target of the table's inline-table row.
type convStruct struct {
	A int `toml:"a"`
}

// conversionCase is one cell of the conversion table, checked through the
// reflection front end (DecodeNode) and through a descriptor field
// (Validate). The two disagree only where a Go width is involved, which a
// descriptor cannot express: those rows say so.
type conversionCase struct {
	name string
	toml string // the value, as written in a document

	newTarget func() any // a fresh pointer to the Go target
	want      any        // the value the target holds when it decodes
	goKind    ErrorKind  // the diagnostic kind, when it does not
	goFails   bool

	field     Field // the descriptor equivalent of the target
	specKind  ErrorKind
	specFails bool
}

func conversionCases() []conversionCase {
	return []conversionCase{
		{
			name: "string into a string", toml: `"hi"`,
			newTarget: func() any { return new(string) }, want: "hi",
			field: Field{Kind: FieldKindString},
		},
		{
			name: "string into an integer", toml: `"hi"`,
			newTarget: func() any { return new(int) }, goFails: true, goKind: KindTypeMismatch,
			field: Field{Kind: FieldKindInteger}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "integer into an int", toml: `42`,
			newTarget: func() any { return new(int) }, want: 42,
			field: Field{Kind: FieldKindInteger},
		},
		{
			name: "integer at the int8 boundary", toml: `127`,
			newTarget: func() any { return new(int8) }, want: int8(127),
			field: Field{Kind: FieldKindInteger},
		},
		{
			// A width only the Go target declares: the descriptor's integer
			// field is an int64, which holds it.
			name: "integer overflowing an int8", toml: `128`,
			newTarget: func() any { return new(int8) }, goFails: true, goKind: KindInexact,
			field: Field{Kind: FieldKindInteger},
		},
		{
			name: "negative integer into a uint", toml: `-1`,
			newTarget: func() any { return new(uint) }, goFails: true, goKind: KindInexact,
			field: Field{Kind: FieldKindInteger},
		},
		{
			name: "integer into a float64, exactly", toml: `42`,
			newTarget: func() any { return new(float64) }, want: 42.0,
			field: Field{Kind: FieldKindFloat},
		},
		{
			// 2^53+1: both front ends refuse it, because both are float64 here.
			name: "integer no float64 holds exactly", toml: `9007199254740993`,
			newTarget: func() any { return new(float64) }, goFails: true, goKind: KindInexact,
			field: Field{Kind: FieldKindFloat}, specFails: true, specKind: KindInexact,
		},
		{
			// 2^24+1: exact in a float64, not in a float32.
			name: "integer no float32 holds exactly", toml: `16777217`,
			newTarget: func() any { return new(float32) }, goFails: true, goKind: KindInexact,
			field: Field{Kind: FieldKindFloat},
		},
		{
			name: "float into a float64", toml: `3.5`,
			newTarget: func() any { return new(float64) }, want: 3.5,
			field: Field{Kind: FieldKindFloat},
		},
		{
			name: "float truncated into a float32", toml: `3.141592653589793`,
			newTarget: func() any { return new(float32) }, want: float32(3.141592653589793),
			field: Field{Kind: FieldKindFloat},
		},
		{
			name: "float overflowing a float32", toml: `3.5e39`,
			newTarget: func() any { return new(float32) }, goFails: true, goKind: KindInexact,
			field: Field{Kind: FieldKindFloat},
		},
		{
			name: "whole float into an integer", toml: `1.0`,
			newTarget: func() any { return new(int) }, goFails: true, goKind: KindTypeMismatch,
			field: Field{Kind: FieldKindInteger}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "boolean into a bool", toml: `true`,
			newTarget: func() any { return new(bool) }, want: true,
			field: Field{Kind: FieldKindBoolean},
		},
		{
			name: "boolean into a string", toml: `true`,
			newTarget: func() any { return new(string) }, goFails: true, goKind: KindTypeMismatch,
			field: Field{Kind: FieldKindString}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "offset date-time into a time.Time", toml: `1979-05-27T07:32:00Z`,
			newTarget: func() any { return new(time.Time) }, want: time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC),
			field: Field{Kind: FieldKindOffsetDateTime},
		},
		{
			name: "local date-time into a time.Time", toml: `1979-05-27T07:32:00`,
			newTarget: func() any { return new(time.Time) }, want: time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC),
			field: Field{Kind: FieldKindLocalDateTime},
		},
		{
			name: "local date-time into an offset date-time field", toml: `1979-05-27T07:32:00`,
			newTarget: func() any { return new(time.Time) }, want: time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC),
			field: Field{Kind: FieldKindOffsetDateTime}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "local date into a time.Time", toml: `1979-05-27`,
			newTarget: func() any { return new(time.Time) }, want: time.Date(1979, 5, 27, 0, 0, 0, 0, time.UTC),
			field: Field{Kind: FieldKindLocalDate},
		},
		{
			name: "local time into a time.Time", toml: `07:32:00`,
			newTarget: func() any { return new(time.Time) }, goFails: true, goKind: KindTypeMismatch,
			field: Field{Kind: FieldKindOffsetDateTime}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "local time into a LocalTime", toml: `07:32:00`,
			newTarget: func() any { return new(LocalTime) }, want: LocalTime{Hour: 7, Minute: 32},
			field: Field{Kind: FieldKindLocalTime},
		},
		{
			name: "array into a slice", toml: `[1, 2, 3]`,
			newTarget: func() any { return new([]int) }, want: []int{1, 2, 3},
			field: Field{Kind: FieldKindArray, Elem: &Field{Kind: FieldKindInteger}},
		},
		{
			name: "array into a Go array of its own length", toml: `[1, 2, 3]`,
			newTarget: func() any { return new([3]int) }, want: [3]int{1, 2, 3},
			field: Field{Kind: FieldKindArray, Elem: &Field{Kind: FieldKindInteger}},
		},
		{
			name: "array under-filling a Go array", toml: `[1, 2]`,
			newTarget: func() any { return new([3]int) }, goFails: true, goKind: KindInexact,
			field: Field{Kind: FieldKindArray, Elem: &Field{Kind: FieldKindInteger}},
		},
		{
			name: "array with an element of the wrong kind", toml: `[1, "x"]`,
			newTarget: func() any { return new([]int) }, goFails: true, goKind: KindTypeMismatch,
			field:     Field{Kind: FieldKindArray, Elem: &Field{Kind: FieldKindInteger}},
			specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "inline table into a struct", toml: `{a = 1}`,
			newTarget: func() any { return new(convStruct) }, want: convStruct{A: 1},
			field: Field{Kind: FieldKindTable, Table: &Spec{Fields: map[string]Field{"a": {Kind: FieldKindInteger}}}},
		},
		{
			name: "inline table into a string", toml: `{a = 1}`,
			newTarget: func() any { return new(string) }, goFails: true, goKind: KindTypeMismatch,
			field: Field{Kind: FieldKindString}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "array into a string", toml: `[1]`,
			newTarget: func() any { return new(string) }, goFails: true, goKind: KindTypeMismatch,
			field: Field{Kind: FieldKindString}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "anything into any", toml: `[1, "two"]`,
			newTarget: func() any { return new(any) }, want: []any{int64(1), "two"},
			field: FieldAny(),
		},
	}
}

// Fails if a row of the conversion table stops holding for either of its
// consumers: the reflection front end, and a descriptor field. (The accessor
// families join this test when they move onto the same table.)
func TestConversionTable_BothConsumers(t *testing.T) {
	for _, tc := range conversionCases() {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := Parse([]byte("val = " + tc.toml + "\n"))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			node, err := doc.Resolve("val")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}

			target := tc.newTarget()
			err = DecodeNode(node, target)
			switch {
			case tc.goFails && err == nil:
				t.Errorf("the Go target accepted the value: %#v", reflect.ValueOf(target).Elem().Interface())
			case tc.goFails:
				if diags := diagnosticsOf(t, err); diags[0].Kind != tc.goKind {
					t.Errorf("the Go target reported %s, want %s (%v)", diags[0].Kind, tc.goKind, err)
				}
			case err != nil:
				t.Errorf("the Go target refused the value: %v", err)
			default:
				got := reflect.ValueOf(target).Elem().Interface()
				if !sameDecoded(got, tc.want) {
					t.Errorf("decoded %#v, want %#v", got, tc.want)
				}
			}

			err = doc.Validate(&Spec{Fields: map[string]Field{"val": tc.field}})
			switch {
			case tc.specFails && err == nil:
				t.Error("the descriptor field accepted the value")
			case tc.specFails:
				if diags := diagnosticsOf(t, err); diags[0].Kind != tc.specKind {
					t.Errorf("the descriptor field reported %s, want %s (%v)", diags[0].Kind, tc.specKind, err)
				}
			case err != nil:
				t.Errorf("the descriptor field refused the value: %v", err)
			}
		})
	}
}

// sameDecoded compares two decoded values, comparing instants by instant.
func sameDecoded(got, want any) bool {
	if a, ok := got.(time.Time); ok {
		b, ok := want.(time.Time)
		return ok && a.Equal(b)
	}
	return reflect.DeepEqual(got, want)
}
