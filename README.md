# go.hocon — HOCON Parser for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/o3co/go.hocon.svg)](https://pkg.go.dev/github.com/o3co/go.hocon)
[![Go Report Card](https://goreportcard.com/badge/github.com/o3co/go.hocon)](https://goreportcard.com/report/github.com/o3co/go.hocon)
[![CI](https://github.com/o3co/go.hocon/actions/workflows/test.yml/badge.svg)](https://github.com/o3co/go.hocon/actions/workflows/test.yml)
[![Lint](https://github.com/o3co/go.hocon/actions/workflows/lint.yml/badge.svg)](https://github.com/o3co/go.hocon/actions/workflows/lint.yml)
[![codecov](https://codecov.io/gh/o3co/go.hocon/branch/main/graph/badge.svg)](https://codecov.io/gh/o3co/go.hocon)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

A [Lightbend HOCON](https://github.com/lightbend/config/blob/main/HOCON.md) parser for Go. See [Spec Compliance](#spec-compliance) for the current conformance rate.

> **Implemented by [Claude](https://claude.ai/) (Anthropic)** — designed and built end-to-end with Claude Code.
> Reviewed by [GitHub Copilot](https://github.com/features/copilot) and [OpenAI Codex](https://openai.com/index/openai-codex/).

[日本語](README.ja.md)

**Library stance** -- go.hocon is a HOCON config loader, not a low-level parser API. Its purpose is reading `.hocon` files and providing typed access via the Config API (`GetString`, `GetInt`, `GetFloat64`, `GetBool`, `GetDuration`, `GetBytes`, `Unmarshal`). Internal types such as `ScalarVal` may change between minor versions.

**Cross-language conformance** -- This implementation is tested against shared expected-JSON fixtures from [o3co/xx.hocon](https://github.com/o3co/xx.hocon) alongside [ts.hocon](https://github.com/o3co/ts.hocon) and [rs.hocon](https://github.com/o3co/rs.hocon) to ensure all three implementations meet the same Lightbend HOCON specification.

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

## Performance

Measured on Apple M4 Pro with `go test -bench` (built-in Go benchmark framework). Each iteration includes parsing and a `GetString` lookup. Run `go test -bench=. -benchmem ./...` to reproduce.

| Scenario | ops/sec | Time per op |
| --- | --- | --- |
| Small config (10 keys) | ~173,000 | ~5.8 µs |
| Medium config (100 keys) | ~27,000 | ~38 µs |
| Large config (1,000 keys) | ~2,000 | ~422 µs |
| 10 substitutions | ~70,000 | ~14 µs |
| 50 substitutions | ~16,000 | ~61 µs |
| 100 substitutions | ~8,000 | ~121 µs |
| Depth 5 nesting | ~168,000 | ~6.0 µs |
| Depth 10 nesting | ~108,000 | ~9.3 µs |
| Depth 20 nesting | ~64,000 | ~15.6 µs |

For typical application configs (loaded once at startup), the parsing cost is negligible — even a 1,000-key config parses in under 1 ms.

## Comparison

✅ Full support / ⚠️ Partial / ❌ Not supported

### HOCON Implementation

| Feature | go.hocon | [gurkankaymak/hocon](https://github.com/gurkankaymak/hocon) |
| --- | :---: | :---: |
| Substitutions (`${path}`) | ✅ | ✅ |
| Optional substitutions (`${?path}`) | ✅ | ✅ |
| Include | ✅ | ✅ |
| `include required(...)` | ✅ | ❌ |
| Object/Array concatenation | ✅ | ⚠️ |
| Type coercion | ✅ | ⚠️ |
| Duration parsing (`30s`, `5m`) | ✅ | ✅ |
| Byte size parsing (`512MB`) | ✅ | ❌ |
| `+=` append | ✅ | ✅ |
| Struct unmarshal | ✅ | ❌ |
| `Option[T]` safe access | ✅ | ❌ |
| Env variable fallback | ✅ | ✅ |

### Config Framework

| | go.hocon | [viper](https://github.com/spf13/viper) |
| --- | :---: | :---: |
| **Formats** | | |
| HOCON | ✅ | ❌ |
| JSON | ✅ | ✅ |
| YAML | ❌ | ✅ |
| TOML | ❌ | ✅ |
| Env vars | ✅ (fallback) | ✅ |
| .properties | ✅ (via include) | ✅ |
| **Features** | | |
| Substitutions | ✅ | ❌ |
| File includes | ✅ | ❌ |
| Type coercion | ✅ | ✅ |
| Struct unmarshal | ✅ | ✅ |
| Watch/reload | ❌ | ✅ |
| Remote config | ❌ | ✅ |

## Spec Compliance

Conformance against the [Lightbend HOCON specification](https://github.com/lightbend/config/blob/main/HOCON.md) is tracked at item granularity in [`docs/spec-compliance.md`](docs/spec-compliance.md). The table below is a snapshot as of 2026-05-12; see [`xx.hocon/docs/compliance-matrix.md`](https://github.com/o3co/xx.hocon/blob/main/docs/compliance-matrix.md) for live cross-impl values.

| Metric | Status |
| --- | --- |
| Spec total (incl. out-of-scope) | **55.7%** |
| In-scope only | **61.6%** |
| Lightbend `equiv01`–`equiv05` + `test01`–`test13` | 13/13 passing |
| [hocon2](https://github.com/o3co/hocon2) conformance (JSON/YAML/TOML/Properties output) | 77/77 passing |

## Related Projects

| Project | Language | Registry | Description |
|---------|----------|----------|-------------|
| [ts.hocon](https://github.com/o3co/ts.hocon) | TypeScript | [npm](https://www.npmjs.com/package/@o3co/ts.hocon) | HOCON parser for TypeScript/Node.js |
| [rs.hocon](https://github.com/o3co/rs.hocon) | Rust | [crates.io](https://crates.io/crates/o3co-hocon) | HOCON parser for Rust |
| [hocon2](https://github.com/o3co/hocon2) | Go | [pkg.go.dev](https://pkg.go.dev/github.com/o3co/hocon2) | HOCON → JSON/YAML/TOML/Properties CLI |

The three parser implementations ([ts.hocon](https://github.com/o3co/ts.hocon), [rs.hocon](https://github.com/o3co/rs.hocon), [go.hocon](https://github.com/o3co/go.hocon)) are all tracked against the same Lightbend HOCON spec — see the [cross-impl roll-up](https://github.com/o3co/xx.hocon/blob/main/docs/compliance-matrix.md) for per-impl conformance rates.

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

## Known Limitations

- **`include url(...)`** is not supported. Fetching remote configuration is outside the scope of this parser. Use your application's HTTP client to fetch the content, then pass it to `ParseString()`.
- **`include classpath(...)`** is not supported. This is a JVM-specific include form with no equivalent outside Java runtimes.
- **No watch/reload** — the library parses config at load time. For live-reloading, re-call `ParseString()` or `ParseFile()` on change.
- **No streaming parser** — the entire input is loaded into memory.
- **`.properties` include** — supports basic `key=value` syntax. Does not support multiline values (backslash continuation), unicode escapes, or key escaping from the full Java .properties specification.

For full API documentation, see [pkg.go.dev](https://pkg.go.dev/github.com/o3co/go.hocon).

## Security Considerations

When parsing untrusted HOCON input, be aware of:

- **Path traversal in includes:** `include "../../../etc/passwd.conf"` will resolve relative to `BaseDir`. Validate include paths if parsing untrusted input.
- **Input size:** The parser has no built-in input size limit. For untrusted input, validate size before calling `ParseString()`.

## License

Apache License 2.0 — see [LICENSE](LICENSE).

Copyright 2026 1o1 Co. Ltd.
