# HOCON Go Library Design

**Date:** 2026-03-18
**Author:** o3co Inc.
**License:** Apache 2.0
**Module:** `github.com/o3co/go.lib`
**Package:** `github.com/o3co/go.lib/hocon`
**Inspired by:** https://github.com/gurkankaymak/hocon

---

## Overview

A Go library for parsing HOCON (Human-Optimized Config Object Notation), targeting full compliance with the [Lightbend HOCON specification](https://github.com/lightbend/config/blob/main/HOCON.md). The library is designed as the first package in the `go.lib` monorepo.

**Goals:**

- Full Lightbend HOCON spec compliance (see scope below)
- Idiomatic Go API (Go 1.21+, generics)
- Panic-on-missing for required values (fail-fast for config)
- `Option[T]` for optional values
- `Unmarshal` to struct (encoding/json style)
- Lightbend official test suite integration

**Out of scope for v1.0:**

- `include url("...")` and `include classpath("...")` forms (file-based include only)
- `include required(...)` — missing include files are treated as errors unconditionally
- Substitution in keys (`${?app.name}.host = ...`)
- `CONFIG_FORCE_x` JVM system property override convention
- Microsecond (`us`) duration unit (all other HOCON duration units are supported)
- Byte size units above TB/TiB (PB, PiB, EB, EiB, ZB, ZiB, YB, YiB) — `int64` overflows above ~9 EiB; practical configs do not use these
- Custom base directory injection for `ParseString` — include directives within a string input resolve relative to `os.Getwd()`; callers needing custom base directories should write the config to a temp file and use `ParseFile` instead

---

## Architecture

Two-phase design: **Parse** then **Resolve**. Parsing builds an unresolved AST; resolution expands includes, resolves substitutions, and merges objects into a final value tree.

```
Input string/file
  → Lexer     → []Token
  → Parser    → AST (unresolved nodes)
  → Resolver  → Value tree (fully resolved, immutable)
  → Config    → Public API
```

Parsing and resolution are intentionally decoupled. This separation allows the resolver to handle substitution timing, self-referential substitutions, circular reference detection, and include expansion correctly — issues that plague single-pass implementations.

The resolved value tree is **immutable** after construction. All `*Config` values are safe for concurrent read access from multiple goroutines without synchronization.

---

## Package Structure

```
github.com/o3co/go.lib/
└── hocon/
    ├── hocon.go          // Entry points: ParseString, ParseFile
    ├── config.go         // Config type, GetXxx, Has, Keys, WithFallback
    ├── value.go          // Value types: Object, Array, Scalar
    ├── option.go         // Option[T] generic type
    ├── unmarshal.go      // Unmarshal — struct tag support
    ├── errors.go         // Error types with line/col information
    ├── internal/
    │   ├── lexer/
    │   │   └── lexer.go      // Tokenizer
    │   ├── parser/
    │   │   ├── ast.go        // AST node definitions
    │   │   └── parser.go     // AST construction
    │   └── resolver/
    │       └── resolver.go   // Substitution resolution, include processing
    └── testdata/
        ├── hocon/            // Lightbend official test suite
        └── fixtures/         // Custom edge-case fixtures
```

`internal/` packages are not exposed to consumers. The only public surface is the `hocon` package.

---

## Path Syntax

All `GetXxx(path string)` and `Has(path string)` methods accept a dot-separated path expression:

- `"server.host"` → key `host` inside object `server`
- `"arr.0"` → first element of array `arr` (integer segment for array indexing)
- Paths are case-sensitive
- An empty path `""` panics with `ConfigError`
- A trailing dot `"server."` panics with `ConfigError`
- Quoted segments are not supported in path expressions (v1.0)

Dot notation in HOCON keys (`a.b = 1`) is syntactic sugar for nested objects (`a: { b: 1 }`). Path resolution operates on the resolved object tree, not on raw key strings.

---

## Internal Design

### Lexer (`internal/lexer`)

Tokenizes HOCON input, tracking line and column for error reporting.

```go
type TokenType int

const (
    TokenInvalid TokenType = iota  // zero value sentinel — never produced by valid input
    TokenString
    TokenInt
    TokenFloat
    TokenBool
    TokenNull
    TokenLBrace, TokenRBrace
    TokenLBracket, TokenRBracket
    TokenComma, TokenColon, TokenEquals, TokenPlusEquals
    TokenSubstitution      // ${path}
    TokenOptSubstitution   // ${?path}
    TokenInclude
    TokenNewline, TokenEOF
)

type Token struct {
    Type  TokenType
    Value string
    Line  int
    Col   int
}
```

**Numeric token rules:**

- Tokens with a decimal point or exponent (`1.0`, `1e5`) → `TokenFloat`; value stores the raw string
- Pure integer literals → `TokenInt`; value stores the raw string
- The parser converts `TokenInt` → `int64`, `TokenFloat` → `float64`

**Newline handling:**
A `TokenNewline` is emitted for each line break. The parser uses newlines as value terminators within a concatenation context: a value continues onto the next line only when the current line ends mid-expression (e.g., after a `+` in a `+=` or inside an array). Otherwise a newline ends the current value.

**Unquoted string termination:**
An unquoted string token ends at any of: `:`, `=`, `{`, `}`, `[`, `]`, `,`, `+`, `#`, `\`, `^`, `?`, `!`, `@`, `&`, newline, or EOF. This follows the Lightbend HOCON spec definition of forbidden characters in unquoted strings.

**Triple-quoted strings:**
Content between `"""` delimiters is taken verbatim — no escape processing occurs (backslash is a literal character, `\n` is two characters `\` and `n`, not a newline). Leading and trailing whitespace on each line is preserved as-is. The only way to end a triple-quoted string is `"""`.

### AST (`internal/parser/ast.go`)

Represents the unresolved parse tree. Substitutions and includes are preserved as nodes for the resolver to handle.

```go
type Node interface{ node() }

type ObjectNode  struct{ Fields []FieldNode }
type FieldNode   struct{ Key []string; Value Node; Append bool } // Append = +=
type ArrayNode   struct{ Elements []Node }
type ScalarNode  struct{ Value any }           // string/int64/float64/bool/nil (null)
type ConcatNode  struct{ Nodes []Node }        // string or array concatenation
type SubstNode   struct{ Path string; Optional bool } // ${path} / ${?path}
type IncludeNode struct{ Path string }         // file-based only (v1.0)
```

**`ConcatNode` type dispatch:**
`ConcatNode` is produced only for string and array concatenation. Object concatenation is never represented as `ConcatNode`; consecutive objects at the same key are handled entirely through the duplicate-key recursive merge path. The resolver determines string vs. array mode by examining the first non-substitution resolved element:

- First resolved element is an `ArrayNode` → array concatenation; all elements must resolve to arrays, otherwise `ResolveError`
- First resolved element is a scalar or string → string concatenation; each element is coerced to its string representation
- First resolved element is an `ObjectNode` → `ResolveError` (objects cannot appear in concatenation; use duplicate-key merge instead)

### Resolver (`internal/resolver`)

Transforms the AST into a fully resolved value tree.

**Include expansion:**

Supported syntactic forms:

- `include "file.conf"` — bare quoted path (canonical Lightbend form)
- `include file("file.conf")` — explicit `file()` form

Both forms behave identically. `url(...)` and `classpath(...)` forms are out of scope for v1.0.

Resolution:

- `IncludeNode` → read file relative to the **including file's directory** (not the root) → parse recursively → merge into parent `ObjectNode`
- The resolver maintains a stack of current file paths to resolve relative includes correctly during recursive expansion
- For `ParseString` calls that contain `include` directives: the base directory defaults to the current working directory (`os.Getwd()`). If the working directory cannot be determined, resolution of relative includes returns a `ResolveError`.

**Substitution resolution (`${path}` and `${?path}`):**

- Look up `path` in the current fully-merged value tree (dot-separated segments)
- If not found: fall back to `os.Getenv(path)` using the **literal** path string (e.g. `"server.host"` → `os.Getenv("server.host")`). Note: environment variable names with dots are shell-hostile; this follows the Lightbend spec literally. A future version may add an uppercase/underscore transformation.
- If still not found:
  - `${path}` (required): return `ResolveError`
  - `${?path}` (optional): the node is removed entirely; the containing field is dropped from the object, or the element is dropped from the array

**Self-referential substitutions:**
A field may reference its own previous value to append or prepend:

```hocon
path = ["/usr/bin"]
path = ${path} ["/usr/local/bin"]  # => ["/usr/bin", "/usr/local/bin"]
```

The resolver uses the value of `path` from the fallback config (prior merge state) when resolving a self-reference. This is distinct from circular references.

**Circular reference detection:**
The resolver maintains an in-progress resolution stack. If resolving path `A` triggers resolution of path `B` which in turn requires path `A`, a `ResolveError` is returned. Self-referential substitutions (using the fallback value) do not trigger this check.

**`+=` (array append):**
`FieldNode.Append = true` means: look up the current value at the key (in fallback or prior merge), treat it as an array, and append the new value. Equivalent to `key = ${?key} [newValue]`. If the existing value is present but is not an array, a `ResolveError` is returned.

**Duplicate keys:**

- Both values resolve to `ObjectNode`: recursive merge (later keys override)
- Otherwise: last value wins

**`null` values:**
`null` in HOCON is a first-class value (`ScalarNode{Value: nil}`). It is distinct from a missing key:

- `cfg.Has("key")` returns `true` for a key explicitly set to `null`
- `cfg.GetString("key")` on a `null` value panics with `ConfigError` — reason: type mismatch (value exists but is `null`, which is not a string)
- `GetXxxOption` methods return `None` for `null` values (null is treated as "no typed value")

**`WithFallback` semantics:**
Returns a new `*Config` whose root object is the deep merge of receiver over fallback. Neither the receiver nor the fallback is mutated. If `fallback` is `nil`, the receiver is returned unchanged (no new instance is created). The resolved value tree is immutable; `WithFallback` produces a new tree via deep merge.

### Error Types (`errors.go`)

```go
// ParseError is returned when lexing or parsing fails.
type ParseError struct {
    Message  string
    Line     int
    Col      int
    FilePath string // non-empty when inside an include file
}

// ResolveError is returned when resolution fails (substitution, include, circular ref).
type ResolveError struct {
    Message  string
    Path     string // HOCON substitution path e.g. "server.host"
    Line     int    // source line where the substitution appears (0 if unavailable)
    Col      int    // source column
    FilePath string // file path when resolving an include
}

// ConfigError is used in panics from GetXxx methods.
type ConfigError struct {
    Message string
    Path    string // HOCON access path e.g. "server.host"
}
```

`ParseString` and `ParseFile` return `*ParseError` or `*ResolveError` wrapped in Go's standard `error`. Callers can type-assert for detail.

---

## Public API

### Entry Points

```go
func ParseString(input string) (*Config, error)
func ParseFile(path string) (*Config, error)
```

### Config — Scalar Values

Required access panics with `ConfigError` when:

- The path does not exist (key absent from the config)
- The value is `null` (type mismatch: null is not a typed value)
- The value's type does not match the requested Go type (e.g. calling `GetString` on an integer)

Type matching is strict: HOCON integers are not auto-coerced to float, and strings are not parsed as numbers.

```go
// String
func (c *Config) GetString(path string) string
func (c *Config) GetStringOption(path string) Option[string]

// Integer — GetInt64 is the primary type; GetInt is a wrapper
func (c *Config) GetInt64(path string) int64
func (c *Config) GetInt64Option(path string) Option[int64]
func (c *Config) GetInt(path string) int                    // int(GetInt64(path))
func (c *Config) GetIntOption(path string) Option[int]      // wrapper

// Float — GetFloat64 is the primary type; GetFloat32 is a narrowing wrapper.
// GetFloat32 performs float64→float32 conversion: out-of-range values silently
// become ±Inf (Go's standard float32 conversion behavior).
func (c *Config) GetFloat64(path string) float64
func (c *Config) GetFloat64Option(path string) Option[float64]
func (c *Config) GetFloat32(path string) float32            // float32(GetFloat64(path))
func (c *Config) GetFloat32Option(path string) Option[float32] // wrapper

// Bool
func (c *Config) GetBool(path string) bool
func (c *Config) GetBoolOption(path string) Option[bool]

// HOCON duration. Supported units: ns/nanoseconds, ms/milliseconds, s/seconds,
// m/minutes, h/hours, d/days. "d"/"days" maps to 24*time.Hour.
// Microseconds (us) are out of scope for v1.0.
func (c *Config) GetDuration(path string) time.Duration
func (c *Config) GetDurationOption(path string) Option[time.Duration]

// HOCON byte size (e.g. "100KB", "1MiB")
func (c *Config) GetBytes(path string) int64
func (c *Config) GetBytesOption(path string) Option[int64]
```

### Config — Slices

```go
func (c *Config) GetStringSlice(path string) []string
func (c *Config) GetStringSliceOption(path string) Option[[]string]

func (c *Config) GetInt64Slice(path string) []int64
func (c *Config) GetInt64SliceOption(path string) Option[[]int64]
func (c *Config) GetIntSlice(path string) []int
func (c *Config) GetIntSliceOption(path string) Option[[]int]

// Array of objects — each element becomes a *Config.
// Panics with ConfigError if any element is not an object.
func (c *Config) GetConfigSlice(path string) []*Config
func (c *Config) GetConfigSliceOption(path string) Option[[]*Config]
```

### Config — Object / Structure

```go
func (c *Config) GetConfig(path string) *Config
func (c *Config) GetConfigOption(path string) Option[*Config]

// Has returns true if the path exists, including when the value is null.
func (c *Config) Has(path string) bool

// Keys returns the direct child key names of the current object as single
// path segments (not dotted). For { a.b = 1 }, Keys() returns ["a"].
// On a *Config obtained via GetConfig("a"), Keys() returns ["b"].
// Order: receiver's keys first (insertion order), then fallback-only keys
// (insertion order) when the config was produced by WithFallback.
func (c *Config) Keys() []string
```

### Config — Merge / Unmarshal

```go
// WithFallback returns a new Config (deep merge of receiver over fallback).
// Neither receiver nor fallback is mutated.
// If fallback is nil, returns the receiver unchanged.
func (c *Config) WithFallback(fallback *Config) *Config

// Unmarshal maps the config into v using `hocon` struct tags.
// v must be a non-nil pointer to a struct.
func (c *Config) Unmarshal(v any) error
```

### Unmarshal Struct Tags

```go
type ServerConfig struct {
    Host    string      `hocon:"host"`
    Port    int         `hocon:"port"`
    TLS     *TLSConfig  `hocon:"tls,omitempty"`
    Aliases []string    `hocon:"aliases"`
    Workers []*Worker   `hocon:"workers"`
}

var s ServerConfig
err := cfg.Unmarshal(&s)
```

**Tag semantics:**

- `hocon:"key"` — maps the HOCON key to this field; panics if absent (same as `GetXxx`)
- `hocon:"key,omitempty"` — if the HOCON path is absent **or null**, the struct field is left **unchanged** (its pre-populated value is preserved, not overwritten with zero). `Has()` returns true for null, but `omitempty` treats null as "no value" for struct population purposes.
- No tag → field name lowercased is used as the key

**Type coercion during `Unmarshal`:**

- HOCON integer → Go `int`, `int64`: exact; → Go `float64`, `float32`: widening (float32 may lose precision)
- HOCON float → Go `float64`: exact; → Go `float32`: narrowing (out-of-range becomes ±Inf)
- HOCON string → Go `string` only; not coerced to numeric types
- HOCON bool → Go `bool` only
- HOCON array → Go slice: each element is coerced recursively; element type mismatch returns an error
- HOCON object → Go struct: recursive `Unmarshal` using struct tags
- HOCON array of objects → Go `[]*StructType` or `[]StructType`: each element unmarshaled as a struct
- HOCON object → `map[string]any`: supported; values are Go-native types (`string`, `int64`, `float64`, `bool`, `nil`, `[]any`, `map[string]any`). Duration and byte-size values are stored as their **raw string** (e.g. `"10s"`, `"100KB"`) — not converted to `time.Duration` or `int64`.
- All other type mismatches return an `error` (do not panic)

### Option[T]

```go
type Option[T any] struct{ /* unexported */ }

// Constructors (used internally; available to callers for testing and composition)
func Some[T any](v T) Option[T]
func None[T any]() Option[T]

func (o Option[T]) IsSome() bool
func (o Option[T]) IsNone() bool
func (o Option[T]) Get() (T, bool)
func (o Option[T]) OrElse(def T) T
```

`GetXxxOption` returns `None` for absent paths and for `null` values (null has no typed representation).

---

## HOCON Spec Coverage (v1.0)

| Feature | Status | Notes |
| --- | --- | --- |
| Comments (`#`, `//`) | Supported | |
| `=` as synonym for `:` | Supported | |
| Omitting root braces | Supported | |
| Optional commas / trailing commas | Supported | |
| Unquoted strings | Supported | Termination chars per Lightbend spec |
| Triple-quoted strings | Supported | |
| Duplicate key merging | Supported | Objects merge recursively; others last-wins |
| Variable substitution `${path}` | Supported | Error if unresolved |
| Optional substitution `${?path}` | Supported | Field dropped if unresolved |
| Self-referential substitution | Supported | Uses fallback/prior value |
| Environment variable fallback | Supported | `os.Getenv(path)` with literal path string |
| Array append `+=` | Supported | Error if existing value is not an array |
| String concatenation | Supported | |
| Array concatenation | Supported | |
| Object concatenation | Supported | Via duplicate-key recursive merge |
| Include directive (file) | Supported | Relative to including file's directory |
| Include `url(...)` / `classpath(...)` | Out of scope | v1.0 file-based only |
| `include required(...)` | Out of scope | All includes are required by default |
| Substitution in keys | Out of scope | |
| `null` values | Supported | Distinct from missing; `Has()` returns true |
| Duration values | Supported | ns, ms, s, m, h, d (d = 24h); us out of scope |
| Byte size values | Supported | B, KB, KiB, MB, MiB, GB, GiB, TB, TiB (PB+ out of scope) |
| Circular reference detection | Supported | ResolveError at resolution time |
| `WithFallback` deep merge | Supported | Immutable, new instance |
| `CONFIG_FORCE_x` overrides | Out of scope | JVM convention |

---

## Testing Strategy

### Lightbend Official Test Suite

The Lightbend config test suite is placed under `testdata/hocon/`. Each `.conf` file has a corresponding `.json` expected output. Tests scan the directory automatically:

```go
func TestLightbendSuite(t *testing.T) {
    entries, _ := os.ReadDir("testdata/hocon")
    for _, conf := range confFiles(entries) {
        t.Run(conf, func(t *testing.T) {
            cfg, err := ParseFile("testdata/hocon/" + conf)
            // compare against corresponding .json
        })
    }
}
```

### Coverage by Layer

| Layer | Scope |
| --- | --- |
| `internal/lexer` | Token-level unit tests |
| `internal/parser` | AST node unit tests |
| `internal/resolver` | Substitution, self-reference, include, circular reference, null, += error unit tests |
| `hocon` (public API) | Lightbend suite + Unmarshal / Option / GetBytes / null / GetConfigSlice integration tests |

---

## Design Decisions

**Why panic for required values?**
Config values are required for the system to start. Returning zero values silently masks misconfiguration. Panicking at startup is explicit and fail-fast — consistent with how Go programs treat programmer errors.

**Why two-phase (AST + Resolver)?**
The HOCON spec is inherently two-phase: parse first, then resolve substitutions with full context. Single-pass implementations struggle with substitution timing, self-referential values, and include ordering. Separating the phases makes each independently testable and ensures spec-correct behavior.

**Why immutable `WithFallback`?**
The resolved value tree is immutable after construction, making `*Config` safe for concurrent reads. Mutation-free merging eliminates the need for synchronization and makes data flow explicit.

**Why Go 1.21+?**
Generics (1.18+) enable `Option[T]` without per-type boilerplate. Go 1.21 adds `slices`, `maps`, and `cmp` stdlib packages that simplify implementation without external dependencies.

**Why separate `ParseError` and `ResolveError`?**
Parse errors (bad syntax) and resolve errors (missing substitution, circular reference) occur at different phases with different context. Callers may want to distinguish them — for example, to show the line/col of a syntax error vs. the substitution path of a missing variable.

**Why treat `null` as a type-mismatch panic in `GetXxx`?**
`null` in HOCON is an explicit value, not the absence of a value — `Has()` returns true. But typed getters like `GetString` cannot return a meaningful Go value for `null`. Panicking with a type-mismatch `ConfigError` is consistent with calling `GetString` on an integer: the key exists, but the type is wrong. `GetXxxOption` returns `None` for `null`, giving callers an explicit null-safe path.
