package tomledit

import "testing"

// This library is pinned to TOML 1.0: every TOML 1.1-only construct must fail
// to parse. The official toml-test suite's 1.1 cases are skipped by
// testsToSkip_1_0 (tomltest_test.go), so these are the only tests asserting
// rejection of 1.1 syntax.
func TestParseRejectsTOML11Syntax(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"newline in inline table", "x = { a = 1,\n b = 2 }\n"},
		{"trailing comma in inline table", "x = { a = 1, }\n"},
		{"hex escape", "s = \"a\\x41b\"\n"},
		{"esc escape", "s = \"a\\eb\"\n"},
		{"secondless local time", "t = 07:32\n"},
		{"secondless local datetime", "d = 2026-08-02T07:32\n"},
		{"secondless offset datetime", "d = 2026-08-02T07:32Z\n"},
		{"non-ascii bare key", "café = 1\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.input)); err == nil {
				t.Fatalf("expected parse error for TOML 1.1-only syntax: %q", tc.input)
			}
		})
	}
}
