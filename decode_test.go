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
// plus keys buried under a construct that is itself refused. The keys spelled
// "Name" and "Host" differ from a declared key only in case: matching is exact
// in both front ends, so each is an unknown key like any other.
const engineTestDoc = `name = "app"
port = "8080"
extra = true
Name = "cased"

[server]
host = "h"
bogus = 1
Host = "cased"

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
	cfg, err := Decode[engineConfig](doc)
	if err == nil {
		t.Fatal("Decode accepted a document full of violations")
	}

	want := []struct {
		kind ErrorKind
		path string
		line int
		col  int
	}{
		{KindTypeMismatch, "port", 2, 1},
		{KindUnknownKey, "extra", 3, 1},
		{KindUnknownKey, "Name", 4, 1},
		{KindUnknownKey, "server.bogus", 8, 1},
		{KindUnknownKey, "server.Host", 9, 1},
		{KindUnknownTable, "unknown_table", 11, 2},
		{KindTypeMismatch, "items[0].label", 16, 1},
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

	// The keys that decoded cleanly are unreachable: a failed decode answers
	// with diagnostics and nothing else.
	if cfg != nil {
		t.Errorf("a failed decode returned %+v, want no value at all", cfg)
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
	_, err = Decode[engineConfig](doc)
	for _, d := range diagnosticsOf(t, err) {
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
// kind and required-ness -- the kinds, the paths, and their order. The fixture
// includes keys that differ from a declared key only in case, so the agreement
// covers how each front end matches spelling, rather than avoiding it.
func TestEngine_FrontEndsReportIdentically(t *testing.T) {
	doc, err := Parse([]byte(engineTestDoc))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = Decode[engineConfig](doc)
	viaStruct := diagnosticsOf(t, err)
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
	_, err = Decode[engineConfig](doc)
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

// selfDecoding declares the two method sets a decoder elsewhere would honour --
// an UnmarshalTOML handed the node, and an UnmarshalText handed a string's
// bytes -- and records whether either ever ran. This package's engine is the
// only decode path, so neither may.
type selfDecoding struct {
	Ran   string `toml:"-"`
	Inner int    `toml:"inner"`
}

func (s *selfDecoding) UnmarshalTOML(node Node) error {
	s.Ran = "UnmarshalTOML"
	return nil
}

func (s *selfDecoding) UnmarshalText(text []byte) error {
	s.Ran = "UnmarshalText"
	return nil
}

// Fails if a target's own UnmarshalTOML or UnmarshalText runs during a decode.
// No consumer code runs inside the walk: a type declaring those methods is
// decoded through the conversion table like any other, so its table shape is
// what the document must match and a string is refused because the table
// refuses it -- not deferred to the target's own parsing.
func TestEngine_ATargetNeverDecodesItself(t *testing.T) {
	t.Run("a table decodes through the table's own fields", func(t *testing.T) {
		doc := mustParse(t, "[val]\ninner = 1\n")
		cfg, err := Decode[struct {
			Val selfDecoding `toml:"val"`
		}](doc)
		if err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		if cfg.Val.Ran != "" {
			t.Errorf("the target's own %s ran", cfg.Val.Ran)
		}
		if cfg.Val.Inner != 1 {
			t.Errorf("Inner = %d, want the field the engine wrote (1)", cfg.Val.Inner)
		}
	})

	t.Run("a string is a type mismatch, not the target's to parse", func(t *testing.T) {
		doc := mustParse(t, "val = \"x\"\n")
		cfg, err := Decode[struct {
			Val selfDecoding `toml:"val"`
		}](doc)
		if err == nil {
			t.Fatalf("Decode let the target parse a string itself: %+v", cfg)
		}
		if !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("err = %v, want a type-mismatch diagnostic", err)
		}
	})

	t.Run("a time.Time refuses an RFC 3339 string, as the accessors do", func(t *testing.T) {
		doc := mustParse(t, "when = \"1979-05-27T07:32:00Z\"\n")
		cfg, err := Decode[struct {
			When time.Time `toml:"when"`
		}](doc)
		if err == nil {
			t.Fatalf("Decode read a string as a time.Time: %+v", cfg)
		}
		if !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("err = %v, want a type-mismatch diagnostic", err)
		}
	})
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
	server, err := DecodeNode[Server](table)
	if err != nil {
		t.Fatalf("DecodeNode on a table: %v", err)
	}
	if server.Host != "h" || !reflect.DeepEqual(server.Ports, []int{1, 2}) || server.Inline["x"] != 1 {
		t.Errorf("decoded %+v, want the whole table", server)
	}

	node, err := doc.Resolve("server.host")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	host, err := DecodeNode[string](node)
	if err != nil {
		t.Fatalf("DecodeNode on a scalar: %v", err)
	}
	if *host != "h" {
		t.Errorf("host = %q, want %q", *host, "h")
	}

	arr, err := doc.Resolve("server.ports")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	ports, err := DecodeNode[[2]int](arr)
	if err != nil {
		t.Fatalf("DecodeNode on an array: %v", err)
	}
	if *ports != [2]int{1, 2} {
		t.Errorf("ports = %v, want [1 2]", *ports)
	}

	// A key carries no value of its own.
	kv := &KeyNode{parts: []string{"host"}}
	if _, err := DecodeNode[string](kv); err == nil {
		t.Error("DecodeNode accepted a key node")
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
	server, err := DecodeNode[Server](table)
	if err == nil {
		t.Fatalf("DecodeNode accepted an unknown key: %+v", server)
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

	// decode reads the value as the Go target of this row. The target type is
	// a compile-time argument, so each row spells its own decodeAs.
	decode  decodeSide
	want    any       // the value the target holds when it decodes
	goKind  ErrorKind // the diagnostic kind, when it does not
	goFails bool

	field     Field // the descriptor equivalent of the target
	specKind  ErrorKind
	specFails bool
}

func conversionCases() []conversionCase {
	return []conversionCase{
		{
			name: "string into a string", toml: `"hi"`,
			decode: decodeAs[string](), want: "hi",
			field: Field{Kind: FieldKindString},
		},
		{
			name: "string into an integer", toml: `"hi"`,
			decode: decodeAs[int](), goFails: true, goKind: KindTypeMismatch,
			field: Field{Kind: FieldKindInteger}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "integer into an int", toml: `42`,
			decode: decodeAs[int](), want: 42,
			field: Field{Kind: FieldKindInteger},
		},
		{
			name: "integer at the int8 boundary", toml: `127`,
			decode: decodeAs[int8](), want: int8(127),
			field: Field{Kind: FieldKindInteger},
		},
		{
			// A width only the Go target declares: the descriptor's integer
			// field is an int64, which holds it.
			name: "integer overflowing an int8", toml: `128`,
			decode: decodeAs[int8](), goFails: true, goKind: KindInexact,
			field: Field{Kind: FieldKindInteger},
		},
		{
			name: "negative integer into a uint", toml: `-1`,
			decode: decodeAs[uint](), goFails: true, goKind: KindInexact,
			field: Field{Kind: FieldKindInteger},
		},
		{
			name: "integer into a float64, exactly", toml: `42`,
			decode: decodeAs[float64](), want: 42.0,
			field: Field{Kind: FieldKindFloat},
		},
		{
			// 2^53+1: both front ends refuse it, because both are float64 here.
			name: "integer no float64 holds exactly", toml: `9007199254740993`,
			decode: decodeAs[float64](), goFails: true, goKind: KindInexact,
			field: Field{Kind: FieldKindFloat}, specFails: true, specKind: KindInexact,
		},
		{
			// 2^24+1: exact in a float64, not in a float32.
			name: "integer no float32 holds exactly", toml: `16777217`,
			decode: decodeAs[float32](), goFails: true, goKind: KindInexact,
			field: Field{Kind: FieldKindFloat},
		},
		{
			name: "float into a float64", toml: `3.5`,
			decode: decodeAs[float64](), want: 3.5,
			field: Field{Kind: FieldKindFloat},
		},
		{
			name: "float truncated into a float32", toml: `3.141592653589793`,
			decode: decodeAs[float32](), want: float32(3.141592653589793),
			field: Field{Kind: FieldKindFloat},
		},
		{
			name: "float overflowing a float32", toml: `3.5e39`,
			decode: decodeAs[float32](), goFails: true, goKind: KindInexact,
			field: Field{Kind: FieldKindFloat},
		},
		{
			name: "whole float into an integer", toml: `1.0`,
			decode: decodeAs[int](), goFails: true, goKind: KindTypeMismatch,
			field: Field{Kind: FieldKindInteger}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "boolean into a bool", toml: `true`,
			decode: decodeAs[bool](), want: true,
			field: Field{Kind: FieldKindBoolean},
		},
		{
			name: "boolean into a string", toml: `true`,
			decode: decodeAs[string](), goFails: true, goKind: KindTypeMismatch,
			field: Field{Kind: FieldKindString}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "offset date-time into a time.Time", toml: `1979-05-27T07:32:00Z`,
			decode: decodeAs[time.Time](), want: time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC),
			field: Field{Kind: FieldKindOffsetDateTime},
		},
		{
			name: "local date-time into a time.Time", toml: `1979-05-27T07:32:00`,
			decode: decodeAs[time.Time](), want: time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC),
			field: Field{Kind: FieldKindLocalDateTime},
		},
		{
			name: "local date-time into an offset date-time field", toml: `1979-05-27T07:32:00`,
			decode: decodeAs[time.Time](), want: time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC),
			field: Field{Kind: FieldKindOffsetDateTime}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "local date into a time.Time", toml: `1979-05-27`,
			decode: decodeAs[time.Time](), want: time.Date(1979, 5, 27, 0, 0, 0, 0, time.UTC),
			field: Field{Kind: FieldKindLocalDate},
		},
		{
			name: "local time into a time.Time", toml: `07:32:00`,
			decode: decodeAs[time.Time](), goFails: true, goKind: KindTypeMismatch,
			field: Field{Kind: FieldKindOffsetDateTime}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "local time into a LocalTime", toml: `07:32:00`,
			decode: decodeAs[LocalTime](), want: LocalTime{Hour: 7, Minute: 32},
			field: Field{Kind: FieldKindLocalTime},
		},
		{
			name: "array into a slice", toml: `[1, 2, 3]`,
			decode: decodeAs[[]int](), want: []int{1, 2, 3},
			field: Field{Kind: FieldKindArray, Elem: &Field{Kind: FieldKindInteger}},
		},
		{
			name: "array into a Go array of its own length", toml: `[1, 2, 3]`,
			decode: decodeAs[[3]int](), want: [3]int{1, 2, 3},
			field: Field{Kind: FieldKindArray, Elem: &Field{Kind: FieldKindInteger}},
		},
		{
			name: "array under-filling a Go array", toml: `[1, 2]`,
			decode: decodeAs[[3]int](), goFails: true, goKind: KindInexact,
			field: Field{Kind: FieldKindArray, Elem: &Field{Kind: FieldKindInteger}},
		},
		{
			name: "array with an element of the wrong kind", toml: `[1, "x"]`,
			decode: decodeAs[[]int](), goFails: true, goKind: KindTypeMismatch,
			field:     Field{Kind: FieldKindArray, Elem: &Field{Kind: FieldKindInteger}},
			specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "inline table into a struct", toml: `{a = 1}`,
			decode: decodeAs[convStruct](), want: convStruct{A: 1},
			field: Field{Kind: FieldKindTable, Table: &Spec{Fields: map[string]Field{"a": {Kind: FieldKindInteger}}}},
		},
		{
			name: "inline table into a string", toml: `{a = 1}`,
			decode: decodeAs[string](), goFails: true, goKind: KindTypeMismatch,
			field: Field{Kind: FieldKindString}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "array into a string", toml: `[1]`,
			decode: decodeAs[string](), goFails: true, goKind: KindTypeMismatch,
			field: Field{Kind: FieldKindString}, specFails: true, specKind: KindTypeMismatch,
		},
		{
			name: "anything into any", toml: `[1, "two"]`,
			decode: decodeAs[any](), want: []any{int64(1), "two"},
			field: FieldAny(),
		},
	}
}

// Fails if a row of the conversion table stops holding for either of its
// consumers: the reflection front end, and a descriptor field. The third
// consumer, the accessor families, is checked against the same cases by
// TestConversionTable_AccessorFamilies, which asserts it answers what a decode
// target of the same Go type answers.
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

			got, err := tc.decode.read(node)
			switch {
			case tc.goFails && err == nil:
				t.Errorf("the Go target accepted the value: %#v", got)
			case tc.goFails:
				if diags := diagnosticsOf(t, err); diags[0].Kind != tc.goKind {
					t.Errorf("the Go target reported %s, want %s (%v)", diags[0].Kind, tc.goKind, err)
				}
			case err != nil:
				t.Errorf("the Go target refused the value: %v", err)
			default:
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

// --- the value-returning contract ---
//
// A decode returns a value it allocated itself, or diagnostics and nothing
// else. The tests below pin what that buys: after a failure there is no state
// anywhere the caller can reach, and the one form that starts from a caller's
// data takes a factory, so what it fills is still an allocation of its own.

// overlayInner is the pointer-reachable half of the seed below: a failed decode
// must not have written through the pointer the caller holds.
type overlayInner struct {
	Count int `toml:"count"`
}

// overlaySeed carries one of each thing a decode could write THROUGH rather
// than into: a map, a pointer, and a slice.
type overlaySeed struct {
	Name  string            `toml:"name"`
	Tags  map[string]string `toml:"tags"`
	Inner *overlayInner     `toml:"inner"`
	Ports []int             `toml:"ports"`
}

// freshOverlaySeed builds a seed from base without sharing anything with it,
// which is what a seed factory is for.
func freshOverlaySeed(base overlaySeed) func() overlaySeed {
	return func() overlaySeed {
		tags := make(map[string]string, len(base.Tags))
		for k, v := range base.Tags {
			tags[k] = v
		}
		return overlaySeed{
			Name:  base.Name,
			Tags:  tags,
			Inner: &overlayInner{Count: base.Inner.Count},
			Ports: append([]int(nil), base.Ports...),
		}
	}
}

// baseOverlaySeed is the value the caller holds in the tests below.
func baseOverlaySeed() overlaySeed {
	return overlaySeed{
		Name:  "seed",
		Tags:  map[string]string{"env": "dev"},
		Inner: &overlayInner{Count: 7},
		Ports: []int{1},
	}
}

// Fails if a failed decode leaves anything behind for the caller to find. The
// engine collects violations, so it keeps walking and keeps writing after the
// first one -- into an allocation of its own, which it then drops. Nothing the
// caller holds, and nothing reachable from it through a map or a pointer, may
// have moved.
func TestDecode_AFailureLeavesNothingObservable(t *testing.T) {
	// The document writes name, tags and inner.count cleanly, then violates:
	// the keys before, beside and after the violation are exactly the ones a
	// partial write would have left behind.
	doc := mustParse(t, `name = "from the document"
ports = [9, 9]

[tags]
env = "prod"

[inner]
count = 42

[unknown_table]
x = 1
`)

	// The plain form: there is not even a target to ask about.
	if cfg, err := Decode[overlaySeed](doc); err == nil {
		t.Fatal("Decode accepted a document with an unknown table")
	} else if cfg != nil {
		t.Errorf("a failed Decode returned %+v, want no value at all", cfg)
	}

	// The seed form: the caller's own value, and everything reachable from it.
	base := baseOverlaySeed()
	tags, inner, ports := base.Tags, base.Inner, base.Ports
	got, written, err := DecodeOver(doc, freshOverlaySeed(base))
	if err == nil {
		t.Fatal("DecodeOver accepted a document with an unknown table")
	}
	if got != nil || written != nil {
		t.Errorf("a failed DecodeOver returned (%+v, %v), want no value and no paths", got, written)
	}
	if base.Name != "seed" || base.Inner.Count != 7 {
		t.Errorf("the caller's seed reads %+v, want it untouched", base)
	}
	if v := tags["env"]; v != "dev" {
		t.Errorf("the caller's map reads %q, want the decode never to have reached it", v)
	}
	if inner.Count != 7 {
		t.Errorf("the caller's pointee reads %d, want the decode never to have reached it", inner.Count)
	}
	if len(ports) != 1 || ports[0] != 1 {
		t.Errorf("the caller's slice reads %v, want the decode never to have reached it", ports)
	}
}

// Fails if the seed stops surviving where the document is silent, or if the
// written paths stop naming what the document supplied. Together they are the
// defaults-overlay pattern: the seed says what the value is when the file says
// nothing, and the paths say which of it the file replaced.
func TestDecodeOver_TheSeedSurvivesWhereTheDocumentIsSilent(t *testing.T) {
	doc := mustParse(t, `name = "from the document"
ports = [8080]

[inner]
count = 42
`)

	base := baseOverlaySeed()
	got, written, err := DecodeOver(doc, freshOverlaySeed(base))
	if err != nil {
		t.Fatalf("DecodeOver failed: %v", err)
	}
	if got.Name != "from the document" {
		t.Errorf("Name = %q, want the document's value", got.Name)
	}
	if got.Inner.Count != 42 {
		t.Errorf("Inner.Count = %d, want the document's value", got.Inner.Count)
	}
	if !reflect.DeepEqual(got.Ports, []int{8080}) {
		t.Errorf("Ports = %v, want the document's array", got.Ports)
	}
	// The document never mentions tags, so the seed's own map is what remains.
	if got.Tags["env"] != "dev" {
		t.Errorf("Tags = %v, want the seed's map where the document is silent", got.Tags)
	}
	// An array is written whole, so it is one path rather than one per element,
	// and the paths read in document order.
	want := []string{"name", "ports", "inner.count"}
	if !reflect.DeepEqual(written, want) {
		t.Errorf("written = %v, want %v", written, want)
	}

	// The caller's value is not the decoded one: the factory built a seed of
	// its own for the decode to fill.
	if got.Tags["env"] = "touched"; base.Tags["env"] != "dev" {
		t.Error("writing to the decoded value reached the caller's own map")
	}
	if got.Inner == base.Inner {
		t.Error("the decoded value shares its pointee with the caller's seed")
	}
}

// Fails if the seed factory stops being called per decode: two decodes must
// not share the value they fill, or one caller's result is another's starting
// point.
func TestDecodeOver_TheFactoryBuildsAFreshSeedPerCall(t *testing.T) {
	doc := mustParse(t, "name = \"x\"\n")
	base := baseOverlaySeed()
	calls := 0
	seed := func() overlaySeed {
		calls++
		return freshOverlaySeed(base)()
	}

	first, _, err := DecodeOver(doc, seed)
	if err != nil {
		t.Fatalf("DecodeOver failed: %v", err)
	}
	second, _, err := DecodeOver(doc, seed)
	if err != nil {
		t.Fatalf("DecodeOver failed: %v", err)
	}
	if calls != 2 {
		t.Errorf("the factory ran %d times for two decodes, want 2", calls)
	}
	if first == second || first.Inner == second.Inner || &first.Tags == &second.Tags {
		t.Error("two decodes share the value they filled")
	}
	first.Tags["env"] = "touched"
	if second.Tags["env"] != "dev" {
		t.Error("writing to one decoded value reached the other")
	}
}

// Fails if a missing seed factory is accepted. There is no default seed to fall
// back to: a decode with nothing to overlay is Decode, which the caller can
// spell.
func TestDecodeOver_ANilFactoryIsRefused(t *testing.T) {
	doc := mustParse(t, "name = \"x\"\n")
	if _, _, err := DecodeOver[overlaySeed](doc, nil); err == nil {
		t.Fatal("DecodeOver accepted a nil seed factory")
	}
}

// Fails if a value written WHOLE stops counting as one written path: an
// any-typed table and an array are each replaced entire, so the paths inside
// them were never written on their own account.
func TestDecodeOver_AValueWrittenWholeIsOnePath(t *testing.T) {
	type Config struct {
		Ports  []int             `toml:"ports"`
		Native any               `toml:"native"`
		Tags   map[string]string `toml:"tags"`
	}
	doc := mustParse(t, `ports = [1, 2]

[native]
a = 1
b = 2

[tags]
x = "1"
y = "2"
`)
	_, written, err := DecodeOver(doc, func() Config { return Config{} })
	if err != nil {
		t.Fatalf("DecodeOver failed: %v", err)
	}
	// The array and the any-typed table are one path each; the map is written
	// key by key, because each key is a value of its own.
	want := []string{"ports", "native", "tags.x", "tags.y"}
	if !reflect.DeepEqual(written, want) {
		t.Errorf("written = %v, want %v", written, want)
	}
}
