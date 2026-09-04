package tomledit

import (
	"strings"
	"testing"
)

// =============================================================================
// 1. Basic formatting: messy input -> clean output
// =============================================================================

func TestFormatBasicFormatting(t *testing.T) {
	input := `host="localhost"
port  =  8080
[server]
debug=true
[server.database]
name="mydb"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	// Check consistent spacing.
	if !strings.Contains(got, `host = "localhost"`) {
		t.Errorf("expected normalized spacing for host, got:\n%s", got)
	}
	if !strings.Contains(got, "port = 8080") {
		t.Errorf("expected normalized spacing for port, got:\n%s", got)
	}
	if !strings.Contains(got, "debug = true") {
		t.Errorf("expected normalized spacing for debug, got:\n%s", got)
	}
	if !strings.Contains(got, `name = "mydb"`) {
		t.Errorf("expected normalized spacing for name, got:\n%s", got)
	}
	// Blank line before [server] table (comes after top-level KVs).
	if !strings.Contains(got, "port = 8080\n\n[server]") {
		t.Errorf("expected blank line before [server], got:\n%s", got)
	}
	// Blank line before [server.database].
	if !strings.Contains(got, "debug = true\n\n[server.database]") {
		t.Errorf("expected blank line before [server.database], got:\n%s", got)
	}
	// Table headers should be normalized.
	if !strings.Contains(got, "[server]") {
		t.Errorf("expected [server] header, got:\n%s", got)
	}
	if !strings.Contains(got, "[server.database]") {
		t.Errorf("expected [server.database] header, got:\n%s", got)
	}
}

// =============================================================================
// 2. Comment preservation
// =============================================================================

func TestFormatCommentPreservation(t *testing.T) {
	input := `# file header
host = "localhost" # primary host
# standalone comment
port = 8080
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "# file header") {
		t.Errorf("expected leading comment preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "# primary host") {
		t.Errorf("expected inline comment preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "# standalone comment") {
		t.Errorf("expected standalone comment preserved, got:\n%s", got)
	}
}

// =============================================================================
// 3. Trailing whitespace removal
// =============================================================================

func TestFormatTrailingWhitespaceRemoval(t *testing.T) {
	// Input has trailing spaces on lines.
	input := "host = \"localhost\"   \nport = 8080\t\t\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	lines := strings.Split(got, "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if line != trimmed {
			t.Errorf("line %d has trailing whitespace: %q", i+1, line)
		}
	}
}

// =============================================================================
// 4. Multi-line array formatting (long array exceeds line width)
// =============================================================================

func TestFormatMultiLineArray(t *testing.T) {
	input := `items = ["first-long-element", "second-long-element", "third-long-element", "fourth-long-element"]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// Use a smaller line width to force multi-line.
	got := string(doc.Format(WithLineWidth(40)))
	if !strings.Contains(got, "[\n") {
		t.Errorf("expected multi-line array, got:\n%s", got)
	}
	if !strings.Contains(got, `"first-long-element",`) {
		t.Errorf("expected trailing comma on elements, got:\n%s", got)
	}
	// Should end the array with ]
	if !strings.Contains(got, "]") {
		t.Errorf("expected closing bracket, got:\n%s", got)
	}
}

// =============================================================================
// 5. Short array stays inline
// =============================================================================

func TestFormatShortArrayStaysInline(t *testing.T) {
	input := "a = [1, 2, 3]\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if strings.Contains(got, "[\n") {
		t.Errorf("short array should stay inline, got:\n%s", got)
	}
	if !strings.Contains(got, "a = [1, 2, 3]") {
		t.Errorf("expected inline array, got:\n%s", got)
	}
}

// =============================================================================
// 6. Inline table formatting
// =============================================================================

func TestFormatInlineTable(t *testing.T) {
	input := "t = {  x=1,   y  = 2  }\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "{x = 1, y = 2}") {
		t.Errorf("expected normalized inline table, got:\n%s", got)
	}
}

// =============================================================================
// 7. Table header normalization
// =============================================================================

func TestFormatTableHeaderNormalization(t *testing.T) {
	input := "[ server . database ]\nname = \"mydb\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "[server.database]") {
		t.Errorf("expected normalized table header, got:\n%s", got)
	}
	// Should NOT contain spaces inside brackets.
	if strings.Contains(got, "[ ") || strings.Contains(got, " ]") {
		t.Errorf("table header should not have spaces inside brackets, got:\n%s", got)
	}
}

// =============================================================================
// 8. Multiple blank lines collapsed
// =============================================================================

// Fails if a run of blank lines stops collapsing to exactly one, in either
// direction: several surviving, or the whole run disappearing.
func TestFormatMultipleBlankLinesCollapsed(t *testing.T) {
	input := "[a]\nx = 1\n\n\n\n[b]\ny = 2\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	const want = "[a]\nx = 1\n\n[b]\ny = 2\n"
	if got := string(doc.Format()); got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
	// The option is not what puts the blank line there: with it off the run
	// still collapses to one rather than vanishing.
	if got := string(doc.Format(WithTableBlankLine(false))); got != want {
		t.Errorf("Format(WithTableBlankLine(false)) = %q, want %q", got, want)
	}
}

// =============================================================================
// 9. Trailing newline
// =============================================================================

func TestFormatTrailingNewline(t *testing.T) {
	input := "a = 1"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("expected trailing newline, got: %q", got)
	}
	// Should end with exactly one newline, not two.
	if strings.HasSuffix(got, "\n\n") {
		t.Errorf("expected exactly one trailing newline, got: %q", got)
	}
}

// =============================================================================
// 10. Format does NOT mutate document
// =============================================================================

func TestFormatDoesNotMutateDocument(t *testing.T) {
	input := "host=\"localhost\"\nport  =  8080\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Capture original Bytes() output.
	originalBytes := string(doc.Bytes())

	// Call Format (should not mutate).
	_ = doc.Format()

	// Bytes() should still return the original.
	afterBytes := string(doc.Bytes())
	if afterBytes != originalBytes {
		t.Errorf("Format() mutated the document:\n  before: %q\n  after:  %q", originalBytes, afterBytes)
	}
}

// =============================================================================
// 11. FormatConfig options
// =============================================================================

func TestFormatWithLineWidth(t *testing.T) {
	input := `items = ["alpha", "beta", "gamma", "delta"]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// With default width (80), should stay inline.
	got80 := string(doc.Format(WithLineWidth(80)))
	if strings.Contains(got80, "[\n") {
		t.Errorf("with line width 80, array should be inline, got:\n%s", got80)
	}

	// With narrow width, should go multi-line.
	got20 := string(doc.Format(WithLineWidth(20)))
	if !strings.Contains(got20, "[\n") {
		t.Errorf("with line width 20, array should be multi-line, got:\n%s", got20)
	}
}

// The option is insertion-only: turning it off stops the formatter from adding
// a blank line where the writer left none, and does nothing to the blank lines
// the document already carries.
//
// Fails if the option ever starts removing blank lines, or stops inserting one
// before a table written flush against what precedes it.
func TestFormatWithTableBlankLineFalse(t *testing.T) {
	// No blank line written: with the option off none is inserted.
	doc, err := Parse([]byte("a = 1\n[server]\nhost = \"localhost\"\n"))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format(WithTableBlankLine(false)))
	if strings.Contains(got, "\n\n") {
		t.Errorf("with TableBlankLine=false, expected no blank line to be inserted, got:\n%s", got)
	}

	// A blank line written: the option off leaves it standing.
	doc, err = Parse([]byte("a = 1\n\n[server]\nhost = \"localhost\"\n"))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got = string(doc.Format(WithTableBlankLine(false)))
	if !strings.Contains(got, "a = 1\n\n[server]") {
		t.Errorf("with TableBlankLine=false, the document's own blank line should survive, got:\n%s", got)
	}
}

func TestFormatWithIndentWidth(t *testing.T) {
	input := "[server]\nhost = \"localhost\"\nport = 8080\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format(WithIndentWidth(4)))
	if !strings.Contains(got, "    host") {
		t.Errorf("with indent width 4, expected 4-space indent, got:\n%s", got)
	}
	if !strings.Contains(got, "    port") {
		t.Errorf("with indent width 4, expected 4-space indent on port, got:\n%s", got)
	}
}

// =============================================================================
// 12. Default config
// =============================================================================

func TestFormatDefaultConfig(t *testing.T) {
	input := "a = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// Format with no options should use defaults.
	got := string(doc.Format())
	if !strings.Contains(got, "a = 1") {
		t.Errorf("default format should produce clean output, got:\n%s", got)
	}
}

func TestFormatDefaultConfigValues(t *testing.T) {
	cfg := DefaultFormatConfig()
	if cfg.IndentWidth != 0 {
		t.Errorf("default IndentWidth = %d, want 0", cfg.IndentWidth)
	}
	if cfg.LineWidth != 80 {
		t.Errorf("default LineWidth = %d, want 80", cfg.LineWidth)
	}
	if !cfg.TableBlankLine {
		t.Error("default TableBlankLine should be true")
	}
}

// =============================================================================
// 13. Empty document
// =============================================================================

func TestFormatEmptyDocument(t *testing.T) {
	doc := &Document{}
	got := string(doc.Format())
	if got != "\n" {
		t.Errorf("empty document format should be single newline, got %q", got)
	}
}

func TestFormatEmptyDocumentParsed(t *testing.T) {
	doc, err := Parse([]byte(""))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if got != "\n" {
		t.Errorf("empty parsed document format should be single newline, got %q", got)
	}
}

// =============================================================================
// 14. Complex realistic document
// =============================================================================

func TestFormatComplexRealisticDocument(t *testing.T) {
	input := `# Application config
title="My Application"
version  =  42
debug=true
[server]
host="0.0.0.0"
port  =  8080
weight=3.14
  [ server . tls ]
  enabled =true
cert='/etc/ssl/cert.pem'


[[database]]
name="primary"
tags=["fast","reliable"]
config =   { timeout=30, retries  = 3 }


[[database]]
name="replica"
tags = [  "slow"  ,  "backup"  ]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	got := string(doc.Format())

	// Verify structural correctness.
	if !strings.Contains(got, "# Application config") {
		t.Error("missing header comment")
	}
	if !strings.Contains(got, `title = "My Application"`) {
		t.Errorf("expected normalized title, got:\n%s", got)
	}
	if !strings.Contains(got, "version = 42") {
		t.Errorf("expected normalized version, got:\n%s", got)
	}
	if !strings.Contains(got, "debug = true") {
		t.Errorf("expected normalized debug, got:\n%s", got)
	}
	if !strings.Contains(got, "[server]") {
		t.Errorf("expected [server], got:\n%s", got)
	}
	if !strings.Contains(got, `host = "0.0.0.0"`) {
		t.Errorf("expected normalized host, got:\n%s", got)
	}
	if !strings.Contains(got, "port = 8080") {
		t.Errorf("expected normalized port, got:\n%s", got)
	}
	if !strings.Contains(got, "weight = 3.14") {
		t.Errorf("expected normalized weight, got:\n%s", got)
	}
	if !strings.Contains(got, "[server.tls]") {
		t.Errorf("expected normalized [server.tls], got:\n%s", got)
	}
	if !strings.Contains(got, "enabled = true") {
		t.Errorf("expected normalized enabled, got:\n%s", got)
	}
	if !strings.Contains(got, "[[database]]") {
		t.Errorf("expected [[database]], got:\n%s", got)
	}
	if !strings.Contains(got, `name = "primary"`) {
		t.Errorf("expected normalized name, got:\n%s", got)
	}
	if !strings.Contains(got, `name = "replica"`) {
		t.Errorf("expected normalized replica name, got:\n%s", got)
	}
	if !strings.Contains(got, "{timeout = 30, retries = 3}") {
		t.Errorf("expected normalized inline table, got:\n%s", got)
	}

	// No trailing whitespace on any line.
	for i, line := range strings.Split(got, "\n") {
		trimmed := strings.TrimRight(line, " \t")
		if line != trimmed {
			t.Errorf("line %d has trailing whitespace: %q", i+1, line)
		}
	}

	// No triple blank lines.
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("should not have triple blank lines, got:\n%s", got)
	}

	// Ends with exactly one newline.
	if !strings.HasSuffix(got, "\n") {
		t.Error("should end with newline")
	}
	if strings.HasSuffix(got, "\n\n") {
		t.Error("should end with exactly one newline")
	}

	// Verify the output is valid TOML.
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output is not valid TOML: %v\noutput:\n%s", err, got)
	}
}

// =============================================================================
// 15. Array table formatting
// =============================================================================

func TestFormatArrayTable(t *testing.T) {
	input := "[[products]]\nname = \"Hammer\"\n[[products]]\nname = \"Nail\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	// Should have blank line between array table entries.
	count := strings.Count(got, "[[products]]")
	if count != 2 {
		t.Errorf("expected 2 [[products]] headers, got %d\noutput:\n%s", count, got)
	}
	// Output should be valid TOML.
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output is not valid TOML: %v\noutput:\n%s", err, got)
	}
}

// =============================================================================
// 16. Key rendering: bare vs quoted
// =============================================================================

func TestFormatKeyRendering(t *testing.T) {
	input := `"simple" = 1
"key with spaces" = 2
bare-key = 3
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	// "simple" should be rendered as bare key since it's valid.
	if !strings.Contains(got, "simple = 1") {
		t.Errorf("expected bare key for 'simple', got:\n%s", got)
	}
	// "key with spaces" must remain quoted.
	if !strings.Contains(got, `"key with spaces" = 2`) {
		t.Errorf("expected quoted key for 'key with spaces', got:\n%s", got)
	}
	// bare-key should stay bare.
	if !strings.Contains(got, "bare-key = 3") {
		t.Errorf("expected bare key for 'bare-key', got:\n%s", got)
	}
}

// =============================================================================
// 17. Format output is valid TOML (round-trip)
// =============================================================================

func TestFormatOutputIsParseable(t *testing.T) {
	input := `# Config
title = "Test"
[server]
host = "localhost"
port = 8080
[[items]]
name = "one"
tags = ["a", "b"]
[[items]]
name = "two"
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	formatted := doc.Format()
	doc2, err := Parse(formatted)
	if err != nil {
		t.Fatalf("formatted output is not valid TOML: %v\noutput:\n%s", err, string(formatted))
	}
	// Verify some values survived.
	if v, ok := doc2.GetString("title"); !ok || v != "Test" {
		t.Errorf("title not preserved: %q", v)
	}
}

// =============================================================================
// 18. Multi-line array with indentation
// =============================================================================

func TestFormatMultiLineArrayWithIndent(t *testing.T) {
	input := `[section]
items = ["very-long-element-one", "very-long-element-two", "very-long-element-three"]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format(WithLineWidth(40), WithIndentWidth(4)))
	// Array should be multi-line with proper indentation.
	if !strings.Contains(got, "[\n") {
		t.Errorf("expected multi-line array, got:\n%s", got)
	}
	// Elements should have deeper indentation.
	if !strings.Contains(got, "        \"very-long-element-one\",") {
		t.Errorf("expected indented array elements, got:\n%s", got)
	}
	// Verify valid TOML.
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output is not valid TOML: %v\noutput:\n%s", err, got)
	}
}

// =============================================================================
// 19. Inline comment normalization
// =============================================================================

func TestFormatInlineCommentNormalization(t *testing.T) {
	input := "host = \"localhost\"     # the host\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	// Should have exactly two spaces before #.
	if !strings.Contains(got, `host = "localhost"  # the host`) {
		t.Errorf("expected normalized inline comment, got:\n%s", got)
	}
}

// =============================================================================
// 20. First table at start of document has no preceding blank line
// =============================================================================

func TestFormatFirstTableNoBlankLine(t *testing.T) {
	input := "[server]\nhost = \"localhost\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	// Should NOT start with a blank line.
	if strings.HasPrefix(got, "\n") {
		t.Errorf("first table should not have blank line before it, got:\n%q", got)
	}
}

// =============================================================================
// 21. Boolean rendering
// =============================================================================

func TestFormatBooleanValues(t *testing.T) {
	input := "debug = true\nverbose = false\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "debug = true") {
		t.Errorf("expected 'debug = true', got:\n%s", got)
	}
	if !strings.Contains(got, "verbose = false") {
		t.Errorf("expected 'verbose = false', got:\n%s", got)
	}
}

// =============================================================================
// 22. Date/time values
// =============================================================================

func TestFormatDateTimeValues(t *testing.T) {
	input := `d = 1979-05-27
t = 07:32:00
dt = 1979-05-27T07:32:00Z
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "d = 1979-05-27") {
		t.Errorf("expected date, got:\n%s", got)
	}
	if !strings.Contains(got, "t = 07:32:00") {
		t.Errorf("expected time, got:\n%s", got)
	}
	if !strings.Contains(got, "dt = 1979-05-27T07:32:00") {
		t.Errorf("expected datetime, got:\n%s", got)
	}
}

// =============================================================================
// 23. Empty inline table and empty array
// =============================================================================

func TestFormatEmptyContainers(t *testing.T) {
	input := "a = []\nt = {}\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "a = []") {
		t.Errorf("expected empty array, got:\n%s", got)
	}
	if !strings.Contains(got, "t = {}") {
		t.Errorf("expected empty inline table, got:\n%s", got)
	}
}
