# Canonical-form conformance

> Split from todo/canonical-form-conformance-and-roundtrip-tests.md; the
> round-trip half is done and moved to todo/.done/roundtrip-corpus-tests.md.
> This half stays active, blocked on the canonical-form specification's
> publication. Text below preserved verbatim.

## Context

The ecosystem has adopted a TOML usage profile (pinned to TOML 1.0 — this
project's existing 1.0-strict stance is now official policy, not just current
state) that includes a **written canonical formatting specification**,
authored and maintained in the ecosystem's schema toolchain. The spec is the
authority; formatters conform to it. This project's formatter is the Go-side
implementation of that canonical form.

## Problem (conformance half)

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

## Affected files

- `format.go`, `FormatConfig` and options (conformance audit/alignment)

## Effort

Conformance: small-to-medium depending on how far current formatter behavior
sits from the spec; unknowable until the spec lands.
