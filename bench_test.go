package fiberadapter_test

import (
	"reflect"
	"testing"
	"unsafe"

	fiberadapter "github.com/ashrafAli23/nestgo-fiber-adapter"
	core "github.com/ashrafAli23/nestgo/core"
	"github.com/ashrafAli23/nestgo/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/valyala/fasthttp"
)

// bindServer attaches an idle *fasthttp.Server to a hand-built RequestCtx.
//
// Adaptation (documented per task brief step 3): nestgo-fiber-adapter's
// seedRequestContext (context.go) calls RequestCtx.Done() on every request
// so core.Context's RequestCtx() carries real cancellation semantics. Done()
// dereferences the RequestCtx's unexported owning-*Server field
// (ctx.s.done). In production that field is always set, because fasthttp
// only ever constructs a RequestCtx via a running *Server. This benchmark
// builds a RequestCtx by hand (no real server, no network, per the brief),
// so the field is nil and Done() segfaults. We attach an idle *Server via
// reflect/unsafe so Done() reads a nil `done` channel instead of dereferencing
// a nil *Server — a nil Done() channel is documented-valid ("Done may return
// nil if this context can never be canceled"), and a benchmark request is
// never canceled anyway. Raw-fiber handlers don't call Done(), so they don't
// need this, but it's applied uniformly for both raw and NestGo paths.
func bindServer(ctx *fasthttp.RequestCtx) {
	f := reflect.ValueOf(ctx).Elem().FieldByName("s")
	reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.ValueOf(&fasthttp.Server{}))
}

// benchHandler measures one in-process request per iteration (no network).
func benchHandler(b *testing.B, h fasthttp.RequestHandler, path string) {
	ctx := &fasthttp.RequestCtx{}
	bindServer(ctx)
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI(path)
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

func rawFiber() fasthttp.RequestHandler {
	app := fiber.New()
	app.Get("/hello", func(c fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) })
	return app.Handler()
}

func nestGoFiber() fasthttp.RequestHandler {
	cfg := core.DefaultConfig()
	cfg.DisableLogger = true
	s := fiberadapter.New(cfg)
	s.GET("/hello", func(c core.Context) error { return c.JSON(200, map[string]bool{"ok": true}) })
	return s.Underlying().(*fiber.App).Handler()
}

func rawFiberMiddleware3() fasthttp.RequestHandler {
	app := fiber.New()
	app.Use(recover.New())
	app.Use(func(c fiber.Ctx) error { c.Set("X-Request-ID", "bench"); return c.Next() })
	app.Use(func(c fiber.Ctx) error { return c.Next() }) // allow-all "guard"
	app.Get("/hello", func(c fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) })
	return app.Handler()
}

func nestGoFiberMiddleware3() fasthttp.RequestHandler {
	cfg := core.DefaultConfig()
	cfg.DisableLogger = true
	s := fiberadapter.New(cfg)
	s.Use(middleware.Recovery(), middleware.RequestID())
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
