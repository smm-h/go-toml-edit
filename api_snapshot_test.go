// The test lives in the external test package: the package's own
// identifiers include `parser`, `token` and `lexer`, which would collide with
// the go/parser and go/token imports this generator needs.
package tomledit_test

// The exported-API snapshot: a committed, deterministic listing of everything
// this package exports, compared against the package sources on every test
// run. Any change to the exported surface -- an added or removed identifier, a
// changed signature, a new or dropped interface method, a moved constant
// value -- fails TestAPISnapshot until the snapshot is regenerated, so an
// unintended surface change can never pass silently.
//
// Regenerate the snapshot for the working tree:
//
//	go generate ./...
//
// which runs:
//
//	go test -run TestAPISnapshot -update-api-snapshot
//
// Snapshot another source tree (used once to capture the frozen v0.3.0
// baseline that the next release's migration table is diffed against):
//
//	tmp=$(mktemp -d)
//	git archive v0.3.0 | tar -x -C "$tmp"
//	go test -run TestAPISnapshot -update-api-snapshot \
//	    -api-snapshot-src "$tmp" \
//	    -api-snapshot-out testdata/api-snapshot-v0.3.0.txt \
//	    -api-snapshot-label v0.3.0
//
// and then delete the temporary directory.
//
// The listing is produced by type-checking the package's non-test sources, so
// it records real method sets (including methods promoted from embedded
// unexported types) and resolved constant values, not just what the
// declarations spell out.

//go:generate go test -run TestAPISnapshot -update-api-snapshot

import (
	"flag"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var (
	apiSnapshotUpdate = flag.Bool("update-api-snapshot", false,
		"rewrite the API snapshot file instead of comparing against it")
	apiSnapshotSrc = flag.String("api-snapshot-src", ".",
		"directory holding the package sources to snapshot")
	apiSnapshotOut = flag.String("api-snapshot-out", filepath.Join("testdata", "api-snapshot.txt"),
		"snapshot file to compare against or rewrite")
	apiSnapshotLabel = flag.String("api-snapshot-label", "working tree",
		"source description recorded in the snapshot header")
)

// apiSnapshotBaseline is the frozen listing of the last released version's
// exported surface. It is never regenerated: the release's migration table is
// the diff of the current snapshot against it.
const apiSnapshotBaseline = "testdata/api-snapshot-v0.3.0.txt"

func TestAPISnapshot(t *testing.T) {
	got, err := buildAPISnapshot(*apiSnapshotSrc, *apiSnapshotLabel)
	if err != nil {
		t.Fatalf("building API snapshot from %s: %v", *apiSnapshotSrc, err)
	}

	if *apiSnapshotUpdate {
		if err := os.MkdirAll(filepath.Dir(*apiSnapshotOut), 0o755); err != nil {
			t.Fatalf("creating snapshot directory: %v", err)
		}
		if err := os.WriteFile(*apiSnapshotOut, []byte(got), 0o644); err != nil {
			t.Fatalf("writing %s: %v", *apiSnapshotOut, err)
		}
		t.Logf("wrote %s", *apiSnapshotOut)
		return
	}

	want, err := os.ReadFile(*apiSnapshotOut)
	if err != nil {
		t.Fatalf("reading %s: %v (regenerate with: go generate ./...)", *apiSnapshotOut, err)
	}
	if got == string(want) {
		return
	}
	t.Errorf("the exported API no longer matches %s:\n%s\n\n"+
		"If the change is intended, regenerate the snapshot with: go generate ./...",
		*apiSnapshotOut, apiSnapshotDiff(string(want), got))
}

// TestAPISnapshotBaselineIntact guards the frozen baseline artifact against
// accidental truncation or rewriting: it must exist, carry its own header, and
// list a surface.
func TestAPISnapshotBaselineIntact(t *testing.T) {
	data, err := os.ReadFile(apiSnapshotBaseline)
	if err != nil {
		t.Fatalf("reading the frozen baseline: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "# Exported API of package ") {
		t.Fatalf("%s does not look like a snapshot", apiSnapshotBaseline)
	}
	if lines[1] != "# Source: v0.3.0" {
		t.Fatalf("%s no longer labels itself v0.3.0 (got %q); the frozen baseline must never be regenerated", apiSnapshotBaseline, lines[1])
	}
	var entries int
	for _, line := range lines {
		if line != "" && !strings.HasPrefix(line, "#") {
			entries++
		}
	}
	if entries == 0 {
		t.Fatalf("%s lists no exported identifiers", apiSnapshotBaseline)
	}
}

// apiSnapshotDiff renders a line-level difference between two snapshots. The
// listing is sorted, so a set difference is enough to describe any change.
func apiSnapshotDiff(want, got string) string {
	inWant := map[string]bool{}
	for _, line := range strings.Split(want, "\n") {
		inWant[line] = true
	}
	inGot := map[string]bool{}
	for _, line := range strings.Split(got, "\n") {
		inGot[line] = true
	}
	var b strings.Builder
	for _, line := range strings.Split(want, "\n") {
		if line != "" && !inGot[line] {
			fmt.Fprintf(&b, "-%s\n", line)
		}
	}
	for _, line := range strings.Split(got, "\n") {
		if line != "" && !inWant[line] {
			fmt.Fprintf(&b, "+%s\n", line)
		}
	}
	if b.Len() == 0 {
		return "(the entries are identical; only ordering or blank lines differ)"
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildAPISnapshot type-checks the package whose non-test sources live in dir
// and renders its exported surface as a deterministic textual listing.
func buildAPISnapshot(dir, label string) (string, error) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	var files []*ast.File
	pkgName := ""
	for _, name := range names {
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return "", err
		}
		if pkgName == "" {
			pkgName = file.Name.Name
		} else if file.Name.Name != pkgName {
			return "", fmt.Errorf("%s declares package %s, want %s", name, file.Name.Name, pkgName)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no non-test Go sources in %s", dir)
	}

	conf := types.Config{
		Importer:         importer.ForCompiler(fset, "source", nil),
		IgnoreFuncBodies: true,
	}
	pkg, err := conf.Check(pkgName, fset, files, nil)
	if err != nil {
		return "", err
	}

	qual := func(p *types.Package) string {
		if p == pkg {
			return ""
		}
		return p.Name()
	}

	var consts, vars, funcs []string
	var typeBlocks []string
	scope := pkg.Scope()
	scopeNames := scope.Names()
	sort.Strings(scopeNames)
	for _, name := range scopeNames {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		switch obj := obj.(type) {
		case *types.Const:
			consts = append(consts, fmt.Sprintf("const %s %s = %s",
				obj.Name(), types.TypeString(obj.Type(), qual), obj.Val().String()))
		case *types.Var:
			vars = append(vars, fmt.Sprintf("var %s %s",
				obj.Name(), types.TypeString(obj.Type(), qual)))
		case *types.Func:
			funcs = append(funcs, "func "+obj.Name()+
				strings.TrimPrefix(types.TypeString(obj.Type(), qual), "func"))
		case *types.TypeName:
			typeBlocks = append(typeBlocks, apiTypeBlock(obj, qual))
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Exported API of package %s.\n", pkgName)
	fmt.Fprintf(&b, "# Source: %s\n", label)
	b.WriteString("# Generated by TestAPISnapshot -- do not edit by hand (go generate ./...).\n")
	for _, section := range [][]string{consts, vars, funcs} {
		if len(section) == 0 {
			continue
		}
		b.WriteString("\n")
		for _, line := range section {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	for _, block := range typeBlocks {
		b.WriteString("\n")
		b.WriteString(block)
	}
	return b.String(), nil
}

// apiTypeBlock renders one exported type: its declaration line, its exported
// struct fields or interface methods, and its exported method set.
func apiTypeBlock(obj *types.TypeName, qual types.Qualifier) string {
	var b strings.Builder
	name := obj.Name()

	if obj.IsAlias() {
		fmt.Fprintf(&b, "type %s = %s\n", name, types.TypeString(obj.Type(), qual))
		return b.String()
	}

	switch under := obj.Type().Underlying().(type) {
	case *types.Struct:
		fmt.Fprintf(&b, "type %s struct\n", name)
		for i := 0; i < under.NumFields(); i++ {
			f := under.Field(i)
			if !f.Exported() {
				continue
			}
			line := fmt.Sprintf("\tfield %s %s", f.Name(), types.TypeString(f.Type(), qual))
			if f.Embedded() {
				line += " (embedded)"
			}
			if tag := under.Tag(i); tag != "" {
				line += " " + strconv.Quote(tag)
			}
			b.WriteString(line + "\n")
		}
	case *types.Interface:
		sealed := ""
		var methods []string
		for i := 0; i < under.NumMethods(); i++ {
			m := under.Method(i)
			if !m.Exported() {
				sealed = " (sealed)"
				continue
			}
			methods = append(methods, fmt.Sprintf("\tmethod %s%s", m.Name(),
				strings.TrimPrefix(types.TypeString(m.Type(), qual), "func")))
		}
		slices.Sort(methods)
		fmt.Fprintf(&b, "type %s interface%s\n", name, sealed)
		for _, m := range methods {
			b.WriteString(m + "\n")
		}
	default:
		fmt.Fprintf(&b, "type %s %s\n", name, types.TypeString(under, qual))
	}

	// The method set of the pointer type contains every method callable on an
	// addressable value, including methods promoted from embedded types. A
	// method absent from the value type's own set needs a pointer receiver.
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return b.String()
	}
	if _, isIface := obj.Type().Underlying().(*types.Interface); isIface {
		return b.String()
	}
	valueSet := map[string]bool{}
	vms := types.NewMethodSet(named)
	for i := 0; i < vms.Len(); i++ {
		valueSet[vms.At(i).Obj().Name()] = true
	}
	var methods []string
	pms := types.NewMethodSet(types.NewPointer(named))
	for i := 0; i < pms.Len(); i++ {
		fn, ok := pms.At(i).Obj().(*types.Func)
		if !ok || !fn.Exported() {
			continue
		}
		recv := name
		if !valueSet[fn.Name()] {
			recv = "*" + name
		}
		methods = append(methods, fmt.Sprintf("\tmethod (%s) %s%s", recv, fn.Name(),
			strings.TrimPrefix(types.TypeString(fn.Type(), qual), "func")))
	}
	slices.Sort(methods)
	for _, m := range methods {
		b.WriteString(m + "\n")
	}
	return b.String()
}
