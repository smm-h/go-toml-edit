package tomledit

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	tomltest "github.com/toml-lang/toml-test/v2"
)

// toml-test suite integration: validates spec compliance by checking that
// all valid TOML files parse and round-trip, and all invalid files reject.

// testsToSkip_1_0 lists tests that are TOML 1.1 features, excluded for 1.0.
// Derived from version.go in toml-test.
var testsToSkip_1_0 = map[string]bool{
	"valid/spec-1.1.0":                   true,
	"invalid/spec-1.1.0":                 true,
	"valid/string/escape-esc":            true,
	"valid/string/hex-escape":            true,
	"invalid/string/bad-hex-esc":         true,
	"valid/datetime/no-seconds":          true,
	"valid/inline-table/newline":         true,
	"valid/inline-table/newline-comment": true,
	"invalid/control/multi-cr":           true,
	"invalid/control/rawmulti-cr":        true,
}

func shouldSkip(testPath string) bool {
	// Exact match
	if testsToSkip_1_0[testPath] {
		return true
	}
	// Prefix match for directories
	for skip := range testsToSkip_1_0 {
		if strings.HasSuffix(skip, "/") {
			if strings.HasPrefix(testPath, skip) {
				return true
			}
		}
		// Also handle glob-style directory skips (e.g. "valid/spec-1.1.0")
		// by checking if test path starts with skip + "/"
		if strings.HasPrefix(testPath, skip+"/") {
			return true
		}
	}
	return false
}

func TestTomlTestValid(t *testing.T) {
	fsys := tomltest.TestCases()

	var total, skipped, parseFail, roundtripFail int

	err := fs.WalkDir(fsys, "valid", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".toml") {
			return nil
		}

		// Normalize test name: strip .toml suffix
		testName := strings.TrimSuffix(path, ".toml")

		if shouldSkip(testName) {
			skipped++
			return nil
		}

		total++

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			t.Errorf("[%s] read error: %v", testName, err)
			parseFail++
			return nil
		}

		t.Run(filepath.ToSlash(testName), func(t *testing.T) {
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

		return nil
	})
	if err != nil {
		t.Fatalf("walking valid tests: %v", err)
	}

	t.Logf("Valid tests: total=%d, skipped=%d, parseFail=%d, roundtripFail=%d",
		total, skipped, parseFail, roundtripFail)
}

func TestTomlTestInvalid(t *testing.T) {
	fsys := tomltest.TestCases()

	var total, skipped int

	err := fs.WalkDir(fsys, "invalid", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".toml") {
			return nil
		}

		testName := strings.TrimSuffix(path, ".toml")

		if shouldSkip(testName) {
			skipped++
			return nil
		}

		total++

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			t.Errorf("[%s] read error: %v", testName, err)
			return nil
		}

		t.Run(filepath.ToSlash(testName), func(t *testing.T) {
			_, err := Parse(data)
			if err == nil {
				t.Errorf("expected parse error but got none")
			}
		})

		return nil
	})
	if err != nil {
		t.Fatalf("walking invalid tests: %v", err)
	}

	t.Logf("Invalid tests: total=%d, skipped=%d",
		total, skipped)
}
