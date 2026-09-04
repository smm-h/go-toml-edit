package tomledit

import (
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// The spurious blank line: a blank-line run node the formatter skips must not
// count as emitted content.
// =============================================================================

// A document whose only content is blank lines holds them as a blank-run node.
// Appending a table after it puts that node ahead of the first thing the
// formatter emits: the formatter used to count the skipped node as content and
// open its output with a blank line. Reachable only from an edit sequence --
// the parser emits a blank-run node at end of input, where nothing follows it.
//
// Fails if the formatter ever again opens its output with a blank line.
func TestFormatNoLeadingBlankLineAfterEditedBlankRun(t *testing.T) {
	doc, err := Parse([]byte("\n\n"))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if err := doc.NewTable("server"); err != nil {
		t.Fatalf("NewTable error: %v", err)
	}
	if err := doc.Set("server.host", "example"); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	const want = "[server]\nhost = \"example\"\n"
	if got := string(doc.Format()); got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
	if got := string(doc.Format(WithTableBlankLine(false))); got != want {
		t.Errorf("Format(WithTableBlankLine(false)) = %q, want %q", got, want)
	}
}

// The same defect reached through a document that already carries content:
// deleting the only key of the leading table leaves the blank run standing
// alone ahead of the tables that follow it.
//
// Fails if a skipped blank-run node ever again makes the formatter believe
// content stands above the next table.
func TestFormatBlankRunNodeIsNotContent(t *testing.T) {
	doc, err := Parse([]byte("# header\n\n"))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if err := doc.NewTable("a"); err != nil {
		t.Fatalf("NewTable error: %v", err)
	}
	if err := doc.Set("a.x", int64(1)); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	// The comment is content; the blank run between it and [a] collapses to
	// one blank line, and the table option inserts nothing on top of it.
	const want = "# header\n\n[a]\nx = 1\n"
	if got := string(doc.Format()); got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

// =============================================================================
// Blank-line grouping preservation
// =============================================================================

// Fails if a blank-line run stops collapsing to exactly one blank line, if a
// zero-length gap starts growing one, or if the formatter starts dropping the
// user's grouping again.
func TestFormatPreservesBlankLineGrouping(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "run of three collapses to one",
			input: "a = 1\n\n\n\nb = 2\n",
			want:  "a = 1\n\nb = 2\n",
		},
		{
			name:  "no gap stays no gap",
			input: "a = 1\nb = 2\n",
			want:  "a = 1\nb = 2\n",
		},
		{
			name:  "one blank stays one",
			input: "a = 1\n\nb = 2\n",
			want:  "a = 1\n\nb = 2\n",
		},
		{
			name:  "blank lines at the start of the document are dropped",
			input: "\n\n\na = 1\n",
			want:  "a = 1\n",
		},
		{
			name:  "trailing blank lines are dropped",
			input: "a = 1\n\n\n",
			want:  "a = 1\n",
		},
		{
			name:  "grouping around a standalone comment",
			input: "a = 1\n\n# note\n\nb = 2\n",
			want:  "a = 1\n\n# note\n\nb = 2\n",
		},
		{
			name:  "gap before a comment, none after it",
			input: "a = 1\n\n# note\nb = 2\n",
			want:  "a = 1\n\n# note\nb = 2\n",
		},
		{
			name:  "gap after a comment, none before it",
			input: "a = 1\n# note\n\nb = 2\n",
			want:  "a = 1\n# note\n\nb = 2\n",
		},
		{
			name:  "gap between two leading comments",
			input: "# one\n\n# two\na = 1\n",
			want:  "# one\n\n# two\na = 1\n",
		},
		{
			name:  "grouping inside a table body",
			input: "[t]\nx = 1\n\n\ny = 2\n",
			want:  "[t]\nx = 1\n\ny = 2\n",
		},
		{
			name:  "gap right after a table header",
			input: "[t]\n\nx = 1\n",
			want:  "[t]\n\nx = 1\n",
		},
		{
			name:  "grouping between array tables",
			input: "[[t]]\nx = 1\n\n\n[[t]]\nx = 2\n",
			want:  "[[t]]\nx = 1\n\n[[t]]\nx = 2\n",
		},
		{
			name:  "trailing comment keeps its gap",
			input: "a = 1\n\n\n# trailing\n",
			want:  "a = 1\n\n# trailing\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			got := string(doc.Format(WithTableBlankLine(false)))
			if got != tt.want {
				t.Errorf("Format(WithTableBlankLine(false)) = %q, want %q", got, tt.want)
			}
		})
	}
}

// The table-blank-line option only ever ADDS a blank line before a table that
// has none; it never removes the grouping the document already carries, and it
// never doubles a blank line that is already there.
//
// Fails if the option starts removing blank lines, or starts inserting a second
// one where a blank line already stands.
func TestFormatTableBlankLineIsInsertionOnly(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOn  string
		wantOff string
	}{
		{
			name:    "inserted where missing",
			input:   "a = 1\n[t]\nx = 1\n",
			wantOn:  "a = 1\n\n[t]\nx = 1\n",
			wantOff: "a = 1\n[t]\nx = 1\n",
		},
		{
			name:    "not doubled where present",
			input:   "a = 1\n\n[t]\nx = 1\n",
			wantOn:  "a = 1\n\n[t]\nx = 1\n",
			wantOff: "a = 1\n\n[t]\nx = 1\n",
		},
		{
			name:    "a run before a table still collapses to one",
			input:   "a = 1\n\n\n\n[t]\nx = 1\n",
			wantOn:  "a = 1\n\n[t]\nx = 1\n",
			wantOff: "a = 1\n\n[t]\nx = 1\n",
		},
		{
			name:    "never inserted before the first construct",
			input:   "[t]\nx = 1\n",
			wantOn:  "[t]\nx = 1\n",
			wantOff: "[t]\nx = 1\n",
		},
		{
			name:    "inserted above the table's leading comments",
			input:   "a = 1\n# about t\n[t]\nx = 1\n",
			wantOn:  "a = 1\n\n# about t\n[t]\nx = 1\n",
			wantOff: "a = 1\n# about t\n[t]\nx = 1\n",
		},
		{
			name:    "existing gap between comment and header survives either way",
			input:   "a = 1\n# about t\n\n[t]\nx = 1\n",
			wantOn:  "a = 1\n\n# about t\n\n[t]\nx = 1\n",
			wantOff: "a = 1\n# about t\n\n[t]\nx = 1\n",
		},
		{
			name:    "array tables too",
			input:   "a = 1\n[[t]]\nx = 1\n",
			wantOn:  "a = 1\n\n[[t]]\nx = 1\n",
			wantOff: "a = 1\n[[t]]\nx = 1\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if got := string(doc.Format(WithTableBlankLine(true))); got != tt.wantOn {
				t.Errorf("Format(WithTableBlankLine(true)) = %q, want %q", got, tt.wantOn)
			}
			if got := string(doc.Format(WithTableBlankLine(false))); got != tt.wantOff {
				t.Errorf("Format(WithTableBlankLine(false)) = %q, want %q", got, tt.wantOff)
			}
		})
	}
}

// =============================================================================
// Every formatting-option combination over one blank-line fixture
// =============================================================================

// blankLineFixture carries every shape the blank-line rules have to answer for:
// leading blank lines, a run between top-level keys, a gap around a standalone
// comment, a table with and without a gap before it, a gap inside a table body,
// an array long enough to break under a narrow line width, and trailing blanks.
const blankLineFixture = "\n\n" +
	"a = 1\n" +
	"\n\n\n" +
	"b = 2\n" +
	"\n" +
	"# a note\n" +
	"\n" +
	"items = [\"alpha\", \"beta\", \"gamma\"]\n" +
	"[first]\n" +
	"x = 1\n" +
	"\n\n" +
	"y = 2\n" +
	"\n" +
	"# about second\n" +
	"[second]\n" +
	"z = 3\n" +
	"\n\n"

// The whole option space over the fixture, asserted per combination. Fails on
// ANY change to the formatter's output for any option combination -- which is
// the point: it is the pin under the blank-line rules and the option
// composition, not a sample of them.
func TestFormatOptionCombinationsOverBlankLineFixture(t *testing.T) {
	doc, err := Parse([]byte(blankLineFixture))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	indents := []int{0, 2}
	widths := []int{80, 20}
	tableBlanks := []bool{true, false}

	// want[key] is the exact output for the combination named by the key.
	want := map[string]string{
		"indent=0,width=80,tableblank=true": "a = 1\n" +
			"\n" +
			"b = 2\n" +
			"\n" +
			"# a note\n" +
			"\n" +
			"items = [\"alpha\", \"beta\", \"gamma\"]\n" +
			"\n" +
			"[first]\n" +
			"x = 1\n" +
			"\n" +
			"y = 2\n" +
			"\n" +
			"# about second\n" +
			"[second]\n" +
			"z = 3\n",
		"indent=0,width=80,tableblank=false": "a = 1\n" +
			"\n" +
			"b = 2\n" +
			"\n" +
			"# a note\n" +
			"\n" +
			"items = [\"alpha\", \"beta\", \"gamma\"]\n" +
			"[first]\n" +
			"x = 1\n" +
			"\n" +
			"y = 2\n" +
			"\n" +
			"# about second\n" +
			"[second]\n" +
			"z = 3\n",
		"indent=0,width=20,tableblank=true": "a = 1\n" +
			"\n" +
			"b = 2\n" +
			"\n" +
			"# a note\n" +
			"\n" +
			"items = [\n" +
			"\"alpha\",\n" +
			"\"beta\",\n" +
			"\"gamma\",\n" +
			"]\n" +
			"\n" +
			"[first]\n" +
			"x = 1\n" +
			"\n" +
			"y = 2\n" +
			"\n" +
			"# about second\n" +
			"[second]\n" +
			"z = 3\n",
		"indent=0,width=20,tableblank=false": "a = 1\n" +
			"\n" +
			"b = 2\n" +
			"\n" +
			"# a note\n" +
			"\n" +
			"items = [\n" +
			"\"alpha\",\n" +
			"\"beta\",\n" +
			"\"gamma\",\n" +
			"]\n" +
			"[first]\n" +
			"x = 1\n" +
			"\n" +
			"y = 2\n" +
			"\n" +
			"# about second\n" +
			"[second]\n" +
			"z = 3\n",
		"indent=2,width=80,tableblank=true": "a = 1\n" +
			"\n" +
			"b = 2\n" +
			"\n" +
			"# a note\n" +
			"\n" +
			"items = [\"alpha\", \"beta\", \"gamma\"]\n" +
			"\n" +
			"[first]\n" +
			"  x = 1\n" +
			"\n" +
			"  y = 2\n" +
			"\n" +
			"# about second\n" +
			"[second]\n" +
			"  z = 3\n",
		"indent=2,width=80,tableblank=false": "a = 1\n" +
			"\n" +
			"b = 2\n" +
			"\n" +
			"# a note\n" +
			"\n" +
			"items = [\"alpha\", \"beta\", \"gamma\"]\n" +
			"[first]\n" +
			"  x = 1\n" +
			"\n" +
			"  y = 2\n" +
			"\n" +
			"# about second\n" +
			"[second]\n" +
			"  z = 3\n",
		"indent=2,width=20,tableblank=true": "a = 1\n" +
			"\n" +
			"b = 2\n" +
			"\n" +
			"# a note\n" +
			"\n" +
			"items = [\n" +
			"  \"alpha\",\n" +
			"  \"beta\",\n" +
			"  \"gamma\",\n" +
			"]\n" +
			"\n" +
			"[first]\n" +
			"  x = 1\n" +
			"\n" +
			"  y = 2\n" +
			"\n" +
			"# about second\n" +
			"[second]\n" +
			"  z = 3\n",
		"indent=2,width=20,tableblank=false": "a = 1\n" +
			"\n" +
			"b = 2\n" +
			"\n" +
			"# a note\n" +
			"\n" +
			"items = [\n" +
			"  \"alpha\",\n" +
			"  \"beta\",\n" +
			"  \"gamma\",\n" +
			"]\n" +
			"[first]\n" +
			"  x = 1\n" +
			"\n" +
			"  y = 2\n" +
			"\n" +
			"# about second\n" +
			"[second]\n" +
			"  z = 3\n",
	}

	seen := make(map[string]bool, len(want))
	for _, indent := range indents {
		for _, width := range widths {
			for _, tableBlank := range tableBlanks {
				key := fmt.Sprintf("indent=%d,width=%d,tableblank=%t", indent, width, tableBlank)
				seen[key] = true
				expected, ok := want[key]
				if !ok {
					t.Fatalf("no expected output recorded for combination %s", key)
				}
				got := string(doc.Format(
					WithIndentWidth(indent),
					WithLineWidth(width),
					WithTableBlankLine(tableBlank),
				))
				if got != expected {
					t.Errorf("combination %s:\n got: %q\nwant: %q", key, got, expected)
				}
				// Every combination's output must still parse, and formatting
				// it again must reproduce it exactly.
				redoc, err := Parse([]byte(got))
				if err != nil {
					t.Errorf("combination %s: output does not parse: %v\n%s", key, err, got)
					continue
				}
				again := string(redoc.Format(
					WithIndentWidth(indent),
					WithLineWidth(width),
					WithTableBlankLine(tableBlank),
				))
				if again != got {
					t.Errorf("combination %s is not idempotent:\n first: %q\nsecond: %q", key, got, again)
				}
			}
		}
	}
	for key := range want {
		if !seen[key] {
			t.Errorf("expected output recorded for %s, which the combination sweep never produced", key)
		}
	}

	// Formatting never mutates the document.
	if got := string(doc.Bytes()); got != blankLineFixture {
		t.Errorf("Format() mutated the document: Bytes() = %q, want %q", got, blankLineFixture)
	}
}

// Guard on the fixture itself: it must exercise every blank-line shape the
// rules answer for. Fails if someone edits the fixture down to something that
// no longer covers them.
func TestBlankLineFixtureCoversTheShapes(t *testing.T) {
	if !strings.HasPrefix(blankLineFixture, "\n") {
		t.Error("fixture must start with a blank line")
	}
	if !strings.HasSuffix(blankLineFixture, "\n\n") {
		t.Error("fixture must end with blank lines")
	}
	if !strings.Contains(blankLineFixture, "\n\n\n") {
		t.Error("fixture must contain a run of more than one blank line")
	}
	if !strings.Contains(blankLineFixture, "items = [") {
		t.Error("fixture must contain an array that a narrow line width breaks")
	}
	if !strings.Contains(blankLineFixture, "\n[first]") {
		t.Error("fixture must contain a table with no blank line before it")
	}
	if !strings.Contains(blankLineFixture, "# about second\n[second]") {
		t.Error("fixture must contain a table with a leading comment")
	}
}
