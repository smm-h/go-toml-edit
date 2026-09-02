# Design record: the strictness-and-fidelity redesign

This file is the decision ledger for the library-wide redesign of go-toml-edit
(strict-by-default decoding, a public logical read-layer, full AST
encapsulation, lexeme-preserving rendering, a unified diagnostic contract, and
the pruning/renaming of the public surface). Every ruling below is binding on
the implementation. Where a ruling has an origin tag, it means:

- `[deliberate]` — the user authored or explicitly chose this against or beyond
  a recommendation. Strongly held.
- `[approved]` — the user picked a recommended option. Weakly held by
  convention: walk back freely if evidence turns against it, and never cite it
  back to the user as their deliberate intent.
- `[derived]` — implementation-internal, resolved by the session under the
  constraints of the other rulings. Revisable without a user decision whenever
  the surrounding rulings permit.

The standing process rules governing all work under this record:

- **Mechanism per assertion.** Wherever this record claims a property, the
  claim names the mechanism that fails when the property does not hold (a
  compile error, a test, a count assertion, a derived list) — or is explicitly
  marked `(descriptive)`, meaning it states intent or context rather than a
  verifiable property. A property claim with neither is a defect in this
  record.
- **Every test names its failure mode.** Each new test states, in a one-line
  comment, what change would make it fail. Mechanism: a test-source check that
  scans this package's test files added by the redesign for the marker comment
  and fails on a test function lacking one.

Vocabulary used throughout, defined once:

- **Trivia** — the non-semantic bytes attached to a node: leading whitespace,
  leading comments, the inline comment, trailing newline.
- **Fragments** — the byte ranges a rendered construct decomposes into. For a
  key-value line: leading trivia, key bytes, separator bytes (whitespace and
  `=`), value bytes, inline comment, trailing newline. For a table header:
  leading trivia, bracket-and-key bytes, inline comment, trailing newline.
- **Dirty** — a FRAGMENT whose original bytes are no longer valid because the
  corresponding content was mutated. The serializer re-renders dirty fragments
  and splices the original bytes of clean ones. (A node is "dirty" when any of
  its fragments is.)
- **Lexeme** — the exact source bytes of a value as written (`0x2A`, `1e3`,
  `+nan`), stored on the node at parse time; the value fragment's clean bytes.
- **The read-layer** (also "the fold") — the ordered logical-tree view of a
  document defined in the read-layer section below.
- **The wave** — the library-side implementation work under this record,
  ending in one release.
- **The sweep** — the post-release migration of the consumer repositories.
- **Consumer** — a repository with at least one Go file importing this
  package. A repository that merely requires the module in go.mod needs, at
  most, a `go mod tidy` (and, where it also requires strictcli, the strictcli
  bump described in the sweep section) when its direct dependencies move.

## 1. Identity and constraints

- The library is developed fleet-first: its consumers are the local projects
  that depend on it, and their demonstrated needs drive the API. `[approved]`
  `(descriptive)`
- No decision may be one that a later publicization of the library would force
  us to walk back. Publicity itself is unscheduled and tracked in the todo
  directory. `[deliberate]` `(descriptive)`
- The module path (`github.com/smm-h/go-toml-edit`) and package name
  (`tomledit`) both stay. The README's import example is changed to the named
  form (`tomledit "github.com/smm-h/go-toml-edit"`), and the README gains a
  "no runtime dependencies" statement; both edits go in `docs/_README.md`,
  from which the read-only `README.md` is generated. `[approved]`
- The library is a single Go package with no internal subpackages
  (pre-existing convention, reaffirmed). Every encapsulation claim below is
  therefore about what the type system makes representable, not about package
  boundaries. `(descriptive)`
- The BurntSushi/toml and toml-test dependencies stay in the main module.
  Mechanism for the no-runtime-dependencies claim: a test asserting the
  non-test import graph pulls in no external module. Moving them to a nested
  module was rejected: it would silently remove the compliance suite from a
  root `go test ./...`, weakening the release's most important check for a
  cosmetic gain. `[approved]`
- TOML 1.0 only, full compliance with the official toml-test suite,
  non-negotiable (pre-existing convention, reaffirmed; mechanism: the corpus
  tests of the release-and-process section).
- Pre-stable versioning: the redesign ships as one breaking minor release on
  0.x. No 1.x tag ever. No retraction of prior versions, no deprecate-first
  intermediate release. `[approved]` `(descriptive)`

## 2. Strict decoding: one engine

- Decoding is strict by default and strictness is the only mode. An unknown
  key, a wrong-typed value, or a missing required key is a hard error. There
  is no lenient mode, no option to skip checks. Mechanism: the flipped
  leniency tests plus the exclusion/required test set. `[deliberate]`
- One validation engine: the core is descriptor-driven (the expected document
  shape described as data). The reflection-based struct `Unmarshal` is a
  front end that derives a descriptor from the struct and runs the same
  engine. Mechanism: a test decodes one document through the struct front end
  and through an equivalent hand-built descriptor and asserts identical
  diagnostics (path, kind, order). `[approved]`
- The descriptor surface (contract in the API-contracts section) is exported
  and hand-constructible without reflection: consumers with runtime-known
  schemas build descriptors from their own registries. Mechanism: a test in
  which a descriptor built by hand (no reflection) validates a document.
  `[approved]`
- Engine scope: presence and type and required-ness are the decoder's job;
  relationships between values (choices/enums, ranges, uniqueness, patterns,
  custom messages) are not. Consumers keep thin app-side validators for
  those, written against the read-layer so they retain source positions.
  `[deliberate]` `(descriptive)` (An "enum in the descriptor" extension will
  be recorded in the deferred-work todo of the deferred-and-rejected section
  as an open decision with pros and cons, not a rejection.)
- Exclusions are refusals: a document key naming a `toml:"-"` field or an
  unexported field is a hard error, exactly like any unknown key. Exclusion
  means "this name is not part of the document universe", never "present but
  ignored". Mechanism: a test per exclusion category. `[approved]`
- Unknown struct-tag options are hard errors at field-mapping construction
  (with the map-only Marshal deleted, `omitempty` no longer means anything; a
  meaningless tag option must fail loudly, not silently no-op). Mechanism: a
  test per rejected option, each asserting the error names the offending
  option. `[derived]`
- An unknown table (or an unknown array-of-tables) produces one error naming
  it and listing its direct child keys — one actionable error carrying the
  immediate inventory, not one error per contained key and not a recursive
  listing. Mechanism: a fixture test asserting the single error and its key
  list. `[approved; the direct-children depth and the array-of-tables
  extension are derived]`
- **The conversion table.** All value conversion — engine, struct front end,
  and every accessor family — is driven by one table, written once in code
  and reproduced here as the ruling. Mechanism: a table-driven test runs
  every row (source kind, target kind, boundary values) through the decode
  engine and each accessor family and asserts identical results; the code
  table is the test's input, so a second copy of any rule fails it.

  | TOML value | Go targets accepted | Rule |
  |---|---|---|
  | string | `string`; `encoding.TextUnmarshaler` | verbatim |
  | integer | `int`, `int8`–`int64`, `uint`–`uint64` | range-checked; overflow and negative-into-unsigned are hard errors |
  | integer | `float64`, `float32` | only if exactly representable in the target type; inexact is a hard error (the widening rule) |
  | float | `float64` | verbatim |
  | float | `float32` | range-checked; a value outside float32 range is a hard error; precision truncation to the declared target is permitted (the caller declared the width) |
  | float | integer targets | never — a hard error even for whole floats |
  | boolean | `bool` | verbatim |
  | offset date-time | `time.Time` | verbatim |
  | local date-time | `LocalDateTime`; `time.Time` | the `time.Time` conversion is kept from the existing decoder (the declared target expresses intent); flavor information is not invented — the produced `time.Time` has no offset semantics beyond what the existing behavior assigns |
  | local date | `LocalDate`; `time.Time` | same as local date-time |
  | local time | `LocalTime` | verbatim |
  | array | slice, fixed-size array (over-length errors), `[]any` | elementwise by this table |
  | any table form | struct, `map[string]T`, `map[string]any`, `any` | per the descriptor/front end; map values decode elementwise by this table |
  | any value | `any` | the existing native mapping (string, int64, float64, bool, time.Time, the Local types, `[]any`, `map[string]any`) |

  The coercion principle, stated precisely: conversion BETWEEN TOML types
  happens only when provably value-preserving (the integer-to-float row);
  narrowing INTO the declared Go target's width is the caller's explicit
  choice and is range-checked, never silent-wrapping. Custom `Unmarshaler`
  and `encoding.TextUnmarshaler` hooks, embedded struct promotion,
  exact-then-case-insensitive field matching, and pointer targets are all
  kept from the existing decoder (mechanism: the existing unmarshal suite,
  which stays green except the flipped leniency tests). `[approved for the
  widening row; derived for the rest, which preserves existing tested
  behavior]`

## 3. Decode error reporting

- Strict decode stops at the FIRST violation in document order, and returns
  it wrapped in the aggregate error type (carrying a list that, under this
  ruling, always holds one diagnostic). This was re-confirmed on corrected
  evidence: the one fleet implementation of strict decode (wavescript's
  strictdecode) deliberately stops at the first violation, and the
  multi-diagnostic reporting observed in other consumers sits in their
  post-decode domain-validation layers, which this design keeps app-side.
  Shipping the aggregate TYPE regardless of behavior makes a later upgrade
  to collecting independent violations purely additive — no consumer
  breaks — so that upgrade is deferred work, built when a consumer asks.
  Mechanism: a multi-violation fixture asserting exactly one diagnostic is
  returned and it is the first in document order. `[approved]`
- The aggregate error's contract: it implements `Unwrap() []error` returning
  the diagnostics in document order; `errors.As` with a single-diagnostic
  target yields the first diagnostic (standard traversal order); `errors.As`
  with the aggregate target yields the whole list; `errors.Is` against a kind
  sentinel matches if any contained diagnostic carries that kind. Mechanism:
  a contract test exercising all four behaviors. `[derived]`
- Parsing is likewise first-error-only (a parse cannot meaningfully continue
  past a syntax error); parse errors are not wrapped in the aggregate.
  `[approved]` `(descriptive)`

## 4. The unified diagnostic contract

- Parse, decode, edit, and access errors share one structured shape: document
  path, position, span, message, and an optional filename. Matching via
  `errors.Is` (kind sentinels) and `errors.As` (structured access) is the
  documented compatibility contract; the type implements `Unwrap` wherever it
  wraps. Mechanism: a contract test matching one representative error from
  each producing surface through both idioms. `[approved]`
- The complete kind set (exported constants of `ErrorKind`, one sentinel
  each; the API-snapshot mechanism of the public-surface section holds this
  closed):
  - `KindSyntax` — lexing/parsing failure.
  - `KindUnknownKey` — strict decode: a key matching no descriptor field
    (including excluded names).
  - `KindUnknownTable` — strict decode: an unknown table or array-of-tables,
    with its direct-child inventory.
  - `KindMissingKey` — strict decode: a required key absent.
  - `KindTypeMismatch` — a value whose kind the conversion table refuses for
    the target (decode and access type-check stage alike).
  - `KindInexact` — the conversion-table widening/narrowing rows' range and
    exactness failures.
  - `KindNotFound` — access/edit: the path names nothing.
  - `KindBadPath` — path syntax error.
  - `KindWrongContainer` — access/edit: a path step is structurally
    inapplicable (key on an array, index on a scalar, and the like).
  - `KindBadInput` — an invalid input value to an editing operation
    (unsupported Go type, sign-bit NaN, duplicate ordered-input key,
    non-bijective permutation, unsigned overflow).
  - `KindConflict` — an edit refused because it would produce an invalid
    document (`NewTable` colliding with an array-of-tables, duplicate table
    creation).
  - `KindRoundTrip` — `WriteFile`'s validation failure: carries the filename
    and the byte offset of the first divergence. `[derived]`
- `Position` carries line, column, and byte offset. This is a change to the
  position type itself, so every node span carries the offset: the token
  gains an offset, position advancement carries it, and every span
  construction site populates it. (The strictspec consumer's
  offset-derivation workaround reconstructs offsets for node spans, so only
  a position-level offset discharges that deletion.) The diagnostic embeds a
  `Position` rather than parallel scalar fields, so the position triple is
  declared once. Mechanism: extended span tests asserting offsets across
  construct kinds. `[derived]`
- The current `ParseError` identifier does not survive: the unified type is
  named `Error` (settled-names section), so every consumer touch-point fails
  to compile and gets visited during the sweep (one consumer uses a bare
  type assertion that would otherwise silently stop matching). `[derived]`
- Every parser-stage error site is routed through the single error
  constructor and carries offset and snippet (before this wave, only
  lexer-stage errors fill them). Mechanism: a test walks this package's own
  source and fails on any composite literal of the diagnostic type outside
  the constructor's file. `[derived]`
- The array-of-tables collection span is synthesized by the read-layer, which
  owns the collection concept: from the first entry's header start to the
  last entry's content end (the end of that entry's last child, not its
  header). The concrete array-table node's own span (its header brackets) is
  unchanged. Mechanism: a span assertion in the read-layer suite. `[derived]`
- Default path rendering in messages uses the library's own path syntax
  (dotted keys, bracket indices, quoted-when-not-bare segments) — the same
  strings the path API accepts. Mechanism: a test resolves a diagnostic's
  rendered path via `Get` on the same document. Errors are data; consumers
  with their own diagnostic vocabularies render the structured fields
  themselves. `[approved]`

## 5. The logical read-layer

- A designed, read-only logical-tree API sits alongside the concrete syntax
  AST. The two surfaces answer different questions: the read-layer answers
  logical questions (what the document means — values, structure, order),
  and the AST answers syntactic questions (what the file contains, in the
  form it was written — spelling, trivia, spans of concrete constructs). A
  consumer reading values uses the layer; a consumer editing, or inspecting
  how something was written, uses the AST and the path API. That sentence
  pair is the contract and belongs in the package documentation. `[approved]`
  `(descriptive)`
- Its semantics are taken from the folds that two consumers implemented
  independently (strictspec's internal `tomldoc` converter and wavescript's
  internal `strictdecode` document fold): first-appearance key order across
  all binding forms; implicit records created by dotted keys and deep
  headers; dotted-key expansion; array-of-tables entries with last-entry
  addressing for sub-table headers; inline tables folding into ordinary
  records; first-header-wins span anchoring; a per-key span on every entry.
  Mechanism: one exported-behavior suite over the fold's semantics,
  including a port of strictspec's `tomldoc` test expectations; that suite
  is the only statement of the semantics, so any second implementation must
  pass it verbatim. `[approved]`
- Every silent-tolerance point of strictspec's `tomldoc` fold becomes a hard
  internal error in this implementation: the value-conversion default branch
  that misclassifies an unhandled node as a string, the finalize precedence
  that would silently drop a colliding sub-record, and the finalize default
  branch that fabricates an empty record for an unassigned slot. Mechanism:
  internal-error paths covered by tests with synthetic inputs. `[derived]`
- Implementation directive: build the layer by retaining and extending the
  parser's existing definition tracker (which already computes implicit
  records, dotted expansion, and first-header-wins, then discards the
  tree). Fall-back criterion, decided now: if the extension would require
  exporting parser-internal state or weakening the parser's conflict
  detection, build a separate post-parse fold instead — the exported-behavior
  suite is identical either way, so the choice is invisible at the API.
  `[derived]`
- The layer is built lazily: constructed on first read-layer access,
  invalidated by a document-level generation counter. The counter is bumped
  by the same single unexported mutator that the node-model section mandates
  for value writes — a setter that forgets to bump is unrepresentable, not
  merely forbidden. Until encapsulation makes that airtight, the layer is
  built eagerly; the switch to lazy is part of the node-model pass in the
  total order, and lands with a test asserting `Parse` alone does not build
  the layer (an internal build counter), so a leftover eager build cannot
  linger invisibly. `[derived]`
- Path resolution is re-based on the layer. The internal virtual-view types
  it replaces are retired from resolution immediately and deleted once their
  last dependents (iteration, merge) are ported. Getter behavior across the
  wave: the path-level getters and the Cursor terminals change SIGNATURE per
  the accessor section; their answers are unchanged except that the float
  accessors (path-level, the Cursor's float terminal, and the new node-level
  family) accept an exactly-representable integer, which the previous float
  getters refused. Mechanism: the existing document/path tests, updated for
  signatures, with the widening flips as the only behavioral edits.
  `[derived]`
- Path helpers are exported: parse, join, and quote, as the single authority
  for path text. Mechanism: a round-trip test (parse of a joined path yields
  the original segments, over dotted, quoted, and indexed shapes), plus the
  diagnostic paste-back test of the diagnostics section. The renderer-side
  bare-key predicate remains separate and documented as a TOML-syntax rule
  (it governs how keys are written in TOML, not how paths are spelled).
  Diagnostics, defaults-seeding, and every other path-producing feature call
  the exported helpers. `[derived]`

## 6. The node model

- The `Node` interface stays the universal handle (type, span, raw, comments)
  so traversal callbacks and cursor navigation keep their signatures. Value
  access moves to a `Scalar` sub-interface carrying `Value()` and the typed
  accessors. Scalar node types: string, integer, float, boolean, and the
  four date-time nodes. Container node types (document, table, array-table,
  array, inline table) — interface AND concrete types — lose `Value()`
  entirely and expose ordered entries/elements; keeping the concrete method
  would leave the wrong question compilable on any concretely-typed
  variable. The remaining node types are neither: the key node exposes its
  parts, the comment node its text, and the key-value node its key and value
  nodes, each through named accessors, none through `Value()`. This closes a
  live consumer bug: howmuchleft's profile reading asserts `[]interface{}`
  on an array node's `Value()` (which returns the node slice), so the read
  has always silently returned nil. Mechanism: `Value()` absent from every
  non-scalar type is held by the API-snapshot test. `[approved; the
  concrete-type removal and the per-type disposition are derived]`
- Full encapsulation: node struct fields are unexported; reads go through
  accessor methods; writes go only through setters that mark the affected
  fragment dirty and distinguish value mutation (which invalidates the
  stored lexeme) from trivia mutation (which preserves it). String style and
  integer base remain readable through accessors (`StringStyle` and
  `IntegerBase` stay exported as their types); the `Trivia` struct is
  unexported by the same argument that unexports the token vocabulary — it
  is reachable from no exported API, and the normalized comment getters plus
  `Raw()` cover the read need. Mechanism: the API-snapshot test plus a
  reflection test asserting no exported fields remain on node types.
  `[approved; the Trivia/style dispositions are derived]`
- Lexeme invalidation is structural, not disciplinary: each scalar's payload
  and its raw lexeme live in one small unexported struct whose only mutator
  clears the lexeme and bumps the read-layer generation counter — writing
  the value without either effect is unrepresentable, in-package included.
  `[approved]`
- Setters propagate dirtiness upward (parent references maintained by the
  structural operations), replacing the per-render recursive subtree-
  dirtiness walk with a constant-time check. Mechanism: a test asserts the
  dirtiness check visits the same number of nodes (via an internal counter)
  for a shallow and a deeply nested document; a deep-nesting benchmark
  additionally tracks the render cost. `[derived]`
- The document type is renamed from `DocumentNode` to `Document`. The
  complete rename set is the settled-names section, which is closed.
  Sequencing of the rename relative to the other mechanical passes is stated
  once, in the release-and-process section. `[approved]`
- The exported token vocabulary (token types and the token struct) is
  unexported: it is reachable from no exported API and contradicts the
  parser-is-internal decision. A future streaming parser designs its own
  token surface additively if ever built. `[approved]`

## 7. Fidelity and rendering

- **The fragment contract.** Rendering works per fragment, not per node: a
  construct's clean fragments splice their original bytes; only dirty
  fragments re-render. Mutations invalidate exactly their fragment: a value
  write invalidates the value fragment (the lexeme); a comment or
  leading-comment write invalidates that trivia fragment; nothing else
  invalidates anything. Separator bytes, intra-header whitespace, and key
  bytes are never invalidated by value or trivia writes — they re-render
  only when the construct itself is created or structurally replaced.
  Consequences, each covered by the corpus battery of the release-and-
  process section: editing a value leaves the line's spacing and comment
  bytes identical; editing a comment leaves the value's spelling identical;
  editing one element of a container leaves every sibling's bytes
  identical. Header key raw parts (computed by the parser today and
  discarded) are wired into both header node types so header re-renders
  preserve key spelling; inline-table key rendering prefers raw parts.
  `[approved; the fragment decomposition is derived]`
- Constructed and value-mutated scalars render canonically: lowercase hex in
  unicode escapes; the float form is shortest-round-trip decimal digits
  with a float marker always present (a `.` or an exponent), using
  lowercase-`e` exponent notation exactly when the shortest form requires an
  exponent, and `-0.0` keeping its sign. This restates the schema
  toolchain's value-rendering appendix (strictspec's
  `spec/appendix-rendering.md`, its float-form section), which is the
  origin of the rule; the record's statement here is self-contained so the
  implementor needs no external document, and a conformance test pins the
  behavior locally. Special floats render as `nan`, `inf`, and `-inf`; the
  library never writes `+nan`, `-nan`, or `+inf`. Mechanism: rendering tests
  per rule, including a special-float enumeration. `[approved; the inline
  restatement and the infinity spellings are derived]`
- The correct literal renderers (string quoting, key quoting, float
  formatting) are exported for consumers. This replaces the hand-written
  TOML value renderer in wavescript, whose string quoter is Go's
  `strconv.Quote` (emitting `\a`, `\v`, and `\xNN` escapes that TOML 1.0
  rejects) and whose float path renders special floats through
  `strconv.FormatFloat`'s `NaN`/`+Inf` spellings, which are not valid TOML.
  A consumer renderer implementing another project's own documented
  rendering rule — strictcli's cross-language canonical float, strictspec's
  format-neutral value renderer — is not a replacement target. Mechanism:
  exported-renderer tests covering the invalid-escape and special-float
  cases. `[approved; the defect description was corrected twice during
  review — the surviving claims are verified against the consumer tree]`
- All TOML-valid spellings remain valid INPUT (`+nan`, `-nan`, `+inf`, any
  quoting, any integer base — the compliance suite is non-negotiable) and
  are preserved byte-for-byte while untouched. Canonicalization applies only
  to what the library writes. `[deliberate]`
- `Format()` preserves the user's blank-line grouping: runs of blank lines
  collapse to one. The table-blank-line option is insertion-only — it
  controls whether a missing blank line before a table header is inserted,
  never whether existing ones are removed. Mechanism: a test iterates every
  formatting-option combination over a blank-line fixture and asserts no
  combination removes the grouping. `[approved]`
- Whole-document canonical form (one true table style, key order,
  indentation) is out of scope: it belongs to a document-level canonical-form
  specification whose authorship is filed as a deliverable in the strictspec
  project. `[approved]` `(descriptive)`

## 8. The Set-equality contract

- `Set(path, value)` — and every value-writing entry point sharing its path,
  including the create-if-missing variant and `EnsureDefaults` — is a no-op
  if and only if the bytes it would write for that value are exactly the
  value-fragment bytes already stored. When the target carries no stored
  lexeme (constructed, or previously value-mutated), the comparison is
  against its canonical rendering. For a container-valued Set (a map, an
  ordered pair slice, or a slice), the comparison is the rendered container
  against the stored container's full byte range — rarely equal in
  practice, and the rule needs no container-specific exception. Equality is
  decided before any mutator runs: a no-op never touches the payload, so a
  stored lexeme and the fragment states survive it. `[deliberate; the
  no-lexeme and container cases are derived]`
- One rule, no special cases: it covers NaN spellings (setting NaN over
  `nan` is a no-op; over `+nan` it writes `nan`), infinities (positive
  infinity over `inf` is a no-op; over `+inf` it writes `inf`), signed
  zeros, integer-vs-float, date-time offsets, string quoting, and integer
  bases alike. The accepted consequences: an idempotent tool that Sets a
  value stored in a non-canonical spelling (octal, hex, underscores,
  exponent floats, literal quotes, `+inf`) normalizes that one value's
  spelling on first touch and is byte-stable thereafter; and a same-content
  Set over a literal-quoted string converts it to basic quoting. What you
  Set is what the file says. Mechanism, two corpus properties: Set-then-Set
  is byte-stable (the second identical Set changes nothing), and for values
  already in canonical spelling a same-value Set is byte-identical to not
  setting at all. `[deliberate]`
- `Set` refuses non-canonical NaN input: a NaN with its sign bit set is a
  hard error (`KindBadInput`), never silently normalized. The accepted NaN
  writes `nan`. (Infinities need no input rule: a Go infinity's sign is its
  value.) Mechanism: input-refusal tests. `[deliberate]`
- A no-op `Set` never clears dirtiness from an earlier edit — otherwise
  saving could silently revert a previous, unsaved change. Mechanism: an
  edit-then-identical-Set test asserting the first edit still renders.
  `[approved]`
- The whole contract lives in the exported doc comments of the value-writing
  entry points, not only in this record: it is user-visible behavior.
  `[derived]`

## 9. The write path

- Structural operations (signatures in the API-contracts section) exist
  before encapsulation removes direct field access, and their scope is the
  demonstrated need: arbitrary permutation of a node's children, key
  reordering within a table, array element append and remove at a path, and
  an ordered inline-table input type. The permutation is total — a bijection
  on the child indices; a duplicate, missing, or out-of-range index or a
  length mismatch is a hard error (`KindBadInput`) naming the offending
  index, and nothing is reordered. Standalone comments are children and move
  with their assigned positions. Known tension, probed by the acceptance
  test below: the driving consumer's reorder also DROPS children absent
  from its ordering list — under this API that composes as delete-then-
  permute, and the acceptance test must exercise that composition. The
  pgdesign document/table reorder scenario is captured as an in-library
  acceptance test BEFORE the operation signatures freeze. `[approved; the
  totality rule and the composition note are derived]`
- The ordered inline-table input is a slice of key/value pairs (the `Pair`
  type of the API-contracts section). A duplicate key is a hard error
  (consistent with the no-silent-tolerance stance); key syntax is validated
  at the Set call (the slice itself is plain data). It serves inline-table
  construction; ordinary tables are built through the table-creation and
  set operations. Mechanism: construction tests incl. the duplicate
  refusal. `[derived]`
- `EnsureDefaults` (renamed from `MergeDefaults`, redesigned to the shape
  howmuchleft's implementation proved): input is an ORDERED slice of
  path/value pairs — a map input would make the appended-key order and
  therefore the output bytes nondeterministic — seeding only missing paths,
  returning the list of paths it added, delegating to the create-if-missing
  set operation internally. Mechanism: the seeding scenario test plus a
  determinism assertion (same input slice, byte-identical output).
  `[approved; the ordered-input requirement is derived]`
- The comment API's public spelling is path-based (`SetComment(path, text)`,
  `SetLeadingComments(path, texts)`); the per-node setters become unexported,
  resolving a method-shadowing collision on the document type. Normalized
  comment-text getters (content without the `#` and surrounding whitespace)
  are added; `Raw()` remains for byte-exact inspection. Mechanism: comment
  API tests; the API-snapshot holds the surface. `[approved]`
- `ParseFile` (filename flows into the diagnostic contract) and an atomic
  validate-on-write `WriteFile` are added. `WriteFile`'s contract is the
  deterministic invariant set, each item covered by test: the temp file is
  created in the destination's directory; the document must
  round-trip-validate before the rename; an injected write or rename failure
  leaves the destination bytes untouched and no temp file behind; file mode
  survives the replace. A round-trip-validation failure is `KindRoundTrip`,
  carrying the filename and the byte offset of the first divergence.
  Kernel-level crash injection is not part of the suite. `[approved; the
  failure kind is derived]`

## 10. Typed accessors

- One design across all read surfaces, built as unexported stages — navigate
  (its failures: `KindNotFound`, `KindBadPath`, `KindWrongContainer`),
  type-check (its failure: `KindTypeMismatch`), convert (its failure:
  `KindInexact`) — so the failure kinds are produced by different functions
  and cannot be conflated. The conversion table of the strict-decoding
  section drives BOTH the type-check stage (which node kinds are acceptable
  for a target) and the convert stage (whether the specific value passes);
  it is shared with the decode engine. Mechanism: a test per stage asserts
  the kind sentinel, including that a read on an absent path reports
  not-found rather than wrong-type; plus the cross-family table test.
  `[approved]`
- Accessor errors are the unified `Error` type — same contract, same kinds,
  path and span and expected/got and offending value populated. If an
  allocation-free status is ever wanted, it must be this contract's kind,
  never a parallel enum. `[approved]`
- Ergonomics: path-level and node-level accessors return `(T, error)`;
  Cursor terminals also return `(T, error)` (one vocabulary), with the
  Cursor's `Err()` retained only for chains ending in navigation. A comma-ok
  existence surface (`Lookup`/`Has`) serves the optional-with-default read;
  its allocation budget is "no allocations beyond path parsing" (path
  strings must be parsed; a pre-parsed path type is not part of this wave).
  Mechanism: an allocations-per-run assertion at the path-parse floor.
  `[approved; the allocation budget correction is derived]`

## 11. Public surface end-states

- Kept and re-based on the read-layer: the fluent Cursor (thin sugar over
  the shared resolution engine), Walk (traversal over the layer's ordered
  enumeration), Merge, and Diff. Diff compares logical structure — two
  documents expressing the same structure in different TOML spellings compare
  equal; before porting, the concrete case that today compares unequal is
  named in a test, or the port is reclassified as verification of existing
  behavior (an open determination, resolved during the port either way).
  `[approved]`
- Deleted: the path-based `Items`/`Len` on the document (superseded by the
  read-layer's `Record.Entries`/`Record.Len`; the Cursor's own iteration
  survives), and the map-only `Marshal` (no callers anywhere in the fleet;
  its forced alphabetization contradicted the one plausible consumer; a real
  struct-to-TOML Marshal is deferred work, not this campaign). `[approved]`
- Mechanism for the whole surface: a committed exported-API snapshot test —
  an un-performed deletion, an unruled addition, and an unruled `ErrorKind`
  member all fail it. The release's migration table is generated by diffing
  this snapshot against the same snapshot generated from the last released
  version, so the table and the test share one derivation. `[derived]`
- Small correctness fixes riding the wave (each red-green): overflow
  checking on unsigned integer conversion in the value-to-node path
  (`KindBadInput`); `NewTable` refusing a name collision with an existing
  array-of-tables (`KindConflict`; the current behavior can produce output
  the library's own parser rejects); removal of the unused parent-dirty
  parameter once dirtiness propagation exists; the formatter's spurious
  blank line from skipped blank-line nodes (fixed inside the blank-line-
  preservation rework, with a pinning regression test). `[derived]`

## 12. Consumer end-states and sweep constraints

The sweep follows the module graph. Constraints discovered and binding:

- Order: release this library; then migrate strictcli and its conformance
  harness in one commit and RELEASE strictcli (several repos require both
  modules — minimal version selection would otherwise compile strictcli's
  unmigrated source against the new API inside their builds); migrate
  safegit in the same working session (its Go workspace `use`s the strictcli
  checkout, so its tests go red the moment that checkout is migrated); every
  remaining consumer bumps BOTH module requirements in one commit
  (howmuchleft's strictcli bump spans more released versions than the
  others' — it lags the fleet's strictcli version). saferm — which
  workspace-`use`s the strictcli checkout but does not import this package —
  gets a `go work sync` plus the strictcli requirement bump when strictcli
  releases. `[derived]`
- Each consumer's authoritative break list is produced mechanically before
  the release: at the end of the library work, every consumer is built
  against the pre-release tree in a scratch clone with a temporary replace
  directive (never committed), and the compiler output drives the sweep and
  the migration table in the release notes. `[derived]`
- Per-consumer rulings. wavescript: deletes its internal strict-decode layer
  and its typed-read, enumeration, comment, value-rendering, and write-cycle
  helpers. strictcli: builds runtime descriptors from its flag and config
  registries, keeps its cross-language message vocabularies (rendering
  engine error data), migrates its bare error type assertion, adopts
  `ParseFile`, keeps its dry-run effects-handle writes. (Out of this wave's
  scope: strictcli's conditional unknown-key rejection — a todo is filed in
  the strictcli project proposing to investigate and make it unconditional
  across all of its implementations; that work belongs to strictcli's own
  campaign and is not a sweep item.) pgdesign: the minimal-correct
  migration — delete the excluded-field workaround and the two-pass hand
  decode (decoding its dynamic sections directly into map fields, which the
  map-of-struct fix makes work), plus the mechanical conversions; its
  generated validator stack stays untouched and remains the shape authority
  (the handover question is a later, separate campaign). howmuchleft: keeps
  its startup auto-create of the config file; a present-but-invalid file
  becomes a hard positioned error instead of a silent fall-back to defaults;
  adopts `EnsureDefaults`, plain (non-pointer) config fields, the
  container-entries fix for its always-nil profile reading (a named
  consumer-facing defect in the changelog), and the preserving serializer
  for write-back. safegit: adopts required tags for presence and keeps
  pointer fields only for its mutual-exclusivity domain rule. dirstat:
  adopts a descriptor for its flat schema and keeps choices, uniqueness, and
  bespoke messages app-side. strictspec: keeps its own format-neutral
  document model; its TOML front end shrinks to a thin conversion from the
  read-layer, and its byte-offset derivation and span-synthesis workarounds
  are deleted. `[approved; the howmuchleft failure policy and the pgdesign
  shape were re-ruled on corrected premises]`

## 13. Release and process

- One release at the end of the library work, a breaking minor on 0.x, with
  changelog entries recorded per work item as it is committed (never batched
  at release time — the per-entry commit limits refuse large ranges), and a
  migration table in the release notes generated from the API-snapshot diff
  of the public-surface section. The sweep follows the release. `[approved]`
- The implementation's total order, stated once (other sections point here):
  repository hygiene and test-infrastructure hardening; the mechanical
  `DocumentNode`-to-`Document` rename as its own scripted early pass (it has
  no semantic dependencies, and everything after it is written against the
  final name); the diagnostic contract; the read-layer and path re-basing;
  the decode engine (absorbing the decode-bug regression tests); the
  structural operations; the node-model pass (interface split, field
  unexporting, setter discipline, dirtiness propagation, and the read-layer's
  eager-to-lazy switch); fragment-based rendering and value
  canonicalization; the formatter's blank-line preservation; the ports
  (Cursor, Walk, Merge, Diff) and the deletions and the token/`Trivia`
  unexporting as one late scripted identifier sweep; the documentation truth
  pass; the pre-release consumer compile check; the release. Semantic
  changes follow dependency order; mechanical changes collapse into as few
  scripted passes as that order allows, each with a dry run, asserted
  per-file occurrence counts, and a reviewed diff. `[derived]`
- Repository hygiene precedes the wave: a gofmt sweep as its own commit, CI
  steps failing on unformatted code and on staticcheck findings, the go
  directive relaxed to the minor version, and a CI matrix carrying more than
  one Go version (which requires switching the workflow from reading the
  version out of go.mod to matrix-supplied versions). The CI job name stays
  `test` — the publish workflow's check matcher keys on it and already
  tolerates a matrix suffix. `[derived]`
- Test infrastructure: the compliance-suite skip list is derived by
  differencing the corpus walk against the corpus's own version-filtered
  listing API restricted to the valid and invalid trees (the listing also
  contains an encoder mirror the harness does not use); corpus case counts
  are asserted in the test, which is the single authority (the counts — and
  the audit-test-file count beside them — are removed from the
  documentation; the carrier is the CLAUDE template, `docs/_CLAUDE.md`);
  every fuzz target reads one committed seed list, with only minimized
  crashers added to it; the read-error signal in the corpus walk survives
  the removal of the misleading counters. `[derived]`
- The corpus test battery (each entry naming its property): the
  fragment-fidelity passes — for every valid corpus file, mutate each value
  through public setters and assert all other bytes unchanged; mutate trivia
  and assert value bytes unchanged; the renderer-correctness pass — with
  lexeme splicing disabled through an internal test-only switch, render,
  re-parse, and assert semantic equality plus an idempotent second render;
  the stale-lexeme pass — set every scalar to a NEW value and assert
  re-parsed values match; and the Set-equality passes of the Set-equality
  section (Set-then-Set byte-stability; canonical-spelling same-value
  no-op). `[derived]`
- Red-green discipline for the decode bugs fixed by the engine replacement
  (map-of-struct path truncation; array-of-tables under a plain table
  silently dropped; the map-element reflect panic): tests phrased entirely
  on the public surface, committed before the engine work with their red
  failure output pasted into the commit message (the old decoder will not
  exist to reproduce it), turning green under the engine. The audit test
  that only compared two decode entry points against each other is
  strengthened to assert actual values. `[derived]`

## 14. Deferred and rejected

A deferred-work todo will be filed at the end of the campaign (shown to the
user before filing) carrying: the real struct-to-TOML Marshal (with the note
that pgdesign's exporter could shrink onto it); the full structural-
manipulation suite (node moves across containers, positional insertion,
table/inline conversion); the splice API (replacing an exact byte span in
place, a capability one consumer implements format-neutrally for itself);
the upgrade of strict decode from first-error to collecting independent
violations (additive on the aggregate error type; build when a consumer
asks); a pre-parsed path type for allocation-free lookups; the
enum-in-descriptor extension as an open decision with pros and cons; and the
presence-reporting API recorded as rejected with its rationale (required-key
support plus `EnsureDefaults` absorbed the need; the remaining consumer case
is domain logic on a data shape where pointer fields are the better
instrument). A kernel-level crash-injection harness for `WriteFile` and a
struct-size budget test were considered and rejected (flaky in CI; a
hand-maintained number that breaks on legitimate change).

## 15. Settled names

Closed — this section is the complete rename-and-new-name set. Each entry
marks its origin (renamed from X, or new). All `[approved]`:

- **`Error`** (renamed from `ParseError`, generalized) / **`Errors`** (new,
  the aggregate) / **`ErrorKind`** (new) with per-kind sentinels. A
  package's single error type named `Error` is the established Go idiom
  (`url.Error`, `exec.Error`).
- **`Record` / `Entry`** (new) — the read-layer's container and element.
  Deliberately not `Table`: the type uniformly covers all four TOML
  spellings and must not be confusable with the concrete `TableNode`.
- **`Scalar`** (new) — the value-carrying node sub-interface.
- **`Spec` / `Field`** (new) — the descriptor and its per-key element; the
  proven names from wavescript's implementation the sweep deletes.
  Deliberately not `Schema`: that word is strictspec's domain.
- **`RenameKey`** (renamed from `Rename`) — renames a key in place, cannot
  move nodes; a future move operation can take a truthful name of its own.
- **`EnsureDefaults`** (renamed from `MergeDefaults`, redesigned) — the
  proven name from the consumer whose implementation the design adopts.
- **`Document`** (renamed from `DocumentNode`).
- **`Pair`** (new) — the ordered key/value element used by the ordered
  inline-table input and `EnsureDefaults`.

## 16. API contracts

The exported shapes an implementor builds to. Signatures are binding in
shape (receiver, parameters, returns); parameter names are free. The
API-snapshot test of the public-surface section holds all of these.

- **Read-layer.** `(*Document).Root() *Record` — the logical root,
  constructed per the read-layer section. `(*Record).Entries() iter.Seq[Entry]`
  (first-appearance order), `(*Record).Len() int`, `(*Record).Get(key string)
  (Entry, bool)`. `Entry` exposes `Key() string`, `KeySpan() Span`, and its
  value through exactly one of: `Record() (*Record, bool)` (any table
  spelling, including an inline table), `Records() ([]*Record, bool)` (an
  array-of-tables; the collection span of the diagnostics section is
  `RecordsSpan()`), or `Node() Node` (scalars and plain arrays — the
  concrete node, so read-then-edit and span/style inspection are one step).
  The layer is read-only: no type in it has a mutating method (mechanism:
  the API snapshot). `[derived]`
- **Descriptor.** `Spec` is `{Fields map[string]Field; Dynamic *Field;
  Other FieldPolicy}` in shape: `Fields` names the known keys; `Dynamic`,
  when non-nil, is the uniform descriptor for arbitrary additional keys;
  the explicit spellings replacing the reference implementation's nil
  overloading are `FieldAny()` (an any-value field constructor) and
  `SpecClosed()` (a no-keys table), so absence of a sub-descriptor is
  always an error, never a meaning. `Field` carries the expected kind (a
  closed kind enum covering the scalar kinds, array-with-element, table-
  with-spec, and any), a `Required bool`, an element `*Field` for arrays,
  and a `*Spec` for tables. Entry points: `(*Document).Validate(*Spec)
  error` (engine only), `(*Document).Decode(v any) error` and
  `Unmarshal(data []byte, v any) error` (struct front end), and
  `DecodeNode(n Node, v any) error` (the node-level decode of the
  strict-decoding section). Exact enum spellings are the implementor's
  within these shapes. `[derived]`
- **Structural operations.** `(*Document).PermuteChildren(path string,
  order []int) error` (bijection per the write-path section; empty path
  addresses the document's own children), `(*Document).SortKeys(path
  string, less func(a, b string) bool) error`,
  `(*Document).AppendToArray(path string, value any) error`,
  `(*Document).RemoveFromArray(path string, index int) error` (negative
  indices per the path syntax). The ordered inline-table input is
  `[]Pair` with `Pair{Key string, Value any}` (exported fields — it is
  plain input data, not an encapsulated node), accepted by `Set`/
  `SetCreate` wherever `map[string]any` is. `EnsureDefaults(defaults
  []Pair) (added []string, err error)` on `*Document`, paths as `Pair.Key`.
  `[derived]`
- **Accessors.** On `Scalar` and mirrored at path level and Cursor
  terminals: `AsString() (string, error)`, `AsInt() (int64, error)`,
  `AsFloat() (float64, error)`, `AsBool() (bool, error)`, `AsTime() 
  (time.Time, error)` (offset date-times), plus the Local-type accessors;
  path level keeps the `GetString(path)`-style spellings with `(T, error)`
  returns. `Lookup(path string) (Node, bool)` and `Has(path string) bool`
  are the comma-ok existence surface. `[derived]`
- **Renderers and path helpers.** `QuoteString(string) string` (basic-string
  form), `QuoteKey(string) string`, `FormatFloat(float64) string` (the
  canonical form of the fidelity section), and the path helpers
  `ParsePath(string) ([]PathSegment, error)` / `JoinPath([]PathSegment)
  string` with an exported `PathSegment`. `[derived]`
