# E11 `include package(...)` — go.hocon API Surface and Registry Design

**Status**: Draft — awaiting ★1 Yoshi approval
**Branch**: `feat/include-package-design`
**Spec**: `extra-spec-conventions.md` §E11
**Out of scope**: implementation code, tests, perf optimization

---

## 1. Repository Survey Findings

### Package layout (relevant to E11)

| Layer | Package | Role |
| --- | --- | --- |
| Public API | `hocon` (root) | `ParseString`, `ParseFile`, `Config`, error types |
| Parser | `internal/parser` | Tokenizes + builds AST; `IncludeNode`, `parser.Error` |
| Resolver | `internal/resolver` | Walks AST → `Val` tree; all include I/O happens here |
| Support | `internal/lexer`, `internal/properties` | Lexer tokens; `.properties` format |

### Current include handling

`parseInclude()` in `internal/parser/parser.go` recognizes `file(...)`, bare `"..."`, and `required(...)` wrappers. It does NOT read files — it only builds `IncludeNode{Path, Required, IsFile}`.

File I/O happens entirely in `internal/resolver/resolver.go` in `resolveInclude()` → `loadIncludeFile()` → `parseAndResolve()`. Cycle detection uses `r.includeStack *[]string` (normalized absolute paths); the stack is shared across recursive child resolvers.

### Error conventions

- `internal/parser` uses `*parser.Error` (unexported from root, mapped to public `*ParseError` in `hocon.go`).
- `internal/resolver` uses `*resolver.ResolveError` (mapped to public `*ResolveError` in `hocon.go`).
- No sentinel `var Err...` variables exist yet; all errors are typed struct pointers.
- No `database/sql`-style registries exist yet; this would be the first.

### `doc.go`

Present; package-level documentation is in `doc.go`. No `init()` examples exist yet.

---

## 2. Design Decisions

### Decision 1 — `RegisterPackage` signature

**Chosen**: `func RegisterPackage(identifier, file string, content []byte) error`

Rationale:

- `[]byte` matches the type returned by `//go:embed` when used with `//go:embed foo.conf` into a `[]byte` var. This is the primary intended usage pattern.
- `string` content would require callers using `embed.FS` to call `fs.ReadFile(...)` and then `string(data)` — an extra step with no benefit.
- `embed.FS` as an argument was considered and rejected: it forces the registry to store a reference to the FS object and call `fs.ReadFile` at lookup time, creating a deferred-error surface. Eagerly storing `[]byte` at registration time is simpler, catches embedded-file errors at init time (not parse time), and aligns with `database/sql`'s eager-registration model.
- E11 decision 3 requires a byte-equal idempotency check — `[]byte` makes this direct with `bytes.Equal` (see Decision 4).
- The function returns `error` rather than panicking, letting `init()` callers choose their preferred error handling (see Decision 7).

**Input validation at registration time** (not deferred to parse time):

- `identifier` must be non-empty. Empty identifier → `*PackageCollisionError`-style registration error (not a parse error). Rationale: the parser already validates the identifier is a non-empty HOCON string at parse time (E11 decision 1), but `RegisterPackage` can be called before any parse; catching it eagerly gives a better error location.
- `file` is re-validated at registration time using the same `validatePackageFile` rules applied at parse time (see Decision 9). This catches misconfigured `init()` calls before a parse ever triggers the registry lookup.

**Signature** (informative, not final code):

```go
// RegisterPackage registers a HOCON content string for use by include package(...) directives.
// The identifier is an opaque registry key (by convention a Go module path such as
// "github.com/o3co/auth"). The file argument is a forward-slash-separated relative path
// within that identifier's namespace. content is the raw HOCON source bytes (typically
// loaded via //go:embed).
//
// Re-registering byte-identical content under the same (identifier, file) key is idempotent
// and returns nil. Registering different content under the same key returns *PackageCollisionError.
// Registering an empty identifier or an invalid file path returns a *RegistrationError.
func RegisterPackage(identifier, file string, content []byte) error
```

### Decision 2 — Global registry storage

**Chosen**: `sync.RWMutex` + `map[pkgKey][]byte`

Rationale:

- `sync.Map` is optimized for high-read, write-once patterns with many goroutines, but its API is untyped (`interface{}`) and makes the byte-equal comparison on idempotent re-registration awkward (need type assertion before `bytes.Equal`). A typed `map` + `sync.RWMutex` is marginally more code but more readable and auditable.
- `sync.Once` for initialization is unnecessary — a package-level `var` initialized at declaration time (`var registry = &pkgRegistry{m: make(map[pkgKey][]byte)}`) gives simpler, correct initialization without lazy-init complexity.
- The registry type is unexported; only `RegisterPackage` and `ResetPackageRegistry` (see Decision 3) touch it.

**Registry placement**: the registry lives in the root `hocon` package, in a new file `registry.go`. This keeps it adjacent to the public API (`ParseString`, `ParseFile`) and avoids making `internal/resolver` import a package-level global. Instead, `hocon.go`'s `parseWith` function passes the registry lookup function into `resolver.Options` as a callback (see Decision 9 for how the resolver consumes it).

**Key type**:

```go
type pkgKey struct {
    identifier string
    file       string
}
```

Struct keys work directly as map keys (comparable), no string concatenation needed.

### Decision 3 — Test reset API

**Chosen**: exported `ResetPackageRegistry()` with a doc comment marking it test-only.

Rationale:

- This matches the Go standard library precedent: `net/http`'s `DefaultServeMux` and similar registries expose reset/override helpers that are formally "not for production" but exported for integration testing ergonomics.
- A `RegisterPackageForTesting` alternative was considered but rejected: it would duplicate the registration logic and complicate the collision check (would "testing" registrations collide with "real" ones?).
- Per-test sub-registry (accepting a registry object) was considered and rejected: it would change `RegisterPackage` from a global side-effect call (required for the `init()` + `_ "..."` pattern) into an instance-based API, fundamentally changing the model.
- The chosen approach aligns with go.hocon's existing test patterns: tests in the root package use package-level state directly; no test-injection infrastructure exists yet. `ResetPackageRegistry()` follows the simplest path.

**Doc convention** (informative):

```go
// ResetPackageRegistry removes all registered packages from the global registry.
// It is intended for use in tests only; calling it in production code causes
// subsequent include package(...) directives to fail with lookup errors.
func ResetPackageRegistry()
```

### Decision 4 — Idempotent re-registration (E11 decision 3)

**Chosen**: `bytes.Equal(existing, content)` at registration time.

Rationale:

- Hash comparison (SHA-256 or similar) would require storing the hash in addition to or instead of the content. Storing the hash alone would prevent future inspection; storing both wastes memory. For the expected content sizes (HOCON config files, typically < 64 KB), `bytes.Equal` is fast enough and correct.
- The check fires inside `RegisterPackage`, protected by a write-lock. If the byte sequences are equal, return `nil` (idempotent). If they differ, return `*PackageCollisionError` (see Decision 5).

### Decision 5 — Collision error

**Chosen**: custom struct `*PackageCollisionError` with identifier and file fields.

Rationale:

- A sentinel `var ErrPackageCollision = errors.New(...)` is easy to `errors.Is` against but carries no diagnostic context. When an `init()` collision fires, the only information available at the call site is the identifier and file — the developer needs those to diagnose which packages are conflicting.
- The existing error convention in go.hocon uses struct pointers (`*ParseError`, `*ResolveError`) with contextual fields. A struct collision error is consistent.

**Shape** (informative):

```go
// PackageCollisionError is returned by RegisterPackage when content already
// registered under (Identifier, File) differs from the new content being registered.
// This typically indicates two different import paths or major-version forks
// registered the same E11 identifier — resolve by ensuring only one registration wins.
type PackageCollisionError struct {
    Identifier string
    File       string
}

func (e *PackageCollisionError) Error() string {
    return fmt.Sprintf(
        "hocon: package registry collision for identifier %q file %q: "+
            "two different contents registered under the same key; "+
            "check for conflicting import paths or major-version forks "+
            "that register the same identifier",
        e.Identifier, e.File,
    )
}
```

Callers can `errors.As(err, new(*PackageCollisionError))` to extract structured fields, or `err != nil` for the simple panic-on-collision `init()` pattern.

Note on Go MVS: Go's Minimum Version Selection normally picks exactly one version per module path, so true "two versions loaded" collisions are unusual. The more realistic collision scenario is two distinct import paths (forks, or `v2` major-version paths) that both call `RegisterPackage` with the same E11 identifier string. The error message names this explicitly.

### Decision 6 — API placement

**Chosen**: root `hocon` package, new file `registry.go`.

Rationale:

- A `hocon/packages` subpackage was considered: it would allow importing the registry without importing the parser. However, `RegisterPackage` is the only exported symbol needed; the providing package only calls `hocon.RegisterPackage` — one import suffices. A subpackage would add an import path consumers need to remember, with no practical benefit for the single-function surface.
- The existing API lives entirely in the root `hocon` package; placing the registry there is consistent.
- `internal/resolver` will access the registry via an injected callback in `resolver.Options` — it does NOT import the root `hocon` package (that would be a circular import). The callback approach keeps the internal packages free of root-package dependencies.

### Decision 7 — Init-time error handling

**Chosen**: `RegisterPackage` returns `error`; providing packages SHOULD panic on non-nil error in `init()`.

Rationale:

- `database/sql.Register` panics directly. This is idiomatic for "driver already registered" collisions that represent a programming error. go.hocon should follow the same convention.
- However, `RegisterPackage` itself does NOT panic — it returns the error. This allows:
  - (a) The conventional `init()` pattern: `if err := hocon.RegisterPackage(...); err != nil { panic(err) }`.
  - (b) Test code using `ResetPackageRegistry` to call `RegisterPackage` in a `t.Cleanup` context where panicking is undesirable.
  - (c) Future non-init callers (e.g., dynamic plugin loading) that want graceful error handling.

**Template for providing packages** (see Decision 8).

### Decision 8 — Documentation template for providing packages

Providing packages (those shipping HOCON config via `include package(...)`) should follow this pattern:

```go
package mylib

import (
    _ "embed"
    "github.com/o3co/go.hocon"
)

//go:embed reference.conf
var referenceConf []byte

func init() {
    // RegisterPackage makes this package's HOCON config available to any
    // application that imports _ "github.com/myorg/mylib".
    // The identifier must match the string used in include package(...) directives:
    //   include package("github.com/myorg/mylib", "reference.conf")
    if err := hocon.RegisterPackage("github.com/myorg/mylib", "reference.conf", referenceConf); err != nil {
        panic(err)
    }
}
```

App side — the application `.go` file (or `cmd/main.go`) must include a blank import for each config-providing dependency:

```go
import (
    _ "github.com/myorg/mylib" // triggers init() → hocon.RegisterPackage
)
```

**Transitive deps**: Go's runtime calls `init()` for every transitively imported package before `main()`, so cascading via `import _ "..."` is the idiomatic mechanism. Explicit delegation calls are a fallback for libraries that pre-date this convention.

**Troubleshooting — "package not found in registry"**: Unlike `database/sql` driver misses (which surface at `sql.Open` call time), go.hocon package misses surface at `hocon.ParseFile` / `hocon.ParseString` time, when the `include package(...)` directive is first encountered. The most common cause: the providing package's `init()` registered correctly, but the application binary does not transitively import that package anywhere (the `_ "github.com/myorg/mylib"` blank import is absent). The lookup-miss error message names the missing identifier and file and suggests the missing import explicitly.

### Decision 9 — File argument validation (E11 decision 6)

**Chosen**: validate in the **parser** (`internal/parser/parser.go` in `parseInclude`), at AST construction time, before any resolver involvement. The same validation logic is re-used in `RegisterPackage` for the `file` argument (see Decision 1).

Rationale:

- E11 decision 6 says violations are "parse errors". Validating at parse time is consistent with that framing and with how other include argument errors are raised (e.g., unquoted include argument → `parseInclude` returns `*parser.Error`).
- The resolver is the wrong place: by the time `resolveInclude` runs, the error has passed through the AST. Raising it at parse time gives a line/col position in the error message.

**Validation shape** (informative, in `parseInclude` after consuming the `package(...)` form):

```go
// validatePackageFile checks E11 decision 6 constraints on the <file> argument.
// Returns a descriptive error if any constraint is violated.
// Called at parse time (returns *parser.Error via newError) and at registration time.
func validatePackageFile(file string) error {
    if file == "" {
        return fmt.Errorf("include package(...) file argument must be non-empty")
    }
    if strings.HasPrefix(file, "/") {
        return fmt.Errorf("include package(...) file argument must not be an absolute path: %q", file)
    }
    if strings.Contains(file, "\\") {
        return fmt.Errorf("include package(...) file argument must use forward-slash separators: %q", file)
    }
    if strings.Contains(file, "//") {
        return fmt.Errorf("include package(...) file argument must not contain consecutive slashes: %q", file)
    }
    for _, seg := range strings.Split(file, "/") {
        if seg == "." || seg == ".." {
            return fmt.Errorf(`include package(...) file argument must not contain "." or ".." segments: %q`, file)
        }
    }
    return nil
}
```

At parse time, `parseInclude` wraps the returned error via `newError(line, col, "%s", err)` to attach source position. At registration time, `RegisterPackage` wraps it as a `*RegistrationError`. An `IncludeNode` with `IsPackage=true` in the AST is always valid per decision 6.

### Decision 10 — Cycle detection (E11 decision 8)

**Where it lives**: in `loadPackageInclude` (the package-equivalent of `loadIncludeFile`), using the shared `includeStack`.

**Current cycle key**: normalized absolute filesystem path (string). Works for `file(...)` / bare `"..."` includes. Filesystem paths cannot contain NUL bytes.

**New cycle key for `package(...)`**: length-prefixed encoding to guarantee unambiguous round-trip:

```
"package:" + len(identifier) + ":" + identifier + ":" + file
```

Example: `package:22:github.com/o3co/auth:reference.conf`

Rationale for length-prefixed encoding over NUL-byte separator: E11 decision 6 validates the `file` argument (rejects control characters implicitly via the path constraint rules), but does not explicitly reject NUL bytes from the `identifier`. Go module paths cannot contain NUL, but `RegisterPackage` accepts arbitrary identifiers. The length-prefix encoding is unambiguous regardless of identifier content and avoids any collision risk. The `"package:"` prefix guarantees no collision with absolute filesystem paths (which begin with `/`).

**Shape**: the `includeStack` stays `*[]string` — no type change. The `loadPackageInclude` function constructs the key, checks it against the stack, pushes, defers pop, then resolves the content.

```go
cycleKey := fmt.Sprintf("package:%d:%s:%s", len(identifier), identifier, file)
for _, p := range *r.includeStack {
    if p == cycleKey {
        return nil, &ResolveError{
            Message: fmt.Sprintf("circular include: package(%q, %q)", identifier, file),
        }
    }
}
*r.includeStack = append(*r.includeStack, cycleKey)
defer func() { *r.includeStack = (*r.includeStack)[:len(*r.includeStack)-1] }()
```

**Cross-kind cycles** (e.g., `file(...)` includes `package(...)` which includes the original file via `file(...)`) are detected naturally because both kinds share the same stack.

---

## 3. AST Changes

`IncludeNode` in `internal/parser/ast.go` needs three new fields:

```go
type IncludeNode struct {
    pos
    Path      string // for file/bare includes
    Required  bool
    IsFile    bool
    // E11:
    IsPackage bool   // true when qualifier is package(...)
    PkgID     string // package identifier (only when IsPackage)
    PkgFile   string // package file path (only when IsPackage)
}
```

`Path` remains the existing field for file-based includes. `IsPackage` is a distinct bool (not repurposing `IsFile`) to keep type-dispatch explicit at all callsites.

---

## 4. Resolver Integration

`resolver.Options` gains a callback:

```go
type Options struct {
    BaseDir  string
    Fallback *ObjectVal
    // PackageLookup, if non-nil, is called by resolveInclude for package(...) includes.
    // It returns the registered HOCON source content, or (nil, non-nil error) on miss.
    PackageLookup func(identifier, file string) ([]byte, error)
}
```

`hocon.go`'s `parseWith` populates `PackageLookup` from the global registry:

```go
res, err := resolver.Resolve(ast, resolver.Options{
    BaseDir:       baseDir,
    PackageLookup: globalRegistry.lookup,
})
```

`resolveInclude` dispatches on `inc.IsPackage` to call `loadPackageInclude` rather than `loadIncludeFile`. `loadPackageInclude` calls `opts.PackageLookup`, checks for miss, handles cycle detection (see Decision 10), then calls `parseAndResolve` with the content bytes.

**Required flag semantics for package includes (E11 decision 4 and 7)**: package lookup miss is ALWAYS a hard error, regardless of the `Required` flag on `IncludeNode`. E11 decision 4 establishes this unconditionally: there is no "optional package include" — if the registry key is missing, the parse fails. The `Required` field only matters for `file(...)` includes (where `Required=false` means silently ignore a missing file). `loadPackageInclude` MUST NOT consult `inc.Required` when deciding whether to error on a miss; it always errors. The `required(package(...))` form sets `Required=true` as future-proofing for any future "optional package include" toggle — today it has no additional effect beyond what decision 4 already mandates.

**Critical: `PackageLookup` propagation through child resolvers.**
`parseAndResolve` currently constructs a child `resolver` with `Options{BaseDir: filepath.Dir(filePath)}`, omitting all other options. When implementing, `PackageLookup` MUST be copied from the parent `resolver.opts` into the child options, or any `include package(...)` directive inside an included file will fail with a nil-callback panic or silent miss. The propagation pattern already used for `includeStack` (shared pointer) is the model:

```go
childResolver := &resolver{
    opts: Options{
        BaseDir:       filepath.Dir(filePath),
        PackageLookup: r.opts.PackageLookup, // must propagate
    },
    includeStack: r.includeStack, // already shared
    // ... other fields
}
```

**Lookup miss error** — when `PackageLookup` returns a miss, `resolveInclude` raises a `*ResolveError` with a message hinting at the likely-missing `_ "..."` import:

```text
resolve error: package("github.com/o3co/auth", "reference.conf") not found in registry;
  ensure the providing package is imported with _ "github.com/o3co/auth" in your application
```

---

## 5. Open Questions for ★1 (Yoshi to decide)

These are pre-implementation decisions that affect the public API surface. All other decisions above are recommendations; these five require explicit approval before implementation starts.

1. **Collision error: typed struct vs sentinel + `Is` chain?**
   Current proposal: `*PackageCollisionError` struct (consistent with `*ParseError`, `*ResolveError`). Alternative: `var ErrPackageCollision = errors.New(...)` + `fmt.Errorf("...: %w", ErrPackageCollision)` for simpler `errors.Is` matching without type assertions. Recommendation: struct (consistency with existing error API).

2. **`PackageLookup` callback vs. `internal/registry` subpackage?**
   Current proposal: inject via `resolver.Options` callback (avoids circular import; keeps resolver testable with mock lookup). Alternative: extract `internal/registry` so both root `hocon` and `internal/resolver` can import it directly without circular dependency. Recommendation: callback for now; `internal/registry` only if registry logic grows (hot-reload, per-parse override).

3. **`ResetPackageRegistry` build tag?**
   Current proposal: exported without build constraint, protected only by doc comment (consistent with `net/http` precedent). Alternative: `//go:build testing` constraint prevents production builds from linking the reset path, at the cost of integration test friction (external test packages need a special build tag). Recommendation: no build tag.

4. **`IncludeNode.Path` reuse vs. new fields?**
   Current proposal: new `IsPackage bool`, `PkgID string`, `PkgFile string` fields (explicit, no sentinel encoding in `Path`). Alternative: repurpose `Path` as `identifier + "\x00" + file` with `IsPackage` discriminant, keeping the struct smaller. Recommendation: new fields (clarity at callsites).

5. **`RegistrationError` type for invalid `identifier`/`file` at registration time?**
   `RegisterPackage` needs to return an error for empty identifier or invalid file. Current proposal: introduce `*RegistrationError{Field, Value, Reason}`. Alternative: reuse `*PackageCollisionError` shape or return plain `errors.New(...)`. Recommendation: `*RegistrationError` (distinct from collision semantics).

---

## 6. Constraint Summary

| E11 Decision | Go design mapping |
| --- | --- |
| 1 — identifier shape convention | `RegisterPackage` accepts any non-empty string; empty → `*RegistrationError` |
| 2 — two-arg form mandatory | `parseInclude` enforces; one-arg → `*parser.Error` |
| 3 — collision = error; idempotent byte-equal ok | `RegisterPackage` `bytes.Equal` check → nil or `*PackageCollisionError` |
| 4 — lookup miss = always hard error | `loadPackageInclude` → `*ResolveError` regardless of `Required` flag |
| 5 — byte-exact, case-sensitive | `pkgKey` struct comparison; no normalization |
| 6 — file arg constraints | `validatePackageFile` in `parseInclude` and `RegisterPackage` |
| 7 — `required(package(...))` follows existing required semantics | `IncludeNode.Required` preserved for future use; today has no additional effect |
| 8 — cycle detection | `includeStack` length-prefixed key `"package:N:id:file"` |
