package tomledit

import (
	"errors"
	"testing"
)

// RenameKey renames a BINDING, whatever constructs spell it out: the pairs
// written under the name and the headers of the tables written under it. The
// renamed key part is the only fragment that stops splicing, so the rest of
// every construct it touches comes back byte-identical.

// renamed parses src, renames path to newKey and returns what the document
// renders.
func renamed(t *testing.T, src, path, newKey string) string {
	t.Helper()
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if err := doc.RenameKey(path, newKey); err != nil {
		t.Fatalf("RenameKey(%q, %q) on %q: %v", path, newKey, src, err)
	}
	return string(doc.Bytes())
}

// Fails if a key bound by a table header stops being renameable -- the header
// spells the binding out, so renaming the key renames the header's key part.
func TestRenameKeyRenamesTableHeader(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		path    string
		newKey  string
		want    string
		comment string
	}{
		{
			name:   "simple header",
			src:    "[server]\nhost = \"x\"\n",
			path:   "server",
			newKey: "web",
			want:   "[web]\nhost = \"x\"\n",
		},
		{
			name:   "final part of a dotted header",
			src:    "[a.b]\nx = 1\n",
			path:   "a.b",
			newKey: "c",
			want:   "[a.c]\nx = 1\n",
		},
		{
			name:   "prefix part, spelled by several headers",
			src:    "[a.b]\nx = 1\n[a.c]\ny = 2\n",
			path:   "a",
			newKey: "z",
			want:   "[z.b]\nx = 1\n[z.c]\ny = 2\n",
		},
		{
			name:   "every entry of an array of tables",
			src:    "[[items]]\nn = 1\n\n[[items]]\nn = 2\n",
			path:   "items",
			newKey: "things",
			want:   "[[things]]\nn = 1\n\n[[things]]\nn = 2\n",
		},
		{
			name:   "a sub-table header under an array of tables",
			src:    "[[s]]\n[s.t]\nx = 1\n",
			path:   "s",
			newKey: "q",
			want:   "[[q]]\n[q.t]\nx = 1\n",
		},
		{
			name:   "a name a dotted key and a header both spell out",
			src:    "a.b = 1\n[a.c]\nx = 2\n",
			path:   "a",
			newKey: "z",
			want:   "z.b = 1\n[z.c]\nx = 2\n",
		},
		{
			name:   "a key inside a header table, unchanged behaviour",
			src:    "[t]\nold = 1\n",
			path:   "t.old",
			newKey: "new",
			want:   "[t]\nnew = 1\n",
		},
		{
			name:   "a name needing quotes",
			src:    "[plain]\nx = 1\n",
			path:   "plain",
			newKey: "needs quoting",
			want:   "[\"needs quoting\"]\nx = 1\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renamed(t, tc.src, tc.path, tc.newKey); got != tc.want {
				t.Errorf("rendered %q, want %q", got, tc.want)
			}
		})
	}
}

// Fails if a rename stops being confined to the renamed key part: the brackets,
// the other parts, the whitespace between them and the header's comment are
// fragments the rename never touched.
func TestRenameKeyTouchesOneFragment(t *testing.T) {
	cases := []struct {
		src    string
		path   string
		newKey string
		want   string
	}{
		{"[ 'a' . b ]  # h\nx = 1\n", "a.b", "c", "[ 'a' . c ]  # h\nx = 1\n"},
		{"[ 'a' . b ]  # h\nx = 1\n", "a", "z", "[ z . b ]  # h\nx = 1\n"},
		{"[[  \"s\"  ]] # h\n", "s", "t", "[[  t  ]] # h\n"},
		{"a . 'b'  =  1  # c\n", "a.b", "z", "a . z  =  1  # c\n"},
	}
	for _, tc := range cases {
		if got := renamed(t, tc.src, tc.path, tc.newKey); got != tc.want {
			t.Errorf("RenameKey(%q, %q) on %q rendered %q, want %q",
				tc.path, tc.newKey, tc.src, got, tc.want)
		}
	}
}

// Fails if the renamed document stops reading back as the rename says: the
// binding must answer under its new name and be gone under the old one.
func TestRenameKeyRebindsTheName(t *testing.T) {
	doc, err := Parse([]byte("[a.b]\nx = 1\n[a.c]\ny = 2\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := doc.RenameKey("a", "z"); err != nil {
		t.Fatalf("RenameKey: %v", err)
	}
	if v, err := doc.GetInt("z.b.x"); err != nil || v != 1 {
		t.Errorf("z.b.x reads as (%d, %v), want (1, true)", v, err)
	}
	if doc.Has("a") {
		t.Errorf("the old name still resolves")
	}
	if _, ok := doc.Root().Get("z"); !ok {
		t.Errorf("the read-layer does not carry the new name")
	}
}

// Fails if renaming a header onto a name something else already binds stops
// being refused -- two constructs would then spell one key.
func TestRenameKeyRefusesHeaderNameCollision(t *testing.T) {
	cases := []struct {
		name string
		src  string
		path string
		to   string
	}{
		{"onto a table", "[a]\nx = 1\n[b]\ny = 2\n", "a", "b"},
		{"onto a value", "b = 1\n[a]\nx = 1\n", "a", "b"},
		{"onto an array of tables", "[a]\nx = 1\n[[b]]\n", "a", "b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			before := string(doc.Bytes())
			err = doc.RenameKey(tc.path, tc.to)
			var diag *Error
			if !errors.As(err, &diag) || diag.Kind != KindConflict {
				t.Fatalf("RenameKey returned %v, want a KindConflict diagnostic", err)
			}
			if got := string(doc.Bytes()); got != before {
				t.Errorf("the refused rename changed the document: %q", got)
			}
		})
	}
}

// Fails if a rename of a name nothing binds stops being reported as not found.
func TestRenameKeyMissingNameNotFound(t *testing.T) {
	doc, err := Parse([]byte("[a]\nx = 1\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var diag *Error
	if err := doc.RenameKey("nope", "other"); !errors.As(err, &diag) || diag.Kind != KindNotFound {
		t.Fatalf("RenameKey returned %v, want a KindNotFound diagnostic", err)
	}
}
