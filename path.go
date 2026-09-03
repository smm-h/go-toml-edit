package tomledit

import (
	"strconv"
	"strings"
)

// SegmentKind distinguishes the two kinds of path step: a lookup by key and a
// lookup by position.
type SegmentKind int

const (
	// SegmentKey addresses a child by name.
	SegmentKey SegmentKind = iota
	// SegmentIndex addresses an element by position. A negative index counts
	// from the end, so -1 is the last element.
	SegmentIndex
)

// PathSegment is one step of a parsed path. Kind says which of the remaining
// fields carries the step: Key for SegmentKey, Index for SegmentIndex.
type PathSegment struct {
	Kind  SegmentKind
	Key   string // the key to look up, for SegmentKey
	Index int    // the element position, for SegmentIndex; negative counts from the end
}

// ParsePath parses a path in this package's path syntax into its segments.
// Every path-addressed operation in the package -- Resolve, Lookup, Set,
// Delete, the comment setters -- reads its path with it, so a path that parses
// here is spelled the way they expect.
//
// Syntax:
//   - A dot separates key segments: "server.host".
//   - Brackets hold an index, which may be negative: "items[0]", "items[-1]".
//   - Brackets may follow each other for nested arrays: "matrix[0][1]".
//   - A quoted segment carries a key verbatim, so a key with dots or spaces
//     stays one segment: `server."host.name"`. The quotes belong to the path
//     syntax, not to the key.
//   - A backslash escapes the next byte: `host\.name` is the single key
//     "host.name", and inside a quoted segment `\"` is a quote.
//
// The empty path names nothing and is reported as a KindBadPath diagnostic;
// so are an unclosed bracket or quote, a non-numeric index, and a trailing dot.
func ParsePath(path string) ([]PathSegment, error) {
	if path == "" {
		return nil, newError(KindBadPath, "empty path")
	}

	var segments []PathSegment
	i := 0

	for i < len(path) {
		// Skip leading dot (separator between segments), but not at start
		if i > 0 && path[i] == '.' {
			i++
			if i >= len(path) {
				return nil, newError(KindBadPath, "trailing dot in path")
			}
		}

		if path[i] == '[' {
			// Index segment
			seg, newI, err := parseIndex(path, i)
			if err != nil {
				return nil, err
			}
			segments = append(segments, seg)
			i = newI
		} else if path[i] == '"' {
			// Quoted key segment
			seg, newI, err := parseQuotedKey(path, i)
			if err != nil {
				return nil, err
			}
			segments = append(segments, seg)
			i = newI
		} else {
			// Bare key segment (may contain escaped dots)
			seg, newI := parseBareKey(path, i)
			segments = append(segments, seg)
			i = newI
		}
	}

	return segments, nil
}

// JoinPath renders segments as path text, and is the single quoting authority
// for paths: a key segment is written bare when every byte of it is legal in a
// bare key, and quoted (with quotes and backslashes escaped) otherwise, so that
// ParsePath reads back exactly the segments JoinPath was given. An index
// segment is written in brackets, attached to whatever precedes it.
//
// The result of joining no segments is the empty string, which ParsePath
// refuses: a path names at least one step.
func JoinPath(segs []PathSegment) string {
	var b strings.Builder
	for _, seg := range segs {
		if seg.Kind == SegmentIndex {
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(seg.Index))
			b.WriteByte(']')
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(quotePathKey(seg.Key))
	}
	return b.String()
}

// quotePathKey renders one key as a path segment, quoting it unless every byte
// is legal in a bare key.
func quotePathKey(key string) string {
	if isBarePathKey(key) {
		return key
	}
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(key); i++ {
		if key[i] == '"' || key[i] == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(key[i])
	}
	b.WriteByte('"')
	return b.String()
}

// isBarePathKey reports whether key can appear in a path without quotes. It is
// deliberately separate from the renderer's bare-key predicate: this one
// governs path text, that one governs TOML source.
func isBarePathKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		if !isBareKeyChar(key[i]) {
			return false
		}
	}
	return true
}

// parseIndex parses "[N]" or "[-N]" starting at position i.
func parseIndex(path string, i int) (PathSegment, int, error) {
	if i >= len(path) || path[i] != '[' {
		return PathSegment{}, i, newError(KindBadPath, "expected '['")
	}
	i++ // skip '['

	end := strings.IndexByte(path[i:], ']')
	if end < 0 {
		return PathSegment{}, i, newError(KindBadPath, "unclosed bracket in path")
	}
	end += i

	content := path[i:end]
	if content == "" {
		return PathSegment{}, end, newError(KindBadPath, "empty index in path")
	}

	idx, err := strconv.Atoi(content)
	if err != nil {
		return PathSegment{}, end, newError(KindBadPath, "non-numeric index %q in path", content)
	}

	return PathSegment{Kind: SegmentIndex, Index: idx}, end + 1, nil
}

// parseQuotedKey parses a quoted key segment like "host.name" starting at position i.
func parseQuotedKey(path string, i int) (PathSegment, int, error) {
	if i >= len(path) || path[i] != '"' {
		return PathSegment{}, i, newError(KindBadPath, "expected '\"'")
	}
	i++ // skip opening quote

	var key strings.Builder
	for i < len(path) {
		if path[i] == '\\' && i+1 < len(path) {
			// Escaped character inside quotes
			key.WriteByte(path[i+1])
			i += 2
			continue
		}
		if path[i] == '"' {
			return PathSegment{Kind: SegmentKey, Key: key.String()}, i + 1, nil
		}
		key.WriteByte(path[i])
		i++
	}
	return PathSegment{}, i, newError(KindBadPath, "unclosed quote in path")
}

// parseBareKey parses a bare key segment, handling backslash-escaped dots.
func parseBareKey(path string, i int) (PathSegment, int) {
	var key strings.Builder
	for i < len(path) {
		if path[i] == '\\' && i+1 < len(path) && path[i+1] == '.' {
			// Escaped dot: literal dot in key
			key.WriteByte('.')
			i += 2
			continue
		}
		if path[i] == '.' || path[i] == '[' {
			break
		}
		key.WriteByte(path[i])
		i++
	}
	return PathSegment{Kind: SegmentKey, Key: key.String()}, i
}

// pathFromKeys renders a key path (the parts of a table header or a dotted
// key) as path text through JoinPath, so error messages and the diagnostic
// path field quote exactly like the path parser reads.
func pathFromKeys(parts []string) string {
	segs := make([]PathSegment, len(parts))
	for i, part := range parts {
		segs[i] = PathSegment{Kind: SegmentKey, Key: part}
	}
	return JoinPath(segs)
}
