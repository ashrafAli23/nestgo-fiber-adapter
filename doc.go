// Package fiberadapter provides a [Fiber] v3 adapter for NestGo, the
// NestJS-style web framework for Go.
//
// Full documentation: https://ashrafali23.github.io/nestgo/adapters.html
//
// It implements [core.Server], [core.Router], and [core.Context] on top of
// [github.com/gofiber/fiber/v3], letting you use NestGo's Guards, Interceptors,
// Pipes, and Middleware ecosystem with Fiber's high-performance HTTP engine.
//
// # Install
//
//	go get github.com/ashrafAli23/nestgo-fiber-adapter
//
// # Quick Start
//
//	package main
//
//	import (
//	    "github.com/ashrafAli23/nestgo/core"
//	    fiber "github.com/ashrafAli23/nestgo-fiber-adapter"
//	    "github.com/ashrafAli23/nestgo/middleware"
//	)
//
//	func main() {
//	    server := fiber.New(core.DefaultConfig())
//
//	    server.Use(middleware.Recovery())
//	    server.Use(middleware.CORS())
//
//	    server.GET("/hello", func(c core.Context) error {
//	        return c.JSON(200, map[string]string{"message": "Hello from Fiber!"})
//	    })
//
//	    server.Start(":3000")
//	}
//
// # Architecture
//
// This adapter bridges NestGo's zero-dep core interfaces to Fiber v3:
//
//	┌──────────────────────┐       ┌───────────────────────────┐
//	│  core.Server         │──────▶│  FiberServer              │
//	│  core.Router         │──────▶│  FiberRouter              │
//	│  core.Context        │──────▶│  FiberContext              │
//	└──────────────────────┘       └───────────────────────────┘
//
// Your handlers only import [core.Context]. The adapter translates every call
// to the underlying [fiber.Ctx] — you never touch Fiber APIs directly unless
// you choose to via [FiberContext.Underlying].
//
// # Context Lifetime & Use-After-Release Protection
//
// Fiber recycles its underlying context objects and buffers after each
// request. This adapter allocates a fresh [FiberContext] per request (the
// wrapper struct is intentionally NOT pooled), and every method checks an
// [atomic.Bool] released flag that is set once when the handler returns and
// never cleared. If you accidentally use a context after the handler returns,
// it reliably panics with a clear message instead of silently reading another
// request's data. Note the panic protects the wrapper only — the underlying
// fiber/fasthttp buffers are still recycled, so [FiberContext.Clone] remains
// the only safe way to hand request data to a goroutine.
//
// To safely pass context to a goroutine, call [FiberContext.Clone]:
//
//	server.GET("/async", func(c core.Context) error {
//	    snapshot := c.Clone()
//	    go func() {
//	        defer fiberadapter.ReleaseSnapshot(snapshot)
//	        ip := snapshot.ClientIP() // safe — reads from copied data
//	        _ = ip
//	    }()
//	    return c.JSON(200, map[string]string{"status": "accepted"})
//	})
//
// [Clone] returns a [FiberContextSnapshot] — a read-only copy of request data
// (method, path, headers, query params, body, IP). Response methods on snapshots
// return errors. Snapshots are also pooled; call [ReleaseSnapshot] when done to
// reduce GC pressure.
//
// # Route Groups
//
// Use [FiberServer.Group] (or [FiberRouter.Group]) to create prefixed sub-routers
// with their own middleware:
//
//	api := server.Group("/api/v1", middleware.RateLimit())
//	api.GET("/users", listUsers)
//	api.POST("/users", createUser)
//
// # Accessing the Raw Fiber App
//
// For advanced Fiber-specific features (static files, WebSocket upgrade, etc.),
// access the underlying [fiber.App]:
//
//	app := server.Underlying().(*fiber.App)
//	app.Static("/public", "./static")
//
// Similarly, within a handler you can access the raw [fiber.Ctx]:
//
//	server.GET("/raw", func(c core.Context) error {
//	    fc := c.Underlying().(fiber.Ctx)
//	    _ = fc // use Fiber-specific APIs
//	    return c.JSON(200, nil)
//	})
//
// # Request Context
//
// [FiberContext.RequestCtx] returns a small wrapper [context.Context], not
// the *fasthttp.RequestCtx itself. The wrapper carries the same cancellation
// signal (Done fires on server shutdown — fasthttp cannot signal per-request
// client disconnects) and delegates Deadline and Value to the underlying
// request context, but it is NOT type-assertable to *fasthttp.RequestCtx.
// For raw fasthttp access go through [FiberContext.Underlying]:
//
//	fc := c.Underlying().(fiber.Ctx)
//	raw := fc.RequestCtx() // *fasthttp.RequestCtx
//
// # Streaming Responses
//
// [FiberContext.SendStream] sets fasthttp's ImmediateHeaderFlush for the
// current response, so the status and headers reach the client as soon as
// streaming starts instead of waiting for the first body chunk — SSE
// (EventSource) clients and readers that block until headers arrive no
// longer deadlock. [FiberContext.ResponseBody] returns a copy of the
// buffered response body, and nil for a streamed response (fasthttp's
// Response.Body would otherwise drain and close the stream).
//
// # Fiber Error Translation
//
// Errors raised by fiber/fasthttp themselves arrive as [fiber.Error] values:
// 413 when the [core.Config] BodyLimit is exceeded, 404 for an unmatched
// route, 405 for a wrong method. The adapter translates them into
// [core.HTTPError] with the same status code and message before invoking
// the error handler ([core.DefaultErrorHandler] or your [core.Config]
// ErrorHandler), so they keep their real status instead of being
// genericized into a 500. Any other non-HTTPError error still becomes a
// generic 500.
//
// One consequence: a handler that returns a hand-rolled [fiber.NewError]
// with a code and message surfaces both to the client. Prefer
// [core.NewHTTPError] in NestGo handlers.
//
// # Performance Characteristics
//
//   - Per-request context structs — one small allocation per request buys a
//     reliable use-after-release panic (pooled wrappers could silently expose
//     another request's data to stale references)
//   - Use-after-release checks via [atomic.Bool] — single atomic load, ~1ns overhead
//   - Snapshot pooling with map reuse — [Clone] reuses maps via [clear] instead of reallocating
//   - Go 1.23 range-over-func iterators — headers and query args use [iter.Seq2], no deprecated VisitAll
//
// [Fiber]: https://gofiber.io
package fiberadapter
