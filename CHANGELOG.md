# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- `SendStream` (including SSE) no longer deadlocks clients that wait for headers before producing data: `ImmediateHeaderFlush` is set so status and headers reach the client at stream start instead of sitting buffered until the first body chunk
- An error returned after `SendStream` no longer hangs the request: error dispatch checks `IsBodyStream()` before `Body()` (fasthttp's `Body()` drains a body stream), and `ResponseBody()` returns `nil` for streamed responses (matches the Gin adapter)
- `*fiber.Error` raised by fiber/fasthttp (413 body-too-large, 404, 405) is translated to `core.HTTPError` so its real status code survives dispatch instead of coming out as a 500; unknown non-HTTP errors still return a generic 500
- Data race between the request context's `Done()` and `Server.Shutdown` under `-race` avoided: the request context captures fasthttp's `Done()` channel once at request start (same channel, same close — cancellation is unchanged)

### Changed

- **Behavior change:** `RequestCtx()` returns a small wrapper context, not `*fasthttp.RequestCtx` itself, so it is no longer directly type-assertable to `*fasthttp.RequestCtx` — use `c.Underlying().(fiber.Ctx)`, the documented route
- **Behavior change:** a handler returning a hand-rolled `fiber.NewError(code, msg)` now surfaces `msg` to the client with `code` (previously genericized to a 500); framework-raised fiber errors use fixed status strings, so no request data can leak

### Added

- Runs the `nestgo` `conformance` suite (22 checks) in `conformance_test.go`
- `.github/workflows/ci.yml` — build, vet, race tests, and `govulncheck` on every push to `main` and pull request
- Benchmarks comparing the raw engine with NestGo on it (`go test -bench .`), results in `BENCHMARKS.md`; CI runs them as a smoke test
- `ExampleNew` runnable example for pkg.go.dev
- Dependabot (weekly, grouped minor/patch), issue and PR templates, `SECURITY.md`, `CODE_OF_CONDUCT.md`

---

## [1.4.0] - 2026-08-24

### Security

- Bumped the `go` directive from `1.25.0` to `1.25.14`, pulling in Go standard-library security fixes. `govulncheck ./...` reports zero reachable vulnerabilities (the only remaining advisory is the unfixable "openpgp unmaintained" notice in `golang.org/x/crypto`, which this module never imports).

### Changed

- **Upgraded Fiber from v3.1.0 to v3.5.0** (no API changes required in the adapter). Core dependency `github.com/ashrafAli23/nestgo` upgraded to `v1.4.0`.
- Upgraded transitive dependencies to latest stable, including `valyala/fasthttp` v1.73.0, `klauspost/compress` v1.19.2, `golang.org/x/crypto` v0.55.0, and `golang.org/x/net` v0.58.0.
- **Behavior change:** middleware added via `Use()` after a route is registered no longer applies to that route — middleware chains are now composed into the route's handler at registration time (converges with the Gin adapter).
- `FiberContext` is no longer pooled: each request allocates a small fresh wrapper so use-after-release panics are deterministic. Underlying Fiber/fasthttp buffers are still recycled, so `Clone()` remains required for goroutines.

### Added

- `FiberContext` implements the `core.ResponseResetter`, `core.ResponseHeaderReader`, and `core.SameSiteCookieSetter` capabilities (enables ETag 304 rewrites, Idempotency Content-Type replay, and CSRF SameSite cookies)
- `core.Config.IdleTimeout` is now mapped onto `fiber.Config.IdleTimeout` (`ReadHeaderTimeout`/`MaxHeaderBytes` have no Fiber v3 equivalent and remain unmapped)

### Fixed

- Panic recovery added: a panicking handler or middleware becomes a logged 500 instead of killing the process (adapter-level recover for composed chains, plus Fiber's recover middleware for native handlers)
- `ResponseBody()` returns an adapter-owned copy instead of fasthttp's live pooled buffer, so cached responses (e.g. the Idempotency middleware) can never alias recycled memory
- `Clone()` snapshots no longer alias fasthttp memory: method, path, client IP, params, and the full URL are copied; snapshot header lookups are case-insensitive with first-occurrence semantics
- Handler errors now propagate back through `Use()`/`Group()` middleware: each route is composed into a single native handler and the configured `ErrorHandler` runs exactly once, only when nothing has been written
- `RequestCtx()` carries the request's `*fasthttp.RequestCtx` (server shutdown cancellation and deadlines) instead of `context.Background()`
- `Body()` returns a stable adapter-owned copy that survives handler completion instead of a fasthttp-aliased slice
- Snapshot pool no longer retains previous-request state; snapshot locals are goroutine-safe; a double `ReleaseSnapshot` is a no-op
- Cross-adapter parity: `String` writes the format verbatim when no values are given (literal `%` preserved), `QueryDefault` returns the default only when the key is absent, `FullURL` includes scheme and host, and `SendBytes` defaults Content-Type to `application/octet-stream`

---

## [1.3.0] - 2026-04-09

### Changed

- Upgraded `github.com/ashrafAli23/nestgo` core dependency to `v1.3.0`.
- **Logger Integration:** Replaced all internal `fmt` calls with `core.Log()` to integrate with NestGo's centralized logging system.
- **Enhanced Debug Mode:** The `core.Config.Debug` flag now automatically controls Fiber's `EnablePrintRoutes` (to show routing table) and `DisableStartupMessage` (to silence the Fiber banner in production).

### Added

- Implemented `ANY()` method on `Router` to support all HTTP methods (delegates to Fiber's `All()`).
- Added `StartTLS(addr, certFile, keyFile)` support for HTTPS servers using Fiber's native listener.

---

## [1.2.0] - 2026-04-06

### Fixed

- **Timeout Support:** Fixed an issue where the `Timeout()` middleware would drop responses on the Fiber adapter by ensuring the request context deadline is correctly propagates.

### Changed

- Upgraded `github.com/ashrafAli23/nestgo` core dependency to `v1.2.0`.

---

## [1.1.0] - 2026-04-05

### Added

- Initial release of the NestGo Fiber Adapter.
- Full implementation of `core.Server`, `core.Router`, and `core.Context` interfaces.
- Advanced context pooling with `sync.Pool`.
- Read-only context snapshots for safe concurrent usage in goroutines.
- Use-after-release protection to prevent common Fiber context pitfalls.
- Graceful shutdown support with context cancellation.
