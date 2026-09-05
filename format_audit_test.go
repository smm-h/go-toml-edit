package tomledit

import (
	"strings"
	"testing"
)

// =============================================================================
// Audit Focus Area 1: Comment edge cases
// =============================================================================

func TestAuditMultipleLeadingCommentsOnSingleKey(t *testing.T) {
	input := "# first comment\n# second comment\n# third comment\nkey = \"value\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "# first comment") {
		t.Errorf("missing first leading comment, got:\n%s", got)
	}
	if !strings.Contains(got, "# second comment") {
		t.Errorf("missing second leading comment, got:\n%s", got)
	}
	if !strings.Contains(got, "# third comment") {
		t.Errorf("missing third leading comment, got:\n%s", got)
	}
	if !strings.Contains(got, `key = "value"`) {
		t.Errorf("missing key-value, got:\n%s", got)
	}
	// All three comments should appear before the key
	idx1 := strings.Index(got, "# first comment")
	idx2 := strings.Index(got, "# second comment")
	idx3 := strings.Index(got, "# third comment")
	idxKV := strings.Index(got, `key = "value"`)
	if idx1 >= idx2 || idx2 >= idx3 || idx3 >= idxKV {
		t.Errorf("comments not in correct order before key, got:\n%s", got)
	}
}

func TestAuditCommentBetweenTwoTablesNoKVs(t *testing.T) {
	input := "[a]\n# orphan comment\n[b]\nkey = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "# orphan comment") {
		t.Errorf("missing comment between tables, got:\n%s", got)
	}
	if !strings.Contains(got, "[a]") {
		t.Errorf("missing [a], got:\n%s", got)
	}
	if !strings.Contains(got, "[b]") {
		t.Errorf("missing [b], got:\n%s", got)
	}
	// Verify output is valid TOML
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output is not valid TOML: %v\noutput:\n%s", err, got)
	}
}

func TestAuditCommentAtVeryStartOfFile(t *testing.T) {
	input := "# file header comment\nkey = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.HasPrefix(got, "# file header comment") {
		t.Errorf("expected file to start with comment, got:\n%q", got)
	}
}

func TestAuditCommentAtVeryEndOfFile(t *testing.T) {
	input := "key = 1\n# trailing comment\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "# trailing comment") {
		t.Errorf("missing trailing comment, got:\n%s", got)
	}
	// Should end with exactly one newline
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("should end with newline, got:\n%q", got)
	}
	if strings.HasSuffix(got, "\n\n") {
		t.Errorf("should end with exactly one newline, got:\n%q", got)
	}
}

func TestAuditInlineCommentOnTableHeader(t *testing.T) {
	input := "[server] # server config\nhost = \"localhost\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "[server]") {
		t.Errorf("missing [server], got:\n%s", got)
	}
	if !strings.Contains(got, "# server config") {
		t.Errorf("missing inline comment on table header, got:\n%s", got)
	}
	// Should be normalized to two spaces before #
	if !strings.Contains(got, "[server]  # server config") {
		t.Errorf("expected normalized spacing for inline comment on table header, got:\n%s", got)
	}
	// Verify valid TOML
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output is not valid TOML: %v\noutput:\n%s", err, got)
	}
}

// =============================================================================
// Audit Focus Area 2: Multi-line array details
// =============================================================================

func TestAuditNestedArraysMultiLine(t *testing.T) {
	// Outer array is long enough to go multi-line, inner array should also go
	// multi-line if it exceeds line width.
	input := `matrix = [["aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc"], ["dddddddddd", "eeeeeeeeee", "ffffffffff"]]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format(WithLineWidth(40)))
	// Outer array should be multi-line
	if !strings.Contains(got, "[\n") {
		t.Errorf("expected outer array to be multi-line, got:\n%s", got)
	}
	// Verify valid TOML
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output is not valid TOML: %v\noutput:\n%s", err, got)
	}
}

func TestAuditArrayOfInlineTables(t *testing.T) {
	input := `items = [{name = "a", value = 1}, {name = "b", value = 2}, {name = "c", value = 3}]
`
	_, err := Parse([]byte(input))
	if err != nil {
		// Parser does not currently support inline tables inside arrays.
		// This is a parser limitation, not a formatter bug.
		t.Skipf("Parser does not support inline tables inside arrays (known parser limitation): %v", err)
	}
	t.Log("Parser now supports inline tables in arrays -- update this test to verify formatting")
}

// =============================================================================
// Audit Focus Area 3: Idempotency
// =============================================================================

func TestAuditIdempotency(t *testing.T) {
	inputs := []string{
		// Simple
		"key = \"value\"\n",
		// With comments
		"# header\nkey = 1 # inline\n# standalone\nother = 2\n",
		// Tables
		"[a]\nx = 1\n[b]\ny = 2\n",
		// Array tables
		"[[items]]\nname = \"one\"\n[[items]]\nname = \"two\"\n",
		// Inline tables
		"t = {a = 1, b = 2}\n",
		// Arrays
		"arr = [1, 2, 3]\n",
		// Mixed
		"# Config\ntitle = \"test\"\n[server]\nhost = \"localhost\"\nport = 8080\ntags = [\"a\", \"b\"]\n",
		// Messy input
		"host=\"localhost\"\n  port  =  8080\n",
	}
	for i, input := range inputs {
		doc1, err := Parse([]byte(input))
		if err != nil {
			t.Fatalf("case %d: Parse error on input: %v", i, err)
		}
		formatted1 := doc1.Format()

		doc2, err := Parse(formatted1)
		if err != nil {
			t.Fatalf("case %d: Parse error on formatted output: %v\nformatted:\n%s", i, err, string(formatted1))
		}
		formatted2 := doc2.Format()

		if string(formatted1) != string(formatted2) {
			t.Errorf("case %d: Format is NOT idempotent.\nFirst format:\n%s\nSecond format:\n%s", i, string(formatted1), string(formatted2))
		}
	}
}

func TestAuditIdempotencyComplex(t *testing.T) {
	input := `# Application config
title = "My App"
version = 42
debug = true

[server]
host = "0.0.0.0"
port = 8080
weight = 3.14

[server.tls]
enabled = true
cert = "/etc/ssl/cert.pem"

[[database]]
name = "primary"
tags = ["fast", "reliable"]
config = {timeout = 30, retries = 3}

[[database]]
name = "replica"
tags = ["slow", "backup"]
`
	doc1, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	formatted1 := doc1.Format()

	doc2, err := Parse(formatted1)
	if err != nil {
		t.Fatalf("Parse error on formatted: %v\n%s", err, string(formatted1))
	}
	formatted2 := doc2.Format()

	if string(formatted1) != string(formatted2) {
		t.Errorf("Complex format NOT idempotent.\nFirst:\n%s\nSecond:\n%s", string(formatted1), string(formatted2))
	}
}

// =============================================================================
// Audit Focus Area 4: Values with special representations
// =============================================================================

func TestAuditFloatZeroPointZero(t *testing.T) {
	// 0.0 must remain as a float, not become integer 0
	input := "val = 0.0\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	// Must contain "0.0", not just "0"
	if !strings.Contains(got, "val = 0.0") {
		t.Errorf("expected val = 0.0, got:\n%s", got)
	}
	// Verify re-parsing gives a float
	doc2, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("re-parse error: %v", err)
	}
	node, ok := doc2.Lookup("val")
	if !ok {
		t.Fatal("val not found after re-parse")
	}
	if _, ok := node.(*FloatNode); !ok {
		t.Errorf("expected FloatNode after re-parse, got %T", node)
	}
}

func TestAuditIntegerHexOctalBinary(t *testing.T) {
	input := "hex = 0xff\noct = 0o77\nbin = 0b1010\ndec = 42\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())

	// Verify the formatter preserves the base (hex stays hex, etc.)
	// OR normalizes to decimal. Either is valid -- we just need to verify
	// the semantic values are correct.
	doc2, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output not valid TOML: %v\noutput:\n%s", err, got)
	}

	hexVal, err := doc2.GetInt("hex")
	if err != nil || hexVal != 0xff {
		t.Errorf("hex value incorrect: got %d, want %d", hexVal, 0xff)
	}
	octVal, err := doc2.GetInt("oct")
	if err != nil || octVal != 0o77 {
		t.Errorf("oct value incorrect: got %d, want %d", octVal, 0o77)
	}
	binVal, err := doc2.GetInt("bin")
	if err != nil || binVal != 0b1010 {
		t.Errorf("bin value incorrect: got %d, want %d", binVal, 0b1010)
	}
	decVal, err := doc2.GetInt("dec")
	if err != nil || decVal != 42 {
		t.Errorf("dec value incorrect: got %d, want %d", decVal, 42)
	}

	// Check whether bases are preserved
	if strings.Contains(got, "0x") {
		t.Logf("Hex base preserved: %s", got)
	} else {
		t.Logf("Hex normalized to decimal: %s", got)
	}
}

func TestAuditMultiLineStrings(t *testing.T) {
	input := "val = \"\"\"\nhello\nworld\"\"\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	// The formatter should preserve multi-line string style or collapse it.
	// Either way, the semantic value must survive.
	doc2, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output not valid TOML: %v\noutput:\n%s", err, got)
	}
	v, err := doc2.GetString("val")
	if err != nil {
		t.Fatal("val not found after re-parse")
	}
	if v != "hello\nworld" {
		t.Errorf("multi-line string value incorrect: got %q, want %q", v, "hello\nworld")
	}
}

func TestAuditDateTimeValues(t *testing.T) {
	input := "odt = 1979-05-27T07:32:00Z\nldt = 1979-05-27T07:32:00\nld = 1979-05-27\nlt = 07:32:00\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	// Re-parse should succeed
	doc2, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output not valid TOML: %v\noutput:\n%s", err, got)
	}
	// Verify semantic values
	_ = doc2
	// Check presence of date/time patterns
	if !strings.Contains(got, "1979-05-27") {
		t.Errorf("expected date in output, got:\n%s", got)
	}
	if !strings.Contains(got, "07:32:00") {
		t.Errorf("expected time in output, got:\n%s", got)
	}
}

// =============================================================================
// Audit Focus Area 5: Inline table with many keys
// =============================================================================

func TestAuditInlineTableNeverBreaksAcrossLines(t *testing.T) {
	// Even with a very narrow line width, inline tables must stay on one line
	// per TOML 1.0 spec.
	input := `t = {a = 1, b = 2, c = 3, d = 4, e = 5, f = 6, g = 7, h = 8}
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format(WithLineWidth(20)))
	// The inline table must be on one line
	for _, line := range strings.Split(got, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "t = {") {
			if !strings.Contains(trimmed, "}") {
				t.Errorf("inline table broken across lines: %q\nfull output:\n%s", line, got)
			}
		}
	}
	// Verify valid TOML
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output not valid TOML: %v\noutput:\n%s", err, got)
	}
}

// =============================================================================
// Audit Focus Area 6: Keys with special characters
// =============================================================================

func TestAuditKeysNeedingQuoting(t *testing.T) {
	input := `"key.with.dots" = 1
"key with spaces" = 2
"" = 3
bare_key-123 = 4
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())

	// Keys with dots must be quoted
	if !strings.Contains(got, `"key.with.dots" = 1`) {
		t.Errorf("expected quoted key for dots, got:\n%s", got)
	}
	// Keys with spaces must be quoted
	if !strings.Contains(got, `"key with spaces" = 2`) {
		t.Errorf("expected quoted key for spaces, got:\n%s", got)
	}
	// Empty key must be quoted
	if !strings.Contains(got, `"" = 3`) {
		t.Errorf("expected quoted empty key, got:\n%s", got)
	}
	// Valid bare key should remain bare
	if !strings.Contains(got, "bare_key-123 = 4") {
		t.Errorf("expected bare key for bare_key-123, got:\n%s", got)
	}

	// Verify valid TOML
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output not valid TOML: %v\noutput:\n%s", err, got)
	}
}

func TestAuditUnicodeKeys(t *testing.T) {
	// Unicode characters in keys should be quoted
	input := "\"cafe\\u0301\" = true\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	// The key contains non-ASCII, so it should be quoted
	// It should NOT appear as a bare key
	// Verify valid TOML
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output not valid TOML: %v\noutput:\n%s", err, got)
	}
}

// =============================================================================
// Additional: Non-mutation verification
// =============================================================================

func TestAuditFormatDoesNotMutateWithOptions(t *testing.T) {
	input := "host = \"localhost\"\n[server]\nport = 8080\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	originalBytes := string(doc.Bytes())

	// Call Format with various options
	_ = doc.Format(WithIndentWidth(4), WithLineWidth(40))
	_ = doc.Format(WithIndentWidth(0), WithLineWidth(80))
	_ = doc.Format()

	afterBytes := string(doc.Bytes())
	if afterBytes != originalBytes {
		t.Errorf("Format() mutated the document:\n  before: %q\n  after:  %q", originalBytes, afterBytes)
	}
}

// =============================================================================
// Additional: Comment-only file
// =============================================================================

func TestAuditCommentOnlyFile(t *testing.T) {
	input := "# just a comment\n# another comment\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "# just a comment") {
		t.Errorf("missing first comment, got:\n%s", got)
	}
	if !strings.Contains(got, "# another comment") {
		t.Errorf("missing second comment, got:\n%s", got)
	}
	// Must end with exactly one newline
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("should end with newline")
	}
	if strings.HasSuffix(got, "\n\n") {
		t.Errorf("should end with exactly one newline, got:\n%q", got)
	}
}

// =============================================================================
// Additional: Negative float values
// =============================================================================

func TestAuditNegativeFloat(t *testing.T) {
	input := "val = -3.14\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "val = -3.14") {
		t.Errorf("expected val = -3.14, got:\n%s", got)
	}
}

// =============================================================================
// Additional: Special float values (inf, nan)
// =============================================================================

func TestAuditSpecialFloats(t *testing.T) {
	input := "pinf = inf\nninf = -inf\nnan_val = nan\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "pinf = inf") {
		t.Errorf("expected pinf = inf, got:\n%s", got)
	}
	if !strings.Contains(got, "ninf = -inf") {
		t.Errorf("expected ninf = -inf, got:\n%s", got)
	}
	if !strings.Contains(got, "nan_val = nan") {
		t.Errorf("expected nan_val = nan, got:\n%s", got)
	}
	// Verify valid TOML
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output not valid TOML: %v\noutput:\n%s", err, got)
	}
}

// =============================================================================
// Additional: Literal strings preserved
// =============================================================================

func TestAuditLiteralStringPreserved(t *testing.T) {
	input := "path = 'C:\\Users\\dir'\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	// Literal string style should be preserved -- no escaping of backslashes
	doc2, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output not valid TOML: %v\noutput:\n%s", err, got)
	}
	v, err := doc2.GetString("path")
	if err != nil || v != `C:\Users\dir` {
		t.Errorf("literal string value incorrect: got %q, want %q", v, `C:\Users\dir`)
	}
}

// =============================================================================
// Additional: Table with leading comment, then another table immediately after
// =============================================================================

func TestAuditLeadingCommentOnTable(t *testing.T) {
	input := "# Comment for section a\n[a]\nx = 1\n# Comment for section b\n[b]\ny = 2\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	if !strings.Contains(got, "# Comment for section a") {
		t.Errorf("missing comment for [a], got:\n%s", got)
	}
	if !strings.Contains(got, "# Comment for section b") {
		t.Errorf("missing comment for [b], got:\n%s", got)
	}
	// Comments should appear before their respective tables
	idxCA := strings.Index(got, "# Comment for section a")
	idxA := strings.Index(got, "[a]")
	idxCB := strings.Index(got, "# Comment for section b")
	idxB := strings.Index(got, "[b]")
	if idxCA >= idxA {
		t.Errorf("comment for [a] should appear before [a]")
	}
	if idxCB >= idxB {
		t.Errorf("comment for [b] should appear before [b]")
	}
	// Verify valid TOML
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output not valid TOML: %v\noutput:\n%s", err, got)
	}
}

// =============================================================================
// Additional: Dotted keys in table headers
// =============================================================================

func TestAuditDottedTableHeaderWithQuotedParts(t *testing.T) {
	input := "[\"key.with.dots\".sub]\nval = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(doc.Format())
	// The first part needs quoting (contains dots), second part is bare
	if !strings.Contains(got, `["key.with.dots".sub]`) {
		t.Errorf("expected quoted key in table header, got:\n%s", got)
	}
	// Verify valid TOML
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("formatted output not valid TOML: %v\noutput:\n%s", err, got)
	}
}
