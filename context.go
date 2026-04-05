package fiberadapter

import (
	"fmt"
	"io"
	"mime/multipart"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	core "github.com/ashrafAli23/nestgo/core"

	"github.com/gofiber/fiber/v3"
)

var _ core.Context = (*FiberContext)(nil)

var contextPool = sync.Pool{
	New: func() interface{} { return &FiberContext{} },
}

func acquireContext(fc fiber.Ctx) *FiberContext {
	ctx := contextPool.Get().(*FiberContext)
	ctx.fiberCtx = fc
	ctx.released.Store(false)
	return ctx
}

func releaseContext(ctx *FiberContext) {
	ctx.released.Store(true)
	ctx.fiberCtx = nil
	contextPool.Put(ctx)
}

// checkReleased panics with a clear message if the context is used after release.
// Uses atomic.Bool for data-race-free checks without mutex overhead.
func (c *FiberContext) checkReleased() {
	if c.released.Load() {
		panic("[NestGo] use-after-release: FiberContext used after handler returned. " +
			"Fiber contexts are pooled and recycled. Use c.Clone() before passing to goroutines.")
	}
}

// FiberContext wraps fiber.Ctx to implement core.Context.
type FiberContext struct {
	fiberCtx fiber.Ctx
	released atomic.Bool
}

// ─── Request ────────────────────────────────────────────────────────────────

func (c *FiberContext) Method() string          { c.checkReleased(); return c.fiberCtx.Method() }
func (c *FiberContext) Path() string            { c.checkReleased(); return c.fiberCtx.Route().Path }
func (c *FiberContext) Param(key string) string { c.checkReleased(); return c.fiberCtx.Params(key) }
func (c *FiberContext) Query(key string) string {
	c.checkReleased()
	return fiber.Query[string](c.fiberCtx, key)
}

func (c *FiberContext) QueryDefault(key, def string) string {
	c.checkReleased()
	val := fiber.Query[string](c.fiberCtx, key)
	if val == "" {
		return def
	}
	return val
}

func (c *FiberContext) GetHeader(key string) string { c.checkReleased(); return c.fiberCtx.Get(key) }
func (c *FiberContext) Cookie(name string) string   { c.checkReleased(); return c.fiberCtx.Cookies(name) }
func (c *FiberContext) Body() ([]byte, error)       { c.checkReleased(); return c.fiberCtx.Body(), nil }
func (c *FiberContext) Bind(v interface{}) error    { c.checkReleased(); return c.fiberCtx.Bind().Body(v) }
func (c *FiberContext) FormValue(key string) string {
	c.checkReleased()
	return c.fiberCtx.FormValue(key)
}
func (c *FiberContext) ContentType() string { c.checkReleased(); return c.fiberCtx.Get("Content-Type") }

func (c *FiberContext) FormFile(key string) (*multipart.FileHeader, error) {
	c.checkReleased()
	return c.fiberCtx.FormFile(key)
}

func (c *FiberContext) IsWebSocket() bool {
	c.checkReleased()
	return strings.EqualFold(c.fiberCtx.Get("Upgrade"), "websocket")
}

// ─── Response ───────────────────────────────────────────────────────────────

func (c *FiberContext) Status(code int) core.Context {
	c.checkReleased()
	c.fiberCtx.Status(code)
	return c
}

func (c *FiberContext) JSON(status int, data interface{}) error {
	c.checkReleased()
	c.fiberCtx.Status(status)
	return c.fiberCtx.JSON(data)
}

func (c *FiberContext) XML(status int, data interface{}) error {
	c.checkReleased()
	c.fiberCtx.Status(status)
	return c.fiberCtx.XML(data)
}

func (c *FiberContext) String(status int, format string, vals ...interface{}) error {
	c.checkReleased()
	c.fiberCtx.Status(status)
	return c.fiberCtx.SendString(fmt.Sprintf(format, vals...))
}

func (c *FiberContext) SendBytes(status int, data []byte) error {
	c.checkReleased()
	c.fiberCtx.Status(status)
	return c.fiberCtx.Send(data)
}

func (c *FiberContext) SendStream(stream io.Reader) error {
	c.checkReleased()
	return c.fiberCtx.SendStream(stream)
}

func (c *FiberContext) NoContent(status int) error {
	c.checkReleased()
	c.fiberCtx.Status(status)
	return nil
}
func (c *FiberContext) SetHeader(k, v string) { c.checkReleased(); c.fiberCtx.Set(k, v) }

func (c *FiberContext) SetCookie(name, value string, maxAge int, path, domain string, secure, httpOnly bool) {
	c.checkReleased()
	c.fiberCtx.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Domain:   domain,
		MaxAge:   maxAge,
		Expires:  time.Now().Add(time.Duration(maxAge) * time.Second),
		Secure:   secure,
		HTTPOnly: httpOnly,
	})
}

func (c *FiberContext) Redirect(status int, url string) error {
	c.checkReleased()
	return c.fiberCtx.Redirect().Status(status).To(url)
}

// ─── Metadata ───────────────────────────────────────────────────────────────

func (c *FiberContext) ClientIP() string { c.checkReleased(); return c.fiberCtx.IP() }
func (c *FiberContext) FullURL() string  { c.checkReleased(); return c.fiberCtx.OriginalURL() }

// ─── Context Storage ────────────────────────────────────────────────────────

func (c *FiberContext) Set(key string, value interface{}) {
	c.checkReleased()
	c.fiberCtx.Locals(key, value)
}

func (c *FiberContext) Get(key string) (interface{}, bool) {
	c.checkReleased()
	val := c.fiberCtx.Locals(key)
	if val == nil {
		return nil, false
	}
	return val, true
}

// ─── Flow Control ───────────────────────────────────────────────────────────

func (c *FiberContext) Next() error             { c.checkReleased(); return c.fiberCtx.Next() }
func (c *FiberContext) Underlying() interface{} { c.checkReleased(); return c.fiberCtx }

// Clone returns a snapshot of the FiberContext that is safe to use in goroutines.
// Fiber's context is NOT safe to use after the handler returns, so we copy
// the essential request data into a standalone struct.
func (c *FiberContext) Clone() core.Context {
	s := acquireSnapshot()
	s.method = c.fiberCtx.Method()
	s.path = c.fiberCtx.Route().Path
	s.ip = c.fiberCtx.IP()
	s.fullURL = c.fiberCtx.OriginalURL()
	s.body = append(s.body[:0], c.fiberCtx.Body()...)

	// Reuse pooled maps instead of allocating new ones every Clone().
	c.copyHeadersInto(s)
	c.copyParamsInto(s)
	c.copyQueriesInto(s)
	return s
}

func (c *FiberContext) copyHeadersInto(s *FiberContextSnapshot) {
	if s.headers == nil {
		s.headers = make(map[string]string)
	} else {
		clear(s.headers)
	}
	for key, value := range c.fiberCtx.Request().Header.All() {
		s.headers[string(key)] = string(value)
	}
}

func (c *FiberContext) copyParamsInto(s *FiberContextSnapshot) {
	if s.params == nil {
		s.params = make(map[string]string)
	} else {
		clear(s.params)
	}
	for _, key := range c.fiberCtx.Route().Params {
		s.params[key] = c.fiberCtx.Params(key)
	}
}

func (c *FiberContext) copyQueriesInto(s *FiberContextSnapshot) {
	if s.queries == nil {
		s.queries = make(map[string]string)
	} else {
		clear(s.queries)
	}
	for key, value := range c.fiberCtx.Request().URI().QueryArgs().All() {
		s.queries[string(key)] = string(value)
	}
}

// ─── Internal helpers ───────────────────────────────────────────────────────

func wrapHandler(handler core.HandlerFunc, errHandler core.ErrorHandler) fiber.Handler {
	return func(fc fiber.Ctx) error {
		ctx := acquireContext(fc)
		defer releaseContext(ctx)
		if err := handler(ctx); err != nil {
			if errHandler != nil {
				errHandler(ctx, err)
			} else {
				core.DefaultErrorHandler(ctx, err)
			}
			return nil
		}
		return nil
	}
}

func wrapMiddleware(mw core.MiddlewareFunc, errHandler core.ErrorHandler) fiber.Handler {
	return func(fc fiber.Ctx) error {
		ctx := acquireContext(fc)
		defer releaseContext(ctx)
		next := func(c core.Context) error { return fc.Next() }
		handler := mw(next)
		if err := handler(ctx); err != nil {
			if errHandler != nil {
				errHandler(ctx, err)
			} else {
				core.DefaultErrorHandler(ctx, err)
			}
			return nil
		}
		return nil
	}
}

func applyRouteMiddleware(handler core.HandlerFunc, mws []core.MiddlewareFunc) core.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}
