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

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
