# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
