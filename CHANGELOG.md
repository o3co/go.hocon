# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/o3co/go.hocon/compare/v0.3.0...HEAD
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
