# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **E11 — `include package("<id>", "<file>")` qualifier** (xx.hocon [#33](https://github.com/o3co/xx.hocon/issues/33), [#36](https://github.com/o3co/xx.hocon/pull/36)). A new include qualifier with **service-locator semantics** — looks up `.conf` files registered under a stable name via a global registry. **Not a Java classpath equivalent** (no auto-discovery, no auto `reference.conf` merge, no transitive auto-resolution). Closest analog: JVM `ServiceLoader` / JNDI. New public surface:
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
