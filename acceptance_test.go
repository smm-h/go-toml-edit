package tomledit

import (
	"strings"
	"testing"
)

// The reorder acceptance scenario: the shape of the consumer work the
// structural operations exist for. A tool holds an external ranking of the
// constructs it manages, classifies what the document carries, drops what its
// ranking does not name, and rewrites the file in the ranked order with every
// comment still attached to the construct it describes.
//
// The record marks the structural-operation signatures provisional until this
// passes, so it is written as a consumer would write it -- reading through the
// public surface, composing the drop as Delete and the reorder as
// PermuteChildren -- rather than reaching into the package.

// Fails if the reorder scenario stops being expressible through the public
// surface: a dropped construct that survives, a reordering that needs an
// operation the package does not have, a comment that parts from its
// construct, or a document that stops folding afterwards.
func TestAcceptance_RankedReorderWithDrops(t *testing.T) {
	const src = `# the users table
[users]
kind = "core"

# an index over users
[users_by_email]
kind = "index"

# a scratch table nobody ranked
[tmp]
kind = "scratch"

# the accounts table
[accounts]
kind = "core"

# an index over accounts
[accounts_by_id]
kind = "index"
# nothing below this line
`
	doc := parseOrFail(t, src)

	// The consumer's ranking: the classified kinds it manages, in the order it
	// wants them grouped. A construct classified as anything else is dropped.
	ranking := []string{"core", "index"}

	// Classify what the document carries, in document order, reading through
	// the read-layer rather than the syntax.
	byKind := map[string][]string{}
	var unranked []string
	for entry := range doc.Root().Entries() {
		kind, ok := doc.GetString(entry.Key() + ".kind")
		if !ok {
			t.Fatalf("table %q carries no kind", entry.Key())
		}
		if !contains(ranking, kind) {
			unranked = append(unranked, entry.Key())
			continue
		}
		byKind[kind] = append(byKind[kind], entry.Key())
	}

	// Drop the constructs the ranking does not name.
	for _, name := range unranked {
		if err := doc.Delete(name); err != nil {
			t.Fatalf("Delete(%q): %v", name, err)
		}
	}

	// The wanted order: grouped by ranked kind, document order within a group.
	var wanted []string
	for _, kind := range ranking {
		wanted = append(wanted, byKind[kind]...)
	}

	// Gather the permutation from the children as they stand after the drops.
	// Every child must appear exactly once; here every child is a table, and
	// the comments -- the ones above a header and the one written inside the
	// last table -- are parts of their own constructs rather than children of
	// the document.
	position := map[string]int{}
	for i, child := range doc.Children {
		tbl, ok := child.(*TableNode)
		if !ok {
			t.Fatalf("document child %d is a %s, not a table", i, child.Type())
		}
		position[tbl.KeyPath[0]] = i
	}
	order := make([]int, 0, len(doc.Children))
	for _, name := range wanted {
		i, ok := position[name]
		if !ok {
			t.Fatalf("table %q is not among the document's children", name)
		}
		order = append(order, i)
	}

	if err := doc.PermuteChildren("", order); err != nil {
		t.Fatalf("PermuteChildren: %v", err)
	}

	const want = `# the users table
[users]
kind = "core"

# the accounts table
[accounts]
kind = "core"

# an index over users
[users_by_email]
kind = "index"

# an index over accounts
[accounts_by_id]
kind = "index"
# nothing below this line
`
	got := string(doc.Bytes())
	if got != want {
		t.Errorf("the rewritten document reads\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "tmp") || strings.Contains(got, "nobody ranked") {
		t.Errorf("the dropped table or its comment survived:\n%s", got)
	}
	if _, err := Parse([]byte(got)); err != nil {
		t.Fatalf("the rewritten document is not valid TOML: %v\n%s", err, got)
	}
	mustFold(t, doc)
}

// Fails if a permutation stops carrying the whole document forward: the
// scenario above depends on the totality refusal, which is what tells a
// consumer it forgot a child rather than silently dropping one.
func TestAcceptance_ReorderRefusesToDropSilently(t *testing.T) {
	doc := parseOrFail(t, "[a]\n[b]\n[c]\n")
	if err := doc.PermuteChildren("", []int{2, 0}); err == nil {
		t.Fatalf("a permutation naming two of three children was accepted: %q", doc.Bytes())
	}
	if got := string(doc.Bytes()); got != "[a]\n[b]\n[c]\n" {
		t.Errorf("the refused permutation changed the document: %q", got)
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
