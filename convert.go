package tomledit

import (
	"reflect"
	"strings"
	"time"
)

// The conversion table: the single authority on which document values a decode
// target accepts, and on what a converted value is allowed to lose. It drives
// both stages of decoding -- the type check (is this value's kind acceptable
// for this target?) and the conversion itself (does this particular value
// survive the trip?) -- and both front ends read it, so a hand-built
// descriptor and a Go struct field of the same kind answer alike.
//
// The principle it encodes: conversion BETWEEN TOML types happens only when it
// is provably value-preserving (the integer-into-float rows, checked for
// exactness); narrowing INTO the width the caller declared is the caller's own
// choice, range-checked, never silently wrapped.

// valueKind is what the table calls a document value.
type valueKind int

const (
	valString valueKind = iota
	valInteger
	valFloat
	valBoolean
	valOffsetDateTime
	valLocalDateTime
	valLocalDate
	valLocalTime
	valArray
	valArrayOfTables
	valTable
)

var valueKindNames = [...]string{
	valString:         "string",
	valInteger:        "integer",
	valFloat:          "float",
	valBoolean:        "boolean",
	valOffsetDateTime: "offset date-time",
	valLocalDateTime:  "local date-time",
	valLocalDate:      "local date",
	valLocalTime:      "local time",
	valArray:          "array",
	valArrayOfTables:  "array of tables",
	valTable:          "table",
}

// String returns the human-readable name of the value kind.
func (k valueKind) String() string {
	if int(k) >= 0 && int(k) < len(valueKindNames) {
		return valueKindNames[k]
	}
	return "unknown"
}

// targetClass is what the table calls a decode target: the Go type or the
// descriptor field kind a document value is being converted into.
type targetClass int

const (
	targetString         targetClass = iota
	targetInteger                    // the signed Go integers, and a descriptor integer field
	targetUnsigned                   // the unsigned Go integers
	targetFloat                      // float32, float64, and a descriptor float field
	targetBoolean                    //
	targetTime                       // time.Time
	targetOffsetDateTime             // a descriptor offset-date-time field
	targetLocalDateTime              // LocalDateTime, and the matching descriptor field
	targetLocalDate                  // LocalDate, and the matching descriptor field
	targetLocalTime                  // LocalTime, and the matching descriptor field
	targetArray                      // a Go slice or array, and a descriptor array field
	targetTable                      // a Go struct or map, and a descriptor table field
	targetAny                        // an empty interface, and the descriptor's any field
)

// conversionRow is one row of the conversion table: a target, and the document
// values it accepts. A value kind the row does not list is a type mismatch;
// whether a listed value converts is then decided by convertValue, which
// enforces the per-row rules stated in the comments below.
type conversionRow struct {
	target  targetClass
	accepts []valueKind
}

// conversionTable is the table itself. Every acceptance question in the
// package is answered from here.
var conversionTable = []conversionRow{
	// string: verbatim.
	{target: targetString, accepts: []valueKind{valString}},
	// integer: range-checked into the declared width; an overflow, and a
	// negative value in an unsigned target, are refused.
	{target: targetInteger, accepts: []valueKind{valInteger}},
	{target: targetUnsigned, accepts: []valueKind{valInteger}},
	// float: a float verbatim into float64, range-checked into float32 (the
	// caller declared the width, so truncated precision is their choice); an
	// integer only when the target holds it exactly -- the widening rule.
	{target: targetFloat, accepts: []valueKind{valFloat, valInteger}},
	{target: targetBoolean, accepts: []valueKind{valBoolean}},
	// time.Time: an offset date-time verbatim; the local flavors convert,
	// because the declared target expresses the intent. A local time has no
	// date to convert with, so it is not among them.
	{target: targetTime, accepts: []valueKind{valOffsetDateTime, valLocalDateTime, valLocalDate}},
	{target: targetOffsetDateTime, accepts: []valueKind{valOffsetDateTime}},
	{target: targetLocalDateTime, accepts: []valueKind{valLocalDateTime}},
	{target: targetLocalDate, accepts: []valueKind{valLocalDate}},
	{target: targetLocalTime, accepts: []valueKind{valLocalTime}},
	// containers: accepted whole, then decoded elementwise by this same
	// table. An array-of-tables is an array whose elements are tables.
	{target: targetArray, accepts: []valueKind{valArray, valArrayOfTables}},
	{target: targetTable, accepts: []valueKind{valTable}},
	// any: total by construction -- every document value has a native form.
	{target: targetAny, accepts: allValueKinds()},
}

// allValueKinds returns every value kind, in table order.
func allValueKinds() []valueKind {
	kinds := make([]valueKind, 0, len(valueKindNames))
	for k := range valueKindNames {
		kinds = append(kinds, valueKind(k))
	}
	return kinds
}

// conversionAccepts indexes the table for lookup. It is derived from the table
// rather than written out again, so the two cannot drift apart.
var conversionAccepts = func() map[targetClass]map[valueKind]bool {
	index := map[targetClass]map[valueKind]bool{}
	for _, row := range conversionTable {
		set := map[valueKind]bool{}
		for _, k := range row.accepts {
			set[k] = true
		}
		index[row.target] = set
	}
	return index
}()

// accepts reports whether the table lets a value of this kind reach this
// target at all -- the type-check stage.
func (t targetClass) accepts(k valueKind) bool {
	return conversionAccepts[t][k]
}

// accepted lists the value kinds this target takes, in table order.
func (t targetClass) accepted() []valueKind {
	for _, row := range conversionTable {
		if row.target == t {
			return row.accepts
		}
	}
	return nil
}

// describeAccepted renders the accepted kinds of a target for a diagnostic
// that has no better name for it than what it takes.
func (t targetClass) describeAccepted() string {
	names := make([]string, 0, 4)
	for _, k := range t.accepted() {
		names = append(names, k.String())
	}
	switch len(names) {
	case 0:
		return "nothing"
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
	}
}

// valueKindOf reports which row of the table a value node belongs to.
func valueKindOf(n Node) (valueKind, bool) {
	switch n.(type) {
	case *StringNode:
		return valString, true
	case *IntegerNode:
		return valInteger, true
	case *FloatNode:
		return valFloat, true
	case *BooleanNode:
		return valBoolean, true
	case *DateTimeNode:
		return valOffsetDateTime, true
	case *LocalDateTimeNode:
		return valLocalDateTime, true
	case *LocalDateNode:
		return valLocalDate, true
	case *LocalTimeNode:
		return valLocalTime, true
	case *ArrayNode:
		return valArray, true
	case *InlineTableNode:
		return valTable, true
	}
	return 0, false
}

var (
	timeType          = reflect.TypeOf(time.Time{})
	localDateTimeType = reflect.TypeOf(LocalDateTime{})
	localDateType     = reflect.TypeOf(LocalDate{})
	localTimeType     = reflect.TypeOf(LocalTime{})
)

// targetClassOf reports which row of the table a Go type reads, and whether
// the table has one for it at all: a channel, a function or a non-empty
// interface is not a decode target.
func targetClassOf(rt reflect.Type) (targetClass, bool) {
	switch rt {
	case timeType:
		return targetTime, true
	case localDateTimeType:
		return targetLocalDateTime, true
	case localDateType:
		return targetLocalDate, true
	case localTimeType:
		return targetLocalTime, true
	}
	switch rt.Kind() {
	case reflect.String:
		return targetString, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return targetInteger, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return targetUnsigned, true
	case reflect.Float32, reflect.Float64:
		return targetFloat, true
	case reflect.Bool:
		return targetBoolean, true
	case reflect.Slice, reflect.Array:
		return targetArray, true
	case reflect.Struct, reflect.Map:
		return targetTable, true
	case reflect.Interface:
		if rt.NumMethod() == 0 {
			return targetAny, true
		}
	}
	return 0, false
}

// convertValue writes the value a scalar node carries into rv, applying the
// table's range, sign and exactness rules for the declared width. The type
// check has already accepted the pairing, so a refusal here is always about
// the particular value, and is always KindInexact.
//
// rv is invalid when the caller is validating rather than decoding; the rules
// that do not depend on a Go width still run, against the widths a descriptor
// field implies (int64 for an integer field, float64 for a float field).
func convertValue(n Node, class targetClass, rv reflect.Value) *Error {
	switch class {
	case targetString:
		if rv.IsValid() {
			rv.SetString(n.(*StringNode).Val)
		}
	case targetInteger:
		v := n.(*IntegerNode).Val
		if rv.IsValid() {
			if rv.OverflowInt(v) {
				return newError(KindInexact, "integer %d overflows %s", v, rv.Type()).withValue(v)
			}
			rv.SetInt(v)
		}
	case targetUnsigned:
		v := n.(*IntegerNode).Val
		if v < 0 {
			return newError(KindInexact, "negative integer %d cannot be stored in %s", v, describeType(rv, "an unsigned integer")).withValue(v)
		}
		if rv.IsValid() {
			if rv.OverflowUint(uint64(v)) {
				return newError(KindInexact, "integer %d overflows %s", v, rv.Type()).withValue(v)
			}
			rv.SetUint(uint64(v))
		}
	case targetFloat:
		bits := 64
		if rv.IsValid() {
			bits = rv.Type().Bits()
		}
		switch v := n.(type) {
		case *IntegerNode:
			f, exact := exactFloat(v.Val, bits)
			if !exact {
				return newError(KindInexact, "integer %d is not exactly representable in %s",
					v.Val, describeType(rv, "float64")).withValue(v.Val)
			}
			if rv.IsValid() {
				rv.SetFloat(f)
			}
		case *FloatNode:
			if rv.IsValid() {
				if rv.OverflowFloat(v.Val) {
					return newError(KindInexact, "float %g overflows %s", v.Val, rv.Type()).withValue(v.Val)
				}
				rv.SetFloat(v.Val)
			}
		}
	case targetBoolean:
		if rv.IsValid() {
			rv.SetBool(n.(*BooleanNode).Val)
		}
	case targetTime:
		if rv.IsValid() {
			rv.Set(reflect.ValueOf(goTime(n)))
		}
	case targetLocalDateTime:
		if rv.IsValid() {
			rv.Set(reflect.ValueOf(n.(*LocalDateTimeNode).Val))
		}
	case targetLocalDate:
		if rv.IsValid() {
			rv.Set(reflect.ValueOf(n.(*LocalDateNode).Val))
		}
	case targetLocalTime:
		if rv.IsValid() {
			rv.Set(reflect.ValueOf(n.(*LocalTimeNode).Val))
		}
	}
	return nil
}

// describeType names rv's type for a diagnostic, or falls back to the width a
// descriptor field implies when there is no Go target.
func describeType(rv reflect.Value, fallback string) string {
	if rv.IsValid() {
		return rv.Type().String()
	}
	return fallback
}

// exactFloat converts an integer into a float of the given width and reports
// whether the value survived: the widening rule refuses anything else.
func exactFloat(i int64, bits int) (float64, bool) {
	f := float64(i)
	// 2^63 is the first value int64 cannot express, so a float that reached it
	// rounded up and the round-trip below would be undefined.
	if f >= 9223372036854775808.0 {
		return f, false
	}
	if int64(f) != i {
		return f, false
	}
	if bits == 32 {
		narrowed := float32(f)
		if float64(narrowed) != f {
			return f, false
		}
		return float64(narrowed), true
	}
	return f, true
}

// goTime converts a date-time node into a time.Time, per the table's
// time.Time row: an offset date-time verbatim, a local flavor read as UTC.
func goTime(n Node) time.Time {
	switch v := n.(type) {
	case *DateTimeNode:
		return v.Val
	case *LocalDateTimeNode:
		return time.Date(v.Val.Year, time.Month(v.Val.Month), v.Val.Day,
			v.Val.Hour, v.Val.Minute, v.Val.Second, v.Val.Nanosecond, time.UTC)
	case *LocalDateNode:
		return time.Date(v.Val.Year, time.Month(v.Val.Month), v.Val.Day, 0, 0, 0, 0, time.UTC)
	}
	return time.Time{}
}

// nativeValue returns the Go value an any-typed target receives for a value
// node: the native mapping of the conversion table's last row.
func nativeValue(n Node) (any, *Error) {
	switch v := n.(type) {
	case *StringNode:
		return v.Val, nil
	case *IntegerNode:
		return v.Val, nil
	case *FloatNode:
		return v.Val, nil
	case *BooleanNode:
		return v.Val, nil
	case *DateTimeNode:
		return v.Val, nil
	case *LocalDateTimeNode:
		return v.Val, nil
	case *LocalDateNode:
		return v.Val, nil
	case *LocalTimeNode:
		return v.Val, nil
	case *ArrayNode:
		out := make([]any, len(v.Elements))
		for i, elem := range v.Elements {
			native, err := nativeValue(elem)
			if err != nil {
				return nil, err
			}
			out[i] = native
		}
		return out, nil
	case *InlineTableNode:
		rec, err := foldInlineTable(v)
		if err != nil {
			return nil, asDiagnostic(err)
		}
		return nativeRecord(rec)
	}
	return nil, newError(KindTypeMismatch, "a %s node carries no value", n.Type())
}

// nativeRecord returns the map an any-typed target receives for a record.
func nativeRecord(rec *Record) (any, *Error) {
	out := make(map[string]any, rec.Len())
	for e := range rec.Entries() {
		native, err := nativeEntry(e)
		if err != nil {
			return nil, err
		}
		out[e.key] = native
	}
	return out, nil
}

// nativeEntry returns the Go value an any-typed target receives for one entry,
// whatever the entry holds.
func nativeEntry(e Entry) (any, *Error) {
	switch e.kind {
	case EntryRecord:
		return nativeRecord(e.record)
	case EntryRecords:
		out := make([]any, len(e.records))
		for i, rec := range e.records {
			native, err := nativeRecord(rec)
			if err != nil {
				return nil, err
			}
			out[i] = native
		}
		return out, nil
	default:
		return nativeValue(e.node)
	}
}
