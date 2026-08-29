# go-toml-edit

Comment-preserving TOML editing library for Go. The Go equivalent of Python's tomlkit.

> Split from the founding document (todo/original-idea.md). This part records the
> scope that was built, verified against the code on disk. The unbuilt remainder
> (error-handling completion, publicity, v2 roadmap) stays active in
> todo/original-idea-remaining.md.

## Origin

There is no production-ready Go library for editing TOML files while preserving comments, whitespace, and formatting:

| Library | Status |
|---------|--------|
| pelletier/go-toml/v2 | Explicitly removed document editing from scope. `unstable` sub-package gives read-only AST with byte ranges and comment nodes, but no write-back API. |
| BurntSushi/toml | Discards comments entirely. Standard Marshal/Unmarshal only. |
| akiyosi/tomlwriter | Claims comment-preserving writes. 0 stars, last touched 2018, unmaintained. |

Python solved this years ago with tomlkit (maintained by python-poetry): full comment/whitespace/formatting preservation with a dict-like API for surgical edits. It's the gold standard. Go has nothing comparable. Any Go tool that needs to modify a user's TOML config without destroying their comments and formatting has no good option today. go-toml-edit fills this gap.

## Project Identity

| Property | Value |
|----------|-------|
| Name | go-toml-edit |
| Repository | github.com/smm-h/go-toml-edit |
| Module path | github.com/smm-h/go-toml-edit |
| Type | Pure Go library (no CLI, no binary) |
| TOML spec | 1.0 only |
| Go version | 1.21+ |
| Dependencies | Minimal (stdlib-heavy, small battle-tested deps allowed) |
| License | TBD |

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Project type | Pure library | The ecosystem gap is a library, not a tool. |
| TOML spec | 1.0 only | Stable spec. Covers all production use cases. 1.1 is still draft. |
| Core model | Hybrid AST with byte ranges | AST nodes carry byte offsets. Edit the AST, re-render only changed subtrees while splicing unchanged bytes from original. Best preservation fidelity. Similar spirit to tomlkit's Trivia model but operates at the byte level rather than token level. |
| Codec scope (v1) | Edit + Unmarshal | Ship the differentiator (editing) first. Marshal deferred to v2. |
| Struct tags | Full support | `toml:"name"`, `toml:",omitempty"`, `toml:"-"`. Matches BurntSushi/toml and encoding/json conventions. |
| Formatting | Preserve + Format() | Preserve existing formatting on round-trip. Also offer Format() for whole-doc or subtree reformatting. |
| Parser API | Internal only | Parser is implementation detail. Only the Document API is public. |
| Format scope | TOML only | No JSONC, no YAML. |
| Streaming/events | No | Document-oriented API only. |

## Architecture

### Core model: hybrid AST with byte ranges

Most of a TOML file doesn't change during an edit. The hybrid model exploits this:

1. Parse TOML source into an AST where every node records its byte range (start, end) in the original source.
2. Trivia (comments, whitespace, blank lines) is attached to adjacent nodes, not discarded.
3. When a node is modified via the API, it's marked dirty.
4. On serialization: dirty nodes are re-rendered; clean nodes are spliced directly from the original byte slice. Guarantees zero-diff round-trips for untouched regions.

### Node types

| Node type | TOML construct | Example |
|-----------|---------------|---------|
| Document | Root container | (the whole file) |
| Table | Standard table | `[server]` |
| ArrayTable | Array of tables | `[[products]]` |
| KeyValue | Key-value pair | `name = "Tom"` |
| Key | Bare or quoted key | `name`, `"name"`, `'name'` |
| DottedKey | Dotted key | `server.host` |
| String | Basic or literal string | `"hello"`, `'hello'` |
| MultilineString | Multi-line strings | `"""..."""`, `'''...'''` |
| Integer | Integer value | `42`, `0xff`, `0o77`, `0b1010` |
| Float | Float value | `3.14`, `inf`, `nan` |
| Boolean | Boolean value | `true`, `false` |
| DateTime | Offset date-time | `1979-05-27T07:32:00Z` |
| LocalDateTime | Local date-time | `1979-05-27T07:32:00` |
| LocalDate | Local date | `1979-05-27` |
| LocalTime | Local time | `07:32:00` |
| Array | Array value | `[1, 2, 3]` |
| InlineTable | Inline table | `{name = "Tom", age = 30}` |

### Trivia model

Every node has associated trivia:

- Leading whitespace (spaces, tabs before the node)
- Leading comments (full-line comments above the node)
- Inline comment (comment after the value on the same line)
- Trailing whitespace/newlines

Trivia is stored as raw byte slices referencing the original source. On re-render of a dirty node, default trivia formatting is applied (configurable).

## Public API (v1)

### Document operations

```go
func Parse(source []byte) (*Document, error)
func (d *Document) Bytes() []byte
func (d *Document) Format() []byte
```

### Reading values

```go
func (d *Document) Get(path string) Node
func (d *Document) GetString(path string) (string, bool)
func (d *Document) GetInt(path string) (int64, bool)
func (d *Document) GetBool(path string) (bool, bool)
func (d *Document) GetFloat(path string) (float64, bool)
func (d *Document) GetTime(path string) (time.Time, bool)
```

### Editing values

```go
func (d *Document) Set(path string, value any) error
func (d *Document) Delete(path string) error
func (d *Document) Rename(from, to string) error
```

### Table/array operations

```go
func (d *Document) NewTable(path string) error
func (d *Document) NewArrayTable(path string) error
```

### Unmarshal

```go
func Unmarshal(data []byte, v any) error
func (d *Document) Decode(v any) error
```

### Dot-path addressing

- `server.host` -- key `host` in table `[server]`
- `products.0.name` -- first element of `[[products]]`, key `name`
- Backslash escapes literal dots: `server.host\.name` -- key `host.name` in table `[server]`

### Node interface

```go
type Node interface {
    Type() NodeType
    Value() any
    Comment() string
    SetComment(comment string)
    LeadingComments() []string
    SetLeadingComments(comments []string)
    Raw() []byte
}
```

## TOML 1.0 Feature Coverage

| Feature | Notes |
|---------|-------|
| Bare keys | Alphanumeric + `-` + `_` |
| Quoted keys | Basic (`"..."`) and literal (`'...'`) |
| Dotted keys | `a.b.c = value` |
| Basic strings | Escape sequences: `\t`, `\n`, `\r`, `\\`, `\"`, `\uXXXX`, `\UXXXXXXXX` |
| Literal strings | No escaping: `'...'` |
| Multi-line basic strings | `"""..."""` with line-ending backslash trimming |
| Multi-line literal strings | `'''...'''` |
| Integers | Decimal, hex (`0x`), octal (`0o`), binary (`0b`), underscores |
| Floats | Decimal, exponent, `inf`, `nan`, underscores |
| Booleans | `true`, `false` |
| Offset date-time | RFC 3339 |
| Local date-time | No timezone |
| Local date | Date only |
| Local time | Time only |
| Arrays | Homogeneous and heterogeneous, trailing commas, multi-line |
| Inline tables | Single-line, no trailing commas (TOML 1.0) |
| Standard tables | `[table]`, `[a.b.c]` |
| Array of tables | `[[array]]` |
| Comments | `# ...` (full-line and inline) |
| Super tables | Implicit parent tables from dotted definitions |

## Unmarshal Type Mapping

| TOML type | Go type(s) |
|-----------|-----------|
| String | `string` |
| Integer | `int`, `int8`, `int16`, `int32`, `int64`, `uint`, `uint8`, `uint16`, `uint32`, `uint64` |
| Float | `float32`, `float64` |
| Boolean | `bool` |
| Offset date-time | `time.Time` |
| Local date-time | custom `LocalDateTime` type |
| Local date | custom `LocalDate` type |
| Local time | custom `LocalTime` type |
| Array | slice, array |
| Table | struct, `map[string]any`, `map[string]T` |
| Inline table | same as Table |

## Struct Tag Specification

Format: `toml:"key,options"`

| Tag | Behavior |
|-----|----------|
| `toml:"name"` | Use "name" as the TOML key |
| `toml:"-"` | Skip this field entirely |
| `toml:",omitempty"` | Reserved for v2 Marshal |
| No tag | Use field name, exact case match first, then case-insensitive |
| Embedded struct | Promote fields (like encoding/json) |
| `any` / `interface{}` | Decode to `map[string]any`, `[]any`, `string`, `int64`, `float64`, `bool`, `time.Time` |

## Formatter

The `Format()` method normalizes formatting while preserving all comments:

- Consistent key-value spacing: `key = value`
- One blank line between tables
- No trailing whitespace
- Multi-line arrays: one element per line if array exceeds configurable line width
- Comments stay attached to their associated nodes
- Configurable via `FormatConfig` struct (indentation, line width, etc.)

## Testing Strategy

- Table-driven tests for all TOML 1.0 features
- Round-trip tests: parse and serialize every test case, assert byte-identical output
- Edit tests: parse, modify specific paths, assert only the modified region changed
- Official toml-test suite (https://github.com/toml-lang/toml-test) for spec compliance
- Unmarshal tests: all type mappings, struct tag combinations, edge cases
- Fuzz testing for the parser (Go's built-in fuzzing)
- Benchmarks against pelletier/go-toml/v2 for parse and unmarshal performance

## Effort Estimate

| Component | Estimated LOC | Notes |
|-----------|--------------|-------|
| Lexer/tokenizer | 400-600 | Token types, string/number/date parsing, comment extraction |
| Parser (tokens to AST) | 500-800 | All TOML 1.0 constructs, byte range tracking, trivia attachment |
| AST node types + trivia | 300-500 | Node interfaces, concrete types, trivia storage |
| Document API (Get/Set/Delete/Rename) | 400-600 | Dot-path resolution, dirty tracking, intermediate table creation |
| Serializer (AST to bytes) | 300-500 | Dirty node rendering, clean region splicing, offset management |
| Formatter | 200-400 | Pretty-printing, configurable style |
| Unmarshal (reflection-based) | 400-600 | Struct tags, type mapping, embedded structs, error reporting |
| Tests | 2000-3000 | Table-driven, round-trip, edit, unmarshal, fuzz |
| **Total** | **~4500-7000** | Source + tests |

## Distribution and Publicity

### Publishing

- Develop under github.com/smm-h/go-toml-edit
- Release management via rlsbl (`rlsbl release`)
- Cross-platform binaries not needed (pure library), but tagged releases enable `go install` and module proxy caching

## Standalone Identity

go-toml-edit is a general-purpose TOML editing library. Its API is designed for any Go program that needs to read, modify, and write back TOML files without losing user formatting. Not tied to any specific tool or workflow.

tomlkit proved the demand for this class of library -- Poetry, PDM, Hatch, and dozens of Python tools depend on it. The Go ecosystem has the same need (Hugo, CockroachDB, Consul, any Go CLI with TOML config) but no solution. go-toml-edit is that solution.
