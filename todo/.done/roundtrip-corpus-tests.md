# Round-trip corpus tests

> Split from todo/canonical-form-conformance-and-roundtrip-tests.md; this part
> is done (the round-trip battery over the valid toml-test corpus exists, with
> the corpus counts asserted). The conformance half stays active in
> todo/canonical-form-conformance.md. Text below preserved verbatim.

## Context

The ecosystem has adopted a TOML usage profile (pinned to TOML 1.0 — this
project's existing 1.0-strict stance is now official policy, not just current
state) that includes a **written canonical formatting specification**,
authored and maintained in the ecosystem's schema toolchain. The spec is the
authority; formatters conform to it. This project's formatter is the Go-side
implementation of that canonical form.

This project already runs the official toml-test suite pinned to the 1.0
corpus (1.1 tests skipped via `testsToSkip_1_0`), and empirically rejects
1.1-only syntax (verified: multi-line inline tables, `\x` escapes, secondless
times all produce clean errors). That stance stays.

## Problem (round-trip half)

2. **Round-trip tests over the toml-test valid corpus.** toml-test validates
   decoding; this project's product is editing. Add a test that feeds every
   toml-test *valid* file (1.0 corpus) through parse → no-op edit → serialize
   and asserts byte-identical output. This covers the comment/format
   preservation guarantee that the decoder-oriented suite cannot. Also pin
   the toml-test corpus version explicitly so corpus updates are deliberate.

## Affected files

- `tomltest_test.go` (round-trip mode over valid corpus, corpus version pin)
- `go.mod` (corpus version pinning)

## Effort

Round-trip tests: small (the suite integration already exists; add the
no-op-edit assertion path).
