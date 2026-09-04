//go:build ignore

// Comma-ok to error conversion sweep.
//
// Converting an accessor from (T, bool) to (T, error) leaves every call site
// binding a variable that is now an error and using it as a boolean. A textual
// sweep cannot do that safely: a test function often binds `ok` several times
// -- once from a converted accessor and once from a comma-ok lookup that keeps
// its shape -- and only the type checker knows which `ok` a later `!ok` refers
// to. So this tool type-checks the package, finds the variable each converted
// call binds, and rewrites exactly the references to THAT variable:
//
//	v, ok := doc.GetInt(p)     ->  v, err := doc.GetInt(p)
//	if !ok { ... }             ->  if err != nil { ... }
//	if ok { ... }              ->  if err == nil { ... }
//	if v, ok := ...; !ok || v != want   ->  ...; err != nil || v != want
//
// A binding whose variable was declared somewhere else -- by a comma-ok lookup
// earlier in the function, say -- is reported instead of rewritten: renaming it
// would rename that other binding too. Those are the sites to convert by hand.
//
// Usage:
//
//	go run scripts/comma_ok_to_error.go --dry-run --method Document.GetInt [--method ...]
//	go run scripts/comma_ok_to_error.go --apply   --method Document.GetInt [--method ...]
//
// Exactly one of --dry-run and --apply is required; there is no default. The
// dry run reports per-file, per-method statement counts and the reference sites
// each rewrite would touch, and writes nothing. A sweep that matches no
// statement at all exits non-zero: a zero-match run is a mistyped method list,
// not a no-op worth reporting as success.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type methodList []string

func (m *methodList) String() string     { return strings.Join(*m, ",") }
func (m *methodList) Set(s string) error { *m = append(*m, s); return nil }

// edit is one byte-range replacement in one file.
type edit struct {
	start, end int
	text       string
	what       string
}

func main() {
	var methods methodList
	dry := flag.Bool("dry-run", false, "report what would change and write nothing")
	apply := flag.Bool("apply", false, "perform the rewrite")
	dir := flag.String("dir", ".", "package directory")
	newName := flag.String("name", "err", "the name the bound variable takes")
	flag.Var(&methods, "method", "TYPE.METHOD to convert, repeatable (e.g. Document.GetInt)")
	flag.Parse()

	if *dry == *apply {
		fmt.Fprintln(os.Stderr, "exactly one of --dry-run and --apply is required")
		os.Exit(2)
	}
	if len(methods) == 0 {
		fmt.Fprintln(os.Stderr, "at least one --method is required")
		os.Exit(2)
	}
	wanted := map[string]bool{}
	for _, m := range methods {
		wanted[m] = true
	}

	fset := token.NewFileSet()
	names, err := goFiles(*dir)
	if err != nil {
		fatal(err)
	}
	var files []*ast.File
	src := map[string][]byte{}
	pkgName := ""
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			fatal(err)
		}
		file, err := parser.ParseFile(fset, name, data, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			fatal(err)
		}
		// Only the package's own files (including its internal tests); an
		// external test package is a separate package and is left alone.
		if pkgName == "" && !strings.HasSuffix(file.Name.Name, "_test") {
			pkgName = file.Name.Name
		}
		if file.Name.Name != pkgName {
			continue
		}
		src[name] = data
		files = append(files, file)
	}

	info := &types.Info{
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	conf := types.Config{
		Importer: importer.ForCompiler(fset, "source", nil),
		Error:    func(error) {}, // the call sites do not type-check yet; that is the point
	}
	_, _ = conf.Check(pkgName, fset, files, info)

	// The variables the converted calls bind, and the statement that binds each.
	type binding struct {
		def    *ast.Ident
		method string
		file   string
	}
	bound := map[types.Object]*binding{}
	var manual []string
	var reuse []*ast.Ident // bindings that assign to a variable declared elsewhere
	reuseMethod := map[*ast.Ident]string{}
	statements := map[string]int{} // file+method -> count

	for _, file := range files {
		fileName := fset.Position(file.Pos()).Filename
		ast.Inspect(file, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 2 || len(as.Rhs) != 1 {
				return true
			}
			call, ok := as.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			qualified := qualify(info, sel)
			if !wanted[qualified] {
				return true
			}
			statements[fileName+"\t"+qualified]++

			ident, ok := as.Lhs[1].(*ast.Ident)
			if !ok || ident.Name == "_" {
				return true
			}
			if obj := info.Defs[ident]; obj != nil {
				bound[obj] = &binding{def: ident, method: qualified, file: fileName}
				return true
			}
			// The variable is not declared here. Whether that is a problem
			// depends on what declared it, which is only known once every
			// binding has been collected.
			reuse = append(reuse, ident)
			reuseMethod[ident] = qualified
			return true
		})
	}
	for _, ident := range reuse {
		if bound[info.Uses[ident]] != nil {
			continue // declared by another converted call: renamed with it
		}
		manual = append(manual, fmt.Sprintf("%s: %s assigns to %q, declared elsewhere",
			fset.Position(ident.Pos()), reuseMethod[ident], ident.Name))
	}

	if len(statements) == 0 {
		fatal(fmt.Errorf("no call site matched %v", methods))
	}

	// Every reference to a bound variable, with the shape its context needs.
	edits := map[string][]edit{}
	add := func(pos, end token.Pos, text, what string) {
		p := fset.Position(pos)
		edits[p.Filename] = append(edits[p.Filename], edit{
			start: p.Offset, end: fset.Position(end).Offset, text: text, what: what,
		})
	}
	for _, b := range bound {
		add(b.def.Pos(), b.def.End(), *newName, "binding")
	}
	for _, file := range files {
		var stack []ast.Node
		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, n)
			ident, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			obj := info.Uses[ident]
			if obj == nil || bound[obj] == nil {
				return true
			}
			var parent ast.Node
			if len(stack) > 1 {
				parent = stack[len(stack)-2]
			}
			switch p := parent.(type) {
			case *ast.UnaryExpr:
				if p.Op == token.NOT {
					add(p.Pos(), p.End(), *newName+" != nil", "!ok")
					return true
				}
			case *ast.BinaryExpr:
				if p.Op == token.LAND || p.Op == token.LOR {
					add(ident.Pos(), ident.End(), *newName+" == nil", "ok in a boolean operand")
					return true
				}
			case *ast.IfStmt:
				if p.Cond == ast.Expr(ident) {
					add(ident.Pos(), ident.End(), *newName+" == nil", "if ok")
					return true
				}
			case *ast.ForStmt:
				if p.Cond == ast.Expr(ident) {
					add(ident.Pos(), ident.End(), *newName+" == nil", "for ok")
					return true
				}
			}
			add(ident.Pos(), ident.End(), *newName, "reference")
			return true
		})
	}

	report(statements, edits, manual)

	if *dry {
		return
	}
	for name, list := range edits {
		data := src[name]
		sort.Slice(list, func(i, j int) bool { return list[i].start > list[j].start })
		out := append([]byte(nil), data...)
		for _, e := range list {
			out = append(out[:e.start], append([]byte(e.text), out[e.end:]...)...)
		}
		formatted, err := format.Source(out)
		if err != nil {
			// Leave the unformatted bytes: the compiler names what is wrong.
			formatted = out
			fmt.Fprintf(os.Stderr, "%s: not gofmt-parseable after the rewrite: %v\n", name, err)
		}
		if bytes.Equal(formatted, data) {
			continue
		}
		if err := os.WriteFile(name, formatted, 0o644); err != nil {
			fatal(err)
		}
	}
	fmt.Printf("rewrote %d file(s)\n", len(edits))
}

// qualify renders a method selection as TYPE.METHOD, with the receiver's
// pointer and package qualification stripped.
func qualify(info *types.Info, sel *ast.SelectorExpr) string {
	s := info.Selections[sel]
	if s == nil {
		return ""
	}
	fn, ok := s.Obj().(*types.Func)
	if !ok {
		return ""
	}
	recv := fn.Type().(*types.Signature).Recv()
	if recv == nil {
		return ""
	}
	name := types.TypeString(recv.Type(), func(*types.Package) string { return "" })
	name = strings.TrimPrefix(name, "*")
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	return name + "." + fn.Name()
}

func report(statements map[string]int, edits map[string][]edit, manual []string) {
	keys := make([]string, 0, len(statements))
	for k := range statements {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	total := 0
	fmt.Println("call sites, per file and method:")
	for _, k := range keys {
		parts := strings.SplitN(k, "\t", 2)
		fmt.Printf("  %-40s %-24s %d\n", parts[0], parts[1], statements[k])
		total += statements[k]
	}
	fmt.Printf("  %-40s %-24s %d\n", "TOTAL", "", total)

	files := make([]string, 0, len(edits))
	for f := range edits {
		files = append(files, f)
	}
	sort.Strings(files)
	shapes := map[string]int{}
	rewrites := 0
	fmt.Println("references rewritten, per file:")
	for _, f := range files {
		fmt.Printf("  %-40s %d\n", f, len(edits[f]))
		rewrites += len(edits[f])
		for _, e := range edits[f] {
			shapes[e.what]++
		}
	}
	fmt.Printf("  %-40s %d\n", "TOTAL", rewrites)
	fmt.Println("reference shapes:")
	shapeKeys := make([]string, 0, len(shapes))
	for k := range shapes {
		shapeKeys = append(shapeKeys, k)
	}
	sort.Strings(shapeKeys)
	for _, k := range shapeKeys {
		fmt.Printf("  %-40s %d\n", k, shapes[k])
	}
	if len(manual) > 0 {
		fmt.Printf("left for hand conversion (%d):\n", len(manual))
		for _, m := range manual {
			fmt.Printf("  %s\n", m)
		}
	}
}

func goFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		names = append(names, filepath.Join(dir, e.Name()))
	}
	sort.Strings(names)
	return names, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "comma_ok_to_error:", err)
	os.Exit(1)
}
