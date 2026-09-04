package tomledit

import (
	"math"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// 1. Clean round-trip via Bytes() method
// =============================================================================

func TestBytesCleanRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple-kv", "key = \"value\"\n"},
		{"integer", "n = 42\n"},
		{"float", "f = 3.14\n"},
		{"bool", "b = true\n"},
		{"array", "a = [1, 2, 3]\n"},
		{"inline-table", "t = {x = 1, y = 2}\n"},
		{"table", "[server]\nhost = \"localhost\"\n"},
		{"array-table", "[[products]]\nname = \"Hammer\"\n"},
		{"comment-before-kv", "# comment\nkey = 1\n"},
		{"inline-comment", "key = 1 # inline\n"},
		{"multiple-tables", "[a]\nx = 1\n\n[b]\ny = 2\n"},
		{"dotted-key", "a.b.c = 1\n"},
		{"empty-doc", ""},
		{"only-newline", "\n"},
		{"blank-lines", "a = 1\n\nb = 2\n"},
		{"local-date", "d = 1979-05-27\n"},
		{"local-time", "t = 07:32:00\n"},
		{"offset-datetime", "dt = 1979-05-27T07:32:00Z\n"},
		{"multiline-basic", "s = \"\"\"\nhello\"\"\"\n"},
		{"multiline-literal", "s = '''\nhello'''\n"},
		{"literal-string", "s = 'hello'\n"},
		{"hex-integer", "n = 0xDEAD\n"},
		{"empty-inline-table", "t = {}\n"},
		{"empty-array", "a = []\n"},
		{"multiline-array", "a = [\n  1,\n  2,\n]\n"},
		{"complex", "# header\n[server]\nhost = \"0.0.0.0\" # primary\nport = 8080\n\n[[items]]\nname = \"one\"\n"},
		{"crlf", "[server]\r\nhost = \"localhost\"\r\nport = 8080\r\n"},
		{"whitespace-around-equals", "a\t=\t1\nb  =  2\nc=3\n"},
		{"comment-between-tables", "[a]\nx = 1\n\n# between tables\n\n[b]\ny = 2\n"},
		{"nested-inline-table", "t = {a = {b = 1}}\n"},
		{"array-of-inline-tables", "a = [{x = 1}, {x = 2}]\n"},
		{"whitespace-in-brackets", "[ server ]\nhost = \"localhost\"\n"},
		{"whitespace-in-double-brackets", "[[ items ]]\nname = \"one\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			got := string(doc.Bytes())
			if got != tt.input {
				t.Errorf("Bytes() round-trip mismatch:\n  input:  %q\n  output: %q", tt.input, got)
			}
		})
	}
}

func TestBytesCleanRoundTripComprehensive(t *testing.T) {
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
	got := string(doc.Bytes())
	if got != input {
		t.Errorf("Bytes() round-trip mismatch:\ninput len=%d\ngot len=%d", len(input), len(got))
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

// =============================================================================
// 2. Dirty leaf rendering: construct nodes programmatically (dirty), render them
// =============================================================================

func TestRenderStringBasic(t *testing.T) {
	n := &StringNode{val: scalarOf("hello world"), style: StringBasic}
	n.markDirty()
	got := string(renderValue(n))
	if got != `"hello world"` {
		t.Errorf("got %q, want %q", got, `"hello world"`)
	}
}

func TestRenderStringBasicEscapes(t *testing.T) {
	n := &StringNode{val: scalarOf("line1\nline2\ttab\\back\"quote"), style: StringBasic}
	n.markDirty()
	got := string(renderValue(n))
	expected := `"line1\nline2\ttab\\back\"quote"`
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestRenderStringBasicControlChars(t *testing.T) {
	n := &StringNode{val: scalarOf("hello" + string(rune(0)) + "world"), style: StringBasic}
	n.markDirty()
	got := string(renderValue(n))
	if !strings.Contains(got, "\\u0000") {
		t.Errorf("expected \\u0000 escape, got %q", got)
	}
}

func TestRenderStringLiteral(t *testing.T) {
	n := &StringNode{val: scalarOf(`C:\path\to\file`), style: StringLiteral}
	n.markDirty()
	got := string(renderValue(n))
	if got != `'C:\path\to\file'` {
		t.Errorf("got %q, want %q", got, `'C:\path\to\file'`)
	}
}

func TestRenderStringLiteralFallbackToBasic(t *testing.T) {
	// Literal strings cannot contain single quotes; should fall back to basic
	n := &StringNode{val: scalarOf("it's here"), style: StringLiteral}
	n.markDirty()
	got := string(renderValue(n))
	if !strings.HasPrefix(got, `"`) {
		t.Errorf("expected basic string fallback, got %q", got)
	}
	if !strings.Contains(got, "it's here") {
		t.Errorf("value should be preserved, got %q", got)
	}
}

func TestRenderStringMultiLineBasic(t *testing.T) {
	n := &StringNode{val: scalarOf("hello\nworld"), style: StringMultiLineBasic}
	n.markDirty()
	got := string(renderValue(n))
	if !strings.HasPrefix(got, `"""`+"\n") {
		t.Errorf("expected multi-line basic string prefix, got %q", got)
	}
	if !strings.HasSuffix(got, `"""`) {
		t.Errorf("expected multi-line basic string suffix, got %q", got)
	}
}

func TestRenderStringMultiLineLiteral(t *testing.T) {
	n := &StringNode{val: scalarOf("hello\nworld"), style: StringMultiLineLiteral}
	n.markDirty()
	got := string(renderValue(n))
	if !strings.HasPrefix(got, "'''\n") {
		t.Errorf("expected multi-line literal string prefix, got %q", got)
	}
	if !strings.HasSuffix(got, "'''") {
		t.Errorf("expected multi-line literal string suffix, got %q", got)
	}
}

func TestRenderIntegerDecimal(t *testing.T) {
	tests := []struct {
		val  int64
		want string
	}{
		{42, "42"},
		{-17, "-17"},
		{0, "0"},
		{1000, "1000"},
	}
	for _, tt := range tests {
		n := &IntegerNode{val: scalarOf[int64](tt.val), base: IntegerDecimal}
		n.markDirty()
		got := string(renderValue(n))
		if got != tt.want {
			t.Errorf("renderInteger(%d) = %q, want %q", tt.val, got, tt.want)
		}
	}
}

func TestRenderIntegerHex(t *testing.T) {
	n := &IntegerNode{val: scalarOf[int64](0xDEAD), base: IntegerHex}
	n.markDirty()
	got := string(renderValue(n))
	if got != "0xdead" {
		t.Errorf("got %q, want %q", got, "0xdead")
	}
}

func TestRenderIntegerOctal(t *testing.T) {
	n := &IntegerNode{val: scalarOf[int64](0o755), base: IntegerOctal}
	n.markDirty()
	got := string(renderValue(n))
	if got != "0o755" {
		t.Errorf("got %q, want %q", got, "0o755")
	}
}

func TestRenderIntegerBinary(t *testing.T) {
	n := &IntegerNode{val: scalarOf[int64](0b1101), base: IntegerBinary}
	n.markDirty()
	got := string(renderValue(n))
	if got != "0b1101" {
		t.Errorf("got %q, want %q", got, "0b1101")
	}
}

func TestRenderFloat(t *testing.T) {
	tests := []struct {
		val  float64
		want string
	}{
		{3.14, "3.14"},
		{1.0, "1.0"},
		{-0.5, "-0.5"},
		{100.0, "100.0"},
	}
	for _, tt := range tests {
		n := &FloatNode{val: scalarOf(tt.val)}
		n.markDirty()
		got := string(renderValue(n))
		if got != tt.want {
			t.Errorf("renderFloat(%f) = %q, want %q", tt.val, got, tt.want)
		}
	}
}

func TestRenderFloatSpecial(t *testing.T) {
	tests := []struct {
		val  float64
		want string
	}{
		{math.Inf(1), "inf"},
		{math.Inf(-1), "-inf"},
		{math.NaN(), "nan"},
	}
	for _, tt := range tests {
		n := &FloatNode{val: scalarOf(tt.val)}
		n.markDirty()
		got := string(renderValue(n))
		if got != tt.want {
			t.Errorf("renderFloat(special) = %q, want %q", got, tt.want)
		}
	}
}

func TestRenderBoolean(t *testing.T) {
	n1 := &BooleanNode{val: scalarOf(true)}
	n1.markDirty()
	if got := string(renderValue(n1)); got != "true" {
		t.Errorf("got %q, want %q", got, "true")
	}

	n2 := &BooleanNode{val: scalarOf(false)}
	n2.markDirty()
	if got := string(renderValue(n2)); got != "false" {
		t.Errorf("got %q, want %q", got, "false")
	}
}

func TestRenderDateTime(t *testing.T) {
	val := time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC)
	n := &DateTimeNode{val: scalarOf(val)}
	n.markDirty()
	got := string(renderValue(n))
	if !strings.Contains(got, "1979-05-27") {
		t.Errorf("expected date in output, got %q", got)
	}
	if !strings.Contains(got, "07:32:00") {
		t.Errorf("expected time in output, got %q", got)
	}
}

func TestRenderLocalDateTime(t *testing.T) {
	n := &LocalDateTimeNode{val: scalarOf(LocalDateTime{Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32, Second: 0})}
	n.markDirty()
	got := string(renderValue(n))
	if got != "1979-05-27T07:32:00" {
		t.Errorf("got %q, want %q", got, "1979-05-27T07:32:00")
	}
}

func TestRenderLocalDateTimeWithNanos(t *testing.T) {
	n := &LocalDateTimeNode{val: scalarOf(LocalDateTime{Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32, Second: 0, Nanosecond: 999000000})}
	n.markDirty()
	got := string(renderValue(n))
	if got != "1979-05-27T07:32:00.999" {
		t.Errorf("got %q, want %q", got, "1979-05-27T07:32:00.999")
	}
}

func TestRenderLocalDate(t *testing.T) {
	n := &LocalDateNode{val: scalarOf(LocalDate{Year: 1979, Month: 5, Day: 27})}
	n.markDirty()
	got := string(renderValue(n))
	if got != "1979-05-27" {
		t.Errorf("got %q, want %q", got, "1979-05-27")
	}
}

func TestRenderLocalTime(t *testing.T) {
	n := &LocalTimeNode{val: scalarOf(LocalTime{Hour: 7, Minute: 32, Second: 0})}
	n.markDirty()
	got := string(renderValue(n))
	if got != "07:32:00" {
		t.Errorf("got %q, want %q", got, "07:32:00")
	}
}

func TestRenderLocalTimeWithNanos(t *testing.T) {
	n := &LocalTimeNode{val: scalarOf(LocalTime{Hour: 7, Minute: 32, Second: 0, Nanosecond: 999000000})}
	n.markDirty()
	got := string(renderValue(n))
	if got != "07:32:00.999" {
		t.Errorf("got %q, want %q", got, "07:32:00.999")
	}
}

func TestRenderArrayEmpty(t *testing.T) {
	n := &ArrayNode{}
	n.markDirty()
	got := string(renderValue(n))
	if got != "[]" {
		t.Errorf("got %q, want %q", got, "[]")
	}
}

func TestRenderArraySingle(t *testing.T) {
	elem := &IntegerNode{val: scalarOf[int64](42), base: IntegerDecimal}
	elem.markDirty()
	n := &ArrayNode{elements: []Node{elem}}
	n.markDirty()
	got := string(renderValue(n))
	if got != "[42]" {
		t.Errorf("got %q, want %q", got, "[42]")
	}
}

func TestRenderArrayMulti(t *testing.T) {
	elems := []Node{
		&IntegerNode{nodeBase: nodeBase{dirty: true}, val: scalarOf[int64](1), base: IntegerDecimal},
		&IntegerNode{nodeBase: nodeBase{dirty: true}, val: scalarOf[int64](2), base: IntegerDecimal},
		&IntegerNode{nodeBase: nodeBase{dirty: true}, val: scalarOf[int64](3), base: IntegerDecimal},
	}
	n := &ArrayNode{elements: elems}
	n.markDirty()
	got := string(renderValue(n))
	if got != "[1, 2, 3]" {
		t.Errorf("got %q, want %q", got, "[1, 2, 3]")
	}
}

func TestRenderInlineTableEmpty(t *testing.T) {
	n := &InlineTableNode{}
	n.markDirty()
	got := string(renderValue(n))
	if got != "{}" {
		t.Errorf("got %q, want %q", got, "{}")
	}
}

func TestRenderInlineTableEntries(t *testing.T) {
	k1 := &KeyNode{parts: []string{"x"}}
	k1.markDirty()
	v1 := &IntegerNode{val: scalarOf[int64](1), base: IntegerDecimal}
	v1.markDirty()
	kv1 := &KeyValueNode{key: k1, val: v1}
	kv1.markDirty()

	k2 := &KeyNode{parts: []string{"y"}}
	k2.markDirty()
	v2 := &IntegerNode{val: scalarOf[int64](2), base: IntegerDecimal}
	v2.markDirty()
	kv2 := &KeyValueNode{key: k2, val: v2}
	kv2.markDirty()

	n := &InlineTableNode{children: []Node{kv1, kv2}}
	n.markDirty()
	got := string(renderValue(n))
	if got != "{x = 1, y = 2}" {
		t.Errorf("got %q, want %q", got, "{x = 1, y = 2}")
	}
}

// =============================================================================
// 3. Mixed clean/dirty: parse, modify one node, verify only that changes
// =============================================================================

func TestBytesMixedCleanDirty(t *testing.T) {
	input := "name = \"alice\"\nage = 30\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	// Modify the second key-value's value
	kv := doc.children[1].(*KeyValueNode)
	intNode := kv.val.(*IntegerNode)
	intNode.setValue(31, IntegerDecimal)
	kv.markDirty()

	got := string(doc.Bytes())
	expected := "name = \"alice\"\nage = 31\n"
	if got != expected {
		t.Errorf("mixed clean/dirty mismatch:\n  got:  %q\n  want: %q", got, expected)
	}
}

func TestBytesMixedCleanDirtyTable(t *testing.T) {
	input := "[server]\nhost = \"localhost\"\nport = 8080\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	// Modify the host value
	tbl := doc.children[0].(*TableNode)
	kv := tbl.children[0].(*KeyValueNode)
	strNode := kv.val.(*StringNode)
	strNode.setValue("0.0.0.0", StringBasic)
	kv.markDirty()

	got := string(doc.Bytes())
	// Table header should be unchanged, port should be unchanged
	if !strings.Contains(got, "[server]\n") {
		t.Errorf("table header should be preserved, got %q", got)
	}
	if !strings.Contains(got, "port = 8080") {
		t.Errorf("port should be preserved, got %q", got)
	}
	if !strings.Contains(got, "0.0.0.0") {
		t.Errorf("host should be changed, got %q", got)
	}
}

func TestBytesMixedPreservesComments(t *testing.T) {
	input := "# header comment\nname = \"alice\" # inline\nage = 30\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	// Only modify age
	kv := doc.children[1].(*KeyValueNode)
	intNode := kv.val.(*IntegerNode)
	intNode.setValue(31, IntegerDecimal)
	kv.markDirty()

	got := string(doc.Bytes())
	// The comment should be preserved exactly
	if !strings.Contains(got, "# header comment\n") {
		t.Errorf("leading comment should be preserved, got %q", got)
	}
	if !strings.Contains(got, "# inline") {
		t.Errorf("inline comment should be preserved, got %q", got)
	}
}

// =============================================================================
// 4. Trivia on dirty nodes
// =============================================================================

func TestBytesDirtyNodeWithComments(t *testing.T) {
	// Construct a dirty key-value with trivia set
	key := &KeyNode{parts: []string{"name"}}
	key.markDirty()
	val := &StringNode{val: scalarOf("alice"), style: StringBasic}
	val.markDirty()
	kv := &KeyValueNode{key: key, val: val}
	kv.markDirty()
	kv.nodeTrivia.InlineComment = []byte("# the name")

	doc := &Document{children: []Node{kv}}

	got := string(doc.Bytes())
	if !strings.Contains(got, "# the name") {
		t.Errorf("inline comment should appear, got %q", got)
	}
	if !strings.Contains(got, `name = "alice"`) {
		t.Errorf("key=value should appear, got %q", got)
	}
}

func TestBytesDirtyNodeWithLeadingComments(t *testing.T) {
	key := &KeyNode{parts: []string{"port"}}
	key.markDirty()
	val := &IntegerNode{val: scalarOf[int64](8080), base: IntegerDecimal}
	val.markDirty()
	kv := &KeyValueNode{key: key, val: val}
	kv.markDirty()
	kv.nodeTrivia.LeadingComments = [][]byte{
		[]byte("# port configuration\n"),
	}

	doc := &Document{children: []Node{kv}}

	got := string(doc.Bytes())
	if !strings.Contains(got, "# port configuration\n") {
		t.Errorf("leading comment should appear, got %q", got)
	}
	if !strings.Contains(got, "port = 8080") {
		t.Errorf("key=value should appear, got %q", got)
	}
}

// =============================================================================
// 5. New nodes: construct a Document with programmatic children, verify output
// =============================================================================

func TestBytesNewDocument(t *testing.T) {
	// Build a document entirely from programmatic nodes (all dirty)
	titleKey := &KeyNode{parts: []string{"title"}}
	titleKey.markDirty()
	titleVal := &StringNode{val: scalarOf("My App"), style: StringBasic}
	titleVal.markDirty()
	titleKV := &KeyValueNode{key: titleKey, val: titleVal}
	titleKV.markDirty()

	verKey := &KeyNode{parts: []string{"version"}}
	verKey.markDirty()
	verVal := &IntegerNode{val: scalarOf[int64](1), base: IntegerDecimal}
	verVal.markDirty()
	verKV := &KeyValueNode{key: verKey, val: verVal}
	verKV.markDirty()

	doc := &Document{children: []Node{titleKV, verKV}}
	got := string(doc.Bytes())

	// Verify it contains expected output
	if !strings.Contains(got, `title = "My App"`) {
		t.Errorf("expected title, got %q", got)
	}
	if !strings.Contains(got, "version = 1") {
		t.Errorf("expected version, got %q", got)
	}

	// Parse the output to verify it's valid TOML
	doc2, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("output is not valid TOML: %v\noutput: %q", err, got)
	}
	if len(doc2.children) != 2 {
		t.Errorf("parsed doc has %d children, want 2", len(doc2.children))
	}
}

func TestBytesNewDocumentWithTable(t *testing.T) {
	tbl := &TableNode{keyPath: []string{"server"}}
	tbl.markDirty()

	hostKey := &KeyNode{parts: []string{"host"}}
	hostKey.markDirty()
	hostVal := &StringNode{val: scalarOf("localhost"), style: StringBasic}
	hostVal.markDirty()
	hostKV := &KeyValueNode{key: hostKey, val: hostVal}
	hostKV.markDirty()
	mustSetContents(t, tbl, []Node{hostKV})

	doc := &Document{children: []Node{tbl}}
	got := string(doc.Bytes())

	if !strings.Contains(got, "[server]") {
		t.Errorf("expected table header, got %q", got)
	}
	if !strings.Contains(got, `host = "localhost"`) {
		t.Errorf("expected host, got %q", got)
	}

	// Parse the output to verify it's valid TOML
	_, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("output is not valid TOML: %v\noutput: %q", err, got)
	}
}

func TestBytesNewDocumentWithArrayTable(t *testing.T) {
	atbl := &ArrayTableNode{keyPath: []string{"items"}}
	atbl.markDirty()

	nameKey := &KeyNode{parts: []string{"name"}}
	nameKey.markDirty()
	nameVal := &StringNode{val: scalarOf("one"), style: StringBasic}
	nameVal.markDirty()
	nameKV := &KeyValueNode{key: nameKey, val: nameVal}
	nameKV.markDirty()
	mustSetContents(t, atbl, []Node{nameKV})

	doc := &Document{children: []Node{atbl}}
	got := string(doc.Bytes())

	if !strings.Contains(got, "[[items]]") {
		t.Errorf("expected array table header, got %q", got)
	}
	if !strings.Contains(got, `name = "one"`) {
		t.Errorf("expected name, got %q", got)
	}

	// Parse the output to verify it's valid TOML
	_, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("output is not valid TOML: %v\noutput: %q", err, got)
	}
}

// =============================================================================
// 6. Key rendering
// =============================================================================

func TestRenderBareKey(t *testing.T) {
	k := &KeyNode{parts: []string{"simple"}}
	k.markDirty()
	got := string(renderKey(k))
	if got != "simple" {
		t.Errorf("got %q, want %q", got, "simple")
	}
}

func TestRenderQuotedKey(t *testing.T) {
	k := &KeyNode{parts: []string{"key with spaces"}}
	k.markDirty()
	got := string(renderKey(k))
	if got != `"key with spaces"` {
		t.Errorf("got %q, want %q", got, `"key with spaces"`)
	}
}

func TestRenderDottedKey(t *testing.T) {
	k := &KeyNode{parts: []string{"a", "b", "c"}}
	k.markDirty()
	got := string(renderKey(k))
	if got != "a.b.c" {
		t.Errorf("got %q, want %q", got, "a.b.c")
	}
}

func TestRenderMixedDottedKey(t *testing.T) {
	k := &KeyNode{parts: []string{"server", "host name", "port"}}
	k.markDirty()
	got := string(renderKey(k))
	if got != `server."host name".port` {
		t.Errorf("got %q, want %q", got, `server."host name".port`)
	}
}

func TestRenderEmptyKey(t *testing.T) {
	// Empty key must be quoted
	k := &KeyNode{parts: []string{""}}
	k.markDirty()
	got := string(renderKey(k))
	if got != `""` {
		t.Errorf("got %q, want %q", got, `""`)
	}
}

// =============================================================================
// 7. isBareKey
// =============================================================================

func TestIsBareKey(t *testing.T) {
	tests := []struct {
		key  string
		bare bool
	}{
		{"simple", true},
		{"with-dash", true},
		{"with_underscore", true},
		{"123", true},
		{"abc123", true},
		{"", false},
		{"has space", false},
		{"has.dot", false},
		{"has\"quote", false},
		{"has=equals", false},
		{"has[bracket", false},
	}
	for _, tt := range tests {
		got := isBareKey(tt.key)
		if got != tt.bare {
			t.Errorf("isBareKey(%q) = %v, want %v", tt.key, got, tt.bare)
		}
	}
}

// =============================================================================
// 8. Table header rendering
// =============================================================================

func TestRenderTableHeaderSimple(t *testing.T) {
	tbl := &TableNode{keyPath: []string{"server"}}
	tbl.markDirty()
	got := string(renderTableHeader(tbl))
	if got != "[server]\n" {
		t.Errorf("got %q, want %q", got, "[server]\n")
	}
}

func TestRenderTableHeaderDotted(t *testing.T) {
	tbl := &TableNode{keyPath: []string{"server", "tls"}}
	tbl.markDirty()
	got := string(renderTableHeader(tbl))
	if got != "[server.tls]\n" {
		t.Errorf("got %q, want %q", got, "[server.tls]\n")
	}
}

func TestRenderTableHeaderWithQuotedKey(t *testing.T) {
	tbl := &TableNode{keyPath: []string{"server", "host name"}}
	tbl.markDirty()
	got := string(renderTableHeader(tbl))
	if got != `[server."host name"]`+"\n" {
		t.Errorf("got %q, want %q", got, `[server."host name"]`+"\n")
	}
}

func TestRenderTableHeaderWithInlineComment(t *testing.T) {
	tbl := &TableNode{keyPath: []string{"server"}}
	tbl.markDirty()
	tbl.nodeTrivia.InlineComment = []byte("# config")
	got := string(renderTableHeader(tbl))
	if got != "[server] # config\n" {
		t.Errorf("got %q, want %q", got, "[server] # config\n")
	}
}

func TestRenderArrayTableHeaderSimple(t *testing.T) {
	atbl := &ArrayTableNode{keyPath: []string{"items"}}
	atbl.markDirty()
	got := string(renderArrayTableHeader(atbl))
	if got != "[[items]]\n" {
		t.Errorf("got %q, want %q", got, "[[items]]\n")
	}
}

func TestRenderArrayTableHeaderWithComment(t *testing.T) {
	atbl := &ArrayTableNode{keyPath: []string{"items"}}
	atbl.markDirty()
	atbl.nodeTrivia.InlineComment = []byte("# list")
	got := string(renderArrayTableHeader(atbl))
	if got != "[[items]] # list\n" {
		t.Errorf("got %q, want %q", got, "[[items]] # list\n")
	}
}

// =============================================================================
// 9. Comment rendering
// =============================================================================

func TestRenderCommentDirty(t *testing.T) {
	cn := &CommentNode{text: "hello"}
	cn.markDirty()
	got := string(serializeNode(cn))
	if got != "# hello\n" {
		t.Errorf("got %q, want %q", got, "# hello\n")
	}
}

func TestRenderCommentAlreadyHasHash(t *testing.T) {
	cn := &CommentNode{text: "# already has hash"}
	cn.markDirty()
	got := string(serializeNode(cn))
	if got != "# already has hash\n" {
		t.Errorf("got %q, want %q", got, "# already has hash\n")
	}
}

// =============================================================================
// 10. Semantic equivalence: round-trip through Bytes() then Parse
// =============================================================================

func TestBytesOutputParseable(t *testing.T) {
	// Construct a complex document programmatically
	titleKey := &KeyNode{parts: []string{"title"}}
	titleKey.markDirty()
	titleVal := &StringNode{val: scalarOf("Test"), style: StringBasic}
	titleVal.markDirty()
	titleKV := &KeyValueNode{key: titleKey, val: titleVal}
	titleKV.markDirty()

	debugKey := &KeyNode{parts: []string{"debug"}}
	debugKey.markDirty()
	debugVal := &BooleanNode{val: scalarOf(true)}
	debugVal.markDirty()
	debugKV := &KeyValueNode{key: debugKey, val: debugVal}
	debugKV.markDirty()

	piKey := &KeyNode{parts: []string{"pi"}}
	piKey.markDirty()
	piVal := &FloatNode{val: scalarOf(3.14159)}
	piVal.markDirty()
	piKV := &KeyValueNode{key: piKey, val: piVal}
	piKV.markDirty()

	tbl := &TableNode{keyPath: []string{"server"}}
	tbl.markDirty()
	portKey := &KeyNode{parts: []string{"port"}}
	portKey.markDirty()
	portVal := &IntegerNode{val: scalarOf[int64](8080), base: IntegerDecimal}
	portVal.markDirty()
	portKV := &KeyValueNode{key: portKey, val: portVal}
	portKV.markDirty()
	mustSetContents(t, tbl, []Node{portKV})

	doc := &Document{children: []Node{titleKV, debugKV, piKV, tbl}}
	out := doc.Bytes()

	// Parse the output
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("Bytes() output is not valid TOML: %v\noutput: %q", err, string(out))
	}

	// Verify semantic equivalence
	if len(doc2.children) < 4 {
		t.Fatalf("parsed doc has %d children, want at least 4", len(doc2.children))
	}

	// Check title
	kv := doc2.children[0].(*KeyValueNode)
	if kv.key.parts[0] != "title" {
		t.Errorf("first key = %q, want %q", kv.key.parts[0], "title")
	}
	sv := kv.val.(*StringNode)
	if sv.val.get() != "Test" {
		t.Errorf("title = %q, want %q", sv.val.get(), "Test")
	}

	// Check debug
	kv = doc2.children[1].(*KeyValueNode)
	bv := kv.val.(*BooleanNode)
	if bv.val.get() != true {
		t.Error("debug should be true")
	}

	// Check pi
	kv = doc2.children[2].(*KeyValueNode)
	fv := kv.val.(*FloatNode)
	if math.Abs(fv.val.get()-3.14159) > 0.00001 {
		t.Errorf("pi = %f, want 3.14159", fv.val.get())
	}

	// Check server table
	tbl2 := doc2.children[3].(*TableNode)
	if tbl2.keyPath[0] != "server" {
		t.Errorf("table key = %v, want [server]", tbl2.keyPath)
	}
	portKV2 := tbl2.children[0].(*KeyValueNode)
	iv := portKV2.val.(*IntegerNode)
	if iv.val.get() != 8080 {
		t.Errorf("port = %d, want 8080", iv.val.get())
	}
}

// =============================================================================
// 11. Edge cases
// =============================================================================

func TestBytesEmptyDocument(t *testing.T) {
	doc := &Document{}
	got := doc.Bytes()
	if len(got) != 0 {
		t.Errorf("expected empty bytes, got %q", got)
	}
}

func TestBytesDirtyOnlyValue(t *testing.T) {
	// Parse a doc, only mark the value dirty (not the KV node).
	// Even though the KV itself is clean, a dirty descendant must trigger
	// re-rendering so the edit is not silently lost.
	input := "name = \"alice\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	kv := doc.children[0].(*KeyValueNode)
	strNode := kv.val.(*StringNode)
	strNode.setValue("bob", StringBasic)
	// Note: kv itself is NOT marked dirty

	got := string(doc.Bytes())
	// The dirty value subtree must cause re-rendering so "bob" appears
	want := "name = \"bob\"\n"
	if got != want {
		t.Errorf("dirty value should trigger re-render, got %q, want %q", got, want)
	}
}

func TestBytesMultipleArrayTablesDirty(t *testing.T) {
	atbl1 := &ArrayTableNode{keyPath: []string{"items"}}
	atbl1.markDirty()
	name1Key := &KeyNode{parts: []string{"name"}}
	name1Key.markDirty()
	name1Val := &StringNode{val: scalarOf("one"), style: StringBasic}
	name1Val.markDirty()
	name1KV := &KeyValueNode{key: name1Key, val: name1Val}
	name1KV.markDirty()
	mustSetContents(t, atbl1, []Node{name1KV})

	atbl2 := &ArrayTableNode{keyPath: []string{"items"}}
	atbl2.markDirty()
	name2Key := &KeyNode{parts: []string{"name"}}
	name2Key.markDirty()
	name2Val := &StringNode{val: scalarOf("two"), style: StringBasic}
	name2Val.markDirty()
	name2KV := &KeyValueNode{key: name2Key, val: name2Val}
	name2KV.markDirty()
	mustSetContents(t, atbl2, []Node{name2KV})

	doc := &Document{children: []Node{atbl1, atbl2}}
	got := string(doc.Bytes())

	// Count occurrences of [[items]]
	count := strings.Count(got, "[[items]]")
	if count != 2 {
		t.Errorf("expected 2 array table headers, got %d\noutput: %q", count, got)
	}

	// Parse the output to verify validity
	_, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("output is not valid TOML: %v\noutput: %q", err, got)
	}
}

func TestRenderStringWithUnicode(t *testing.T) {
	n := &StringNode{val: scalarOf("café"), style: StringBasic}
	n.markDirty()
	got := string(renderValue(n))
	// Should contain the Unicode combining accent (no escaping needed for printable chars)
	if !strings.Contains(got, "café") {
		t.Errorf("expected Unicode preserved, got %q", got)
	}
}

func TestRenderKeyValueDottedDirty(t *testing.T) {
	key := &KeyNode{parts: []string{"a", "b", "c"}}
	key.markDirty()
	val := &IntegerNode{val: scalarOf[int64](42), base: IntegerDecimal}
	val.markDirty()
	kv := &KeyValueNode{key: key, val: val}
	kv.markDirty()

	doc := &Document{children: []Node{kv}}
	got := string(doc.Bytes())
	if !strings.Contains(got, "a.b.c = 42") {
		t.Errorf("expected dotted key, got %q", got)
	}
}
