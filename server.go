package fiberadapter

import (
	"context"
	"fmt"
	"time"

	core "github.com/ashrafAli23/nestgo/core"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
)

// Compile-time checks
var _ core.Server = (*FiberServer)(nil)
var _ core.Router = (*FiberRouter)(nil)

// ═══════════════════════════════════════════════════════════════════════════
// FiberServer — implements core.Server
// ═══════════════════════════════════════════════════════════════════════════

type FiberServer struct {
	app    *fiber.App
	config *core.Config
	router *FiberRouter
}

// New creates a new FiberServer with the given config.
func New(config *core.Config) core.Server {
	if config == nil {
		config = core.DefaultConfig()
	}

	fiberConfig := fiber.Config{
		AppName:      config.AppName,
		ReadTimeout:  time.Duration(config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(config.WriteTimeout) * time.Second,
		BodyLimit:    config.BodyLimit,
	}

	app := fiber.New(fiberConfig)

	server := &FiberServer{
		app:    app,
		config: config,
	}

	// Create root router wrapping the fiber.App (which implements fiber.Router).
	server.router = &FiberRouter{
		app:        app,
		router:     app,
		errHandler: config.ErrorHandler,
	}

	return server
}

// ─── core.Server implementation ─────────────────────────────────────────────

func (s *FiberServer) Start(addr string) error {
	if addr == "" {
		addr = s.config.Addr
	}

	fmt.Printf("[NestGo] Starting Fiber server on %s\n", addr)
	return s.app.Listen(addr)
}

func (s *FiberServer) Shutdown(ctx context.Context) error {
	fmt.Println("[NestGo] Shutting down Fiber server...")
	return s.app.ShutdownWithContext(ctx)
}

func (s *FiberServer) Name() string {
	return "fiber"
}

func (s *FiberServer) Underlying() interface{} {
	return s.app
}

// ─── core.Router delegation ─────────────────────────────────────────────────

func (s *FiberServer) GET(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	s.router.GET(path, handler, mw...)
}

func (s *FiberServer) POST(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	s.router.POST(path, handler, mw...)
}

func (s *FiberServer) PUT(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	s.router.PUT(path, handler, mw...)
}

func (s *FiberServer) DELETE(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	s.router.DELETE(path, handler, mw...)
}

func (s *FiberServer) PATCH(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	s.router.PATCH(path, handler, mw...)
}

func (s *FiberServer) OPTIONS(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	s.router.OPTIONS(path, handler, mw...)
}

func (s *FiberServer) HEAD(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	s.router.HEAD(path, handler, mw...)
}

func (s *FiberServer) Group(prefix string, mw ...core.MiddlewareFunc) core.Router {
	return s.router.Group(prefix, mw...)
}

func (s *FiberServer) Use(mw ...core.MiddlewareFunc) {
	s.router.Use(mw...)
}

func (s *FiberServer) Static(path string, root string, mw ...core.MiddlewareFunc) {
	s.router.Static(path, root, mw...)
}

func (s *FiberServer) StaticFile(path string, filePath string, mw ...core.MiddlewareFunc) {
	s.router.StaticFile(path, filePath, mw...)
}

// ═══════════════════════════════════════════════════════════════════════════
// FiberRouter — implements core.Router
// ═══════════════════════════════════════════════════════════════════════════

// FiberRouter wraps fiber's routing.
// We keep a reference to the app for root-level operations,
// and a fiber.Router for group-level operations (groups are also fiber.Router).
type FiberRouter struct {
	app        *fiber.App
	router     fiber.Router
	errHandler core.ErrorHandler
}

func (r *FiberRouter) GET(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	finalHandler := applyRouteMiddleware(handler, mw)
	r.router.Get(path, wrapHandler(finalHandler, r.errHandler))
}

func (r *FiberRouter) POST(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	finalHandler := applyRouteMiddleware(handler, mw)
	r.router.Post(path, wrapHandler(finalHandler, r.errHandler))
}

func (r *FiberRouter) PUT(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	finalHandler := applyRouteMiddleware(handler, mw)
	r.router.Put(path, wrapHandler(finalHandler, r.errHandler))
}

func (r *FiberRouter) DELETE(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	finalHandler := applyRouteMiddleware(handler, mw)
	r.router.Delete(path, wrapHandler(finalHandler, r.errHandler))
}

func (r *FiberRouter) PATCH(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	finalHandler := applyRouteMiddleware(handler, mw)
	r.router.Patch(path, wrapHandler(finalHandler, r.errHandler))
}

func (r *FiberRouter) OPTIONS(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	finalHandler := applyRouteMiddleware(handler, mw)
	r.router.Options(path, wrapHandler(finalHandler, r.errHandler))
}

func (r *FiberRouter) HEAD(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	finalHandler := applyRouteMiddleware(handler, mw)
	r.router.Head(path, wrapHandler(finalHandler, r.errHandler))
}

func (r *FiberRouter) Group(prefix string, mw ...core.MiddlewareFunc) core.Router {
	// Collect middleware handlers for the group.
	handlers := make([]any, 0, len(mw))
	for _, m := range mw {
		handlers = append(handlers, wrapMiddleware(m, r.errHandler))
	}

	fiberGroup := r.router.Group(prefix, handlers...)

	return &FiberRouter{
		app:        r.app,
		router:     fiberGroup,
		errHandler: r.errHandler,
	}
}

func (r *FiberRouter) Use(mw ...core.MiddlewareFunc) {
	for _, m := range mw {
		r.router.Use(wrapMiddleware(m, r.errHandler))
	}
}

func (r *FiberRouter) Static(path string, root string, mw ...core.MiddlewareFunc) {
	if len(mw) > 0 {
		args := make([]any, 0, len(mw)+2)
		args = append(args, path)
		for _, m := range mw {
			args = append(args, wrapMiddleware(m, r.errHandler))
		}
		args = append(args, static.New(root))
		r.router.Use(args...)
	} else {
		r.router.Use(path, static.New(root))
	}
}

func (r *FiberRouter) StaticFile(path string, filePath string, mw ...core.MiddlewareFunc) {
	sendFile := func(c fiber.Ctx) error {
		return c.SendFile(filePath)
	}

	if len(mw) > 0 {
		handlers := make([]any, 0, len(mw))
		for _, m := range mw {
			handlers = append(handlers, wrapMiddleware(m, r.errHandler))
		}
		r.router.Get(path, sendFile, handlers...)
	} else {
		r.router.Get(path, sendFile)
	}
}
