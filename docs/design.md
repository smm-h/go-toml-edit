---
title: Design Guide
description: Internal design of go-toml-edit covering the lex-parse-render pipeline, AST structure, dirty tracking, comment preservation, and round-trip guarantees.
---

# Design Guide

go-toml-edit is a comment-preserving TOML parser and editor for Go. Most TOML libraries parse into Go values (structs, maps) and discard everything that is not data: comments, whitespace, quoting style, key ordering, blank lines. go-toml-edit takes a different approach. It parses into a lossless AST that retains every byte of the original source, then provides editing operations that surgically modify only the nodes you touch. The result is that unmodified parts of the file come back byte-for-byte identical.

This matters for configuration files. TOML files are often maintained by humans who add comments to explain why a setting exists, group related keys with blank lines, and choose specific quoting styles for readability. A tool that rewrites the entire file after changing one value destroys that context. go-toml-edit exists so that programmatic edits respect the human's work.

## Pipeline

The library follows a three-stage pipeline -- lex, parse, render -- where the lexer tokenizes every byte of the source (including whitespace and comments), the parser builds a lossless AST with trivia attached to content nodes, and the renderer selectively emits raw bytes for clean nodes or re-renders dirty nodes from their semantic values.

### Lexing

The lexer (`lexer.go`) scans TOML source bytes into a flat sequence of tokens. Every byte in the source is accounted for -- whitespace, newlines, and comments each produce their own token types (`TokenWhitespace`, `TokenNewline`, `TokenComment`). Each token records its raw bytes and source position (1-based line and column). The lexer tracks context (key position vs. value position) and bracket nesting depth to correctly distinguish table headers (`[table]`) from array values (`[1, 2, 3]`).

### Parsing

The parser (`parser.go`) is a recursive-descent parser that consumes the token stream and builds a tree of AST nodes. The central mechanism is **trivia collection**: before parsing each content node (a key-value pair, a table header), the parser sweeps up all preceding whitespace, blank lines, and comment tokens. These become the node's **trivia** -- metadata that travels with the node through edits and back out during serialization.

Trivia is stored in the `Trivia` struct, which every node carries via `nodeBase`:

- `LeadingWhitespace`: spaces/tabs on the same line before the node
- `LeadingComments`: full comment lines (including their `#` prefix and trailing newline) that appear above the node
- `InlineComment`: a `# ...` comment on the same line after the node's value
- `TrailingNewline`: the newline byte(s) that terminate the line

Each node also stores its `raw` bytes -- the exact slice of the original source that produced it -- and a `Span` recording its start and end positions.

### Rendering

The renderer (`render.go`) serializes the AST back to bytes by walking the node tree and checking each node's dirty flag, which determines whether the node's original raw bytes can be copied directly or whether the node must be re-rendered from its semantic value with standard formatting.

- **Clean nodes** (never modified): the renderer copies `node.Raw()` directly into the output. This is what guarantees byte-for-byte round-trip fidelity.
- **Dirty nodes** (created or modified by Set, Delete, Rename, etc.): the renderer regenerates the bytes from the node's semantic value (e.g., re-rendering an integer from its `int64`, a string from its `string` with the appropriate quoting style).

The check is recursive: if a key-value pair's value is an array, and one element of that array was modified, the array node is dirty even though the key-value pair's key is clean. The function `isSubtreeDirty` walks the tree to determine this.

This design means that serialization cost is proportional to the number of edits, not the document size. A 10,000-line config file with one changed value produces output where 9,999 lines are byte-for-byte copies of the original.

## AST structure

### Node interface

All AST nodes implement the `Node` interface, which provides uniform access to the node's type, semantic value, comments, raw source bytes, and source span regardless of whether the node represents a scalar, a table, an array, or a document root.

```go
type Node interface {
    Type() NodeType
    Value() any
    Comment() string
    LeadingComments() []string
    Raw() []byte
    Span() Span
}
```

`Type()` returns an enum (`NodeDocument`, `NodeTable`, `NodeKeyValue`, `NodeString`, `NodeInteger`, etc.). `Value()` returns the semantic Go value (e.g., `string`, `int64`, `bool`, `time.Time`). `Raw()` returns the original source bytes. `Span()` returns the source range.

### Concrete node types

The document root is a `DocumentNode` whose `Children` slice contains the top-level entries: `KeyValueNode`, `TableNode`, `ArrayTableNode`, and `CommentNode`. Each node type carries its own trivia and raw bytes, and specialized fields capture TOML-specific semantics such as key quoting styles, integer bases, string styles, and array trailing comments.

- **TableNode** represents a `[table]` header. It has a `KeyPath` (e.g., `["server", "database"]` for `[server.database]`) and a `Children` slice of its key-value pairs.
- **ArrayTableNode** represents an `[[array-table]]` header. Same structure as TableNode, but multiple entries with the same KeyPath form successive elements of the array.
- **KeyValueNode** pairs a `KeyNode` (the key, possibly dotted) with a value node.
- **KeyNode** stores the key's `Parts` (decoded strings), `RawParts` (original bytes per part), and `Styles` (quoting style per part -- bare, basic-quoted, or literal-quoted). This lets the renderer preserve the original quoting style of a clean key.
- **Value nodes**: `StringNode`, `IntegerNode`, `FloatNode`, `BooleanNode`, `DateTimeNode`, `LocalDateTimeNode`, `LocalDateNode`, `LocalTimeNode`. Each stores both its semantic value and metadata like `StringStyle` or `IntegerBase`.
- **ArrayNode** holds `Elements` (a slice of value nodes) and `TrailingComments` (comments after the last element inside the brackets).
- **InlineTableNode** holds `Children` (a slice of `KeyValueNode`).
- **CommentNode** represents standalone comment lines or blank lines that are not attached to any content node (typically at the end of the file or between tables).

### Virtual node types

TOML has complex scoping rules. A table header `[a.b.c]` implicitly creates intermediate tables `a` and `a.b`. Dotted keys like `database.host = "localhost"` and `database.port = 5432` implicitly form a table under `database`. These scenarios are handled by four internal virtual node types that are never exposed in the public API:

- **dottedKeyView**: represents intermediate access into a dotted key. When resolving path `database` and the document contains `database.host = "localhost"`, this view points at the first part of the dotted key.
- **dottedKeyGroup**: groups multiple `KeyValueNode`s that share a dotted-key prefix. If the document has `database.host` and `database.port`, resolving `database` returns a group containing both.
- **compoundTableView**: represents an implicit intermediate table created by compound KeyPaths. If the document has `[a.b.c]` and you resolve `a`, this view collects all tables whose KeyPath starts with `["a"]`.
- **arrayTableCollection**: groups all `ArrayTableNode`s with the same KeyPath so they can be indexed. Resolving `servers` when the document has multiple `[[servers]]` entries returns this collection.

These virtual types implement the `Node` interface (via `nullNode` embedding) but exist only during path resolution. They have no raw bytes, no span, and no trivia.

## Path resolution

Paths use dot-separated keys with bracket indices (`server.host`, `items[0]`, `items[-1]`, `"key.with.dots"`), and the path parser in `path.go` produces a sequence of `pathSegment` values representing either key lookups or index lookups that the resolver walks step by step through the AST from the document root.

Resolution (`document.go`) walks the AST from the document root. At each step, it dispatches on the current node type:

- **DocumentNode**: searches top-level KVs, then tables, then array-tables, then builds compound views for implicit intermediate tables.
- **TableNode**: searches its direct children, then sub-tables in the document whose KeyPath extends this table's KeyPath.
- **ArrayTableNode**: same as TableNode, but scoped to the entries between this `[[table]]` header and the next one with the same KeyPath.
- **InlineTableNode**: searches its children directly.
- **Virtual nodes**: delegate to their underlying data structures.

This layered resolution is what makes path-based access work transparently regardless of whether a table is defined with a `[header]`, dotted keys, or inline syntax.

## Editing model

### Dirty tracking

Every node has a `dirty` boolean. Parsing produces all-clean nodes. Edit operations (`Set`, `Delete`, `Rename`, `SetComment`, `SetLeadingComments`) mark affected nodes as dirty. New nodes created by `Set` or `SetCreate` are born dirty (they have no raw bytes to preserve).

Dirty tracking is per-node, not per-document. When you call `Set("server.port", 9090)`, only the port's value node and its parent `KeyValueNode` are marked dirty. The `[server]` header, the `host` key-value pair, and all comments remain clean and will be emitted from their raw bytes.

### Set and SetCreate

`Set` updates an existing value at a path. If the final key does not exist in an existing parent table, it appends a new key-value pair. `SetCreate` extends this by auto-creating intermediate `[table]` headers when they do not exist.

Go values are converted to AST nodes by `valueToNode`: strings become `StringNode`, integers become `IntegerNode`, `map[string]any` becomes `InlineTableNode`, slices become `ArrayNode`, and so on. Passing an existing `Node` value is also supported for direct AST manipulation.

### Delete

`Delete` removes the node at a path. It handles key-value pairs, entire tables (including their sub-tables), array-of-tables entries, and array elements. Delete is idempotent: deleting a non-existent path returns nil (no error).

### Rename

`Rename` changes the key name of a node at a path. It updates the key's `Parts` and `RawParts`, marks both the key and its parent as dirty, and checks for conflicts with existing sibling keys.

## Comment preservation

Comments are preserved at three levels in the AST, ensuring that every comment in the original source survives parsing, editing, and serialization without loss or displacement, regardless of which nodes are modified.

1. **Leading comments**: full comment lines above a node. Stored as `[][]byte` in `Trivia.LeadingComments`. Each entry includes the `#` prefix and trailing newline.
2. **Inline comments**: a comment on the same line after a value. Stored as `[]byte` in `Trivia.InlineComment`.
3. **Standalone comments**: `CommentNode` entries in the document or table children list, representing comments that are not attached to any key-value pair (e.g., comments at the end of a file, or comments between tables with no following key-value pair).
4. **Array comments**: arrays have both per-element comments (via element trivia) and `TrailingComments` (comments after the last element but before the closing `]`).

The `SetComment` and `SetLeadingComments` methods on `DocumentNode` allow programmatic modification of comments at any path. They resolve to the appropriate node (the `KeyValueNode` for key-value paths, the `TableNode` for table paths) and update its trivia. Setting comments inside inline tables is rejected because the TOML specification does not allow comments within inline tables.

During `Merge`, comments from the source document are preserved: for new keys, comments are copied along with the value; for existing keys, source leading comments are appended to the target's leading comments, and source inline comments fill in only where the target has none.

## Round-trip guarantee

The round-trip guarantee states that for any valid TOML input `x`, calling `Parse(x).Bytes()` produces output identical to `x`, byte for byte, with no changes to whitespace, comments, quoting style, key ordering, or blank lines. This property is enforced by the test suite including fuzz testing and the full toml-test compliance suite.

The guarantee holds because:

- The lexer captures every byte (including whitespace and comments) as tokens with their raw bytes.
- The parser stores all trivia (whitespace, comments, newlines) in the node's `Trivia` struct.
- Each node stores its complete `raw` bytes.
- The renderer checks each node's dirty flag and uses raw bytes for clean nodes.

After editing, the guarantee narrows to: **unmodified regions** of the document are reproduced byte-for-byte. Modified nodes are re-rendered from their semantic values with standard formatting (e.g., `key = value\n`).

## Spans

Each node records a `Span` -- the half-open source range `[Start, End)` it occupied in the original source. Spans are set by `Parse` and reflect the most recent parse. Important constraints:

- Edit operations do not update spans. A node whose value was changed still carries the span from when it was parsed.
- Programmatically created nodes (via Set, Marshal) carry the zero Span (`IsValid()` returns false).
- To get fresh spans after editing, serialize with `Bytes()` and re-parse the result.

## Additional capabilities

### Format

`Format()` produces normalized TOML bytes by re-rendering every node from its semantic value, ignoring all raw bytes. It applies consistent formatting: configurable indentation width, line width for array wrapping, and blank lines before table headers. Format does not mutate the document -- it returns a new byte slice.

### Walk

`Walk` visits every key-value pair in document order, producing dot-separated paths with bracket indices for array-of-tables entries (e.g., `servers[0].host`). It handles the complex ownership rules of array-of-tables: sub-tables between two `[[array]]` headers with the same KeyPath belong to the preceding entry.

### Diff and Merge

`Diff` compares two documents by collecting all leaf values and producing a list of `Change` values (Added, Removed, Modified). `Merge` applies default values from one document into another, recursing into nested tables and treating scalars and arrays as atomic.

### Cursor

The `Cursor` API provides fluent, nil-safe navigation: `doc.Key("server").Key("host").String()`. A cursor captures the first error and propagates it through subsequent calls, so callers only need to check `Err()` at the end.

### Items and Len

`Items` returns a Go 1.23+ range-over-func iterator for array elements or array-of-tables entries, providing index-value pairs that work with Go's range syntax. `Len` returns the element count for arrays and array-of-tables, or negative one if the path is not found or does not point to a countable node.

### Unmarshal and Marshal

`Unmarshal` parses TOML into Go structs or maps using struct tags (`toml:"name"`). `Marshal` converts a `map[string]any` into formatted TOML bytes. Both operate through the AST: Unmarshal parses first, then walks the tree; Marshal builds a tree, then formats it.

## Limitations and edge cases

- **Span staleness**: spans become stale after edits. The library does not recompute spans because doing so would require re-lexing the modified output. If you need accurate spans after editing, re-parse the output of `Bytes()`.
- **Inline table comments**: TOML 1.0 forbids comments inside inline tables. `SetComment` returns an error if the target is inside an inline table.
- **Trivia attachment**: the parser attaches trivia to the next content node. A comment at the very end of a file (after all tables) becomes a standalone `CommentNode`. A comment between two tables is attached as leading trivia to the second table. This heuristic is correct for most real-world files but can produce surprising results for unusual comment placement.
- **Dirty re-rendering style**: when a dirty node is re-rendered, it uses standard formatting (`key = value`, basic string quoting). The original formatting choices (e.g., literal strings, hex integers, alignment whitespace) are lost for that node. Unmodified sibling nodes retain their formatting.
- **No partial key preservation on dirty KVs**: when a key-value pair is marked dirty (e.g., its value was changed via Set), the renderer re-renders the entire line. If the key was originally quoted with literal strings (`'host'`), the re-rendered key preserves the original quoting only if the key node itself is clean. If the key was also modified (e.g., via Rename), it is re-rendered with standard bare-key or basic-string quoting.
- **Marshal limitations**: `Marshal` only accepts map types as the root value. Struct encoding is not supported. Nested maps produce one level of `[section]` headers; deeper nesting uses inline tables.
