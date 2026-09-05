package tomledit

import (
	"errors"
	"testing"
)

// Invalid UTF-8 at a write. A TOML document is UTF-8 text, and the renderers
// that write a string or a key decode what they are handed -- so an invalid
// byte becomes U+FFFD and the document reads back a value, or a name, the
// caller never wrote. Every operation that WRITES one refuses it with
// KindBadInput instead, which is what the []Pair input has always done for its
// keys; the exported renderers stay total, and no write reaches them with
// invalid input because the refusal comes first.

// badUTF8 is a string no TOML document can carry: two bytes that never appear
// in valid UTF-8.
const badUTF8 = "\xff\xfe"

// refuseWrite asserts an editing operation was refused as bad input, and that
// the document still reads exactly as it did before the call.
func refuseWrite(t *testing.T, doc *Document, before, op string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s was accepted; the document now reads %q", op, doc.Bytes())
	}
	if !errors.Is(err, ErrBadInput) {
		t.Errorf("%s reported %v, want a bad-input diagnostic", op, err)
	}
	if got := string(doc.Bytes()); got != before {
		t.Errorf("%s changed the document: %q, was %q", op, got, before)
	}
}

// --- string values ---

// Fails if a string value that is not valid UTF-8 reaches a document: it would
// be written as replacement characters and read back as a different string.
func TestSet_RefusesAStringValueThatIsNotUTF8(t *testing.T) {
	const src = "x = 1\n"
	doc := parseOrFail(t, src)
	refuseWrite(t, doc, src, "Set", doc.Set("x", badUTF8))
}

func TestSetCreate_RefusesAStringValueThatIsNotUTF8(t *testing.T) {
	const src = "x = 1\n"
	doc := parseOrFail(t, src)
	refuseWrite(t, doc, src, "SetCreate", doc.SetCreate("brand.new.key", badUTF8))
}

func TestAppendToArray_RefusesAStringValueThatIsNotUTF8(t *testing.T) {
	const src = "items = [1]\n"
	doc := parseOrFail(t, src)
	refuseWrite(t, doc, src, "AppendToArray", doc.AppendToArray("items", badUTF8))
}

func TestEnsureDefaults_RefusesAStringValueThatIsNotUTF8(t *testing.T) {
	const src = "x = 1\n"
	doc := parseOrFail(t, src)
	_, err := doc.EnsureDefaults([]Default{{Path: "server.host", Value: badUTF8}})
	refuseWrite(t, doc, src, "EnsureDefaults", err)
}

// Fails if an invalid string reaches a document from inside a container value:
// every container converts its elements through the same conversion, so the
// refusal is inherited rather than restated.
func TestSet_RefusesAnInvalidStringInsideAContainer(t *testing.T) {
	const src = "x = 1\n"
	cases := []struct {
		name  string
		value any
	}{
		{"slice", []any{"fine", badUTF8}},
		{"typed slice", []string{"fine", badUTF8}},
		{"map", map[string]any{"k": badUTF8}},
		{"pairs", []Pair{{Key: "k", Value: badUTF8}}},
		{"nested", []any{map[string]any{"k": []any{badUTF8}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseOrFail(t, src)
			refuseWrite(t, doc, src, "Set", doc.Set("x", tc.value))
		})
	}
}

// --- keys ---

// Fails if a map key that is not valid UTF-8 reaches a document: no quoting
// makes such a key writable, which is what the []Pair input already says.
func TestSet_RefusesAMapKeyThatIsNotUTF8(t *testing.T) {
	const src = "x = 1\n"
	doc := parseOrFail(t, src)
	refuseWrite(t, doc, src, "Set", doc.Set("x", map[string]any{badUTF8: 1}))
}

// Fails if a path key that is not valid UTF-8 is written: the key the path
// names is created from those very bytes.
func TestSet_RefusesAPathKeyThatIsNotUTF8(t *testing.T) {
	const src = "x = 1\n"
	doc := parseOrFail(t, src)
	refuseWrite(t, doc, src, "Set", doc.Set(pathOfKey(badUTF8), 1))
}

func TestSetCreate_RefusesAPathKeyThatIsNotUTF8(t *testing.T) {
	const src = "x = 1\n"
	doc := parseOrFail(t, src)
	refuseWrite(t, doc, src, "SetCreate", doc.SetCreate("brand."+pathOfKey(badUTF8)+".key", 1))
}

func TestNewTable_RefusesAPathKeyThatIsNotUTF8(t *testing.T) {
	const src = "x = 1\n"
	doc := parseOrFail(t, src)
	refuseWrite(t, doc, src, "NewTable", doc.NewTable(pathOfKey(badUTF8)))
}

func TestNewArrayTable_RefusesAPathKeyThatIsNotUTF8(t *testing.T) {
	const src = "x = 1\n"
	doc := parseOrFail(t, src)
	refuseWrite(t, doc, src, "NewArrayTable", doc.NewArrayTable(pathOfKey(badUTF8)))
}

// Fails if a rename can name a key that is not valid UTF-8: the new name is
// written into every construct that spells the old one out.
func TestRenameKey_RefusesATargetThatIsNotUTF8(t *testing.T) {
	const src = "x = 1\n"
	doc := parseOrFail(t, src)
	refuseWrite(t, doc, src, "RenameKey", doc.RenameKey("x", badUTF8))
}

// pathOfKey spells one key as a path ParsePath reads back as that single key,
// through the package's own path-quoting authority -- so the invalid bytes
// reach the write verbatim rather than being rewritten by the test.
func pathOfKey(key string) string {
	return JoinPath([]PathSegment{{Kind: SegmentKey, Key: key}})
}

// --- the renderers stay total ---

// Fails if the exported renderers stop being total on input no write path lets
// through. They are pure functions with no error to report, so they answer for
// whatever they are handed: an invalid byte renders as U+FFFD. Their inverse
// property is stated for valid UTF-8, which is all a write can carry.
func TestQuoteString_RendersInvalidUTF8AsReplacementCharacters(t *testing.T) {
	const want = "\"\uFFFD\uFFFD\""
	if got := QuoteString(badUTF8); got != want {
		t.Errorf("QuoteString rendered %q, want %q", got, want)
	}
	if got := QuoteKey(badUTF8); got != want {
		t.Errorf("QuoteKey rendered %q, want %q", got, want)
	}
}
