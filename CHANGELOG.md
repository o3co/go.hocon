# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed — `adapters/yaml`: a leading empty document discarded the whole file

**BREAKING** (input previously accepted is now refused; quote or remove the
extra `---` if the file really is one document).

`yaml.Parse("---\n---\na: 1\n")` returned an empty config with no error, and so
did every `---`-prefixed stream whose first document holds nothing but comments
— the shape generated YAML takes once a header block is stripped. The F5.7
guard against multi-document streams never ran: goccy answers `io.EOF` for
these, and the adapter read that as "empty input" (F5.9). Every document was
dropped, which is a worse outcome than the silent loss of document 2..n that
F5.7 exists to prevent. `parser.ParseBytes` cannot see it either — it reports
one body-less document — so the count is now taken from the library's token
stream, which still carries both `---` markers and knows that a `---` inside a
block scalar or a quoted string is text ([#166](https://github.com/o3co/go.hocon/issues/166)).

A trailing `---` (`a: 1\n---\n`) is a second, empty document and is now refused
for the same reason; ts.hocon, rs.hocon and py.hocon already refused it, so
this closes a four-way parity gap rather than opening one. A directive line
(`%YAML 1.2`) still belongs to the document that follows it, and a malformed
document is still reported as malformed by the decoder rather than as a stream.

### Fixed — README stated four things that were no longer true

The README's factual claims had drifted release by release, with nothing
checking them. All four are corrected, and the ones that can be recomputed from
a source of truth in this repository are now pinned by `docs_test.go`, which
runs on every pull request and on `develop`. That is where the gate has to be
for a Go module: pushing the tag *is* the publish, so nothing at release time
can hold a stale README back — only refusing to merge it can.

- **The stated minimum Go version was wrong.** The README said "Requires Go
  1.21+" while `go.mod` has declared `go 1.23.0` since v1.8.0, so a user on
  1.21 or 1.22 got a toolchain error on `go get`. Now 1.23+, and pinned against
  the `go` directive.
- **"Stricter than Lightbend — S8.6 leading-hyphen rejection" described behavior
  retracted in v1.3.0.** `a = -foo`, `a = -`, and (since v1.9.0) `a.-foo = 1`
  all parse; the section told readers to quote values that need no quoting. The
  E8 amendment and its retraction are already recorded in the [1.3.0] and
  [1.9.0] sections, so the section is removed rather than rewritten.
- **The compliance rates were a 2026-05-13 snapshot** (71.8% / 80.2%) against
  the current 89.0% / 98.4%. They are now recomputed from the status glyphs in
  `docs/spec-compliance.md` by `TestDocs_ReadmeComplianceRates` and compared
  against the table, so the two cannot diverge again.
- **Both comparison tables were stale in the reader's favour and against the
  other project's.** gurkankaymak/hocon v1.3.0 has deferred resolution
  (`ParseStringUnresolved` + `WithFallback` + `Resolve`), `include required(...)`,
  object/array concatenation and type coercion — all four were marked ❌ or ⚠️.
  Our own row was stale too: YAML and TOML were ❌ although `adapters/` has
  shipped both since v1.10.0. Both tables now name the version they were
  verified against and carry the date.

## [1.11.0] - 2026-07-26

### Added — `Config.RenderHOCON()`, a HOCON text emitter

- **A resolved config can now be rendered back to HOCON text**, the missing
  reverse of parsing. `RenderHOCON()` walks the value tree (objects, arrays,
  string / number / boolean / null scalars — exactly what `FromMap` and the
  format adapters produce) and returns idiomatic HOCON: root fields without
  enclosing braces, nested objects as `key { … }`, arrays newline-separated,
  two-space indent.
- The contract is **round-trip**, not byte formatting: parsing the output
  yields the same tree. A scalar is quoted whenever leaving it bare would
  re-parse as a different type (a string `"8080"` stays a string, `no` stays a
  string, an empty or space-padded value is quoted); a multi-line value is
  triple-quoted when that is lossless and escaped in double quotes otherwise.
  An unresolved placeholder is an error — substitutions have no textual round
  trip through a value tree.
- This is the core half of the reverse-conversion work tracked in
  [xx.hocon#74](https://github.com/o3co/xx.hocon/issues/74); hocon2's
  `xxx2hocon` commands build on it.

### Fixed

- **`adapters/` did not build for consumers**: `adapters/go.mod` still required
  core `v1.9.0`, which predates the `internal/properties` syntax-layer API the
  module uses. The dev-only `replace => ../` hid it inside this repo —
  consumers ignore a dependency's `replace`. To see it as a consumer did:
  `go get github.com/o3co/go.hocon/adapters` (the module has no tag of its own
  yet, so this resolves a pseudo-version off the default branch, which carried
  `require v1.9.0`) then build — `assignment mismatch: 2 variables but
  syntax.Parse returns 1 value`. The requirement now names `v1.10.0`, and CI
  builds the adapters module with the `replace` dropped against the published
  core, on every PR and again at release, so a stale require cannot ship.
- **`adapters/yaml` silently dropped a value when two keys resolved alike**:
  `1.0:` and `"1":`, `0x10:` and `"16":`, `~:` and `"null":`, `true:` and
  `True:`, `8:` and `0o10:` — each pair becomes one object key, and the loser
  vanished without an error. (goccy's own duplicate detection compares key
  text, so it catches `1:` against `"1":` and no further.) Per spec F5.3 this
  is now an error naming both spellings, their line numbers and the path. Keys
  a tag resolves to something other than a scalar are covered too: `!!timestamp
  2002-12-14` and the string `"2002-12-14 00:00:00 +0000 UTC"` are one key in
  the decoded map.
  The check runs on the document, since a decoded map has already discarded
  the evidence; merge keys are exempt, as `<<:` legitimately supplies a key the
  mapping then overrides.
- **`adapters/yaml.FromValue` resolved colliding key forms nondeterministically**:
  in an injected `map[any]any`, the int key `1` and the string key `"1"` both
  normalize to `"1"`, and whichever the randomized map iteration visited last
  won. Now an error naming both — and the message itself no longer depends on
  iteration order, since all key problems are collected and sorted before one
  is reported.
- **`adapters/env` reported a collision that was not there, and missed the
  distinction between two that were**: the collision index joined path
  segments with NUL, so `APP_A<NUL>B` (one segment, reachable through
  `Options.Environ`) collided with `APP_A__B` (two); and the message joined the
  path on `.` unconditionally, so a literal-dot key and a `__` path read
  identically. Segments are now length-prefixed, and the path is rendered as a
  HOCON path expression (`a.b`, `"foo.bar"`) — the format py.hocon and
  rs.hocon use.
- **`adapters/env` admitted undecodable environment entries** (spec F1.9(b)):
  a bulk mount asks for a whole namespace, but a variable in it whose name or
  value was not valid UTF-8 was mounted anyway, its raw bytes becoming a key no
  lookup could reach. Such an entry is now an error naming the variable — the
  value is never echoed, since that is where credentials live. The check runs
  **after** the prefix filter, so an undecodable variable the caller never
  asked for still cannot break an unrelated mount.
- **`adapters/env` folded case beyond ASCII** (spec F1.3): `strings.ToLower`
  turns `İ` (U+0130) into `i`, so `APP_İ` and `APP_I` collided here while
  Python, JS and Rust kept them apart. Only `A`–`Z` is folded now.
- **A leading UTF-8 BOM is stripped** by every adapter (spec F0.9). A BOM used
  to become part of the first key in YAML and Properties — `a: 1` yielded a key
  that `GetString("a")` could not find, the plausible-but-wrong outcome this
  project treats as worse than an error — and made JSONC and TOML fail with a
  message about a stray character. The core parser already ignored it.

### Changed

- **`adapters/jsonc` is stricter about what follows the top-level value**
  (spec F3.2). `{"a":1} }`, `{"a":1} ]`, `{"a":1} }}]]` and `{"a":1} } {"b":2}`
  used to parse, silently dropping everything past the first value: the check
  was `Decoder.More`, a one-token peek that reports false on a closing bracket.
  A document now holds exactly one value; whitespace and comments may follow
  it, nothing else. Rejection costs a token rather than a decode, so a large
  trailing payload is not materialized just to be discarded.
- **`adapters/jsonc` comments now separate tokens** (spec F3.2). A block comment
  was removed outright, so `1/*x*/2` became the single number `12`; it is now
  replaced by whitespace and the input is a syntax error. A `//` comment also
  ends at a lone CR, not only at LF — previously a CR-terminated comment
  swallowed the rest of the file.

## [1.10.0] - 2026-07-25

### Added — `adapters/`, a nested module for reading foreign config formats

- **`include`-adjacent formats owned by other programs can now be mounted as
  config**: `adapters/properties`, `adapters/env`, `adapters/jsonc`,
  `adapters/toml` and `adapters/yaml` each return a fully resolved `*Config` you place under your own
  document with `WithFallback`, so a `${...}` can reach into it. Ingestion is
  AST-level — a document is decoded and built into a value tree, never rendered
  to HOCON text — so there is no emitter and no escaping to get wrong.
- `adapters/` is a **module of its own**, so `github.com/o3co/go.hocon` still has
  zero dependencies: importing the parser pulls in nothing. Only
  `adapters/toml` brings a dependency (`pelletier/go-toml/v2`), and only for
  those who import it.
- `adapters/yaml` uses `goccy/go-yaml` rather than `gopkg.in/yaml.v3`, whose
  last release is 2022 and whose upstream is archived. It follows the YAML 1.2
  core schema, so the "Norway problem" does not arise: `no`, `yes`, `on` and
  `off` stay strings. A multi-document stream is refused rather than decoded,
  because decoding one returns the first document and drops the rest silently.
- Plain JSON needs no adapter, since HOCON is a JSON superset; a conformance
  test now proves that rather than leaving it asserted. Reading a single
  environment variable needs none either — that is what `${?VAR}` is for. The
  `env` package is for mounting a whole prefixed namespace as a subtree.
- Note for callers: parse the host document with
  `WithResolveSubstitutions(false)` before attaching the fallback. `ParseFile`
  resolves as it parses, so a `${...}` aimed at the foreign layer would fail
  before the fallback is ever attached.

### Fixed — `include "x.properties"` now accepts the whole of java.util.Properties syntax

- **Backslash continuations, escapes, and whitespace separators in an included
  `.properties` file were mishandled**, and a continued line was silently
  dropped. Lightbend hands an included properties file to
  `java.util.Properties`; this parser implemented roughly the "key=value with #
  comments" subset. Concretely, with `a = one\` continued by `two`, `b\:c = 2`,
  `d = x\ty`, `e = é` and `f value = 3`, every one of the five produced a
  wrong key or a wrong value — `{"a":"one\\","b\\":"c = 2","d":"x\\ty",
  "e":"\\u00e9","f value":"3"}` instead of `{"a":"onetwo","b:c":"2","d":"x\ty",
  "e":"é","f":"value = 3"}`.
- Now handled: `\` line continuation, the `\t \n \r \f \uXXXX` escapes
  (including surrogate pairs), escaped separators (`\:` `\=` `\ `) belonging to
  the key, whitespace alone as a separator, `\r\n` and bare `\r` line endings,
  and UTF-8 validation.
- **Behavior change**: trailing whitespace in a value is now **preserved**.
  Java skips whitespace before a value but never after it, so `key = value  `
  yields `"value  "`. The previous parser trimmed both ends.
- A malformed escape (an unpaired `\uXXXX` surrogate, a truncated one) is now a
  `ResolveError` naming the file, rather than silently mangled text. Go strings
  cannot hold an unpaired surrogate, which would otherwise become U+FFFD.
- The syntax layer is shared with `adapters/properties`, so the include path and
  the adapter cannot drift apart.

### Fixed — empty path elements and backticks rejected in key/value position (S11.7 + S8.1, [xx.hocon#68](https://github.com/o3co/xx.hocon/issues/68))

- **`a..b: 3`, `.a: 3`, `a...c: 4` and `a...c."": 4` now error instead of silently
  collapsing to `{"a":{"b":3}}` / `{"a":3}` / `{"a":{"c":4}}`.** HOCON.md L515-519 is
  explicit that an empty path element must always be quoted: `a."".b` is a valid
  three-element path, but `a..b` — and any path starting or ending with `.` — is
  invalid. `parseKey` split each unquoted key token on `.` and dropped every empty
  piece as "leading/trailing dot noise", so genuine empty elements vanished. It now
  routes through `splitKeySegments`, which drops only the two structural pieces (a
  leading empty when the path already has segments — the token's `.` is the separator
  after `"a"` / `a. ` — and a trailing empty, the continuation marker the post-loop
  `trailingDot` guard still polices) and rejects the rest. The substitution-path
  position was already correct (the lexer's `parseSubstBody` state machine rejects
  `${?a..b}` / `${?.a}` / `${?a.}`); only the key position diverged. Quoted empty
  elements (`a."".b`, `"".b`) and E13 path-whitespace forms (`a . b`, `a. .b`,
  `a b. c`) are unaffected. Pinned by `path-empty-segment/pe01–pe08`.
- **Backtick is now forbidden in unquoted strings**, so ``a = `t` ``, ``` `k` = 1 ```
  and ``a = x`y`` error. HOCON.md L245-247 lists `` ` `` in the forbidden set; every
  other member was already rejected, and `isUnquotedSubstChar` already excluded it
  inside `${...}` — only the `unquotedForbidden` set for ordinary unquoted strings
  leaked. A backtick inside a quoted (or triple-quoted) string remains ordinary
  content, and `(` / `)` stay out of the forbidden set per
  [xx.hocon#34](https://github.com/o3co/xx.hocon/issues/34). Pinned by
  `unquoted-forbidden/uf01–uf04`.
- Both groups are driven by `issue68_path_empty_segment_test.go`; `make testdata` now
  fetches the two new fixture directories.

### Fixed — whitespace-separated undefined optionals materialize the separator ([#158](https://github.com/o3co/go.hocon/issues/158))

- **`cf = ${?vv} ${?vv}` (all operands undefined, whitespace between) now yields
  `cf = " "` instead of dropping the field**, matching the reference
  implementation and the ts/rs/py siblings. Per HOCON.md §Substitutions an
  undefined `${?foo}` becomes an empty string inside a value concatenation with
  another string — the inter-token whitespace is that string. The spec's
  field-drop example (`foo : ${?bar}${?baz}`, no whitespace) still drops the
  field. Found by the harvested ecosystem corpus
  ([xx.hocon#66](https://github.com/o3co/xx.hocon/pull/66),
  `mikai233-hocon-rs/concat3.conf`). Pinned by
  `issue158_ws_only_optional_concat_test.go`.

## [1.9.0] - 2026-07-23

Cross-impl release coordinated to land at v1.9.0 across ts.hocon / go.hocon / rs.hocon / py.hocon. Covers the two same-day spec corrections from [xx.hocon#62](https://github.com/o3co/xx.hocon/pull/62) (S3.1 — empty document parses to `{}`) and [xx.hocon#64](https://github.com/o3co/xx.hocon/pull/64) (S3.5 — array-root document rejected with a type error). MINOR (not PATCH): go adds public API surface (`ResolveError.Cause` field + `Unwrap()` method — additive, enables `errors.Is`/`errors.As` traversal), and the error-taxonomy / empty-document behavior changes are consumer-observable. The tag is the version (no version file).

### Fixed — array-root document rejected with a type error (S3.5, [xx.hocon#64](https://github.com/o3co/xx.hocon/pull/64))

- **`ParseString("[1,2]")` now returns `*ConfigError` ("document has type array rather
  than object at file root", HOCON.md L989-991, with origin + bracket position) instead
  of a `*ParseError` "expected key".** An array-root document is syntactically valid
  HOCON; the reference implementation parses it and rejects at the Config boundary
  (`Parseable.forceParsedToObject`, `ConfigException.WrongType`). `parseRoot` now
  parses the array fully — malformed arrays (`[1,2`) and trailing content remain
  `*ParseError`s — and returns the `parser.ErrArrayAtRoot` sentinel, mapped to
  `ConfigError` at the public boundary. Include paths (file + package) raise
  `*ResolveError` "included file has array at file root … (HOCON.md L993-994)" naming
  the included source, improving the S14b.1 diagnostic. The package-content error
  wrapper now preserves the cause chain (`ResolveError.Cause`), so `errors.Is` works
  through package parse errors. Net behavior is unchanged (array-root documents still
  error) — only the error class, layer, and message change. Pinned by
  `s3_5_array_root_test.go` (xx.hocon `array-root/ar01–ar03` `.error` sidecars).

### Fixed — empty document parses to `{}` (S3.1 corrected, [xx.hocon#62](https://github.com/o3co/xx.hocon/pull/62))

- **`ParseString("")` (and whitespace-only / comment-only / BOM-only input) returns an
  empty `Config` instead of erroring.** The S3.1 checklist item "Empty file is invalid
  (HOCON.md L130)" misread the L130-132 *JSON baseline* as HOCON-normative; the
  L134-136 brace-omission relaxation parses any document not beginning with `[` or `{`
  as if enclosed in `{}` — an empty document is therefore the empty object. Confirmed
  by the reference implementation (Lightbend's `"Empty document"` error is
  `ConfigSyntax.JSON`-only; `ConfigFactory.parseString("")` is a valid empty config in
  its own test suite). Restores pre-guard behavior — the cluster 3h `parseRoot` reject
  was a regression. The rule now applies uniformly: top-level parse, file includes
  (the former #105 `isEmptyOrCommentOnlyHocon` carve-out is removed — the parser
  handles it naturally), and package includes (whitespace-only / comment-only
  registered content now contributes `{}` like zero-byte content — the zero-byte-only
  special-case is gone). Pure loosening — no previously-valid input changes meaning;
  previously-rejected empty documents now succeed. Pinned by
  `TestS3_1_EmptyFile_ParsesToEmptyObject` (ef01–ef06 `{}` sidecars now normative),
  `TestSpec_S3_1_EmptyDocumentParsesToEmptyObject`,
  `TestIssue105_TopLevelEmptyParsesToEmptyObject`, and
  `TestPackageLookupWhitespaceOnlyContentSucceeds`.

## [1.8.0] - 2026-06-16

### Added

- `Config.UnmarshalPath(path string, v any) error` — unmarshal the value at a
  path into `v`, accepting **any** node (object, array, or scalar), e.g.
  `UnmarshalPath("servers", &[]Server{})`. Unlike `GetConfig(path).Unmarshal`
  (objects only) it returns an error (matching `Unmarshal`'s idiom, not the
  panicking getters): missing path, an unresolved substitution at/under `path`
  (the error wraps `ErrNotResolved`, detectable via `errors.Is`), or an
  unmarshal failure.
- `Unmarshal` / `UnmarshalPath` now support `any` (`interface{}`) targets,
  decoding a node into the natural Go value (`map[string]any` / `[]any` /
  `string` / `int64` for integer-shaped numbers or `float64` otherwise / `bool`
  / `nil`).

### Changed

- Integer unmarshalling is now consistent with the typed getters and with
  rs.hocon: a whole-number float/exponent (including quoted, e.g. `"1e3"`,
  `1.0`) coerces, but a **non-whole** float such as `1.5` is now **rejected**
  instead of silently truncated to `1`, and integer targets are overflow-checked
  (`int8`/`int16`/`int32` no longer silently wrap).
- A numeric-keyed object (`{ "0" = …, "1" = … }`) now unmarshals into a slice
  target, matching the typed slice getters and rs.hocon sequence deserialization.

Value introspection in Go remains served by the existing typed getters
(`GetString` / `GetInt64` / …); rs.hocon's `HoconValue` accessors have no Go
equivalent by design (Go exposes no public value handle).

### Fixed — integer coercion precision hole for float-like literals ≥ 2^52 ([xx.hocon#56](https://github.com/o3co/xx.hocon/issues/56))

- **Float-like integer unmarshalling now derives wholeness and the integer value
  from the raw decimal text instead of an intermediate `float64`.** The previous
  `f != math.Trunc(f)` check was unsound at high magnitudes: above 2^52 a
  `float64` cannot represent fractional parts, so a non-whole literal like
  `9007199254740992.5` rounded to a whole `float64` and was wrongly accepted, and
  a would-be-whole literal like `9007199254740993.0` rounded to the wrong integer
  (silent off-by-one). The text-based `wholeFloatToInt64` helper rejects genuinely
  non-whole values at any magnitude and yields the exact integer for whole ones
  (including `int64` boundaries and large whole literals such as `1e16`), with a
  guard so a huge exponent cannot trigger an unbounded allocation. Byte-identical
  to rs.hocon's `whole_float_to_i64`. ts.hocon is unaffected (no integer
  coercion). Pinned by `issue56_i64_coercion_test.go`.

## [1.7.1] - 2026-06-14

Cross-impl coordinated patch release (v1.7.1 across go.hocon / ts.hocon / rs.hocon). **No functional changes in go.hocon.** The substantive change in this patch is rs.hocon's false-positive `circular substitution` fix ([rs.hocon#136](https://github.com/o3co/rs.hocon/pull/136)); go.hocon already resolves the same self-ref-below-merge shapes as of v1.7.0 (its [#135](https://github.com/o3co/go.hocon/pull/135) defer-substitution-resolution work), so this release carries no go-side change and exists for cross-impl version parity (precedent: v1.7.0's coordinated sync). A cross-impl audit added [#148](https://github.com/o3co/go.hocon/pull/148) — test-only regression pins for that resolved behavior ([#147](https://github.com/o3co/go.hocon/issues/147)). No public API changes; safe drop-in upgrade from v1.7.0.

## [1.7.0] - 2026-05-30

Cross-impl release coordinated to land at v1.7.0 across go.hocon / ts.hocon / rs.hocon. Drives the version bump: a new public-API surface — the error-returning `(T, error)` accessor family `GetXxxE` ([#142](https://github.com/o3co/go.hocon/issues/142)) — that completes the accessor triad alongside the existing panic-getters and Option-getters, bringing go.hocon's surface in line with rs.hocon's `Result`-primary API. Also closes the last open item of the go.hocon#131–#135 audit ([#135](https://github.com/o3co/go.hocon/issues/135) — defer substitution resolution across includes, the Critical Lightbend-parity fix in the #128 family) and the cross-impl leading-zero JSON-render validity fix ([xx.hocon#50](https://github.com/o3co/xx.hocon/issues/50)) shipped byte-aligned with ts.hocon v1.7.0. No breaking changes; safe drop-in upgrade from v1.6.1.

### Added — error-returning `(T, error)` accessor family ([#142](https://github.com/o3co/go.hocon/issues/142))

- **New `GetXxxE` accessors** complete the triad alongside the existing panic-getters (`GetXxx`) and Option-getters (`GetXxxOption`), mirroring rs.hocon's `Result`-primary API (`get_string -> Result<String, ConfigError>`) and giving Go's `(T, error)` idiom for surfacing expected config-shape failures (previously only panic or lossy None were available). Members: `GetStringE`, `GetInt64E`, `GetIntE`, `GetFloat64E`, `GetFloat32E`, `GetBoolE`, `GetDurationE`, `GetBytesE`, `GetStringSliceE`, `GetInt64SliceE`, `GetIntSliceE`, `GetConfigE`, `GetConfigSliceE` (the full panic-getter set). Returns `*ConfigError` on missing path / non-scalar (or non-array / non-object for the matching variants) / null / type-conversion failure / unresolved placeholder; the unresolved-placeholder error wraps `ErrNotResolved` so callers can `errors.Is(err, hocon.ErrNotResolved)` (matching the existing panic-getter contract via `errors.As` on the recovered `*ConfigError`). The implementation is additive — existing panic-getters and Option-getters are unchanged — and shares three private helpers (`lookupE` / `getScalarE` / `getArrayE`) for the lookup-and-convert core. S15 numeric-object→array conversion applies on the slice E-accessors identically to the existing `GetXxxSlice` / `GetXxxSliceOption` contract. Pinned by `config_e_test.go` (100% coverage of `config_e.go`).

### Fixed — defer substitution resolution across includes ([#135](https://github.com/o3co/go.hocon/issues/135))

- **Substitutions in an included file now resolve against the fully-merged document, not the include-local scope** (Lightbend parity; the last open item of the go.hocon#131–#135 audit, in the #128 family). `include "common" (base = 11; computed = ${base}); include "override" (base = 17); value = ${computed}` now yields `base=17, computed=17, value=17` — previously `computed` froze to `11` because the include child resolved in isolation before `override`'s `base` merged. go.hocon now follows the deferred model rs.hocon / ts.hocon already use: `parseAndResolve` builds the included file into an **unresolved** tree, the parent merges it unresolved, and substitutions resolve once over the merged tree. The bespoke include-merge is kept (not routed through `MergeUnresolved`, which carries `WithFallback`-only composition-barrier semantics) and adapted for unresolved children. Three interactions were re-established for the deferred model: the `resolveSubst` delayed-merge is no longer gated to single-segment paths, so a relativized `${foo.a}` sees the same `ConfigDelayedMergeObject` a top-level `${a}` would (test09/test10 conformance); cross-include `+=` accumulation (#134) is stitched via `rehydrateSentinel(inclPrior, foldedExisting)` instead of the eager-model sentinel-in-value trick; `deepMerge` + `setPath` carry the self-ref prior for nested-object `+=` chains (`selfRefFullKey`). Pinned by `internal/resolver/issue135_deferred_include_test.go`; go/rs/ts agree on the repro. This supersedes the "#135 stays deferred" note in v1.6.0/v1.6.1.

### Fixed — leading-zero numeric values render as valid canonical JSON ([xx.hocon#50](https://github.com/o3co/xx.hocon/issues/50))

- **Leading-zero numeric *value* literals now render as canonical JSON numbers** (Lightbend / rs.hocon parity). The differential adapter (`RenderJSONForTest`) emitted numeric-value lexemes verbatim, so `b = 023` / `c = 08.53` produced `{"b":023,"c":08.53}` — not valid JSON, and divergent from Lightbend/rs (which emit `23` / `8.53`). `writeValJSON` now strips redundant leading zeros from the integer part of a number lexeme before emitting (`023`→`23`, `-08.53`→`-8.53`, `007`→`7`; `0`, `0.5`, `1.0` unchanged), matching the xx.hocon `lzv01` cross-impl fixture and the existing `us13` (`{"a":1}`). Render-only: `GetString`/concat still return the verbatim source lexeme (S10.11), so `GetString("023")`→`"023"` and `GetInt64("023")`→`23` are unchanged. The broader numeric-canonical family (exponent `1e3`→`1000.0`, trailing-zero `1.50`→`1.5`, `-0`→`0`) is a separate, untested convergence tracked as a follow-up. Pinned by `TestRenderJSONForTest_LeadingZeroNumberCanonicalized`.

## [1.6.1] - 2026-05-29

Bugfix release: S13b.2 `+=` accumulation across includes ([#134](https://github.com/o3co/go.hocon/issues/134)) — the follow-up deferred from v1.6.0 — plus an `Unmarshal`-into-map explicit-null preservation fix ([#131](https://github.com/o3co/go.hocon/issues/131)) from the go.hocon#131–#135 audit. No public API changes; safe drop-in upgrade from v1.6.0. (The remaining go-only audit item — [#135](https://github.com/o3co/go.hocon/issues/135), defer substitution resolution across includes — is a Critical include-child eager-vs-deferred resolution change in the same family as #128 and stays deferred to a follow-up.)

### Fixed — S13b.2 `+=` accumulation across includes ([#134](https://github.com/o3co/go.hocon/issues/134))

- **Repeated `+=` array appends across included files now accumulate in document order**, matching Lightbend's treat-includes-as-textual-inlining semantics (HOCON.md L732, `a += b` ≡ `a = ${?a} [b]`). `include "first" (items += "a"); include "second" (items += "b"); items += "main"` now yields `["a", "b", "main"]` instead of dropping earlier includes' elements. `+=` was an eager lookup-and-append that snapshotted the existing array in each included file's isolated scope, so the cross-include merge overwrote it. The fix desugars `+=` to the fully-qualified `${?key} [value]` self-ref concat at resolve time (`FieldNode.AppendToConcat`), so it flows through the chained-self-ref machinery (#118/#120). Optional self-refs with no in-child prior are now preserved as `knownAbsent` sentinels in an include child (rather than dropped to absent), and the include-merge / `deepMerge` rehydrate those sentinels against the parent's pre-merge value (`rehydrateSentinel`) — splicing the included chain onto the accumulated value across the boundary. Reset semantics (an explicit `k = [...]` before a `k +=`, in an included file or the parent) leave no sentinel and so correctly overwrite. Pinned by 12 tests in `internal/resolver/s13b2_plus_equals_include_test.go`, including within-file `+=` chains inside a later include merged onto a non-empty destination, multi-write includes, nested-path, and prefix-mounted includes.
- **Deferred `WithFallback` + `+=` no longer stack-overflows** (multi-agent-review, cross-impl with ts.hocon / rs.hocon). After the desugar, a fallback's `+=` value is a `${?k} [...]` self-ref concat; `MergeUnresolved` recorded it as the receiver's prior unfolded, so the receiver's `${?k}` followed a prior still containing `${?k}` → infinite recursion. `MergeUnresolved` now folds the displaced fallback value self-ref-free (`foldOrSkipPrior`) before recording it, so `ParseStringWithOptions("items += \"r\"", deferred).WithFallback(ParseStringWithOptions("items += \"f\"", deferred)).Resolve(...)` yields `["f","r"]`. Pinned by `TestS13b2_DeferredWithFallbackPlusEqualsAccumulates`.
- **Incidental fix from the desugar: `key += [array]` now nests the RHS instead of flattening it.** Because `a += b` ≡ `a = ${?a} [b]`, an array RHS is appended as a single element: `a = [1, 2]; a += [3]` now yields `[1, 2, [3]]` (spec-correct, matching ts.hocon / rs.hocon), where the old eager append spread the RHS array's elements to produce `[1, 2, 3]`. A bare scalar RHS (`a += 3`) is unchanged. Pinned by `TestResolver_ObjectPlusEqualsAppendsArray`.

### Fixed — explicit null fallback keys dropped by `Unmarshal` into a map ([#131](https://github.com/o3co/go.hocon/issues/131))

- **Explicit `null` values contributed by a fallback config now survive `Unmarshal` into a `map[string]any`** (Lightbend parity). Given `config.WithFallback(config.GetConfig("variables"))` where `variables` carries `value = null`, the resolved root correctly held a top-level `value = null` — `MergeUnresolved`, the resolver, and `renderJSON` all preserved it — but `Unmarshal` into a `map[string]any` dropped the key. The defect was `Unmarshal`-only: for an interface-valued map, `valToAny` returns an untyped Go `nil`, `reflect.ValueOf(nil)` is an *invalid* `reflect.Value`, and `SetMapIndex` with an invalid value *deletes* the entry instead of storing key→nil. Nested nulls were unaffected because `valToAny` builds nested maps via plain `m[k] = nil` assignment. The fix stores a typed zero of the map's element type for nils, so explicit nulls remain visible keys. Pinned by `TestIssue131_NullFallbackPreservedInUnmarshalMap` + `TestIssue131_PlainNullKeyPreservedInUnmarshalMap`.

## [1.6.0] - 2026-05-28

Cross-impl release coordinated to land at v1.6.0 across go.hocon / ts.hocon / rs.hocon. Two string-concat spec fixes from the go.hocon#131–#135 audit, plus an incidental S10.15 spec-compliance fix that fell out of the #132 refactor. No public API changes; safe drop-in upgrade from v1.5.3. (go.hocon#134 — `+=` accumulation across includes — and the go-only #131 / #135 are deferred to follow-ups: #134 needs the multi-agent-review treatment the chained-self-ref machinery already required, and #135 is a Critical include-child eager-vs-deferred resolution change in the same family as #128.)

### Fixed — S10.5 inner whitespace in value concatenation ([#132](https://github.com/o3co/go.hocon/issues/132))

- **Literal whitespace runs between simple values in a string concatenation are now preserved verbatim** (HOCON.md §String value concatenation L332). `parseValue` inserted a single hardcoded `" "` separator between concat pieces, collapsing every multi-space run to one space (`foo   bar` → `"foo bar"`, and `"left"  ${?UNSET}  "right"` → `"left  right"` instead of Lightbend's `"left    right"`). The fix carries the literal `Token.PrecedingWhitespace` run (the lexer field E13 added for key-position whitespace) into the value-position separator. To keep separator identity while preserving the literal run, `ScalarNode` / `ScalarVal` gain a `Separator bool` flag and `isSeparator` now reads the flag instead of detecting a single-space `Raw`. Single-space concatenations are unchanged. Pinned by `TestS10_5_ValueConcatWhitespace` + `TestS10_5_UndefinedOptionalKeepsBothRuns`.

### Fixed — S10.11 numeric lexeme preserved for stringification ([#133](https://github.com/o3co/go.hocon/issues/133))

- **Numbers now stringify "as written in the source file" when concatenated** (HOCON.md L366). `parseSingleValue` canonicalized integer lexemes via `strconv.FormatInt` (`05` → `"5"`, and `00_example` — lexed as `TokenInt("00")` + unquoted `_example` — → `"0_example"`), losing the source spelling. `version = ${major}.${minor}` with `minor = 05` produced `"26.5"` instead of `"26.05"`. The `ScalarNode.Raw` now retains the source lexeme; the numeric accessors (`GetInt64` etc. re-parse `Raw` via `strconv.ParseInt`) still drop leading zeros / negative-zero sign for the standalone value, so this refines — not reverses — the earlier E8/F3 canonicalization. The two E8 unit tests + the octal-like `GetString` test are updated to assert lexeme preservation plus a semantic-accessor check. Pinned by `TestS10_11_NumericLexemePreserved` + `TestS10_11_NumericPrefixUnquotedKeepsLexeme`.

### Fixed — S10.15 quoted whitespace between array substitutions now errors ([#83](https://github.com/o3co/go.hocon/issues/83))

- **`${a} " " ${b}` (a quoted whitespace string between two array substitutions) now raises the spec-required type error** (HOCON.md L442) instead of silently merging the arrays. This fell out of the #132 refactor: the old `isSeparator` matched on `Raw == " "`, which also classified a *quoted* `" "` as a parser separator and stripped it (merging `[1]` and `[2]` into `[1,2]`). With the explicit `Separator` flag, a quoted `" "` correctly carries `Separator=false` and is not stripped, so the array + string + array concat raises the S10.13 type error. The `_Pin` test documenting the merged-array bug was removed and the `_Spec` test un-skipped.

## [1.5.3] - 2026-05-26

Bugfix release: include-child env-with-default fallback ([#128](https://github.com/o3co/go.hocon/issues/128)), E6 cross-source list-suffix env-fallback ordering ([xx.hocon#22](https://github.com/o3co/xx.hocon/issues/22) + S13c.5), C3 cluster 3h cross-impl resolver bugs ([xx.hocon#27](https://github.com/o3co/xx.hocon/issues/27) — sr15 / sr13), and E13 key-position parsing alignment ([xx.hocon#42](https://github.com/o3co/xx.hocon/issues/42)). No public API changes; safe drop-in upgrade from v1.5.2.

### Fixed — include-child env-with-default fallback ([#128](https://github.com/o3co/go.hocon/issues/128))

- **`key = "default"; key = ${?ENV_VAR}` no longer erases the prior default when reached through an include and `ENV_VAR` is unset** ([#128](https://github.com/o3co/go.hocon/issues/128)). The canonical Lightbend reference.conf idiom — shipped via every per-package `reference.conf` and used by every consumer of `include package(...)` (E11) for HOCON-native config decentralization — was structurally broken in v1.4.1–v1.5.2: env unset → field vanishes (silent regression vs. Lightbend semantics where the prior duplicate-key assignment must be retained). Direct-parse path was unaffected — only the through-include path. Root cause: `resolveSubstitutions` constructed a fresh `result` object and copied `obj.values[k]` forward but dropped `obj.priorValues`; the include-child's lenient-pass placeholder then reached the parent's strict pass with an empty priorValues map, so the optional-`nil` return path dropped the field instead of falling back to the prior. Fix preserves `obj.priorValues` into `result.priorValues` at the top of `resolveSubstitutions`, scoped to `inIncludeChild` so already-resolved top-level Configs don't carry stale priorValues into later `WithFallback` composition (Codex review on PR [#129](https://github.com/o3co/go.hocon/pull/129)). Cross-impl regression coverage landed on ts.hocon (PR [#138](https://github.com/o3co/ts.hocon/pull/138)) and rs.hocon (PR [#126](https://github.com/o3co/rs.hocon/pull/126)) — both unaffected (single-pass substitution resolver over a merge-time-populated `priorValues`); these PRs pin that invariant so a future refactor to a multi-pass shape cannot silently regress to go.hocon's pre-fix shape.

### Fixed — E6 cross-source list-suffix env-fallback ordering ([xx.hocon#22](https://github.com/o3co/xx.hocon/issues/22))

- **`${X[]}` in an included file now resolves against the parent config tree before falling through to env-var list expansion** ([xx.hocon#22](https://github.com/o3co/xx.hocon/issues/22)). Per Lightbend 1.4.6 `ResolveSource.java:100-130`, config exhaustion (prefixed lookup + original-path S14c.2 fallback) runs BEFORE any env-var fallback including listSuffix expansion. Reorder in `resolveSubst`: (1) prefixed config lookup, (2) original-path config fallback (S14c.2 — moved BEFORE listSuffix), (3) listSuffix env-var list expansion, (4) scalar env fallback. Adds `TestLightbend_Test03_S14c2SubtreeFallback` (test03.conf cross-source) and the ev12c-include-config-defined-wins fixture to the S13c success suite.
- **S13c.5 invariant preserved across the reorder**: the new original-path scalar env lookup (S14c.2 step) is gated on `!listSuffix` so a host env `X=scalar` (without `X_0`) cannot return a `ScalarVal` for `${X[]}` substitutions — all env access for listSuffix is delegated exclusively to `resolveEnvList`. Pinned by `TestResolver_IncludeListSuffixNoScalarFallbackCrossSource` (Codex P2 review on PR [#127](https://github.com/o3co/go.hocon/pull/127)).

### Fixed — C3 cluster 3h cross-impl resolver bugs ([xx.hocon#27](https://github.com/o3co/xx.hocon/issues/27) E14)

- **sr15 — "drop first concat" universal cross-impl resolver bug** ([xx.hocon#27](https://github.com/o3co/xx.hocon/issues/27)). Optional self-references with no prior value at save time previously caused the prior-save to be skipped, so the first concat in chains like `a = ${?a} [...]; a = ${?a} [...]` dropped its element. Fix introduces an internal `knownAbsent` sentinel in `foldselfref.go`: optional no-prior self-refs fold to the sentinel instead of skipping the prior-save. The sentinel resolves to undefined in `resolveSubst` and is ignored by pointer-identity self-ref detection. Container-aware via `containsKnownAbsentSentinel` / `rehydrateSentinel` extended to recurse into `ArrayVal.Elements` + `ObjectVal` field values (sentinels can be embedded via `foldOptionalSelfRefAbsent`). Array rehydration uses single-element insertion (not splice-flatten) per HOCON spec — Lightbend immediate-resolve equivalence verified via probe. (sr12, sr14, sr16 already passed pre-fix via the existing pointer-identity `containsSubstByIdentity` mechanism.)
- **sr13 — nested prior-lookup regression from the sr15 fix**. The sr15 fix exposed a nested re-entrant path in `resolveSubst` where nested-scope prior lookup was lost. Both prior-lookup paths (re-entrant and pointer-identity self-ref) now route through a shared `findPrior(...)` helper, restoring the nested lookup. Pinned by `TestS13a13_NestedPriorDoesNotCollideWithTopLevel` plus 45 new unit tests in `internal/resolver/foldselfref_test.go`.

### Changed — E13 key-position parsing (xx.hocon [#42](https://github.com/o3co/xx.hocon/issues/42))

- **S8.6 is no longer enforced on key path segments** — `foo -bar = 1`, `foo.-bar = 1`, `-foo bar = 1`, `foo -1bar = 1` etc. now parse verbatim per Lightbend 1.4.3. The HOCON.md L270-276 "begin with `-` requires digit" rule is a value-position lexer-disambiguation rule (governed by E8 in [xx.hocon extra-spec-conventions](https://github.com/o3co/xx.hocon/blob/main/docs/extra-spec-conventions.md)); key-position is governed by path-element parsing rules where Lightbend takes characters verbatim. Pinned by 8 new fixtures (`key-hyphen-position/kh01–kh08`) in xx.hocon main. Pure loosening — no previously-valid input is now rejected. `validateKeySegment` removed from `internal/parser/parser.go` along with its two call sites.
- **Path-expression whitespace adjacent to dots is preserved verbatim** — `a b. c = 1` → `{"a b":{" c":1}}` (leading space on `" c"` preserved); `a b.\tc = 1` → `{"a b":{"\tc":1}}` (HOCON_WS tab uniformly preserved); `a .b = 1` → `{"a ":{"b":1}}` (trailing space on `"a "` preserved). Per Lightbend's char-by-char path parsing. Pinned by 6 new fixtures (`path-expr-whitespace/pw01–pw05, pw07`) + 1 error fixture (`pw06: a b. = 1` → BadPath). See [xx.hocon E13](https://github.com/o3co/xx.hocon/blob/main/docs/extra-spec-conventions.md#e13).
- **Behavior change — key string normalisation no longer fires for path-WS-adjacent-to-dot inputs**. Inputs like `a .b = X` previously produced path `["a", "b"]`; now produce `["a ", "b"]`. Tab between key tokens is now preserved (was normalised to single ASCII space) — `a\tb = 1` now yields key `["a\tb"]` instead of `["a b"]`. Narrow set of affected inputs.
- **Bundled fix — trailing-dot key paths now consistently reject**. `foo. = 1`, `a.b. = 1`, `a b. = 1`, `a. . = 1` now return parse error ("path has a trailing period — empty key segment not allowed"). Pre-E13 these silently parsed to the prefix segments. Aligned with Lightbend BadPath and E13 boundary fixture `pw06`. Leading-dot (`.foo = 1`) and double-dot (`a..b = 1`) in key paths are NOT addressed in this PR (pre-existing silent-accept gap, no xx.hocon fixture yet — tracked as a follow-up).
- **Bundled fix — dot-WS-dot in key paths produces a WS segment per Lightbend**. `a. .b = 1` now yields `["a", " ", "b"]` (`{"a":{" ":{"b":1}}}`); `a b. .c = 1` yields `["a b", " ", "c"]`. Cross-impl convergent fix with rs.hocon (where Codex multi-agent-review caught the original gap) and ts.hocon.

#### Implementation

- **Lexer** (`internal/lexer/lexer.go`): adds `Token.PrecedingWhitespace string` field (literal whitespace chars consumed since the previous token, accumulated in `Lexer.skippedWhitespace`). Token type lives in `internal/lexer/` (non-public per Go internal/ convention) so no source-break concern.
- **Parser** (`internal/parser/parser.go::parseKey`):
  - Removes `validateKeySegment` function and its two call sites (initial-split path and numeric-concat-resplit path).
  - Replaces literal `" "` joiner in space-concat path with the token's `PrecedingWhitespace`.
  - Adds `postDotPrefix` state for post-trailing-dot WS handling, with dot-WS-dot branch promoting the WS to its own segment.
  - Leading-dot branch now sets `spaceConcat=true` when the dot-leading token has `PrecedingSpace` (so WS-before-dot becomes trailing on prev segment in the next iteration).
  - Adds post-loop guard: trailing-dot at end of key path returns parse error.

#### Fixed (Copilot review on PR #125)

- **Trailing-dot guard now fires for all input shapes** (Copilot G1). Previously the trailing-dot continuation `continue`d unconditionally without checking that the next token could start a key segment, allowing `=` / `:` / `{` / newline / EOF to be silently absorbed as a key segment so the post-loop `trailingDot` guard never fired. Inputs like `foo. = 1` errored later with the misleading "expected ':', '=' or '{' after key" message instead of the correct "trailing period" BadPath. Fix: switch on `p.current.Type` before continue and break out when the next token is not key-eligible. Pinned by `TestE13_TrailingDot_TriggersBadPath_NotConsumeSeparator` (6 cases: `foo. = 1`, `a. = 1`, `a. = "x"`, `a. {b=2}`, `a.b. = 1`, `a. \n`).
- **`PrecedingSpace` / `PrecedingWhitespace` doc accuracy** (Copilot G2). Doc previously claimed `PrecedingSpace` becomes true for "whitespace OR a comment", but `skipWhitespaceAndComments` only sets `skippedSpace` on the HOCON_WS branch — comments alone never set it. Updated the doc to match the impl (`PrecedingSpace ⇔ PrecedingWhitespace != ""` in current grammar) and noted what a future grammar change introducing an inline comment would need to adjust.

## [1.5.2] - 2026-05-23

Bugfix release: value-interior self-referential substitution ([#120](https://github.com/o3co/go.hocon/issues/120)) — follow-up to v1.5.1's #118 chain fix. No public API changes; safe drop-in upgrade from v1.5.1. Cross-impl with rs.hocon v1.5.1 and ts.hocon v1.5.1 (which combine #118-equivalent and #120-equivalent fixes in a single release; see [rs.hocon#119](https://github.com/o3co/rs.hocon/issues/119) and [ts.hocon#131](https://github.com/o3co/ts.hocon/issues/131)).

### Fixed — value-interior self-referential substitution

- **Self-references embedded inside array elements or object field values no longer crash / error** ([#120](https://github.com/o3co/go.hocon/issues/120)). Patterns like `a = [${a}, "x"]` repeated, `a = [${a}]` repeated, `o = { history = ${o}, v = N }` (even at chain length 2), the nested-path variant (`r.s = {...}; r.s = { history = ${r.s}, ... }`), and the include-merge variant (parent `o = {v=1}`, included `o = { history = ${o}, v = 2 }`) were not covered by the v1.5.1 #118 fix because `foldSelfRef`'s walker only traversed `substPlaceholder` / `concatPlaceholder` trees AND the prior-save trigger at three sites was gated on `!merged`, silently skipping the object-deep-merge case where the merged value retained `${key}` but no prior was recorded. The walker is now extended to recurse through `ArrayVal.Elements` and `ObjectVal` field values; `resolveSubst`'s self-ref detection is extended via a new `containsSubstByIdentity` helper to recognise the resolved placeholder when it appears as an array element or object field value (pointer-identity, same criterion the existing concat case used); and the prior-save trigger is removed at all three duplicate-key save sites — top-level `resolveObject` direct assignment, `setPath` nested-path assignment, and include-merge object-merge — so the save fires regardless of whether the new value is deep-merged with the existing. The existing fold-or-skip logic handles the chain invariant (induction: every saved prior is self-ref-free). Surfaced during v1.5.1 cross-impl audit (cgordon-driven #118 follow-up); asymmetric application of the un-gating across the three save sites was caught by multi-agent code review on PR #123.

## [1.5.1] - 2026-05-23

Bugfix release: chained self-referential append crash fix ([#118](https://github.com/o3co/go.hocon/issues/118)), reported by [@cgordon](https://github.com/cgordon), and a lenient-mode include resolver follow-up ([#45](https://github.com/o3co/go.hocon/issues/45)). No public API changes; safe drop-in upgrade from v1.5.0. Cross-impl with rs.hocon v1.5.1 and ts.hocon v1.5.1 (same chain-class fix; see [rs.hocon#119](https://github.com/o3co/rs.hocon/issues/119) and [ts.hocon#131](https://github.com/o3co/ts.hocon/issues/131)).

### Fixed — chained self-referential substitution

- **Chained self-referential append no longer crashes the resolver** ([#118](https://github.com/o3co/go.hocon/issues/118)). When a key was self-referentially appended three or more times — either directly (`a = ${a} ["x"]` repeated), through chained `include` directives (each included file appending `branches = ${branches} [...]`), through object value-concat (`obj = ${obj} {key=val}` repeated), on a nested path (`r.x = ${r.x} [...]` repeated), or inside a nested object block (`r { x = ${r.x} [...] }` repeated) — the resolver entered infinite recursion and aborted the process with a Go runtime stack overflow. The deferred `Resolve` API path had the same failure mode. The fix folds self-referential `${key}` references in the prior value at save time so the recorded prior is always self-ref-free, restoring Lightbend Config's chain semantics where each `${key}` resolves to the value immediately prior to its current assignment. The nested-path and nested-object scoped variants previously errored with `circular reference detected`; those cases now also resolve correctly. Out-of-scope: self-references embedded inside array elements or object fields (e.g. `a = [${a}, 1]` chained), tracked separately in [#120](https://github.com/o3co/go.hocon/issues/120).

### Fixed — lenient optional substitution

- **Optional substitutions in included files now see parent-scope values** ([#45](https://github.com/o3co/go.hocon/issues/45)). Previously, in the lenient resolver pass used by include processing, unresolved optional substitutions (`${?path}`) were dropped immediately, so an included file referencing a parent's value (e.g. `result = ${?parent_val}`) lost the field before the parent's value could be supplied. The fix preserves the placeholder during the lenient pass; the final / strict pass still drops optional substitutions with no value. Found by Copilot review on the include-relativization PR.

## [1.5.0] - 2026-05-23

Cross-impl spec-compliance release with [rs.hocon v1.5.0](https://github.com/o3co/rs.hocon/releases/tag/v1.5.0) and [ts.hocon v1.5.0](https://github.com/o3co/ts.hocon/releases/tag/v1.5.0). Three spec-compliance bugfixes ([#83](https://github.com/o3co/go.hocon/issues/83) S8.6 multi-tail key concat, [#66](https://github.com/o3co/go.hocon/issues/66) S11.8 bool/null literal keys, [#65](https://github.com/o3co/go.hocon/issues/65) S10.8 unquoted space-concat in keys), one resolver fix ([#45](https://github.com/o3co/go.hocon/issues/45) lenient optional substitution through includes), and parser error-path coverage hardening ([#37](https://github.com/o3co/go.hocon/issues/37): 86.4% → 92.0% statement coverage on `internal/parser`). No public API changes; safe drop-in upgrade from v1.4.1.

### Fixed — S8.6 multi-tail key concat

- **Signed-numeric and chained adjacent-token keys now parse as a single key segment** ([#83](https://github.com/o3co/go.hocon/issues/83)). Follow-up to #65 / #81-followup: the parseKey concat branch was single-shot and only accepted `TokenString` / `TokenBool` / `TokenNull` / `TokenInclude` tails. This rejected signed-numeric chains because go.hocon's `readNumber` produces `TokenInt("-456")` when `-` is immediately followed by digits, so `123-456` arrives as two adjacent `TokenInt` tokens. The fix turns the concat branch into a loop that absorbs adjacent `TokenInt` / `TokenFloat` tails alongside the existing set, so chains like `123-456 = 1` → `{"123-456": 1}`, `123-456abc = 1` → `{"123-456abc": 1}`, and `123-456.foo = 1` → path `["123-456", "foo"]` all converge. Quoted-key gating preserved — `"a.b"-1 = 1` stays rejected so the literal `.` inside the quoted segment is never re-interpreted as a path separator. Cross-impl: ts.hocon and rs.hocon read `123-456` as a single unquoted token via their Option B lexer model, so they handle this naturally; this PR closes go.hocon's Option A divergence.

### Fixed — S11.8 spec compliance

- **Boolean (`true` / `false`) and null literal keys now accepted as stringified single-segment keys** ([#66](https://github.com/o3co/go.hocon/issues/66)). Per HOCON spec L504 ("if you have a path expression then it must always be converted to a string, so `true` becomes the string \"true\""), boolean and null literals appearing in key position must be stringified to their literal text. Previously `true = 42`, `false = 0`, and `null = 1` all errored at parse time with `expected key, got 4` (TokenBool) or `got 5` (TokenNull) because `parseKey` rejected anything that wasn't `TokenString` / `TokenInt` / `TokenFloat`. The fix extends the initial token-type check in `parseKey` to also accept `TokenBool` and `TokenNull`; the existing unquoted-branch produces the correct stringified single-segment key via `strings.Split(raw, ".")` on the literal text. `TokenInclude` remains rejected at the same site per S12.5 (the `include` directive is dispatched before `parseKey` runs). Composes cleanly with S10.8 (`true false = 1` → key `"true false"`) and pre-existing dotted-path semantics (`true.foo = 1` → path `["true", "foo"]` via the lexer's keyword-promotion-on-exact-match rule). rs.hocon and ts.hocon already conformed.

### Tests

- **Parser error path coverage** ([#37](https://github.com/o3co/go.hocon/issues/37)). Added `internal/parser/error_paths_test.go` with 24 focused tests exercising uncovered `newError()` call sites: `ParseBytes`, `Error.Error()` with no position, `include url(...)` / `classpath(...)` inside `file(...)` or `required(...)` qualifiers via `skipToIncludePath`, `include required(url(...))` / `include required(classpath(...))` same-token and whitespace-separated forms, missing `(` after `required` / `file` in include directives, missing inner `(` after `file` inside `required(file(...))`, `skipToIncludePath` EOF/newline before path string, invalid int/float literals in value position, unexpected token in value position, empty key, and `validateKeySegment` dash-only / dash-letter rejections. Statement coverage on `internal/parser` improved from **86.4% → 92.0%**.

### Fixed — S10.8 spec compliance

- **Unquoted space-concat in field keys now accepted as a single key** ([#65](https://github.com/o3co/go.hocon/issues/65)). Per HOCON spec L317 ("string value concatenation is allowed in field keys") and L553-560 (`a b c : 42` is equivalent to `"a b c" : 42`), space-separated unquoted tokens before the `:`/`=`/`{`/`+=` separator must merge into a single key. Previously `foo bar = 1` errored with `expected ':', '=' or '{' after key`; now it parses as key `"foo bar"`. The fix extends `parseKey` in `internal/parser/parser.go` with a space-concat continuation branch driven by a new `spaceConcat` flag: when the next key token has `PrecedingSpace`, the first dot-split piece of that token merges into the LAST existing segment with a literal space; any remaining dot-split pieces become new path segments. Quoted+unquoted mixed concat (`"foo bar" baz = 1`) and inline-object shorthand (`a b { x = 1 }`) work transitively. A leading `.` in the spaced-in token keeps path-separator semantics per S11.1 (via the pre-existing leading-dot continuation branch, which takes priority over space-concat): `a .b = 1` → `["a", "b"]`, `a.b .c = 1` → `["a", "b", "c"]`. Cross-impl with [ts.hocon PR #128](https://github.com/o3co/ts.hocon/pull/128) and [rs.hocon PR #115](https://github.com/o3co/rs.hocon/pull/115).

## [1.4.1] - 2026-05-22

Bugfix release: two cgordon-reported include-resolution divergences from Lightbend Config. Pure include-path behaviour; no public API changes; safe drop-in upgrade from v1.4.0.

### Fixed

- **Include ordering / self-referential append through include** ([#106](https://github.com/o3co/go.hocon/issues/106)). When an `include` directive appeared after an existing key in the parent file, the parent's value was incorrectly kept instead of being overridden by the included file's value. Lightbend Config treats included content as if it had been written inline at the include position; `go.hocon` now matches those semantics. Self-referential appends like `steps = ${steps} [{ name = child }]` placed inside an included file now resolve against the parent's prior value, matching Lightbend. Reported by [@cgordon](https://github.com/cgordon).

### Changed — resolver

- **Lenient self-referential substitution defer (E12 `AllowUnresolved`)**. Under `Resolve(opts.WithAllowUnresolved(true))`, a required self-referential `${k}` with no prior value used to error; it now defers the placeholder so a subsequent merge supplying a prior can complete resolution. This is required by the include-ordering fix above (the include child resolver runs in lenient mode); user-visible behaviour change is limited to `AllowUnresolved=true`, where the error is replaced by an unresolved placeholder.

### Changed — include path

- **Empty / comment-only / whitespace-only included files contribute an empty config** ([#105](https://github.com/o3co/go.hocon/issues/105), Lightbend compatibility). Previously, `include "empty.conf"` (or comment-only / whitespace-only / BOM-only content) errored with `empty file is not a valid HOCON document (HOCON.md L130)`. This blocked the common optional-override-file pattern. The carve-out is **narrow** — applies only to the file-include code path; top-level empty parses (`ParseString("")`, `ParseFile` on an empty file as the root) still error per S3.1. Reported by [@cgordon](https://github.com/cgordon).

## [1.4.0] - 2026-05-21

### Added — E11 `include package("<id>", "<file>")` qualifier

xx.hocon [#33](https://github.com/o3co/xx.hocon/issues/33), [#36](https://github.com/o3co/xx.hocon/pull/36). A new include qualifier with **service-locator semantics** — looks up `.conf` files registered under a stable name via a global registry. **Not a Java classpath equivalent** (no auto-discovery, no auto `reference.conf` merge, no transitive auto-resolution). Closest analog: JVM `ServiceLoader` / JNDI. New public surface:

- `include package("github.com/myorg/pkg", "reference.conf")` syntax — two-arg form mandatory; one-arg + missing-comma rejected at parse time.
- `RegisterPackage(identifier, file, content string) error` — global registry, parallel to `database/sql.Register`. Idempotent byte-equal re-registration allowed; different-content collision returns `*PackageCollisionError`. Call from a config-providing package's `init()` after `//go:embed`.
- `*RegistrationError` — input validation failures at registration time (empty identifier, file constraint violations including Windows-style `C:/` paths and trailing slash).
- `PackageLookup func(id, file string) (string, error)` parser option — overrides the global registry for test injection / custom resolution; correctly propagated to child resolvers across nested include chains.
- `ResetPackageRegistry()` — exported behind `//go:build testing` build tag (hidden from production builds). Use for test isolation.
- `IncludeNode.PkgID` / `IncludeNode.PkgFile` — new AST fields populated when the include qualifier is `package(...)`.
- `ResolveError.Cause` field + `Unwrap()` method — package lookup errors are now `errors.Is`/`errors.As`-discoverable.
- File argument validated **after HOCON string unescaping**: rejects empty, absolute, `..`, `./`, backslash, consecutive `/`, Windows drive prefix, trailing slash.
- Cycle detection: length-prefixed `"package:N:id:file"` cycle key integrated with existing include-cycle detection.
- `include required(package(...))` — registry miss is unconditional hard error regardless of `Required` flag state (per E11 decision 7).

### Added — E12 deferred substitution resolution (closes [#99](https://github.com/o3co/go.hocon/issues/99))

This release adds the Lightbend-aligned `parse → withFallback → resolve()`
lifecycle requested by [@cgordon in #99](https://github.com/o3co/go.hocon/issues/99).
Existing `ParseString` / `ParseFile` behaviour is unchanged (still
parse-and-resolve in one call); the new API surface is purely additive.

**New entry points:**

- `ParseStringWithOptions(input, opts ParseOptions)` and
  `ParseFileWithOptions(path, opts ParseOptions)` — `opts.ResolveSubstitutions()=false`
  produces an unresolved `Config` whose `IsResolved()` is `false` when the input
  contains any `${...}`.
- `FromMap(values map[string]any, originDescription string)` — construct a
  Config from an in-memory map.  Lightbend `ConfigValueFactory.fromMap`
  parallel.
- `Empty(originDescription string)` — empty Config.

**New methods on `*Config`:**

- `Resolve(opts ResolveOptions) (*Config, error)` — single top-level resolve
  over the entire merged fallback stack.  Idempotent on already-resolved
  configs.
- `ResolveWith(source *Config, opts ResolveOptions) (*Config, error)` —
  resolves receiver using source for substitution lookup; source's keys are
  NOT merged into the result.  Precondition: source must be resolved
  (otherwise returns `ErrNotResolved`).
- `IsResolved() bool` — reports whole-config resolution state.
- `WithFallback(other *Config) *Config` — now accepts unresolved operands;
  preserves substitution placeholders into the merged tree.  Receiver-wins
  semantics unchanged.  Result is resolved iff both inputs are resolved.

**New types:**

- `ParseOptions` — builder via `DefaultParseOptions().WithResolveSubstitutions(...)`
  and `.WithOriginDescription(...)`.  `ParseOptions{}` zero-value literal is
  NOT a valid invocation (documented).
- `ResolveOptions` — builder via `DefaultResolveOptions().WithUseSystemEnvironment(...)`
  and `.WithAllowUnresolved(...)`.

**New errors:**

- `ErrNotResolved` sentinel.  Getters on unresolved paths panic with
  `*ConfigError` whose `Unwrap()` returns `ErrNotResolved` — use
  `errors.Is(err, hocon.ErrNotResolved)`.

**Cross-spec amendments (no behavioural changes for callers using the existing
fused `ParseString`; only relevant when using the deferred-resolution lifecycle):**

- S13a × WithFallback: self-reference lookback walks across fallback
  layers.  Receiver `a = ${?a} extra` with fallback `a = base` resolves
  to `a = "base extra"`.
- S10 × AllowUnresolved: type-incompatible concat errors fire even under
  `AllowUnresolved=true`; only missing-value errors are deferred.
- Hidden substitutions in overridden values are not evaluated (HOCON.md
  §Substitutions L670-703 already-specified behaviour, now explicitly
  pinned for the deferred lifecycle).

### Fixed

- S13.15 multi-optional-undef concat materialisation: `a = ${?x}${?y}` with
  both `x` and `y` undefined now correctly omits the field (was producing
  `{"a":""}` empty string).  Per HOCON.md L640.  Bundled with E12 since
  the dr28 fixture surfaced the spec divergence.
- **parser: accept comma separator after newline** (closes [#104](https://github.com/o3co/go.hocon/issues/104), [@cgordon](https://github.com/cgordon) report). `skipSeparator()` previously consumed only `comma → newlines`, rejecting valid HOCON like `a: 1\n,\nb: 2` with `unexpected token 10`. Lightbend (1.4.6) accepts the pattern. Fix: consume newlines on both sides of the optional comma. Preserves rejection of leading commas (`,a: 1`) and repeated commas (`a: 1,, b: 2`).

**Spec source:** [xx.hocon#37](https://github.com/o3co/xx.hocon/issues/37) /
E12 in `docs/extra-spec-conventions.md`.

## [1.3.1] - 2026-05-21

### Fixed

- **S8.1/S8.8 — parens `(` `)` in unquoted strings** ([xx.hocon#34](https://github.com/o3co/xx.hocon/issues/34) external report by @cgordon, [go.hocon#100](https://github.com/o3co/go.hocon/issues/100), upstream spec PR [xx.hocon#35](https://github.com/o3co/xx.hocon/pull/35)). Real-world inputs like `description = Build API spec for X (internal)` and `a = hello (world)` previously parse-errored at `(`. They now parse to `{"description":"Build API spec for X (internal)"}` / `{"a":"hello (world)"}`, matching the spec (HOCON.md L274 forbidden set does NOT include `(` or `)`) and the established behavior of ts.hocon and rs.hocon.

  **Root cause** (pre-1.3.1): two layers rejected parens in value position:
  1. `internal/lexer/lexer.go` emitted `TokenLParen` / `TokenRParen` as standalone tokens unconditionally for every `(` / `)` it saw.
  2. The lexer's `unquotedForbidden` character set included `()`, breaking any unquoted run on a paren.

  **Fix** (Option C, mirrors `ts.hocon parseInclude` structure): parens become ordinary unquoted-continue chars at the lexer; the parser's `parseInclude` switches to **string-match on the unquoted token value** for the include resource forms — `file(...)`, `required(...)`, `classpath(...)`, `url(...)`. With no whitespace before `(`, the lexer produces a single unquoted token like `file(`, `required(file(`, etc. With whitespace, the next token starts with `(`; both forms are accepted. Trailing `)` is consumed by a "skip until end-of-statement" loop after the path string (newline / `}` / EOF / `,`). `TokenLParen` / `TokenRParen` constants removed from `internal/lexer/lexer.go` (no longer emitted; previously only consumed inside `parseInclude`). All references were `internal/`-scoped per Go's import-path rule — non-breaking.

  **Multi-agent review hardening** (Claude + Codex convergent finding on this PR's review cycle): path discovery is **stricter than the pure ts.hocon mirror** to avoid silent data loss. The new `skipToIncludePath` helper only advances over genuine include-syntax noise tokens (bare `(`, `file`/`url`/`classpath` with optional `(`-prefix). Encountering a statement-boundary token (`,` / `}` / `=` / `:` / `+=` / `{` / a bare identifier / a number) before the quoted path string raises a parse error rather than silently scanning forward. Pre-hardening, inputs like `include file() , b = "x"` would have silently turned into `include "x"` (dropping `b`). The same helper detects `file()` inside `required ( file("foo"))` (whitespace-separated nested form) and correctly sets `IsFile=true` on the include node — this is a divergence from ts.hocon (which misses this case, tracked upstream). Same-token resource-name detection inside `required(...)` is also tightened: `strings.HasPrefix("file")` / `"url"` / `"classpath"` (which false-matched `fileX(` / `urlencode(` / etc.) is replaced with exact `file(` / `url(` / `classpath(` prefix checks. Unknown resource names inside `required(...)` (e.g. `required(abc("..."))`) now raise an explicit error instead of being silently treated as bare-path includes. New regression test: `TestIncludeFile_DoesNotSilentlyMaskMalformedIncludes` (5 cases).

  **Behavior change — permissive close-paren**: `include file("foo"` (missing close `)`) and `include required(file("foo")` (missing outer `)`) now parse successfully instead of raising a parse error. This matches Lightbend's lenient handling and ts.hocon's behavior. Tests `TestIncludeFile_MissingClosingParen` and `TestIncludeRequired_MissingOuterClosingParen` were removed for this reason.

  **Behavior change — error message wording**: `include file(42)` / `include file()` / `include file({a:1})` still parse-error, but the error message changed from `"filename string"` to `"expected include path string in include file(...) directive"` (reflects the skip-until-quoted-string strategy). `TestIncludeFile_NonStringArgument` updated.

  Cross-impl pin: 6 new conformance fixtures `testdata/hocon/unquoted-parens/up01-up06` (mid-token, leading, real-world prose, nested, both unbalanced directions) wired in `s8_unquoted_parens_test.go`. Lightbend typesafe-config 1.4.3 ground truth.

  xx.hocon SHA pin bumped to `5b9c1ba`.

- **Testdata sync (free-rider)**: ev12a / ev12b / ev13 fixtures from xx.hocon's env-var-list suite (cluster 3g, pinned in xx.hocon SHA `5beedfa`) were not committed to this repo during the v1.3.0 cycle. The fresh `make testdata` run on this branch surfaced them; they are committed here so subsequent fetches do not present them as untracked. No behavior change — these fixtures already exercise existing pass paths.

## [1.3.0] - 2026-05-21

v1.3 is a spec-compliance bugfix release. The implementation has been corrected to match the HOCON spec and Lightbend typesafe-config reference behavior across several previously-divergent areas (E8 value-position lexing + leading-zero canonicalization + `+` reservation enforcement, concat type-checking, `include` key reservation, empty-file rejection, single-letter byte units, `.properties` object-wins, duration/bytes default unit, S13c env-var list). The spec did not change; the parser was simply wrong in places.

A subset of these fixes change observable runtime behavior. The most likely user-visible changes are **concat type-check tightening** (e.g. `[1, 2] 3` was permissively coerced to `[1, 2, 3]`; now returns `*ResolveError` per HOCON.md L373/L385) and the **`+` reservation** (`+foo` / `${a}+bar` / `+c = 1` were accepted as unquoted; now rejected per HOCON's `+=` operator reservation — matches ts.hocon/rs.hocon/Lightbend). If your `.conf` files use these patterns, audit them — mitigation is to rewrite as explicit arrays/objects or quote the affected keys/values. Other fixes have narrow practical impact — read `### BREAKING Changes` and `### Fixed` below if your CI fails to upgrade cleanly. We elected MINOR (not MAJOR) because no API or architectural changes occurred; v2.0 is reserved for parser/lexer rewrites or similar structural shifts.

### BREAKING Changes

- **E8 amendment — Lightbend reading of HOCON.md L270-276** ([xx.hocon#31](https://github.com/o3co/xx.hocon/issues/31), [xx.hocon#32](https://github.com/o3co/xx.hocon/pull/32) commit `dd102e8`).
  xx.hocon's extra-spec-conventions E8 was rewritten to adopt Lightbend's pragmatic reading of HOCON.md L270-276: "begin" = **value-position begin** (first component of a concatenation), not token-position begin at any lexer offset. go.hocon retracts the v1.2.0 strict-spec posture (see the v1.2.0 retraction note below) and now matches:

  - **Reverted BREAKING from v1.2.0** — `a = -foo`, `a = -bar`, `a = -` now lex as unquoted strings (`{"a":"-foo"}` / `{"a":"-"}`), matching Lightbend. The v1.2.0 reject was correct for the strict-spec reading at the time but is superseded by the E8 amendment. RFC 8259's JSON-number grammar requires a digit after `-`, so bare `-` / `-foo` fall outside L270's disallow scope. Implementation: `nextToken` now peeks `pos+1` when `ch == '-'` — digit → `readNumber`, else → `readUnquotedOrKeyword`.
  - **Concat-continuation now accepted** — `b = ${a}-bar` (and the symmetric `${a}.bar` / `${a}1bar` / `"foo"-bar` cases) resolves to the expected unquoted concat (e.g. `"foo-bar"`). Previously rejected by the strict `-` reject in `readNumber`. Driven by external issue [xx.hocon#31](https://github.com/o3co/xx.hocon/issues/31) — first issue from outside o3co (@cgordon).
  - **F3 BREAKING** — `a = 01` now resolves with `ScalarNode.Raw = "1"` (canonicalized via `strconv.ParseInt` → `FormatInt` in `parser.parseSingleValue`'s `TokenInt` case; was raw `"01"`). The canonicalization also handles negative zero: `a = -0` now resolves with `Raw = "0"`. **BREAKING surface**: `GetString("a")` returns `"1"` / `"0"` (was `"01"` / `"-0"`); the same normalization reaches **value-concat strings** that begin with an integer token (e.g. `a = 01s` was `GetString` `"01s"`, now `"1s"`; `a = 0123foo` was `"0123foo"`, now `"123foo"`). `GetInt64`, `GetFloat64`, JSON serialization, `Unmarshal`, `parseDuration`, `parseBytes` are unchanged — they all already produced the canonical integer or were identity on the canonical form. **Migration**: callers comparing `GetString` against literal `"01"` / `"-0"` / `"01s"` / `"0123foo"` text must update to compare the canonical form, or read the original `.conf` text themselves. Matches Lightbend's `parseLong` behavior. Note: normalization is **value-position only** — `parseKey` reads the same `TokenInt.Value` upstream, so the lexer keeps verbatim digit text. `01 = x` continues to parse under key `"01"` (NOT renamed to `"1"`); the same holds for numeric-key concat like `01abc = x` → key `"01abc"`.
  - **`+` reservation now enforced** at value-start (`a = +foo`), concat-continuation (`b = ${a}+bar`), and key position (`+c = 1`) per HOCON's `+=` operator reservation. **BREAKING**: pre-E8 go.hocon **accepted** these as unquoted strings or keys (a pre-existing gap vs. ts/rs/Lightbend — go.hocon's bare-`+` dispatch called `readUnquoted("+", ...)`). The E8 PR closes the gap by emitting `TokenError` from the bare-`+` dispatch. **Migration**: callers relying on `+foo` parsing as the string `"+foo"` must switch to a quoted form (`a = "+foo"`); same for keys (`"+c" = 1`). Side effect: us15 `a = 1e+x` is now caught by the same gate (the `+` left after exponent-backtrack hits the bare-`+` dispatch), so us15 moved from "known gap" to the regular error fixtures.
  - Path-element strict checks preserved (out of E8 scope): `parseSubstBody`'s segment-start `-` check (internal/lexer/lexer.go) and the per-segment check in `parseKey` (internal/parser/) — these police path-element composition, not value-position unquoted strings. Tests `${-foo}` and `a.-foo = 1` still throw a lex/parse error.

- **S10.4/S10.13/S10.19 concat type-check tightening (Phase 6 #3b)**: `joinPair` in the resolver now raises `*ResolveError` for every spec-disallowed type pair in value concatenation. Previously `[1, 2] 3` resolved to `[1, 2, 3]` (permissive scalar-append to array); it now returns a `*ResolveError`. The same applies to: array+object (`[1] {b:2}`), object+array (`{b:2} [1]`), object+scalar (`{b:1} x`), scalar+object (`x {b:1}`), and substitution-resolved equivalents (S10.19: `a = [1] ${obj}` where `obj` is a non-numerically-keyed object). Matches Lightbend semantics per HOCON.md L373 and L385. **Migration**: users who depended on permissive scalar-to-array concat must rewrite as explicit arrays (`[1, 2, 3]`) or use separate assignments. The Phase 6 #2 numeric-keyed object bridge (`obj = {"0":"x","1":"y"}; a = [1] ${obj}` → `[1, "x", "y"]`) is preserved unchanged. Fixtures: xx.hocon ce01–ce15. Closes #63.

### Fixed

- **S3.1 empty file rejection (Phase 6 #3h)**: `ParseString("")`, `ParseFile` on empty/whitespace/comment-only/BOM-only inputs now return a `*parser.Error` per HOCON.md L130 ("Empty files are invalid documents"). The check lives in `parseRoot` after `skipNewlines`: if the first non-newline token is EOF, the document is rejected. Explicit empty object `{}` and files with at least one field are unaffected. Fixtures: xx.hocon ef01–ef06 (SHA 5beedfa). Closes #75.

- **S21.4 single-letter byte abbreviations as powers of two (Phase 6 #3h)**: `K`/`k`, `M`/`m`, `G`/`g`, `T`/`t`, `P`/`p`, `E`/`e` added to the `parseBytes` multiplier map with their respective powers-of-two values per HOCON.md L1385 (java -Xmx convention). Previously these returned `unknown byte unit` errors. Overflow-checked multiplication added for both integer path (8E and above return `"byte size overflows int64 representable range"` error) and fractional path (result checked against `math.MaxInt64`/`math.MinInt64`). `Z`/`z`/`Y`/`y` deferred (overflow i64 — future cluster). Fixtures: xx.hocon bsl01–bsl09; ub05-bytes-with-unit expected updated to 1048576. Closes #73.

- **S23.4 .properties object-wins rule (Phase 6 #3h)**: `propsToObjectVal` in `internal/resolver/resolver.go` now correctly applies HOCON.md L1485: when a dotted key conflicts with a scalar key after sorted iteration, the object wins. Two bugs fixed: (1) non-leaf segment with existing scalar previously `break`ed (dropped the insertion); now replaces the scalar with a new object and descends. (2) leaf segment with existing object previously unconditionally wrote the scalar; now skips (object wins). Sort-based key processing (`sort.Strings`) was already in place; results are deterministic regardless of input line order. Fixtures: xx.hocon pc01–pc04. Closes #84.

- **S18.4 + S19.1 + S19.2 duration/bytes accessors (Phase 6 #3d)**: `GetDuration`/`GetBytes` now interpret bare numbers and strings-with-no-unit as default unit (ms / bytes per HOCON.md L1301/L1341). Adds `nano`/`nanos` aliases (S19.1) and microsecond units `us`/`micro`/`micros`/`microsecond`/`microseconds` (S19.2). `GetBytes` rejects negative byte sizes at accessor layer (panic); `GetBytesOption` returns None for negative. Whitespace trimming uses HOCON_WS predicate (not `unicode.IsSpace`). Lightbend-faithful per-family fractional: duration → float64*nanos; bytes → int64(f)*mult (truncate toward zero). Free rider: S18.1 number-value default-unit fixed in same `parseDuration` path. Fixtures: ud01-ud08, ub01-ub06, un01-un03. Closes #81, #82.

- **S12.5: reject `include` as start of key path expression (Phase 6 #3e)**: `include.foo = 1` now raises a `*parser.Error` (`'include' is reserved at the start of a key path; use "include" (quoted) or rename the key — HOCON.md L570`). Previously silently accepted, producing key `["include","foo"]`. Bare `include = x` was already rejected; the error message is now spec-aligned. Quoted `"include" = 1` and non-initial `foo.include = 1` continue to work. Side-fix: `include unquoted-file` (S14a.10, #80) — `parseInclude` now requires the argument to be a quoted `TokenString`; unquoted arguments raise *parser.Error. Also fixes `a = include` (value-position `include`) which previously caused "unexpected token" — now correctly parsed as unquoted string `"include"`. Fixtures: xx.hocon ir01–ir14. Closes #67. Side-closes #80.

### Added

- **S13c — env-var list expansion (`${X[]}` / `${?X[]}`)**: `${NAME[]}` now resolves by scanning `NAME_0`, `NAME_1`, … in the environment until the first absent index and returning a string array. `${?NAME[]}` with no `_0` element removes the key (optional); `${NAME[]}` with no `_0` element is a `ResolveError` (required). Config-defined values take precedence over the env-var list lookup (extra-spec convention E6: config wins). ASCII space or tab between the path expression and `[]` is accepted (extra-spec convention E7; e.g. `${NAME []}`). Empty-string element values are preserved as array elements — the stop condition is the environment key being *absent*, not the value being empty. `${NAME[]}` inside an included file under a nested scope falls back from the relativized `outer.NAME_*` form to the bare `NAME_*` form (matching existing scalar env-var fallback order). S13c.5: when `[]` suffix is present, the bare scalar env key is never consulted as fallback. Fixtures: `testdata/hocon/env-var-list/ev01–ev11` — all 10 success + 1 error fixture pass. (The cross-impl spec originally classified ev08-self-append as a tripwire pending cluster 3f / S13a.13, but a multi-impl probe — ts/rs/go all pass naturally — confirmed the assumption was wrong: ev08's `x = ["x"]; x = ${?x} ${?LIST[]}` has a clear prior value for `x` and does not exercise the S13a.13 "no prior value" lookback gap.)

## [1.2.0] - 2026-05-18

### Changed

- **BREAKING (S8.6)**: `a = -foo`, `a = -bar`, `a = -` and other `-`-not-followed-by-digit inputs are now lex errors. Per HOCON.md L270–276, a leading `-` must begin a number literal (i.e. be followed by a digit). Previously these were silently accepted (`-foo` tokenized as `TokenInt("-") + TokenString("foo")` and then value-coerced). The same rule is applied per-segment in `parseKey` after dot-split, so `a.-foo = 1` is now rejected. `readNumber` now implements **greedy-with-backtrack** per the HOCON.md number grammar (fractional/exponent productions backtrack to the last valid number end), so `1ex`/`1.x`/`0xff` etc. emit `TokenInt(1)` followed by `TokenString("ex")`/`TokenString(".x")`/`TokenString("xff")` respectively (the value-concat result matches Lightbend's output). Mitigation for the breaking case: quote the value (`a = "-foo"`). Note: this is intentionally stricter than Lightbend's reference implementation, which falls back to unquoted on number-parse failure. Digit-leading inputs that resolve to strings via value-concat are unaffected. See `docs/spec-compliance.md` §S8.6 for the remaining gaps tracked under #60 (digit-leading strict rejection: us13 `01`, us15 `1e+x`). The parser numeric-key support for us08 and us09 is delivered separately in the Fixed section below (#81-followup).

  > **⚠ Retracted by E8 amendment (2026-05-20)**: the value-position `-` reject described above was reverted in the [1.3.0] section. xx.hocon E8 was rewritten to adopt Lightbend's pragmatic reading of HOCON.md L270-276 ([xx.hocon#31](https://github.com/o3co/xx.hocon/issues/31), driven by external report @cgordon). The `parseSubstBody` and `parseKey` dotted-key-segment strict checks are NOT retracted — those remain in force as out-of-E8-scope path-element rules.
- Substitution body tokenization: `${...}` internals are now tokenized at lex time via `parseSubstBody`. `substPlaceholder.Segments` is now `[]lexer.Segment` (text + position). `TokenOptSubstitution` removed — unified into `TokenSubstitution` with `tok.Subst.Optional`.
- `readQuotedStringBody` extracted as a shared helper used by both top-level strings and substitution quoted segments, ensuring consistent escape handling in both contexts.
- `parseSubstPath` resolver re-parse removed — segments are consumed directly from the lexer token.

### Fixed

- **S8.6 / S11.3 / S11.4 (#81-followup)**: `parseKey` now accepts `TokenFloat` as a key start (dot-split into nested path segments) and concatenates an adjacent unquoted token (`TokenString`, `TokenBool`, `TokenNull`, `TokenInclude`) with no preceding whitespace into the last key segment. This unblocks the spec-correct success cases `123abc = 1` → `{"123abc": 1}` (us08), `3.14 = "v"` → `{"3":{"14":"v"}}` (us09), `1.2.3 = x` → path `["1","2","3"]` (S11.3), `10.0foo = x` → path `["10","0foo"]` (S11.4), and `123true = 1` → `{"123true": 1}` (consistent with `123true.foo = 1` which already worked). The concat is gated to numeric-leading keys only, so quoted segments like `"a.b"c = 1` remain rejected and their literal `.` is never re-interpreted as a path separator. Lex-time S8.6 enforcement and per-segment validation for `-`-leading segments are unchanged.
- Escape sequences (`\n`, `\t`, `\/`, `\b`, `\f`, `\uXXXX`) inside `${...}` substitution paths are now decoded correctly, matching Lightbend behavior.
- Whitespace concatenation between tokens inside `${...}` (e.g. `${"a" "b"}` → key `"a b"`) now works correctly.
- `\b` (backspace) and `\f` (form-feed) escape sequences in top-level quoted strings are now supported.
- Surrogate codepoints (`\uD800`–`\uDFFF`) in `\uXXXX` escapes are now rejected, matching rs.hocon behavior.

## [1.1.1] - 2026-04-10

### Fixed

- `Unmarshal`: `hocon:"-"` now skips the field, matching the Go ecosystem convention (`encoding/json`, `encoding/xml`, `yaml.v3`, `toml`). Previously it attempted to look up a literal key `"-"`.

## [1.1.0] - 2026-04-05

### Changed

- **Scalar internal representation**: `ScalarVal` changed from `{V any}` to `{Raw string, Type ScalarType}`. Scalars now store the original text and a type discriminant instead of converted Go values. This eliminates type erasure (e.g., `0100` → `100`) and preserves original text.
- `GetString()` now returns `Raw` for **all** scalar types (number, boolean, null), matching Lightbend behavior. Previously it required a `string` type assertion.
- `GetStringSlice()` now works on arrays containing non-string scalars (returns raw text).
- Env var lookup uses raw dot-join instead of `segmentsToKey` (no quoting), matching Lightbend behavior.

### Fixed

- `include file("path")` now resolves relative to the process working directory (CWD) instead of the including file's directory, matching the HOCON spec. Bare `include "path"` continues to resolve relative to the including file's directory. This fixes the Lightbend `file-include` conformance test.
- `GetBool()` now supports `yes`/`no`/`on`/`off` (case-insensitive) per HOCON spec. Previously only `true`/`false` were accepted.
- Quoted-key include relativization: `${"a.b".c}` inside included files now resolves correctly.
- Nested include prefix composition: multi-layer includes accumulate prefixes correctly.
- Duplicate env-var lookup removed in substitution resolver.
- `resolvedCache` key normalization: quoted keys now use canonical form consistently.

### Added

- `ScalarType` enum and `ScalarVal` struct exported from `resolver` package.
- Substitution path segments: `substPlaceholder` uses `segments []string` for correct quoted-key handling.

## [1.0.0] - 2026-04-04

### Added

- Object concatenation via deep merge (was previously an error)
- Circular include detection with path normalization
- Permissive array concatenation (non-array elements pushed as items)
- `include required()` and `include required(file())` directives
- Performance benchmarks in README
- Library comparison tables in README (vs gurkankaymak/hocon, vs viper)
- Security Considerations section in README
- Known Limitations section in README

### Fixed

- Include probe order: `.properties → .json → .conf` (`.conf` wins)
- `\uXXXX` unicode escape implementation in lexer
- Error on unknown escape sequences
- Dedicated `.properties` parser for includes
- Slice Option variants (`GetStringSliceOption`, etc.) now return `None` on type mismatch instead of panic
- Copyright standardized to "1o1 Co. Ltd." across all source files
- Removed dead `math.IsInf` import

### Changed

- `go.mod` minimum version lowered from Go 1.25 to Go 1.23
- Cross-language spec alignment with ts.hocon and rs.hocon

## [0.3.0] - 2026-03-20

### Fixed
- Extensionless `include` now loads **all** matching files (`.properties`, `.json`, `.conf`) and deep-merges them, instead of stopping at the first match. Later formats override earlier ones for conflicting keys. (Closes #4)
- `.properties` file values included via `include` are now treated as strings per the HOCON spec, instead of being type-inferred as bool/int/float. (Closes #5)

### Changed
- Lightbend conformance test suite: **13/13** groups pass (previously 12/13 — `equiv03` was skipped)

## [0.2.1] - 2026-03-20

### Fixed
- `include` without file extension now probes `.properties`, `.json`, and `.conf` in order per the HOCON spec. Previously, extensionless includes failed with "no such file or directory". (Closes #1)

## [0.2.0] - 2026-03-18

### Fixed
- Normalize `\r\n` → `\n` in triple-quoted strings (Windows CRLF compatibility)
- Strip UTF-8 BOM (`\uFEFF`) from input in lexer
- Remove trailing blank line in `parser.go` (gofmt compliance)

### Added
- **lefthook** pre-commit / pre-push / commit-msg hooks
  - pre-commit: gofmt autofix, go vet, go mod tidy
  - pre-push: go test -race, golangci-lint
  - commit-msg: Conventional Commits validation
- **Codecov** coverage reporting in CI
- **GitHub templates**: bug report, feature request, pull request
- **SECURITY.md**: vulnerability reporting policy
- **doc.go**: expanded package documentation with full API sections and spec link
- **.gitignore**: standard Go project ignore rules
- **Test coverage improvements**:
  - String concatenation: whitespace preservation, no-space subst concat, self-referential array concat
  - BOM handling: ParseFile and ParseString with UTF-8 BOM
  - Object assignment modes: brace-merge vs `=` vs `+=` per HOCON spec

### Changed
- golangci-lint: `gosimple` removed (merged into `staticcheck` in v2), `gofmt` moved to formatters section
- CI: coverage step runs only on ubuntu-latest/go1.25 to avoid Windows PowerShell flag parsing issue

## [0.1.8] - 2026-03-18

### Fixed
- Replace self-generated LICENSE text with canonical Apache 2.0 from apache.org — the previous text had different wording throughout and caused pkg.go.dev to fail SPDX license detection (showing UNKNOWN instead of Apache-2.0).

## [0.1.7] - 2026-03-18

### Fixed
- `GetInt64Slice`: array elements that came from env var substitution were stored as `string`, causing a panic on direct `int64` type assertion. Now falls back to `strconv.ParseInt`.
- `Unmarshal` into `int`, `float`, `bool` struct fields: same env-var-as-string issue; now falls back to `strconv` parsing for string values.

## [0.1.6] - 2026-03-18

### Fixed
- `GetInt64`, `GetFloat64`, `GetBool` (and their `Option` variants): environment variables are always strings at the OS level. When a `${?VAR}` substitution overrode an integer/float/bool key, the getter panicked with "expected int64, got string". Now coerces string values via `strconv` at the consumer boundary.

## [0.1.5] - 2026-03-18

### Fixed
- LICENSE file formatting for pkg.go.dev detection (intermediate step; superseded by v0.1.8).

## [0.1.4] - 2026-03-18

### Fixed
- Optional substitution fallback when the prior value is itself a substitution. Example: `permissionVerifier.timeout = ${clients.default.timeout}` followed by `permissionVerifier.timeout = ${?VAR}` (VAR unset) now correctly resolves through the chain instead of dropping the key.
- Root cause: `priorValues` was a flat map keyed by bare field name, causing collisions between same-named keys in different nested objects. Fixed by adding per-`ObjectVal` `priorValues` and a parent-object lookup in `resolveSubst`.

## [0.1.3] - 2026-03-18

### Fixed
- Optional substitution (`${?VAR}`) no longer drops the key when the variable is unset and a prior literal value exists. Example: `host = "0.0.0.0"` followed by `host = ${?HOST}` with `HOST` unset now correctly returns `"0.0.0.0"` instead of panicking with "key not found".

## [0.1.2] - 2026-03-18

### Changed
- Retracted v0.1.0 via `go.mod` `retract` directive (released before LICENSE was added).

## [0.1.1] - 2026-03-18

### Added
- Full HOCON parser: objects, arrays, scalars, substitutions (`${path}`, `${?path}`), self-referential substitutions, deep-merge for duplicate keys, `+=` operator, `include` directives, triple-quoted strings.
- Duration parsing (`10ms`, `2s`, `1h`, `1d`) and byte size parsing (`1KB`, `1KiB`, `1MB`, …).
- `Config` API: `GetString`, `GetInt`, `GetInt64`, `GetFloat32`, `GetFloat64`, `GetBool`, `GetDuration`, `GetBytes`, slice variants, `GetConfig`, `Has`, `Keys`, `WithFallback` — each with a non-panicking `Option[T]` variant.
- Generic `Option[T]` type with `Some`, `None`, `Get`, `IsSome`, `IsNone`, `OrElse`.
- `Unmarshal` into structs (with `hocon` struct tag) and `map[string]any`.
- Error types: `ParseError` (with `Line`, `Col`, `FilePath`), `ResolveError` (with `Path`), `ConfigError` (with `Path`).
- Lightbend official HOCON test suite: 12/13 groups pass (equiv03 skipped — requires `.properties` parsing, out of scope for v1).
- README in English and Japanese.
- Apache 2.0 LICENSE.

[1.0.0]: https://github.com/o3co/go.hocon/compare/v0.3.2...v1.0.0
[0.3.0]: https://github.com/o3co/go.hocon/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/o3co/go.hocon/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/o3co/go.hocon/compare/v0.1.8...v0.2.0
[0.1.8]: https://github.com/o3co/go.hocon/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/o3co/go.hocon/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/o3co/go.hocon/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/o3co/go.hocon/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/o3co/go.hocon/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/o3co/go.hocon/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/o3co/go.hocon/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/o3co/go.hocon/releases/tag/v0.1.1
