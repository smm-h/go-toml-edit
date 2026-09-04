package tomledit

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
// share. The typed path-level getters live in access.go, with the two other
// accessor families and the stages all three share.
