package tomledit

import (
	"errors"
	"fmt"
	"strings"
)

// ErrSkipTable is a sentinel error returned from a Walk visitor function to skip
// the current table's children (or inline table's children). Returning
// ErrSkipTable on a scalar node is a no-op.
var ErrSkipTable = errors.New("skip table")

// WalkMode controls which nodes the Walk visitor function is called for.
type WalkMode int

const (
	// WalkLeaves visits only scalar (leaf) values. Container nodes
	// (InlineTableNode, ArrayNode) are not passed to fn, but their
	// children are still recursed into.
	WalkLeaves WalkMode = iota

	// WalkAll visits containers (inline tables, arrays) AND their children.
	// The visitor is called for every node.
	WalkAll
)

// Walk visits every key-value pair in the document in order, calling fn with
// the dot-path and the value node. Tables and array-of-tables are walked into
// (their children are visited), not yielded as standalone entries. Inline
// tables and arrays are yielded first, then their children are recursed into.
//
// The mode parameter controls which nodes are visited:
//   - WalkLeaves: only scalar values (containers are recursed but not yielded)
//   - WalkAll: containers AND their children are yielded
//
// The path uses dot-separated keys with bracket indices for array-of-tables
// entries (e.g. "servers[0].host"). Return ErrSkipTable from fn to skip the
// children of the current inline table or array. Return any other non-nil
// error to stop the walk immediately.
func (d *DocumentNode) Walk(fn func(path string, node Node) error, mode WalkMode) error {
	// Phase 1: root-level KVs (before any table header)
	for _, child := range d.Children {
		switch child := child.(type) {
		case *TableNode, *ArrayTableNode:
			// stop at first table header
		case *KeyValueNode:
			if err := walkKV("", child, fn, mode); err != nil {
				return err
			}
			continue
		default:
			// CommentNode etc. -- skip
			continue
		}
		// We hit a table/array-table; stop root-level KV scan
		break
	}

	// Phase 2: tables and array-of-tables in document order.
	// Group children by ownership: sub-tables belong to the most recently
	// preceding array-table entry whose KeyPath is a prefix of theirs.
	// This handles both sub-table path prefixing (Bug 2) and per-entry
	// counter reset for nested array-of-tables (Bug 3).

	// Walk the document children, identifying top-level table groups.
	// A child is "owned" by an array-table entry if its KeyPath starts with
	// that entry's KeyPath and it appears between that entry and the next
	// entry with the same KeyPath.
	if err := walkDocumentTables(d.Children, fn, mode); err != nil {
		return err
	}

	return nil
}

// walkDocumentTables processes all TableNode and ArrayTableNode children of
// the document, grouping sub-tables under their owning array-table entries.
func walkDocumentTables(children []Node, fn func(string, Node) error, mode WalkMode) error {
	// Global counters for top-level array-of-tables (those not owned by
	// another array-table entry).
	arrayCounters := map[string]int{}

	i := 0
	for i < len(children) {
		child := children[i]
		switch n := child.(type) {
		case *ArrayTableNode:
			// This is a top-level array-of-tables entry.
			// Collect all children owned by this entry (until the next entry
			// with the same KeyPath).
			counterKey := joinKeyPath(n.KeyPath)
			idx := arrayCounters[counterKey]
			arrayCounters[counterKey] = idx + 1

			prefix := buildArrayTablePath("", n.KeyPath, idx)

			// Walk the entry's own KV children.
			if err := walkTableChildren(prefix, n.Children, fn, mode); err != nil {
				return err
			}

			// Collect owned sub-tables: scan forward until the next entry
			// with the same KeyPath.
			ownedStart := i + 1
			ownedEnd := ownedStart
			for ownedEnd < len(children) {
				oc := children[ownedEnd]
				if at, ok := oc.(*ArrayTableNode); ok {
					if pathsEqual(at.KeyPath, n.KeyPath) {
						break // next entry of the same array-of-tables
					}
				}
				ownedEnd++
			}

			// Walk owned sub-tables with the array-table prefix.
			if err := walkOwnedSubTables(children[ownedStart:ownedEnd], n.KeyPath, prefix, fn, mode); err != nil {
				return err
			}

			i = ownedEnd

		case *TableNode:
			// A top-level table (not owned by any array-table).
			// Check if it's owned by a preceding array-table entry -- but
			// since we process in order and array-table entries consume their
			// owned children, any TableNode we see here is truly top-level.
			prefix := buildPathFromParts("", n.KeyPath)
			if err := walkTableChildren(prefix, n.Children, fn, mode); err != nil {
				return err
			}
			i++

		default:
			i++
		}
	}

	return nil
}

// walkOwnedSubTables walks TableNode and ArrayTableNode children that belong
// to a specific array-table entry. ownerKeyPath is the KeyPath of the owning
// array-table, and ownerPrefix is the resolved path prefix (e.g. "products[0]").
func walkOwnedSubTables(children []Node, ownerKeyPath []string, ownerPrefix string, fn func(string, Node) error, mode WalkMode) error {
	// Per-entry counters for nested array-of-tables (reset per parent entry).
	arrayCounters := map[string]int{}

	i := 0
	for i < len(children) {
		child := children[i]
		switch n := child.(type) {
		case *TableNode:
			// Sub-table of the array-table entry.
			// Its KeyPath starts with ownerKeyPath; strip that prefix and
			// append the remainder to ownerPrefix.
			if hasPrefix(n.KeyPath, ownerKeyPath) {
				suffix := n.KeyPath[len(ownerKeyPath):]
				subPrefix := buildPathFromParts(ownerPrefix, suffix)
				if err := walkTableChildren(subPrefix, n.Children, fn, mode); err != nil {
					return err
				}
			}
			i++

		case *ArrayTableNode:
			// Nested array-of-tables within this entry.
			if hasPrefix(n.KeyPath, ownerKeyPath) {
				counterKey := joinKeyPath(n.KeyPath)
				idx := arrayCounters[counterKey]
				arrayCounters[counterKey] = idx + 1

				suffix := n.KeyPath[len(ownerKeyPath):]
				subPrefix := buildArrayTablePath(ownerPrefix, suffix, idx)

				if err := walkTableChildren(subPrefix, n.Children, fn, mode); err != nil {
					return err
				}

				// Collect sub-tables owned by this nested array-table entry.
				ownedStart := i + 1
				ownedEnd := ownedStart
				for ownedEnd < len(children) {
					oc := children[ownedEnd]
					if at, ok := oc.(*ArrayTableNode); ok {
						if pathsEqual(at.KeyPath, n.KeyPath) {
							break // next entry of the same nested array-of-tables
						}
						// Also stop if we hit another entry of the parent
						// array-of-tables (shouldn't happen since we're already
						// scoped, but be safe).
						if pathsEqual(at.KeyPath, ownerKeyPath) {
							break
						}
					}
					ownedEnd++
				}

				if ownedEnd > ownedStart {
					if err := walkOwnedSubTables(children[ownedStart:ownedEnd], n.KeyPath, subPrefix, fn, mode); err != nil {
						return err
					}
				}

				i = ownedEnd
			} else {
				i++
			}

		default:
			i++
		}
	}

	return nil
}

// walkTableChildren walks the KV children of a table or array-table entry.
// prefix is the dot-path prefix for all children.
func walkTableChildren(prefix string, children []Node, fn func(string, Node) error, mode WalkMode) error {
	for _, child := range children {
		if kv, ok := child.(*KeyValueNode); ok {
			if err := walkKV(prefix, kv, fn, mode); err != nil {
				return err
			}
		}
	}
	return nil
}

// walkKV walks a single key-value pair. If the value is an inline table or
// array, it recurses into it. The prefix is prepended to the key parts.
func walkKV(prefix string, kv *KeyValueNode, fn func(string, Node) error, mode WalkMode) error {
	fullPath := buildPathFromParts(prefix, kv.Key.Parts)
	return walkValue(fullPath, kv.Val, fn, mode)
}

// walkValue visits a value node. Scalars are yielded directly. Inline tables
// and arrays are recursed into. In WalkLeaves mode, containers are not
// yielded to fn but their children are still recursed into.
func walkValue(path string, node Node, fn func(string, Node) error, mode WalkMode) error {
	switch v := node.(type) {
	case *InlineTableNode:
		if mode == WalkAll {
			err := fn(path, v)
			if err != nil {
				if errors.Is(err, ErrSkipTable) {
					return nil
				}
				return err
			}
		}
		for _, child := range v.Children {
			if kv, ok := child.(*KeyValueNode); ok {
				if err := walkKV(path, kv, fn, mode); err != nil {
					return err
				}
			}
		}
		return nil

	case *ArrayNode:
		if mode == WalkAll {
			err := fn(path, v)
			if err != nil {
				if errors.Is(err, ErrSkipTable) {
					return nil
				}
				return err
			}
		}
		for i, elem := range v.Elements {
			elemPath := fmt.Sprintf("%s[%d]", path, i)
			if err := walkValue(elemPath, elem, fn, mode); err != nil {
				return err
			}
		}
		return nil

	default:
		// Scalar value: yield it
		err := fn(path, node)
		if err != nil {
			if errors.Is(err, ErrSkipTable) {
				// ErrSkipTable on a non-table is a no-op (nothing to skip)
				return nil
			}
			return err
		}
		return nil
	}
}

// buildPathFromParts constructs a dot-path by appending key parts to a prefix.
func buildPathFromParts(prefix string, parts []string) string {
	result := prefix
	for _, part := range parts {
		if result != "" {
			result += "."
		}
		result += quoteKeyIfNeeded(part)
	}
	return result
}

// buildArrayTablePath constructs the path for an array-of-tables entry.
// For example, with prefix="" and keyPath=["products"], idx=0, it produces "products[0]".
// With prefix="servers[0]" and keyPath=["servers","db"], idx=1, it produces "servers[0].db[1]".
//
// The keyPath may contain multiple parts (e.g., ["a","b","c"] for [[a.b.c]]).
// All parts except the last are joined with dots; the last gets the index.
func buildArrayTablePath(prefix string, keyPath []string, idx int) string {
	if len(keyPath) == 0 {
		return prefix
	}
	// Build path for all parts except the last
	result := prefix
	for _, part := range keyPath[:len(keyPath)-1] {
		if result != "" {
			result += "."
		}
		result += quoteKeyIfNeeded(part)
	}
	// Last part gets the index
	last := keyPath[len(keyPath)-1]
	if result != "" {
		result += "."
	}
	result += quoteKeyIfNeeded(last) + fmt.Sprintf("[%d]", idx)
	return result
}

// quoteKeyIfNeeded wraps a key in quotes if it contains characters that
// would be ambiguous in a dot-path (dots, brackets, spaces, etc.).
func quoteKeyIfNeeded(key string) string {
	if key == "" {
		return `""`
	}
	for i := 0; i < len(key); i++ {
		if key[i] > 0x7F || !isBareKeyChar(key[i]) {
			return `"` + escapeKey(key) + `"`
		}
	}
	return key
}

// escapeKey escapes quotes and backslashes in a key for quoting.
func escapeKey(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// joinKeyPath joins key parts with dots for use as a map key.
func joinKeyPath(parts []string) string {
	return strings.Join(parts, ".")
}
