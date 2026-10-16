# go.hocon — HOCON Parser for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/o3co/go.hocon.svg)](https://pkg.go.dev/github.com/o3co/go.hocon)
[![Go Report Card](https://goreportcard.com/badge/github.com/o3co/go.hocon)](https://goreportcard.com/report/github.com/o3co/go.hocon)
[![CI](https://github.com/o3co/go.hocon/actions/workflows/test.yml/badge.svg)](https://github.com/o3co/go.hocon/actions/workflows/test.yml)
[![Lint](https://github.com/o3co/go.hocon/actions/workflows/lint.yml/badge.svg)](https://github.com/o3co/go.hocon/actions/workflows/lint.yml)
[![codecov](https://codecov.io/gh/o3co/go.hocon/branch/develop/graph/badge.svg)](https://codecov.io/gh/o3co/go.hocon)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

A [Lightbend HOCON](https://github.com/lightbend/config/blob/main/HOCON.md) parser for Go. See [Spec Compliance](#spec-compliance) for the current conformance rate.

> **Implemented by [Claude](https://claude.ai/) (Anthropic)** — designed and built end-to-end with Claude Code.
> Reviewed by [GitHub Copilot](https://github.com/features/copilot) and [OpenAI Codex](https://openai.com/index/openai-codex/).

**Docs:** [o3co.github.io/hocon](https://o3co.github.io/hocon/) — the cross-language hub for all four implementations, the hocon2 CLI and the shared conformance corpus.

[日本語](README.ja.md)

**Library stance** -- go.hocon is a HOCON config loader, not a low-level parser API. Its purpose is reading `.hocon` files and providing typed access via the Config API (`GetString`, `GetInt`, `GetFloat64`, `GetBool`, `GetDuration`, `GetBytes`, `Unmarshal`). Internal types such as `ScalarVal` may change between minor versions.

**Cross-language conformance** -- This implementation is tested against shared expected-JSON fixtures from [o3co/xx.hocon](https://github.com/o3co/xx.hocon) alongside [ts.hocon](https://github.com/o3co/ts.hocon), [rs.hocon](https://github.com/o3co/rs.hocon), and [py.hocon](https://github.com/o3co/py.hocon) to ensure all four implementations meet the same Lightbend HOCON specification.

---

## Quick Start

### 1. Install

```bash
go get github.com/o3co/go.hocon
```

Requires Go 1.23+.

### 2. Use

```go
import "github.com/o3co/go.hocon"

type App struct {
    Server struct {
        Host string `hocon:"host"`
        Port int    `hocon:"port"`
    } `hocon:"server"`
}

cfg, err := hocon.ParseString(`
  server {
    host = "localhost"
    port = 8080
  }
`)
if err != nil {
    log.Fatal(err)
}

var app App
if err := cfg.Unmarshal(&app); err != nil {
    log.Fatal(err) // fails fast: missing required field, wrong type
}
// app.Server.Host == "localhost", app.Server.Port == 8080

// Single values: Get*E returns (T, error)
host, err := cfg.GetStringE("server.host")
if err != nil {
    log.Fatal(err)
}
```

Prefer `Unmarshal` (whole-struct, fails fast at startup) or the
error-returning `Get*E` getters. Panicking `Get*` and `Option`-returning
`Get*Option` variants exist for code where those semantics fit — see
[Scalar Getters](#scalar-getters).

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

HOCON isn't just a serialization format — it's a **config-injection language**. JSON, YAML, and TOML describe data structures and leave file layering, environment variables, and reference resolution to your code (Pydantic, Serde, Zod, etc.). HOCON bakes those into the spec itself: by the time your program reads the config, fallback files are merged and `${VAR}` references resolved into a single composed object. Conditional branching from "is this value present in this layer?" disappears at the format boundary.

On top of that, HOCON combines the readability of YAML with the structure of JSON — making it a strong fit for anything beyond flat key-value config.

## Configuration that stays inside its module

In a large codebase, configuration tends to leak: one big file that every team edits, defaults copied into each service, and modules that read each other's keys. HOCON lets each module own its configuration the way it owns its code.

- **Each module owns a namespace and ships its own defaults.** The `auth` module keeps `auth { … }` next to its code, with every default filled in. The application never copies those defaults; it brings them in with one `include`.
- **The application overrides only what differs.** Later layers deep-merge over earlier ones, so the application states two keys, not the whole block.
- **Each module reads only its own subtree, through its own schema.** The module asks for `auth` and validates it against the schema it defines. The shape of that section is the module's contract; nothing else in the file is visible to it.
- **Wiring between modules lives in the application.** The module only declares the keys it reads (`auth.db`) and leaves them without a default when the application must supply them. The application connects them to shared values (`auth.db = ${shared.db}`). Modules never reference each other or the application's keys, so dependencies run one way.

```hocon
# auth/reference.conf: shipped inside the auth package, owned by the auth team
auth {
  session { lifetime = 30m, max-body = 64KiB }
  # db: no default; the application supplies auth.db
}
```

```hocon
# application.conf: owned by the application
include package("github.com/acme/auth", "reference.conf")

shared.db { host = db.internal, port = 5432 }
auth.db = ${shared.db}       # wiring lives in the application
auth.session.lifetime = 2h   # override what differs; the rest stays the module's default
```

```go
// inside the auth module
package auth

import (
    _ "embed"
    "fmt"
    "time"

    "github.com/o3co/go.hocon"
)

//go:embed reference.conf
var reference []byte

func init() {
    // Publish the defaults under the module's identifier.
    if err := hocon.RegisterPackage("github.com/acme/auth", "reference.conf", reference); err != nil {
        panic(err)
    }
}

// Auth is the module's contract: the shape of the auth section.
type Auth struct {
    Session struct {
        Lifetime time.Duration  `hocon:"lifetime"`
        MaxBody  hocon.ByteSize `hocon:"max-body"`
    } `hocon:"session"`
    DB struct {
        Host string `hocon:"host"`
        Port int    `hocon:"port"`
    } `hocon:"db"` // no default: the application must supply auth.db
}

// Load reads the auth subtree and nothing else.
func Load(root *hocon.Config) (Auth, error) {
    var auth Auth
    if err := root.UnmarshalPath("auth", &auth); err != nil {
        return auth, err
    }
    if auth.Session.Lifetime <= 0 {
        return auth, fmt.Errorf("auth.session.lifetime must be positive, got %v", auth.Session.Lifetime)
    }
    return auth, nil
}
```

`include package(...)` is not part of HOCON.md: it is a convention shared by the o3co implementations ([E11](https://github.com/o3co/xx.hocon/blob/main/docs/extra-spec-conventions.md#e11)), a non-JVM counterpart to `include classpath(...)`. It is how a module publishes its defaults without the application knowing where its files live (see [Package includes](#package-includes-include-package)). On the JVM, Lightbend's `reference.conf` convention plays the same role.

## Features

- Full HOCON parsing: objects, arrays, scalars, substitutions (`${path}`, `${?path}`)
- Self-referential substitutions (`path = ${path} ["/extra"]`)
- Deep-merge for duplicate keys (last definition wins)
- `+=` append operator
- `include "file.conf"` and `include file("file.conf")` directives
- Triple-quoted strings (`"""..."""`)
- Duration parsing (`10ms`, `2s`, `1h`, `1d`)
- Byte size parsing (`1kB`, `1KiB`, `1MB`, …)
- Period parsing (`7d`, `2w`, `3mo`, `1y`) into `hocon.Period`
- Generic `Option[T]` for safe optional access
- Struct unmarshalling with `hocon` struct tags
- Deferred resolution: separate `Parse` / `WithFallback` / `Resolve` lifecycle for runtime-supplied substitution values (v1.4.0+)
- `FromMap` / `Empty` value factories for in-memory fallback layers (v1.4.0+)
- Format adapters for config owned by other programs — Properties, env, JSONC, JSON5, TOML, YAML (see below)
- No external runtime dependencies — `gopkg.in/yaml.v3` used only in test code (fixture scenario runner)

## API

### Parsing

```go
hocon.ParseString(input string) (*Config, error)
hocon.ParseFile(path string)    (*Config, error)

// Deferred parse (v1.4.0+): returns an unresolved Config when the input
// contains `${...}` and you want to layer fallback values via WithFallback
// before resolving.
hocon.ParseStringWithOptions(input string, opts hocon.ParseOptions) (*Config, error)
hocon.ParseFileWithOptions(path string,    opts hocon.ParseOptions) (*Config, error)
hocon.FromMap(values map[string]any, originDescription string)     (*Config, error)
hocon.Empty(originDescription string)                              *Config
```

#### Deferred resolution example (v1.4.0+)

Use when a substitution references a value only available at runtime (e.g. CI
build number) — see [#99](https://github.com/o3co/go.hocon/issues/99).

```go
cfg, _ := hocon.ParseStringWithOptions(
    `version = ${shortversion}-${CI_RUN_NUMBER}
     variables { shortversion = "1.2.3" }`,
    hocon.DefaultParseOptions().WithResolveSubstitutions(false),
)
runtime, _ := hocon.FromMap(map[string]any{"CI_RUN_NUMBER": "42"}, "")
vars := cfg.GetConfig("variables")
resolved, _ := cfg.
    WithFallback(runtime).
    WithFallback(vars).
    Resolve(hocon.DefaultResolveOptions())
fmt.Println(resolved.GetString("version")) // "1.2.3-42"
```

Use `ResolveOptions.WithUseSystemEnvironment(false)` for hermetic resolution
(no `os.Environ` fallback) or `.WithAllowUnresolved(true)` for partial resolve
(unresolved paths remain placeholders; `Get*E` on them returns an error
satisfying `errors.Is(err, hocon.ErrNotResolved)`).

### Scalar Getters

Three families share the same path and coercion semantics; they differ only
in how a missing key / null / type mismatch comes back. **Default to the
error-returning `Get*E` family** in application code. The panicking family
suits config already validated (e.g. right after `Unmarshal` in `main`); the
`Option` family suits optional keys with a fallback (`OrElse`).

| Error-returning (recommended) | Panicking | Option |
|---|---|---|
| `GetStringE(path) (string, error)` | `GetString(path) string` | `GetStringOption(path) Option[string]` |
| `GetIntE` / `GetInt64E` | `GetInt` / `GetInt64` | `GetIntOption` / `GetInt64Option` |
| `GetFloat64E` / `GetFloat32E` | `GetFloat64` / `GetFloat32` | `GetFloat64Option` / `GetFloat32Option` |
| `GetBoolE` | `GetBool` | `GetBoolOption` |
| `GetDurationE` | `GetDuration` | `GetDurationOption` |
| `GetBytesE` | `GetBytes` | `GetBytesOption` |
| `GetPeriodE` | `GetPeriod` | `GetPeriodOption` |

`Get*E` returns a typed `*ConfigError` (missing path, null, wrong type, or
invalid duration/byte/period format); unresolved-placeholder failures satisfy
`errors.Is(err, hocon.ErrNotResolved)`. The panicking getters panic with the
same `*ConfigError` as payload. `Get*Option` returns `None` instead.

### Slice Getters

```go
cfg.GetStringSlice(path)   []string
cfg.GetInt64Slice(path)    []int64
cfg.GetIntSlice(path)      []int
cfg.GetConfigSlice(path)   []*Config
```

Each has `GetXxxSliceE` (error-returning) and `GetXxxSliceOption` variants.

### Object Access

```go
sub, err := cfg.GetConfigE("server")    // (*Config, error), scoped to "server"
mustSub := cfg.GetConfig("server")      // panicking variant
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
if v, ok := opt.Get(); ok {
    fmt.Println(v)
}
name := opt.OrElse("default")
```

### Unmarshal

```go
type ServerConfig struct {
    Host    string         `hocon:"host"`
    Port    int            `hocon:"port"`
    Timeout time.Duration  `hocon:"timeout,omitempty"`
    MaxBody hocon.ByteSize `hocon:"max-body,omitempty"`
    Tags    []string       `hocon:"tags"`
}

var s ServerConfig
err := cfg.Unmarshal(&s)

// map[string]any also supported
m := make(map[string]any)
err = cfg.Unmarshal(&m)

// UnmarshalPath decodes any node at a path — object, array, or scalar:
var servers []ServerConfig
err = cfg.UnmarshalPath("servers", &servers)
```

Fields without a `hocon` tag use the lowercased field name; `hocon:"-"` skips a field. A missing key is an error, except for an `omitempty` field (which keeps its pre-populated value) and a slice or map field (which stays nil).

A map's key type may be a string type (including a named one such as `type Key string`), a signed or unsigned integer type, or `bool`. An integer key parses strictly as a base-10 integer of the key type's size (`"80"` is a `map[uint16]` key, `"1.0"` is not), and a `bool` key accepts the spellings a bool value accepts (`true`/`false`, `yes`/`no`, `on`/`off`). A key that does not parse, or any other key type, is an error naming the key.

Unit values read with the getters' rules: a `time.Duration` field reads like `GetDuration` (`"30s"`, a bare number is milliseconds), and a `hocon.ByteSize` field reads like `GetBytes` (`"512MiB"`, a bare number is bytes, negative values are rejected). Convert with `int64(s.MaxBody)`. A `null` leaves the field unchanged, as for every scalar in `Unmarshal` (the getters return an error on `null`).

A `hocon.Period` field reads like `GetPeriod`: `"30d"`, `"2w"`, `"6mo"`, `"1y"`, or a bare integer number of days. Weeks fold into `Days`; apply it with `t.AddDate(p.Years, p.Months, p.Days)`. A `null` leaves the field unchanged, as for every scalar in `Unmarshal` (the getters return an error on `null`).

### Package includes (`include package(...)`)

`include package("<identifier>", "<file>")` includes HOCON that a Go package has registered. A library ships its defaults inside its own module, and an application includes them without knowing where the files live on disk. This is a cross-implementation extension ([E11](https://github.com/o3co/xx.hocon/blob/main/docs/extra-spec-conventions.md#e11)), not part of Lightbend HOCON.

The providing package embeds its file and registers it in `init()`:

```go
package auth

import (
    _ "embed"

    "github.com/o3co/go.hocon"
)

//go:embed reference.conf
var reference []byte

func init() {
    if err := hocon.RegisterPackage("github.com/acme/auth", "reference.conf", reference); err != nil {
        panic(err)
    }
}
```

The application imports that package so its `init()` runs before parsing — with a blank import if it uses nothing else from it:

```go
import _ "github.com/acme/auth"

cfg, err := hocon.ParseString(`include package("github.com/acme/auth", "reference.conf")`)
```

- The identifier is, by convention, the providing module's path. The file is a forward-slash relative path; `.` / `..` segments are rejected (`*hocon.RegistrationError`).
- Registering byte-identical content again is a no-op. Different content under the same identifier and file returns `*hocon.PackageCollisionError` — typically two import paths or major versions registering the same identifier.
- Including a package nobody registered is an error, never a silently empty include. `include required(package(...))` is accepted; the one-argument form `package("id")` is a parse error.
- Tests can isolate the global registry with `hocon.UnregisterPackage` / `hocon.ResetPackageRegistry`.

### Error Types

```go
var pe *hocon.ParseError   // lexing/parsing failure — has Line, Col, FilePath
var re *hocon.ResolveError // substitution/include failure — has Path
var ce *hocon.ConfigError  // Get*E returned error / GetXxx panic payload — has Path
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

# += shorthand (appends the value as a single element: a += b ≡ a = ${?a} [b])
items = [1]
items += 2        // [1, 2]
items += [3, 4]   // [1, 2, [3, 4]]  (an array RHS is appended as one nested element)

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

Verified against [gurkankaymak/hocon](https://github.com/gurkankaymak/hocon) **v1.4.0** on 2026-10-16.

| Feature | go.hocon | gurkankaymak/hocon v1.4.0 |
| --- | :---: | :---: |
| Substitutions (`${path}`) | ✅ | ✅ |
| Optional substitutions (`${?path}`) | ✅ | ✅ |
| Include | ✅ | ✅ |
| `include required(...)` / `file(...)` | ✅ | ✅ |
| Object/Array concatenation | ✅ | ✅ |
| Type coercion | ✅ | ✅ |
| Duration parsing (`30s`, `5m`) | ✅ | ✅ |
| Byte size parsing (`512MB`) | ✅ | ❌ (parses as a string) |
| `+=` append | ✅ | ✅ |
| Struct unmarshal | ✅ | ❌ |
| Non-panicking access | ✅ (`GetXxxE` + `Option[T]`) | ✅ (`GetXxxE`) |
| Env variable fallback | ✅ | ✅ |
| Deferred resolution (parse / withFallback / resolve) | ✅ (since v1.4.0) | ✅ (since v1.3.0) |
| `FromMap` / `Empty` value factories | ✅ (since v1.4.0) | ⚠️ (typed `Object{…}.ToConfig()`, no `map[string]any`) |

### Config Framework

Compared against [viper](https://github.com/spf13/viper) **v1.21.0** (still the latest release) on 2026-10-16.

| | go.hocon | viper v1.21.0 |
| --- | :---: | :---: |
| **Formats** | | |
| HOCON | ✅ | ❌ |
| JSON | ✅ | ✅ |
| YAML | ✅ (`adapters/yaml`) | ✅ |
| TOML | ✅ (`adapters/toml`) | ✅ |
| Env vars | ✅ (fallback + `adapters/env`) | ✅ |
| .properties | ✅ (via include + `adapters/properties`) | ✅ |
| **Features** | | |
| Substitutions | ✅ | ❌ |
| File includes | ✅ | ❌ |
| Type coercion | ✅ | ✅ |
| Struct unmarshal | ✅ | ✅ |
| Watch/reload | ❌ | ✅ |
| Remote config | ❌ | ✅ |

## Spec Compliance

Conformance against the [Lightbend HOCON specification](https://github.com/lightbend/config/blob/main/HOCON.md) is tracked at item granularity in [`docs/spec-compliance.md`](docs/spec-compliance.md), which is the source these rates are computed from — `TestDocs_ReadmeComplianceRates` recomputes them and fails the build if this table drifts. See [`xx.hocon/docs/compliance-matrix.md`](https://github.com/o3co/xx.hocon/blob/main/docs/compliance-matrix.md) for the cross-implementation roll-up.

| Metric | Status |
| --- | --- |
| Spec total (incl. out-of-scope) | **92.4%** |
| In-scope only | **100.0%** |
| Lightbend `equiv01`–`equiv05` + `test01`–`test13` | passing (`TestLightbendEquiv` / `TestLightbendSuite`; three `*-expected.json` comparisons in `TestLightbendExpected` are excluded with documented reasons: `test01` is environment-dependent, `test02` and `test03` are known divergences from Lightbend 1.4.6) |
| [hocon2](https://github.com/o3co/hocon2) conformance (JSON/YAML/TOML/Properties output) | passing |

## Format adapters

Config files that belong to *other* programs can be mounted as HOCON, so a
`${...}` in your document can reach into them.

`adapters/` is a **separate Go module** — that is how the parser keeps zero
dependencies — so it has an install of its own:

```bash
go get github.com/o3co/go.hocon/adapters
```

```go
import (
    "github.com/o3co/go.hocon"
    "github.com/o3co/go.hocon/adapters/env"
)

// APP_DB__HOST=db.internal  ->  db.host
base, _ := env.Load(env.Options{Prefix: "APP_"})

// Substitutions must stay unresolved until the fallback is attached.
cfg, _ := hocon.ParseFileWithOptions("app.conf",
    hocon.DefaultParseOptions().WithResolveSubstitutions(false))

merged, _ := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
```

```hocon
# app.conf — ${db.host} resolves against the mounted environment
url = "postgres://"${db.host}":"${db.port}
```

Importing the parser still pulls in nothing; only the adapter you import brings
a dependency.

| Package | Notes |
| --- | --- |
| `adapters/properties` | `java.util.Properties`, sharing this parser's `include` syntax layer |
| `adapters/env` | Bulk-mounts a prefixed namespace; also reads `.env` files |
| `adapters/jsonc` | JSON with comments and trailing commas; no dependency |
| `adapters/json5` | JSON5 documents; hand-rolled scanner, no dependency |
| `adapters/toml` | via `pelletier/go-toml/v2` |
| `adapters/yaml` | via `goccy/go-yaml` |

### Versioning

The adapters module carries its own tags, `adapters/vX.Y.Z`, pairing with the
core version they are built against; the first published tag was
`adapters/v1.11.0`. Its `go.mod` names the core version whose API it uses, and
`go get` brings that core with it.

Plain JSON needs no adapter — HOCON is a JSON superset, so `hocon.ParseFile`
accepts a `.json` file as it stands. Reading a single environment variable needs
none either; that is what `${?VAR}` is for.

Foreign data stays data: a `${a.b}` in a mounted value is literal text, never a
reference, because the file belongs to a program that never agreed to HOCON's
syntax.

### Where the adapters are strict

Ingestion refuses input whose meaning would otherwise depend on something the
caller cannot see — the order a Go map happens to iterate in, or text a decoder
happens to stop before:

- **JSONC holds exactly one value.** Whitespace and comments may follow it;
  anything else, including a stray closer such as `{"a":1} }`, is an error
  rather than silently ignored text.
- **JSONC comments separate tokens.** A comment becomes whitespace, not
  nothing, so `1/*x*/2` is a syntax error rather than the number `12`.
- **YAML keys that stringify alike collide.** A non-string scalar key takes its
  string form, so `1.0:` and `"1":`, or `~:` and `"null":`, are an error naming
  both spellings and their lines rather than one value quietly winning. A `<<:`
  merge key is exempt — it legitimately supplies a key the mapping overrides.
  The same applies to a tree handed to `yaml.FromValue`.

For YAML, scalar resolution belongs to the library rather than to this module —
whether `010` is 8 or 10 is `goccy`'s answer, not a guarantee here.
`yaml.FromValue` takes an already-decoded tree, so a caller who needs a different
library or schema decodes it themselves and hands the result over. See
[`adapters/README.md`](adapters/README.md).

## Related Projects

| Project | Language | Registry | Description |
|---------|----------|----------|-------------|
| [ts.hocon](https://github.com/o3co/ts.hocon) | TypeScript | [npm](https://www.npmjs.com/package/@o3co/ts.hocon) | HOCON parser for TypeScript/Node.js |
| [rs.hocon](https://github.com/o3co/rs.hocon) | Rust | [crates.io](https://crates.io/crates/hocon-parser) | HOCON parser for Rust |
| [py.hocon](https://github.com/o3co/py.hocon) | Python | [PyPI](https://pypi.org/project/hocon-parser/) | HOCON parser for Python |
| [hocon2](https://github.com/o3co/hocon2) | Go | [pkg.go.dev](https://pkg.go.dev/github.com/o3co/hocon2) | HOCON → JSON/YAML/TOML/Properties CLI |

The four parser implementations ([ts.hocon](https://github.com/o3co/ts.hocon), [rs.hocon](https://github.com/o3co/rs.hocon), [go.hocon](https://github.com/o3co/go.hocon), [py.hocon](https://github.com/o3co/py.hocon)) are all tracked against the same Lightbend HOCON spec — see the [cross-impl roll-up](https://github.com/o3co/xx.hocon/blob/main/docs/compliance-matrix.md) for per-impl conformance rates.

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

For full API documentation, see [pkg.go.dev](https://pkg.go.dev/github.com/o3co/go.hocon).

## Security Considerations

When parsing untrusted HOCON input, be aware of:

- **Path traversal in includes:** `include "../../../etc/passwd.conf"` resolves relative to the including file's directory (the working directory for `ParseString`). Validate include paths if parsing untrusted input.
- **Input size:** The parser has no built-in input size limit. For untrusted input, validate size before calling `ParseString()`.

## License

Apache License 2.0 — see [LICENSE](LICENSE).

Copyright 2026 1o1 Co. Ltd.
