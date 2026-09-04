package tomledit

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Audit Area 1: String escaping correctness
// =============================================================================

func TestAuditStringEscapeBackslash(t *testing.T) {
	n := &StringNode{val: scalarOf("a\\b"), style: StringBasic}
	n.markDirty()
	got := string(renderValue(n))
	want := "\"a\\\\b\""
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAuditStringEscapeDoubleQuote(t *testing.T) {
	n := &StringNode{val: scalarOf("say \"hello\""), style: StringBasic}
	n.markDirty()
	got := string(renderValue(n))
	want := "\"say \\\"hello\\\"\""
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAuditStringEscapeNamedControlChars(t *testing.T) {
	tests := []struct {
		val  string
		want string
	}{
		{"\b", "\"\\b\""},
		{"\t", "\"\\t\""},
		{"\n", "\"\\n\""},
		{"\f", "\"\\f\""},
		{"\r", "\"\\r\""},
	}
	for _, tt := range tests {
		n := &StringNode{val: scalarOf(tt.val), style: StringBasic}
		n.markDirty()
		got := string(renderValue(n))
		if got != tt.want {
			t.Errorf("renderBasicString(%q) = %q, want %q", tt.val, got, tt.want)
		}
	}
}

func TestAuditStringEscapeControlCharUnicodeEscape(t *testing.T) {
	// Control chars outside the named escapes should use \uXXXX
	tests := []struct {
		char rune
		esc  string
	}{
		{0x00, "\\u0000"},
		{0x01, "\\u0001"},
		{0x1F, "\\u001F"},
		{0x7F, "\\u007F"},
	}
	for _, tt := range tests {
		n := &StringNode{val: scalarOf(string(tt.char)), style: StringBasic}
		n.markDirty()
		got := string(renderValue(n))
		want := "\"" + tt.esc + "\""
		if got != want {
			t.Errorf("renderBasicString(0x%02X) = %q, want %q", tt.char, got, want)
		}
	}
}

func TestAuditStringNullByteRoundTrip(t *testing.T) {
	val := "before\x00after"
	n := &StringNode{val: scalarOf(val), style: StringBasic}
	n.markDirty()
	got := string(renderValue(n))
	if !strings.Contains(got, "\\u0000") {
		t.Errorf("null byte should be escaped as \\u0000, got %q", got)
	}
	// Re-parse to verify validity
	toml := "x = " + got + "\n"
	doc, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
	}
	kv := doc.children[0].(*KeyValueNode)
	sv := kv.val.(*StringNode)
	if sv.val.get() != val {
		t.Errorf("round-trip value mismatch: got %q, want %q", sv.val.get(), val)
	}
}

func TestAuditStringRoundTripAllStyles(t *testing.T) {
	tests := []struct {
		name  string
		val   string
		style StringStyle
	}{
		{"basic-simple", "hello", StringBasic},
		{"basic-escapes", "line1\nline2\ttab", StringBasic},
		{"literal-simple", "C:\\Users\\test", StringLiteral},
		{"multiline-basic", "line1\nline2\nline3", StringMultiLineBasic},
		{"multiline-literal", "line1\nline2\nline3", StringMultiLineLiteral},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &StringNode{val: scalarOf(tt.val), style: tt.style}
			n.markDirty()
			rendered := string(renderValue(n))
			toml := "x = " + rendered + "\n"
			doc, err := Parse([]byte(toml))
			if err != nil {
				t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
			}
			kv := doc.children[0].(*KeyValueNode)
			sv := kv.val.(*StringNode)
			if sv.val.get() != tt.val {
				t.Errorf("round-trip value mismatch: got %q, want %q", sv.val.get(), tt.val)
			}
		})
	}
}

func TestAuditStringLiteralFallbackOnNewline(t *testing.T) {
	n := &StringNode{val: scalarOf("line1\nline2"), style: StringLiteral}
	n.markDirty()
	got := string(renderValue(n))
	if !strings.HasPrefix(got, "\"") {
		t.Errorf("expected fallback to basic string, got %q", got)
	}
	if !strings.Contains(got, "\\n") {
		t.Errorf("newline should be escaped in basic string fallback, got %q", got)
	}
}

func TestAuditStringMultiLineLiteralFallbackOnTripleQuote(t *testing.T) {
	n := &StringNode{val: scalarOf("has ''' inside"), style: StringMultiLineLiteral}
	n.markDirty()
	got := string(renderValue(n))
	if !strings.HasPrefix(got, "\"\"\"") {
		t.Errorf("expected fallback to multi-line basic, got %q", got)
	}
}

func TestAuditMultiLineBasicStringQuoteHandling(t *testing.T) {
	n := &StringNode{val: scalarOf("say \"hello\" to \"world\""), style: StringMultiLineBasic}
	n.markDirty()
	got := string(renderValue(n))
	toml := "x = " + got + "\n"
	doc, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
	}
	kv := doc.children[0].(*KeyValueNode)
	sv := kv.val.(*StringNode)
	if sv.val.get() != "say \"hello\" to \"world\"" {
		t.Errorf("round-trip mismatch: got %q", sv.val.get())
	}
}

// =============================================================================
// Audit Area 2: Float rendering precision
// =============================================================================

func TestAuditFloatZero(t *testing.T) {
	n := &FloatNode{val: scalarOf(0.0)}
	n.markDirty()
	got := string(renderValue(n))
	if got != "0.0" {
		t.Errorf("0.0 should render as %q, got %q", "0.0", got)
	}
}

func TestAuditFloatNegativeZero(t *testing.T) {
	n := &FloatNode{val: scalarOf(math.Copysign(0, -1))}
	n.markDirty()
	got := string(renderValue(n))
	if !strings.Contains(got, ".") {
		t.Errorf("negative zero should have decimal point, got %q", got)
	}
}

func TestAuditFloatVerySmall(t *testing.T) {
	n := &FloatNode{val: scalarOf(0.000001)}
	n.markDirty()
	got := string(renderValue(n))
	if !strings.Contains(got, ".") {
		t.Errorf("very small float should have decimal point, got %q", got)
	}
	toml := "x = " + got + "\n"
	doc, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
	}
	kv := doc.children[0].(*KeyValueNode)
	fv := kv.val.(*FloatNode)
	if math.Abs(fv.val.get()-0.000001) > 1e-12 {
		t.Errorf("round-trip value mismatch: got %v, want 0.000001", fv.val.get())
	}
}

func TestAuditFloatVeryLarge(t *testing.T) {
	n := &FloatNode{val: scalarOf(1e15)}
	n.markDirty()
	got := string(renderValue(n))
	if !strings.Contains(got, ".") {
		t.Errorf("large float should have decimal point, got %q", got)
	}
	toml := "x = " + got + "\n"
	doc, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
	}
	kv := doc.children[0].(*KeyValueNode)
	fv := kv.val.(*FloatNode)
	if math.Abs(fv.val.get()-1e15) > 1 {
		t.Errorf("round-trip value mismatch: got %v, want 1e15", fv.val.get())
	}
}

func TestAuditFloatPrecision(t *testing.T) {
	tests := []struct {
		val  float64
		want string
	}{
		{3.141592653589793, "3.141592653589793"},
		{1.0, "1.0"},
		{0.1, "0.1"},
		{100.0, "100.0"},
	}
	for _, tt := range tests {
		n := &FloatNode{val: scalarOf(tt.val)}
		n.markDirty()
		got := string(renderValue(n))
		if got != tt.want {
			t.Errorf("renderFloat(%v) = %q, want %q", tt.val, got, tt.want)
		}
	}
}

func TestAuditFloatSpecialReparse(t *testing.T) {
	tests := []struct {
		val  float64
		repr string
	}{
		{math.Inf(1), "inf"},
		{math.Inf(-1), "-inf"},
		{math.NaN(), "nan"},
	}
	for _, tt := range tests {
		n := &FloatNode{val: scalarOf(tt.val)}
		n.markDirty()
		got := string(renderValue(n))
		if got != tt.repr {
			t.Errorf("renderFloat(%v) = %q, want %q", tt.val, got, tt.repr)
		}
		toml := "x = " + got + "\n"
		doc, err := Parse([]byte(toml))
		if err != nil {
			t.Fatalf("re-parse of %q failed: %v", toml, err)
		}
		kv := doc.children[0].(*KeyValueNode)
		fv := kv.val.(*FloatNode)
		if math.IsNaN(tt.val) {
			if !math.IsNaN(fv.val.get()) {
				t.Errorf("expected NaN, got %v", fv.val.get())
			}
		} else if fv.val.get() != tt.val {
			t.Errorf("round-trip mismatch: got %v, want %v", fv.val.get(), tt.val)
		}
	}
}

// =============================================================================
// Audit Area 3: Array rendering edge cases
// =============================================================================

func TestAuditArrayNestedArrays(t *testing.T) {
	inner1 := &ArrayNode{
		elements: []Node{
			&IntegerNode{nodeBase: nodeBase{dirty: true}, val: scalarOf[int64](1), base: IntegerDecimal},
			&IntegerNode{nodeBase: nodeBase{dirty: true}, val: scalarOf[int64](2), base: IntegerDecimal},
		},
	}
	inner1.markDirty()
	inner2 := &ArrayNode{
		elements: []Node{
			&IntegerNode{nodeBase: nodeBase{dirty: true}, val: scalarOf[int64](3), base: IntegerDecimal},
			&IntegerNode{nodeBase: nodeBase{dirty: true}, val: scalarOf[int64](4), base: IntegerDecimal},
		},
	}
	inner2.markDirty()

	outer := &ArrayNode{elements: []Node{inner1, inner2}}
	outer.markDirty()
	got := string(renderValue(outer))
	want := "[[1, 2], [3, 4]]"
	if got != want {
		t.Errorf("nested array: got %q, want %q", got, want)
	}
	toml := "x = " + got + "\n"
	_, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse of nested array failed: %v\nTOML: %s", err, toml)
	}
}

func TestAuditArrayWithInlineTables(t *testing.T) {
	k1 := &KeyNode{parts: []string{"x"}}
	k1.markDirty()
	v1 := &IntegerNode{val: scalarOf[int64](1), base: IntegerDecimal}
	v1.markDirty()
	kv1 := &KeyValueNode{key: k1, val: v1}
	kv1.markDirty()
	it1 := &InlineTableNode{children: []Node{kv1}}
	it1.markDirty()

	k2 := &KeyNode{parts: []string{"x"}}
	k2.markDirty()
	v2 := &IntegerNode{val: scalarOf[int64](2), base: IntegerDecimal}
	v2.markDirty()
	kv2 := &KeyValueNode{key: k2, val: v2}
	kv2.markDirty()
	it2 := &InlineTableNode{children: []Node{kv2}}
	it2.markDirty()

	arr := &ArrayNode{elements: []Node{it1, it2}}
	arr.markDirty()
	got := string(renderValue(arr))
	want := "[{x = 1}, {x = 2}]"
	if got != want {
		t.Errorf("array of inline tables: got %q, want %q", got, want)
	}
	toml := "x = " + got + "\n"
	_, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
	}
}

func TestAuditArrayWithMixedTypes(t *testing.T) {
	elems := []Node{
		&IntegerNode{nodeBase: nodeBase{dirty: true}, val: scalarOf[int64](1), base: IntegerDecimal},
		&StringNode{nodeBase: nodeBase{dirty: true}, val: scalarOf("two"), style: StringBasic},
		&FloatNode{nodeBase: nodeBase{dirty: true}, val: scalarOf(3.0)},
	}
	arr := &ArrayNode{elements: elems}
	arr.markDirty()
	got := string(renderValue(arr))
	want := "[1, \"two\", 3.0]"
	if got != want {
		t.Errorf("mixed array: got %q, want %q", got, want)
	}
}

func TestAuditEmptyArray(t *testing.T) {
	arr := &ArrayNode{}
	arr.markDirty()
	got := string(renderValue(arr))
	if got != "[]" {
		t.Errorf("empty array: got %q, want %q", got, "[]")
	}
}

// =============================================================================
// Audit Area 4: Tree walk correctness
// =============================================================================

func TestAuditTreeWalkNoDuplication(t *testing.T) {
	input := "[a]\nx = 1\ny = 2\n\n[b]\nz = 3\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(doc.Bytes())
	if got != input {
		t.Errorf("tree walk mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditTreeWalkWithArrayTables(t *testing.T) {
	input := "[[items]]\nname = \"one\"\n\n[[items]]\nname = \"two\"\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(doc.Bytes())
	if got != input {
		t.Errorf("tree walk mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditTreeWalkNestedTables(t *testing.T) {
	input := "[a]\nx = 1\n\n[a.b]\ny = 2\n\n[a.b.c]\nz = 3\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(doc.Bytes())
	if got != input {
		t.Errorf("tree walk mismatch:\n  input:  %q\n  output: %q", input, got)
	}
}

func TestAuditTreeWalkMixedDirtyPreservesOrder(t *testing.T) {
	input := "a = 1\nb = 2\nc = 3\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.children[1].(*KeyValueNode)
	intNode := kv.val.(*IntegerNode)
	intNode.setValue(99, IntegerDecimal)
	kv.markDirty()

	got := string(doc.Bytes())
	want := "a = 1\nb = 99\nc = 3\n"
	if got != want {
		t.Errorf("mixed dirty:\n  got:  %q\n  want: %q", got, want)
	}
}

// =============================================================================
// Audit Area 5: Key rendering
// =============================================================================

func TestAuditBareKeyAllowedChars(t *testing.T) {
	tests := []struct {
		key  string
		bare bool
	}{
		{"abc", true},
		{"ABC", true},
		{"a-b", true},
		{"a_b", true},
		{"123", true},
		{"a1b2", true},
		{"", false},
		{"a b", false},
		{"a.b", false},
		{"a=b", false},
		{"a[b", false},
		{"a]b", false},
		{"a{b", false},
		{"a}b", false},
		{"a#b", false},
		{"a\"b", false},
		{"a'b", false},
		{"a,b", false},
	}
	for _, tt := range tests {
		got := isBareKey(tt.key)
		if got != tt.bare {
			t.Errorf("isBareKey(%q) = %v, want %v", tt.key, got, tt.bare)
		}
	}
}

func TestAuditKeyWithSpecialCharsQuoted(t *testing.T) {
	k := &KeyNode{parts: []string{"key.with.dots"}}
	k.markDirty()
	got := string(renderKeyParts(k))
	if !strings.HasPrefix(got, "\"") {
		t.Errorf("key with dots should be quoted, got %q", got)
	}
}

func TestAuditKeyWithNewlineQuoted(t *testing.T) {
	k := &KeyNode{parts: []string{"key\nwith\nnewlines"}}
	k.markDirty()
	got := string(renderKeyParts(k))
	if !strings.HasPrefix(got, "\"") {
		t.Errorf("key with newlines should be quoted, got %q", got)
	}
	if !strings.Contains(got, "\\n") {
		t.Errorf("newlines in quoted key should be escaped, got %q", got)
	}
}

func TestAuditKeyRenderingRoundTrip(t *testing.T) {
	key := &KeyNode{parts: []string{"my key"}}
	key.markDirty()
	val := &IntegerNode{val: scalarOf[int64](42), base: IntegerDecimal}
	val.markDirty()
	kv := &KeyValueNode{key: key, val: val}
	kv.markDirty()

	doc := &Document{children: []Node{kv}}
	got := string(doc.Bytes())

	doc2, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, got)
	}
	kv2 := doc2.children[0].(*KeyValueNode)
	if kv2.key.parts[0] != "my key" {
		t.Errorf("key round-trip mismatch: got %q, want %q", kv2.key.parts[0], "my key")
	}
}

// =============================================================================
// Audit Area 6: Re-parse validation
// =============================================================================

func TestAuditReparseIntegerAllBases(t *testing.T) {
	tests := []struct {
		val  int64
		base IntegerBase
	}{
		{255, IntegerDecimal},
		{255, IntegerHex},
		{255, IntegerOctal},
		{255, IntegerBinary},
		{0, IntegerDecimal},
		{-42, IntegerDecimal},
	}
	for _, tt := range tests {
		n := &IntegerNode{val: scalarOf[int64](tt.val), base: tt.base}
		n.markDirty()
		rendered := string(renderValue(n))
		toml := "x = " + rendered + "\n"
		doc, err := Parse([]byte(toml))
		if err != nil {
			t.Fatalf("re-parse failed for base %d, val %d: %v\nTOML: %s", tt.base, tt.val, err, toml)
		}
		kv := doc.children[0].(*KeyValueNode)
		iv := kv.val.(*IntegerNode)
		if iv.val.get() != tt.val {
			t.Errorf("integer round-trip mismatch: got %d, want %d (base %d)", iv.val.get(), tt.val, tt.base)
		}
	}
}

func TestAuditReparseBooleans(t *testing.T) {
	for _, v := range []bool{true, false} {
		n := &BooleanNode{val: scalarOf(v)}
		n.markDirty()
		rendered := string(renderValue(n))
		toml := "x = " + rendered + "\n"
		doc, err := Parse([]byte(toml))
		if err != nil {
			t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
		}
		kv := doc.children[0].(*KeyValueNode)
		bv := kv.val.(*BooleanNode)
		if bv.val.get() != v {
			t.Errorf("boolean round-trip: got %v, want %v", bv.val.get(), v)
		}
	}
}

func TestAuditReparseDateTime(t *testing.T) {
	val := time.Date(2023, 12, 25, 10, 30, 45, 123456789, time.UTC)
	n := &DateTimeNode{val: scalarOf(val)}
	n.markDirty()
	rendered := string(renderValue(n))
	toml := "x = " + rendered + "\n"
	doc, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
	}
	kv := doc.children[0].(*KeyValueNode)
	dt := kv.val.(*DateTimeNode)
	if !dt.val.get().Equal(val) {
		t.Errorf("datetime round-trip: got %v, want %v", dt.val.get(), val)
	}
}

func TestAuditReparseLocalDateTime(t *testing.T) {
	ldt := LocalDateTime{Year: 2023, Month: 6, Day: 15, Hour: 14, Minute: 30, Second: 0, Nanosecond: 500000000}
	n := &LocalDateTimeNode{val: scalarOf(ldt)}
	n.markDirty()
	rendered := string(renderValue(n))
	toml := "x = " + rendered + "\n"
	doc, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
	}
	kv := doc.children[0].(*KeyValueNode)
	ldtn := kv.val.(*LocalDateTimeNode)
	if ldtn.val.get().Nanosecond != 500000000 {
		t.Errorf("local datetime nanosecond: got %d, want 500000000", ldtn.val.get().Nanosecond)
	}
}

func TestAuditReparseLocalDate(t *testing.T) {
	ld := LocalDate{Year: 2023, Month: 1, Day: 1}
	n := &LocalDateNode{val: scalarOf(ld)}
	n.markDirty()
	rendered := string(renderValue(n))
	toml := "x = " + rendered + "\n"
	doc, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
	}
	kv := doc.children[0].(*KeyValueNode)
	ldn := kv.val.(*LocalDateNode)
	if ldn.val.get() != ld {
		t.Errorf("local date round-trip: got %v, want %v", ldn.val.get(), ld)
	}
}

func TestAuditReparseLocalTime(t *testing.T) {
	lt := LocalTime{Hour: 23, Minute: 59, Second: 59, Nanosecond: 1}
	n := &LocalTimeNode{val: scalarOf(lt)}
	n.markDirty()
	rendered := string(renderValue(n))
	toml := "x = " + rendered + "\n"
	doc, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
	}
	kv := doc.children[0].(*KeyValueNode)
	ltn := kv.val.(*LocalTimeNode)
	if ltn.val.get().Nanosecond != 1 {
		t.Errorf("local time nanosecond: got %d, want 1", ltn.val.get().Nanosecond)
	}
}

func TestAuditReparseInlineTable(t *testing.T) {
	k1 := &KeyNode{parts: []string{"a"}}
	k1.markDirty()
	v1 := &StringNode{val: scalarOf("hello"), style: StringBasic}
	v1.markDirty()
	kv1 := &KeyValueNode{key: k1, val: v1}
	kv1.markDirty()

	k2 := &KeyNode{parts: []string{"b"}}
	k2.markDirty()
	v2 := &BooleanNode{val: scalarOf(true)}
	v2.markDirty()
	kv2 := &KeyValueNode{key: k2, val: v2}
	kv2.markDirty()

	it := &InlineTableNode{children: []Node{kv1, kv2}}
	it.markDirty()

	rendered := string(renderValue(it))
	toml := "x = " + rendered + "\n"
	_, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse of inline table failed: %v\nTOML: %s", err, toml)
	}
}

func TestAuditReparseArray(t *testing.T) {
	elems := []Node{
		&StringNode{nodeBase: nodeBase{dirty: true}, val: scalarOf("a"), style: StringBasic},
		&StringNode{nodeBase: nodeBase{dirty: true}, val: scalarOf("b"), style: StringBasic},
		&StringNode{nodeBase: nodeBase{dirty: true}, val: scalarOf("c"), style: StringBasic},
	}
	arr := &ArrayNode{elements: elems}
	arr.markDirty()

	rendered := string(renderValue(arr))
	toml := "x = " + rendered + "\n"
	doc, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse of array failed: %v\nTOML: %s", err, toml)
	}
	kv := doc.children[0].(*KeyValueNode)
	an := kv.val.(*ArrayNode)
	if len(an.elements) != 3 {
		t.Errorf("array elements: got %d, want 3", len(an.elements))
	}
}

// =============================================================================
// Audit Area 7: Complete dirty document round-trip
// =============================================================================

func TestAuditCompleteDocumentRoundTrip(t *testing.T) {
	titleKey := &KeyNode{parts: []string{"title"}}
	titleKey.markDirty()
	titleVal := &StringNode{val: scalarOf("My Config"), style: StringBasic}
	titleVal.markDirty()
	titleKV := &KeyValueNode{key: titleKey, val: titleVal}
	titleKV.markDirty()

	serverTbl := &TableNode{keyPath: []string{"server"}}
	serverTbl.markDirty()

	hostKey := &KeyNode{parts: []string{"host"}}
	hostKey.markDirty()
	hostVal := &StringNode{val: scalarOf("0.0.0.0"), style: StringBasic}
	hostVal.markDirty()
	hostKV := &KeyValueNode{key: hostKey, val: hostVal}
	hostKV.markDirty()

	portKey := &KeyNode{parts: []string{"port"}}
	portKey.markDirty()
	portVal := &IntegerNode{val: scalarOf[int64](443), base: IntegerDecimal}
	portVal.markDirty()
	portKV := &KeyValueNode{key: portKey, val: portVal}
	portKV.markDirty()

	mustSetContents(t, serverTbl, []Node{hostKV, portKV})

	item1 := &ArrayTableNode{keyPath: []string{"items"}}
	item1.markDirty()
	nameKey1 := &KeyNode{parts: []string{"name"}}
	nameKey1.markDirty()
	nameVal1 := &StringNode{val: scalarOf("widget"), style: StringBasic}
	nameVal1.markDirty()
	nameKV1 := &KeyValueNode{key: nameKey1, val: nameVal1}
	nameKV1.markDirty()
	mustSetContents(t, item1, []Node{nameKV1})

	item2 := &ArrayTableNode{keyPath: []string{"items"}}
	item2.markDirty()
	nameKey2 := &KeyNode{parts: []string{"name"}}
	nameKey2.markDirty()
	nameVal2 := &StringNode{val: scalarOf("gadget"), style: StringBasic}
	nameVal2.markDirty()
	nameKV2 := &KeyValueNode{key: nameKey2, val: nameVal2}
	nameKV2.markDirty()
	mustSetContents(t, item2, []Node{nameKV2})

	doc := &Document{children: []Node{titleKV, serverTbl, item1, item2}}
	out := doc.Bytes()

	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("complete document is not valid TOML: %v\noutput:\n%s", err, string(out))
	}

	if len(doc2.children) < 4 {
		t.Fatalf("expected at least 4 top-level children, got %d\noutput:\n%s", len(doc2.children), string(out))
	}

	out2 := doc2.Bytes()
	if string(out2) != string(out) {
		t.Errorf("double round-trip mismatch:\n  first:  %q\n  second: %q", string(out), string(out2))
	}
}

// =============================================================================
// Audit Area 8: Table header rendering edge cases
// =============================================================================

func TestAuditTableHeaderWithLeadingComments(t *testing.T) {
	tbl := &TableNode{keyPath: []string{"server"}}
	tbl.markDirty()
	tbl.nodeTrivia.LeadingComments = [][]byte{
		[]byte("# This is the server config\n"),
	}

	got := string(renderTableHeader(tbl))
	if !strings.Contains(got, "# This is the server config\n") {
		t.Errorf("leading comment missing from table header: %q", got)
	}
	if !strings.Contains(got, "[server]") {
		t.Errorf("table header missing: %q", got)
	}
}

func TestAuditArrayTableHeaderDottedKey(t *testing.T) {
	atbl := &ArrayTableNode{keyPath: []string{"a", "b", "c"}}
	atbl.markDirty()
	got := string(renderArrayTableHeader(atbl))
	if got != "[[a.b.c]]\n" {
		t.Errorf("got %q, want %q", got, "[[a.b.c]]\n")
	}
}

// =============================================================================
// Audit Area 9: Inline table KV modification
// =============================================================================

func TestAuditInlineTableKVWithoutRaw(t *testing.T) {
	input := "x = {a = 1, b = 2}\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	kv := doc.children[0].(*KeyValueNode)
	it := kv.val.(*InlineTableNode)

	innerKV := it.children[0].(*KeyValueNode)
	innerInt := innerKV.val.(*IntegerNode)
	innerInt.setValue(99, IntegerDecimal)
	innerKV.markDirty()
	it.markDirty()
	kv.markDirty()

	got := string(doc.Bytes())
	if !strings.Contains(got, "99") {
		t.Errorf("modified inline table value not rendered: %q", got)
	}
	_, err = Parse([]byte(got))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, got)
	}
}

// =============================================================================
// Audit Area 10: Trivia on dirty nodes
// =============================================================================

func TestAuditTriviaLeadingWhitespace(t *testing.T) {
	key := &KeyNode{parts: []string{"x"}}
	key.markDirty()
	val := &IntegerNode{val: scalarOf[int64](1), base: IntegerDecimal}
	val.markDirty()
	kv := &KeyValueNode{key: key, val: val}
	kv.markDirty()
	kv.nodeTrivia.LeadingWhitespace = []byte("  ")

	doc := &Document{children: []Node{kv}}
	got := string(doc.Bytes())
	if !strings.HasPrefix(got, "  x = 1") {
		t.Errorf("leading whitespace not preserved: %q", got)
	}
}

func TestAuditTriviaTrailingNewlineCRLF(t *testing.T) {
	key := &KeyNode{parts: []string{"x"}}
	key.markDirty()
	val := &IntegerNode{val: scalarOf[int64](1), base: IntegerDecimal}
	val.markDirty()
	kv := &KeyValueNode{key: key, val: val}
	kv.markDirty()
	kv.nodeTrivia.TrailingNewline = []byte("\r\n")

	doc := &Document{children: []Node{kv}}
	got := doc.Bytes()
	if !strings.HasSuffix(string(got), "\r\n") {
		t.Errorf("CRLF trailing newline not preserved: %q", string(got))
	}
}

func TestAuditTriviaMultipleLeadingComments(t *testing.T) {
	key := &KeyNode{parts: []string{"x"}}
	key.markDirty()
	val := &IntegerNode{val: scalarOf[int64](1), base: IntegerDecimal}
	val.markDirty()
	kv := &KeyValueNode{key: key, val: val}
	kv.markDirty()
	kv.nodeTrivia.LeadingComments = [][]byte{
		[]byte("# line 1\n"),
		[]byte("# line 2\n"),
	}

	doc := &Document{children: []Node{kv}}
	got := string(doc.Bytes())
	if !strings.Contains(got, "# line 1\n# line 2\n") {
		t.Errorf("multiple leading comments not preserved: %q", got)
	}
}

// =============================================================================
// Audit Area 11: Integer edge cases
// =============================================================================

func TestAuditIntegerNegativeHex(t *testing.T) {
	// TOML spec does NOT allow signs on hex/octal/binary integers.
	n := &IntegerNode{val: scalarOf[int64](-255), base: IntegerHex}
	n.markDirty()
	got := string(renderValue(n))
	toml := "x = " + got + "\n"
	_, err := Parse([]byte(toml))
	if err != nil {
		t.Logf("NOTE: Negative hex integer renders as %q which fails to parse: %v", got, err)
		t.Logf("This is a known limitation -- TOML spec does not allow signed hex/octal/binary")
	}
}

func TestAuditIntegerZeroAllBases(t *testing.T) {
	tests := []struct {
		base IntegerBase
		want string
	}{
		{IntegerDecimal, "0"},
		{IntegerHex, "0x0"},
		{IntegerOctal, "0o0"},
		{IntegerBinary, "0b0"},
	}
	for _, tt := range tests {
		n := &IntegerNode{val: scalarOf[int64](0), base: tt.base}
		n.markDirty()
		got := string(renderValue(n))
		if got != tt.want {
			t.Errorf("zero in base %d: got %q, want %q", tt.base, got, tt.want)
		}
	}
}

// =============================================================================
// Audit Area 12: Comment rendering edge cases
// =============================================================================

func TestAuditCommentEmpty(t *testing.T) {
	cn := &CommentNode{text: ""}
	cn.markDirty()
	got := string(serializeNode(cn))
	if got != "\n" {
		t.Errorf("empty comment: got %q, want %q", got, "\n")
	}
}

func TestAuditCommentWithNewline(t *testing.T) {
	cn := &CommentNode{text: "# already has newline\n"}
	cn.markDirty()
	got := string(serializeNode(cn))
	if strings.HasSuffix(got, "\n\n") {
		t.Errorf("double newline in comment: %q", got)
	}
}

// =============================================================================
// Audit Area 13: formatFractional
// =============================================================================

func TestAuditFormatFractional(t *testing.T) {
	tests := []struct {
		ns   int
		want string
	}{
		{100000000, "1"},
		{120000000, "12"},
		{123000000, "123"},
		{123456789, "123456789"},
		{1, "000000001"},
		{10, "00000001"},
		{999000000, "999"},
	}
	for _, tt := range tests {
		got := formatFractional(tt.ns)
		if got != tt.want {
			t.Errorf("formatFractional(%d) = %q, want %q", tt.ns, got, tt.want)
		}
	}
}

// =============================================================================
// Audit Area 14: isBareKey unicode behavior
// =============================================================================

func TestAuditBareKeyUnicode(t *testing.T) {
	// TOML 1.0 bare keys: A-Za-z0-9 and - and _
	// unicode.IsLetter() includes accented chars, CJK, etc.
	// This documents whether unicode bare keys work end-to-end.
	unicodeKeys := []string{
		"café", // cafe with e-acute
		"é",    // e with acute
	}
	for _, key := range unicodeKeys {
		bare := isBareKey(key)
		t.Logf("isBareKey(%q) = %v (unicode.IsLetter allows it)", key, bare)
		if bare {
			k := &KeyNode{parts: []string{key}}
			k.markDirty()
			v := &IntegerNode{val: scalarOf[int64](1), base: IntegerDecimal}
			v.markDirty()
			kv := &KeyValueNode{key: k, val: v}
			kv.markDirty()
			doc := &Document{children: []Node{kv}}
			out := string(doc.Bytes())
			_, err := Parse([]byte(out))
			if err != nil {
				t.Errorf("unicode bare key %q renders but fails to parse: %v\nTOML: %s", key, err, out)
			}
		}
	}
}

// =============================================================================
// Audit Area 15: Multi-line basic string with three consecutive quotes
// =============================================================================

func TestAuditMultiLineBasicStringWithThreeQuotes(t *testing.T) {
	// The value contains """ which is tricky for multi-line basic strings
	n := &StringNode{val: scalarOf("has \"\"\" inside"), style: StringMultiLineBasic}
	n.markDirty()
	got := string(renderValue(n))
	t.Logf("Rendered: %q", got)
	toml := "x = " + got + "\n"
	_, err := Parse([]byte(toml))
	if err != nil {
		t.Errorf("multi-line basic string with triple quotes fails to parse: %v\nTOML: %s", err, toml)
	}
}

// =============================================================================
// Audit Area 16: renderKeyParts clean key
// =============================================================================

func TestAuditRenderKeyPartsClean(t *testing.T) {
	input := "my_key = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	kv := doc.children[0].(*KeyValueNode)
	got := string(renderKeyParts(kv.key))
	if got != "my_key" {
		t.Errorf("clean key renderKeyParts: got %q, want %q", got, "my_key")
	}
}

// =============================================================================
// Audit Area 17: renderValue returns nil for unknown
// =============================================================================

func TestAuditRenderValueUnknownReturnsNil(t *testing.T) {
	doc := &Document{}
	doc.markDirty()
	got := renderValue(doc)
	if got != nil {
		t.Errorf("renderValue(Document) should return nil, got %q", got)
	}
}

// =============================================================================
// Audit Area 18: Clean CommentNode
// =============================================================================

func TestAuditSerializeCleanComment(t *testing.T) {
	input := "# this is a comment\nkey = 1\n"
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := string(doc.Bytes())
	if got != input {
		t.Errorf("clean comment mismatch:\n  got:  %q\n  want: %q", got, input)
	}
}

// =============================================================================
// Audit Area 19: Negative integer boundary
// =============================================================================

func TestAuditNegativeDecimalInteger(t *testing.T) {
	n := &IntegerNode{val: scalarOf[int64](-9223372036854775808), base: IntegerDecimal}
	n.markDirty()
	got := string(renderValue(n))
	toml := "x = " + got + "\n"
	doc, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
	}
	kv := doc.children[0].(*KeyValueNode)
	iv := kv.val.(*IntegerNode)
	if iv.val.get() != -9223372036854775808 {
		t.Errorf("min int64 round-trip: got %d", iv.val.get())
	}
}

// =============================================================================
// Audit Area 20: DateTime with timezone offset
// =============================================================================

func TestAuditDateTimeWithOffset(t *testing.T) {
	loc := time.FixedZone("EST", -5*3600)
	val := time.Date(2023, 6, 15, 10, 30, 0, 0, loc)
	n := &DateTimeNode{val: scalarOf(val)}
	n.markDirty()
	rendered := string(renderValue(n))
	t.Logf("Rendered datetime with offset: %s", rendered)
	toml := fmt.Sprintf("x = %s\n", rendered)
	doc, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("re-parse failed: %v\nTOML: %s", err, toml)
	}
	kv := doc.children[0].(*KeyValueNode)
	dt := kv.val.(*DateTimeNode)
	if !dt.val.get().Equal(val) {
		t.Errorf("datetime with offset round-trip: got %v, want %v", dt.val.get(), val)
	}
}
