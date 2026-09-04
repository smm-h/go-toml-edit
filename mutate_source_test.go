// The test lives in the external test package for the reason api_snapshot_test
// gives: the package's own identifiers include `parser`, `token` and `lexer`,
// which would collide with the go/parser and go/token imports these source
// walks need.
package tomledit_test

// Two structural guards over the write funnels of mutate.go.
//
// A node's payload and the lexeme it was written as must never disagree, and a
// container's contents and the read-layer folded from them must never disagree
// either. Both invariants are kept by the funnels doing the invalidation the
// writer would otherwise have to remember; both are only as good as the claim
// that nothing writes those fields anywhere else. These walks are that claim.
//
// Construction is exempt on purpose: a composite literal builds a node whose
// payload, lexeme and contents agree by construction, and there is nothing
// stale to invalidate. Only assignment -- to a field, or into a field's slice
// -- can put an existing node into a state its bytes no longer describe.

import (
	"go/ast"
	"sort"
	"testing"
)

// mutatorFile is the one file allowed to write the guarded fields.
const mutatorFile = "mutate.go"

// scalarPayloadFields are the fields of the struct that pairs a scalar's
// payload with its lexeme. Writing either without the other is the drift the
// funnel exists to prevent.
var scalarPayloadFields = map[string]string{
	"payload": "replace a scalar's payload with its kind's setValue, which drops the lexeme with it",
	"lexeme":  "record a lexeme through lexed or setRaw",
}

// structureFields are the fields holding what a container carries, what a key
// spells, and which container a node sits in. Writing one changes the shape the
// read-layer was folded from and the bytes the serializer would splice.
var structureFields = map[string]string{
	"children":         "change a container's contents with setContents, appendContent, insertContent or removeContent",
	"elements":         "change an array's contents with setContents, appendContent, insertContent or removeContent",
	"keyPath":          "build a header's key path in its composite literal",
	"parts":            "rename a key part with renamePart, or build a key with buildPart",
	"rawParts":         "rename a key part with renamePart, or build a key with buildPart",
	"styles":           "rename a key part with renamePart, or build a key with buildPart",
	"key":              "build a pair with newPair",
	"val":              "replace a pair's value with setVal, and a scalar's payload with its kind's setValue",
	"trailingComments": "record an array's trailing comments with buildTrailingComments",
	"parent":           "record a parent link with adopt, which the content funnels call",
	"dirty":            "mark a node with markDirty",
	"subtree":          "mark a node with markDirty, which says so upward",
}

// Fails if anything outside mutate.go writes a scalar's payload or its lexeme.
func TestScalarPayloadWrittenInOneFile(t *testing.T) {
	assertNoFieldWrites(t, scalarPayloadFields)
}

// Fails if anything outside mutate.go writes a container's contents, a key's
// parts, or a node's parent link and dirtiness.
func TestStructureWrittenInOneFile(t *testing.T) {
	assertNoFieldWrites(t, structureFields)
}

// assertNoFieldWrites reports every assignment, anywhere but the mutator file,
// whose target is one of the named fields -- written directly, through a
// compound assignment, or into one of its elements.
func assertNoFieldWrites(t *testing.T, fields map[string]string) {
	t.Helper()
	fset, files := packageFiles(t)

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if name == mutatorFile {
			continue
		}
		ast.Inspect(files[name], func(n ast.Node) bool {
			var targets []ast.Expr
			switch node := n.(type) {
			case *ast.AssignStmt:
				targets = node.Lhs
			case *ast.IncDecStmt:
				targets = []ast.Expr{node.X}
			default:
				return true
			}
			for _, target := range targets {
				field := writtenField(target)
				if field == "" {
					continue
				}
				if remedy, guarded := fields[field]; guarded {
					t.Errorf("%s: writes %s outside %s -- %s",
						fset.Position(target.Pos()), field, mutatorFile, remedy)
				}
			}
			return true
		})
	}
}

// writtenField returns the field name an assignment target names, seeing
// through element and slice writes (`n.parts[0] = x`, `n.children[i:j] = ...`),
// or "" when the target names no field.
func writtenField(target ast.Expr) string {
	for {
		switch e := target.(type) {
		case *ast.IndexExpr:
			target = e.X
		case *ast.SliceExpr:
			target = e.X
		case *ast.StarExpr:
			target = e.X
		case *ast.ParenExpr:
			target = e.X
		case *ast.SelectorExpr:
			return e.Sel.Name
		default:
			return ""
		}
	}
}
