package tomledit

import (
	"fmt"
	"sort"
)

// The descriptor: the expected shape of a document, described as data.
//
// Decoding is strict, and a descriptor is what strictness is measured against:
// a key matching no field is an error, a value of the wrong kind is an error,
// and a required key the document does not carry is an error. The reflection
// front end (Decode, Unmarshal) derives a descriptor from a Go type and runs
// the very same engine, so a consumer whose schema is only known at runtime
// builds a Spec by hand and gets the same answers.

// FieldKind names what a descriptor field expects. There is one member per
// TOML value type -- the four date-time flavors kept apart, never unified --
// plus the two containers and the total FieldKindAny.
type FieldKind int

const (
	// FieldKindString expects a string.
	FieldKindString FieldKind = iota
	// FieldKindInteger expects an integer.
	FieldKindInteger
	// FieldKindFloat expects a float, or an integer that a float holds
	// exactly.
	FieldKindFloat
	// FieldKindBoolean expects a boolean.
	FieldKindBoolean
	// FieldKindOffsetDateTime expects a date-time carrying an offset.
	FieldKindOffsetDateTime
	// FieldKindLocalDateTime expects a date-time without an offset.
	FieldKindLocalDateTime
	// FieldKindLocalDate expects a date.
	FieldKindLocalDate
	// FieldKindLocalTime expects a time of day.
	FieldKindLocalTime
	// FieldKindArray expects an array, or an array-of-tables. The element
	// descriptor is required.
	FieldKindArray
	// FieldKindTable expects a table in any spelling -- a header table, an
	// inline table, or one implied by a longer header or a dotted key. The
	// field set is required.
	FieldKindTable
	// FieldKindAny expects anything, and reports nothing about what it holds.
	FieldKindAny
)

var fieldKindNames = [...]string{
	FieldKindString:         "string",
	FieldKindInteger:        "integer",
	FieldKindFloat:          "float",
	FieldKindBoolean:        "boolean",
	FieldKindOffsetDateTime: "offset date-time",
	FieldKindLocalDateTime:  "local date-time",
	FieldKindLocalDate:      "local date",
	FieldKindLocalTime:      "local time",
	FieldKindArray:          "array",
	FieldKindTable:          "table",
	FieldKindAny:            "any",
}

// String returns the human-readable name of the field kind.
func (k FieldKind) String() string {
	if int(k) >= 0 && int(k) < len(fieldKindNames) {
		return fieldKindNames[k]
	}
	return fmt.Sprintf("FieldKind(%d)", int(k))
}

// fieldKindClasses maps every field kind onto its row of the conversion table,
// so a descriptor field and a Go target of the same kind accept the same
// document values.
var fieldKindClasses = [...]targetClass{
	FieldKindString:         targetString,
	FieldKindInteger:        targetInteger,
	FieldKindFloat:          targetFloat,
	FieldKindBoolean:        targetBoolean,
	FieldKindOffsetDateTime: targetOffsetDateTime,
	FieldKindLocalDateTime:  targetLocalDateTime,
	FieldKindLocalDate:      targetLocalDate,
	FieldKindLocalTime:      targetLocalTime,
	FieldKindArray:          targetArray,
	FieldKindTable:          targetTable,
	FieldKindAny:            targetAny,
}

// valid reports whether k names a kind at all.
func (k FieldKind) valid() bool {
	return int(k) >= 0 && int(k) < len(fieldKindClasses)
}

// Spec is the descriptor of a table: the keys it may carry, and whether keys
// beyond them are permitted at all.
//
// Field values are built whole and assigned into Fields -- map elements are
// not addressable, so mutating one in place is not a supported spelling:
//
//	spec := &tomledit.Spec{Fields: map[string]tomledit.Field{
//	    "host": {Kind: tomledit.FieldKindString, Required: true},
//	    "port": {Kind: tomledit.FieldKindInteger},
//	}}
//
// A missing-key diagnostic is reported in lexicographic key order, because map
// iteration has no order to report in.
type Spec struct {
	// Fields describes the keys the table may carry, by name.
	Fields map[string]Field

	// Dynamic describes every key Fields does not name: it is how a table of
	// arbitrary keys, all of one shape, is spelled. A nil Dynamic means no
	// other key is permitted, and is the one place in a descriptor where nil
	// carries a meaning of its own.
	Dynamic *Field
}

// Field is one expected value of a descriptor.
type Field struct {
	// Kind is what the value must be.
	Kind FieldKind

	// Required makes the absence of the key an error.
	Required bool

	// Elem describes the elements of an array. It is required for
	// FieldKindArray and refused for every other kind.
	Elem *Field

	// Table describes the keys of a table. It is required for
	// FieldKindTable and refused for every other kind.
	Table *Spec
}

// FieldAny returns the descriptor of a value of any kind: the explicit
// spelling of "whatever is here, I am not describing it".
func FieldAny() Field { return Field{Kind: FieldKindAny} }

// Validate checks the document against the descriptor and reports every
// independent violation it finds, in document order, as one aggregate error
// (see Errors): an unknown key or table, a value of a kind the field refuses,
// a value the field cannot hold exactly, and a required key the document does
// not carry. Validation continues across siblings but never descends below a
// construct it has already refused, so one broken table cannot bury the
// document in diagnostics from its interior.
//
// A descriptor that does not describe anything -- an array field with no Elem,
// a table field with no Table, a sub-descriptor on a kind that has no use for
// one -- is refused before the document is looked at.
//
// Validate reports nil when the document satisfies the descriptor.
func (d *Document) Validate(spec *Spec) error {
	_, err := d.checkSpec(spec)
	return err
}

// DecodeSpec checks the document against the descriptor and, when it satisfies
// it, returns the document's values as native Go data: map[string]any for every
// table spelling, []any for arrays and arrays of tables, and string, int64,
// float64, bool, time.Time, LocalDateTime, LocalDate or LocalTime for scalars.
//
// It is the descriptor path's decode: the same engine Validate runs, followed
// by a value built out of the read-layer. No reflection is involved and no
// consumer code runs, so the result is exactly what the document says.
//
// DecodeSpec is ATOMIC, without exception: it returns a map only when the
// document has no violations at all, and (nil, err) otherwise. There is no
// partial map, no half-populated table, and nothing to inspect after an error --
// the diagnostics are the whole answer. This is a stronger promise than the
// typed entry points make, and it holds because nothing outside this package
// participates in building the map.
//
// The errors are Validate's: an *Errors aggregate of every independent
// violation in document order, or a plain error for a descriptor that describes
// nothing.
func (d *Document) DecodeSpec(spec *Spec) (map[string]any, error) {
	root, err := d.checkSpec(spec)
	if err != nil {
		return nil, err
	}
	out, derr := nativeRecord(root)
	if derr != nil {
		return nil, d.diag(derr, "")
	}
	return out.(map[string]any), nil
}

// checkSpec walks the document against the descriptor and returns the read-layer
// it walked, so a caller that also wants the values does not fold twice.
func (d *Document) checkSpec(spec *Spec) (*Record, error) {
	table, err := compileSpec(spec)
	if err != nil {
		return nil, err
	}
	root, err := d.readLayer()
	if err != nil {
		return nil, d.diag(err, "")
	}
	en := newEngine()
	en.walkRecord(root, table, noValue, "")
	if err := d.diag(en.result(), ""); err != nil {
		return nil, err
	}
	return root, nil
}

// --- compilation ---
//
// A Spec is compiled into the same internal descriptor the reflection front
// end derives, so the engine has exactly one shape to walk.

// specCompiler compiles a descriptor, refusing the ones that describe nothing
// and following the pointers a recursive descriptor makes only once.
type specCompiler struct {
	tables map[*Spec]*tableDesc
	fields map[*Field]*desc
}

// compileSpec compiles a whole descriptor, or reports why it cannot be walked.
func compileSpec(spec *Spec) (*tableDesc, error) {
	if spec == nil {
		return nil, fmt.Errorf("tomledit: a nil *Spec describes nothing")
	}
	c := &specCompiler{tables: map[*Spec]*tableDesc{}, fields: map[*Field]*desc{}}
	return c.table(spec, "the descriptor")
}

// table compiles one Spec. where names the descriptor's position for the
// construction errors below.
func (c *specCompiler) table(spec *Spec, where string) (*tableDesc, error) {
	if td, ok := c.tables[spec]; ok {
		return td, nil
	}
	td := &tableDesc{fields: map[string]*fieldSlot{}}
	c.tables[spec] = td

	names := make([]string, 0, len(spec.Fields))
	for name := range spec.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		f := spec.Fields[name]
		d, err := c.field(f, fmt.Sprintf("field %q of %s", name, where))
		if err != nil {
			return nil, err
		}
		td.fields[name] = &fieldSlot{name: name, required: f.Required, desc: d}
		td.names = append(td.names, name)
	}
	if spec.Dynamic != nil {
		d, err := c.fieldPtr(spec.Dynamic, "the dynamic field of "+where)
		if err != nil {
			return nil, err
		}
		td.dynamic = &fieldSlot{desc: d}
	}
	return td, nil
}

// fieldPtr compiles a field reached through a pointer, following each pointer
// only once so a recursive descriptor terminates.
func (c *specCompiler) fieldPtr(f *Field, where string) (*desc, error) {
	if d, ok := c.fields[f]; ok {
		return d, nil
	}
	d := &desc{}
	c.fields[f] = d
	if err := c.fill(d, *f, where); err != nil {
		delete(c.fields, f)
		return nil, err
	}
	return d, nil
}

// field compiles a field held by value.
func (c *specCompiler) field(f Field, where string) (*desc, error) {
	d := &desc{}
	if err := c.fill(d, f, where); err != nil {
		return nil, err
	}
	return d, nil
}

// fill compiles f into d, refusing a field that carries the wrong
// sub-descriptors for its kind.
func (c *specCompiler) fill(d *desc, f Field, where string) error {
	if !f.Kind.valid() {
		return fmt.Errorf("tomledit: %s has an unknown field kind (%d)", where, int(f.Kind))
	}
	d.class = fieldKindClasses[f.Kind]
	d.label = f.Kind.String()

	if f.Elem != nil && f.Kind != FieldKindArray {
		return fmt.Errorf("tomledit: %s is a %s but carries an Elem descriptor, which only an array field uses", where, f.Kind)
	}
	if f.Table != nil && f.Kind != FieldKindTable {
		return fmt.Errorf("tomledit: %s is a %s but carries a Table descriptor, which only a table field uses", where, f.Kind)
	}

	switch f.Kind {
	case FieldKindArray:
		if f.Elem == nil {
			return fmt.Errorf("tomledit: %s is an array but carries no Elem descriptor", where)
		}
		elem, err := c.fieldPtr(f.Elem, "the element of "+where)
		if err != nil {
			return err
		}
		d.elem = elem
	case FieldKindTable:
		if f.Table == nil {
			return fmt.Errorf("tomledit: %s is a table but carries no Table descriptor", where)
		}
		table, err := c.table(f.Table, where)
		if err != nil {
			return err
		}
		d.table = table
	}
	return nil
}
