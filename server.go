package fiberadapter

import (
	"context"
	"time"

	core "github.com/ashrafAli23/nestgo/core"
	"github.com/gofiber/fiber/v3"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
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
		// Note: fiber v3 / fasthttp expose no ReadHeaderTimeout or
		// MaxHeaderBytes equivalent (header size is bounded by the
		// per-connection ReadBufferSize), so those Config fields are not
		// mapped here.
		IdleTimeout: time.Duration(config.IdleTimeout) * time.Second,
		BodyLimit:   config.BodyLimit,
		ErrorHandler: func(fc fiber.Ctx, err error) error {
			// Handles errors returned by NATIVE fiber handlers (e.g. static
			// files) AND transport-level errors fiber/fasthttp raise before
			// any handler runs (e.g. BodyLimit exceeded -> 413). Wrapped core
			// handlers dispatch their own errors and always return nil, so
			// this never double-invokes for them.
			ctx := acquireContext(fc)
			defer releaseContext(ctx)
			// Translate *fiber.Error (fiber/fasthttp's own HTTP semantics,
			// e.g. 413/404/405) into *core.HTTPError first, so it survives
			// core.DefaultErrorHandler's genericization of unknown errors
			// into a 500 instead of being swallowed by it.
			err = translateFiberError(err)
			if config.ErrorHandler != nil {
				config.ErrorHandler(ctx, err)
			} else {
				core.DefaultErrorHandler(ctx, err)
			}
			return nil
		},
	}

	app := fiber.New(fiberConfig)

	// Unconditional panic safety net for NATIVE fiber handlers (static files,
	// etc.). Wrapped core handlers already recover panics in wrapHandler;
	// without this, fiber/fasthttp let a panic kill the whole process.
	app.Use(recoverer.New(recoverer.Config{EnableStackTrace: config.Debug}))

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

	core.Log().Info("starting server", core.F("adapter", "fiber"), core.F("addr", addr))
	return s.app.Listen(addr, fiber.ListenConfig{
		EnablePrintRoutes:     s.config.Debug,
		DisableStartupMessage: !s.config.Debug,
	})
}

func (s *FiberServer) StartTLS(addr, certFile, keyFile string) error {
	if addr == "" {
		addr = s.config.Addr
	}
	core.Log().Info("starting TLS server", core.F("adapter", "fiber"), core.F("addr", addr))
	return s.app.Listen(addr, fiber.ListenConfig{
		CertFile:              certFile,
		CertKeyFile:           keyFile,
		EnablePrintRoutes:     s.config.Debug,
		DisableStartupMessage: !s.config.Debug,
	})
}

func (s *FiberServer) Shutdown(ctx context.Context) error {
	core.Log().Info("shutting down server", core.F("adapter", "fiber"))
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

func (s *FiberServer) ANY(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	s.router.ANY(path, handler, mw...)
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
//
// Middleware architecture: routers/groups accumulate their core middleware
// chain in-adapter (groups inherit the parent's chain). Use() APPENDS to the
// chain instead of registering a native per-middleware wrapper, and at route
// registration the chain is composed in front of the handler — group chain
// outermost, then route middleware, then the handler — into ONE native fiber
// handler (see wrapHandler). This lets handler errors flow through core
// middleware (interceptors/filters see them) and guarantees the error
// handler runs at most once per request. Middleware added via Use() AFTER a
// route is registered does not apply to that route.
type FiberRouter struct {
	app        *fiber.App
	router     fiber.Router
	errHandler core.ErrorHandler
	// middlewares is this router's accumulated core middleware chain,
	// composed outermost at route registration time.
	middlewares []core.MiddlewareFunc
}

// compose builds the single native handler for a route:
// final = group chain(route middleware(handler)).
func (r *FiberRouter) compose(handler core.HandlerFunc, routeMws []core.MiddlewareFunc) fiber.Handler {
	h := applyRouteMiddleware(handler, routeMws)
	h = applyRouteMiddleware(h, r.middlewares)
	return wrapHandler(h, r.errHandler)
}

// chainWith returns a copy of this router's middleware chain with mw appended.
func (r *FiberRouter) chainWith(mw []core.MiddlewareFunc) []core.MiddlewareFunc {
	if len(r.middlewares) == 0 && len(mw) == 0 {
		return nil
	}
	chain := make([]core.MiddlewareFunc, 0, len(r.middlewares)+len(mw))
	chain = append(chain, r.middlewares...)
	chain = append(chain, mw...)
	return chain
}

func (r *FiberRouter) GET(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	r.router.Get(path, r.compose(handler, mw))
}

func (r *FiberRouter) POST(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	r.router.Post(path, r.compose(handler, mw))
}

func (r *FiberRouter) PUT(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	r.router.Put(path, r.compose(handler, mw))
}

func (r *FiberRouter) DELETE(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	r.router.Delete(path, r.compose(handler, mw))
}

func (r *FiberRouter) PATCH(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	r.router.Patch(path, r.compose(handler, mw))
}

func (r *FiberRouter) OPTIONS(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	r.router.Options(path, r.compose(handler, mw))
}

func (r *FiberRouter) HEAD(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	r.router.Head(path, r.compose(handler, mw))
}

func (r *FiberRouter) ANY(path string, handler core.HandlerFunc, mw ...core.MiddlewareFunc) {
	r.router.All(path, r.compose(handler, mw))
}

func (r *FiberRouter) Group(prefix string, mw ...core.MiddlewareFunc) core.Router {
	// The child inherits the parent's chain plus its own middleware; the
	// native fiber group carries only the path prefix.
	return &FiberRouter{
		app:         r.app,
		router:      r.router.Group(prefix),
		errHandler:  r.errHandler,
		middlewares: r.chainWith(mw),
	}
}

// Use appends middleware to this router's chain. It applies to routes
// registered on this router (and groups created from it) AFTER this call.
func (r *FiberRouter) Use(mw ...core.MiddlewareFunc) {
	r.middlewares = append(r.middlewares, mw...)
}

func (r *FiberRouter) Static(path string, root string, mw ...core.MiddlewareFunc) {
	// static.New is a native fiber handler, so the accumulated core chain
	// (plus per-call middleware) is bridged in front of it.
	if chain := r.chainWith(mw); len(chain) > 0 {
		r.router.Use(path, wrapMiddlewareChain(chain, r.errHandler), static.New(root))
	} else {
		r.router.Use(path, static.New(root))
	}
}

func (r *FiberRouter) StaticFile(path string, filePath string, mw ...core.MiddlewareFunc) {
	// Registered as a regular composed route: SendFile is part of
	// core.Context, so the file handler runs through the same chain,
	// panic recovery, and error dispatch as any other route.
	r.GET(path, func(c core.Context) error {
		return c.SendFile(filePath)
	}, mw...)
}
