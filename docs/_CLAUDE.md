---
title: CLAUDE.md
---
# go-toml-edit

Comment-preserving TOML parser and editor library for Go. Single `tomledit` package, no CLI.

## Architecture

Hybrid AST with byte ranges. Every node records its span in the original source. Trivia (comments, whitespace, blank lines) is attached to nodes rather than discarded. On serialization, clean (unmodified) nodes splice original bytes directly; dirty (edited) nodes re-render from semantic values. This gives zero-diff round-trips for untouched regions.

### Parse-edit-render pipeline

1. Lexer tokenizes source, producing tokens with byte offsets
2. Parser builds AST from tokens, attaching trivia (comments, whitespace) to the nearest node
3. Each node stores its byte range `[start, end)` in the original source
4. Edit operations (Set, Delete, Rename) mark affected nodes as dirty
5. Render: clean nodes copy `src[start:end]`; dirty nodes serialize from their semantic value

### Source files

:-: list-modules path="."

### Key design decisions

- Single package: everything is in `tomledit`, no internal/ subpackages
- Virtual node types (dottedKeyView, compoundTableView, arrayTableCollection) handle TOML's complex table semantics without exposing them in the public API
- Path syntax supports: `server.host`, `array[0]`, `array[-1]`, `"key.with.dots"`
- Node interface unifies all AST types: Type(), Value(), Comment(), LeadingComments(), Raw(), Span()
- Spans reflect the most recent Parse: edits never recompute them; programmatically created nodes carry the zero (invalid) Span
- Dirty tracking is per-node, not per-document, so only edited subtrees re-render

## Build and test

```sh
go test ./...           # all tests including toml-test compliance
go test -bench .        # benchmarks against BurntSushi/toml
go test -fuzz FuzzParse # fuzz the parser
```

## Testing strategy

- Audit test files (`*_audit_test.go`) cover parser, serializer, comments, edits, diff, merge, unmarshal, walk, format, cursor
- TOML 1.0 compliance: the full valid and invalid corpora from the official toml-test suite, with the 1.1-only cases skipped (the skip set is derived from toml-test's own version-filtered listing, and the case counts are asserted in the test)
- Round-trip fidelity: `Parse(x).Bytes()` must equal `x` for any valid TOML
- Fuzz testing for parser robustness

## Release workflow

This project uses [rlsbl](https://github.com/smm-h/rlsbl) for release orchestration.

- Run `rlsbl release [patch|minor|major]` to bump version and create a GitHub Release
- Go library -- no publish step. Tagged releases are available via `go get`.
- Use `rlsbl release --dry-run` to preview without making changes
- Never publish manually -- always use `rlsbl release`

## Conventions

- Go library only -- no CLI, no binary
- Full TOML 1.0 compliance is non-negotiable: the toml-test suite must pass
- Comments are first-class: any operation that silently loses comments is a bug
- Round-trip fidelity: `Parse(x).Bytes()` must equal `x` for any valid TOML input
- BurntSushi/toml is a dev dependency for benchmarks only, not a runtime dependency
- toml-test/v2 is a test dependency for compliance verification only
