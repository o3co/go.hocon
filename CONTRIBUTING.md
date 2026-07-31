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
go test ./...
```

No external dependencies — standard library only.

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
- Keep public API consistent with the existing panic / `Option[T]` dual pattern
- New features must include tests
- Internal packages (`internal/`) are not part of the public API — do not add exported symbols there unless necessary

## Submitting a Pull Request

1. Fork the repository and create a branch from `develop`
2. Write tests for your change
3. Ensure `go test ./...` passes
4. Open a PR against `develop` with a clear description of what and why

## Releasing

Go modules are versioned by git tags — there is no version file to bump.
CI triggers Go module proxy indexing automatically when a `v*` tag is pushed.

```bash
git tag v0.4.0
git push origin v0.4.0
```

That's it. The Go module proxy picks up the new version within minutes.

The nested `adapters/` module is versioned by its own tags, in Go's
subdirectory form — the tag name is the module path below the repository root:

```bash
git tag adapters/v1.10.1
git push origin adapters/v1.10.1
```

The two modules are released together, in this order:

1. The PR that changes both already bumped `adapters/go.mod`'s core `require`
   to the version about to be tagged (see [above](#before-releasing-build-the-adapters-without-the-replace)).
2. Tag and push the **core** (`vX.Y.Z`) first, so the version the adapters
   require exists on the proxy.
3. Tag and push the **adapters** (`adapters/vX.Y.Z`).

`release.yml` handles both tag shapes: it triggers proxy indexing for whichever
module was tagged, and for an adapters tag it also builds the published module
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

The adapters version tracks the core version it pairs with (the first tag will
be `adapters/v1.10.x`), so a reader can tell at a glance which parser a given
adapters release was built against.

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
