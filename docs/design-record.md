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

- **Mechanism per assertion.** Wherever this record or the implementation
  claims a property, the claim must name the mechanism that fails when the
  property does not hold (a compile error, a test, a count assertion, a
  derived list). A property enforced only by prose is not enforced.
- **Every test names its failure mode.** Each new test states, in a one-line
  comment, what change would make it fail. A test that cannot answer that is
  measuring nothing.

Vocabulary used throughout, defined once:

- **Trivia** — the non-semantic bytes attached to a node: leading whitespace,
  leading comments, the inline comment, trailing newline.
- **Dirty** — a node marked as edited; the serializer re-renders dirty nodes
  and splices the original bytes of clean ones.
- **Lexeme** — the exact source bytes of a value as written (`0x2A`, `1e3`,
  `+nan`), stored on the node at parse time.
- **The read-layer** (also "the fold") — the ordered logical-tree view of a
  document defined in the read-layer section below.
- **The wave** — the library-side implementation work under this record,
  ending in one release.
- **The sweep** — the post-release migration of the consumer repositories.
- **Consumer** — a repository with at least one Go file importing this
  package. Repositories that merely require the module in go.mod without
  importing it need nothing beyond a `go mod tidy` when their direct
  dependencies move.

## 1. Identity and constraints

- The library is developed fleet-first: its consumers are the local projects
  that depend on it, and their demonstrated needs drive the API. `[approved]`
- No decision may be one that a later publicization of the library would force
  us to walk back. Publicity itself is unscheduled and tracked in the todo
  directory. `[deliberate]`
- The module path (`github.com/smm-h/go-toml-edit`) and package name
  (`tomledit`) both stay. The README's import example is changed to the named
  form (`tomledit "github.com/smm-h/go-toml-edit"`) so readers meet the
  package name at first contact; the edit goes in `docs/_README.md`, from
  which the read-only `README.md` is generated. `[approved]`
- The library is a single Go package with no internal subpackages
  (pre-existing convention, reaffirmed). Every encapsulation claim below is
  therefore about what the type system makes representable, not about package
  boundaries.
- The BurntSushi/toml and toml-test dependencies stay in the main module; the
  library advertises "no runtime dependencies" (mechanism: a test asserting
  the non-test import graph pulls in no external module). Moving them to a
  nested module was rejected because it would silently remove the compliance
  suite from a root `go test ./...`, weakening the release's most important
  check for a cosmetic gain. `[approved]`
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
  shape described as data). The reflection-based struct `Unmarshal` is a
  front end that derives a descriptor from the struct and runs the same
  engine. Mechanism: a test decodes one document through the struct front end
  and through an equivalent hand-built descriptor and asserts identical
  diagnostics (path, kind, order). `[approved]`
- The descriptor surface is exported and hand-constructible without
  reflection: consumers with runtime-known schemas build descriptors from
  their own registries. Mechanism: a test in which a descriptor built by hand
  (no reflection) validates a document. `[approved]`
- Descriptor expressiveness includes: scalar kinds (each date-time flavor
  kept distinct), nested tables, arrays with element descriptors, arrays of
  tables, required markers, dynamic keys (arbitrary key names with one
  uniform value descriptor), and an explicit any-value element. The design
  defects of the reference implementation (wavescript's internal
  `strictdecode` package) are not reproduced: its asymmetric nil semantics —
  where a nil array-element descriptor meant "unchecked" but a nil table
  descriptor meant "no keys allowed" — are replaced by explicit spellings for
  both intents. `[derived]`
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
  (with the map-only Marshal deleted, `omitempty` no longer means anything; a
  meaningless tag option must fail loudly, not silently no-op). Mechanism: a
  test per rejected option, each asserting the error names the offending
  option. `[derived]`
- A node-level decode exists: decode any subtree into a struct. The partial-
  decode idiom is extract-the-subtable-then-decode-it-completely; there is no
  lenient spelling. `[approved]`
- An unknown table (or an unknown array-of-tables) produces one error naming
  it and listing its direct child keys — one actionable error carrying the
  immediate inventory, not one error per contained key and not a recursive
  listing. `[approved; the direct-children depth and the array-of-tables
  extension are derived]`
- Numeric widening: an integer value decodes into a float target if and only
  if the conversion is exact; an integer whose magnitude exceeds what float64
  represents exactly is a hard error. The principle: coercion only when
  provably value-preserving; nothing else ever coerces. It applies uniformly
  to the engine, the struct front end, and every accessor family, because the
  rule is written in one place — the shared conversion table of the accessor
  section. Mechanism: a table-driven test runs every (source kind, target
  kind, boundary value) row through the decode engine and each accessor
  family and asserts identical results. `[approved]`

## 3. Decode error reporting

- Strict decode collects every INDEPENDENT violation in document order and
  returns them as one error value whose `Error()` renders only the first
  diagnostic (so single-error call sites read like a single error), with the
  full list reachable via `errors.As`. "Independent" means: after a
  violation, validation continues across sibling keys and tables but never
  descends below an errored node, so a broken table cannot produce cascading
  nonsense. Mechanism for the no-descent rule: a fixture with a violation on
  a table that itself contains further violations, asserting the exact
  diagnostic list and the absence of the buried ones. `[approved — see the
  open-items section: the evidence base for this ruling was corrected during
  review and the ruling awaits re-confirmation]`
- The aggregate error's contract: it implements `Unwrap() []error` returning
  the diagnostics in document order; `errors.As` with a single-diagnostic
  target yields the first diagnostic (standard traversal order); `errors.As`
  with the aggregate target yields the whole list; `errors.Is` against a kind
  sentinel matches if any contained diagnostic carries that kind. `[derived]`
- Parsing remains first-error-only (a parse cannot meaningfully continue past
  a syntax error). The asymmetry is deliberate and documented. `[approved]`

## 4. The unified diagnostic contract

- Parse, decode, edit, and access errors share one structured shape: document
  path, position, span, message, and an optional filename. Matching via
  `errors.Is` (kind sentinels) and `errors.As` (structured access) is the
  documented compatibility contract; the type implements `Unwrap` wherever it
  wraps. `[approved]`
- `Position` carries line, column, and byte offset. This is a change to the
  position type itself, so every node span carries the offset: the token
  gains an offset, position advancement carries it, and every span
  construction site populates it. (The strictspec consumer's offset-derivation
  workaround reconstructs offsets for node spans, so only a position-level
  offset discharges that deletion.) The diagnostic embeds a `Position` rather
  than parallel scalar fields, so the position triple is declared once.
  `[derived]`
- The current `ParseError` identifier does not survive: the unified type gets
  a new name so that every consumer touch-point fails to compile and gets
  visited during the sweep (one consumer uses a bare type assertion that
  would otherwise silently stop matching). `[derived]`
- Every parser-stage error site is routed through the single error
  constructor and carries offset and snippet (before this wave, only
  lexer-stage errors fill them). Mechanism: a test walks this package's own
  source and fails on any composite literal of the diagnostic type outside
  the constructor's file. `[derived]`
- The array-of-tables collection span is synthesized by the read-layer, which
  owns the collection concept: from the first entry's header start to the
  last entry's content end (the end of that entry's last child, not its
  header). The concrete array-table node's own span (its header brackets) is
  unchanged. `[derived]`
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
- Its semantics are taken from the fold that two consumers implemented
  independently (strictspec's internal `tomldoc` converter and wavescript's
  internal `strictdecode` document fold): first-appearance key order across
  all binding forms; implicit records created by dotted keys and deep
  headers; dotted-key expansion; array-of-tables entries with last-entry
  addressing for sub-table headers; inline tables folding into ordinary
  records; first-header-wins span anchoring; a per-key span on every entry.
  Mechanism: one exported-behavior suite over the fold's semantics,
  including a port of the reference fold's test expectations. `[approved]`
- Every silent-tolerance point of the reference fold becomes a hard internal
  error: the value-conversion default branch that misclassifies an unhandled
  node as a string, the finalize precedence that would silently drop a
  colliding sub-record, and the finalize default branch that fabricates an
  empty record for an unassigned slot. `[derived]`
- Implementation directive: evaluate building the layer by retaining and
  extending the parser's existing definition tracker (which already computes
  implicit records, dotted expansion, and first-header-wins, then discards
  the tree) rather than folding a second tree after parse; the descriptor
  engine then co-traverses that structure instead of walking independently.
  If evaluation finds the coupling worse than a separate fold, fall back to
  the separate fold — the folding semantics exist once either way (mechanism:
  the single exported-behavior suite above is the only statement of those
  semantics; a second fold implementation would have to pass it verbatim).
  `[derived]`
- The layer is built lazily: constructed on first read-layer access,
  invalidated by a document-level generation counter. The counter is bumped
  by the same single unexported mutator that the node-model section mandates
  for value writes — a setter that forgets to bump is unrepresentable, not
  merely forbidden. Until encapsulation makes that airtight, the layer is
  built eagerly; the switch to lazy happens when setters become the only
  write path, and lands with a test asserting `Parse` alone does not build
  the layer (an internal build counter), so a leftover eager build cannot
  linger invisibly. `[derived]`
- Path resolution is re-based on the layer. The internal virtual-view types
  it replaces are retired from resolution immediately and deleted once their
  last dependents (iteration, merge) are ported. The read behavior of
  `Get`/`Resolve` and the typed getters is preserved, except for the widening
  flips: the float getters (path-level, node-level, and the Cursor's float
  terminal), which before this wave refuse an integer node, now accept an
  exactly-representable integer. No other getter changes its answer.
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
  accessors. Container node types — interface AND concrete types — lose
  `Value()` entirely and expose ordered entries instead; keeping the concrete
  method would leave the wrong question compilable on any concretely-typed
  variable. This closes a live consumer bug: howmuchleft's profile reading
  asserts `[]interface{}` on an array node's `Value()` (which returns the
  node slice), so the read has always silently returned nil. `[approved; the
  concrete-type removal is derived]`
- Full encapsulation: node struct fields are unexported; reads go through
  accessor methods; writes go only through setters that mark dirtiness and
  distinguish value mutation (which invalidates the stored lexeme) from
  trivia mutation (which preserves it). `[approved]`
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
- The document type is renamed from `DocumentNode` to `Document`. The rest of
  the rename set is settled in the open-items section before implementation
  begins. Sequencing of the rename relative to the other mechanical passes
  is stated once, in the release-and-process section. `[approved]`
- The exported token vocabulary (token types and the token struct) is
  unexported: it is reachable from no exported API and contradicts the
  parser-is-internal decision. A future streaming parser designs its own
  token surface additively if ever built. `[approved]`

## 7. Fidelity and rendering

- The serializer's dirty rendering prefers still-valid raw lexemes: a node
  whose value has not been mutated re-emits its original bytes even when its
  container or trivia is dirty. Header key raw parts (computed by the parser
  today and discarded) are wired into both header node types and used by the
  header renderers; inline-table key rendering prefers raw parts. Mechanism:
  the corpus mutation pass of the release-and-process section asserts, for
  every container kind, that editing one element leaves every sibling's
  bytes unchanged. `[approved]`
- Constructed and value-mutated scalars render canonically: lowercase hex in
  unicode escapes; the float form follows the schema toolchain's
  value-rendering appendix (strictspec's `spec/appendix-rendering.md`):
  shortest round-tripping digits, a float marker always present, and the
  appendix's rule for when lowercase-e exponent notation is used — the
  appendix is the authority and its rule is not restated here. Special
  floats render as `nan`, `inf`, and `-inf`; the library never writes `+nan`,
  `-nan`, or `+inf`. Mechanism: a test enumerates the renderer's output for
  each special-float value. `[approved; the infinity spellings are derived]`
- The correct literal renderers (string quoting, key quoting, float
  formatting) are exported for consumers. This replaces the hand-written
  TOML renderer in wavescript (a Go-escaping string quoter and a float
  format that drops the float marker, both of which produce invalid or
  wrongly-typed TOML). A consumer renderer implementing another project's
  own documented rendering rule — strictcli's cross-language canonical float,
  strictspec's format-neutral value renderer — is not a replacement target.
  `[approved; the non-target carve-out corrects an earlier wrong claim]`
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
  project. `[approved]`

## 8. The Set-equality contract

- `Set(path, value)` — and every value-writing entry point sharing its path,
  including the create-if-missing variant and the defaults-seeder — is a
  no-op if and only if the bytes it would write are exactly the bytes already
  stored for that value. When the target carries no stored lexeme
  (constructed, or previously value-mutated), the comparison is against its
  canonical rendering. Equality is decided before any mutator runs: a no-op
  never touches the payload, so a stored lexeme survives it. One rule, no
  special cases: it covers NaN spellings (setting NaN over `nan` is a no-op;
  over `+nan` it writes `nan`), infinities (`Set` of positive infinity over
  `inf` is a no-op; over `+inf` it writes `inf`), signed zeros,
  integer-vs-float, date-time offsets, string quoting, and integer bases
  alike. The accepted consequences: an idempotent tool that Sets a value
  stored in a non-canonical spelling (octal, hex, underscores, exponent
  floats, literal quotes, `+inf`) normalizes that one value's spelling on
  first touch and is byte-stable thereafter; and a same-content Set over a
  literal-quoted string converts it to basic quoting. What you Set is what
  the file says. Mechanism: a corpus pass Sets every scalar to its own
  decoded value and asserts byte-identical output, paired with targeted
  tests Setting differing values and asserting canonical bytes.
  `[deliberate]`
- `Set` refuses non-canonical NaN input: a NaN with its sign bit set is a
  hard error, never silently normalized. The accepted NaN writes `nan`.
  (Infinities need no input rule: a Go infinity's sign is its value.)
  `[deliberate]`
- A no-op `Set` never clears dirtiness from an earlier edit — otherwise
  saving could silently revert a previous, unsaved change. Covered by test.
  `[approved]`
- The whole contract lives in the exported doc comments of the value-writing
  entry points, not only in this record: it is user-visible behavior.
  `[derived]`

## 9. The write path

- Structural operations exist before encapsulation removes direct field
  access, and their scope is the demonstrated need: arbitrary permutation of
  a node's children, key reordering/sorting within a table, array element
  append and remove at a path, and an ordered inline-table input type. The
  permutation is total — a bijection on the child indices; a duplicate,
  missing, or out-of-range index or a length mismatch is a hard error naming
  the offending index, and nothing is reordered. Standalone comments are
  children and move with their assigned positions. The heaviest consumer
  reorder scenario (pgdesign's document and table reordering) is captured as
  an in-library acceptance test BEFORE the operation signatures freeze.
  `[approved; the totality rule is derived]`
- The ordered inline-table input is a slice of key/value pairs. A duplicate
  key is a hard error (consistent with the no-silent-tolerance stance); key
  syntax is validated at the Set call (the slice itself is plain data). It
  serves inline-table construction; ordinary tables are built through the
  table-creation and set operations. `[derived]`
- The defaults-seeder is redesigned to the shape a consumer independently
  proved (howmuchleft's config seeding): flat path-keyed defaults, seeding
  only missing paths, returning the list of paths it added. It keeps
  delegating to the create-if-missing set operation internally. `[approved]`
- The comment API's public spelling is path-based (`SetComment(path, text)`,
  `SetLeadingComments(path, texts)`); the per-node setters become unexported,
  resolving a method-shadowing collision on the document type. Normalized
  comment-text getters (content without the `#` and surrounding whitespace)
  are added; raw trivia access remains. `[approved]`
- `ParseFile` (filename flows into the diagnostic contract) and an atomic
  validate-on-write `WriteFile` are added. `WriteFile`'s contract is the
  deterministic invariant set, each item covered by test: the temp file is
  created in the destination's directory; the document must
  round-trip-validate before the rename; an injected write or rename failure
  leaves the destination bytes untouched and no temp file behind; file mode
  survives the replace. A round-trip-validation failure is a diagnostic of
  its own kind, carrying the filename and the byte offset of the first
  divergence — it is not an access error and not a parse error. Kernel-level
  crash injection is not part of the suite. `[approved; the failure kind is
  derived]`

## 10. Typed accessors

- One design across all read surfaces, built as unexported stages — navigate
  (its only failure: absent, bad path, or wrong container), type-check (only
  failure: wrong node kind), convert (only failure: inexact) — so the
  failure kinds are produced by different functions and cannot be conflated.
  Mechanism: a test per stage asserts the kind sentinel, including that a
  read on an absent path reports not-found rather than wrong-type. The
  conversion table behind the convert stage is the single place the widening
  rule is written, and it is shared with the decode engine (mechanism: the
  cross-family table test of the strict-decoding section). `[approved]`
- One exported access-error kind family on the unified diagnostic contract,
  the kind set by whichever stage refused, carrying path, span,
  expected/got, and the offending value. Kind sentinels work with
  `errors.Is`; structured access with `errors.As`. If an allocation-free
  status is ever wanted, it must be this contract's kind, never a parallel
  enum. `[approved]`
- Ergonomics: path-level and node-level accessors return `(T, error)`;
  Cursor terminals also return `(T, error)` (one vocabulary), with the
  Cursor's `Err()` retained only for chains ending in navigation. A comma-ok
  existence surface (`Lookup`/`Has`) keeps the optional-with-default read
  allocation-free (mechanism: an allocations-per-run assertion of zero on
  the comma-ok surface); an absent key through a value accessor reports the
  not-found kind through the same error contract. `[approved]`

## 11. Public surface end-states

- Kept and re-based on the read-layer: the fluent Cursor (thin sugar over
  the shared resolution engine), Walk (traversal over the layer's ordered
  enumeration), Merge, and Diff. Diff compares logical structure — two
  documents expressing the same structure in different TOML spellings compare
  equal; before porting, the concrete case that today compares unequal is
  named in a test, or the port is reclassified as verification of existing
  behavior. `[approved]`
- Deleted: the path-based `Items`/`Len` on the document (superseded by the
  layer's enumeration; the Cursor's own iteration survives), and the
  map-only `Marshal` (no callers anywhere in the fleet; its forced
  alphabetization contradicted the one plausible consumer; a real
  struct-to-TOML Marshal is deferred-work, not this campaign). `[approved]`
- Mechanism for the whole surface: a committed exported-API snapshot test —
  the same artifact the release's migration table is generated from — so an
  un-performed deletion and an unruled addition both fail. `[derived]`
- Small correctness fixes riding the wave: overflow checking on unsigned
  integer conversion in the value-to-node path; `NewTable` refusing a name
  collision with an existing array-of-tables (the current behavior can
  produce output the library's own parser rejects); removal of the unused
  parent-dirty parameter once dirtiness propagation exists; the formatter's
  spurious blank line from skipped blank-line nodes (fixed inside the
  blank-line-preservation rework, with a pinning regression test).
  `[derived]`

## 12. Consumer end-states and sweep constraints

The sweep follows the module graph. Constraints discovered and binding:

- Order: release this library; then migrate strictcli and its conformance
  harness in one commit and RELEASE strictcli (several repos require both
  modules — minimal version selection would otherwise compile strictcli's
  unmigrated source against the new API inside their builds); migrate
  safegit in the same working session (its Go workspace `use`s the strictcli
  checkout, so its tests go red the moment that checkout is migrated); every
  remaining consumer bumps BOTH module requirements in one commit. saferm
  (whose workspace also uses the strictcli checkout, without importing this
  package) gets a `go work sync`. `[derived]`
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
  `ParseFile`, keeps its dry-run effects-handle writes; its conditional
  unknown-key rejection is investigated and made unconditional across all of
  its implementations in its own campaign (a todo is filed there, noting the
  check sits in structurally different places per implementation). pgdesign:
  the minimal-correct migration — delete the excluded-field workaround and
  the two-pass hand decode (decoding its dynamic sections directly into map
  fields, which the map-of-struct fix makes work), plus the mechanical
  conversions; its generated validator stack stays untouched and remains the
  shape authority (the handover question is a later, separate campaign).
  howmuchleft: keeps its startup auto-create of the config file; a
  present-but-invalid file becomes a hard positioned error instead of a
  silent fall-back to defaults; adopts the library defaults-seeder,
  plain (non-pointer) config fields, the container-entries fix for its
  always-nil profile reading (a named consumer-facing defect in the
  changelog), and the preserving serializer for write-back. safegit: adopts
  required tags for presence and keeps pointer fields only for its
  mutual-exclusivity domain rule. dirstat: adopts a descriptor for its flat
  schema and keeps choices, uniqueness, and bespoke messages app-side.
  strictspec: keeps its own format-neutral document model; its TOML front
  end shrinks to a thin conversion from the read-layer, and its byte-offset
  derivation and span-synthesis workarounds are deleted. `[approved; the
  howmuchleft failure policy and the pgdesign shape were re-ruled on
  corrected premises]`

## 13. Release and process

- One release at the end of the library work, a breaking minor on 0.x, with
  changelog entries recorded per work item as it is committed (never batched
  at release time — the per-entry commit limits refuse large ranges), and a
  migration table in the release notes derived from an API-diff between the
  last released version and the final tree. The sweep follows the release.
  `[approved]`
- The implementation's total order, stated once (other sections point here):
  repository hygiene and test-infrastructure hardening; the mechanical
  `DocumentNode`-to-`Document` rename as its own scripted early pass (it has
  no semantic dependencies, and everything after it is written against the
  final name); the diagnostic contract; the read-layer and path re-basing;
  the decode engine (absorbing the decode-bug regression tests); the
  structural operations; the node-model pass (interface split, field
  unexporting, setter discipline); lexeme-dependent rendering and value
  canonicalization; the formatter's blank-line preservation; the ports
  (Cursor, Walk, Merge, Diff) and the deletions and the token unexporting as
  one late scripted identifier sweep; the documentation truth pass; the
  release. Semantic changes follow dependency order; mechanical changes
  collapse into as few scripted passes as that order allows, each with a dry
  run, asserted per-file occurrence counts, and a reviewed diff. `[derived]`
- Repository hygiene precedes the wave: a gofmt sweep as its own commit, CI
  steps failing on unformatted code and on staticcheck findings, the go
  directive relaxed to the minor version, and a CI matrix carrying more than
  one Go version. The CI job name stays `test` — the publish workflow's
  check matcher keys on it. `[derived]`
- Test infrastructure: the compliance-suite skip list is derived from the
  corpus's own version-filtered listing API instead of a hand-copied
  duplicate; corpus case counts are asserted in the test, which is the
  single authority (the counts are removed from the documentation — the
  carrier is the CLAUDE template, `docs/_CLAUDE.md`); every fuzz target
  reads one committed seed list, with only minimized crashers added to it;
  the read-error signal in the corpus walk survives the removal of the
  misleading counters. `[derived]`
- The corpus test battery: with lexeme preference on, mark every node dirty
  and require byte-identical output over the full valid corpus (the fidelity
  property); with lexeme preference off through an internal test-only
  switch, require semantic equality after re-parse plus an idempotent second
  render (the renderer-correctness property); the mutation pass that sets
  every scalar to a NEW value through public setters and asserts both that
  re-parsed values match and that sibling bytes are unchanged (the
  stale-lexeme and sibling-fidelity properties the mark-dirty test cannot
  see); and the Set-idempotence pass of the Set-equality section. `[derived]`
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
conversion); the splice API (replacing an exact byte span in place, a
capability one consumer implements format-neutrally for itself); the
enum-in-descriptor extension as an open decision with pros and cons; and the
presence-reporting API recorded as rejected with its rationale (required-key
support plus the defaults-seeder absorbed the need; the remaining consumer
case is domain logic on a data shape where pointer fields are the better
instrument). A kernel-level crash-injection harness for `WriteFile` and a
struct-size budget test were considered and rejected (flaky in CI; a
hand-maintained number that breaks on legitimate change).

## 15. Open items

- **Names not yet settled** (to be proposed with objective reasoning and
  settled before implementation begins): the unified diagnostic type and its
  aggregate; the read-layer types; the scalar interface; the descriptor
  types; the honest name for the key-rename operation; the redesigned
  defaults-seeder's name.
- **Decode error collection awaits re-confirmation.** The ruling in the
  decode-error section was approved partly on the claim that two consumers
  had hand-built multi-diagnostic DECODE reporting. Review corrected the
  evidence: wavescript's decode layer deliberately stops at the first
  violation, while the multi-diagnostic accumulation in pgdesign and
  strictcli sits in their post-decode domain-validation layers (which this
  design keeps app-side). The corrected case for collection: those app-side
  layers present whole-file error lists to users, and engine-collected
  diagnostics feed the same presentations; the one-pass fixing experience
  and the structural cascade fix stand on their own. The ruling stands
  unless the user, on the corrected evidence, prefers first-error-only.
