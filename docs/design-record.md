# Design record: the strictness-and-fidelity redesign

This file is the decision ledger and specification for the library-wide
redesign of go-toml-edit (strict-by-default decoding, a public logical
read-layer, full AST encapsulation, lexeme-preserving rendering, a unified
diagnostic contract, and the pruning/renaming of the public surface). Every
ruling below is binding on the implementation; where reality contradicts the
record mid-implementation, the discrepancy is escalated, not silently
resolved. Origin tags:

- `[deliberate]` — the user authored or explicitly chose this against or beyond
  a recommendation. Strongly held.
- `[approved]` — the user picked a recommended option. Weakly held by
  convention: walk back freely if evidence turns against it, and never cite it
  back to the user as their deliberate intent.
- `[derived]` — implementation-internal, resolved by the session under the
  constraints of the other rulings. Revisable without a user decision whenever
  the surrounding rulings permit.
- `[%% trust-adopted]` — the user accepted the session's recommendation
  without making the decision themselves (the `%%` answer). Freely
  reversible, and never to be cited back to the user as their deliberate
  intent; a future session may walk it back on its own reasoning.

Process rule: where the record claims a verifiable property it names the
test or check that fails when the property does not hold; claims of intent
or context carry no mechanism and are just prose. Every new test states, in
a one-line comment, what change would make it fail (auditor-enforced).

Vocabulary used throughout, defined once:

- **Trivia** — the non-semantic bytes attached to a node: leading whitespace,
  leading blank lines, leading comments, the gap between the construct and
  its inline comment, the inline comment, trailing newline. Blank lines gain
  an explicit representation in trivia as part of this wave, closing the
  pre-existing hole where a re-rendered node lost its blank-line separation.
  A run of blank lines is stored as its BYTES rather than as a count, so a
  blank line carrying whitespace of its own survives verbatim — a strict
  superset of what a count preserves. `[%% trust-adopted for the bytes form]`
- **Fragments** — the byte ranges a rendered construct decomposes into. For a
  key-value line: leading trivia, key bytes, separator bytes (whitespace and
  `=`), value bytes, the gap standing between the value and its inline
  comment, inline comment, trailing newline. For a table header:
  leading trivia, the per-part key bytes and the bracket/dot bytes around
  them, inline comment, trailing newline. For an array: open bracket,
  per-element fragments (each with its own leading/trailing trivia bytes),
  separator bytes, close bracket. For an inline table: open brace, per-pair
  fragments (key bytes, separator, value bytes), separator bytes, close
  brace. Fragment dirtiness recurses: a clean sub-fragment inside a dirty
  container still splices its original bytes. The value-to-inline-comment gap
  (`inlineGap`) is a fragment of its own, not part of the comment: a line
  re-rendering for some other reason writes the writer's spacing back
  exactly, including none at all in `x = 1# note`, while a comment written
  where there was none gets one space and removing a comment clears the gap,
  so no whitespace is left pointing at nothing. `[%% trust-adopted]`
- **Dirty** — a FRAGMENT whose original bytes are no longer valid because the
  corresponding content was mutated. The serializer re-renders dirty
  fragments and splices the original bytes of clean ones. (A node is "dirty"
  when any of its fragments is.)
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
- No decision may be one that a later publicization of the library would force
  us to walk back. Publicity itself is unscheduled and tracked in the todo
  directory. `[deliberate]`
- The module path (`github.com/smm-h/go-toml-edit`) and package name
  (`tomledit`) both stay. The README's import example is changed to the named
  form (`tomledit "github.com/smm-h/go-toml-edit"`), and the README gains a
  "no runtime dependencies" statement; both edits go in `docs/_README.md`,
  from which the read-only `README.md` is generated. `[approved]`
- The library is a single Go package with no internal subpackages
  (pre-existing convention, reaffirmed). Encapsulation claims below are about
  what the type system makes representable, not about package boundaries.
- The BurntSushi/toml and toml-test dependencies stay in the main module
  (mechanism for the no-runtime-dependencies claim: a test asserting the
  non-test import graph pulls in no external module). A nested test module
  was rejected: it would silently remove the compliance suite from a root
  `go test ./...`. `[approved]`
- TOML 1.0 only, full compliance with the official toml-test suite,
  non-negotiable (pre-existing convention; mechanism: the corpus tests of the
  release-and-process section).
- Pre-stable versioning: the redesign ships as one breaking minor release on
  0.x. No 1.x tag ever. No retraction of prior versions, no deprecate-first
  intermediate release. `[approved]`

## 2. Strict decoding: one engine

- Decoding is strict by default and strictness is the only mode. An unknown
  key, a wrong-typed value, or a missing required key is a hard error. There
  is no lenient mode, no option to skip checks. A map-typed or `any` target
  is a TOTAL descriptor — every key matches it by construction, so it
  reports no unknown keys; that is totality, not leniency. Mechanism: the
  strictness test set (the tests that previously asserted leniency, flipped,
  plus the exclusion/required/tag-option tests and a map-target test).
  `[deliberate; the map-totality statement is derived]`
- One validation engine: the core is descriptor-driven (the expected document
  shape described as data). The reflection-based struct `Unmarshal` is a
  front end that derives a descriptor from the struct and runs the same
  engine. The engine walks the read-layer, never the raw AST (this is what
  makes dotted keys, headers, and inline tables indistinguishable to
  validation). Mechanism: a test decodes one document through the struct
  front end and through an equivalent hand-built descriptor and asserts
  identical diagnostics — path, kind, and order — scoped to what both
  spellings can express: presence, kind, and required-ness (a hand-built
  descriptor does not express Go target widths, so width-dependent
  diagnostics are outside the comparison — the width exception is the ONLY
  exception; exact-only key matching in both schema sources means the
  comparison needs no case exception). `[approved]`
- The descriptor surface (shapes in the API-contracts section) is exported
  and hand-constructible without reflection: consumers with runtime-known
  schemas build descriptors from their own registries. Mechanism: a test in
  which a descriptor built by hand (no reflection) validates a document.
  `[approved]`
- Engine scope: presence and type and required-ness are the decoder's job;
  relationships between values (choices/enums, ranges, uniqueness, patterns,
  custom messages) are not. Consumers keep thin app-side validators for
  those, written against the read-layer so they retain source positions.
  `[deliberate]` (An "enum in the descriptor" extension goes into the
  deferred-work todo as an open decision, not a rejection.)
- Key matching is EXACT-ONLY, in both schema sources: a document key binds a
  struct field only when it equals the field's `toml` tag name or, absent a
  tag, the field name itself; a hand-built `Spec` matches only the exact
  strings in `Fields`. A key differing in case matches nothing and is
  therefore an unknown key. The existing decoder's exact-then-case-
  insensitive fallback does not survive: case-folding is a lenience policy,
  and this decoder has one mode. Mechanism: a test per schema source
  asserting a case-differing document key produces `KindUnknownKey`.
  `[deliberate]`
- Exclusions are refusals: a document key naming a `toml:"-"` field or an
  unexported field is a hard error, exactly like any unknown key. Exclusion
  means "this name is not part of the document universe", never "present but
  ignored". A `toml` tag on an UNEXPORTED field is inert: it is not a
  construction error, it names nothing, and its only observable consequence
  is that a document key spelling that name is an unknown key (reflection
  cannot write the field, so honoring the tag is impossible and refusing
  the struct would reject valid Go). Mechanism: a test per exclusion
  category, plus a test that a tagged unexported field constructs cleanly
  and rejects the key. `[approved]`
- Unknown struct-tag options are hard errors at field-mapping construction —
  a meaningless tag option must fail loudly, not silently no-op (tag options
  such as `omitempty` are read by nothing in the package). Mechanism: a test
  per rejected option, each asserting the error names the offending option.
  `[derived]`
- An unknown table (or an unknown array-of-tables) produces one error naming
  it and listing the direct child keys of the offending construct (for an
  array-of-tables, its first entry's direct keys) — one actionable error
  carrying the immediate inventory, not one error per contained key and not
  a recursive listing. Mechanism: a fixture test asserting the single error
  and its key list. `[approved; depth and the array-of-tables details are
  derived]`
- Custom-decode hook precedence is unchanged from the existing decoder: a
  target implementing `Unmarshaler` is handed the node before any table row
  applies; `encoding.TextUnmarshaler` applies to string nodes next; the
  conversion table governs everything else. `[derived]`
- The `Unmarshaler` hook applies WHEREVER the implementing type appears,
  including the top-level target of `Decode` and `DecodeNode` — the root
  target receives the document node (for `DecodeNode`, the node it was
  handed). There is no root exception: "the node the key binds" reads as
  "the node the target is bound to", and the root target is bound to the
  document. Mechanism: a test decoding into a root-level `Unmarshaler` and
  asserting the hook received the document node and no field-walk ran.
  `[approved]`
- A consumer hook's error (`Unmarshaler.UnmarshalTOML`, or
  `encoding.TextUnmarshaler` on a string value) is COLLECTED like any other
  violation: one diagnostic of kind `KindHookError` carrying the hook's own
  error as its wrapped error, positioned at the node the hook was handed.
  The walk continues across siblings and never descends below the errored
  node — the same independence rule as every other diagnostic, so one
  failing hook does not abort the rest of the document. Mechanism: a fixture
  with two failing hooks and a valid sibling asserting both diagnostics, the
  sibling's success, and `errors.Is` reaching the consumer's error through
  the aggregate. `[approved]`
- **The conversion table.** All value conversion — engine, struct front end,
  and every accessor family — is driven by one table, written once in code
  and reproduced here as the ruling. It drives BOTH the type-check stage
  (which node kinds are acceptable for a target) and the convert stage
  (whether the specific value passes). Mechanism: a table-driven test runs
  every row (source kind, target kind, boundary values) through the decode
  engine and each accessor family and asserts identical results.

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
  | local date-time | `LocalDateTime`; `time.Time` | the `time.Time` conversion is kept from the existing decoder (the declared target expresses intent) |
  | local date | `LocalDate`; `time.Time` | same as local date-time |
  | local time | `LocalTime` | verbatim |
  | array | slice; `[]any` | elementwise by this table |
  | array | fixed-size Go array | EXACT length required — both under- and over-length are hard errors naming expected and got (a fixed-size array declares its arity; the any-length spelling is a slice; zero-filling would invent values); elementwise by this table. Changes shipped under-fill behavior, red-green |
  | any table form | struct, `map[string]T`, `map[string]any`, `any` | per the descriptor/front end; map values decode elementwise by this table |
  | any value | `any` | the existing native mapping (string, int64, float64, bool, time.Time, the Local types, `[]any`, `map[string]any`) |

  The coercion principle: conversion BETWEEN TOML types happens only when
  provably value-preserving (the integer-to-float rows, exactness-checked);
  narrowing INTO the declared Go target's width is the caller's explicit
  choice, range-checked, never silent-wrapping. Custom hooks, embedded
  struct promotion, and pointer targets are all kept from the existing
  decoder; its field matching is not — matching is exact-only per the
  key-matching ruling above. `[approved for the
  widening rows; deliberate for the fixed-array exact-length row; derived
  for the rest]`

- **The one place the accessors and a decode target answer differently, and
  why it is not a table divergence.** The table has no string-to-`time.Time`
  row, so `AsTime` on a string node is `KindTypeMismatch`. A DECODE target of
  type `time.Time` nonetheless accepts an RFC 3339 string, because
  `time.Time` implements `encoding.TextUnmarshaler` and the hook precedence
  above runs a string value's text hook BEFORE the table is consulted. The
  difference is between the surfaces, not between their tables: a decode
  target is declared by the caller and may bring its own decoder, while an
  accessor's target is fixed by the method and has no hook to run. Mechanism:
  the accessor/decode agreement test, which excludes exactly this pairing and
  states the exclusion as this rule. The documentation-truth pass states the
  divergence explicitly — a reader who finds `AsTime` refusing a string a
  struct field accepts must be able to read why. `[%% trust-adopted]`

- The criterion for keeping or rejecting an ecosystem convention (the
  `encoding/json` family's habits, and the existing decoder's inheritance of
  them): a convention about MECHANISM is kept — where a hook applies, what a
  hook's error means, what state a failed decode leaves, how a tag is
  spelled; a convention that is a LENIENCE POLICY is rejected — case-folding
  document keys onto field names, tolerating unknown keys, silently ignoring
  a meaningless tag option. Mechanism is what consumers must interoperate
  with; lenience is a stance this library takes for itself, and its stance
  is strictness. This criterion retro-explains the key-matching, exclusion,
  and unknown-tag-option rulings together. `[%% trust-adopted]`

## 3. Decode error reporting

- Strict decode COLLECTS every independent violation in document order and
  returns them as one aggregate error. "Independent" means: after a
  violation, validation continues across sibling keys and tables but never
  descends below an errored node, so a broken construct cannot produce
  cascading nonsense from its own interior. The aggregate's `Error()`
  renders only the first diagnostic (single-error call sites read like a
  single error); the full list is reachable via `errors.As`. Mechanism: a
  fixture with several independent violations plus violations buried under
  an errored table, asserting the exact diagnostic list, its document
  order, and the absence of the buried ones. `[deliberate — chosen against
  the recommendation after the evidence for both directions had been
  corrected twice; final]`
- The aggregate error's contract: it implements `Unwrap() []error` returning
  the diagnostics in document order; `errors.As` with a single-diagnostic
  target yields the first diagnostic (standard traversal order); `errors.As`
  with the aggregate target yields the whole list; `errors.Is` against a kind
  sentinel matches if any contained diagnostic carries that kind. An empty
  aggregate is never returned (no violations means a nil error). Mechanism:
  a contract test exercising all four behaviors. `[derived]`
- Parsing remains first-error-only (a parse cannot meaningfully continue
  past a syntax error); parse errors are not wrapped in the aggregate. The
  asymmetry is deliberate and documented. `[approved]`
- **Failed-decode target state.** Collecting diagnostics means the engine
  keeps walking after a violation, so a decode that wrote into a
  caller-supplied target would leave it PARTIALLY WRITTEN: the keys that
  decoded cleanly before, beside, and after the errored ones would already be
  in it. The problem is resolved structurally rather than documented away —
  the decode entry points are VALUE-RETURNING (shapes in the API-contracts
  section): each allocates the value it returns, and on any failure returns a
  nil result, so there is no caller-owned target to observe after an error
  and the partial work is unreachable. The descriptor path is
  `DecodeSpec(spec *Spec) (map[string]any, error)`, atomic with no exception
  clause. Written-path reporting therefore has no failure-side object to hang
  off: a failed decode has no observable partial result at all, so nothing
  needs naming there, and an `Errors`-side accessor would answer about a
  value nobody holds. It lives on the SUCCESS side instead, as the second
  result of `DecodeOver` — the defaults-overlay form, where the caller's
  question is which values the document supplied and which the seed left
  behind. It names the paths the decode wrote, in document order and in the
  library's path syntax, a value written whole counting as one path; on
  failure it is nil like the result. Mechanism: a written-path test over a
  fixture where the document overrides part of a seed. `[%% trust-adopted]`

## 4. The unified diagnostic contract

- Parse, decode, edit, and access errors share one structured shape (the
  `Error` type; fields in the API-contracts section): kind, document path,
  position, span, message, optional filename, and kind-specific detail.
  Matching via `errors.Is` (kind sentinels) and `errors.As` (structured
  access) is the documented compatibility contract. Diagnostics are always
  `*Error`. Mechanism: a contract test matching one representative error
  from each producing surface through both idioms. `[approved]`
- **What is a diagnostic and what is a plain error.** A failure that depends
  on the DOCUMENT — its syntax, its keys, its values, the path a caller
  asked for — is a diagnostic: an `*Error` with a kind, a path, and a
  position, because the consumer's remedy is to look at a place in a file. A
  failure that depends only on the consumer's own Go code — a target of the
  wrong shape, an invalid or unknown struct-tag option, a descriptor built
  with a missing sub-descriptor — is a plain error, because there is no
  document position to point at and the remedy is to edit Go source. This
  criterion is the reason the construction-time failures of the strict-
  decoding section are plain errors while everything the engine reports is
  a diagnostic. `[%% trust-adopted]`
- The complete kind set (held closed by the API-snapshot test):
  - `KindSyntax` — lexing/parsing failure.
  - `KindUnknownKey` — strict decode: a key matching no descriptor field
    (including excluded names).
  - `KindUnknownTable` — strict decode: an unknown table or array-of-tables;
    carries the direct-child inventory in the `Keys` field.
  - `KindMissingKey` — strict decode: a required key absent.
  - `KindTypeMismatch` — a value whose kind the conversion table refuses for
    the target (decode and access type-check alike); carries expected/got.
  - `KindInexact` — the conversion table's range, exactness, and arity
    failures (including fixed-array length mismatch); carries the offending
    value.
  - `KindHookError` — strict decode: a consumer hook (`UnmarshalTOML`, or
    `encoding.TextUnmarshaler` on a string value) returned an error; wraps
    that error, so `errors.Is`/`errors.As` reach the consumer's own error
    type through the aggregate. Two details of the envelope, `[%%
    trust-adopted]`: its message is the consumer's own error text VERBATIM,
    restating nothing — the diagnostic supplies the position, the path and
    the kind, and a prefix of the library's own would only push the
    consumer's sentence rightward in every rendering; and the kind renders as
    "custom decoder error", naming whose failure it reports rather than
    describing a mechanism the consumer never sees.
  - `KindNotFound` — access/edit: the path names nothing.
  - `KindBadPath` — path syntax error.
  - `KindWrongContainer` — access/edit: a path step, or the operation
    itself, is structurally inapplicable to what the path names (key on an
    array, index on a scalar, a concrete-node read on a logical-only path
    per the read-layer section, a value write at a name bound by a
    structural construct per the Set-equality section, a comment write into
    a container that cannot host a comment per the write-path section).
  - `KindBadInput` — an invalid input value to an editing operation
    (unsupported Go type, sign-bit NaN, duplicate ordered-input key,
    non-bijective permutation, unsigned overflow).
  - `KindConflict` — an edit refused because it would produce an invalid
    document (`NewTable` colliding with an array-of-tables, duplicate table
    creation).
  - `KindRoundTrip` — `WriteFile`'s validation failure: carries the filename
    and the byte offset of the first divergence in its own field. `[derived]`
- **`(*Error).Error()` renders in the compiler convention**: the parts
  [location, path, message], each omitted when empty, joined by `": "`. The
  location is `file:LINE:COL` when both a filename and a position are known,
  `LINE:COL` when only a position is, the bare `file` when only a filename
  is, and absent when neither is — so a diagnostic from a `ParseFile`
  document reads `config.toml:12:7: server.port: expected integer, got
  string`, and one from an in-memory parse drops the filename part only.
  Mechanism: a rendering test covering all four location shapes and the
  empty-path case. `[approved]`
- `Position` carries line, column, and byte offset. This is a change to the
  position type itself, so every node span carries the offset: the token
  gains an offset, position advancement carries it, and every span
  construction site populates it. The diagnostic embeds a `Position` rather
  than parallel scalar fields. Mechanism: extended span tests asserting
  offsets across construct kinds. `[derived]`
- The `ParseError` identifier does not survive: the unified type is named
  `Error`, so every consumer touch-point fails to compile and gets visited
  during the sweep (one consumer uses a bare type assertion that would
  otherwise silently stop matching). `[derived]`
- Every parser-stage error site is routed through the single error
  constructor and carries offset and snippet (before this wave, only
  lexer-stage errors fill them). Mechanism: a test walks this package's own
  source and fails on any composite literal of the diagnostic type outside
  the constructor's file. `[derived]`
- The array-of-tables collection span is synthesized by the read-layer,
  which owns the collection concept: from the first entry's header start to
  the last entry's content end (the entry's header end when the entry has no
  children), where "content end" is pinned by the read-layer section — the
  end of the last entry's OWN AST children. The concrete array-table node's
  own span is unchanged.
  Mechanism: span assertions in the read-layer suite. `[derived]`
- A document parsed via `ParseFile` remembers its filename, and every later
  diagnostic produced from that document (decode, access, edit, write)
  carries it. Mechanism: a test asserting an edit error on a
  ParseFile-loaded document names the file. `[derived]`
- Default path rendering in messages uses the library's own path syntax
  (dotted keys, bracket indices, quoted-when-not-bare segments), produced by
  the exported path helpers. Mechanism: a test round-trips a diagnostic's
  rendered path through `ParsePath`/`JoinPath`. Errors are data; consumers
  with their own diagnostic vocabularies render the structured fields
  themselves. `[approved]`

## 5. The logical read-layer

- A designed, read-only logical-tree API sits alongside the concrete syntax
  AST. The read-layer answers logical questions (what the document means —
  values, structure, order); the AST answers syntactic questions (what the
  file contains, in the form it was written — spelling, trivia, spans of
  concrete constructs). A consumer reading values uses the layer; a consumer
  editing, or inspecting how something was written, uses the AST and the
  path API. That contract belongs in the package documentation. `[approved]`
- The two surfaces have opposite stances, and the pair explains most of the
  individual rulings on either side. **Reads are spelling-blind**: the layer
  exposes whatever concretely exists, without regard to how it was written —
  an inline table and a header table answer the same questions, a value's
  base or quoting style is invisible to a reader who did not ask for it.
  **Writes are structurally conservative**: a value write touches value
  fragments and nothing else, and a structural construct changes only via a
  structural operation or an explicit `Delete` — the library never rewrites
  one spelling into another as a side effect of a write the caller meant as
  a value edit. `[%% trust-adopted]`
- Fold semantics, stated as rules (the fold suite is the executable form;
  worked examples below are normative):
  1. Key order is first-appearance order across all binding forms.
  2. Dotted keys expand: `a.b.c = 1` creates implicit records `a` and `b`.
  3. A header `[a.b]` whose prefix records do not exist creates them
     implicitly; a later `[a]` header reopens the implicit record (worked
     example: `[a.b]\nx=1\n[a]\ny=1` yields record `a` with entries `b`
     then `y`, `a` anchored at its own header, first-header-wins).
  4. Array-of-tables entries collect under one key; a sub-table header under
     an array-of-tables prefix addresses the LAST entry (worked example:
     `[[s]]\n[[s]]\n[s.t]\nx=1` puts `t` inside the second `s` entry — the
     most recent one).
  5. Inline tables fold into ordinary records; a record's origin spelling is
     not distinguishable through the layer.
  6. Every entry carries the key's span; every record carries its anchoring
     span (first header, or the creating construct).
  7. A conflict the parser accepts cannot reach the fold; any internal
     impossibility (an unhandled node kind, a slot collision) is a hard
     internal error, never a silent guess. `[approved; the rule
     formulation is derived]`
- Two span pins under rule 6, each the answer to a question the rule alone
  does not settle:
  1. When a later header re-anchors a record that an earlier construct only
     implied, the entry's `KeySpan` MOVES WITH THE ANCHOR — it is the
     anchoring header's own key part. In `[a.b]` followed by `[a]`, the root
     entry `a` reports the `a` inside the `[a]` header, not the `a` part of
     the earlier `[a.b]` header: the span answers "where is this record
     declared", and after re-anchoring that is the explicit header.
  2. A collection's span ends at the last entry's OWN AST children. A
     sub-table header that folds INTO that entry (`[[s]]` then `[s.t]`) is a
     separate top-level construct, not a child of the entry's node, and does
     not extend the collection's span.
  Mechanism: span assertions in the fold suite for both shapes.
  `[%% trust-adopted]`
- `Entry.Node()` answers only for VALUE-KINDED entries (scalars and plain
  arrays) — a record-kinded entry has no single node to hand back in
  general, and the comma-ok answer keeps that honest. The concrete backing
  construct of a record is reachable from the record itself:
  `(*Record) Node() (Node, bool)` returns `(node, true)` for a record backed
  by one concrete construct — a header table, an inline table, an
  array-of-tables ENTRY record, and the root record (whose node is the
  `Document`) — and `(nil, false)` for an implied record (a dotted-key
  prefix, or a prefix implied by a deeper header and never given a header of
  its own). Mechanism: a fold test asserting the answer for each of the four
  backed shapes and for an implied record. `[approved]`
- Implementation directive: the layer is a separate post-parse fold. (An
  earlier directive to extend the parser's definition tracker was reversed:
  the tracker keeps unordered child maps, retains only the last
  array-of-tables entry, and anchors last-header-wins.) `[derived]`
- The layer is built lazily and cached: constructed on first read-layer
  access under a synchronization guard keyed to a document-level generation
  counter, so CONCURRENT READS OF A SHARED DOCUMENT REMAIN SAFE — the
  existing concurrent-read guarantee and its race-detector test survive
  unchanged. The counter is bumped from two structural funnels: the single
  unexported scalar payload mutator (value writes), and the single
  unexported structure mutator through which every change to container
  content or key parts routes. Invalidation is whole-layer; the rebuild-per-
  read cost of alternating edits and reads is accepted for this wave
  (incremental invalidation is deferred work). Until the node-model pass
  makes the funnels airtight, the layer is uncached (every access rebuilds —
  correct, merely slow); the caching switch lands with the node-model pass,
  with a test asserting `Parse` alone does not build the layer. Mechanism
  for the funnels: the source-walking test pattern (no writes to the guarded
  fields outside the mutator files). `[approved for the synchronized
  stance; derived for the rest]`
- Layer handles (`*Record`, `Entry`) are snapshots: valid until the next
  mutation of the document, stale afterwards; reading a stale handle returns
  stale data (documented, not detected). Mutating the document while
  iterating `Entries()` is unspecified behavior (documented). `[derived]`
- Path resolution is re-based on the layer. The internal virtual-view types
  it replaces are retired from resolution immediately and deleted once their
  last dependents are ported. **Logical-only paths** (an array-of-tables
  collection; a compound table with no single concrete node): `Resolve`
  fails with `KindWrongContainer`; `Lookup` returns `(nil, false)` and `Has`
  returns `false` — the comma-ok surface answers CONCRETE-node existence,
  and the asymmetry with the layer is documented. EDIT paths keep collection
  addressing: `Delete("products[0]")` and index steps through collections
  address the concrete entry nodes as today. `Get` (the error-swallowing,
  nil-returning read) is DELETED. Getter behavior across the wave: the
  path-level getters and the Cursor terminals change signature per the
  accessor section; answers are unchanged except the float accessors accept
  an exactly-representable integer (previously refused). `[approved for the
  refusals and the Get deletion; derived for the rest]`
- The Cursor is re-based on the read-layer internally: its position is a
  layer position (a record, an array-of-tables entry list, or a concrete
  node), so `Key` and `At` navigate compound tables and collections without
  the deleted views; terminals behave per the accessor section; a terminal
  at a position with no concrete node (`Node()` at a record for which
  `Record.Node` answers `(nil, false)` — an implied or compound record)
  reports `KindWrongContainer` through `Err()`. Behavioral change beyond
  signatures: none — chains that resolved before resolve after. Mechanism:
  the existing cursor suites, updated for signatures. `[derived]`
- Path helpers are exported: `ParsePath` and `JoinPath`, with `JoinPath` as
  the single quoting authority for path text. Mechanism: a round-trip test
  over dotted, quoted, and indexed shapes. The renderer-side bare-key
  predicate remains separate (a TOML-syntax rule, not a path rule).
  `[derived]`

## 6. The node model

- The `Node` interface drops `Value()` and stays the universal handle:
  `Type()`, `Span()`, `Raw()`, `Comment()`, `LeadingComments()` — the last
  two being the normalized comment getters of the write-path section, not a
  pair beside them, so this list is the whole interface. A `Scalar`
  sub-interface embeds `Node` and carries `Value()` plus the typed
  accessors. Per-type disposition: string, integer, float, boolean, and the
  four date-time node types implement `Scalar`; document, table,
  array-table, array, and inline-table nodes expose ordered
  children/elements through the accessor block and have NO `Value()` even as
  concrete methods; key, key-value, and comment nodes expose their parts,
  key/value pair, and text through named accessors, no `Value()`. Internal
  compile fix: Diff's leaf collection reads values through `Scalar`. This
  closes a live consumer bug (a `[]interface{}` assertion on an array's
  `Value()` that always returned nil). Mechanism: the API-snapshot test
  holds the interface sets. `[approved; per-type disposition derived]`
- Full encapsulation: node struct fields are unexported; reads go through
  the accessor block; writes go only through setters that mark the affected
  fragment dirty and distinguish value mutation (invalidates the stored
  lexeme) from trivia mutation (preserves it). `StringStyle` and
  `IntegerBase` stay exported as types, readable through accessors. The
  `Trivia` struct is unexported (reachable from no exported API; normalized
  getters and `Raw()` cover reads). External node construction: the
  `Set`/`SetCreate` pre-built-`Node` input branch is DELETED along with it —
  values enter as Go values (canonical rendering) and spelling control
  beyond that is not part of this wave (recorded in the deferred todo).
  Mechanism: the API-snapshot test plus a reflection test over all `Node`
  implementers asserting no exported fields. `[approved; the construction
  deletion and Trivia/style dispositions are derived]`
- Encapsulation extends to what a read HANDS BACK: `Raw()` returns a copy of
  the node's bytes and `RawParts()` a copy of the key's per-part bytes, as
  every other slice-returning accessor does. A read accessor that returned
  the document's own backing array would let a caller edit the document by
  writing into what it read, outside every mutator, every dirtiness mark and
  every generation bump — unexported fields would encapsulate nothing.
  Mechanism: the accessor-copy test (mutate a returned slice, assert the
  render is unchanged), plus a test per `Raw` and `RawParts` — the latter
  copying each PART and not only the outer slice, which the accessor-copy
  test cannot see. `[%% trust-adopted]`
- Lexeme invalidation is structural: each scalar's payload and lexeme live
  in one unexported struct whose only mutator clears the lexeme and bumps
  the generation counter. The structure mutator is the counterpart for shape
  changes. Mechanism: the source-walking tests of the read-layer section.
  `[approved]`
- Setters propagate dirtiness upward (parent references maintained by the
  structure mutator), replacing the per-render recursive subtree-dirtiness
  walk with a constant-time check. Mechanism: a test asserts the dirtiness
  check visits the same number of nodes (internal counter) for a shallow and
  a deeply nested document. `[derived]`
- The document type is renamed `DocumentNode` → `Document`. `[approved]`
- The exported token vocabulary is unexported (reachable from no exported
  API; a future streaming parser designs its own surface). Mechanism: the
  API-snapshot test. `[approved]`

## 7. Fidelity and rendering

- **The fragment contract.** Rendering works per fragment (decomposition in
  the Vocabulary): clean fragments splice their original bytes; only dirty
  fragments re-render. A value write invalidates the value fragment; a
  comment write invalidates that trivia fragment; nothing else invalidates
  anything. Separator bytes, intra-header whitespace, and key bytes are
  never invalidated by value or trivia writes. `RenameKey` invalidates only
  the renamed key part's bytes — sibling parts and header brackets splice.
  The blank-line runs in trivia survive trivia re-renders.
  Consequences, covered by the corpus battery: editing a value leaves the
  line's spacing and comment bytes identical; editing a comment leaves the
  value's spelling identical; editing one element of a container leaves
  every sibling's bytes (including interior comments) identical; renaming a
  key leaves the rest of its header identical. Header key raw parts
  (computed by the parser today and discarded) are wired into both header
  node types; inline-table key rendering prefers raw parts. `[approved; the
  decomposition details are derived]`
- **What `RenameKey` renames is the BINDING**, whatever constructs spell it
  out — which is why it works on HEADER paths and not only on key-value
  ones. A name bound by a value is renamed in the pair that writes it; a
  name bound by a table is renamed in every header naming that table, the
  headers of the tables NESTED inside it included, and in every dotted pair
  written under the name; a name bound by an array-of-tables is renamed in
  every entry's header. Renaming one spelling and leaving the others would
  produce a document binding two different names to one table's content.
  The fidelity half is unchanged: each renamed key PART is the only fragment
  invalidated, so brackets, sibling parts, interior whitespace and the
  line's comment all splice. A refused rename changes nothing. Mechanism:
  rename tests over each binding kind, including a nested-header fixture.
  `[%% trust-adopted]`
- **The construct separator is the serializer's, not a fragment's.** The
  document and each table body write a newline at the join between two
  constructs when the preceding one did not end in a newline. It belongs to
  neither construct's fragments: a file ending without a final newline gives
  its last construct none either, which is right until something is written
  after it — and then the two would run together into a document that no
  longer parses. Mechanism: the untouched round-trip pass over the corpus
  (which contains such files) plus an append-after-a-newline-less-file test.
  `[%% trust-adopted]`
- **Canonical rendering** (what constructed and value-mutated scalars, and
  the exported renderers, produce):
  - String: basic (double-quoted) form; escapes limited to the TOML set with
    lowercase `\u` hex for control characters; non-ASCII verbatim.
  - Integer: decimal digits, optional leading `-`, no underscores, no `+`.
  - Float: `strconv.FormatFloat(f, 'g', -1, 64)`, then a `.0` appended when
    the result contains neither `.` nor `e` — shortest round-trip form with
    a float marker always present, lowercase-`e` signed exponent as Go
    emits it; `-0.0` keeps its sign. Specials: `nan`, `inf`, `-inf` — the
    library never writes `+nan`, `-nan`, or `+inf`; the exported
    `FormatFloat` is total (any NaN including negative NaN renders `nan`).
  - Boolean: `true` / `false`.
  - Offset date-time: RFC 3339 with uppercase `T`, seconds always present,
    fractional seconds trimmed of trailing zeros, `Z` for zero offset,
    `±HH:MM` otherwise. Local date-time/date/time: the same conventions
    minus the offset.
  Mechanism: rendering tests per rule, including a special-float
  enumeration through the renderer and `FormatFloat`. `[approved; the
  per-kind completion is derived]`
- The literal renderers (string quoting, key quoting, float formatting) are
  exported for consumers, replacing wavescript's hand-written TOML renderer
  (Go-escaping string quoter; special-float spellings invalid in TOML).
  Consumer renderers implementing another project's own documented rule
  (strictcli's cross-language float, strictspec's format-neutral renderer)
  are not replacement targets. `[approved]`
- The exported renderers are TOTAL, `QuoteString` and `QuoteKey` included:
  every Go string has an output, and a string that is not valid UTF-8 is
  rendered with each invalid byte as U+FFFD. Their inverse property — the
  result parses back to the input — is scoped to valid UTF-8, and that scope
  costs nothing, because the write paths refuse invalid UTF-8 before it ever
  reaches a renderer (see the write-refusal ruling in the Set-equality
  section). A renderer that refused instead would put an error return on the
  simplest functions in the package for an input the library cannot be
  handed. Mechanism: totality tests over invalid UTF-8, and the
  refusal tests that keep documents away from it. `[%% trust-adopted]`
- All TOML-valid spellings remain valid INPUT and are preserved
  byte-for-byte while untouched. Canonicalization applies only to what the
  library writes. `[deliberate]`
- `Format()` preserves the user's blank-line grouping AT DOCUMENT AND
  TABLE-BODY LEVEL: runs collapse to one, and where the writer left no gap
  the formatter opens none. The scope is the whole rule — a multi-line array
  is RESTRUCTURED wholesale (inline or multi-line from the configured line
  width), so blank runs between its elements do not survive `Format`;
  `Bytes` preserves them, which is the difference between the two exits.
  Two further bounds: the output never opens with a blank line, and trailing
  blanks collapse to the single final newline, so blank lines at either end
  of the document are dropped. The table-blank-line option is
  insertion-only. This rework also fixes the
  formatter's spurious blank line from skipped blank-line nodes (pinned by a
  regression test built from an edit sequence — the defect is unreachable
  from parsing alone). Mechanism: a test iterating every formatting-option
  combination over a blank-line fixture, plus the grouping cases at document
  and table-body level and both document edges; the array-level bound
  follows from wholesale array restructuring and is stated in `Format`'s doc
  comment rather than pinned by a test of its own.
  `[approved; the scope statement is %% trust-adopted]`
- Whole-document canonical form (table style, key order, indentation) is out
  of scope: it belongs to a document-level canonical-form specification
  whose authorship is filed as a deliverable in the strictspec project.
  `[approved]`

## 8. The Set-equality contract

- `Set(path, value)` and `SetCreate` — the value-replacing entry points —
  are a no-op if and only if the bytes
  it would write for that value are exactly the value-fragment bytes already
  stored. With no stored lexeme (constructed or previously value-mutated
  targets), the comparison is against the canonical rendering.
  (`EnsureDefaults` writes only missing paths, so the rule is moot there;
  `AppendToArray` always writes by nature.) For a
  container-valued Set (a map, a `[]Pair`, or a slice), the comparison is
  the rendered container against the stored container's full byte range;
  when they differ the container is REPLACED WHOLESALE — interior comments
  and spellings of the old container do not survive into the new one, which
  is the documented consequence of setting a container value. Equality is
  decided before any mutator runs. `[deliberate; the no-lexeme and container
  cases are derived]`
- One rule, no special cases: NaN spellings, infinities, signed zeros,
  integer-vs-float, date-time offsets, string quoting, and integer bases
  alike. Accepted consequences: an idempotent tool normalizes a
  non-canonical spelling on first touch and is byte-stable thereafter; a
  same-content Set over a literal-quoted string converts it to basic
  quoting. What you Set is what the file says. Mechanism: Set-then-Set
  byte-stability over the corpus, plus canonical-spelling same-value no-op
  tests. `[deliberate]`
- `Set` refuses sign-bit NaN input (`KindBadInput`); the accepted NaN writes
  `nan`. Infinities need no input rule. Mechanism: input-refusal tests.
  `[deliberate]`
- **Every write path refuses text that is not valid UTF-8**, with
  `KindBadInput`: a string value, a string anywhere inside a container value
  (the containers convert their elements through the same conversion, so the
  refusal is inherited rather than restated), a map key, a `[]Pair` key, a
  key the path itself names, a `RenameKey` target, and the keys of a
  `NewTable`/`NewArrayTable` header. A TOML document is UTF-8: such bytes
  would be written as replacement characters and read back as something
  other than what the caller wrote. The rule and the exported renderers'
  totality are one design, not two — no document can acquire mangled content
  through the library's writes, which is exactly what lets `QuoteString` and
  `QuoteKey` stay total instead of growing an error return. Mechanism: a
  refusal test per write path, each asserting the document is byte-identical
  afterwards. `[%% trust-adopted]`
- `SetCreate` is ALL-OR-NOTHING: the value is converted, and every key the
  path names is checked, BEFORE a single intermediate table is made, so a
  refused write creates nothing — no headers of the path are left behind
  standing empty. The tables exist to carry the value; a write that does not
  happen creates nothing to carry. Mechanism: refusal tests asserting the
  document is unchanged after a `SetCreate` with a refused value on a path
  none of whose tables exist. `[%% trust-adopted]`
- `Set`/`SetCreate` targeting a name bound by a concrete `[header]` table
  REFUSES with `KindWrongContainer`. Value writes touch value fragments;
  structural constructs change only via structural operations or an explicit
  `Delete`. Replacing a header table with a value would mean deleting a
  construct, its children, and its comments as a side effect of what the
  caller spelled as a value write; a caller who wants that spells it as
  `Delete` followed by the write. (The refusal is about the BINDING, not the
  written form: an inline-table-valued key is a value fragment and Set
  replaces it wholesale per the container rule above.) Mechanism: a refusal
  test per binding kind. `[deliberate]`
- A no-op `Set` never clears dirtiness from an earlier edit. Mechanism: an
  edit-then-identical-Set test asserting the first edit still renders.
  `[approved]`
- `Delete` on a missing path remains a silent no-op — idempotent removal is
  the deliberate contract (an error would make ensure-absent loops
  impossible to write cleanly), EXCEPT that the existing defect where a
  missing-key delete inside an inline table dirtied the table is fixed.
  `[derived]`
- Writes into a record with NO concrete node of its own (a table implied by
  dotted keys or by a longer header) are not refusals: an EXISTING key's
  concrete key-value node is reachable by every write surface (edit,
  delete, comment) regardless of the parent record's own node; CREATING a
  new key uses the one TOML-valid spelling per case — under a
  dotted-implied parent, a dotted key-value written in the region that
  implied it (materializing a header there would be the redefinition the
  parser refuses); under a header-implied parent, the anchoring header is
  materialized and the key written beneath it, matching `EnsureDefaults`'
  standard-table intermediates. The ruled target-name refusals above are
  untouched by this. `[%% trust-adopted]`
- `NewTable`/`NewArrayTable` refuse exactly the constructions the parser's
  own definition tracker refuses, and nothing more: redefining a
  dotted-implied table at the header's FINAL key is refused
  (`KindConflict`); descending through a dotted-implied prefix to create a
  deeper sub-table is legal, as the compliance corpus's own fixtures show.
  `[%% trust-adopted]`
- The contract lives in the exported doc comments of the value-writing entry
  points. `[derived]`

## 9. The write path

- Structural operations: child PERMUTATION, array element APPEND and
  REMOVE, and the ordered inline-table INPUT type. (A key-sorting
  convenience was walked back: its only hard contract question concerned
  comment placement, and the driving consumer computes explicit orderings
  from external ranking data a comparator could not express; a declarative
  ordering convenience is deferred work, to be designed only after the
  deferred structural comment model settles.) The permutation is total — a
  bijection; violations are `KindBadInput` naming the offending index where
  one exists (an out-of-range or repeated index; a wrong-length order names
  none), and nothing is reordered. Its doc comment carries the drift note
  (read-then-permute in one editing sequence). `PermuteChildren` addresses
  the children/elements of any concrete container node (document, table,
  array-table entry, array, inline table); a logical-only path refuses per
  the read-layer section, and reordering array-of-tables entries is a
  document-level (or parent-level) permutation of the entry nodes. The
  driving consumer's reorder also DROPS children — composed as
  delete-then-permute; the pgdesign reorder scenario (grouping by
  classified kind with comments traveling, unlisted children dropped) is
  captured as an in-library acceptance test before the operation signatures
  freeze, and the §16 signatures are provisional until that test passes.
  `[approved; totality and composition are derived]`
- The ordered inline-table input is `[]Pair` (single keys, no paths). A
  duplicate key is a hard error; key syntax is validated at the Set call.
  `[derived]`
- `EnsureDefaults` (renamed from `MergeDefaults`, redesigned to the shape
  howmuchleft's implementation proved): input is an ordered `[]Default`
  (full paths — the old sub-path parameter is gone), seeding only missing
  paths, returning the list of paths it added; missing intermediate tables
  are created as standard tables (never inline), so output is deterministic;
  on the first error, paths already added stay added and are reported in
  `added` (documented partial-application contract). Mechanism: the seeding
  scenario test plus a determinism assertion. `[approved; the ordered-input
  and partial-application rules are derived]`
- The comment API's public spelling is path-based (`SetComment(path, text)`,
  `SetLeadingComments(path, texts)`); per-node setters become unexported.
  Normalization is a RE-PURPOSING of the getters the interface already
  carries, not an addition beside them: `Comment()` and `LeadingComments()`
  answer the content without the `#` and the whitespace around it, so the
  `Node` member list stays exactly what the node-model section enumerates and
  there is no parallel raw-comment pair. `Raw()` is the byte-exact read.
  `[approved; the re-purposing is %% trust-adopted]`
- The normalized getters answer in the text the path-based setters take, so
  content read from one document and written into another is written as it
  was read. Two inputs do not survive that round trip, both because content
  and marker stop being separable once the marker is gone: content that
  itself begins with `#` (`## section` reads as `# section`, and the setter
  puts its own marker in front, giving `# # section`), and the empty comment
  (`#` reads as `""`, which is exactly what the setters take to mean
  removal, so writing it back removes the comment rather than restoring it).
  A caller needing the bytes exactly reads `Raw()`. Mechanism: an audit test
  asserting both exceptions. `[%% trust-adopted]`
- Comment TEXT is refused when a comment cannot carry it: `SetComment` and
  `SetLeadingComments` report `KindBadInput` for invalid UTF-8, for a
  newline or carriage return, and for any other control character except a
  tab — the lexer's own rules for reading a comment, mirrored at the write,
  so a document cannot be written into a form that no longer parses back
  into itself. Validation runs BEFORE path resolution and before the first
  line of a multi-line block is written, so bad text on a missing path
  reports `KindBadInput` rather than `KindNotFound`, and one refused element
  refuses the whole call. Mechanism: a refusal test per rejected character
  class, a whole-block refusal test, and a test that a refused write leaves
  the comments already there untouched; the resolution ORDER itself is
  stated in the entry points' doc comments and carries no test of its own.
  `[%% trust-adopted]`
- A comment write targeting a key INSIDE AN INLINE TABLE refuses with
  `KindWrongContainer`, not `KindConflict`. TOML gives an inline table no
  place to put a comment, so the container kind structurally cannot host the
  operation — the same family as renaming through an array index, which is
  already a wrong-container refusal. `KindConflict` is for an edit that
  would produce an INVALID document; this edit has nowhere to be written at
  all. Mechanism: a refusal test asserting the kind, alongside the
  array-index rename refusal. `[%% trust-adopted]`
- `ParseFile` and an atomic validate-on-write `WriteFile` are added.
  `WriteFile` contract, each item tested: temp file in the destination's
  directory; the rendered bytes must re-parse AND the re-parse's re-render
  must byte-equal them (yields a first-divergence offset); an injected write
  or rename failure leaves the destination untouched and no temp file
  behind; an existing destination's file mode survives; a new file is
  created with mode 0o644 before umask. A round-trip failure is
  `KindRoundTrip`. Kernel-level crash injection is out of scope. `[approved;
  details derived]`
- Further `WriteFile` details, each settled by the implementation within that
  contract: `KindRoundTrip.Offset` carries the byte at which the two
  renderings disagree AND, when the rendered bytes do not parse at all, the
  offset at which they stopped being TOML (with the parse error wrapped, so
  `errors.Is` still reaches it) — one field, two ways of failing the same
  round trip. A FILESYSTEM failure is reported as the underlying error, the
  raw `*fs.PathError`, never wrapped into an `*Error`: nothing about the
  document is wrong, so there is nothing to diagnose about it — the same
  stance `ParseFile` takes when a file cannot be read. `WriteFile` does NOT
  rebind the document's remembered filename — the document remembers where
  it was READ from, and writing a copy elsewhere would otherwise re-point
  every later diagnostic at a file the document did not come from; the
  destination reaches the round-trip diagnostic directly instead. Mode: an
  existing destination keeps its own mode, applied outright with a chmod
  because the create that made the temp file applied the umask, while a new
  file is created 0o644 and takes the umask as an ordinary create would.
  `[%% trust-adopted]`

## 10. Typed accessors

- One design across all read surfaces: unexported stages — navigate
  (failures: `KindNotFound`, `KindBadPath`, `KindWrongContainer`),
  type-check (`KindTypeMismatch`), convert (`KindInexact`) — with the
  conversion table driving both checking stages, shared with the decode
  engine. Mechanism: a test per stage asserting the kind sentinel, plus the
  cross-family table test. `[approved]`
- Accessor errors are the unified `*Error` — same contract, kinds, path,
  span, expected/got, offending value. `[approved]`
- Ergonomics: path-level and node-level accessors return `(T, error)`;
  Cursor terminals return `(T, error)` with `Err()` retained for
  navigation-terminated chains. `Lookup`/`Has` are the comma-ok existence
  surface; allocation budget: no allocations beyond path parsing, asserted
  once the layer cache is warm. `AsTime` follows the conversion table
  (offset date-times verbatim; local flavors convert as the table's
  `time.Time` rows say). `[approved; the AsTime alignment is derived]`

## 11. Public surface end-states

- Kept: the Cursor (re-based per the read-layer section), Merge (re-based on
  the layer), Walk (kept ON THE AST, documented as the syntactic traversal;
  logical traversal is a consumer recursion over `Record.Entries`), and
  Diff. Diff's semantics, ruled concretely and pinned by test: integer and
  float never compare equal; structural spellings of the same logical shape
  compare equal (an inline array of inline tables equals an
  array-of-tables). The current implementation already exhibits both, so the
  Diff work is verification-and-pinning. `[approved]`
- **Diff equality, completed.** The governing property is that A DOCUMENT
  ALWAYS COMPARES EQUAL TO ITSELF, and two comparisons follow from it rather
  than from IEEE or from `reflect.DeepEqual`: two not-a-numbers compare
  equal (IEEE inequality would make every document carrying a `nan` differ
  from itself), and two offset date-times naming one instant compare equal
  whatever zone offsets they were written in (the instant is the value; the
  offset is spelling). The counterweight is that two values of different Go
  types are never equal, which is what keeps an integer and a float apart —
  the same one rule stated as int≠float above. Mechanism: the Diff pinning
  suite, including a self-comparison over documents carrying `nan` and
  offset date-times. `[%% trust-adopted]`
- **Merge semantics**, as ruled by the implementation within the "re-based on
  the layer" heading, `[%% trust-adopted]`:
  - Merge is LAYER-BASED on both sides: what merges is what the source
    MEANS, not how it is written, and a key the target spells with a longer
    header counts as present just as much as one with a header of its own.
  - The TARGET WINS for every key it already carries; array-of-tables are
    atomic (anything already at the path keeps what it has, entries and
    all).
  - A table NEW in the target and written inline arrives as one construct,
    through the ordered `[]Pair` form, so the source's key order survives —
    a map would have been alphabetized by the write path. A dotted key
    inside such a table folds into nested pairs, which is a change of
    spelling with identical meaning, since that is what the key already
    named.
  - Only the BINDING LINE's comments travel to a new key: the comments above
    it and the one after it on the same line. Comments written INSIDE a
    container value do not, because a new container is written from its
    values. A table merged key by key keeps each key's own comments, every
    one of them being a binding line.
  - Merging one source TWICE doubles its leading comments: the second merge
    finds every key present, and the existing-key rule appends.
  - A source whose only content is `[deep.nest]` leaves an EMPTY `[deep]`
    header in the target above it — Merge writes through `SetCreate`, which
    spells every intermediate table the path names as a header of its own.
  Each is stated in `Merge`'s doc comment as contract rather than accident,
  and pinned by the merge suites.
- Deleted: `Get`; the path-based `Items`/`Len` on the document (entry
  enumeration via `Record.Entries`, array-of-tables length via the entry's
  records, plain array length via the array node's elements accessor; the
  Cursor's own iteration survives as part of the kept Cursor surface); the
  map-only `Marshal` (no fleet callers at verification time; a real
  struct-to-TOML Marshal is deferred work); the pre-built-`Node` input
  branch of `Set`/`SetCreate` (node-model section). `[approved]`
- Mechanism for the whole surface: a committed exported-API snapshot test —
  an un-performed deletion, an unruled addition, and an unruled kind member
  all fail it. The snapshot is created at the start of the wave (baseline
  captured from the last released version BEFORE the rename pass) and the
  release's migration table is the diff of the two snapshots, extended with
  the decode-time breaks the consumer check discovers. `[derived]`
- Small correctness fixes riding the wave (each red-green): unsigned
  overflow checking in the value-to-node path; `NewTable` refusing an
  array-of-tables name collision; removal of the unused parent-dirty
  parameter; fixed-array exact-length decoding; the inline-table
  delete-missing-key dirty fix; the formatter's spurious blank line.
  `[derived; the exact-length fix is deliberate]`

## 12. Consumer end-states and sweep constraints

- Order: release this library; migrate strictcli and its conformance harness
  in one commit and RELEASE strictcli (several repos require both modules —
  minimal version selection would otherwise compile strictcli's unmigrated
  source against the new API); migrate safegit in the same working session
  (its Go workspace `use`s the strictcli checkout); every remaining consumer
  bumps BOTH module requirements in one commit. saferm (workspace-`use`s
  strictcli, does not import this package) gets a `go work sync` plus the
  strictcli bump. `[derived]`
- Each consumer's break list is produced mechanically before the release, in
  two parts: a scratch-clone build against the pre-release tree (temporary
  replace directive, never committed) capturing compile breaks, and each
  consumer's own test suite plus fixture corpus run against the same tree
  capturing decode-time breaks (strictness errors no compiler can see).
  `[derived]`
- Per-consumer rulings. wavescript: deletes its internal strict-decode layer
  and its typed-read, enumeration, comment, value-rendering, and write-cycle
  helpers; its two path-based `Len` call sites move to the read-layer.
  strictcli: builds runtime descriptors from its registries, keeps its
  cross-language message vocabularies rendering engine error data, migrates
  its bare error type assertion, adopts `ParseFile`, keeps its effects-
  handle writes. (Out of scope: its conditional unknown-key rejection — a
  todo is filed in the strictcli project; not a sweep item.) pgdesign: the
  minimal-correct migration — delete the excluded-field workaround and the
  two-pass hand decode (the map-of-struct fix makes direct decoding work),
  strip the `omitempty` tag options its migration structs carry (decode-time
  breaks), plus the mechanical conversions; its generated validator stack
  stays untouched (handover is a later campaign). howmuchleft: keeps its
  startup auto-create; a present-but-invalid config becomes a hard
  positioned error instead of a stderr-warning-plus-defaults fall-back;
  adopts `EnsureDefaults`, plain config fields, the container-elements fix
  for its always-nil profile reading (also fixing the consequence that
  profile registration overwrote the whole list — a named consumer-facing
  defect in its changelog), and the preserving serializer for write-back.
  safegit: adopts required tags for presence; keeps pointer fields for its
  optional-presence and mutual-exclusivity domain logic. dirstat: adopts a
  descriptor for its flat schema; keeps choices, uniqueness, and bespoke
  messages app-side. strictspec: keeps its format-neutral document model;
  its TOML front end shrinks to a thin conversion from the read-layer; its
  byte-offset derivation workaround is deleted (note: its span-synthesis
  helper ends collection spans at the last entry's header, where the
  library's collection span ends at content — the conversion reconciles
  this explicitly). `[approved; the howmuchleft and pgdesign shapes were
  re-ruled on corrected premises]`

## 13. Release and process

- One release at the end of the library work, a breaking minor on 0.x, with
  changelog entries recorded per work item as committed (never batched at
  release time), and the migration table per the public-surface section. The
  sweep follows the release. `[approved]`
- The implementation's total order (other sections point here): repository
  hygiene and test-infrastructure hardening; the API-snapshot baseline
  captured from the last released version; the mechanical
  `DocumentNode`-to-`Document` and `Rename`-to-`RenameKey` rename pass; the
  diagnostic contract with `ParseFile` and the §1 README/import-graph
  items; the read-layer, path re-basing (the `Get` deletion, the
  logical-path refusals, the Cursor re-basing), and the exported path
  helpers; the decode engine and descriptor surface (absorbing the
  decode-bug regression tests and the fixed-array exact-length flip); the
  structural operations, ordered input types, and `EnsureDefaults`; the
  node-model pass (interface split, field unexporting, both mutator
  funnels, fragment state, dirtiness propagation, the comment-API
  respelling, the layer's caching switch); fragment-based rendering, the
  Set-equality contract, and value canonicalization with the exported
  renderers; the formatter's blank-line preservation; the accessor
  conversion of all three families (including the scripted
  `(T, bool)`-to-`(T, error)` call-site sweep); `WriteFile`; the ports and
  pinning work (Merge, Walk documentation, Diff pinning) and the deletions
  and the token/`Trivia` unexporting as one late scripted identifier sweep;
  the documentation truth pass; the pre-release consumer compile-and-test
  check; the release. Semantic changes follow dependency order; mechanical
  changes collapse into as few scripted passes as that order allows, each
  with a dry run, asserted per-file occurrence counts, and a reviewed diff.
  `[derived]`
- Repository hygiene precedes the wave: a gofmt sweep as its own commit; CI
  steps failing on unformatted code and staticcheck findings (the kept
  `SkipTable` sentinel is renamed `ErrSkipTable` in the same commit that
  adds the staticcheck step, so CI is never red on sentinel naming in
  between); the go directive set to the actual floor (1.23,
  for the iterator types) with a CI matrix of that floor plus stable
  (matrix-supplied versions replacing go-version-file). The CI job name
  stays `test` — the publish workflow's check matcher keys on it. `[derived]`
- Test infrastructure: the compliance-suite skip list is derived by
  differencing the corpus walk against the corpus's own version-filtered
  listing API restricted to the valid and invalid trees; corpus case counts
  are asserted in the test (the counts and the audit-file count are removed
  from `docs/_CLAUDE.md`); every fuzz target reads one committed seed list,
  minimized crashers only; the read-error signal in the corpus walk
  survives the removal of the misleading counters. `[derived]`
- The corpus battery (each entry naming its property): the untouched
  round-trip pass — for every valid corpus file, `Parse(x).Bytes() == x`;
  the fragment-fidelity passes — mutate each value through public setters
  and assert all other bytes unchanged; mutate trivia and assert value
  bytes unchanged; rename each renameable key and assert the rest of its
  construct unchanged; the renderer-correctness pass — with lexeme splicing
  disabled through an internal test-only switch, render, re-parse, assert
  semantic equality (NaN compared by kind) plus an idempotent second
  render; the stale-lexeme pass — set every scalar to a new value (numerics
  perturbed, strings appended, booleans flipped, date-times shifted) and
  assert re-parsed values match and the write was not a no-op; and the
  Set-equality passes of the Set-equality section. `[derived]`
- Red-green discipline for the decode bugs fixed by the engine replacement
  (map-of-struct path truncation; array-of-tables under a plain table
  silently dropped; the map-element reflect panic; fixed-array under-fill):
  tests phrased on the public surface, committed before the engine work
  with their red output pasted into the commit message, turning green under
  the engine. The audit test that only compared two decode entry points is
  strengthened to assert values. `[derived]`

## 14. Deferred and rejected

The deferred-work todo (`todo/deferred-redesign-work.md`, filed) carries:
the real struct-to-TOML Marshal (with the note
that pgdesign's exporter could shrink onto it); the remaining structural-
manipulation suite (node moves, positional insertion, table/inline
conversion); the declarative key-ordering convenience (deferred by ruling —
designed only after the structural comment model settles); the structural
comment model end-state (containers holding only key-values plus a
trailing-trivia slot, per-comment attachment direction as data); external
node construction with spelling control (deleted this wave with no
replacement); the splice API; the upgrade of the engine's independence rule
if consumers want finer collection semantics; incremental read-layer
invalidation; a pre-parsed path type for allocation-free lookups; the
enum-in-descriptor extension (open decision); and the presence-reporting
API (rejected with rationale: required-key support plus `EnsureDefaults`
absorbed the need). Rejected outright: a kernel-level crash-injection
harness for `WriteFile` and a struct-size budget test (flaky; a
hand-maintained number).

## 15. Settled names

The user-ruled name set (renamed-from or new). All `[approved]`:

- **`Error`** (renamed from `ParseError`, generalized) / **`Errors`** (new,
  the aggregate) / **`ErrorKind`** (new) with per-kind `Err*` sentinels.
- **`Record` / `Entry`** (new) — the read-layer's container and element.
  Deliberately not `Table` (must not be confusable with `TableNode`).
- **`Scalar`** (new) — the value-carrying node sub-interface.
- **`Spec` / `Field`** (new) — the descriptor and its per-key element.
  Deliberately not `Schema` (strictspec's domain).
- **`RenameKey`** (renamed from `Rename`) — renames a key in place.
- **`EnsureDefaults`** (renamed from `MergeDefaults`, redesigned).
- **`Document`** (renamed from `DocumentNode`).
- **`Pair`** (new) — the key/value element of the ordered inline-table
  input (single keys). **`Default`** (new) — the path/value element of
  `EnsureDefaults` (full paths). Split deliberately: one type carrying both
  grammars invited silent misuse in both directions. `[deliberate]`
- **`ErrSkipTable`** (renamed from `SkipTable`) — sentinel naming.

Names introduced by the API-contracts section beyond this set are `[derived]`
and freely revisable within their stated shapes.

## 16. API contracts

Shapes are stated as Go declarations (English paraphrase does not
type-check); signatures are binding in shape, parameter names free, held by
the API-snapshot test. The structural-operation signatures are provisional
until the pgdesign acceptance test of the write-path section passes. The
value-returning shape of the decode entry points, `DecodeOver` and its
written-path result among them, is `[%% trust-adopted]`, ruled in the
decode-error-reporting section.

```go
// Diagnostics. All diagnostics are *Error; Error/Is/Unwrap use pointer
// receivers. Sentinels are hand-written exported vars of an unexported
// error type, one per kind; a drift test iterates the kind list and
// asserts each kind has a sentinel and vice versa.
type ErrorKind int // KindSyntax, KindUnknownKey, ... (closed set, section 4)

type Position struct{ Line, Column, Offset int } // 1-based lines/cols, 0-based offset

type Error struct {
    Kind     ErrorKind
    Path     string   // document path, library path syntax
    Pos      Position // primary position
    Span     Span
    Message  string
    File     string   // empty when no file is known
    Snippet  string   // source excerpt, parse-stage diagnostics
    Expected string   // type-mismatch detail
    Got      string   // type-mismatch detail
    Value    any      // offending value (inexact, bad-input)
    Keys     []string // unknown-table inventory
    Offset   int      // KindRoundTrip: first divergence in rendered bytes
    // wraps an underlying error where one exists; Unwrap() error
}
// (e *Error) Error() string — the compiler convention: the non-empty parts
// of [location, path, message] joined by ": ", where location is
// "file:LINE:COL", "LINE:COL", "file", or absent (section 4).

type Errors struct{ /* diagnostics in document order */ }
// (e *Errors) Error() string  — renders the first diagnostic
// (e *Errors) Unwrap() []error — all diagnostics, document order

func ParseFile(path string) (*Document, error)
// (d *Document) WriteFile(path string) error

// Read-layer (read-only: no type below has a mutating method).
// (d *Document) Root() *Record
// (r *Record) Entries() iter.Seq[Entry] — first-appearance order
// (r *Record) Len() int                 — entry count
// (r *Record) Get(key string) (Entry, bool)
// (r *Record) Span() Span               — the record's anchoring span
// (r *Record) Node() (Node, bool)       — the backing concrete construct:
//   header table, inline table, array-of-tables entry, or the Document for
//   the root record; (nil, false) for an implied record
// (e Entry) Key() string
// (e Entry) KeySpan() Span
// (e Entry) Kind() EntryKind            — scalar/array, record, records
// (e Entry) Record() (*Record, bool)    — any table spelling
// (e Entry) Records() ([]*Record, bool) — array-of-tables entries (copy)
// (e Entry) RecordsSpan() Span          — the synthesized collection span
// (e Entry) Node() (Node, bool)         — scalars and plain arrays
//   (value-kinded entries only; a record-kinded entry answers through
//   Record.Node above)

// Descriptor. Field values are built whole and assigned into Fields
// (map elements are not addressable; in-place mutation is not a
// supported spelling). Missing-required diagnostics are reported in
// lexicographic key order (map iteration is unordered).
type Spec struct {
    Fields  map[string]Field
    Dynamic *Field // uniform descriptor for arbitrary extra keys;
                   // nil means no extra keys are permitted — the one
                   // licensed nil-as-meaning in the descriptor
}
type Field struct {
    Kind     FieldKind // string, integer, float, boolean, the four
                       // date-time flavors as FOUR members (no union),
                       // array, table, any
    Required bool
    Elem     *Field // required for array kinds (construction-time error otherwise)
    Table    *Spec  // required for table kinds (construction-time error otherwise)
}
func FieldAny() Field // the explicit any-value spelling
// (d *Document) Validate(*Spec) error          — engine only
//
// The decode entry points RETURN the value they build: each allocates its own
// result and answers (nil, err) on failure, so there is no caller-owned target
// to observe after an error (decode-error-reporting section). Go has no
// parameterized methods, so the document- and node-level forms are package
// functions taking the document or the node as their first argument.
// func Unmarshal[T any](data []byte) (*T, error)  — parse + struct front end
// func Decode[T any](d *Document) (*T, error)     — struct front end
// func DecodeNode[T any](n Node) (*T, error)      — node-level decode; accepts
//   container nodes (tables, array-tables, inline tables, arrays) and,
//   for scalar targets, scalar nodes
// func DecodeOver[T any](d *Document, seed func() T) (*T, []string, error)
//   — the defaults-overlay form: the FACTORY builds the seed (so what the
//   decode fills is never memory the caller still holds), the document
//   overlays it, and the second result names the paths the decode wrote, in
//   document order and in the library's path syntax, a value written whole
//   counting as one path; nil on failure
// (d *Document) DecodeSpec(spec *Spec) (map[string]any, error) — the
//   descriptor path's value-returning form; atomic, no partial result on
//   error (decode-error-reporting section)

// Structural operations (concrete containers; logical-only paths refuse).
// (d *Document) PermuteChildren(path string, order []int) error
//   gather semantics: order[i] = index of the existing child moving to
//   position i; children [A, B] with order [1, 0] yield [B, A]; empty
//   path addresses the document's own children
// (d *Document) AppendToArray(path string, value any) error
// (d *Document) RemoveFromArray(path string, index int) error // negative ok
type Pair struct{ Key string; Value any }    // ordered inline-table input
type Default struct{ Path string; Value any } // EnsureDefaults input
// (d *Document) EnsureDefaults(defaults []Default) (added []string, err error)

// AST node accessors (names mirror the fields they replace; returned
// slices are copies — a test mutates a returned slice and asserts the
// render is unchanged).
// (d *Document) Children() []Node
// (t *TableNode) Children() []Node        (t *TableNode) KeyPath() []string
// (a *ArrayTableNode) Children() []Node   (a *ArrayTableNode) KeyPath() []string
// (a *ArrayNode) Elements() []Node
// (i *InlineTableNode) Children() []Node
// (k *KeyValueNode) Key() *KeyNode        (k *KeyValueNode) Val() Node
// (k *KeyNode) Parts() []string  RawParts() [][]byte  Styles() []StringStyle
// (c *CommentNode) Text() string
// (s *StringNode) Style() StringStyle     (n *IntegerNode) Base() IntegerBase

// Accessors. On Scalar and mirrored at path level:
// AsString() (string, error)   AsInt() (int64, error)
// AsFloat() (float64, error)   AsBool() (bool, error)
// AsTime() (time.Time, error)  — per the conversion table's time.Time rows
// AsLocalDateTime() / AsLocalDate() / AsLocalTime() — node level only
// Path level keeps GetString(path)-style spellings with (T, error).
// Cursor terminals keep short spellings — String(), Int(), Float(),
// Bool(), Time() — with (T, error); String() (string, error) is
// deliberately not a fmt.Stringer.
// (d *Document) Resolve(path string) (Node, error)
// (d *Document) Lookup(path string) (Node, bool)
// (d *Document) Has(path string) bool

// Renderers and path helpers.
// func QuoteString(s string) string // basic-string form
// func QuoteKey(s string) string
// func FormatFloat(f float64) string // total; canonical form incl. specials
type SegmentKind int // key, index
type PathSegment struct{ Kind SegmentKind; Key string; Index int } // negative Index legal
// func ParsePath(s string) ([]PathSegment, error)
// func JoinPath(segs []PathSegment) string // the quoting authority for paths
```
