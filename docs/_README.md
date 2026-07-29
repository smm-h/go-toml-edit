---
title: README.md
---
# go-toml-edit

Comment-preserving TOML editing for Go.

## Why

Every Go TOML library either discards comments during parsing or provides
read-only access to them. If you parse a config file, change one value, and
write it back, the comments are gone. Python has
[tomlkit](https://github.com/sdispater/tomlkit); Go had nothing.
go-toml-edit fills this gap with a lossless AST that preserves every comment,
blank line, and formatting detail through arbitrary edits.

## Before / After

```go
package main

import (
	"fmt"

	"github.com/smm-h/go-toml-edit"
)

func main() {
	doc, _ := tomledit.Parse([]byte(`# Server config
[server]
host = "localhost"  # primary host
port = 8080
`))

	doc.Set("server.port", 9090)
	fmt.Print(string(doc.Bytes()))
}
```

Output -- comments survive the edit:

```toml
# Server config
[server]
host = "localhost"  # primary host
port = 9090
```

## Installation

```
go get github.com/smm-h/go-toml-edit
```

## Feature Comparison

| Feature | go-toml-edit | BurntSushi/toml | pelletier/go-toml/v2 |
|---------|-------------|-----------------|---------------------|
| Comment preservation | Yes | No | Read-only (unstable) |
| Round-trip editing | Yes | No | No |
| Set/Delete/Rename API | Yes | No | No |
| Unmarshal to struct | Yes | Yes | Yes |
| Marshal from Go values | Maps only (structs in v2) | Yes | Yes |
| Position spans on AST nodes | Yes | No | No |
| TOML 1.0 compliance | Full | Full | Full |
| Formatter | Yes | No | No |
| Document diffing | Yes | No | No |
| Deep merge | Yes | No | No |

## Quick Start

### Parse and Read

```go
doc, _ := tomledit.Parse([]byte(`[server]
host = "localhost"
port = 8080
`))
fmt.Println(doc.GetString("server.host")) // "localhost", true
fmt.Println(doc.GetInt("server.port"))    // 8080, true
```

### Edit Values

```go
doc.Set("server.port", 9090)              // update existing key
doc.SetCreate("database.host", "db.local") // create key + intermediate table
doc.Delete("server.debug")                 // remove a key
doc.Rename("server.host", "address")       // rename a key
```

### Comment Management

```go
doc.SetComment("server.port", "default: 8080")
doc.SetLeadingComments("server", []string{
	"Server configuration",
	"See docs for all options",
})
```

### Fluent Cursor

```go
host, ok := doc.Key("database").Key("host").String()
port, ok := doc.Key("database").Key("port").Int()
```

### Unmarshal

```go
type Config struct {
	Title  string `toml:"title"`
	Server struct {
		Host string `toml:"host"`
		Port int    `toml:"port"`
	} `toml:"server"`
}
var cfg Config
tomledit.Unmarshal(data, &cfg)
```

### Format

```go
formatted := doc.Format(tomledit.WithIndentWidth(2))
```

### Walk

```go
doc.Walk(func(path string, node tomledit.Node) error {
	fmt.Printf("%s = %v\n", path, node.Value())
	return nil
}, tomledit.WalkLeaves)
```

### Position Spans

Every AST node carries its source position from the most recent parse,
enabling custom semantic validation (unknown keys, wrong types, out-of-range
values) with precise line/column diagnostics:

```go
doc.Walk(func(path string, node tomledit.Node) error {
	sp := node.Span() // 1-based line/column, byte columns
	fmt.Printf("%s at %d:%d-%d:%d\n", path,
		sp.Start.Line, sp.Start.Column, sp.End.Line, sp.End.Column)
	return nil
}, tomledit.WalkLeaves)
```

`Span()` is available on all nodes: tables and array-tables (covering only
the `[header]`), key-value pairs (key through value), keys, all scalars,
arrays, inline tables, and comments. `Span.End` is exclusive -- the position
immediately after the node's last byte.

**Span policy:** spans reflect the last `Parse`. Edit operations do not
recompute them: nodes created programmatically (via `Set`, `Merge`,
`Marshal`, ...) return the zero span (`Span.IsValid()` reports false), and
edited nodes keep their parse-time spans. Re-parse the output of `Bytes()`
when fresh positions are needed after editing.

### Array Iteration

```go
for i, item := range doc.Items("servers") {
	fmt.Printf("[%d] %v\n", i, item.Value())
}
fmt.Println(doc.Len("servers"))
```

### Diff

```go
changes := tomledit.Diff(a, b)
for _, c := range changes {
	fmt.Printf("%s: %s\n", c.Kind, c.Path)
}
// removed: age
// added: email
// modified: name
```

### Merge

```go
base, _ := tomledit.Parse([]byte(`[server]
host = "localhost"
`))
defaults, _ := tomledit.Parse([]byte(`[server]
host = "0.0.0.0"
port = 8080
`))
base.Merge(defaults)
// host keeps "localhost" (already set); port added from defaults.
```

## Performance

go-toml-edit retains the full AST for comment-preserving round-trip editing, so
it allocates more memory than decode-only libraries. Parse speed is competitive.

Benchmarks on a 13th-gen Intel i7-13620H (`go test -bench .`):

| Benchmark | go-toml-edit | BurntSushi/toml |
|-----------|-------------|-----------------|
| Parse (small) | 15.7 us, 30.8 KB/op | 17.9 us, 15.4 KB/op |
| Unmarshal (small) | 23.9 us, 37.1 KB/op | 21.8 us, 17.5 KB/op |
| Parse (large) | 274 us, 595 KB/op | 246 us, 171 KB/op |

Higher memory usage reflects the full AST required for comment-preserving
round-trip editing. A decode-only library can discard source positions, trivia
tokens, and comment nodes that go-toml-edit must retain.

## API Reference

[pkg.go.dev/github.com/smm-h/go-toml-edit](https://pkg.go.dev/github.com/smm-h/go-toml-edit)

## Roadmap

Planned for v2: Marshal (struct to TOML), TOML 1.1, streaming parser.
See `todo/` for details.

## License

[MIT](LICENSE)
