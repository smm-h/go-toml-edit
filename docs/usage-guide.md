---
title: Usage Guide
description: "Comprehensive guide to parsing, querying, editing, and serializing TOML documents with go-toml-edit, including comments, merging, diffing, and marshaling."
nav_group: "Guides"
nav_order: 1
---

# Usage Guide

go-toml-edit is a Go library for parsing, editing, and serializing TOML documents while preserving comments, whitespace, and formatting. It provides a lossless AST that keeps every byte of the original document intact, only re-rendering nodes that you explicitly modify.

## Installation

```
go get github.com/smm-h/go-toml-edit
```

All public types and functions live in the `tomledit` package.

```go
import "github.com/smm-h/go-toml-edit"
```

## Parsing TOML

### From a byte slice

The primary entry point is `tomledit.Parse`, which takes a `[]byte` containing TOML source text and returns a `*Document` AST that preserves every byte of the original input including comments, whitespace, and quoting styles. Parse errors are returned as `*tomledit.ParseError` values with line and column information for precise diagnostics.

```go
input := []byte(`
# Server configuration
[server]
host = "localhost"  # bind address
port = 8080
`)

doc, err := tomledit.Parse(input)
if err != nil {
    log.Fatal(err)
}
```

Parse errors are returned as `*tomledit.ParseError` values with line and column information:

```go
if pe, ok := err.(*tomledit.ParseError); ok {
    fmt.Printf("Parse error at line %d, column %d: %s\n",
        pe.Line, pe.Column, pe.Message)
}
```

### From a file

There is no dedicated file-reading function; read the file into memory with `os.ReadFile` or similar, then pass the resulting byte slice to `tomledit.Parse`. This keeps the library free of I/O dependencies and gives you full control over file handling, encoding, and error reporting.

```go
data, err := os.ReadFile("config.toml")
if err != nil {
    log.Fatal(err)
}
doc, err := tomledit.Parse(data)
if err != nil {
    log.Fatal(err)
}
```

## Querying values

### Path-based access with Get

The `Get` method resolves a dot-separated path through the document's AST and returns the value node at that location. It returns `nil` if the path does not exist or is syntactically invalid, making it safe for simple lookups where you only need to check for a nil result rather than inspect a detailed error.

```go
node := doc.Get("server.host")
if node != nil {
    fmt.Println(node.Value()) // "localhost"
}
```

Path syntax:

- Dot separates keys: `"server.host"`, `"database.connection.pool_size"`
- Brackets index into arrays: `"items[0]"`, `"items[-1]"` (negative indices count from the end)
- Quoted segments for keys containing dots: `"\"host.name\""` (Go string: `"\"host.name\""`)

### Typed getters

Convenience methods extract values with type checking, combining path resolution and type assertion in a single call. Each returns a value of the expected Go type and a boolean indicating whether the path existed and the value had the correct type, returning the zero value and false otherwise.

```go
host, ok := doc.GetString("server.host")     // (string, bool)
port, ok := doc.GetInt("server.port")         // (int64, bool)
debug, ok := doc.GetBool("server.debug")      // (bool, bool)
rate, ok := doc.GetFloat("server.rate")        // (float64, bool)
ts, ok := doc.GetTime("server.started_at")     // (time.Time, bool)
```

If the path does not exist or the value is a different type, the zero value and `false` are returned.

### Resolve with error details

`Resolve` works like `Get` but returns an error instead of `nil` when the path cannot be resolved, making it possible to distinguish between a path that does not exist in the document and a path that is syntactically invalid. This is useful when you need to provide detailed diagnostics about why a lookup failed.

```go
node, err := doc.Resolve("server.host")
if err != nil {
    fmt.Println("resolution failed:", err)
}
```

### Cursor API

The cursor provides a fluent, nil-safe API for step-by-step navigation through the document tree, where each method call returns a new cursor positioned at the next node. Errors are captured internally and deferred until you explicitly check `Err()`, so you can chain multiple navigation steps without intermediate error checks.

```go
host, ok := doc.Key("server").Key("host").String()
fmt.Println(host, ok) // "localhost" true

port, ok := doc.Key("server").Key("port").Int()
fmt.Println(port, ok) // 8080 true
```

Navigate into arrays with `At`:

```go
name, ok := doc.Key("products").At(0).Key("name").String()
```

Check for navigation errors:

```go
cursor := doc.Key("nonexistent").Key("path")
if cursor.Err() != nil {
    fmt.Println("navigation error:", cursor.Err())
}
```

### Iterating over arrays

The `Items` method returns a Go 1.23+ range-over-func iterator that yields index-value pairs for array elements and array-of-tables entries, providing a natural way to loop over ordered collections in the document without manual index tracking or length checks.

```go
for i, node := range doc.Items("servers") {
    fmt.Printf("server %d: %v\n", i, node.Value())
}
```

Use `Len` to check the number of elements without iterating:

```go
n := doc.Len("servers") // -1 if not found or not an array
```

Both are also available on cursor values:

```go
cursor := doc.Key("servers")
for i, node := range cursor.Items() {
    fmt.Printf("server %d\n", i)
    _ = node
}
```

## Modifying values

### Set

`Set` updates the value at a path, replacing the existing value node while preserving any comments and trivia attached to the key-value pair. If the final key does not exist in an existing parent table, it is appended as a new key-value pair. Intermediate path segments must already exist; use `SetCreate` if you need automatic table creation.

```go
err := doc.Set("server.port", 9090)
err = doc.Set("server.host", "0.0.0.0")
err = doc.Set("server.debug", true)
err = doc.Set("server.rate", 0.75)
```

Supported Go types: `string`, `bool`, `int`/`int8`-`int64`, `uint`/`uint8`-`uint64`, `float32`/`float64`, `time.Time`, `tomledit.LocalDateTime`, `tomledit.LocalDate`, `tomledit.LocalTime`, `[]any`, `map[string]any`, and any value implementing the `tomledit.Node` interface.

### SetCreate

`SetCreate` extends `Set` by automatically creating intermediate `[table]` headers along the path when they do not already exist, making it possible to write deeply nested values in a single call without manually creating each parent table first. The created tables are appended to the document in order.

```go
// Even if [database] does not exist yet, this creates it automatically.
err := doc.SetCreate("database.connection.host", "db.example.com")
```

### Setting arrays and inline tables

Pass a `[]any` slice to create a TOML array or a `map[string]any` to create an inline table. Go values are automatically converted to the appropriate AST node types, and typed slices such as `[]string` or `[]int` are also accepted for convenience.

```go
err := doc.Set("server.ports", []any{8080, 8081, 8082})
err = doc.Set("server.tls", map[string]any{
    "cert": "/path/to/cert.pem",
    "key":  "/path/to/key.pem",
})
```

Typed slices (e.g., `[]string`, `[]int`) are also accepted.

### Setting array elements by index

```go
err := doc.Set("server.ports[0]", 9090)     // replace first element
err = doc.Set("server.ports[-1]", 9999)      // replace last element
```

## Adding new sections

### NewTable

Create a new `[table]` header in the document, which appends a table section that can then be populated with key-value pairs using `Set`. Returns an error if a table with the specified path already exists, preventing accidental duplication of sections.

```go
err := doc.NewTable("logging")
// Then populate it:
err = doc.Set("logging.level", "info")
err = doc.Set("logging.file", "/var/log/app.log")
```

Returns an error if a table with that path already exists.

### NewArrayTable

Append a new `[[array-table]]` entry to the document. Multiple entries with the same path represent successive elements of the array, following the TOML specification's array-of-tables semantics. Use negative indexing (`[-1]`) to set values on the most recently appended entry.

```go
err := doc.NewArrayTable("products")
err = doc.Set("products[-1].name", "Widget")
err = doc.Set("products[-1].price", 9.99)

err = doc.NewArrayTable("products")
err = doc.Set("products[-1].name", "Gadget")
err = doc.Set("products[-1].price", 19.99)
```

## Removing keys

`Delete` removes the node at a path, handling key-value pairs, entire table sections including their sub-tables, array-of-tables entries, and individual array elements. It is idempotent and safe to call on paths that do not exist, returning nil without error in that case.

```go
err := doc.Delete("server.debug")    // remove a key-value pair
err = doc.Delete("logging")          // remove an entire table
err = doc.Delete("products[0]")      // remove an array element
err = doc.Delete("nonexistent.key")  // no-op, no error
```

## Renaming keys

`RenameKey` changes the key name of a node at a given path, updating the key's internal parts and raw representation while marking the node as dirty for re-rendering. It returns an error if the target name already exists among sibling keys or the path is not found in the document.

```go
err := doc.RenameKey("server.host", "bind_address")
// server.host is now server.bind_address
```

## Walking the document

`Walk` visits every key-value pair in document order, calling a callback function with the dot-separated path and the value node at each position. It handles the complex ownership rules of array-of-tables entries and supports two modes for controlling whether container nodes are yielded to the callback.

```go
err := doc.Walk(func(path string, node tomledit.Node) error {
    fmt.Printf("%s = %v (%s)\n", path, node.Value(), node.Type())
    return nil
}, tomledit.WalkLeaves)
```

Two walk modes are available:

- `tomledit.WalkLeaves` -- visits only scalar (leaf) values; containers (arrays, inline tables) are recursed into but not yielded to the callback
- `tomledit.WalkAll` -- visits containers AND their children; the callback receives inline tables and arrays as well as their elements

Return `tomledit.ErrSkipTable` from the callback to skip the children of the current inline table or array.

Array-of-tables paths include the index: `"products[0].name"`, `"products[1].name"`.

## Comments

go-toml-edit preserves all comments from the original document through its trivia system, which attaches leading comments, inline comments, and standalone comments to their nearest content nodes. You can also read and write comments programmatically using the `Comment()`, `LeadingComments()`, `SetComment()`, and `SetLeadingComments()` methods.

### Reading comments

Every node exposes `Comment()` for the inline comment on the same line after the value and `LeadingComments()` for comment lines that appear above it in the source. Both methods return the comment text as it appears in the original document, preserving the exact wording and formatting.

```go
node := doc.Get("server.host")
if node != nil {
    fmt.Println("inline:", node.Comment())
    fmt.Println("leading:", node.LeadingComments())
}
```

### Setting inline comments

`SetComment` sets the inline comment on a node at the specified path, placing the comment text on the same line after the value. The `"# "` prefix is added automatically, and passing an empty string removes the comment entirely without affecting the value or any leading comments.

```go
err := doc.SetComment("server.port", "default HTTP port")
// Produces: port = 8080 # default HTTP port
```

Pass an empty string to remove the comment:

```go
err := doc.SetComment("server.port", "")
```

### Setting leading comments

`SetLeadingComments` replaces all comment lines above a node at the specified path with the provided strings. Each string should be the bare comment text without the `"# "` prefix, which is added automatically during rendering. This is useful for adding documentation or explanatory notes directly above configuration keys.

```go
err := doc.SetLeadingComments("server.host", []string{
    "The hostname or IP address to bind to.",
    "Use 0.0.0.0 for all interfaces.",
})
```

This produces:

```toml
# The hostname or IP address to bind to.
# Use 0.0.0.0 for all interfaces.
host = "localhost"
```

### Comments and inline tables

The TOML specification does not allow comments inside inline tables, which must fit on a single line. Attempting to set a comment on a member of an inline table returns an error from `SetComment` and `SetLeadingComments`, enforcing this constraint at the API level rather than silently producing invalid TOML.

## Comment preservation guarantee

The library guarantees that parsing a document and serializing it back with `Bytes()` produces the exact original bytes, with no changes to comments, whitespace, quoting styles, or formatting. When you modify a value, only the changed node is re-rendered while all surrounding comments and formatting are preserved from the original source bytes.

```go
original := []byte(`# My config
[server]
host = "localhost"  # primary host
port = 8080
`)

doc, _ := tomledit.Parse(original)
output := doc.Bytes()

// output is byte-for-byte identical to original
fmt.Println(bytes.Equal(original, output)) // true
```

When you modify a value, only the changed node is re-rendered. Everything else -- comments, blank lines, indentation, quoting style -- is preserved from the original source:

```go
doc, _ := tomledit.Parse([]byte(`# Server settings
[server]
host = "localhost"  # bind address
port = 8080         # default port
debug = true
`))

doc.Set("server.port", 9090)
doc.Delete("server.debug")

fmt.Print(string(doc.Bytes()))
// Output:
// # Server settings
// [server]
// host = "localhost"  # bind address
// port = 9090         # default port
```

The comment on `port` survives the value change. The comment on `host` is untouched. Blank lines and the section comment are preserved exactly.

## Serializing back to TOML

### Bytes -- round-trip fidelity

`Bytes()` returns the document as TOML bytes with round-trip fidelity, where unmodified nodes emit their original raw bytes directly and only dirty (edited) nodes are re-rendered from their semantic values. This makes the serialization cost proportional to the number of edits rather than the document size.

```go
output := doc.Bytes()
os.WriteFile("config.toml", output, 0644)
```

### Format -- canonical style

`Format()` re-renders the entire document from semantic values with consistent formatting, ignoring all original raw bytes and quoting choices. This is useful for enforcing a canonical style across TOML files, and it supports configurable options for indentation width, line width for array wrapping, and blank lines before table headers.

```go
output := doc.Format()
```

Configure formatting with options:

```go
output := doc.Format(
    tomledit.WithIndentWidth(2),       // indent values under tables
    tomledit.WithLineWidth(100),       // line width before arrays wrap
    tomledit.WithTableBlankLine(true), // blank line before tables
)
```

`Format` does not mutate the document -- it produces a new byte slice.

## Diffing two documents

`Diff` compares two parsed documents by collecting all leaf values from each and producing a list of `Change` values that describe what was added, removed, or modified between them. Each change includes the dot-separated path, the old value, the new value, and a kind enum indicating the type of change.

```go
a, _ := tomledit.Parse([]byte(`name = "Alice"
age = 30
`))
b, _ := tomledit.Parse([]byte(`name = "Bob"
email = "bob@example.com"
`))

changes := tomledit.Diff(a, b)
for _, c := range changes {
    fmt.Printf("%s: %s (old=%v, new=%v)\n",
        c.Kind, c.Path, c.OldValue, c.NewValue)
}
// removed: age (old=30, new=<nil>)
// added: email (old=<nil>, new=bob@example.com)
// modified: name (old=Alice, new=Bob)
```

Change kinds: `tomledit.Added`, `tomledit.Removed`, `tomledit.Modified`.

## Merging documents

### MergeDefaults

Apply default values from a `map[string]any` to an existing document, recursing into nested tables and only setting keys that are missing in the target. Existing values are never overwritten, making this safe for layering configuration defaults underneath user-specified settings without losing any user customizations.

```go
doc, _ := tomledit.Parse([]byte(`[server]
host = "localhost"
`))

err := doc.MergeDefaults("", map[string]any{
    "server": map[string]any{
        "host": "0.0.0.0",   // existing -- kept as "localhost"
        "port": 8080,         // missing -- added
    },
    "logging": map[string]any{
        "level": "info",      // missing -- added
    },
})
```

### Merge

Merge all values from another parsed `*Document` into the current document, using the same recursive semantics as `MergeDefaults` where only missing keys are set and existing values are never overwritten. Comments from the source document are also carried over: leading comments are appended to the target's existing comments, and inline comments fill in where the target has none.

```go
base, _ := tomledit.Parse([]byte(`[server]
host = "localhost"
`))
defaults, _ := tomledit.Parse([]byte(`[server]
host = "0.0.0.0"
port = 8080
`))

err := base.Merge(defaults)
// base.server.host is still "localhost"
// base.server.port is now 8080
```

## Unmarshaling into Go types

### Unmarshal

Parse TOML data and decode it into a Go struct or map in a single call, combining parsing and decoding for convenience. Struct fields are matched by `toml` struct tag first, then by exact field name, then by case-insensitive name, and unknown TOML keys are silently ignored to allow forward-compatible configuration files.

```go
type Config struct {
    Title  string `toml:"title"`
    Server struct {
        Host string `toml:"host"`
        Port int    `toml:"port"`
    } `toml:"server"`
}

var cfg Config
err := tomledit.Unmarshal([]byte(`
title = "My App"

[server]
host = "0.0.0.0"
port = 443
`), &cfg)

fmt.Println(cfg.Title)       // "My App"
fmt.Println(cfg.Server.Host) // "0.0.0.0"
fmt.Println(cfg.Server.Port) // 443
```

Struct fields are matched by `toml` tag, then by exact field name, then by case-insensitive name. Unknown TOML keys are silently ignored.

### Decode

`Decode` operates on an already-parsed `*Document`, decoding its values into a Go struct or map without re-parsing the TOML source. Use this when you need both the AST for comment-preserving edits and the decoded Go values for application logic, avoiding the overhead of parsing the source twice.

```go
doc, err := tomledit.Parse(data)
if err != nil {
    log.Fatal(err)
}

var cfg Config
err = doc.Decode(&cfg)
if err != nil {
    log.Fatal(err)
}

// Use cfg for reading, doc for editing
fmt.Println(cfg.Server.Port)
doc.Set("server.port", cfg.Server.Port + 1)
```

### Unmarshal into maps

You can unmarshal into `map[string]any` for schema-free access when you do not have a predefined struct type, which is useful for generic TOML processing tools, configuration merging, or inspecting documents with unknown schemas. Nested tables become nested maps, and arrays become slices of their element types.

```go
var m map[string]any
err := tomledit.Unmarshal(data, &m)
```

## Marshaling from Go types

`Marshal` encodes a `map[string]any` as formatted TOML bytes, converting top-level keys into key-value pairs and nested maps into `[section]` table headers with alphabetically sorted keys. Only map types are accepted as the root value; struct encoding is not supported.

```go
data, err := tomledit.Marshal(map[string]any{
    "title": "My App",
    "server": map[string]any{
        "host": "localhost",
        "port": 8080,
    },
})
fmt.Print(string(data))
// title = "My App"
//
// [server]
// host = "localhost"
// port = 8080
```

Top-level keys become key-value pairs. Nested maps become `[section]` table headers. Keys are sorted alphabetically.

## Node types

Every value in the AST is a `tomledit.Node`, which provides a uniform interface for accessing the node's type, semantic value, comments, raw bytes, and source span. The concrete types cover all TOML value kinds plus the structural container types for tables, arrays, and array-of-tables headers.

| Node type | Go type | `Value()` returns |
|---|---|---|
| `*StringNode` | string | `string` |
| `*IntegerNode` | integer | `int64` |
| `*FloatNode` | float | `float64` |
| `*BooleanNode` | boolean | `bool` |
| `*DateTimeNode` | offset date-time | `time.Time` |
| `*LocalDateTimeNode` | local date-time | `tomledit.LocalDateTime` |
| `*LocalDateNode` | local date | `tomledit.LocalDate` |
| `*LocalTimeNode` | local time | `tomledit.LocalTime` |
| `*ArrayNode` | array | `[]Node` (Elements) |
| `*InlineTableNode` | inline table | `[]Node` (Children) |
| `*TableNode` | `[table]` header | `[]Node` (Children) |
| `*ArrayTableNode` | `[[array-table]]` header | `[]Node` (Children) |

Use type assertions to access node-specific fields:

```go
node := doc.Get("server.port")
if intNode, ok := node.(*tomledit.IntegerNode); ok {
    fmt.Println(intNode.Val)  // 8080
    fmt.Println(intNode.Base) // tomledit.IntegerDecimal
}
```

## Source positions

Every node from a parsed document carries a `Span` indicating its half-open byte range and line/column position in the original source, which is useful for error reporting, source mapping, and building editor integrations. Programmatically created nodes carry a zero span where `IsValid()` returns false, and edits do not recompute spans on existing nodes.

```go
node := doc.Get("server.host")
span := node.Span()
if span.IsValid() {
    fmt.Printf("line %d col %d to line %d col %d\n",
        span.Start.Line, span.Start.Column,
        span.End.Line, span.End.Column)
}
```

Nodes created programmatically (via `Set`, `Marshal`, etc.) have a zero span where `IsValid()` returns `false`. Edits do not recompute spans on existing nodes.

## Complete example

This example demonstrates a typical workflow: parse a TOML configuration file from disk, update existing values, remove a key, add a new section with auto-created intermediate tables, attach a comment, rename a key, and write the modified document back while preserving all original comments and formatting in untouched regions.

```go
package main

import (
    "log"
    "os"

    "github.com/smm-h/go-toml-edit"
)

func main() {
    data, err := os.ReadFile("config.toml")
    if err != nil {
        log.Fatal(err)
    }

    doc, err := tomledit.Parse(data)
    if err != nil {
        log.Fatal(err)
    }

    // Update existing values
    doc.Set("server.port", 9090)
    doc.Set("server.host", "0.0.0.0")

    // Remove a key
    doc.Delete("server.debug")

    // Add a new section with auto-created intermediate tables
    doc.SetCreate("database.host", "db.example.com")
    doc.SetCreate("database.port", 5432)
    doc.SetCreate("database.name", "myapp")

    // Add a comment
    doc.SetComment("database.host", "primary database server")

    // Rename a key
    doc.RenameKey("server.host", "bind_address")

    // Write back, preserving all original comments and formatting
    err = os.WriteFile("config.toml", doc.Bytes(), 0644)
    if err != nil {
        log.Fatal(err)
    }
}
```
