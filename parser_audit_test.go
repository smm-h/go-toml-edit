package tomledit

import (
	"math"
	"strings"
	"testing"
)

// =============================================================================
// Audit tests for parser.go -- probing edge cases and verifying requirements
// =============================================================================

// --- Round-trip fidelity edge cases ---

func TestAuditRoundTripNoTrailingNewline(t *testing.T) {
	// File not ending with a newline
	input := "key = 42"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripMultipleBlankLinesBetweenTables(t *testing.T) {
	input := "[a]\nx = 1\n\n\n\n[b]\ny = 2\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripCommentsBetweenArrayElements(t *testing.T) {
	input := "a = [\n  1, # first\n  # standalone comment\n  2,\n  3\n]\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripMixedWhitespaceAroundEquals(t *testing.T) {
	input := "a\t=\t1\nb  =  2\nc=3\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripMixedIndentation(t *testing.T) {
	input := "[server]\n  host = \"localhost\"\n\tport = 8080\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripCommentAtStartOfFile(t *testing.T) {
	input := "# first line comment\nkey = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripCommentAtEndOfFile(t *testing.T) {
	input := "key = 1\n# trailing comment\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripCommentAtEndNoNewline(t *testing.T) {
	// Comment at very end of file with no trailing newline
	input := "key = 1\n# trailing comment"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripOnlyComments(t *testing.T) {
	input := "# comment 1\n# comment 2\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripOnlyBlankLines(t *testing.T) {
	input := "\n\n\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripCommentBetweenTableHeaderAndFirstKV(t *testing.T) {
	input := "[server]\n# config follows\nhost = \"localhost\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripTwoConsecutiveCommentLines(t *testing.T) {
	input := "# line 1\n# line 2\nkey = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripCRLFThroughout(t *testing.T) {
	input := "[server]\r\nhost = \"localhost\"\r\nport = 8080\r\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripWhitespaceInsideBrackets(t *testing.T) {
	input := "[ server ]\nhost = \"localhost\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripWhitespaceInsideDoubleBrackets(t *testing.T) {
	input := "[[ items ]]\nname = \"one\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripBlankLineBeforeFirstTable(t *testing.T) {
	input := "\n\n[server]\nhost = \"localhost\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripInlineTableWithSpaces(t *testing.T) {
	input := "t = { x = 1 , y = 2 }\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripNestedInlineTable(t *testing.T) {
	input := "t = {a = {b = 1}}\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripArrayOfInlineTables(t *testing.T) {
	input := "a = [{x = 1}, {x = 2}]\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditRoundTripDeeplyNestedArray(t *testing.T) {
	input := "a = [[[1, 2], [3]], [[4]]]\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

// --- Trivia edge cases ---

func TestAuditTriviaCommentBetweenTableAndFirstKV(t *testing.T) {
	input := "[server]\n# config follows\nhost = \"localhost\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	tbl := doc.children[0].(*TableNode)
	if len(tbl.children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tbl.children))
	}
	kv := tbl.children[0].(*KeyValueNode)
	lc := kv.LeadingComments()
	if len(lc) != 1 {
		t.Fatalf("expected 1 leading comment on kv, got %d", len(lc))
	}
	if !strings.Contains(lc[0], "config follows") {
		t.Errorf("comment = %q", lc[0])
	}
}

func TestAuditTriviaConsecutiveComments(t *testing.T) {
	input := "# comment 1\n# comment 2\n# comment 3\nkey = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.children[0].(*KeyValueNode)
	lc := kv.LeadingComments()
	if len(lc) != 3 {
		t.Fatalf("expected 3 leading comments, got %d", len(lc))
	}
}

func TestAuditTriviaCommentAtStartAttachesToFirstNode(t *testing.T) {
	input := "# file header\nkey = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.children[0].(*KeyValueNode)
	lc := kv.LeadingComments()
	if len(lc) != 1 {
		t.Fatalf("expected 1 leading comment, got %d", len(lc))
	}
	if !strings.Contains(lc[0], "file header") {
		t.Errorf("comment = %q", lc[0])
	}
}

func TestAuditTriviaCommentAtEndOfFileBecomesOrphan(t *testing.T) {
	input := "key = 1\n# trailing\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	// Should have 2 children: kv + orphan comment
	if len(doc.children) < 2 {
		t.Fatalf("expected at least 2 children, got %d", len(doc.children))
	}
	_, ok := doc.children[1].(*CommentNode)
	if !ok {
		t.Errorf("child 1: expected CommentNode, got %T", doc.children[1])
	}
}

func TestAuditTriviaOrphanCommentBetweenTables(t *testing.T) {
	input := "[a]\nx = 1\n\n# between tables\n\n[b]\ny = 2\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	// Verify round-trip
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

// --- Value parsing edge cases ---

func TestAuditIntegerOverflow(t *testing.T) {
	// Max int64 = 9223372036854775807
	input := "n = 9223372036854775807\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	iv := doc.children[0].(*KeyValueNode).val.(*IntegerNode)
	if iv.val.get() != 9223372036854775807 {
		t.Errorf("val = %d, want max int64", iv.val.get())
	}

	// Overflow should error
	input2 := "n = 9223372036854775808\n"
	_, err = Parse([]byte(input2))
	if err == nil {
		t.Fatal("expected error for integer overflow")
	}
}

func TestAuditIntegerMinValue(t *testing.T) {
	// Min int64 = -9223372036854775808
	input := "n = -9223372036854775808\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	iv := doc.children[0].(*KeyValueNode).val.(*IntegerNode)
	if iv.val.get() != -9223372036854775808 {
		t.Errorf("val = %d, want min int64", iv.val.get())
	}
}

func TestAuditUnicodeEscapeSurrogatePair(t *testing.T) {
	// Surrogate code points should ideally be rejected, or at least handled
	// TOML spec says \uD800 through \uDFFF are not valid Unicode scalars
	// Go's rune type will accept them but they're not valid UTF-8
	input := "s = \"\\uD800\"\n"
	doc, err := Parse([]byte(input))
	// This is a gray area -- the TOML spec says Unicode scalar values only
	// but the implementation may or may not reject surrogates.
	// We just record what happens.
	if err != nil {
		t.Logf("surrogate pair \\uD800 correctly rejected: %v", err)
	} else {
		sv := doc.children[0].(*KeyValueNode).val.(*StringNode)
		t.Logf("surrogate pair \\uD800 accepted, decoded as %q (len=%d)", sv.val.get(), len(sv.val.get()))
	}
}

func TestAuditUnicodeEscape4DigitEmoji(t *testing.T) {
	// é = e with acute accent
	input := "s = \"caf\\u00E9\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	sv := doc.children[0].(*KeyValueNode).val.(*StringNode)
	if sv.val.get() != "café" {
		t.Errorf("val = %q, want %q", sv.val.get(), "café")
	}
}

func TestAuditUnicodeEscape8Digit(t *testing.T) {
	// \U0001F600 = grinning face emoji
	input := "s = \"\\U0001F600\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	sv := doc.children[0].(*KeyValueNode).val.(*StringNode)
	if sv.val.get() != "\U0001F600" {
		t.Errorf("val = %q, want %q", sv.val.get(), "\U0001F600")
	}
}

func TestAuditMultiLineStringCRLF(t *testing.T) {
	// Multi-line basic string with CRLF
	input := "s = \"\"\"\r\nhello\r\nworld\"\"\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	sv := doc.children[0].(*KeyValueNode).val.(*StringNode)
	// The first CRLF after """ should be trimmed
	// The remaining content should preserve CRLF
	if !strings.Contains(sv.val.get(), "hello") {
		t.Errorf("val = %q", sv.val.get())
	}
}

func TestAuditMultiLineStringLineEndingBackslash(t *testing.T) {
	input := "s = \"\"\"\nhello \\\n  world\"\"\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	sv := doc.children[0].(*KeyValueNode).val.(*StringNode)
	if sv.val.get() != "hello world" {
		t.Errorf("val = %q, want %q", sv.val.get(), "hello world")
	}
}

func TestAuditEmptyMultiLineBasicString(t *testing.T) {
	input := "s = \"\"\"\n\"\"\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	sv := doc.children[0].(*KeyValueNode).val.(*StringNode)
	if sv.val.get() != "" {
		t.Errorf("val = %q, want empty string", sv.val.get())
	}
}

func TestAuditEmptyMultiLineLiteralString(t *testing.T) {
	input := "s = '''\n'''\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	sv := doc.children[0].(*KeyValueNode).val.(*StringNode)
	if sv.val.get() != "" {
		t.Errorf("val = %q, want empty string", sv.val.get())
	}
}

func TestAuditEmptyBasicString(t *testing.T) {
	input := "s = \"\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	sv := doc.children[0].(*KeyValueNode).val.(*StringNode)
	if sv.val.get() != "" {
		t.Errorf("val = %q, want empty string", sv.val.get())
	}
}

func TestAuditEmptyLiteralString(t *testing.T) {
	input := "s = ''\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	sv := doc.children[0].(*KeyValueNode).val.(*StringNode)
	if sv.val.get() != "" {
		t.Errorf("val = %q, want empty string", sv.val.get())
	}
}

func TestAuditNegativeNaN(t *testing.T) {
	input := "n = -nan\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	fv := doc.children[0].(*KeyValueNode).val.(*FloatNode)
	if !math.IsNaN(fv.val.get()) {
		t.Errorf("expected NaN, got %f", fv.val.get())
	}
}

func TestAuditPositiveNaN(t *testing.T) {
	input := "n = +nan\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	fv := doc.children[0].(*KeyValueNode).val.(*FloatNode)
	if !math.IsNaN(fv.val.get()) {
		t.Errorf("expected NaN, got %f", fv.val.get())
	}
}

func TestAuditZeroFloat(t *testing.T) {
	input := "n = 0.0\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	fv := doc.children[0].(*KeyValueNode).val.(*FloatNode)
	if fv.val.get() != 0.0 {
		t.Errorf("val = %f, want 0.0", fv.val.get())
	}
}

func TestAuditIntegerWithLeadingPlus(t *testing.T) {
	input := "n = +0\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	iv := doc.children[0].(*KeyValueNode).val.(*IntegerNode)
	if iv.val.get() != 0 {
		t.Errorf("val = %d, want 0", iv.val.get())
	}
}

// --- Validation edge cases ---

func TestAuditDottedKeyConflictValueThenTable(t *testing.T) {
	// a.b = 1 then a.b.c = 2 should fail because a.b is a value
	input := "a.b = 1\na.b.c = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for dotted key conflict (value then table)")
	}
}

func TestAuditDuplicateDottedKey(t *testing.T) {
	// a.b = 1, a.b = 2 should fail
	input := "a.b = 1\na.b = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for duplicate dotted key")
	}
}

func TestAuditTableAfterArrayTable(t *testing.T) {
	// [[a]] then [a] should fail
	input := "[[a]]\nx = 1\n[a]\ny = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for table after array-table with same key")
	}
}

func TestAuditArrayTableAfterTable(t *testing.T) {
	// [a] then [[a]] should fail
	input := "[a]\nx = 1\n[[a]]\ny = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for array-table after table with same key")
	}
}

func TestAuditDuplicateKeyInTable(t *testing.T) {
	input := "[server]\nhost = \"a\"\nhost = \"b\"\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for duplicate key in table")
	}
}

func TestAuditImplicitTableCanBeExtended(t *testing.T) {
	// a.b.c = 1 defines [a] via dotted key.
	// [a] tries to reopen it with a table header, which is invalid
	// per TOML spec: dotted-key-defined tables cannot be reopened with
	// [table] headers.
	input := "a.b.c = 1\n[a]\nd = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error: dotted-key-defined table cannot be reopened with [table] header")
	}
}

func TestAuditArrayTableSubtable(t *testing.T) {
	// [[fruit]] then [fruit.details] -- valid TOML
	input := "[[fruit]]\nname = \"apple\"\n[fruit.details]\ncolor = \"red\"\n"
	_, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditDuplicateKeyAcrossDottedAndSimple(t *testing.T) {
	// Under [a], define b.c = 1, then b.c = 2 should fail
	input := "[a]\nb.c = 1\nb.c = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for duplicate dotted key within table")
	}
}

// --- Key style verification ---

func TestAuditKeyStyleTracking(t *testing.T) {
	input := "bare = 1\n\"quoted\" = 2\n'literal' = 3\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	kv1 := doc.children[0].(*KeyValueNode)
	if kv1.key.styles[0] != StringBasic {
		t.Errorf("bare key style = %d, want StringBasic", kv1.key.styles[0])
	}

	kv2 := doc.children[1].(*KeyValueNode)
	if kv2.key.styles[0] != StringBasic {
		t.Errorf("quoted key style = %d, want StringBasic", kv2.key.styles[0])
	}

	kv3 := doc.children[2].(*KeyValueNode)
	if kv3.key.styles[0] != StringLiteral {
		t.Errorf("literal key style = %d, want StringLiteral", kv3.key.styles[0])
	}
}

func TestAuditKeyRawParts(t *testing.T) {
	input := "server.\"host name\".port = 8080\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.children[0].(*KeyValueNode)
	if len(kv.key.rawParts) != 3 {
		t.Fatalf("expected 3 raw parts, got %d", len(kv.key.rawParts))
	}
	if string(kv.key.rawParts[0]) != "server" {
		t.Errorf("raw part 0 = %q", kv.key.rawParts[0])
	}
	if string(kv.key.rawParts[1]) != "\"host name\"" {
		t.Errorf("raw part 1 = %q", kv.key.rawParts[1])
	}
	if string(kv.key.rawParts[2]) != "port" {
		t.Errorf("raw part 2 = %q", kv.key.rawParts[2])
	}
}

// --- Date/time edge cases ---

func TestAuditOffsetDateTimeWithFractional(t *testing.T) {
	input := "dt = 1979-05-27T07:32:00.999999Z\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	dt := doc.children[0].(*KeyValueNode).val.(*DateTimeNode)
	if dt.val.get().Year() != 1979 || dt.val.get().Month() != 5 || dt.val.get().Day() != 27 {
		t.Errorf("date = %v", dt.val.get())
	}
	if dt.val.get().Nanosecond() == 0 {
		t.Error("expected non-zero nanoseconds")
	}
}

func TestAuditLocalDateTimeSpaceSeparator(t *testing.T) {
	// TOML allows space instead of T as separator
	input := "dt = 1979-05-27 07:32:00\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	ldt := doc.children[0].(*KeyValueNode).val.(*LocalDateTimeNode)
	if ldt.val.get().Year != 1979 || ldt.val.get().Hour != 7 {
		t.Errorf("datetime = %+v", ldt.val.get())
	}
}

// --- Array edge cases ---

func TestAuditArrayOfMixedTypes(t *testing.T) {
	// TOML 1.0 allows mixed-type arrays
	input := "a = [1, \"two\", 3.0]\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	arr := doc.children[0].(*KeyValueNode).val.(*ArrayNode)
	if len(arr.elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr.elements))
	}
	_, ok1 := arr.elements[0].(*IntegerNode)
	_, ok2 := arr.elements[1].(*StringNode)
	_, ok3 := arr.elements[2].(*FloatNode)
	if !ok1 || !ok2 || !ok3 {
		t.Errorf("types: %T, %T, %T", arr.elements[0], arr.elements[1], arr.elements[2])
	}
}

func TestAuditArrayEmptyMultiline(t *testing.T) {
	input := "a = [\n]\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	arr := doc.children[0].(*KeyValueNode).val.(*ArrayNode)
	if len(arr.elements) != 0 {
		t.Errorf("expected empty array, got %d elements", len(arr.elements))
	}
}

func TestAuditArraySingleElementTrailingComma(t *testing.T) {
	input := "a = [1,]\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	arr := doc.children[0].(*KeyValueNode).val.(*ArrayNode)
	if len(arr.elements) != 1 {
		t.Fatalf("expected 1 element, got %d", len(arr.elements))
	}
}

// --- Inline table edge cases ---

func TestAuditInlineTableNestedDottedKey(t *testing.T) {
	input := "t = {a.b.c = 1}\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	it := doc.children[0].(*KeyValueNode).val.(*InlineTableNode)
	if len(it.children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(it.children))
	}
	kv := it.children[0].(*KeyValueNode)
	if len(kv.key.parts) != 3 {
		t.Errorf("expected 3 key parts, got %d", len(kv.key.parts))
	}
}

func TestAuditInlineTableSingleEntry(t *testing.T) {
	input := "t = {x = 1}\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	it := doc.children[0].(*KeyValueNode).val.(*InlineTableNode)
	if len(it.children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(it.children))
	}
}

// --- Complex round-trip with all features ---

func TestAuditRoundTripComprehensive(t *testing.T) {
	input := `# File header comment

# Title comment
title = "My App" # inline comment on title
version = 1_000
debug = false

[server]
host = "0.0.0.0"
port = 8080
weight = 3.14

  [server.tls]
  enabled = true
  cert = '/etc/ssl/cert.pem'

# Array table section
[[database]]
name = "primary"
tags = ["fast", "reliable"]
config = {timeout = 30, retries = 3}

[[database]]
name = "replica"
tags = [
  "slow",
  "backup",
]
`
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\ninput len=%d\ngot len=%d", len(input), len(got))
		for i := 0; i < len(input) && i < len(got); i++ {
			if input[i] != got[i] {
				start := i - 10
				if start < 0 {
					start = 0
				}
				end := i + 20
				if end > len(input) {
					end = len(input)
				}
				endG := i + 20
				if endG > len(got) {
					endG = len(got)
				}
				t.Errorf("first diff at byte %d:\n  input: %q\n  got:   %q", i, input[start:end], got[start:endG])
				break
			}
		}
	}
}

// --- Document.Raw() ---

func TestAuditDocumentRaw(t *testing.T) {
	// Document itself doesn't carry Raw bytes -- the children do.
	// Verify collectRawBytes works by walking children.
	input := "key = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	// Document.Raw() is nil/empty
	if len(doc.Raw()) != 0 {
		t.Logf("Document.Raw() has %d bytes -- this is fine if raw walk still works", len(doc.Raw()))
	}
}

// --- Escape sequence edge cases ---

func TestAuditAllBasicEscapes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"backspace", `s = "\b"` + "\n", "\b"},
		{"tab", `s = "\t"` + "\n", "\t"},
		{"newline", `s = "\n"` + "\n", "\n"},
		{"formfeed", `s = "\f"` + "\n", "\f"},
		{"carriage-return", `s = "\r"` + "\n", "\r"},
		{"double-quote", `s = "\""` + "\n", "\""},
		{"backslash", `s = "\\"` + "\n", "\\"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			sv := doc.children[0].(*KeyValueNode).val.(*StringNode)
			if sv.val.get() != tt.expected {
				t.Errorf("val = %q, want %q", sv.val.get(), tt.expected)
			}
		})
	}
}

func TestAuditInvalidEscapeRejected(t *testing.T) {
	input := "s = \"\\a\"\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for invalid escape \\a")
	}
}

// --- Table with inline comment on header ---

func TestAuditTableHeaderInlineComment(t *testing.T) {
	input := "[server] # the server config\nhost = \"localhost\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	tbl := doc.children[0].(*TableNode)
	if !strings.Contains(tbl.Comment(), "the server config") {
		t.Errorf("inline comment = %q", tbl.Comment())
	}
	// Round-trip
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

// --- Array table with inline comment on header ---

func TestAuditArrayTableHeaderInlineComment(t *testing.T) {
	input := "[[items]] # list of items\nname = \"one\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	atbl := doc.children[0].(*ArrayTableNode)
	if !strings.Contains(atbl.Comment(), "list of items") {
		t.Errorf("inline comment = %q", atbl.Comment())
	}
	// Round-trip
	got := string(collectRawBytes(doc))
	if got != input {
		t.Errorf("round-trip mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

// --- Float exponent notation ---

func TestAuditFloatExponentEdgeCases(t *testing.T) {
	tests := []struct {
		input string
		val   float64
	}{
		{"n = 1e0\n", 1.0},
		{"n = 1E0\n", 1.0},
		{"n = 1.0e+0\n", 1.0},
		{"n = 1.0e-0\n", 1.0},
		{"n = -1e2\n", -100.0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			doc, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			fv := doc.children[0].(*KeyValueNode).val.(*FloatNode)
			if fv.val.get() != tt.val {
				t.Errorf("val = %f, want %f", fv.val.get(), tt.val)
			}
		})
	}
}

// --- Bare key style: bare keys are marked StringBasic, which could confuse ---

func TestAuditBareKeyVsQuotedKeyStyle(t *testing.T) {
	// Both bare and basic-quoted keys get StringBasic.
	// This means you cannot distinguish a bare key from a quoted key by style alone.
	// This is documented behavior but worth noting.
	input := "bare = 1\n\"quoted\" = 2\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv1 := doc.children[0].(*KeyValueNode)
	kv2 := doc.children[1].(*KeyValueNode)
	// Both are StringBasic -- the only way to distinguish is by looking at RawParts
	if kv1.key.styles[0] != kv2.key.styles[0] {
		t.Logf("bare key style %d vs quoted key style %d -- different (unexpected)", kv1.key.styles[0], kv2.key.styles[0])
	} else {
		// They're the same -- check RawParts distinguish them
		if string(kv1.key.rawParts[0]) == "bare" && string(kv2.key.rawParts[0]) == "\"quoted\"" {
			t.Logf("bare vs quoted keys are both StringBasic but distinguishable via RawParts -- this is a known limitation")
		}
	}
}

// =============================================================================
// BUG: Inline table does NOT check for duplicate keys
// =============================================================================

func TestAuditInlineTableDuplicateKey(t *testing.T) {
	// TOML spec: duplicate keys within an inline table are invalid
	input := "t = {x = 1, x = 2}\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("BUG: expected error for duplicate key in inline table, but got nil -- inline table does not track key definitions")
	}
}

// =============================================================================
// BUG: Inline table allows trailing comma (TOML 1.0 forbids it)
// =============================================================================

func TestAuditInlineTableTrailingCommaForbidden(t *testing.T) {
	// TOML 1.0 spec: "A trailing comma (also called terminating comma) is
	// not permitted after the last key/value pair in an inline table."
	input := "t = {x = 1, y = 2,}\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("BUG: expected error for trailing comma in inline table, but got nil -- TOML 1.0 forbids trailing commas in inline tables")
	}
}

// =============================================================================
// Verify surrogate rejection (TOML spec mandates Unicode scalar values only)
// =============================================================================

func TestAuditUnicodeSurrogateShouldBeRejected(t *testing.T) {
	// TOML spec: "Unicode escape: \uXXXX / \UXXXXXXXX - Unicode code point"
	// Only Unicode scalar values are valid (U+0000..U+D7FF, U+E000..U+10FFFF)
	// Surrogates U+D800..U+DFFF must be rejected.
	inputs := []string{
		"s = \"\\uD800\"\n",
		"s = \"\\uDFFF\"\n",
		"s = \"\\uDBFF\"\n",
		"s = \"\\uDC00\"\n",
	}
	for _, input := range inputs {
		_, err := Parse([]byte(input))
		if err == nil {
			t.Errorf("BUG: expected error for surrogate code point in %q, got nil -- TOML spec requires Unicode scalar values only", input)
		}
	}
}

// =============================================================================
// Additional edge case: implicit table then used as value
// =============================================================================

func TestAuditImplicitTableThenValue(t *testing.T) {
	// a.b = 1 creates implicit "a" with child "b"
	// Then a = 2 should fail because "a" is already an implicit table with children
	input := "a.b = 1\na = 2\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error: 'a' is already an implicit table with children, cannot assign value")
	}
}

// =============================================================================
// Edge case: empty key parts
// =============================================================================

func TestAuditEmptyQuotedKey(t *testing.T) {
	// TOML allows empty quoted keys: "" = "value"
	input := "\"\" = \"value\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.children[0].(*KeyValueNode)
	if kv.key.parts[0] != "" {
		t.Errorf("key = %q, want empty string", kv.key.parts[0])
	}
}

func TestAuditEmptyLiteralQuotedKey(t *testing.T) {
	// TOML allows empty literal keys: '' = "value"
	input := "'' = \"value\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.children[0].(*KeyValueNode)
	if kv.key.parts[0] != "" {
		t.Errorf("key = %q, want empty string", kv.key.parts[0])
	}
}
