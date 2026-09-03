package tomledit

import (
	"strings"
	"testing"
)

func TestDottedKeyThenTableRedefine(t *testing.T) {
	// a.b.c = 1 makes a.b implicit
	// [a.b] promotes it to explicit
	// then c = 2 under [a.b] should conflict with the already-defined c
	_, err := Parse([]byte("a.b.c = 1\n[a.b]\nc = 2\n"))
	if err == nil {
		t.Fatal("expected duplicate key error for a.b.c")
	}
	if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "already defined") {
		t.Logf("error message: %v", err)
	}
}

func TestDottedKeyThenTableNewKey(t *testing.T) {
	// a.b.c = 1 defines a.b via dotted key.
	// [a.b] tries to reopen it with a table header, which is invalid
	// per TOML spec: "Since the table has already been specified using
	// dotted keys, it cannot be directly defined using a header."
	_, err := Parse([]byte("a.b.c = 1\n[a.b]\nd = 2\n"))
	if err == nil {
		t.Fatal("expected error: dotted-key-defined table cannot be reopened with [table] header")
	}
}

// Fails if a definition-tracker diagnostic renders a key path by joining the
// parts with raw dots: a segment that needs quoting then reads as two
// segments, and the path text in the message no longer pastes back through
// ParsePath. Every case below uses the single key "a.b", whose path text is
// the quoted spelling.
func TestTrackerDiagnosticPathsPasteBack(t *testing.T) {
	cases := []struct {
		name string
		// input parses to exactly one definition-tracker diagnostic.
		input string
		// want is the substring the message must carry, in the message's own
		// surroundings (header brackets where the site writes them).
		want string
		// pathText is the path text alone, as it appears inside want.
		pathText string
		// segs is what pathText must paste back to.
		segs []string
	}{
		{
			name:     "table already defined",
			input:    "[\"a.b\"]\n[\"a.b\"]\n",
			want:     `table ["a.b"] already defined`,
			pathText: `"a.b"`,
			segs:     []string{"a.b"},
		},
		{
			name:     "table over array table",
			input:    "[[\"a.b\"]]\n[\"a.b\"]\n",
			want:     `cannot define ["a.b"] as a table, already defined as array table`,
			pathText: `"a.b"`,
			segs:     []string{"a.b"},
		},
		{
			name:     "table over dotted key",
			input:    "\"a.b\".c = 1\n[\"a.b\"]\n",
			want:     `cannot define ["a.b"] as a table, already implicitly defined via dotted key`,
			pathText: `"a.b"`,
			segs:     []string{"a.b"},
		},
		{
			name:     "array table over table",
			input:    "[\"a.b\"]\n[[\"a.b\"]]\n",
			want:     `cannot define [["a.b"]] as array table, already defined as table`,
			pathText: `"a.b"`,
			segs:     []string{"a.b"},
		},
		{
			name:     "array table over dotted key",
			input:    "\"a.b\".c = 1\n[[\"a.b\"]]\n",
			want:     `cannot define [["a.b"]] as array table, already implicitly defined via dotted key`,
			pathText: `"a.b"`,
			segs:     []string{"a.b"},
		},
		{
			name:     "array table over implicit table with sub-entries",
			input:    "[\"a.b\".c]\nx = 1\n[[\"a.b\"]]\n",
			want:     `cannot define [["a.b"]] as array table, already used as table with sub-entries`,
			pathText: `"a.b"`,
			segs:     []string{"a.b"},
		},
		{
			name:     "duplicate key",
			input:    "\"a.b\" = 1\n\"a.b\" = 2\n",
			want:     `duplicate key "a.b"`,
			pathText: `"a.b"`,
			segs:     []string{"a.b"},
		},
		{
			name:     "duplicate dotted key",
			input:    "\"a.b\".c = 1\n\"a.b\".c = 2\n",
			want:     `duplicate key "a.b".c`,
			pathText: `"a.b".c`,
			segs:     []string{"a.b", "c"},
		},
		{
			name:     "key conflicts with table",
			input:    "[x.\"a.b\".c]\ny = 1\n[x]\n\"a.b\".c = 2\n",
			want:     `key "a.b".c conflicts with table definition`,
			pathText: `"a.b".c`,
			segs:     []string{"a.b", "c"},
		},
		{
			name:     "key already defined as a table",
			input:    "\"a.b\".c.d = 1\n\"a.b\".c = 2\n",
			want:     `key "a.b".c is already defined as a table`,
			pathText: `"a.b".c`,
			segs:     []string{"a.b", "c"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.input))
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want a diagnostic", tt.input)
			}
			msg := err.Error()
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("diagnostic is %q, want it to carry %q", msg, tt.want)
			}
			if !strings.Contains(tt.want, tt.pathText) {
				t.Fatalf("test case is malformed: %q does not carry %q", tt.want, tt.pathText)
			}
			segs, err := ParsePath(tt.pathText)
			if err != nil {
				t.Fatalf("the path text %q in the diagnostic does not parse: %v", tt.pathText, err)
			}
			want := make([]PathSegment, len(tt.segs))
			for i, key := range tt.segs {
				want[i] = PathSegment{Kind: SegmentKey, Key: key}
			}
			if !segmentsEqual(segs, want) {
				t.Errorf("the path text %q reads as %+v, want %+v", tt.pathText, segs, want)
			}
		})
	}
}
