# Parse and allocate at the speed of a decode-only library

## Context

The README's performance section states that a decode-only library parses
faster and allocates less, because this library keeps the full syntax tree
(source positions, trivia, comment nodes, the lexeme of every value) that
comment-preserving round-trip editing needs. The benchmark suite compares
against BurntSushi/toml to make that trade-off visible, and the public
description deliberately makes no speed claim.

## Problem

Being slower than the decode-only libraries is accepted today as the price
of the retained tree. It does not have to be: the tree can be retained
without the allocation pattern that makes it slow, and a library that is
both lossless and as fast as the decode-only ones removes the reason to
reach for anything else.

## Solutions

### Arena-allocate the tree and index lazily

Allocate nodes from one slab per document instead of one heap object each,
store trivia as offsets into the source rather than copied strings, and
build the key index on first lookup rather than at parse time.

- Pros: attacks the allocation count directly (hundreds of allocations per
  small document today), keeps the public API.
- Cons: a substantial rewrite of the parser's data structures.

### Two-phase parse

Parse to a compact token stream first and materialise nodes only for the
tables an edit touches, re-emitting untouched spans byte-for-byte from the
source.

- Pros: the common case (edit one value in a large file) does almost no
  tree work.
- Cons: two code paths for the same grammar; every operation that walks the
  whole document (Format, validation) must still materialise everything.

## Measure first

`benchmarks.txt` and `go test -bench .` record the comparison; the target is
parity with BurntSushi/toml on BenchmarkParse and BenchmarkUnmarshal at the
same allocation order, checked by a benchmark-regression test so a later
change cannot give the speed back.

## Affected files

- the parser and node types, the trivia representation, the key index
- `bench_test.go`, `benchmarks.txt`
- the README's performance section, which then states parity instead of
  the trade-off

## Effort

Large.
