package tomledit

import (
	"math"
	"reflect"
	"sort"
	"time"
)

// ChangeKind identifies the type of difference between two documents.
type ChangeKind int

const (
	Added    ChangeKind = iota // Added means the key exists in b but not in a.
	Removed                    // Removed means the key exists in a but not in b.
	Modified                   // Modified means the key exists in both but with different values.
)

var changeKindNames = [...]string{
	Added:    "added",
	Removed:  "removed",
	Modified: "modified",
}

// String returns the human-readable name of the change kind.
func (k ChangeKind) String() string {
	if int(k) >= 0 && int(k) < len(changeKindNames) {
		return changeKindNames[k]
	}
	return "unknown"
}

// Change represents a single difference between two documents.
// OldValue is nil for Added changes; NewValue is nil for Removed changes.
type Change struct {
	Kind     ChangeKind
	Path     string
	OldValue any // nil for Added
	NewValue any // nil for Removed
}

// Diff returns all differences between documents a and b.
//
// It walks both documents to collect all leaf (scalar) values, then compares
// them. Container nodes (inline tables, arrays) are not compared directly;
// instead their individual elements are compared. Changes are sorted by path
// (alphabetical), then by kind (Removed, Modified, Added).
//
// The comparison reads values, never spellings. Two documents writing one
// value differently -- 0x2A against 42, 1_000 against 1000, one instant in two
// zone offsets, a literal string against a basic one, an array-of-tables
// against an inline array of inline tables -- report no difference. A document
// therefore always compares equal to itself.
//
// Types are not bridged: an integer and a float never compare equal, so 1 and
// 1.0 are a modification.
func Diff(a, b *Document) []Change {
	aLeaves := collectLeaves(a)
	bLeaves := collectLeaves(b)

	var changes []Change

	// Removed or Modified: paths in a
	for path, oldVal := range aLeaves {
		newVal, exists := bLeaves[path]
		if !exists {
			changes = append(changes, Change{
				Kind:     Removed,
				Path:     path,
				OldValue: oldVal,
			})
		} else if !valuesEqual(oldVal, newVal) {
			changes = append(changes, Change{
				Kind:     Modified,
				Path:     path,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}

	// Added: paths in b but not in a
	for path, newVal := range bLeaves {
		if _, exists := aLeaves[path]; !exists {
			changes = append(changes, Change{
				Kind:     Added,
				Path:     path,
				NewValue: newVal,
			})
		}
	}

	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Path != changes[j].Path {
			return changes[i].Path < changes[j].Path
		}
		return changes[i].Kind < changes[j].Kind
	})

	return changes
}

// collectLeaves walks a document and returns a map of path -> semantic value
// for all leaf (scalar) nodes.
func collectLeaves(doc *Document) map[string]any {
	leaves := make(map[string]any)
	if doc == nil {
		return leaves
	}
	doc.Walk(func(path string, node Node) error {
		// WalkLeaves yields only value-carrying nodes, which are exactly the
		// ones that answer for a value.
		if s, ok := node.(Scalar); ok {
			leaves[path] = s.Value()
		}
		return nil
	}, WalkLeaves)
	return leaves
}

// valuesEqual compares two leaf values.
//
// Two values of different Go types are never equal, which is what keeps an
// integer and a float apart. Within a type, the comparison is by value and not
// by spelling: two offset date-times naming one instant are equal whatever
// zones they were written in, and two not-a-numbers are equal because a
// document must compare equal to itself and IEEE inequality would deny it.
func valuesEqual(a, b any) bool {
	switch va := a.(type) {
	case time.Time:
		tb, ok := b.(time.Time)
		return ok && va.Equal(tb)
	case float64:
		fb, ok := b.(float64)
		if !ok {
			return false
		}
		if math.IsNaN(va) && math.IsNaN(fb) {
			return true
		}
		return va == fb
	}
	return reflect.DeepEqual(a, b)
}
