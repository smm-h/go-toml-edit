package tomledit

import "time"

// normalizeIndex converts a possibly-negative index to a valid non-negative index.
func normalizeIndex(index, length int) (int, error) {
	if length == 0 {
		return 0, newError(KindNotFound, "index %d out of range (empty collection)", index)
	}
	idx := index
	if idx < 0 {
		idx = length + idx
	}
	if idx < 0 || idx >= length {
		return 0, newError(KindNotFound, "index %d out of range (length %d)", index, length)
	}
	return idx, nil
}

// pathsEqual returns true if two string slices are equal.
func pathsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// hasPrefix returns true if path starts with prefix.
func hasPrefix(path, prefix []string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

// --- Public API methods on Document ---
//
// Resolve, Lookup and Has live in resolve.go, with the read-layer walk they
// share. The typed getters below are their conveniences.

// GetString resolves the path and returns the string value. Returns ("", false)
// if the path is not found or the value is not a string.
func (d *Document) GetString(path string) (string, bool) {
	node, ok := d.Lookup(path)
	if !ok {
		return "", false
	}
	if s, ok := node.(*StringNode); ok {
		return s.val.get(), true
	}
	return "", false
}

// GetInt resolves the path and returns the integer value. Returns (0, false)
// if the path is not found or the value is not an integer.
func (d *Document) GetInt(path string) (int64, bool) {
	node, ok := d.Lookup(path)
	if !ok {
		return 0, false
	}
	if n, ok := node.(*IntegerNode); ok {
		return n.val.get(), true
	}
	return 0, false
}

// GetBool resolves the path and returns the boolean value. Returns (false, false)
// if the path is not found or the value is not a boolean.
func (d *Document) GetBool(path string) (bool, bool) {
	node, ok := d.Lookup(path)
	if !ok {
		return false, false
	}
	if b, ok := node.(*BooleanNode); ok {
		return b.val.get(), true
	}
	return false, false
}

// GetFloat resolves the path and returns the float64 value. Returns (0, false)
// if the path is not found or the value is not a float.
func (d *Document) GetFloat(path string) (float64, bool) {
	node, ok := d.Lookup(path)
	if !ok {
		return 0, false
	}
	if f, ok := node.(*FloatNode); ok {
		return f.val.get(), true
	}
	return 0, false
}

// GetTime resolves the path and returns a time.Time value. Returns (time.Time{}, false)
// if the path is not found or the value is not an offset date-time.
func (d *Document) GetTime(path string) (time.Time, bool) {
	node, ok := d.Lookup(path)
	if !ok {
		return time.Time{}, false
	}
	if dt, ok := node.(*DateTimeNode); ok {
		return dt.val.get(), true
	}
	return time.Time{}, false
}
