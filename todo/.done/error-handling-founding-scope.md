# Error handling: remaining scope from the founding document

> Split from todo/original-idea-remaining.md; this section is done — parse
> errors carry line/column, byte offset, snippet and expected/got; edit errors
> are typed diagnostics; Delete on a missing path is the documented silent
> no-op; Set rejects Go types with no TOML representation. The publicity and
> v2-roadmap sections stay active in todo/publicity-and-v2-roadmap.md. Text
> below preserved verbatim.

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
