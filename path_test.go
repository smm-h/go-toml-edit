package tomledit

import "testing"

func TestParsePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    []PathSegment
		wantErr bool
	}{
		{
			name: "simple dotted key",
			path: "server.host",
			want: []PathSegment{
				{Kind: SegmentKey, Key: "server"},
				{Kind: SegmentKey, Key: "host"},
			},
		},
		{
			name: "key with index",
			path: "products[0].name",
			want: []PathSegment{
				{Kind: SegmentKey, Key: "products"},
				{Kind: SegmentIndex, Index: 0},
				{Kind: SegmentKey, Key: "name"},
			},
		},
		{
			name: "negative index",
			path: "products[-1]",
			want: []PathSegment{
				{Kind: SegmentKey, Key: "products"},
				{Kind: SegmentIndex, Index: -1},
			},
		},
		{
			name: "adjacent brackets",
			path: "matrix[0][1]",
			want: []PathSegment{
				{Kind: SegmentKey, Key: "matrix"},
				{Kind: SegmentIndex, Index: 0},
				{Kind: SegmentIndex, Index: 1},
			},
		},
		{
			name: "quoted key with dot",
			path: `server."host.name"`,
			want: []PathSegment{
				{Kind: SegmentKey, Key: "server"},
				{Kind: SegmentKey, Key: "host.name"},
			},
		},
		{
			name: "escaped dot",
			path: `server.host\.name`,
			want: []PathSegment{
				{Kind: SegmentKey, Key: "server"},
				{Kind: SegmentKey, Key: "host.name"},
			},
		},
		{
			name: "single key",
			path: "title",
			want: []PathSegment{
				{Kind: SegmentKey, Key: "title"},
			},
		},
		{
			name: "deeply nested",
			path: "a.b.c.d",
			want: []PathSegment{
				{Kind: SegmentKey, Key: "a"},
				{Kind: SegmentKey, Key: "b"},
				{Kind: SegmentKey, Key: "c"},
				{Kind: SegmentKey, Key: "d"},
			},
		},
		{
			name: "index only",
			path: "[0]",
			want: []PathSegment{
				{Kind: SegmentIndex, Index: 0},
			},
		},
		{
			name: "multiple indices after key",
			path: "a[0][1][2]",
			want: []PathSegment{
				{Kind: SegmentKey, Key: "a"},
				{Kind: SegmentIndex, Index: 0},
				{Kind: SegmentIndex, Index: 1},
				{Kind: SegmentIndex, Index: 2},
			},
		},
		{
			name: "index then key",
			path: "[0].name",
			want: []PathSegment{
				{Kind: SegmentIndex, Index: 0},
				{Kind: SegmentKey, Key: "name"},
			},
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "unclosed bracket",
			path:    "a[0",
			wantErr: true,
		},
		{
			name:    "non-numeric index",
			path:    "a[foo]",
			wantErr: true,
		},
		{
			name:    "empty index",
			path:    "a[]",
			wantErr: true,
		},
		{
			name:    "unclosed quote",
			path:    `a."unclosed`,
			wantErr: true,
		},
		{
			name:    "trailing dot",
			path:    "a.b.",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result: %v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d segments, want %d\n  got:  %+v\n  want: %+v", len(got), len(tt.want), got, tt.want)
			}

			for i, w := range tt.want {
				g := got[i]
				if g.Kind != w.Kind {
					t.Errorf("segment[%d].Kind = %v, want %v", i, g.Kind, w.Kind)
				}
				if w.Kind == SegmentKey && g.Key != w.Key {
					t.Errorf("segment[%d].Key = %q, want %q", i, g.Key, w.Key)
				}
				if w.Kind == SegmentIndex && g.Index != w.Index {
					t.Errorf("segment[%d].Index = %d, want %d", i, g.Index, w.Index)
				}
			}
		})
	}
}

// segmentsEqual reports whether two segment slices carry the same steps.
func segmentsEqual(a, b []PathSegment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Kind != b[i].Kind {
			return false
		}
		if a[i].Kind == SegmentKey && a[i].Key != b[i].Key {
			return false
		}
		if a[i].Kind == SegmentIndex && a[i].Index != b[i].Index {
			return false
		}
	}
	return true
}

// Fails if JoinPath stops quoting a key that ParsePath would otherwise read as
// more than one segment (or as a different key), breaking the identity that
// makes JoinPath the quoting authority for path text.
func TestJoinPathRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		segs []PathSegment
	}{
		{"bare keys", []PathSegment{{Kind: SegmentKey, Key: "server"}, {Kind: SegmentKey, Key: "host"}}},
		{"key then index", []PathSegment{{Kind: SegmentKey, Key: "products"}, {Kind: SegmentIndex, Index: 0}}},
		{"negative index", []PathSegment{{Kind: SegmentKey, Key: "products"}, {Kind: SegmentIndex, Index: -1}}},
		{"adjacent indices", []PathSegment{{Kind: SegmentKey, Key: "matrix"}, {Kind: SegmentIndex, Index: 0}, {Kind: SegmentIndex, Index: 1}}},
		{"index then key", []PathSegment{{Kind: SegmentKey, Key: "rows"}, {Kind: SegmentIndex, Index: 2}, {Kind: SegmentKey, Key: "name"}}},
		{"leading index", []PathSegment{{Kind: SegmentIndex, Index: 3}}},
		{"key with dot", []PathSegment{{Kind: SegmentKey, Key: "host.name"}}},
		{"key with bracket", []PathSegment{{Kind: SegmentKey, Key: "a[0]"}}},
		{"key with space", []PathSegment{{Kind: SegmentKey, Key: "two words"}}},
		{"empty key", []PathSegment{{Kind: SegmentKey, Key: ""}}},
		{"key with quote", []PathSegment{{Kind: SegmentKey, Key: `say "hi"`}}},
		{"key with backslash", []PathSegment{{Kind: SegmentKey, Key: `a\b`}}},
		{"non-ascii key", []PathSegment{{Kind: SegmentKey, Key: "ключ"}}},
		{"mixed", []PathSegment{{Kind: SegmentKey, Key: "a.b"}, {Kind: SegmentIndex, Index: -2}, {Kind: SegmentKey, Key: "c"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := JoinPath(tt.segs)
			got, err := ParsePath(text)
			if err != nil {
				t.Fatalf("ParsePath(%q) (joined from %+v): %v", text, tt.segs, err)
			}
			if !segmentsEqual(got, tt.segs) {
				t.Errorf("round-trip through %q gave %+v, want %+v", text, got, tt.segs)
			}
			if again := JoinPath(got); again != text {
				t.Errorf("re-joining gave %q, want %q", again, text)
			}
		})
	}
}

// Fails if JoinPath starts quoting a key that needs no quotes, or stops
// quoting one that does -- the rendered spelling is what diagnostics show and
// what a reader pastes back into a path.
func TestJoinPathSpelling(t *testing.T) {
	tests := []struct {
		segs []PathSegment
		want string
	}{
		{[]PathSegment{{Kind: SegmentKey, Key: "server"}, {Kind: SegmentKey, Key: "host"}}, "server.host"},
		{[]PathSegment{{Kind: SegmentKey, Key: "a-b_1"}}, "a-b_1"},
		{[]PathSegment{{Kind: SegmentKey, Key: "host.name"}}, `"host.name"`},
		{[]PathSegment{{Kind: SegmentKey, Key: ""}}, `""`},
		{[]PathSegment{{Kind: SegmentKey, Key: `q"b\s`}}, `"q\"b\\s"`},
		{[]PathSegment{{Kind: SegmentKey, Key: "items"}, {Kind: SegmentIndex, Index: -1}}, "items[-1]"},
		{nil, ""},
	}
	for _, tt := range tests {
		if got := JoinPath(tt.segs); got != tt.want {
			t.Errorf("JoinPath(%+v) = %q, want %q", tt.segs, got, tt.want)
		}
	}
}

// Fails if a path spelling the library accepts stops surviving a
// parse-render-parse cycle: the text a diagnostic carries must be pasteable
// back into any path-addressed operation.
func TestParsePathJoinPathPasteBack(t *testing.T) {
	paths := []string{
		"server.host",
		"products[0].name",
		"products[-1]",
		"matrix[0][1]",
		`server."host.name"`,
		`host\.name`,
		`""`,
		`"two words".x`,
		"a.b.c.d",
	}
	for _, p := range paths {
		segs, err := ParsePath(p)
		if err != nil {
			t.Fatalf("ParsePath(%q): %v", p, err)
		}
		text := JoinPath(segs)
		again, err := ParsePath(text)
		if err != nil {
			t.Fatalf("ParsePath(%q) (rendered from %q): %v", text, p, err)
		}
		if !segmentsEqual(again, segs) {
			t.Errorf("%q rendered as %q, which parses to %+v, want %+v", p, text, again, segs)
		}
	}
}
