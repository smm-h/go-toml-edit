# go-toml-edit: remaining scope from the founding document

> Split from the founding document (todo/original-idea.md); the built scope was
> verified against the code on disk and moved to todo/.done/original-idea.md.
> The sections below, preserved verbatim, describe work not yet done: the
> error-handling promises are only partially implemented (typed edit errors do
> not exist; parser-stage errors carry line/column but no byte offset or
> snippet), and the publicity plan and v2 roadmap were never executed.

## Error Handling

Parse errors include:
- Line and column number
- Byte offset
- Snippet of offending source (a few chars of context)
- Clear description of expected vs found

Edit errors:
- `Set` with incompatible path (e.g., setting child of a scalar) returns a typed error
- `Delete` on non-existent path is a silent no-op
- Type validation on `Set`: reject Go types with no TOML representation

## Distribution and Publicity

### Publicity

- Submit to awesome-go (https://github.com/avelino/awesome-go) -- the most visible curated Go library list
- Post announcement to r/golang with a clear problem statement and comparison to existing libraries
- Submit to Go Weekly newsletter (https://golangweekly.com/) -- reaches ~30k Go developers
- Write a blog post framing the problem: "Why can't Go edit TOML files without destroying comments?" -- establish the gap, reference tomlkit as proof it's solvable, show go-toml-edit as the Go answer, include before/after examples
- Post to Hacker News (Show HN) -- TOML tooling and comment preservation are topics that resonate with the HN audience
- Open an issue or discussion on pelletier/go-toml linking go-toml-edit as the editing solution they explicitly declined to build
- Submit to TOML's wiki/ecosystem page (https://github.com/toml-lang/toml/wiki) if a Go section exists
- Tag relevant Go influencers / TOML maintainers on the announcement

## v2 Roadmap

- Marshal: struct-to-TOML serialization with full struct tag support and comment generation
- TOML 1.1 support when the spec stabilizes
- Public streaming/event parser for power users (linters, formatters, custom tooling)
- Performance optimizations (lazy parsing for large files)
