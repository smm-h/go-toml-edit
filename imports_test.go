package tomledit

import (
	goparser "go/parser"
	gotoken "go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The library has NO RUNTIME DEPENDENCIES: the non-test sources import the
// standard library and nothing else. The module requires BurntSushi/toml and
// toml-test, but both are reachable only from _test.go files -- they are the
// benchmark comparison and the compliance corpus -- so a consumer building this
// package links no third-party code.
//
// The README states this, which is why it is asserted rather than assumed.
// An import path whose first segment carries a dot names a module domain;
// standard-library paths never do.
//
// Fails the moment a non-test source file imports anything outside the standard
// library.
func TestNonTestSourcesImportOnlyTheStandardLibrary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	fset := gotoken.NewFileSet()
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := goparser.ParseFile(fset, name, nil, goparser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		checked++
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("%s: unquoting import %s: %v", name, imp.Path.Value, err)
			}
			if first, _, _ := strings.Cut(path, "/"); strings.Contains(first, ".") {
				t.Errorf("%s imports %q, which is outside the standard library; "+
					"the README promises no runtime dependencies", name, path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no non-test source files were examined; the check proved nothing")
	}
}
