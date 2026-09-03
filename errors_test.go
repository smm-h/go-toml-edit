package tomledit

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// The diagnostic contract: one error type, matched by kind through errors.Is
// and structurally through errors.As, and an aggregate that renders as its
// first diagnostic while exposing all of them.

// Fails if the rendering of a diagnostic stops including the file, position
// and path it carries, or starts including them when they are absent.
func TestErrorRendering(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "position only",
			err:  &Error{Kind: KindSyntax, Pos: Position{Line: 3, Column: 10, Offset: 42}, Message: "unexpected character"},
			want: "line 3, column 10: unexpected character",
		},
		{
			name: "message only",
			err:  &Error{Kind: KindNotFound, Message: `key "host" not found`},
			want: `key "host" not found`,
		},
		{
			name: "path and message",
			err:  &Error{Kind: KindNotFound, Path: "server.host", Message: "key not found"},
			want: "server.host: key not found",
		},
		{
			name: "file, position, path and message",
			err: &Error{
				Kind: KindSyntax, File: "config.toml", Path: "server.host",
				Pos: Position{Line: 2, Column: 1, Offset: 6}, Message: "duplicate key",
			},
			want: "config.toml: line 2, column 1: server.host: duplicate key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Fails if a diagnostic stops matching the sentinel of its own kind, starts
// matching another kind's sentinel, or stops being reachable through a
// wrapping chain.
func TestErrorIsAndAs(t *testing.T) {
	diag := &Error{Kind: KindNotFound, Message: "nothing here"}

	if !errors.Is(diag, ErrNotFound) {
		t.Errorf("errors.Is(diag, ErrNotFound) = false, want true")
	}
	if errors.Is(diag, ErrSyntax) {
		t.Errorf("errors.Is(diag, ErrSyntax) = true, want false")
	}

	wrapped := fmt.Errorf("reading config: %w", diag)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Errorf("errors.Is through a wrapping chain = false, want true")
	}
	var got *Error
	if !errors.As(wrapped, &got) {
		t.Fatalf("errors.As through a wrapping chain failed")
	}
	if got != diag {
		t.Errorf("errors.As yielded %v, want the original diagnostic", got)
	}
}

// Fails if a diagnostic stops carrying the error it reports, so that a caller
// can no longer reach the underlying failure.
func TestErrorUnwrapsItsCause(t *testing.T) {
	cause := errors.New("invalid escape sequence")
	diag := newError(KindSyntax, "%s", cause.Error()).wrapping(cause)
	if !errors.Is(diag, cause) {
		t.Errorf("errors.Is(diag, cause) = false, want true")
	}
	if diag.Unwrap() != cause {
		t.Errorf("Unwrap() = %v, want the cause", diag.Unwrap())
	}
}

// Fails if the aggregate stops rendering as its first diagnostic, stops
// exposing every diagnostic in document order, or starts being returned empty.
func TestErrorsAggregateContract(t *testing.T) {
	first := &Error{Kind: KindUnknownKey, Path: "a", Message: "unknown key"}
	second := &Error{Kind: KindMissingKey, Path: "b", Message: "missing key"}
	err := newErrors([]*Error{first, second})
	if err == nil {
		t.Fatalf("newErrors returned nil for a non-empty list")
	}

	if got, want := err.Error(), first.Error(); got != want {
		t.Errorf("Error() = %q, want the first diagnostic %q", got, want)
	}

	var agg *Errors
	if !errors.As(err, &agg) {
		t.Fatalf("errors.As with an *Errors target failed")
	}
	all := agg.Unwrap()
	if len(all) != 2 || all[0] != error(first) || all[1] != error(second) {
		t.Errorf("Unwrap() = %v, want the diagnostics in document order", all)
	}

	var single *Error
	if !errors.As(err, &single) {
		t.Fatalf("errors.As with an *Error target failed")
	}
	if single != first {
		t.Errorf("errors.As yielded %v, want the first diagnostic", single)
	}

	if !errors.Is(err, ErrUnknownKey) || !errors.Is(err, ErrMissingKey) {
		t.Errorf("errors.Is must match the kind of any contained diagnostic")
	}
	if errors.Is(err, ErrSyntax) {
		t.Errorf("errors.Is matched a kind no contained diagnostic carries")
	}

	if got := newErrors(nil); got != nil {
		t.Errorf("newErrors(nil) = %v, want nil: an empty aggregate is never returned", got)
	}
}

// Fails if any error-producing surface stops reporting the unified
// diagnostic: one representative failure per surface, matched by kind through
// errors.Is and structurally through errors.As, as the documented contract
// promises.
func TestDiagnosticContractAcrossSurfaces(t *testing.T) {
	const src = `title = "hello"
tags = [1, 2]
point = { x = 1 }

[server]
host = "localhost"

[[products]]
name = "a"
`
	// wantPath is the path the diagnostic must name; "" means the surface
	// reports no path.
	tests := []struct {
		name     string
		run      func(d *Document) error
		wantKind ErrorKind
		wantSent error
		wantPath string
	}{
		{
			name:     "parse: syntax",
			run:      func(*Document) error { _, err := Parse([]byte("a = = 1\n")); return err },
			wantKind: KindSyntax,
			wantSent: ErrSyntax,
		},
		{
			name:     "resolve: bad path",
			run:      func(d *Document) error { _, err := d.Resolve("tags["); return err },
			wantKind: KindBadPath,
			wantSent: ErrBadPath,
			wantPath: "tags[",
		},
		{
			name:     "resolve: not found",
			run:      func(d *Document) error { _, err := d.Resolve("server.port"); return err },
			wantKind: KindNotFound,
			wantSent: ErrNotFound,
			wantPath: "server.port",
		},
		{
			name:     "resolve: index out of range",
			run:      func(d *Document) error { _, err := d.Resolve("tags[9]"); return err },
			wantKind: KindNotFound,
			wantSent: ErrNotFound,
			wantPath: "tags[9]",
		},
		{
			name:     "resolve: wrong container",
			run:      func(d *Document) error { _, err := d.Resolve("title.nested"); return err },
			wantKind: KindWrongContainer,
			wantSent: ErrWrongContainer,
			wantPath: "title.nested",
		},
		{
			name:     "resolve: key on an array-of-tables collection",
			run:      func(d *Document) error { _, err := d.Resolve("products.name"); return err },
			wantKind: KindWrongContainer,
			wantSent: ErrWrongContainer,
			wantPath: "products.name",
		},
		{
			name:     "set: unsupported value",
			run:      func(d *Document) error { return d.Set("title", make(chan int)) },
			wantKind: KindBadInput,
			wantSent: ErrBadInput,
			wantPath: "title",
		},
		{
			name:     "set: missing parent",
			run:      func(d *Document) error { return d.Set("nope.deep", 1) },
			wantKind: KindNotFound,
			wantSent: ErrNotFound,
			wantPath: "nope.deep",
		},
		{
			name:     "rename: missing key",
			run:      func(d *Document) error { return d.RenameKey("server.port", "p") },
			wantKind: KindNotFound,
			wantSent: ErrNotFound,
			wantPath: "server.port",
		},
		{
			name:     "rename: existing sibling",
			run:      func(d *Document) error { return d.RenameKey("title", "tags") },
			wantKind: KindConflict,
			wantSent: ErrConflict,
			wantPath: "title",
		},
		{
			name:     "rename: an array index",
			run:      func(d *Document) error { return d.RenameKey("tags[0]", "x") },
			wantKind: KindWrongContainer,
			wantSent: ErrWrongContainer,
			wantPath: "tags[0]",
		},
		{
			name:     "new table: already exists",
			run:      func(d *Document) error { return d.NewTable("server") },
			wantKind: KindConflict,
			wantSent: ErrConflict,
			wantPath: "server",
		},
		{
			name:     "new table: index in path",
			run:      func(d *Document) error { return d.NewTable("a[0]") },
			wantKind: KindBadPath,
			wantSent: ErrBadPath,
			wantPath: "a[0]",
		},
		{
			name:     "delete: bad path",
			run:      func(d *Document) error { return d.Delete("a[") },
			wantKind: KindBadPath,
			wantSent: ErrBadPath,
			wantPath: "a[",
		},
		{
			name:     "comment: inside an inline table",
			run:      func(d *Document) error { return d.SetComment("point.x", "no") },
			wantKind: KindConflict,
			wantSent: ErrConflict,
			wantPath: "point.x",
		},
		{
			name:     "comment: missing path",
			run:      func(d *Document) error { return d.SetLeadingComments("server.port", []string{"x"}) },
			wantKind: KindNotFound,
			wantSent: ErrNotFound,
			wantPath: "server.port",
		},
		{
			name:     "cursor: missing key",
			run:      func(d *Document) error { return d.Key("server").Key("port").Err() },
			wantKind: KindNotFound,
			wantSent: ErrNotFound,
		},
		{
			name:     "cursor: index on a scalar",
			run:      func(d *Document) error { return d.Key("title").At(0).Err() },
			wantKind: KindWrongContainer,
			wantSent: ErrWrongContainer,
		},
		{
			name: "merge defaults: unsupported value",
			run: func(d *Document) error {
				return d.MergeDefaults("", map[string]any{"fresh": make(chan int)})
			},
			wantKind: KindBadInput,
			wantSent: ErrBadInput,
			wantPath: "fresh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			err = tt.run(doc)
			if err == nil {
				t.Fatalf("expected an error")
			}
			if !errors.Is(err, tt.wantSent) {
				t.Errorf("errors.Is(err, %v) = false, want true (err: %v)", tt.wantSent, err)
			}
			var diag *Error
			if !errors.As(err, &diag) {
				t.Fatalf("errors.As(*Error) failed for %T: %v", err, err)
			}
			if diag.Kind != tt.wantKind {
				t.Errorf("kind = %v, want %v (err: %v)", diag.Kind, tt.wantKind, err)
			}
			if diag.Message == "" {
				t.Errorf("diagnostic carries no message")
			}
			if tt.wantPath != "" && diag.Path != tt.wantPath {
				t.Errorf("path = %q, want %q", diag.Path, tt.wantPath)
			}
		})
	}
}

// Fails if a parse-stage diagnostic stops carrying a position, a byte offset
// that agrees with it, or the source line it sits on -- the three things every
// error site in the lexer and parser is required to fill.
func TestParseDiagnosticsCarryPositionAndSnippet(t *testing.T) {
	tests := []struct {
		name  string
		input string
		stage string // which stage reports it, for the failure message
	}{
		{"unterminated string", "a = \"hello\n", "lexer"},
		{"invalid character", "a = @\n", "lexer"},
		{"unclosed array", "a = [1, 2", "parser"},
		{"unclosed inline table", "a = {b = 1", "parser"},
		{"unclosed table header", "[foo", "parser"},
		{"missing value", "a =\n", "parser"},
		{"missing equals", "a 1\n", "parser"},
		{"bad integer", "a = 0x\n", "parser"},
		{"duplicate key", "a = 1\na = 2\n", "tracker"},
		{"duplicate table", "[foo]\n[foo]\n", "tracker"},
		{"table over key", "a = 1\n[a]\n", "tracker"},
		{"table over array table", "[[p]]\n[p]\n", "tracker"},
		{"dotted key through a value", "a = 1\na.b = 2\n", "tracker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := []byte(tt.input)
			_, err := Parse(src)
			if err == nil {
				t.Fatalf("%s: expected a parse error for %q", tt.stage, tt.input)
			}
			var diag *Error
			if !errors.As(err, &diag) {
				t.Fatalf("%s: expected *Error, got %T: %v", tt.stage, err, err)
			}
			if diag.Kind != KindSyntax {
				t.Errorf("kind = %v, want %v", diag.Kind, KindSyntax)
			}
			if !diag.Pos.IsValid() {
				t.Fatalf("position is invalid: %+v", diag.Pos)
			}
			if diag.Pos.Offset < 0 || diag.Pos.Offset > len(src) {
				t.Fatalf("offset %d outside the %d-byte source", diag.Pos.Offset, len(src))
			}
			if want := offsetOfLineColumn(src, diag.Pos.Line, diag.Pos.Column); diag.Pos.Offset != want {
				t.Errorf("offset = %d, want %d (line %d, column %d)",
					diag.Pos.Offset, want, diag.Pos.Line, diag.Pos.Column)
			}
			if diag.Snippet == "" {
				t.Errorf("snippet is empty")
			}
			if line := sourceLineAt(src, diag.Pos.Offset); diag.Snippet != line {
				t.Errorf("snippet = %q, want the source line %q", diag.Snippet, line)
			}
		})
	}
}

// sourceLineAt returns the whole source line containing offset.
func sourceLineAt(src []byte, offset int) string {
	if offset < 0 || offset > len(src) {
		return ""
	}
	start, end := offset, offset
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	for end < len(src) && src[end] != '\n' {
		end++
	}
	return string(src[start:end])
}

// Fails if a parser-stage diagnostic stops carrying the span of the construct
// it concerns.
func TestParserDiagnosticCarriesSpan(t *testing.T) {
	src := []byte("a = 1\nb 2\n")
	_, err := Parse(src)
	if err == nil {
		t.Fatalf("expected a parse error")
	}
	var diag *Error
	if !errors.As(err, &diag) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if !diag.Span.IsValid() {
		t.Fatalf("span is invalid: %+v", diag.Span)
	}
	if got := string(src[diag.Span.Start.Offset:diag.Span.End.Offset]); got != "2" {
		t.Errorf("span slices %q, want the offending token %q", got, "2")
	}
}

// Fails if a long source line stops being truncated in the snippet, letting a
// diagnostic quote an unbounded amount of source.
func TestSnippetIsTruncated(t *testing.T) {
	long := "a = " + strings.Repeat("1", 200) + "x\n"
	_, err := Parse([]byte(long))
	if err == nil {
		t.Fatalf("expected a parse error")
	}
	var diag *Error
	if !errors.As(err, &diag) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if len(diag.Snippet) != snippetLimit {
		t.Errorf("snippet length = %d, want %d", len(diag.Snippet), snippetLimit)
	}
}
