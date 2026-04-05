# Contributing to NestGo Fiber Adapter

Thank you for your interest in contributing!

## Getting Started

1. Fork and clone the repository
2. Install dependencies: `go mod download`
3. Run tests: `go test ./...`
4. Make your changes
5. Submit a pull request

## Development

### Prerequisites

- Go 1.23 or later
- Fiber v3

### Project Structure

```
nestgo-fiber-adapter/
  context.go       # FiberContext — implements core.Context
  server.go        # FiberServer/FiberRouter — implements core.Server/Router
  snapshot.go      # FiberContextSnapshot — read-only cloned context
  doc.go           # Package documentation
  example_test.go  # Testable examples for pkg.go.dev
```

### Key Design Principles

1. **Implement core interfaces only** — this adapter should never add methods beyond what `core.Server`, `core.Router`, and `core.Context` define.
2. **Zero allocation on hot path** — use `sync.Pool` for contexts and snapshots, avoid allocations in per-request code.
3. **Use-after-release safety** — every `FiberContext` method must call `checkReleased()` first.
4. **Pool-friendly cleanup** — when returning objects to pools, clear maps with `clear()` instead of setting to `nil` so they can be reused.

### How to Add a New core.Context Method

If the core package adds a new method to the `Context` interface:

1. Add the implementation to `FiberContext` in `context.go` with `c.checkReleased()` as the first call
2. Add a read-only stub (or no-op) to `FiberContextSnapshot` in `snapshot.go`
3. Verify both compile-time checks still pass (`var _ core.Context = ...`)
4. Add a test

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep comments minimal — code should be self-documenting
- Use Go doc format for exported symbols

## Submitting Changes

1. Create a feature branch from `main`
2. Write clear commit messages
3. Ensure `go build ./...` and `go vet ./...` pass
4. Open a PR with a description of what changed and why
