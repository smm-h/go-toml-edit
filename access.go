package tomledit

import (
	"reflect"
	"time"
)

// The typed accessors: one design across the three read surfaces -- the
// path-level getters on a Document, the As* family on a Scalar node, and the
// Cursor's terminals.
//
// A typed read runs three stages, and each stage owns its diagnostic kinds:
//
//  1. navigate -- find what the caller addressed. KindBadPath for a path that
//     does not parse, KindNotFound for one naming nothing, KindWrongContainer
//     for a step that does not apply to what it addresses and for a position no
//     single node stands for. The node-level family has no navigate stage: the
//     caller already holds the node.
//  2. type-check -- is this value's kind acceptable for the target? A refusal
//     is KindTypeMismatch, naming what the target accepts and what the document
//     carries.
//  3. convert -- does this particular value survive the trip? A refusal is
//     KindInexact, carrying the offending value.
//
// The last two stages are the decode engine's own: they read the same
// conversion table and run the same conversion function, so a value a decode
// target accepts is a value the matching accessor accepts, and neither surface
// restates the other's rules. The two rows that make this visible: a float
// target takes an integer it holds exactly, and a time.Time target takes the
// local date-time flavors as well as an offset date-time.

// asValue runs the type-check and convert stages for one node against the Go
// type T. It is the whole accessor family: every accessor on every surface is
// this function plus its own navigate stage.
func asValue[T any](n Node) (T, *Error) {
	var out T
	rv := reflect.ValueOf(&out).Elem()
	class, ok := targetClassOf(rv.Type())
	if !ok {
		// Unreachable through the accessor surfaces: every T they instantiate
		// names a row of the conversion table.
		return out, newError(KindTypeMismatch, "%s reads no document value", rv.Type())
	}
	expected := class.describeAccepted()

	kind, ok := valueKindOf(n)
	if !ok {
		return out, typeMismatch(n, expected, n.Type().String()+" node")
	}
	if !class.accepts(kind) {
		return out, typeMismatch(n, expected, kind.String())
	}
	if err := convertValue(n, class, rv); err != nil {
		return out, placeOn(n, err)
	}
	return out, nil
}

// typeMismatch reports a value the conversion table refuses for the target,
// positioned on the node it read.
func typeMismatch(n Node, expected, got string) *Error {
	return placeOn(n, newError(KindTypeMismatch, "expected %s, got %s", expected, got).expects(expected, got))
}

// placeOn positions an accessor diagnostic at the node it concerns, and over
// that node's own source range.
func placeOn(n Node, e *Error) *Error {
	span := n.Span()
	return e.at(span.Start).within(span)
}

// nodeAs is the node-level accessor: asValue, with the document's filename
// stamped on the diagnostic and a nil *Error returned as a nil error.
func nodeAs[T any](n Node) (T, error) {
	v, err := asValue[T](n)
	if err != nil {
		return v, documentOf(n).diag(err, "")
	}
	return v, nil
}

// getAs is the path-level accessor: navigate to the path, then read the node it
// names as a T.
func getAs[T any](d *Document, path string) (T, error) {
	var zero T
	node, err := d.Resolve(path)
	if err != nil {
		return zero, err
	}
	v, aerr := asValue[T](node)
	if aerr != nil {
		return zero, d.diag(aerr, path)
	}
	return v, nil
}

// cursorAs is the Cursor's terminal: report the navigation failure the chain
// already carries, then read the node the cursor stands on as a T. A position
// no single node stands for is a navigation failure of its own, and is recorded
// on the cursor so that Err reports it too.
func cursorAs[T any](c *Cursor) (T, error) {
	var zero T
	if c.err != nil {
		return zero, c.Err()
	}
	node, ok := c.pos.concrete()
	if !ok {
		c.err = c.pos.noNodeError()
		return zero, c.Err()
	}
	v, aerr := asValue[T](node)
	if aerr != nil {
		return zero, c.doc.diag(aerr, "")
	}
	return v, nil
}

// --- the path-level family ---
//
// Each getter mirrors the node-level As* accessor of the same name, with the
// navigate stage in front of it.

// GetString resolves the path and reads the value it names as a string.
//
// The error is the unified *Error: KindBadPath, KindNotFound or
// KindWrongContainer from the navigation, and KindTypeMismatch when the value
// is not a string.
func (d *Document) GetString(path string) (string, error) { return getAs[string](d, path) }

// GetInt resolves the path and reads the value it names as an int64.
//
// The error is the unified *Error: KindBadPath, KindNotFound or
// KindWrongContainer from the navigation, and KindTypeMismatch when the value
// is not an integer. A float is never an integer, however whole it is written.
func (d *Document) GetInt(path string) (int64, error) { return getAs[int64](d, path) }

// GetBool resolves the path and reads the value it names as a bool.
//
// The error is the unified *Error: KindBadPath, KindNotFound or
// KindWrongContainer from the navigation, and KindTypeMismatch when the value
// is not a boolean.
func (d *Document) GetBool(path string) (bool, error) { return getAs[bool](d, path) }

// GetFloat resolves the path and reads the value it names as a float64: a
// float verbatim, and an integer the target holds exactly.
//
// The error is the unified *Error: KindBadPath, KindNotFound or
// KindWrongContainer from the navigation, KindTypeMismatch when the value is
// neither a float nor an integer, and KindInexact for an integer no float64
// holds exactly.
func (d *Document) GetFloat(path string) (float64, error) { return getAs[float64](d, path) }

// GetTime resolves the path and reads the value it names as a time.Time: an
// offset date-time verbatim, and a local date-time or local date read as UTC,
// because a time.Time target declares that intent. A local time carries no date
// and is refused.
//
// The error is the unified *Error: KindBadPath, KindNotFound or
// KindWrongContainer from the navigation, and KindTypeMismatch when the value
// is not one of the three date-time flavors above.
func (d *Document) GetTime(path string) (time.Time, error) { return getAs[time.Time](d, path) }
