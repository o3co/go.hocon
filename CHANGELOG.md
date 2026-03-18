# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/o3co/go.hocon/compare/v0.1.8...HEAD
[0.1.8]: https://github.com/o3co/go.hocon/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/o3co/go.hocon/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/o3co/go.hocon/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/o3co/go.hocon/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/o3co/go.hocon/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/o3co/go.hocon/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/o3co/go.hocon/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/o3co/go.hocon/releases/tag/v0.1.1
