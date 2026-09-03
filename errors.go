package tomledit

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorKind classifies a diagnostic. The set is closed: every kind has exactly
// one matching sentinel (KindSyntax has ErrSyntax, and so on), and a
// diagnostic reports itself equal to the sentinel of its own kind through
// errors.Is.
type ErrorKind int

const (
	// KindSyntax is a lexing or parsing failure.
	KindSyntax ErrorKind = iota
	// KindUnknownKey is a decoded key matching no field of the target.
	KindUnknownKey
	// KindUnknownTable is a decoded table or array-of-tables matching no
	// field of the target; the diagnostic's Keys field lists the direct
	// child keys of the offending construct.
	KindUnknownTable
	// KindMissingKey is a required key that the document does not carry.
	KindMissingKey
	// KindTypeMismatch is a value whose kind is not acceptable for the
	// target; the diagnostic's Expected and Got fields name both sides.
	KindTypeMismatch
	// KindInexact is a value the target cannot hold exactly: an out-of-range
	// integer, a lossy conversion, or a length mismatch. The diagnostic's
	// Value field carries the offending value.
	KindInexact
	// KindNotFound is a path naming nothing in the document.
	KindNotFound
	// KindBadPath is a syntactically invalid path.
	KindBadPath
	// KindWrongContainer is a path step that is structurally inapplicable:
	// a key on an array, an index on a scalar, a concrete-node operation on
	// a path that names no single node.
	KindWrongContainer
	// KindBadInput is an invalid input value to an editing operation.
	KindBadInput
	// KindConflict is an edit refused because it would produce an invalid
	// document.
	KindConflict
	// KindRoundTrip is a write whose rendered bytes did not survive a
	// re-parse; the diagnostic's Offset field carries the byte offset of the
	// first divergence.
	KindRoundTrip
)

var errorKindNames = [...]string{
	KindSyntax:         "syntax error",
	KindUnknownKey:     "unknown key",
	KindUnknownTable:   "unknown table",
	KindMissingKey:     "missing key",
	KindTypeMismatch:   "type mismatch",
	KindInexact:        "inexact value",
	KindNotFound:       "not found",
	KindBadPath:        "bad path",
	KindWrongContainer: "wrong container",
	KindBadInput:       "bad input",
	KindConflict:       "conflict",
	KindRoundTrip:      "round-trip failure",
}

// String returns the human-readable name of the kind.
func (k ErrorKind) String() string {
	if int(k) >= 0 && int(k) < len(errorKindNames) {
		return errorKindNames[k]
	}
	return fmt.Sprintf("ErrorKind(%d)", int(k))
}

// kindError is the type of the per-kind sentinels below. It is unexported so
// that the sentinels can only be compared, never constructed or switched on by
// a consumer: matching is errors.Is against the sentinel.
type kindError ErrorKind

func (k kindError) Error() string { return "tomledit: " + ErrorKind(k).String() }

// The kind sentinels. Match a diagnostic against one with errors.Is:
//
//	if errors.Is(err, tomledit.ErrNotFound) { ... }
//
// Every ErrorKind has exactly one sentinel and every sentinel has exactly one
// kind; TestErrorKindSentinelDrift fails when one is added without the other.
var (
	ErrSyntax         error = kindError(KindSyntax)         // a lexing or parsing failure
	ErrUnknownKey     error = kindError(KindUnknownKey)     // a key matching no field of the target
	ErrUnknownTable   error = kindError(KindUnknownTable)   // a table matching no field of the target
	ErrMissingKey     error = kindError(KindMissingKey)     // a required key the document does not carry
	ErrTypeMismatch   error = kindError(KindTypeMismatch)   // a value whose kind the target refuses
	ErrInexact        error = kindError(KindInexact)        // a value the target cannot hold exactly
	ErrNotFound       error = kindError(KindNotFound)       // a path naming nothing
	ErrBadPath        error = kindError(KindBadPath)        // a syntactically invalid path
	ErrWrongContainer error = kindError(KindWrongContainer) // a structurally inapplicable path step
	ErrBadInput       error = kindError(KindBadInput)       // an invalid input to an editing operation
	ErrConflict       error = kindError(KindConflict)       // an edit that would produce an invalid document
	ErrRoundTrip      error = kindError(KindRoundTrip)      // rendered bytes that did not survive a re-parse
)

// Error is the one diagnostic type of this package. Parse, edit, and access
// failures are all reported as *Error, so a caller can match them structurally
// (errors.As) or by kind (errors.Is against the matching Err* sentinel)
// instead of by message text:
//
//	var diag *tomledit.Error
//	if errors.As(err, &diag) {
//	    fmt.Println(diag.Kind, diag.Pos.Line, diag.Pos.Column)
//	}
//
// Which fields are populated depends on the kind and on what the reporting
// site knew: Pos, Span and Snippet are filled for parse-stage diagnostics,
// Path for path-addressed operations, File whenever the document's origin is
// known (see ParseFile), and the remaining fields by the kinds documented on
// ErrorKind. An unpopulated field carries its zero value.
type Error struct {
	Kind     ErrorKind // what went wrong
	Path     string    // document path, in this package's path syntax
	Pos      Position  // primary source position; the zero Position when unknown
	Span     Span      // source range of the construct concerned, when known
	Message  string    // human-readable description, without position or file
	File     string    // source file, empty when no file is known
	Snippet  string    // source excerpt, parse-stage diagnostics
	Expected string    // type-mismatch detail: what the target accepts
	Got      string    // type-mismatch detail: what the document carries
	Value    any       // the offending value, for inexact and bad-input kinds
	Keys     []string  // the direct child keys, for unknown-table diagnostics
	Offset   int       // KindRoundTrip: first divergence in the rendered bytes

	err error // the underlying error this diagnostic reports, if any
}

// Error renders the diagnostic: the file and position when they are known,
// then the path, then the message.
func (e *Error) Error() string {
	var b strings.Builder
	if e.File != "" {
		b.WriteString(e.File)
		b.WriteString(": ")
	}
	if e.Pos.IsValid() {
		fmt.Fprintf(&b, "line %d, column %d: ", e.Pos.Line, e.Pos.Column)
	}
	if e.Path != "" {
		b.WriteString(e.Path)
		b.WriteString(": ")
	}
	b.WriteString(e.Message)
	return b.String()
}

// Unwrap returns the underlying error this diagnostic reports, or nil.
func (e *Error) Unwrap() error { return e.err }

// Is reports whether the diagnostic matches target, which is true for the kind
// sentinel of its own kind.
func (e *Error) Is(target error) bool {
	k, ok := target.(kindError)
	return ok && ErrorKind(k) == e.Kind
}

// Errors is an aggregate of diagnostics reported together, in document order.
// It renders as its first diagnostic, so a call site that prints the error
// reads like a single failure; the whole list is reachable through
// errors.Unwrap semantics:
//
//	var all *tomledit.Errors
//	if errors.As(err, &all) {
//	    for _, d := range all.Unwrap() { ... }
//	}
//
// errors.As with an *Error target yields the first diagnostic, and errors.Is
// against a kind sentinel matches when any contained diagnostic carries that
// kind. An empty aggregate is never returned: no diagnostics means a nil error.
type Errors struct {
	diags []*Error
}

// Error renders the first diagnostic of the aggregate.
func (e *Errors) Error() string { return e.diags[0].Error() }

// Unwrap returns every diagnostic of the aggregate, in document order.
func (e *Errors) Unwrap() []error {
	out := make([]error, len(e.diags))
	for i, d := range e.diags {
		out[i] = d
	}
	return out
}

// --- construction ---
//
// Every *Error in this package is built here. Error sites call newError (and
// the builders below) rather than composing the struct themselves, so that a
// new site cannot forget the fields its kind is documented to carry;
// TestDiagnosticsBuiltInOneFile fails on a diagnostic composite literal
// anywhere else.

// newError constructs a diagnostic of the given kind with a formatted message.
func newError(kind ErrorKind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// syntaxErrorAt constructs a parse-stage diagnostic positioned at pos, with
// its snippet taken from src.
func syntaxErrorAt(src []byte, pos Position, format string, args ...any) *Error {
	return newError(KindSyntax, format, args...).at(pos).inSource(src)
}

// newErrors returns the diagnostics as one aggregate, or nil when there are
// none.
func newErrors(diags []*Error) error {
	if len(diags) == 0 {
		return nil
	}
	return &Errors{diags: diags}
}

// at records the primary source position of the diagnostic.
func (e *Error) at(pos Position) *Error {
	e.Pos = pos
	return e
}

// within records the source range of the construct the diagnostic concerns.
func (e *Error) within(span Span) *Error {
	e.Span = span
	return e
}

// inSource fills the snippet from the source line containing the diagnostic's
// position.
func (e *Error) inSource(src []byte) *Error {
	e.Snippet = snippetAt(src, e.Pos.Offset)
	return e
}

// atPath records the document path the failing operation addressed, unless the
// diagnostic already names one.
func (e *Error) atPath(path string) *Error {
	if e.Path == "" {
		e.Path = path
	}
	return e
}

// inFile records the source file, unless the diagnostic already names one.
func (e *Error) inFile(file string) *Error {
	if e.File == "" {
		e.File = file
	}
	return e
}

// withValue records the offending value of a bad-input or inexact diagnostic.
func (e *Error) withValue(v any) *Error {
	e.Value = v
	return e
}

// wrapping records the underlying error the diagnostic reports, so that
// errors.Is and errors.As reach it.
func (e *Error) wrapping(err error) *Error {
	e.err = err
	return e
}

// wrapError adds a context prefix to an error's message. When err is, or
// wraps, a diagnostic, the result is that diagnostic with the prefixed message
// -- same kind, position, span, path and file -- reporting the original;
// otherwise the result is an ordinary wrapped error.
func wrapError(err error, format string, args ...any) error {
	prefix := fmt.Sprintf(format, args...)
	var diag *Error
	if errors.As(err, &diag) {
		out := *diag
		out.Message = prefix + ": " + diag.Message
		out.err = err
		return &out
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

// walkDiagnostics calls fn for every diagnostic reachable from err, following
// both unwrapping forms.
func walkDiagnostics(err error, fn func(*Error)) {
	switch e := err.(type) {
	case *Error:
		if e == nil {
			return
		}
		fn(e)
		walkDiagnostics(e.err, fn)
	case interface{ Unwrap() error }:
		walkDiagnostics(e.Unwrap(), fn)
	case interface{ Unwrap() []error }:
		for _, sub := range e.Unwrap() {
			walkDiagnostics(sub, fn)
		}
	}
}

// stampPath records path on every diagnostic reachable from err that does not
// already name one, and returns err. Public entry points use it so that a
// diagnostic reported from deep inside a resolution still names the path the
// caller asked for.
func stampPath(err error, path string) error {
	if err == nil || path == "" {
		return err
	}
	walkDiagnostics(err, func(d *Error) { d.atPath(path) })
	return err
}

// stampFile records file on every diagnostic reachable from err that does not
// already name one, and returns err. It is how a document read from a file
// makes its diagnostics name that file.
func stampFile(err error, file string) error {
	if err == nil || file == "" {
		return err
	}
	walkDiagnostics(err, func(d *Error) { d.inFile(file) })
	return err
}

// diag records the document's origin -- the file it was read from, and the
// path the caller addressed -- on every diagnostic in err. Public methods
// return through it, so a diagnostic reported anywhere below them names both.
func (d *Document) diag(err error, path string) error {
	if err == nil {
		return err
	}
	err = stampPath(err, path)
	if d != nil {
		err = stampFile(err, d.file)
	}
	return err
}

// snippetLimit caps how many bytes of a source line a diagnostic quotes.
const snippetLimit = 60

// snippetAt returns the source line containing offset, truncated to
// snippetLimit bytes. It returns "" when offset falls outside src.
//
// A diagnostic reported at the end of a newline-terminated source sits on the
// empty line after the last newline, where quoting "the line containing the
// offset" would quote nothing; such an offset quotes the last non-empty line
// instead, which is the line the unfinished construct opened on.
func snippetAt(src []byte, offset int) string {
	if offset < 0 || offset > len(src) {
		return ""
	}
	end := offset
	if end == len(src) {
		// Step back over the empty line a newline-terminated source ends on,
		// and over any blank lines before it, to the last line with content.
		for end > 0 && src[end-1] == '\n' {
			end--
		}
	}
	start := end
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	for end < len(src) && src[end] != '\n' {
		end++
	}
	snippet := string(src[start:end])
	if len(snippet) > snippetLimit {
		snippet = snippet[:snippetLimit]
	}
	return snippet
}
