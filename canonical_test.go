package tomledit

import (
	"math"
	"testing"
)

// Canonical rendering: the bytes the library writes for a value it was handed,
// as opposed to the bytes it preserves for a value it read. Every rule below is
// the design record's, and each test names the change that would break it.

// setValueBytes writes value at key "x" of a one-key document and returns the
// bytes the document renders, which for a written value is its canonical form.
func setValueBytes(t *testing.T, value any) string {
	t.Helper()
	doc, err := Parse([]byte("x = 0\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := doc.Set("x", value); err != nil {
		t.Fatalf("Set(%v): %v", value, err)
	}
	out := string(doc.Bytes())
	const prefix, suffix = "x = ", "\n"
	if len(out) < len(prefix)+len(suffix) {
		t.Fatalf("rendered %q, which is not a key-value line", out)
	}
	return out[len(prefix) : len(out)-len(suffix)]
}

// Fails if a written float stops using the shortest round-trip form with an
// exponent -- the fixed-point formatting this replaced spelled 1e300 as 301
// digits.
func TestCanonicalFloatUsesShortestRoundTrip(t *testing.T) {
	cases := []struct {
		value float64
		want  string
	}{
		{1e300, "1e+300"},
		{1e-9, "1e-09"},
		{3.0, "3.0"},
		{0.5, "0.5"},
		{-2.5, "-2.5"},
		{0, "0.0"},
		{math.Copysign(0, -1), "-0.0"},
		{1.7976931348623157e308, "1.7976931348623157e+308"},
		{float64(0.1) + float64(0.2), "0.30000000000000004"},
	}
	for _, tc := range cases {
		if got := setValueBytes(t, tc.value); got != tc.want {
			t.Errorf("Set(%v) rendered %q, want %q", tc.value, got, tc.want)
		}
	}
}

// Fails if a written special float stops using the one spelling TOML and the
// record agree on: never "+inf", never "+nan" or "-nan".
func TestCanonicalFloatSpecials(t *testing.T) {
	cases := []struct {
		value float64
		want  string
	}{
		{math.Inf(1), "inf"},
		{math.Inf(-1), "-inf"},
		{math.NaN(), "nan"},
	}
	for _, tc := range cases {
		if got := setValueBytes(t, tc.value); got != tc.want {
			t.Errorf("Set(%v) rendered %q, want %q", tc.value, got, tc.want)
		}
	}
}

// Fails if a control character in a written string stops being escaped with
// lowercase hex digits, which is what the record pins and what the uppercase
// spelling this replaced did not do.
func TestCanonicalStringEscapesUseLowercaseHex(t *testing.T) {
	cases := []struct {
		value string
		want  string
	}{
		{"a\x1bb", `"a\u001bb"`},
		{"\x7f", `"\u007f"`},
		{"\x00", `"\u0000"`},
		{"\x1f", `"\u001f"`},
		{"tab\there", `"tab\there"`},
		{"nl\nhere", `"nl\nhere"`},
		{"cr\rhere", `"cr\rhere"`},
		{"bs\bhere", `"bs\bhere"`},
		{"ff\fhere", `"ff\fhere"`},
		{`quote"and\slash`, `"quote\"and\\slash"`},
		{"héllo — ünïcode", `"héllo — ünïcode"`},
	}
	for _, tc := range cases {
		if got := setValueBytes(t, tc.value); got != tc.want {
			t.Errorf("Set(%q) rendered %s, want %s", tc.value, got, tc.want)
		}
	}
}

// Fails if a written integer stops being plain decimal digits.
func TestCanonicalInteger(t *testing.T) {
	cases := []struct {
		value any
		want  string
	}{
		{42, "42"},
		{-42, "-42"},
		{int64(math.MaxInt64), "9223372036854775807"},
		{int64(math.MinInt64), "-9223372036854775808"},
		{uint32(7), "7"},
	}
	for _, tc := range cases {
		if got := setValueBytes(t, tc.value); got != tc.want {
			t.Errorf("Set(%v) rendered %q, want %q", tc.value, got, tc.want)
		}
	}
}

// Fails if a written boolean stops rendering as the bare TOML keyword.
func TestCanonicalBoolean(t *testing.T) {
	if got := setValueBytes(t, true); got != "true" {
		t.Errorf("Set(true) rendered %q, want \"true\"", got)
	}
	if got := setValueBytes(t, false); got != "false" {
		t.Errorf("Set(false) rendered %q, want \"false\"", got)
	}
}

// Fails if a written date-time stops rendering in the record's conventions:
// uppercase T, seconds always, fractional seconds trimmed of trailing zeros,
// "Z" for a zero offset.
func TestCanonicalDateTimes(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"offset utc", mustTime(t, "1979-05-27T07:32:00Z"), "1979-05-27T07:32:00Z"},
		{"offset west", mustTime(t, "1979-05-27T00:32:00-07:00"), "1979-05-27T00:32:00-07:00"},
		{"offset frac", mustTime(t, "1979-05-27T00:32:00.999999-07:00"), "1979-05-27T00:32:00.999999-07:00"},
		{"offset frac trimmed", mustTime(t, "1979-05-27T00:32:00.100000Z"), "1979-05-27T00:32:00.1Z"},
		{"local date-time", LocalDateTime{Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32, Second: 0}, "1979-05-27T07:32:00"},
		{"local date-time frac", LocalDateTime{Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32, Second: 0, Nanosecond: 500000000}, "1979-05-27T07:32:00.5"},
		{"local date", LocalDate{Year: 1979, Month: 5, Day: 27}, "1979-05-27"},
		{"local time", LocalTime{Hour: 7, Minute: 32, Second: 9}, "07:32:09"},
		{"local time frac", LocalTime{Hour: 0, Minute: 32, Second: 0, Nanosecond: 999000000}, "00:32:00.999"},
	}
	for _, tc := range cases {
		if got := setValueBytes(t, tc.value); got != tc.want {
			t.Errorf("%s: rendered %q, want %q", tc.name, got, tc.want)
		}
	}
}

func mustTime(t *testing.T, s string) any {
	t.Helper()
	v, err := parseOffsetDateTime(s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return v
}

// uEsc spells the four-digit unicode escape for a code point, built rather than
// written out so the test source carries no control characters of its own.
func uEsc(hex string) string { return "\\u" + hex }

// Fails if QuoteString stops producing a basic string that reads back as its
// input, or stops writing non-ASCII verbatim.
func TestQuoteString(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", `""`},
		{"plain", `"plain"`},
		{"with \"quotes\"", `"with \"quotes\""`},
		{`back\slash`, `"back\\slash"`},
		{"tab\t", `"tab\t"`},
		{"nl\n", `"nl\n"`},
		{"cr\r", `"cr\r"`},
		{"bs\b", `"bs\b"`},
		{"ff\f", `"ff\f"`},
		{"\x00", `"` + uEsc("0000") + `"`},
		{"\x1f", `"` + uEsc("001f") + `"`},
		{"\x7f", `"` + uEsc("007f") + `"`},
		{"héllo", `"héllo"`},
		{"日本語", `"日本語"`},
		{"emoji 🍕", `"emoji 🍕"`},
	}
	for _, tc := range cases {
		got := QuoteString(tc.in)
		if got != tc.want {
			t.Errorf("QuoteString(%q) = %s, want %s", tc.in, got, tc.want)
		}
		// The quoted form must read back as the value it was built from.
		doc, err := Parse([]byte("x = " + got + "\n"))
		if err != nil {
			t.Errorf("QuoteString(%q) produced %s, which does not parse: %v", tc.in, got, err)
			continue
		}
		if v, err := doc.GetString("x"); err != nil || v != tc.in {
			t.Errorf("QuoteString(%q) read back as (%q, %v)", tc.in, v, err)
		}
	}
}

// Fails if QuoteKey stops leaving a bare-writable key bare, or stops quoting one
// TOML's bare-key rule does not allow.
func TestQuoteKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"key", "key"},
		{"KEY-1_2", "KEY-1_2"},
		{"1234", "1234"},
		{"", `""`},
		{"with space", `"with space"`},
		{"with.dot", `"with.dot"`},
		{"héllo", `"héllo"`},
		{"quote\"here", `"quote\"here"`},
	}
	for _, tc := range cases {
		if got := QuoteKey(tc.in); got != tc.want {
			t.Errorf("QuoteKey(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

// Fails if FormatFloat stops being total -- every float64, the non-finite ones
// included, has exactly one output and no NaN spelling carries a sign.
func TestFormatFloatIsTotal(t *testing.T) {
	negNaN := math.Float64frombits(math.Float64bits(math.NaN()) | (1 << 63))
	if !math.IsNaN(negNaN) || !math.Signbit(negNaN) {
		t.Fatalf("the sign-bit NaN fixture is not one: %v", negNaN)
	}
	cases := []struct {
		name string
		in   float64
		want string
	}{
		{"nan", math.NaN(), "nan"},
		{"sign-bit nan", negNaN, "nan"},
		{"inf", math.Inf(1), "inf"},
		{"-inf", math.Inf(-1), "-inf"},
		{"zero", 0, "0.0"},
		{"negative zero", math.Copysign(0, -1), "-0.0"},
		{"whole", 42, "42.0"},
		{"fraction", 1.5, "1.5"},
		{"tiny", 5e-324, "5e-324"},
		{"huge", math.MaxFloat64, "1.7976931348623157e+308"},
	}
	for _, tc := range cases {
		if got := FormatFloat(tc.in); got != tc.want {
			t.Errorf("%s: FormatFloat = %q, want %q", tc.name, got, tc.want)
		}
	}

	// Every finite output reads back as the same float64.
	for _, f := range []float64{0, -0.5, 1, 1e300, 1e-300, math.Pi, math.SmallestNonzeroFloat64, math.MaxFloat64} {
		s := FormatFloat(f)
		doc, err := Parse([]byte("x = " + s + "\n"))
		if err != nil {
			t.Errorf("FormatFloat(%v) produced %q, which does not parse: %v", f, s, err)
			continue
		}
		back, err := doc.GetFloat("x")
		if err != nil {
			t.Errorf("FormatFloat(%v) produced %q, which does not read back as a float", f, s)
			continue
		}
		if back != f {
			t.Errorf("FormatFloat(%v) produced %q, which reads back as %v", f, s, back)
		}
	}
}
