package tomledit

import (
	"errors"
	"strings"
	"testing"
)

// EnsureDefaults: seeding the paths a document does not carry.

// Fails if a present key is overwritten or a missing one is not added -- the
// operation's whole contract, at one level.
func TestEnsureDefaults_SeedsOnlyMissingPaths(t *testing.T) {
	doc := parseOrFail(t, "[server]\nhost = \"myhost\"\nport = 9090\n")
	added, err := doc.EnsureDefaults([]Default{
		{"server.host", "localhost"},
		{"server.port", 8080},
		{"server.workers", 4},
	})
	if err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	if strings.Join(added, ",") != "server.workers" {
		t.Errorf("added = %v, want only server.workers", added)
	}
	if host, _ := doc.GetString("server.host"); host != "myhost" {
		t.Errorf("host = %q, want the document's own value", host)
	}
	if port, _ := doc.GetInt("server.port"); port != 9090 {
		t.Errorf("port = %d, want the document's own value", port)
	}
	if workers, err := doc.GetInt("server.workers"); err != nil || workers != 4 {
		t.Errorf("workers = %d (ok=%v), want 4", workers, err)
	}
	roundTrip(t, doc)
}

// Fails if presence stops being read through the read-layer: a key written as
// a dotted key, inside a header table or inside an inline table is present
// whichever spelling the document used, and so is a table a longer header
// implies.
func TestEnsureDefaults_PresenceIsSpellingBlind(t *testing.T) {
	doc := parseOrFail(t, "dotted.key = 1\ninline = { key = 2 }\n[a.b]\nx = 3\n")
	added, err := doc.EnsureDefaults([]Default{
		{"dotted.key", 99},
		{"inline.key", 99},
		{"a.b.x", 99},
		{"a", 99},
	})
	if err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("added = %v, want nothing: every path is already carried", added)
	}
	if got := string(doc.Bytes()); got != "dotted.key = 1\ninline = { key = 2 }\n[a.b]\nx = 3\n" {
		t.Errorf("the document was changed: %q", got)
	}
}

// Fails if seeding at depth stops preserving what is there: nothing at any
// level may be overwritten, and every missing key must arrive.
func TestEnsureDefaults_ThreeLevelsDeep(t *testing.T) {
	doc := parseOrFail(t, "[a]\nx = 1\n\n[a.b]\ny = 2\n\n[a.b.c]\nz = 3\n")
	added, err := doc.EnsureDefaults([]Default{
		{"a.x", 999},
		{"a.w", 10},
		{"a.b.y", 999},
		{"a.b.v", 20},
		{"a.b.c.z", 999},
		{"a.b.c.u", 30},
	})
	if err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	if strings.Join(added, ",") != "a.w,a.b.v,a.b.c.u" {
		t.Errorf("added = %v, want the three missing paths in the order given", added)
	}
	for path, want := range map[string]int64{
		"a.x": 1, "a.w": 10, "a.b.y": 2, "a.b.v": 20, "a.b.c.z": 3, "a.b.c.u": 30,
	} {
		if got, err := doc.GetInt(path); err != nil || got != want {
			t.Errorf("%s = %d (ok=%v), want %d", path, got, err, want)
		}
	}
	roundTrip(t, doc)
}

// Fails if seeding into an empty document stops working, or stops creating its
// intermediate tables as standard headers.
func TestEnsureDefaults_CreatesStandardTableIntermediates(t *testing.T) {
	doc := parseOrFail(t, "")
	added, err := doc.EnsureDefaults([]Default{
		{"server.host", "localhost"},
		{"server.port", 8080},
	})
	if err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	if len(added) != 2 {
		t.Errorf("added = %v, want both paths", added)
	}
	got := string(doc.Bytes())
	if !strings.Contains(got, "[server]") {
		t.Errorf("the intermediate table is not a standard header:\n%s", got)
	}
	if strings.Contains(got, "{") {
		t.Errorf("an inline table was written where a header belongs:\n%s", got)
	}
	roundTrip(t, doc)
}

// Fails if the same list against the same document stops writing the same
// bytes: with the input ordered and the intermediates standard tables, the
// result cannot depend on map iteration or on which intermediate came first.
func TestEnsureDefaults_IsDeterministic(t *testing.T) {
	defaults := []Default{
		{"z.deep.key", "z"},
		{"a", 1},
		{"m.list", []any{1, 2, 3}},
		{"m.pairs", []Pair{{"b", 2}, {"a", 1}}},
		{"m.table", map[string]any{"q": 1, "p": 2}},
		{"z.deep.other", "y"},
	}
	const src = "existing = true\n[m]\nkept = 1\n"

	first := parseOrFail(t, src)
	addedFirst, err := first.EnsureDefaults(defaults)
	if err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	want := string(first.Bytes())

	for i := 0; i < 5; i++ {
		again := parseOrFail(t, src)
		addedAgain, err := again.EnsureDefaults(defaults)
		if err != nil {
			t.Fatalf("EnsureDefaults: %v", err)
		}
		if got := string(again.Bytes()); got != want {
			t.Fatalf("run %d wrote different bytes:\n%s\nfirst run:\n%s", i, got, want)
		}
		if strings.Join(addedAgain, ",") != strings.Join(addedFirst, ",") {
			t.Fatalf("run %d added %v, first run added %v", i, addedAgain, addedFirst)
		}
	}
	roundTrip(t, first)
}

// Fails if seeding stops being idempotent: a second pass with the same list
// finds every path present and adds nothing.
func TestEnsureDefaults_SecondPassAddsNothing(t *testing.T) {
	doc := parseOrFail(t, "kept = 1\n")
	defaults := []Default{{"a.b", 1}, {"c", 2}}
	if _, err := doc.EnsureDefaults(defaults); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	before := string(doc.Bytes())
	added, err := doc.EnsureDefaults(defaults)
	if err != nil {
		t.Fatalf("EnsureDefaults (second pass): %v", err)
	}
	if len(added) != 0 {
		t.Errorf("the second pass added %v, want nothing", added)
	}
	if got := string(doc.Bytes()); got != before {
		t.Errorf("the second pass rewrote the document:\n%s\nwant\n%s", got, before)
	}
}

// Fails if the partial-application contract changes: seeding stops at the
// first error, what was written before it stays written, and added names
// exactly those paths.
func TestEnsureDefaults_ReportsWhatItAddedBeforeAnError(t *testing.T) {
	doc := parseOrFail(t, "[t]\nx = 1\n")
	added, err := doc.EnsureDefaults([]Default{
		{"first", 1},
		{"second", make(chan int)}, // no TOML value stands for a channel
		{"third", 3},
	})
	if err == nil {
		t.Fatalf("EnsureDefaults reported success; the document reads %q", doc.Bytes())
	}
	if !errors.Is(err, ErrBadInput) {
		t.Errorf("the error is %v, want the refusal of the unconvertible value", err)
	}
	if strings.Join(added, ",") != "first" {
		t.Errorf("added = %v, want the one path written before the failure", added)
	}
	if !doc.Has("first") {
		t.Error("the path written before the failure was rolled back")
	}
	if doc.Has("third") {
		t.Error("seeding continued past the failure")
	}
}

// Fails if an existing array is merged element by element instead of being
// left alone: a value the document carries is never touched, whatever its
// kind.
func TestEnsureDefaults_ExistingContainersAreAtomic(t *testing.T) {
	doc := parseOrFail(t, "tags = [\"a\", \"b\"]\ntbl = { k = 1 }\n")
	added, err := doc.EnsureDefaults([]Default{
		{"tags", []any{"x", "y", "z"}},
		{"tbl", map[string]any{"k": 9, "extra": 1}},
	})
	if err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("added = %v, want nothing", added)
	}
	if got := string(doc.Bytes()); got != "tags = [\"a\", \"b\"]\ntbl = { k = 1 }\n" {
		t.Errorf("an existing container was changed: %q", got)
	}
}

// Fails if a default value stops being converted the way Set converts one,
// ordered inline tables included.
func TestEnsureDefaults_WritesEveryValueKind(t *testing.T) {
	doc := parseOrFail(t, "")
	if _, err := doc.EnsureDefaults([]Default{
		{"s", "text"},
		{"n", 42},
		{"f", 1.5},
		{"b", true},
		{"list", []any{1, "two"}},
		{"ordered", []Pair{{"first", 1}, {"second", 2}}},
	}); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	const want = "s = \"text\"\nn = 42\nf = 1.5\nb = true\nlist = [1, \"two\"]\nordered = {first = 1, second = 2}\n"
	if got := string(doc.Bytes()); got != want {
		t.Errorf("the document reads\n%q\nwant\n%q", got, want)
	}
	roundTrip(t, doc)
}

// Fails if an empty list stops being the no-op it is.
func TestEnsureDefaults_EmptyListChangesNothing(t *testing.T) {
	doc := parseOrFail(t, "x = 1\n")
	added, err := doc.EnsureDefaults(nil)
	if err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	if added != nil {
		t.Errorf("added = %v, want nil", added)
	}
	if got := string(doc.Bytes()); got != "x = 1\n" {
		t.Errorf("the document was changed: %q", got)
	}
}
