# Contributing to go.hocon

Thank you for your interest in contributing!

## Reporting Bugs

Please open a [GitHub Issue](https://github.com/o3co/go.hocon/issues) and include:

- Go version (`go version`)
- go.hocon version
- A minimal reproducing HOCON snippet
- Expected vs. actual behavior

## Proposing Features

Open an issue first to discuss the proposal before sending a PR. This avoids wasted effort if the direction doesn't fit the project scope.

## Development Setup

```bash
git clone https://github.com/o3co/go.hocon.git
cd go.hocon
make testdata
go test ./...
```

The library itself has no external dependencies. `gopkg.in/yaml.v3` is a
test-only dependency (the fixture scenario runner).

`make testdata` fetches the xx.hocon conformance fixtures, as CI does. Run it
after cloning and again when the fixtures move: without them the conformance
tests skip silently instead of failing.

## Running Tests

```bash
# All tests
go test ./...

# With race detector
go test -race ./...

# Specific package
go test ./internal/resolver/...

# Lightbend spec compliance suite
go test -v -run TestLightbend ./...
```

### The adapters module

`adapters/` is a **separate Go module**, so `./...` at the repository root
stops at its boundary and never tests it. It needs its own run:

```bash
make test-all              # root module + adapters
make -C adapters check     # adapters only: go test -race + golangci-lint
```

CI runs both, so a change confined to `adapters/` is still covered.

### Before releasing: build the adapters without the `replace`

`adapters/go.mod` carries `replace github.com/o3co/go.hocon => ../` so that a
change spanning the parser and an adapter can be made and tested as one commit.
Consumers **ignore a dependency's replace directive** and build against the
version in `require` instead — so an in-repo `go build` can pass while every
consumer's build fails. That is exactly how v1.10.0 shipped an adapters module
requiring a core older than the API it called.

Reproduce the consumer's view:

```bash
rm -rf /tmp/adapters-consumer
cp -R adapters /tmp/adapters-consumer
cd /tmp/adapters-consumer
go mod edit -dropreplace=github.com/o3co/go.hocon
GOFLAGS=-mod=mod go test -count=1 ./...
```

CI runs this on every PR, in the `adapters without replace (published core)`
job. When the require names a version the proxy does not have yet — which is
the normal state of a PR that bumps it ahead of the tag — the job warns and
skips, and the release workflow runs the check for real once the tag exists.

Run `go mod tidy` before the build if you reproduce it by hand.
`go get MODULE@VERSION` records that module and nothing else, so the go.sum
entries for what it imports — the core module, go-toml, go-yaml — are missing
and the build fails on every one of them. The snippet above avoids this by
copying a checkout that already has a complete `go.sum`; a scratch consumer
built from `go mod init` does not.

So the rule is: **a core change that the adapters use means bumping the core
version in `adapters/go.mod` in the same PR**, to the version that will be
tagged. See [Releasing](#releasing) for the tag order that makes that
resolvable.

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep public API consistent with the existing getter families: error-returning `GetXxxE`, panicking `GetXxx`, and `GetXxxOption`
- New features must include tests
- Internal packages (`internal/`) are not part of the public API — do not add exported symbols there unless necessary

## Submitting a Pull Request

1. Fork the repository and create a branch from `develop`
2. Write tests for your change
3. Ensure `go test ./...` passes
4. Open a PR against `develop` with a clear description of what and why

## Releasing

Go modules are versioned by git tags — there is no version file to bump. A tag
**is** the release: the Go module proxy can serve a version as soon as its tag
is pushed, and nothing downstream can stop it. So the gate is upstream of the
tag: **only tag a commit whose CI on `develop` is green**, including the docs
consistency tests in `docs_test.go`.

A release goes through one release-prep PR, then two tags:

1. **Release-prep PR** (merged into `develop` like any other PR):
   - Cut `CHANGELOG.md`: rename `[Unreleased]` to `[X.Y.Z] - YYYY-MM-DD`, open
     a new empty `[Unreleased]`, and update the link references at the bottom.
     `release.yml` builds the GitHub Release body from that section; if the
     section is missing or extracts to nothing, the GitHub Release step fails
     (the module version itself is already published by the tag).
   - Bump `adapters/go.mod`'s core `require` to `vX.Y.Z` — the version about to
     be tagged — unless an earlier PR already did (see
     [above](#before-releasing-build-the-adapters-without-the-replace)). Until
     that core tag exists, CI's "adapters without replace (published core)"
     job warns and skips rather than failing.
2. Tag and push the **core** (`vX.Y.Z`) on the merged, green commit, so the
   version the adapters require exists on the proxy:

   ```bash
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

3. Tag and push the **adapters** in Go's subdirectory form — the tag name is the
   module path below the repository root — on the same commit:

   ```bash
   git tag adapters/vX.Y.Z
   git push origin adapters/vX.Y.Z
   ```

`release.yml` handles both tag shapes: it triggers proxy indexing for whichever
module was tagged; for a core tag it also creates the GitHub Release from the
CHANGELOG section; and for an adapters tag it also builds the published module
the way `go get` delivers it — no checkout, no `replace` — so a tag whose core
requirement cannot be satisfied fails there rather than at a user's build.

That job **retries** the `go get`, because the workflow races the tag that
triggered it. Until the proxy has indexed the nested module, `go get` does not
report it as missing — it falls back to the parent module and says:

```text
go: module github.com/o3co/go.hocon@vX.Y.Z found,
    but does not contain package github.com/o3co/go.hocon/adapters
```

That reads like a packaging mistake and is not one; it means "not indexed yet",
and the parent genuinely does not contain that package because `adapters/` is a
nested module. For `adapters/v1.12.0` it cleared about six minutes after the
tag.

The adapters version tracks the core version it pairs with (the first published
tag was `adapters/v1.11.0`), so a reader can tell at a glance which parser a
given adapters release was built against.

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
