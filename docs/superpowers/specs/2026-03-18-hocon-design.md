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
- Full Lightbend HOCON spec compliance
- Idiomatic Go API (Go 1.21+, generics)
- Panic-on-missing for required values (fail-fast for config)
- `Option[T]` for optional values
- `Unmarshal` to struct (encoding/json style)
- Lightbend official test suite integration

---

## Architecture

Two-phase design: **Parse** then **Resolve**. Parsing builds an unresolved AST; resolution expands includes, resolves substitutions, and merges objects into a final value tree.

```
Input string/file
  → Lexer     → []Token
  → Parser    → AST (unresolved nodes)
  → Resolver  → Value tree (fully resolved)
  → Config    → Public API
```

Parsing and resolution are intentionally decoupled. This separation allows the resolver to handle substitution timing, self-referential substitutions, circular reference detection, and include expansion correctly — issues that plague single-pass implementations.

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

## Internal Design

### Lexer (`internal/lexer`)

Tokenizes HOCON input, tracking line and column for error reporting.

```go
type TokenType int

const (
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

### AST (`internal/parser/ast.go`)

Represents the unresolved parse tree. Substitutions and includes are preserved as nodes for the resolver to handle.

```go
type Node interface{ node() }

type ObjectNode  struct{ Fields []FieldNode }
type FieldNode   struct{ Key []string; Value Node; Append bool } // Append = +=
type ArrayNode   struct{ Elements []Node }
type ScalarNode  struct{ Value any }          // string/int64/float64/bool/nil
type ConcatNode  struct{ Nodes []Node }       // string or array concatenation
type SubstNode   struct{ Path string; Optional bool } // ${path}, ${?path}
type IncludeNode struct{ Path string }
```

### Resolver (`internal/resolver`)

Transforms the AST into a fully resolved value tree:

- **IncludeNode** → read file → parse recursively → merge into parent ObjectNode
- **SubstNode** → resolve path in current value tree → fall back to environment variable → detect circular references via in-progress path stack
- **ConcatNode** → string concatenation or array concatenation based on type
- **Duplicate keys** → if both values are Object: recursive merge; otherwise: last value wins

Circular reference detection uses an explicit resolution stack. Attempting to resolve a path already on the stack returns an error.

### Error Types (`errors.go`)

```go
// ParseError is returned by ParseString/ParseFile
type ParseError struct {
    Message string
    Line    int
    Col     int
    Path    string // file path when inside an include
}

// ConfigError is used in panics from GetXxx methods
type ConfigError struct {
    Message string
    Path    string // HOCON path e.g. "server.host"
}
```

---

## Public API

### Entry Points

```go
func ParseString(input string) (*Config, error)
func ParseFile(path string) (*Config, error)
```

### Config — Scalar Values

Required access panics with `ConfigError` if the path is missing or the type does not match.

```go
// String
func (c *Config) GetString(path string) string
func (c *Config) GetStringOption(path string) Option[string]

// Integer — GetInt64 is the primary type; GetInt is a wrapper
func (c *Config) GetInt64(path string) int64
func (c *Config) GetInt64Option(path string) Option[int64]
func (c *Config) GetInt(path string) int
func (c *Config) GetIntOption(path string) Option[int]

// Float — GetFloat64 is the primary type; GetFloat32 is a wrapper
func (c *Config) GetFloat64(path string) float64
func (c *Config) GetFloat64Option(path string) Option[float64]
func (c *Config) GetFloat32(path string) float32
func (c *Config) GetFloat32Option(path string) Option[float32]

// Bool
func (c *Config) GetBool(path string) bool
func (c *Config) GetBoolOption(path string) Option[bool]

// HOCON duration (e.g. "10ms", "1s", "2h")
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
```

### Config — Object / Structure

```go
func (c *Config) GetConfig(path string) *Config
func (c *Config) GetConfigOption(path string) Option[*Config]
func (c *Config) Has(path string) bool
func (c *Config) Keys() []string  // direct child keys of current object
```

### Config — Merge / Unmarshal

```go
// WithFallback returns a new Config; neither receiver nor argument is mutated.
func (c *Config) WithFallback(fallback *Config) *Config

// Unmarshal maps the config into v using `hocon` struct tags.
func (c *Config) Unmarshal(v any) error
```

### Unmarshal Struct Tags

```go
type ServerConfig struct {
    Host string     `hocon:"host"`
    Port int        `hocon:"port"`
    TLS  *TLSConfig `hocon:"tls,omitempty"`
}

var s ServerConfig
err := cfg.Unmarshal(&s)
```

### Option[T]

```go
type Option[T any] struct{ ... }

func (o Option[T]) IsSome() bool
func (o Option[T]) IsNone() bool
func (o Option[T]) Get() (T, bool)
func (o Option[T]) OrElse(def T) T
```

---

## HOCON Spec Coverage (v1.0)

All features defined in the Lightbend HOCON spec:

| Feature | Notes |
|---|---|
| Comments (`#`, `//`) | |
| `=` as synonym for `:` | |
| Omitting root braces | |
| Optional commas / trailing commas | |
| Unquoted strings | |
| Triple-quoted strings | |
| Duplicate key merging | Objects merge recursively; others last-wins |
| Variable substitution `${path}` | |
| Optional substitution `${?path}` | |
| Environment variable fallback | |
| Array append `+=` | |
| Object concatenation | |
| Include directive | File-based; relative path resolution |
| Duration values | ns, ms, s, m, h, d |
| Byte size values | B, KB, KiB, MB, MiB, GB, GiB, TB, TiB |
| Circular reference detection | Error at resolution time |
| `WithFallback` merge | Immutable |

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
|---|---|
| `internal/lexer` | Token-level unit tests |
| `internal/parser` | AST node unit tests |
| `internal/resolver` | Substitution, include, circular reference unit tests |
| `hocon` (public API) | Lightbend suite + Unmarshal / Option / GetBytes integration tests |

---

## Design Decisions

**Why panic for required values?**
Config values are required for the system to start. Returning zero values silently masks misconfiguration. Panicking at startup is explicit and fail-fast — consistent with how Go programs treat programmer errors.

**Why two-phase (AST + Resolver)?**
The HOCON spec is inherently two-phase: parse first, then resolve substitutions with full context. Single-pass implementations struggle with substitution timing, self-referential values, and include ordering. Separating the phases makes each independently testable and ensures spec-correct behavior.

**Why immutable `WithFallback`?**
Config objects may be shared across goroutines. Mutation-free merging eliminates the need for synchronization and makes data flow explicit.

**Why Go 1.21+?**
Generics (1.18+) enable `Option[T]` without per-type boilerplate. Go 1.21 adds `slices`, `maps`, and `cmp` stdlib packages that simplify implementation without external dependencies.
