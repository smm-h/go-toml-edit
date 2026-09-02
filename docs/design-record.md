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
  ratchet of the other rulings. Revisable without a user decision whenever the
  surrounding rulings permit.

Two standing process rules govern all work under this record:

- **Mechanism per assertion.** Wherever this record or the implementation
  claims a property, the claim must name the mechanism that fails when the
  property does not hold (a compile error, a test, a count assertion, a
  derived list). A property enforced only by prose is not enforced.
- **Every test names its failure mode.** Each new test states, in a one-line
  comment, what change would make it fail. A test that cannot answer that is
  measuring nothing.

## 1. Identity and constraints

- The library is developed fleet-first: its consumers are the local projects
  that depend on it, and their demonstrated needs drive the API. `[approved]`
- No decision may be one that a later publicization of the library would force
  us to walk back. Publicity itself is unscheduled and tracked in the todo
  directory. `[deliberate]`
- The module path (`github.com/smm-h/go-toml-edit`) and package name
  (`tomledit`) both stay. The README's import example shows the named form so
  readers meet the package name at first contact. `[approved]`
- The BurntSushi/toml and toml-test dependencies stay in the main module; the
  library advertises "no runtime dependencies". Moving them to a nested module
  was rejected because it would silently remove the compliance suite from a
  root `go test ./...`, weakening the release's most important check for a
  cosmetic gain. `[approved]`
- TOML 1.0 only, full compliance with the official toml-test suite,
  non-negotiable (pre-existing convention, reaffirmed).
- Pre-stable versioning: the redesign ships as one breaking minor release on
  0.x. No 1.x tag ever. No retraction of prior versions, no deprecate-first
  intermediate release. `[approved]`

## 2. Strict decoding: one engine

- Decoding is strict by default and strictness is the only mode. An unknown
  key, a wrong-typed value, or a missing required key is a hard error. There
  is no lenient mode, no option to skip checks. `[deliberate]`
- One validation engine: the core is descriptor-driven (the expected document
  shape described as data). The reflection-based struct `Unmarshal` is a front
  end that derives a descriptor from the struct and runs the same engine.
  There are never two validation engines to keep in agreement. `[approved]`
- The descriptor surface is exported and hand-constructible without
  reflection: consumers with runtime-known schemas build descriptors from
  their own registries. The proof obligation: a descriptor built by hand (no
  reflection) validates a document, covered by test. `[approved]`
- Descriptor expressiveness includes: scalar kinds (the four date-time
  flavors kept distinct), nested tables, arrays with element descriptors,
  arrays of tables, required markers, dynamic keys (arbitrary key names with
  one uniform value descriptor), and an explicit any-value element. Two
  design defects of the reference implementation are not reproduced: the
  asymmetric nil semantics (where a nil element meant "unchecked" but a nil
  table meant "no keys allowed") are replaced by explicit spellings for both
  intents. `[derived]`
- Engine scope: presence and type and required-ness are the decoder's job;
  relationships between values (choices/enums, ranges, uniqueness, patterns,
  custom messages) are not. Consumers keep thin app-side validators for
  those, written against the read-layer so they retain source positions.
  `[deliberate]` (An "enum in the descriptor" extension is recorded in the
  deferred-work todo as an open decision with pros and cons, not a
  rejection.)
- The struct front end keeps the existing reflection behaviors: embedded
  struct promotion, exact-then-case-insensitive field matching, custom
  `Unmarshaler` and `encoding.TextUnmarshaler` hooks, pointer targets,
  fixed-size arrays. `[derived]`
- Exclusions are refusals: a document key naming a `toml:"-"` field or an
  unexported field is a hard error, exactly like any unknown key. Exclusion
  means "this name is not part of the document universe", never "present but
  ignored" — an exemption spelling would be the opt-out this design bans.
  `[approved]`
- Unknown struct-tag options are hard errors at field-mapping construction
  (with the real Marshal deleted, `omitempty` no longer means anything; a
  meaningless tag option must fail loudly, not silently no-op). `[derived]`
- A node-level decode exists: decode any subtree into a struct. The partial-
  decode idiom is extract-the-subtable-then-decode-it-completely; there is no
  lenient spelling. `[approved]`
- An unknown table produces one error naming the table and listing the keys
  it contained — one actionable error carrying the full inventory, not one
  error per contained key. `[approved]`
- Numeric widening: an integer value decodes into a float target if and only
  if the conversion is exact; an integer whose magnitude exceeds what float64
  represents exactly is a hard error. This is the only coercion anywhere, the
  principle being "coercion only when provably value-preserving". It applies
  uniformly: engine, struct front end, and every accessor family. The rule is
  written in exactly one place (the shared conversion table, section 10).
  `[approved]`

## 3. Decode error reporting

- Strict decode collects every INDEPENDENT violation in document order and
  returns them as one error value: its `Error()` renders only the first
  diagnostic (so single-error call sites read like a single error), and the
  full list is reachable via `errors.As`. "Independent" means: after a
  violation, validation continues across sibling keys and tables but never
  descends below an errored node, so a broken table cannot produce cascading
  nonsense. This replaced an earlier first-error-only ruling after two facts
  emerged: the cascade objection has this structural fix, and the two largest
  consumers had each hand-built multi-diagnostic reporting — demonstrated
  demand. `[approved]`
- Parsing remains first-error-only (a parse cannot meaningfully continue past
  a syntax error). The asymmetry is deliberate and documented. `[approved]`

## 4. The unified diagnostic contract

- Parse, decode, and edit errors share one structured shape: document path,
  position, span, message, and an optional filename. Matching via
  `errors.Is` (kind sentinels) and `errors.As` (structured access) is the
  documented compatibility contract; the type implements `Unwrap` wherever it
  wraps. `[approved]`
- `Position` carries line, column, and byte offset. The diagnostic embeds a
  `Position` rather than parallel scalar fields, so the position triple is
  declared once. `[derived]`
- The current `ParseError` identifier does not survive: the unified type gets
  a new name so that every consumer touch-point fails to compile and gets
  visited during the sweep (one consumer uses a bare type assertion that
  would otherwise silently stop matching). `[derived]`
- Every parser-stage error site is routed through the single error
  constructor and carries offset and snippet (today only lexer-stage errors
  fill them). The self-verifying spelling of the obligation: no site
  constructs the diagnostic literal directly; all go through the constructor.
  `[derived]`
- The document node's span already exists; the array-of-tables collection
  span (first entry's header start to last entry's end) is synthesized by the
  read-layer, which owns the collection concept. `[derived]`
- Default path rendering in messages uses the library's own path syntax
  (dotted keys, bracket indices, quoted-when-not-bare segments) — the same
  strings the path API accepts, so an error's path can be pasted back into
  `Get`/`Set`. Errors are data; consumers with their own diagnostic
  vocabularies render the structured fields themselves. `[approved]`

## 5. The logical read-layer

- A designed, read-only logical-tree API sits alongside the concrete syntax
  AST: the primary way to READ a document. Reads go through the layer;
  mutation goes through paths. This sentence is the contract and belongs in
  the package documentation. `[approved]`
- Its semantics are taken from the proven fold that two consumers implemented
  independently: first-appearance key order across all binding forms;
  implicit records created by dotted keys and deep headers; dotted-key
  expansion; array-of-tables entries with last-entry addressing for
  sub-table headers; inline tables folding into ordinary records;
  first-header-wins span anchoring; a per-key span on every entry.
  `[approved]`
- The two silent-tolerance points of the reference fold (a default branch
  that misclassifies unknown nodes as strings; a precedence order that would
  silently drop a colliding sub-record) become hard internal errors.
  `[derived]`
- Implementation directive: evaluate building the layer by retaining and
  extending the parser's existing definition tracker (which already computes
  implicit records, dotted expansion, and first-header-wins, then discards
  the tree) rather than folding a second tree after parse; the descriptor
  engine then co-traverses that structure instead of walking independently.
  If evaluation finds the coupling worse than a separate fold, fall back to
  the separate fold — but the folding semantics exist once either way.
  `[derived]`
- The layer is built lazily: constructed on first read-layer access,
  invalidated by a document-level generation counter that every setter bumps.
  Until encapsulation makes the counter airtight, the layer is built eagerly;
  the switch to lazy happens when setters become the only write path. The
  eager-then-lazy staging is deliberate and temporary. `[derived]`
- Path resolution is re-based on the layer. The internal virtual-view types
  it replaces are retired from resolution immediately and deleted once their
  last dependents (iteration, merge) are ported; the read behavior of
  `Get`/`Resolve` and the typed getters is preserved except for the
  enumerated widening flips. `[derived]`
- Path helpers are exported: parse, join, and quote, as the single authority
  for path text. The renderer-side bare-key predicate remains a separate,
  documented TOML-syntax rule (it governs how keys are written in TOML, not
  how paths are spelled). Diagnostics, defaults-seeding, and every other
  path-producing feature call the exported helpers. `[derived]`

## 6. The node model

- The `Node` interface stays the universal handle (type, span, raw, comments)
  so traversal callbacks and cursor navigation keep their signatures. Value
  access moves to a `Scalar` sub-interface carrying `Value()` and the typed
  accessors; container nodes expose ordered entries instead. The wrong
  question ("give me the any-value of an array") stops compiling — this
  closed a live consumer bug caused by exactly that call. `[approved]`
- Full encapsulation: node struct fields are unexported; reads go through
  accessor methods; writes go only through setters that mark dirtiness and
  distinguish value mutation (which invalidates the stored lexeme) from
  trivia mutation (which preserves it). `[approved]`
- Lexeme invalidation is structural, not disciplinary: each scalar's payload
  and its raw lexeme live in one small unexported struct whose only mutator
  clears the lexeme — writing the value without clearing the lexeme is
  unrepresentable, in-package included. `[approved]`
- Setters propagate dirtiness upward (parent references maintained by the
  structural operations), so the per-render recursive subtree-dirtiness walk
  is replaced by a constant-time check. A deep-nesting benchmark guards the
  property. `[derived]`
- The document type is renamed from `DocumentNode` to `Document` (the rename
  set is completed by the naming review in this record's review round). The
  rename happens in the same early pass as the interface split and the
  unexporting, so all subsequent work is written against the final shape
  once. `[approved]`
- The exported token vocabulary (token types and the token struct) is
  unexported: it is reachable from no exported API and contradicts the
  parser-is-internal decision. A future streaming parser designs its own
  token surface additively if ever built. `[approved]`

## 7. Fidelity and rendering

- The serializer's dirty rendering prefers still-valid raw lexemes: a node
  whose value has not been mutated re-emits its original bytes even when its
  container or trivia is dirty. Header key raw parts (computed by the parser
  today and discarded) are wired into both header node types and used by the
  header renderers; inline-table key rendering prefers raw parts. Editing one
  element of a container leaves every sibling's spelling byte-identical.
  `[approved]`
- Constructed and value-mutated scalars render canonically: lowercase hex in
  unicode escapes, shortest-round-trip float form always float-marked, `nan`
  as the only special-float spelling the library ever writes. The correct
  literal renderers (string quoting, key quoting, float formatting) are
  exported for consumers, replacing two independently written and incorrect
  reimplementations in the fleet. `[approved]`
- All TOML-valid spellings remain valid INPUT (`+nan`, `-nan`, any quoting,
  any integer base — the compliance suite is non-negotiable) and are
  preserved byte-for-byte while untouched. Canonicalization applies only to
  what the library writes. `[deliberate]`
- `Format()` preserves the user's blank-line grouping: runs of blank lines
  collapse to one; no setting destroys them. The table-blank-line option is
  insertion-only — it controls whether a missing blank line before a table
  header is inserted, never whether existing ones are removed. `[approved]`
- Whole-document canonical form (one true table style, key order,
  indentation) is out of scope: it belongs to a document-level canonical-form
  specification whose authorship is filed as a deliverable in the schema
  toolchain project. `[approved]`

## 8. The Set-equality contract

- `Set(path, value)` is a no-op if and only if the bytes it would write are
  exactly the bytes already stored for that value. One rule, no special
  cases: it covers NaN spellings (setting NaN over `nan` is a no-op; over
  `+nan` it writes `nan`), signed zeros, integer-vs-float, date-time offsets,
  string quoting, and integer bases alike. Two consequences are accepted
  knowingly: an idempotent tool that Sets a value stored in a non-canonical
  spelling (octal, hex, underscores, exponent floats, literal quotes)
  normalizes that one value's spelling on first touch and is byte-stable
  thereafter; and a same-content Set over a literal-quoted string converts it
  to basic quoting. What you Set is what the file says. `[deliberate]`
- `Set` refuses non-canonical NaN input: a NaN with its sign bit set is a
  hard error, never silently normalized. The accepted NaN writes `nan`.
  `[deliberate]`
- A no-op `Set` never clears dirtiness from an earlier edit — otherwise
  saving could silently revert a previous, unsaved change. Covered by test.
  `[approved]`
- The whole contract lives in the exported doc comment of `Set`, not only in
  this record: it is user-visible behavior. `[derived]`

## 9. The write path

- Structural operations exist before encapsulation removes direct field
  access, and their scope is the demonstrated need: arbitrary permutation of
  a node's children (standalone comments traveling with their positions),
  key reordering/sorting within a table, array element append and remove at a
  path, and an ordered inline-table input type (the map input's forced
  alphabetization broke a consumer's ordering requirement). The heaviest
  consumer's reorder scenario is captured as an in-library acceptance test
  BEFORE the operation signatures freeze. `[approved]`
- `MergeDefaults` is redesigned to the shape a consumer independently proved:
  flat path-keyed defaults, seeding only missing paths, returning the list of
  paths it added. It keeps delegating to the create-if-missing set operation
  internally. `[approved]`
- The comment API's public spelling is path-based (`SetComment(path, text)`,
  `SetLeadingComments(path, texts)`); the per-node setters become unexported,
  resolving a method-shadowing collision on the document type. Normalized
  comment-text getters (content without the `#` and surrounding whitespace)
  are added; raw trivia access remains. `[approved]`
- `ParseFile` (filename flows into the diagnostic contract) and an atomic
  validate-on-write `WriteFile` are added. `WriteFile`'s contract is the
  deterministic invariant set: the temp file is created in the destination's
  directory; the document must round-trip-validate before the rename; an
  injected write or rename failure leaves the destination bytes untouched
  and no temp file behind; file mode survives the replace. Kernel-level
  crash injection is not part of the suite. `[approved]`

## 10. Typed accessors

- One design across all read surfaces, built as three unexported stages —
  navigate (its only failure: absent/bad path/wrong container), type-check
  (only failure: wrong node kind), convert (only failure: inexact) — so the
  failure kinds are produced by different functions and cannot be conflated.
  The conversion table behind the convert stage is the single place the
  widening rule is written, and it is shared with the decode engine.
  `[approved]`
- One exported access-error type on the unified diagnostic contract, its kind
  set by whichever stage refused, carrying path, span, expected/got, and the
  offending value. Kind sentinels work with `errors.Is`; structured access
  with `errors.As`. If an allocation-free status is ever wanted, it must be
  this type's kind, never a parallel enum. `[approved]`
- Ergonomics: path-level and node-level accessors return `(T, error)`;
  Cursor terminals also return `(T, error)` (one vocabulary), with the
  Cursor's `Err()` retained only for chains ending in navigation. A comma-ok
  existence surface (`Lookup`/`Has`) keeps the optional-with-default read
  allocation-free; an absent key through a value accessor reports the
  not-found kind through the same error type. `[approved]`

## 11. Public surface end-states

- Kept and re-based on the read-layer: the fluent Cursor (thin sugar over
  the shared resolution engine), Walk (traversal over the layer's ordered
  enumeration), Merge, and Diff. Diff compares logical structure — two
  documents expressing the same structure in different TOML spellings compare
  equal; before porting, the concrete case that today compares unequal is
  named in a test, or the port is reclassified as verification of existing
  behavior. `[approved]`
- Deleted: the path-based `Items`/`Len` on the document (superseded by the
  layer's enumeration; the Cursor's own iteration survives), and the map-only
  `Marshal` (zero users, forced alphabetization contradicted the one
  plausible consumer; a real struct-to-TOML Marshal is deferred-work, not
  this campaign). `[approved]`
- Small correctness fixes riding the wave: overflow checking on unsigned
  integer conversion in the value-to-node path; `NewTable` refusing a name
  collision with an existing array-of-tables (the current behavior can
  produce output the library's own parser rejects); removal of the unused
  parent-dirty parameter once dirtiness propagation exists; the formatter's
  spurious blank line from skipped blank-line nodes (fixed inside the
  blank-line-preservation rework, with a pinning regression test).
  `[derived]`

## 12. Consumer end-states and sweep constraints

The migration follows the module graph. Constraints discovered and binding:

- Order: release this library; then migrate the CLI framework consumer and
  its conformance harness in one commit and RELEASE it (several repos
  require both modules — minimal version selection would otherwise compile
  the framework's unmigrated source against the new API inside their
  builds); migrate the repo whose Go workspace `use`s the framework checkout
  in the same working session (its tests go red the moment the framework's
  local checkout is migrated); every remaining consumer bumps BOTH module
  requirements in one commit. The workspace-using indirect dependent gets a
  `go work sync`. `[derived]`
- Each consumer's authoritative break list is produced mechanically before
  the release: at the end of the library work, every consumer is built
  against the pre-release tree in a scratch clone with a temporary replace
  directive (never committed), and the compiler output drives the sweep and
  the migration table in the release notes. `[derived]`
- Per-consumer rulings: the wavescript project deletes its hand-rolled
  strict-decode layer and its typed-read, enumeration, comment, value-
  rendering, and write-cycle helpers. The CLI framework builds runtime
  descriptors from its registries, keeps its cross-language message
  vocabularies (rendering engine error data), migrates its bare error type
  assertion, adopts `ParseFile`, keeps its effects-handle writes; its
  conditional unknown-key rejection is investigated and made unconditional
  across all of its implementations in its own campaign (a todo is filed
  there, noting the check sits in structurally different places per
  implementation). pgdesign gets the minimal-correct migration: delete the
  excluded-field workaround and the two-pass hand decode (decoding its
  dynamic sections directly into map fields, which the map-of-struct fix
  makes work), plus the mechanical conversions; its generated validator
  stack stays untouched and remains the shape authority (the handover
  question is a later, separate campaign). howmuchleft keeps its startup
  auto-create of the config file; a present-but-invalid file becomes a hard
  positioned error instead of a silent fall-back to defaults; it adopts the
  library defaults-seeder, plain-value fields, the container-entries fix for
  its always-nil profile reading (a named consumer-facing defect in the
  changelog), and the preserving serializer for write-back. safegit adopts
  required tags for presence and keeps pointer fields only for its
  mutual-exclusivity domain rule. dirstat adopts a descriptor for its flat
  schema and keeps choices/uniqueness/bespoke messages app-side. strictspec
  keeps its own format-neutral document model; its TOML front end shrinks to
  a thin conversion from the read-layer, and its byte-offset derivation and
  span-synthesis workarounds are deleted. `[approved; the howmuchleft
  failure policy and pgdesign shape were re-ruled on corrected premises]`

## 13. Release and process

- One release at the end of the library work, a breaking minor on 0.x, with
  changelog entries recorded per work item as it is committed (never batched
  at release time — the per-entry commit limits refuse large ranges), and a
  migration table in the release notes derived from an API-diff between the
  last released version and the final tree. The consumer sweep follows the
  release. `[approved]`
- Ordering principle for the implementation: semantic changes follow
  dependency order (diagnostics before the engine that emits them; the
  read-layer before everything that sits on it; structural operations before
  the encapsulation that removes their workaround; setter discipline before
  lexeme-dependent rendering); mechanical changes (identifier renames, field
  unexporting, token unexporting) collapse into the fewest possible scripted
  passes — one early node-model pass and one late identifier sweep — each
  with a dry run, asserted per-file occurrence counts, and a reviewed diff.
  `[derived]`
- Repository hygiene precedes the wave: a gofmt sweep as its own commit, CI
  steps failing on unformatted code and on staticcheck findings, the go
  directive relaxed to the minor version, and a second Go version in the CI
  matrix. The CI job name stays `test` — the publish workflow's check
  matcher keys on it. `[derived]`
- Test infrastructure: the compliance-suite skip list is derived from the
  corpus's own version-filtered listing API instead of a hand-copied
  duplicate; corpus case counts are asserted in the test (the single
  authority — documentation describes the corpora without numbers); the two
  fuzz targets share one committed seed list, with only minimized crashers
  added to it; the read-error signal in the corpus walk survives the removal
  of the misleading counters. `[derived]`
- The strongest corpus tests: with lexeme preference on, mark every node
  dirty and require byte-identical output over the full valid corpus (the
  fidelity property); with lexeme preference off through an internal
  test-only switch, require semantic equality after re-parse plus an
  idempotent second render (the renderer-correctness property); and a
  mutation pass that sets every scalar to a NEW value through public setters
  and asserts the re-parsed values match (the stale-lexeme corruption
  property that the mark-dirty test cannot see). `[derived]`
- Red-green discipline for the decode bugs fixed by the engine replacement
  (map-of-struct path truncation; array-of-tables under a plain table
  silently dropped; the map-element reflect panic): tests phrased entirely
  on the public surface, committed before the engine work with their red
  failure output pasted into the commit message (the old decoder will not
  exist to reproduce it), turning green under the engine. The audit test
  that only compared two decode entry points against each other is
  strengthened to assert actual values. `[derived]`

## 14. Deferred and rejected

A deferred-work todo is filed at the end of the campaign (shown to the user
before filing) carrying: the real struct-to-TOML Marshal (with the note that
pgdesign's exporter could shrink onto it); the full structural-manipulation
suite (node moves across containers, positional insertion, table/inline
conversion); the splice API; the enum-in-descriptor extension as an open
decision with pros and cons; and the presence-reporting API recorded as
rejected with its rationale (required-key support plus the defaults-seeder
absorbed the need; the remaining consumer case is domain logic on a data
shape where pointer fields are the better instrument). A kernel-level
crash-injection harness for `WriteFile` and a struct-size budget test were
considered and rejected (flaky in CI; a hand-maintained number that breaks on
legitimate change).

## 15. Open in this record's review round

- The rename set beyond `DocumentNode` → `Document`, and the names of the
  read-layer types and the access-error type. To be proposed with objective
  reasoning during this record's review and settled before implementation
  begins.
