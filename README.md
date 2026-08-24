<h1 align="center">NestGo Fiber Adapter</h1>

<p align="center">
  <strong>High-performance Fiber v3 HTTP engine adapter for the NestGo web framework in Go (Golang).</strong>
</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/ashrafAli23/nestgo-fiber-adapter"><img src="https://pkg.go.dev/badge/github.com/ashrafAli23/nestgo-fiber-adapter.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/ashrafAli23/nestgo-fiber-adapter"><img src="https://goreportcard.com/badge/github.com/ashrafAli23/nestgo-fiber-adapter" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
</p>

---

> **📚 Documentation:** <https://ashrafali23.github.io/nestgo/adapters.html>

This package implements the NestGo framework's `core.Server`, `core.Router`, and `core.Context` interfaces on top of [Fiber v3](https://gofiber.io). This allows you to leverage NestGo's powerful dependency injection, Guards, Interceptors, Pipes, and Middleware ecosystem alongside Fiber's ultra-fast, low-memory-footprint HTTP web server for Go.

## Table of Contents

- [Install](#install)
- [Quick Start](#quick-start)
- [Swapping from Gin](#swapping-from-gin)
- [Features](#features)
  - [Per-Request Contexts](#per-request-contexts)
  - [Use-After-Release Protection](#use-after-release-protection)
  - [Panic Recovery](#panic-recovery)
  - [Safe Goroutine Usage with Clone](#safe-goroutine-usage-with-clone)
  - [Route Groups](#route-groups)
  - [Accessing Raw Fiber APIs](#accessing-raw-fiber-apis)
  - [Graceful Shutdown](#graceful-shutdown)
- [Configuration](#configuration)
- [Performance](#performance)
- [Compatibility](#compatibility)
- [API Reference](#api-reference)
- [Related Packages](#related-packages)
- [License](#license)

## Install

```bash
go get github.com/ashrafAli23/nestgo-fiber-adapter
```

**Prerequisites:**

```bash
go get github.com/ashrafAli23/nestgo          # core framework
go get github.com/gofiber/fiber/v3            # fiber v3
```

## Quick Start

```go
package main

import (
    "github.com/ashrafAli23/nestgo/core"
    fiberadapter "github.com/ashrafAli23/nestgo-fiber-adapter"
    "github.com/ashrafAli23/nestgo/middleware"
)

func main() {
    server := fiberadapter.New(core.DefaultConfig())

    // NestGo middleware works out of the box
    server.Use(middleware.Recovery())
    server.Use(middleware.CORS())
    server.Use(middleware.RequestID())

    server.GET("/hello", func(c core.Context) error {
        return c.JSON(200, map[string]string{"message": "Hello from Fiber!"})
    })

    server.Start(":3000")
}
```

## Swapping from Gin

NestGo's adapter pattern means switching from Gin to Fiber is usually a one-line change:

```diff
  import (
      "github.com/ashrafAli23/nestgo/core"
-     adapter "github.com/ashrafAli23/nestgo-gin-adapter"
+     adapter "github.com/ashrafAli23/nestgo-fiber-adapter"
  )

  func main() {
      server := adapter.New(core.DefaultConfig())
      // ... all handlers, middleware, guards, etc. remain identical
  }
```

## Features

### Per-Request Contexts

Fiber recycles its own context (and the underlying fasthttp buffers) after each request. The adapter's `FiberContext` wrapper, however, is deliberately **allocated fresh per request — not pooled**: the struct is tiny (three fields), and skipping the pool makes use-after-release detection deterministic, because the released flag is set once and never cleared. The cost is one small allocation per request, negligible next to the work Fiber itself does.

### Use-After-Release Protection

Every `FiberContext` method checks an `atomic.Bool` released flag (~1ns overhead). If you accidentally use a context after the handler returns, you reliably get a clear panic message instead of silent data corruption:

```text
[NestGo] use-after-release: FiberContext used after handler returned.
Fiber contexts are recycled. Use c.Clone() before passing to goroutines.
```

### Panic Recovery

A panicking handler can no longer kill the process. The adapter recovers panics at two levels: its own recover around every composed handler and middleware chain (turning the panic into a logged 500 through the normal error-dispatch path), plus Fiber's recover middleware as a safety net for native Fiber handlers such as static files (with stack traces when `Debug` is enabled).

### Safe Goroutine Usage with Clone

Because Fiber contexts are explicitly **NOT safe** to use after the HTTP handler returns, you must use `Clone()` to create a read-only snapshot when firing off asynchronous goroutines:

```go
server.GET("/async", func(c core.Context) error {
    snapshot := c.Clone()
    
    go func() {
        defer fiberadapter.ReleaseSnapshot(snapshot) // optional, reduces GC pressure
        
        ip := snapshot.ClientIP()                     // safe
        method := snapshot.Method()                   // safe
        
        // Perform async background tasks...
        _ = ip
        _ = method
    }()
    
    return c.JSON(202, map[string]string{"status": "accepted"})
})
```

`Clone()` returns a `FiberContextSnapshot` containing copied:

- HTTP method, path, IP, full URL (`scheme://host/path?query`)
- Request body (deep copy)
- Headers, query params, route params (map copies — header lookups are case-insensitive, matching the live context)
- Locals (independent map)

Every value is deep-copied, so nothing in the snapshot aliases fasthttp's recycled buffers.

> **Pro Tip**: Snapshots are pooled. Call `ReleaseSnapshot()` when done to maintain maximum high-throughput API performance.

### Route Groups

```go
api := server.Group("/api/v1")
api.GET("/users", listUsers)
api.POST("/users", createUser)

// Nested groups with middleware applied
admin := api.Group("/admin", middleware.RateLimit(middleware.RateLimitConfig{
    Max:    10,
    Window: time.Minute,
}))
admin.DELETE("/users/:id", deleteUser)
```

The middleware chain (group chain outermost, then route middleware, then the handler) is composed into a single native Fiber handler at route registration. Handler errors flow back through every middleware — NestGo exception filters and interceptors see them — and the error handler runs exactly once. One rule follows: middleware added via `Use()` **after** a route is registered does not apply to that route, so register global middleware before your routes (NestGo's DI container already does this automatically).

### Accessing Raw Fiber APIs

For robust and highly specialized Fiber features not covered by NestGo's abstractions, you can access the underlying Fiber objects directly:

```go
// Access the *fiber.App
app := server.Underlying().(*fiber.App)
app.Static("/public", "./static")

// Access fiber.Ctx inside a handler
server.GET("/raw", func(c core.Context) error {
    fc := c.Underlying().(fiber.Ctx)
    // Use Fiber-specific APIs directly
    return c.JSON(200, map[string]any{"ok": true})
})
```

### Graceful Shutdown

Ensures no requests are dropped during server restarts or deployments:

```go
go func() {
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    server.Shutdown(ctx)
}()

server.Start(":3000")
```

## Configuration

Pass a `*core.Config` to `New()`:

```go
server := fiberadapter.New(&core.Config{
    AppName:      "my-api",
    Addr:         ":8080",
    ReadTimeout:  30,               // seconds
    WriteTimeout: 30,               // seconds
    IdleTimeout:  60,               // seconds
    BodyLimit:    10 * 1024 * 1024, // 10MB
    ErrorHandler: customErrorHandler,
})
```

| Field          | Type                | Default   | Description                                                   |
| -------------- | ------------------- | --------- | ------------------------------------------------------------- |
| `AppName`       | `string`            | `""`      | Application name (shown in Fiber's app info)                  |
| `Addr`          | `string`            | `":3000"` | Default listen address                                        |
| `Debug`         | `bool`              | `false`   | Enable Fiber debug output and print routes during startup     |
| `ReadTimeout`   | `int`               | `0`       | Read timeout in seconds                                       |
| `WriteTimeout`  | `int`               | `0`       | Write timeout in seconds                                      |
| `IdleTimeout`   | `int`               | `0`       | Keep-alive idle connection timeout in seconds                 |
| `BodyLimit`     | `int`               | `0`       | Max request body size in bytes (oversized bodies get 413)     |
| `ErrorHandler`  | `core.ErrorHandler` | `nil`     | Custom error handler (defaults to `core.DefaultErrorHandler`) |

> Note: `Config.ReadHeaderTimeout` and `Config.MaxHeaderBytes` are **not** mapped — Fiber v3 / fasthttp expose no equivalent (header size is bounded by fasthttp's per-connection read buffer). `DisableLogger` is not used by this adapter.

## Performance

This adapter is deeply optimized for production web server workloads and highly concurrent Go applications:

| Optimization           | Technique                     | Impact                   |
| ---------------------- | ----------------------------- | ------------------------ |
| Per-request contexts   | Tiny three-field wrapper      | Reliable misuse panics   |
| Release detection      | `atomic.Bool`                 | ~1ns per method call     |
| Snapshot pooling       | `sync.Pool` + map reuse       | Reduced GC on `Clone()`  |
| Header/query iteration | Go 1.23 `iter.Seq2`           | No deprecated `VisitAll` |
| Map reuse in snapshots | `clear()` instead of `make()` | Zero map alloc from pool |

## Compatibility

| Dependency  | Version                   |
| ----------- | ------------------------- |
| Go          | 1.25.14+                  |
| Fiber       | v3.x (tested with v3.5.0) |
| NestGo Core | v1.x                      |

## API Reference

Full developer documentation is available on [pkg.go.dev](https://pkg.go.dev/github.com/ashrafAli23/nestgo-fiber-adapter).

### Exported Types

- **`FiberServer`** — implements `core.Server`. Created via `New()`.
- **`FiberRouter`** — implements `core.Router`. Created via `Group()`.
- **`FiberContext`** — implements `core.Context`. Wraps `fiber.Ctx`.
- **`FiberContextSnapshot`** — implements `core.Context` (read-only). Created via `Clone()`.

### Exported Functions

- **`New(config *core.Config) core.Server`** — creates a new Fiber-backed server.
- **`ReleaseSnapshot(c core.Context)`** — returns a snapshot to the pool for reuse.

## Related Packages

| Package                                                                 | Description                                 |
| ----------------------------------------------------------------------- | ------------------------------------------- |
| [nestgo](https://github.com/ashrafAli23/nestgo)                         | Core framework (interfaces, middleware, DI) |
| [nestgo-gin-adapter](https://github.com/ashrafAli23/nestgo-gin-adapter) | Gin adapter                                 |
| [nestgo-validator](https://github.com/ashrafAli23/nestgo-validator)     | Validation & transformation                 |

## License

[MIT](LICENSE)
