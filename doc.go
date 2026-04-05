// Package fiberadapter provides a [Fiber] v3 adapter for the NestGo framework.
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
// # Context Pooling & Use-After-Release Protection
//
// Fiber recycles its context objects after each request. This adapter mirrors
// that pattern with [sync.Pool]-based context pooling and adds a safety net:
// every [FiberContext] method checks an [atomic.Bool] released flag. If you
// accidentally use a context after the handler returns, it panics with a clear
// message instead of silently corrupting data.
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
// # Performance Characteristics
//
//   - Context pooling via [sync.Pool] — zero allocation per request for context structs
//   - Use-after-release checks via [atomic.Bool] — single atomic load, ~1ns overhead
//   - Snapshot pooling with map reuse — [Clone] reuses maps via [clear] instead of reallocating
//   - Go 1.23 range-over-func iterators — headers and query args use [iter.Seq2], no deprecated VisitAll
//
// [Fiber]: https://gofiber.io
package fiberadapter
