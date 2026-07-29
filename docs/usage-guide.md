---
title: Usage Guide
description: "Comprehensive guide to parsing, querying, editing, and serializing TOML documents with go-toml-edit."
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

The primary entry point is `tomledit.Parse`, which takes a `[]byte` and returns a `*DocumentNode` AST:

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

Read the file into memory first, then parse:

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

The `Get` method resolves a dot-separated path and returns the value node. It returns `nil` if the path is not found or syntactically invalid.

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

Convenience methods extract values with type checking. Each returns a value and a boolean indicating success:

```go
host, ok := doc.GetString("server.host")     // (string, bool)
port, ok := doc.GetInt("server.port")         // (int64, bool)
debug, ok := doc.GetBool("server.debug")      // (bool, bool)
rate, ok := doc.GetFloat("server.rate")        // (float64, bool)
ts, ok := doc.GetTime("server.started_at")     // (time.Time, bool)
```

If the path does not exist or the value is a different type, the zero value and `false` are returned.

### Resolve with error details

`Resolve` works like `Get` but returns an error instead of `nil`, making it possible to distinguish "not found" from "invalid path":

```go
node, err := doc.Resolve("server.host")
if err != nil {
    fmt.Println("resolution failed:", err)
}
```

### Cursor API

The cursor provides a fluent, nil-safe API for step-by-step navigation. Errors are deferred until you check `Err()`:

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

The `Items` method returns a range-over-func iterator for array elements and array-of-tables entries:

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

`Set` updates the value at a path. If the final key does not exist in an existing parent, it is created. Intermediate path segments must already exist.

```go
err := doc.Set("server.port", 9090)
err = doc.Set("server.host", "0.0.0.0")
err = doc.Set("server.debug", true)
err = doc.Set("server.rate", 0.75)
```

Supported Go types: `string`, `bool`, `int`/`int8`-`int64`, `uint`/`uint8`-`uint64`, `float32`/`float64`, `time.Time`, `tomledit.LocalDateTime`, `tomledit.LocalDate`, `tomledit.LocalTime`, `[]any`, `map[string]any`, and any value implementing the `tomledit.Node` interface.

### SetCreate

`SetCreate` is like `Set` but auto-creates intermediate `[table]` headers when they do not exist:

```go
// Even if [database] does not exist yet, this creates it automatically.
err := doc.SetCreate("database.connection.host", "db.example.com")
```

### Setting arrays and inline tables

Pass a `[]any` for arrays or `map[string]any` for inline tables:

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

Create a new `[table]` header:

```go
err := doc.NewTable("logging")
// Then populate it:
err = doc.Set("logging.level", "info")
err = doc.Set("logging.file", "/var/log/app.log")
```

Returns an error if a table with that path already exists.

### NewArrayTable

Append a new `[[array-table]]` entry. Multiple entries with the same path represent successive elements of the array:

```go
err := doc.NewArrayTable("products")
err = doc.Set("products[-1].name", "Widget")
err = doc.Set("products[-1].price", 9.99)

err = doc.NewArrayTable("products")
err = doc.Set("products[-1].name", "Gadget")
err = doc.Set("products[-1].price", 19.99)
```

## Removing keys

`Delete` removes the node at a path. It is safe to call on paths that do not exist (returns `nil`):

```go
err := doc.Delete("server.debug")    // remove a key-value pair
err = doc.Delete("logging")          // remove an entire table
err = doc.Delete("products[0]")      // remove an array element
err = doc.Delete("nonexistent.key")  // no-op, no error
```

## Renaming keys

`Rename` changes the key name at a path. Returns an error if the target name already exists or the path is not found:

```go
err := doc.Rename("server.host", "bind_address")
// server.host is now server.bind_address
```

## Walking the document

`Walk` visits every key-value pair in document order, calling a function with the path and value node:

```go
err := doc.Walk(func(path string, node tomledit.Node) error {
    fmt.Printf("%s = %v (%s)\n", path, node.Value(), node.Type())
    return nil
}, tomledit.WalkLeaves)
```

Two walk modes are available:

- `tomledit.WalkLeaves` -- visits only scalar (leaf) values; containers (arrays, inline tables) are recursed into but not yielded to the callback
- `tomledit.WalkAll` -- visits containers AND their children; the callback receives inline tables and arrays as well as their elements

Return `tomledit.SkipTable` from the callback to skip the children of the current inline table or array.

Array-of-tables paths include the index: `"products[0].name"`, `"products[1].name"`.

## Comments

go-toml-edit preserves all comments from the original document. You can also read and write comments programmatically.

### Reading comments

Every node exposes `Comment()` for the inline comment and `LeadingComments()` for comment lines above it:

```go
node := doc.Get("server.host")
if node != nil {
    fmt.Println("inline:", node.Comment())
    fmt.Println("leading:", node.LeadingComments())
}
```

### Setting inline comments

`SetComment` sets the inline comment on a node. The `"# "` prefix is added automatically:

```go
err := doc.SetComment("server.port", "default HTTP port")
// Produces: port = 8080 # default HTTP port
```

Pass an empty string to remove the comment:

```go
err := doc.SetComment("server.port", "")
```

### Setting leading comments

`SetLeadingComments` sets the comment lines above a node. Each string should be the bare comment text without `"# "` prefix:

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

TOML does not allow comments inside inline tables. Attempting to set a comment on a member of an inline table returns an error.

## Comment preservation guarantee

The library guarantees that parsing a document and serializing it back produces the exact original bytes, with no changes to comments, whitespace, or formatting:

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

`Bytes()` returns the document as TOML bytes. Unmodified nodes emit their original raw bytes; only dirty (edited) nodes are re-rendered:

```go
output := doc.Bytes()
os.WriteFile("config.toml", output, 0644)
```

### Format -- canonical style

`Format()` re-renders the entire document with consistent formatting, ignoring the original raw bytes. This is useful for enforcing a canonical style:

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

`Diff` compares two documents and returns a list of changes:

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

Apply default values to an existing document. Only missing keys are set; existing values are never overwritten:

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

Merge all values from another parsed document. Same recursive semantics as `MergeDefaults`: only missing keys are set. Comments from the source document are also carried over:

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

Parse TOML data and decode it into a Go struct or map in one step:

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

`Decode` operates on an already-parsed `*DocumentNode`. Use this when you need both the AST (for editing) and the decoded values:

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

You can unmarshal into `map[string]any` for schema-free access:

```go
var m map[string]any
err := tomledit.Unmarshal(data, &m)
```

## Marshaling from Go types

`Marshal` encodes a `map[string]any` as formatted TOML bytes:

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

Every value in the AST is a `tomledit.Node`. The concrete types are:

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

Every node from a parsed document carries a `Span` indicating its position in the source:

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

Parse a configuration file, update several values, add a new section, and write it back:

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
    doc.Rename("server.host", "bind_address")

    // Write back, preserving all original comments and formatting
    err = os.WriteFile("config.toml", doc.Bytes(), 0644)
    if err != nil {
        log.Fatal(err)
    }
}
```
