# go.hocon/adapters

Read config files that belong to *other* programs — Properties, environment
variables, JSONC, TOML, YAML — as part of a HOCON configuration.

## Install

This is a module of its own, so it has a `go get` of its own — importing a
subpackage after `go get github.com/o3co/go.hocon` alone will not resolve:

```bash
go get github.com/o3co/go.hocon/adapters
```

Its versions are tagged `adapters/vX.Y.Z`, separately from the parser's
`vX.Y.Z`, and `adapters/go.mod` requires the core version whose API it uses.
The two are released together, so take the matching pair rather than pinning
one and letting the other float backwards.

```go
import (
    "github.com/o3co/go.hocon"
    "github.com/o3co/go.hocon/adapters/env"
)

// APP_DB__HOST=db.internal  ->  db.host
base, err := env.Load(env.Options{Prefix: "APP_"})

cfg, err := hocon.ParseFileWithOptions("app.conf",
    hocon.DefaultParseOptions().WithResolveSubstitutions(false))

merged, err := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
```

```hocon
# app.conf — ${db.host} resolves against the mounted environment
url = "postgres://"${db.host}":"${db.port}
```

## A separate module, not a separate repo

`adapters/` is its own Go module, so `github.com/o3co/go.hocon` keeps zero
dependencies: importing the parser never pulls in a TOML or YAML library.

It stays in this repository so that a change spanning the parser and an adapter
is one commit, one review and one test run. The parser already reads
`.properties` files for `include "x.properties"`, and planned work lets
`include` dispatch on other formats too — splitting that across repositories
would make every such change a two-repo dance that cannot be tested atomically.

For local development `adapters/go.mod` carries `replace ... => ../`, so the
adapters build against the parser in this checkout. Consumers ignore a
dependency's replace directive and get the required version instead, which
means **the adapters must be verified against the published core**, not
against the working tree. CI does that on every PR: the
`adapters without replace (published core)` job drops the replace and builds,
so a `require` left behind by a core change fails review instead of shipping.
Version skew is exactly how v1.10.0 shipped an adapters module that no
consumer could compile.

## Deferring resolution

`hocon.ParseFile` resolves substitutions as it parses, so a `${...}` pointing
into a foreign file fails before the fallback is ever attached. Parse the host
document with `WithResolveSubstitutions(false)`, attach the fallback, then
`Resolve` — as in the example above.

## Semantics

Foreign data is data. Ingestion is AST-level: a document is decoded and turned
straight into a HOCON value tree, with nothing rendered to HOCON text on the
way, so quoting and escaping bugs cannot occur.

Only objects, arrays, strings, numbers, booleans and null are produced — never
substitutions, concatenations or includes — so an ingested config is always
fully resolved. A `${a.b}` appearing in a foreign value stays that literal
text, because the file belongs to a program that never agreed to HOCON's
syntax.

The mapping rules, including how each format's edge cases are pinned, live in
`docs/specs/format-ingestion-mapping.md` in the `hocon` ecosystem scope. Items
are cited from code and error messages as F0.1, F2.5 and so on.

## Packages

| Package | Status | Notes |
| --- | --- | --- |
| `properties` | available | `java.util.Properties` syntax, UTF-8, dotted keys nest |
| `env` | available | Bulk-mounts a prefixed namespace; also reads `.env` files |
| `jsonc` | available | JSON with comments and trailing commas |
| `toml` | available | TOML 1.0 via `pelletier/go-toml/v2` |
| `yaml` | available | via `goccy/go-yaml` (YAML 1.2 core schema) |
| `json5` | planned | |

Plain JSON needs no adapter — HOCON is a JSON superset, so `hocon.ParseFile`
already accepts it. `json_conformance_test.go` keeps that claim honest.

Reading one environment variable needs no adapter either: HOCON's own `${?VAR}`
does that. The `env` package is for mounting a whole prefixed namespace as a
config subtree.

### Notable rules

- **Objects win.** Given `a=1` and `a.b=2` in one Properties file, the scalar
  at `a` is dropped. This matches Lightbend's `PropertiesParser` and does not
  depend on input order.
- **Environment collisions are errors.** `APP_A__B` and `APP_a__b` both map to
  `a.b`; since the environment has no meaningful order, neither silently wins.
- **Integers stay integers.** A JSON or TOML integer becomes an int64; one too
  large to fit is an error rather than a silent widening to float64.
- **`.env` is a small dialect.** `NAME=value`, optional `export `, whole-line
  `#` comments, single quotes literal, double quotes with `\n \r \t \\ \"`.
  Multi-line values and trailing comments are not supported: an unquoted value
  containing ` #` is an error rather than a guess. No `${...}` expansion.
- **Dates become strings.** HOCON has no datetime, so TOML's four date-time
  types serialise to RFC 3339 text.
- **No Norway problem.** YAML is read under the 1.2 core schema, so `no`, `yes`,
  `on` and `off` stay strings and only `true`/`false` are booleans.
- **A YAML stream must hold one document.** Decoding a multi-document stream
  would return the first and drop the rest silently, so it is an error instead.
- **YAML keys that stringify to the same text collide.** A non-string scalar
  key takes its string form, so the int `1` and the string `"1"` in one mapping
  are an error naming both — Go randomizes map iteration, and a last-one-wins
  rule would give a different config on a different run. This applies to a tree
  handed to `yaml.FromValue` as much as to a parsed document.
- **A JSONC document holds exactly one value.** Whitespace and comments may
  follow it; anything else — including a stray closer such as `{"a":1} }` — is
  an error rather than ignored text.
- **JSONC comments separate tokens.** A comment is replaced by whitespace, not
  by nothing, so `1/*x*/2` is a syntax error rather than the number `12`.

## Development

```bash
make -C adapters check   # go test -race + golangci-lint
```

Before releasing, reproduce the consumer's view — the build without the
`replace`, against the published core:

```bash
cp -R adapters /tmp/adapters-consumer
cd /tmp/adapters-consumer
go mod edit -dropreplace=github.com/o3co/go.hocon
GOFLAGS=-mod=mod go build ./...
```

## Status

Pre-1.0. The API may still change while the remaining formats land.
