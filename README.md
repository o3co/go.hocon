# go.hocon

[![Go Reference](https://pkg.go.dev/badge/github.com/o3co/go.hocon.svg)](https://pkg.go.dev/github.com/o3co/go.hocon)
[![Go Report Card](https://goreportcard.com/badge/github.com/o3co/go.hocon)](https://goreportcard.com/report/github.com/o3co/go.hocon)
[![CI](https://github.com/o3co/go.hocon/actions/workflows/test.yml/badge.svg)](https://github.com/o3co/go.hocon/actions/workflows/test.yml)
[![Lint](https://github.com/o3co/go.hocon/actions/workflows/lint.yml/badge.svg)](https://github.com/o3co/go.hocon/actions/workflows/lint.yml)
[![codecov](https://codecov.io/gh/o3co/go.hocon/branch/main/graph/badge.svg)](https://codecov.io/gh/o3co/go.hocon)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

A full [Lightbend HOCON](https://github.com/lightbend/config/blob/main/HOCON.md) spec-compliant Go library.

> **Implemented by [Claude](https://claude.ai/) (Anthropic)** — designed and built end-to-end with Claude Code.
> Reviewed by [GitHub Copilot](https://github.com/features/copilot) and [OpenAI Codex](https://openai.com/index/openai-codex/).

[日本語](README.ja.md)

---

## Quick Start

### 1. Install

```bash
go get github.com/o3co/go.hocon
```

Requires Go 1.21+.

### 2. Use

```go
import "github.com/o3co/go.hocon"

cfg, err := hocon.ParseString(`
  server {
    host = "localhost"
    port = 8080
  }
`)
if err != nil {
    log.Fatal(err)
}

host := cfg.GetString("server.host")  // "localhost"
port := cfg.GetInt("server.port")     // 8080
```

## Why HOCON?

| | `.env` | JSON | YAML | HOCON |
|---|---|---|---|---|
| Comments | No | No | Yes | Yes |
| Nesting | No | Yes | Yes | Yes |
| References / Substitution | No | No | No | Yes (`${var}`) |
| File inclusion | No | No | No | Yes (`include`) |
| Object merging | No | No | Anchors (fragile) | Yes (deep merge) |
| Optional values | No | No | No | Yes (`${?var}`) |
| Trailing commas | N/A | No | N/A | Yes |
| Unquoted strings | Yes | No | Yes | Yes |

HOCON gives you the readability of YAML, the structure of JSON, and features that neither has — substitutions, includes, and deep merge. If your config is more than a few flat key-value pairs, HOCON is worth considering.

## Features

- Full HOCON parsing: objects, arrays, scalars, substitutions (`${path}`, `${?path}`)
- Self-referential substitutions (`path = ${path} ["/extra"]`)
- Deep-merge for duplicate keys (last definition wins)
- `+=` append operator
- `include "file.conf"` and `include file("file.conf")` directives
- Triple-quoted strings (`"""..."""`)
- Duration parsing (`10ms`, `2s`, `1h`, `1d`)
- Byte size parsing (`1KB`, `1KiB`, `1MB`, …)
- Generic `Option[T]` for safe optional access
- Struct unmarshalling with `hocon` struct tags
- No external dependencies — standard library only

## API

### Parsing

```go
hocon.ParseString(input string) (*Config, error)
hocon.ParseFile(path string)    (*Config, error)
```

### Scalar Getters

| Method | Returns | Panics if |
|--------|---------|-----------|
| `GetString(path)` | `string` | missing, null, wrong type |
| `GetInt(path)` | `int` | missing, null, wrong type |
| `GetInt64(path)` | `int64` | missing, null, wrong type |
| `GetFloat64(path)` | `float64` | missing, null, wrong type |
| `GetFloat32(path)` | `float32` | missing, null, wrong type |
| `GetBool(path)` | `bool` | missing, null, wrong type |
| `GetDuration(path)` | `time.Duration` | missing, null, invalid format |
| `GetBytes(path)` | `int64` | missing, null, invalid format |

Each has a corresponding `GetXxxOption(path) Option[T]` variant that returns `None` instead of panicking.

### Slice Getters

```go
cfg.GetStringSlice(path)   []string
cfg.GetInt64Slice(path)    []int64
cfg.GetIntSlice(path)      []int
cfg.GetConfigSlice(path)   []*Config
```

Each has a `GetXxxSliceOption` variant.

### Object Access

```go
sub := cfg.GetConfig("server")          // *Config scoped to "server"
opt := cfg.GetConfigOption("server")    // Option[*Config]
```

### Inspection

```go
cfg.Has("server.host")  // true even for null values
cfg.Keys()              // direct child keys, in declaration order
```

### Fallback Merge

```go
merged := overrides.WithFallback(defaults)
// overrides win; defaults fill in missing keys
```

### Option[T]

```go
opt := cfg.GetStringOption("key")
if opt.IsSome() {
    v, _ := opt.Get()
}
v := opt.OrElse("default")
```

### Unmarshal

```go
type ServerConfig struct {
    Host    string        `hocon:"host"`
    Port    int           `hocon:"port"`
    Timeout time.Duration `hocon:"timeout,omitempty"`
    Tags    []string      `hocon:"tags"`
}

var s ServerConfig
err := cfg.Unmarshal(&s)

// map[string]any also supported
m := make(map[string]any)
err = cfg.Unmarshal(&m)
```

Fields without a `hocon` tag use the lowercased field name. `omitempty` preserves the pre-populated value when the key is missing.

### Error Types

```go
var pe *hocon.ParseError   // lexing/parsing failure — has Line, Col, FilePath
var re *hocon.ResolveError // substitution/include failure — has Path
var ce *hocon.ConfigError  // GetXxx panic payload — has Path
```

## HOCON Examples

```hocon
# Comments with # or //
database {
  host = "db.example.com"
  port = 5432
  url  = "jdbc:"${database.host}":"${database.port}  // substitution + concat
}

# Duplicate keys deep-merge (last wins for scalars)
server { host = localhost }
server { port = 8080 }      // result: { host: localhost, port: 8080 }

# Self-referential append
path = ["/usr/bin"]
path = ${path} ["/usr/local/bin"]  // ["/usr/bin", "/usr/local/bin"]

# += shorthand
items = [1]
items += [2, 3]   // [1, 2, 3]

# Include
include "defaults.conf"
include file("overrides.conf")

# Duration and byte sizes
timeout   = "30s"
cache-ttl = "5m"
max-size  = "512MiB"
```

## Spec Compliance

Tested against the [Lightbend official test suite](https://github.com/lightbend/config/tree/main/config/src/test/resources): **13/13 test groups pass** (equiv01–equiv05 + test01–test13).

Also verified via [hocon2](https://github.com/o3co/hocon2) conformance tests (77/77 pass across JSON, YAML, TOML, and Properties output).

## Related Projects

| Project | Language | Description |
|---------|----------|-------------|
| [rs.hocon](https://github.com/o3co/rs.hocon) | Rust | HOCON parser for Rust |
| [ts.hocon](https://github.com/o3co/ts.hocon) | TypeScript | HOCON parser for TypeScript/Node.js |
| [hocon2](https://github.com/o3co/hocon2) | Go | CLI tools to convert HOCON → JSON/YAML/TOML/Properties |

All implementations are full Lightbend HOCON spec compliant.

## Best Practices

### Config Structure

- **Split by domain**: Separate configuration into logical units (`database.conf`, `server.conf`, `logging.conf`)
- **Use `include` for composition**: Compose a full config from domain-specific files
- **Avoid logic in config**: HOCON is for declarative data, not conditionals or computation

### Environment Variables

- **Minimize `${ENV}` usage**: Prefer `${?ENV}` (optional) with sensible defaults defined in the config itself
- **Never require env vars for local development**: Defaults should work out of the box
- **Document required env vars**: List them in your project's README or a `.env.example`

### Dev / Prod Separation

```text
config/
├── application.conf    # shared defaults
├── dev.conf            # include "application.conf" + dev overrides
└── prod.conf           # include "application.conf" + prod overrides
```

### Validation

- Always validate config at application startup, not at point-of-use
- Use schema validation (Zod for TypeScript, struct unmarshalling for Go, Serde for Rust) to catch errors early

```go
conf, err := hocon.ParseString(`
server {
  host = "localhost"
  port = 8080
}
debug = true
`)
if err != nil {
    log.Fatal(err)
}

var app struct {
    Server struct { Host string; Port int } `hocon:"server"`
    Debug  bool                             `hocon:"debug"`
}
if err := conf.Unmarshal(&app); err != nil {
    log.Fatal(err) // fails fast on startup
}
```

## License

Apache License 2.0 — see [LICENSE](LICENSE).

Copyright 2026 1o1 Co. Ltd.
