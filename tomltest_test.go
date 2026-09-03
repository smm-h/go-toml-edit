package tomledit

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"

	tomltest "github.com/toml-lang/toml-test/v2"
)

// toml-test suite integration: every valid corpus file must parse and
// round-trip byte-for-byte, and every invalid one must be rejected.

// Case counts for the toml-test version pinned in go.mod, asserted so that a
// corpus that silently shrinks (or a skip derivation that starts excluding too
// much) cannot pass as a green run. A toml-test bump changes these numbers --
// re-verify the new corpus deliberately before updating them.
const (
	wantValidCases   = 205
	wantInvalidCases = 474
)

// corpusNames returns the trimmed (".toml" stripped) names of every test file
// under root in the embedded corpus, in walk order.
func corpusNames(t *testing.T, fsys fs.FS, root string) []string {
	t.Helper()
	var names []string
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".toml") {
			return nil
		}
		names = append(names, strings.TrimSuffix(path, ".toml"))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s tests: %v", root, err)
	}
	return names
}

// tomlTestSkips returns the corpus entries excluded for TOML 1.0, derived from
// the corpus's own version-filtered listing (the difference between what the
// walk finds and what the 1.0.0 runner lists) rather than from a hand-copied
// list that drifts when toml-test moves cases between versions.
func tomlTestSkips(t *testing.T, fsys fs.FS) map[string]bool {
	t.Helper()
	runner := tomltest.NewRunner(tomltest.Runner{Version: "1.0.0", Files: fsys})
	listed, err := runner.List()
	if err != nil {
		t.Fatalf("listing 1.0.0 test cases: %v", err)
	}

	// The listing also carries an "encoder/" mirror of the valid tree, which
	// this decoder-only suite does not run.
	keep := make(map[string]bool, len(listed))
	for _, name := range listed {
		if strings.HasPrefix(name, "valid/") || strings.HasPrefix(name, "invalid/") {
			keep[name] = true
		}
	}

	skip := make(map[string]bool)
	for _, root := range []string{"valid", "invalid"} {
		for _, name := range corpusNames(t, fsys, root) {
			if !keep[name] {
				skip[name] = true
			}
		}
	}
	return skip
}

// runCorpus runs check as a subtest for every non-skipped case under root and
// returns how many cases it ran. A file the corpus cannot read is reported as
// an error and not counted, so the caller's count assertion fails too.
func runCorpus(t *testing.T, fsys fs.FS, root string, skip map[string]bool, check func(t *testing.T, data []byte)) int {
	t.Helper()
	ran := 0
	for _, name := range corpusNames(t, fsys, root) {
		if skip[name] {
			continue
		}
		data, err := fs.ReadFile(fsys, name+".toml")
		if err != nil {
			t.Errorf("[%s] read error: %v", name, err)
			continue
		}
		ran++
		t.Run(name, func(t *testing.T) {
			check(t, data)
		})
	}
	return ran
}

// Fails if a valid corpus file stops parsing, stops round-tripping
// byte-for-byte, or if the number of valid cases run changes.
func TestTomlTestValid(t *testing.T) {
	fsys := tomltest.TestCases()
	skip := tomlTestSkips(t, fsys)

	ran := runCorpus(t, fsys, "valid", skip, func(t *testing.T, data []byte) {
		doc, err := Parse(data)
		if err != nil {
			t.Errorf("parse failed: %v", err)
			return
		}

		// Round-trip: Bytes() should produce identical output
		output := doc.Bytes()
		if !bytes.Equal(output, data) {
			// Find first difference for debugging
			diffIdx := 0
			minLen := len(data)
			if len(output) < minLen {
				minLen = len(output)
			}
			for diffIdx < minLen && data[diffIdx] == output[diffIdx] {
				diffIdx++
			}

			context := 40
			start := diffIdx - context
			if start < 0 {
				start = 0
			}
			endOrig := diffIdx + context
			if endOrig > len(data) {
				endOrig = len(data)
			}
			endOut := diffIdx + context
			if endOut > len(output) {
				endOut = len(output)
			}

			t.Errorf("round-trip mismatch at byte %d (input len=%d, output len=%d)\n"+
				"  input[%d:%d]:  %q\n"+
				"  output[%d:%d]: %q",
				diffIdx, len(data), len(output),
				start, endOrig, string(data[start:endOrig]),
				start, endOut, string(output[start:endOut]),
			)
		}
	})

	if ran != wantValidCases {
		t.Errorf("ran %d valid cases, want %d: the toml-test corpus or the 1.0 "+
			"skip derivation changed -- re-verify the corpus and update wantValidCases",
			ran, wantValidCases)
	}
}

// Fails if an invalid corpus file starts parsing successfully, or if the
// number of invalid cases run changes.
func TestTomlTestInvalid(t *testing.T) {
	fsys := tomltest.TestCases()
	skip := tomlTestSkips(t, fsys)

	ran := runCorpus(t, fsys, "invalid", skip, func(t *testing.T, data []byte) {
		if _, err := Parse(data); err == nil {
			t.Errorf("expected parse error but got none")
		}
	})

	if ran != wantInvalidCases {
		t.Errorf("ran %d invalid cases, want %d: the toml-test corpus or the 1.0 "+
			"skip derivation changed -- re-verify the corpus and update wantInvalidCases",
			ran, wantInvalidCases)
	}
}
