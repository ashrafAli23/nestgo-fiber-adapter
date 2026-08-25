package fiberadapter_test

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	fiberadapter "github.com/ashrafAli23/nestgo-fiber-adapter"
	core "github.com/ashrafAli23/nestgo/core"
	"github.com/ashrafAli23/nestgo/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/valyala/fasthttp"
)

// benchHandler measures one in-process request per iteration (no network).
//
// ctx.Init (exported by fasthttp, documented for custom Server
// implementations) builds a properly owned RequestCtx: it sets the
// unexported owning-*Server field via Init2's fakeServer, so RequestCtx.Done()
// — which nestgo-fiber-adapter's seedRequestContext calls on every request to
// give core.Context's RequestCtx() real cancellation semantics — reads a real
// open channel instead of dereferencing a nil *Server. That's the difference
// between this and just zero-valuing a &fasthttp.RequestCtx{}: fasthttp only
// ever constructs a request-ready RequestCtx through a running *Server, and
// Init is the supported way to get that outside of one.
func benchHandler(b *testing.B, h fasthttp.RequestHandler, path string) {
	var req fasthttp.Request
	req.Header.SetMethod(fasthttp.MethodGet)
	req.SetRequestURI(path)

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&req, nil, nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.Response.Reset()
		ctx.ResetUserValues()
		h(ctx)
		if ctx.Response.StatusCode() != 200 {
			b.Fatalf("status %d", ctx.Response.StatusCode())
		}
	}
}

// Both sides return the same map[string]bool payload so JSON encoding does
// identical work; fiber.Map (map[string]any) costs more per encode and would
// make "NestGo allocates less" an artifact of the raw handler's own payload
// type rather than of adapter overhead.
func rawFiber() fasthttp.RequestHandler {
	app := fiber.New()
	app.Get("/hello", func(c fiber.Ctx) error { return c.JSON(map[string]bool{"ok": true}) })
	return app.Handler()
}

func nestGoFiber() fasthttp.RequestHandler {
	cfg := core.DefaultConfig()
	cfg.DisableLogger = true
	s := fiberadapter.New(cfg)
	s.GET("/hello", func(c core.Context) error { return c.JSON(200, map[string]bool{"ok": true}) })
	return s.Underlying().(*fiber.App).Handler()
}

// Middleware3 = recovery + request-id header + an allow-all guard on both sides.
//
// Recovery: fiberadapter.New installs recover.New() unconditionally (see
// server.go), so nestGoFiberMiddleware3 below does not add a second one via
// s.Use — that would double-count recovery only on the NestGo side. The raw
// side installs its own recover.New() here so both paths run exactly one
// recovery layer.
//
// Request-ID: both sides do equivalent real work — generate 16 random bytes
// and hex-encode them into the header — instead of the raw side writing a
// constant string, which would undercount its cost relative to NestGo's
// middleware.RequestID().
func rawFiberMiddleware3() fasthttp.RequestHandler {
	app := fiber.New()
	app.Use(recover.New())
	app.Use(func(c fiber.Ctx) error {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		c.Set("X-Request-ID", hex.EncodeToString(b))
		return c.Next()
	})
	app.Use(func(c fiber.Ctx) error { return c.Next() }) // allow-all "guard"
	app.Get("/hello", func(c fiber.Ctx) error { return c.JSON(map[string]bool{"ok": true}) })
	return app.Handler()
}

func nestGoFiberMiddleware3() fasthttp.RequestHandler {
	cfg := core.DefaultConfig()
	cfg.DisableLogger = true
	s := fiberadapter.New(cfg) // already installs recover.New()
	s.Use(middleware.RequestID())
	allow := core.GuardFunc(func(core.Context) (bool, error) { return true, nil })
	s.GET("/hello", func(c core.Context) error {
		return c.JSON(200, map[string]bool{"ok": true})
	}, core.UseGuards(allow))
	return s.Underlying().(*fiber.App).Handler()
}

func BenchmarkRawFiber_HelloJSON(b *testing.B)    { benchHandler(b, rawFiber(), "/hello") }
func BenchmarkNestGoFiber_HelloJSON(b *testing.B) { benchHandler(b, nestGoFiber(), "/hello") }
func BenchmarkRawFiber_Middleware3(b *testing.B)  { benchHandler(b, rawFiberMiddleware3(), "/hello") }
func BenchmarkNestGoFiber_Middleware3(b *testing.B) {
	benchHandler(b, nestGoFiberMiddleware3(), "/hello")
}
