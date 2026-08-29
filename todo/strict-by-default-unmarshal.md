# Strict-by-default Unmarshal

## Context

`Unmarshal` currently ignores unknown TOML keys silently (documented in
unmarshal.go: "Unknown TOML keys are silently ignored"). For configuration
files this is a correctness hazard: a typo'd or misplaced key simply
vanishes, and the caller has no way to detect it. Wrong-typed values for
known keys are likewise dropped without error in common decode shapes.

## Problem

There is no strict mode at all — callers cannot opt into unknown-key
rejection, and per the fleet's design philosophy (hard errors over silent
tolerance; no escape hatches), strictness should not be an opt-in flag but
the default behavior.

## Proposed work

- Make `Unmarshal` strict by default: an unknown key in the document that
  maps to no field of the target struct is a hard error naming the key and
  its path; a value whose TOML type cannot decode into the target field's
  type is likewise a hard error.
- This is a deliberate BREAKING change and ships as its own release. Every
  consumer must be swept and fixed in the same move (fix the caller, never
  downgrade): audit each call site for structs that intentionally decode a
  subset of a document and give them explicit handling (decode into a
  complete struct, or restructure the document) rather than a leniency
  flag.
- No lenient mode is added. If a genuinely partial decode is ever needed,
  that caller should extract the subtable first and decode it completely.

## Affected surface

- unmarshal.go (the reflection walk), its tests, the README's Unmarshal
  documentation, and the changelog (breaking entry).
- Consumers across the fleet (sweep at release time).

## Effort

Small-to-medium: the library change and tests are roughly a day; the
consumer sweep is the variable part and must ride the same release.
