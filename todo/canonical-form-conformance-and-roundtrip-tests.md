# Canonical-form conformance + round-trip corpus tests

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

## Problem

Two gaps:

1. **Formatter conformance to the canonical-form spec.** Once the written
   spec exists (table style policy, key ordering, datetime precision —
   always write seconds, string style, indentation), audit `format.go` /
   `FormatConfig` against it and align. The spec may constrain options that
   are currently configurable — canonical form means one output, so
   configurability that contradicts the spec should be removed rather than
   defaulted (no implicit defaults; agents must not be able to produce
   non-canonical output by picking options). Blocked on the spec's
   publication; the audit can be prepared by inventorying current formatter
   behavior.
2. **Round-trip tests over the toml-test valid corpus.** toml-test validates
   decoding; this project's product is editing. Add a test that feeds every
   toml-test *valid* file (1.0 corpus) through parse → no-op edit → serialize
   and asserts byte-identical output. This covers the comment/format
   preservation guarantee that the decoder-oriented suite cannot. Also pin
   the toml-test corpus version explicitly so corpus updates are deliberate.

## Solutions

**Option A — both, with round-trip tests first.** Round-trip tests are
unblocked today and harden the core guarantee; conformance follows the spec.
Pros: immediate value; no idle waiting on the spec. Cons: none significant.

**Option B — wait and do both together.** Pros: single test-infrastructure
pass. Cons: leaves the round-trip gap open for no reason.

## Affected files

- `tomltest_test.go` (round-trip mode over valid corpus, corpus version pin)
- `format.go`, `FormatConfig` and options (conformance audit/alignment)
- `go.mod` (corpus version pinning)

## Effort

Round-trip tests: small (the suite integration already exists; add the
no-op-edit assertion path). Conformance: small-to-medium depending on how far
current formatter behavior sits from the spec; unknowable until the spec
lands.
