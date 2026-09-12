# Decode drops an array of tables nested inside another array of tables

## Context

A consumer parsing a curated listing document with `Parse` and then reading the
value tree through the v0.3.0 document decoder found the nested array missing.
The consumer worked around it by walking the AST itself (`TableNode`,
`ArrayTableNode.KeyPath`, `KeyValueNode`) to build its value tree, so it no
longer exercises the decoder. The defect itself is unfixed.

## Problem

Given

```toml
[[category]]
name = "Tools"

[[category.project]]
slug = "a"

[[category.project]]
slug = "b"

[[category]]
name = "Libraries"

[[category.project]]
slug = "c"
```

the v0.3.0 decoder returns `category` as `[{name: "Tools"}, {name: "Libraries"}]`
with every `project` array gone. Nothing is reported: the data is silently
lost. Whether v0.4.0's rewritten `Unmarshal`/`Record` layer has the same
behavior has not been checked.

## Solutions

1. Fix the decoder so an `[[a.b]]` header attaches to the most recent `[[a]]`
   element, as the TOML specification requires, with a red-green test over the
   document above and a deeper nesting (`[[a.b.c]]`). Recommended.
2. Refuse the shape with an error until it is supported. Better than silent
   loss, worse than the fix.

## Affected files

The document decoder and its tests; a conformance case in the canonical-form
suite if one exists for nested arrays of tables.

## Effort

Small: one attachment rule in the decoder plus tests.
