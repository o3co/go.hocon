# deferred-resolution/ — E12 conformance fixtures

This fixture group covers [E12](../../../../docs/extra-spec-conventions.md#e12)
(deferred substitution resolution — Lightbend-aligned `parse / withFallback /
resolve()` lifecycle).

**External origin**: [o3co/go.hocon#99](https://github.com/o3co/go.hocon/issues/99)
**Spec tracking**: [o3co/xx.hocon#37](https://github.com/o3co/xx.hocon/issues/37)
**Design doc**: [`.claude/superpowers/specs/2026-05-21-e12-deferred-resolution-design.md`](../../../../.claude/superpowers/specs/2026-05-21-e12-deferred-resolution-design.md)

## Why YAML and not `.conf`?

Existing fixture groups are single `.conf` files: load → parse+resolve → JSON.
E12 fixtures must express **multi-step scenarios** (parse layer A, fromMap layer
B, `withFallback` chain, then `resolve()` with options), which is structural
information that a single `.conf` cannot encode.

Each fixture is a YAML file describing the scenario. The Java generator
(`generate/src/main/java/DeferredResolutionRunner.java`) runs the scenario
through Lightbend `com.typesafe.config` and emits the expected outcome to
`expected/hocon/deferred-resolution/<name>-expected.{json,error}`. Per-impl
conformance tests read the same YAML and exercise their own public API.

## Scenario YAML schema

```yaml
# REQUIRED — one-line summary
description: <free text>

# OPTIONAL — cross-reference issues / specs
xref: [go.hocon#99, S13a, S10]

# REQUIRED — input sources (atoms), declared by id.
# Sources are pure: each is either `parseString` (with optional parseOptions)
# OR `fromMap`. Derivation (extract subconfig, resolve, etc.) happens in `build`.
sources:
  <id>:
    # exactly one of the following per source:
    parseString: |              # HOCON text to parse
      a = ${b}
      b = 1
    parseOptions:               # OPTIONAL, only valid alongside parseString
      resolveSubstitutions: false
      originDescription: "scenario dr01 receiver"
    # OR
    fromMap:                    # plain-key map for hocon.FromMap
      KEY1: "value1"
      KEY2: 42
      NESTED:
        inner: true
    originDescription: "..."    # OPTIONAL for fromMap

# REQUIRED — explicit step sequence producing the final Config under test.
# The artifact named `result` (or the last step's output, if unnamed) is what
# `expect` is asserted against.
#
# In all `op` records below, `this` / `other` / `source` arguments may reference
# either a source-id (from `sources`) or a previously-built named artifact (`as`
# from an earlier build step). Resolution is by name; sources and built
# artifacts share one namespace.
build:
  - { op: take,          source: <id>,           as: <name> }
  - { op: withFallback,  this: <name>,  other: <id-or-name>,  as: <name> }
  - { op: extract,       this: <name>,  path: <dotted.path>,  as: <name> }
  - { op: resolveWith,   this: <name>,  source: <id-or-name>, allowUnresolved: false, useSystemEnvironment: false, as: <name> }
  - { op: resolve,       this: <name>,  allowUnresolved: false, useSystemEnvironment: false, as: result }

# REQUIRED — expected outcome of the final artifact (or the named artifact)
expect:
  outcome: success | error      # required

  # When outcome=success:
  json: |-                      # OPTIONAL canonical JSON (whitespace-normalised before compare)
    { "a": 1, "b": 1 }
  isResolved: true | false      # OPTIONAL

  # When outcome=success, additional per-path assertions on the final Config:
  getter:                       # OPTIONAL list
    - { path: "a",         expectInt:    1 }
    - { path: "name",      expectString: "alice" }
    - { path: "missing",   expectError:  NotResolved }
    - { path: "obj.child", expectObject: { x: 1, y: 2 } }
    - { path: "list",      expectArray:  [1, 2, 3] }

  # When outcome=error:
  errorAt: <step-index>         # OPTIONAL; which build step is expected to throw (0-based)
  errorCategory:                # REQUIRED — see "Error category mapping" below
    ParseError | ResolveError | NotResolved | TypeError | CycleError
  errorContains: <substring>    # OPTIONAL substring match against error message
```

### `op` reference

| Op              | Args (other than `as`)                                        | Semantics                                                                                                |
| --------------- | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `take`          | `source: <id>`                                                | Make `<id>`'s source-artifact available under `as`.                                                      |
| `extract`       | `this: <name>, path: <dotted.path>`                           | `this.getConfig(path)` — extract a subconfig as a new artifact. Used to promote a nested object into its own layer (e.g. dr01's `variables` subtree). |
| `withFallback`  | `this: <name>, other: <id-or-name>`                           | `this.WithFallback(other)`. `other` may reference a source-id OR a previously-built named artifact.      |
| `resolve`       | `this: <name>, allowUnresolved?, useSystemEnvironment?`       | `this.Resolve(ResolveOptions{...})`. Boolean options use spec defaults if omitted.                       |
| `resolveWith`   | `this: <name>, source: <id-or-name>, allowUnresolved?, useSystemEnvironment?` | `this.ResolveWith(source, ResolveOptions{...})`. Source must be resolved (otherwise expect=error).       |

### Error category mapping

Cross-impl categories. Per-impl test runners map their concrete error to one of these.

| Category       | Lightbend exception                          | go.hocon                       | ts.hocon                | rs.hocon                            |
| -------------- | -------------------------------------------- | ------------------------------ | ----------------------- | ----------------------------------- |
| `ParseError`   | `ConfigException.Parse`                      | `parser.Error`                 | `ParseError`            | `HoconError::Parse(_)`              |
| `ResolveError` | `ConfigException.UnresolvedSubstitution`     | `resolver.Error` / `ResolveError` | `ResolveError`       | `HoconError::Resolve(_)`            |
| `NotResolved`  | `ConfigException.NotResolved`                | `ErrNotResolved`               | `NotResolvedError`      | `ConfigError::NotResolved`          |
| `TypeError`    | `ConfigException.WrongType`                  | (per impl)                     | `WrongTypeError`        | `HoconError::WrongType`             |
| `CycleError`   | `ConfigException.UnresolvedSubstitution` (Lightbend doesn't distinguish) | `resolver.Error` w/ cycle marker | `CycleError`     | `ResolveError::Cycle`               |

## Per-impl test runner contract

Per-impl conformance tests do:

1. **Read scenario YAML**.
2. **Resolve sources** in declaration order:
   - `parseString` → impl's `ParseStringWithOptions` (or equivalent) with `ResolveSubstitutions: false` UNLESS `parseOptions.resolveSubstitutions = true` is set explicitly.
   - `fromMap` → impl's `FromMap`.
   - `extract` → after the referenced artifact is available, extract its subconfig at `path` (impl's `getConfig`-equivalent).
3. **Execute `build` steps** in order, maintaining a name→artifact map.
4. **Validate `expect`**:
   - For `outcome: success`: the final artifact (named `result` if present, else the last step's output) must succeed all `getter` assertions and (if `json` provided) JSON-equal the expected.
   - For `outcome: error`: the build step at index `errorAt` (or any step, if `errorAt` omitted) must throw an error mapped to `errorCategory`. `errorContains` is substring-matched against the error message.

## Generator behaviour (DeferredResolutionRunner)

The Java generator runs each `.yaml` through Lightbend Java and:

- For `outcome: success` scenarios where the resulting Config is **resolved** (`isResolved=true`): emits `<name>-expected.json` (canonical sorted-key JSON) AND `<name>-expected.txt` (`isResolved` flag + getter assertion records). The `.json` file is Lightbend ground truth — per-impl tests JSON-compare against it.
- For `outcome: success` scenarios where the resulting Config is **unresolved** (`isResolved=false`, expected when `AllowUnresolved=true` and substitutions remain): emits `<name>-expected.unresolved-render.txt` (Lightbend's raw render — contains non-JSON substitution placeholder literals like `${b}`, hence the distinct extension) AND `<name>-expected.txt`. Per-impl tests rely on the `.txt` getter records, NOT the raw render.
- For `outcome: error` scenarios: emits `<name>-expected.error` plain text with `Category: <cat>\nAt: <step>\nMessage: <first-line>\n`.
- For `lightbendSkip: true` scenarios: emits `<name>-expected.skip` with the YAML's `description` as rationale. No other expected file is produced for skipped scenarios.

Mismatches between scenario YAML expectations and Lightbend's actual output (`expect.json` differs from rendered JSON, `expect.errorCategory` differs from Lightbend's exception class, `expect.errorAt` differs from actual step index, `expect.errorContains` substring missing from Lightbend's message, or `expect.isResolved` differs) are **hard failures** — the runner emits `<name>-expected.UNEXPECTED` and the build reports an error. This treats Lightbend as the cross-impl ground truth.

## Fixture index

See [design doc § "Fixture inventory"](../../../../.claude/superpowers/specs/2026-05-21-e12-deferred-resolution-design.md#fixture-inventory-30-scenarios) for the full table.

| ID | File | Outcome | Brief |
|---|---|---|---|
| dr01 | `dr01-basic-fallback.yaml` | success | Issue #99 example |
| dr02 | `dr02-frommap-only-fallback.yaml` | success | FromMap-only fallback |
| dr03 | `dr03-multi-layer-fallback.yaml` | success | 3+ fallback layers |
| dr04 | `dr04-self-ref-optional-with-fallback.yaml` | success | `a=${?a} extra` + fallback `a=base` |
| dr05 | `dr05-required-self-ref-with-fallback-prior.yaml` | success | Required self-ref resolves via fallback prior |
| dr06 | `dr06-required-self-ref-no-fallback.yaml` | error | Required self-ref no prior → CycleError (Lightbend treats as cycle since substitution looks back at the value it defines) |
| dr07 | `dr07-allow-unresolved-partial.yaml` | success | AllowUnresolved=true partial; getter assertions |
| dr08 | `dr08-no-system-env.yaml` | error | UseSystemEnvironment=false; env-only sub → ResolveError |
| dr09 | `dr09-getter-on-unresolved.yaml` | success | AllowUnresolved=true; specific getter → NotResolved |
| dr10 | `dr10-non-object-override.yaml` | success | `obj.WithFallback(num).WithFallback(otherObj)` ignores otherObj |
| dr11a | `dr11a-resolve-with-source-keys-absent.yaml` | success | ResolveWith: source keys NOT in result |
| dr11b | `dr11b-resolve-with-unresolved-source.yaml` | error | ResolveWith with unresolved source → NotResolved |
| dr12 | `dr12-origin-preserved.yaml` | error | Resolve error message includes source position |
| dr13 | `dr13-type-error-under-allow-unresolved.yaml` | error | Type error fires under AllowUnresolved=true |
| dr14 | `dr14-deferred-concat-placeholder.yaml` | success | Fully-unresolved concat survives as placeholder; getter → NotResolved |
| dr15 | `dr15-include-deferred.yaml` | success | Include expanded at parse; `${...}` deferred |
| dr16 | `dr16-frommap-nested-coercion.yaml` | success | FromMap with scalars / lists / nested maps |
| dr17 | `dr17-e11-package-include-deferred.yaml` | success | E11 `include package(...)` + deferred. **n/a for Lightbend** (skipped). |
| dr18 | `dr18-cross-layer-cycle.yaml` | error | Receiver `a=${b}`, fallback `b=${a}` → CycleError |
| dr19 | `dr19-resolve-idempotent.yaml` | success | `c.Resolve().Resolve()` equivalent (programmatic-only; see notes) |
| dr20 | `dr20-transitive-single-source.yaml` | success | `a=${b}; b=${c}; c=1` → `a=1` |
| dr21 | `dr21-transitive-cross-layer.yaml` | success | Transitive subst across 3 layers |
| dr22 | `dr22-hidden-single-source.yaml` | success | `foo=${nonexist}; foo=42` → `{foo:42}` (hidden not evaluated) |
| dr23 | `dr23-hidden-across-layers.yaml` | success | Receiver `foo=42`, fallback `foo=${nonexist}` → `{foo:42}` |
| dr24 | `dr24-optional-standalone-undef.yaml` | success | `a = ${?x}` (x undef) → field omitted |
| dr25 | `dr25-optional-string-concat-undef.yaml` | success | `a = ${?x} "tail"` (x undef) → `a = " tail"` |
| dr26 | `dr26-optional-array-concat-undef.yaml` | success | `a = ${?x} [1,2]` (x undef) → `a = [1,2]` |
| dr27 | `dr27-optional-object-merge-undef.yaml` | success | `a = ${?x} { k=1 }` (x undef) → `a = {k:1}` |
| dr28 | `dr28-optional-multi-all-undef.yaml` | success | `a = ${?x}${?y}` (both undef) → field omitted |
| dr29 | `dr29-empty-config-edges.yaml` | success | `Empty().Resolve()`, `c.WithFallback(Empty())`, `Empty().WithFallback(c)` |
| dr30 | `dr30-object-merge-barrier.yaml` | success | Receiver-non-object blocks fallback-object |

## Per-impl override notes

- **dr17** (E11 package include): NOT applicable to Lightbend (Lightbend has no `package(...)` qualifier). Java generator skips. Per-impl tests run.
- **dr19** (idempotency): the assertion "`c.Resolve()` twice yields equivalent Config" is not expressible in scenario YAML (no "compare two builds" op in v1). Per-impl tests verify programmatically. The YAML for dr19 just records the spec assertion as `description`; `expect` is single-resolve outcome.
- **dr12** (origin preservation): the assertion is that the error message contains source-position info (file:line:col). Lightbend formats positions differently from our impls. Generator emits `errorContains` with a Lightbend-format substring; per-impl tests check their format's analogue.

## See also

- [docs/extra-spec-conventions.md § E12](../../../../docs/extra-spec-conventions.md#e12)
- [docs/fixture-conventions.md § Scenario YAML fixtures (E12)](../../../../docs/fixture-conventions.md#scenario-yaml-fixtures-e12)
- [Design doc](../../../../.claude/superpowers/specs/2026-05-21-e12-deferred-resolution-design.md)
