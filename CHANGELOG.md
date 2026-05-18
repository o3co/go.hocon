# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **S13c — env-var list expansion (`${X[]}` / `${?X[]}`)**: `${NAME[]}` now resolves by scanning `NAME_0`, `NAME_1`, … in the environment until the first absent index and returning a string array. `${?NAME[]}` with no `_0` element removes the key (optional); `${NAME[]}` with no `_0` element is a `ResolveError` (required). Config-defined values take precedence over the env-var list lookup (extra-spec convention E6: config wins). ASCII space or tab between the path expression and `[]` is accepted (extra-spec convention E7; e.g. `${NAME []}`). Empty-string element values are preserved as array elements — the stop condition is the environment key being *absent*, not the value being empty. `${NAME[]}` inside an included file under a nested scope falls back from the relativized `outer.NAME_*` form to the bare `NAME_*` form (matching existing scalar env-var fallback order). S13c.5: when `[]` suffix is present, the bare scalar env key is never consulted as fallback. Fixtures: `testdata/hocon/env-var-list/ev01–ev11` (ev08 skipped pending S13a.13 self-ref-lookback fix, cluster 3f).

## [1.2.0] - 2026-05-18

### Changed

- **BREAKING (S8.6)**: `a = -foo`, `a = -bar`, `a = -` and other `-`-not-followed-by-digit inputs are now lex errors. Per HOCON.md L270–276, a leading `-` must begin a number literal (i.e. be followed by a digit). Previously these were silently accepted (`-foo` tokenized as `TokenInt("-") + TokenString("foo")` and then value-coerced). The same rule is applied per-segment in `parseKey` after dot-split, so `a.-foo = 1` is now rejected. `readNumber` now implements **greedy-with-backtrack** per the HOCON.md number grammar (fractional/exponent productions backtrack to the last valid number end), so `1ex`/`1.x`/`0xff` etc. emit `TokenInt(1)` followed by `TokenString("ex")`/`TokenString(".x")`/`TokenString("xff")` respectively (the value-concat result matches Lightbend's output). Mitigation for the breaking case: quote the value (`a = "-foo"`). Note: this is intentionally stricter than Lightbend's reference implementation, which falls back to unquoted on number-parse failure. Digit-leading inputs that resolve to strings via value-concat are unaffected. See `docs/spec-compliance.md` §S8.6 for the remaining gaps tracked under #60 (digit-leading strict rejection: us13 `01`, us15 `1e+x`). The parser numeric-key support for us08 and us09 is delivered separately in the Fixed section below (#81-followup).
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
