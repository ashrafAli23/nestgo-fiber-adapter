package fiberadapter

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	core "github.com/ashrafAli23/nestgo/core"

	"github.com/gofiber/fiber/v3"
)

var _ core.Context = (*FiberContext)(nil)
var _ core.ResponseResetter = (*FiberContext)(nil)
var _ core.ResponseHeaderReader = (*FiberContext)(nil)
var _ core.SameSiteCookieSetter = (*FiberContext)(nil)

// acquireContext creates a fresh FiberContext for the request.
//
// FiberContexts are intentionally NOT pooled: a pooled struct that is
// re-acquired for the next request would let a stale reference (leaked to a
// goroutine) silently read another request's data through the old handle.
// With a per-request allocation the released flag is set exactly once and
// never cleared, so any use-after-release deterministically panics instead.
// The struct is two words plus an atomic.Bool — the allocation is negligible
// next to the work fiber itself does per request.
func acquireContext(fc fiber.Ctx) *FiberContext {
	seedRequestContext(fc)
	return &FiberContext{fiberCtx: fc}
}

// seedRequestContext makes RequestCtx() carry real cancellation/deadline
// semantics: fiber v3's Context() returns context.Background() unless a user
// context was set, so we seed it with the *fasthttp.RequestCtx (which
// implements context.Context and whose Done() fires on server shutdown).
// Note that fasthttp cannot signal per-request client disconnects, so
// cancellation granularity is coarser than net/http-based adapters.
//
// The RequestCtx is wrapped rather than handed out directly: fasthttp's own
// Done()/Err() re-read a *Server-level field (RequestCtx.s.done) on every
// call, which is only ever closed and later nilled out exactly once, during
// Server.Shutdown — unsynchronized on both sides. A handler-spawned goroutine
// that polls Done() in a loop (e.g. an SSE producer bound to this context, a
// documented pattern) then races that nil-write under the Go race detector,
// even though the channel value itself never changes for a request's
// lifetime. fixedDoneContext captures Done() exactly once, up front, so nothing
// downstream ever re-touches the racy field again.
func seedRequestContext(fc fiber.Ctx) {
	if fc.Context() == context.Background() {
		if reqCtx := fc.RequestCtx(); reqCtx != nil {
			fc.SetContext(&fixedDoneContext{Context: reqCtx, done: reqCtx.Done()})
		}
	}
}

// fixedDoneContext wraps a context.Context, overriding Done()/Err() to
// serve a Done() channel captured once at construction time instead of
// querying the wrapped context on every call. See seedRequestContext for why:
// it lets us hand out fasthttp's RequestCtx cancellation signal without
// repeatedly re-reading the underlying racy field.
type fixedDoneContext struct {
	context.Context
	done <-chan struct{}
}

func (c *fixedDoneContext) Done() <-chan struct{} { return c.done }

func (c *fixedDoneContext) Err() error {
	select {
	case <-c.done:
		return context.Canceled
	default:
		return nil
	}
}

func releaseContext(ctx *FiberContext) {
	ctx.released.Store(true)
	ctx.fiberCtx = nil
	ctx.body = nil
}

// checkReleased panics with a clear message if the context is used after the
// handler returned. Because FiberContexts are per-request (not pooled), the
// released flag is never reset and this check is reliable: a leaked reference
// always panics instead of silently reading another request's data.
// Note that the underlying fiber.Ctx / fasthttp buffers are still recycled,
// so Clone() remains the only safe way to hand data to a goroutine.
func (c *FiberContext) checkReleased() {
	if c.released.Load() {
		panic("[NestGo] use-after-release: FiberContext used after handler returned. " +
			"Fiber contexts are recycled. Use c.Clone() before passing to goroutines.")
	}
}

// FiberContext wraps fiber.Ctx to implement core.Context.
type FiberContext struct {
	fiberCtx fiber.Ctx
	// body caches an adapter-owned copy of the request body. fiber/fasthttp
	// body bytes alias per-connection buffers that are recycled after the
	// handler; the cache keeps Body() results stable (matching gin).
	body     []byte
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

// QueryDefault returns the default ONLY when the key is absent; a
// present-but-empty parameter ("?flag=") returns "" (matching gin).
func (c *FiberContext) QueryDefault(key, def string) string {
	c.checkReleased()
	if !c.fiberCtx.RequestCtx().QueryArgs().Has(key) {
		return def
	}
	return fiber.Query[string](c.fiberCtx, key)
}

func (c *FiberContext) GetHeader(key string) string { c.checkReleased(); return c.fiberCtx.Get(key) }
func (c *FiberContext) Cookie(name string) string   { c.checkReleased(); return c.fiberCtx.Cookies(name) }

// Body returns the request body. The bytes are copied once into an
// adapter-owned cache, so the returned slice remains valid after the handler
// returns (fiber's own Body() aliases recycled fasthttp buffers).
func (c *FiberContext) Body() ([]byte, error) {
	c.checkReleased()
	if c.body == nil {
		c.body = append([]byte(nil), c.fiberCtx.Body()...)
	}
	return c.body, nil
}
func (c *FiberContext) Bind(v interface{}) error { c.checkReleased(); return c.fiberCtx.Bind().Body(v) }
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

// String writes format verbatim when no values are given (so a literal '%'
// survives, matching gin); otherwise it applies fmt.Sprintf.
func (c *FiberContext) String(status int, format string, vals ...interface{}) error {
	c.checkReleased()
	c.fiberCtx.Status(status)
	if len(vals) == 0 {
		return c.fiberCtx.SendString(format)
	}
	return c.fiberCtx.SendString(fmt.Sprintf(format, vals...))
}

// SendBytes writes raw bytes. When no Content-Type has been set yet it
// defaults to application/octet-stream (cross-adapter contract).
func (c *FiberContext) SendBytes(status int, data []byte) error {
	c.checkReleased()
	c.fiberCtx.Status(status)
	h := &c.fiberCtx.Response().Header
	// Temporarily disable fasthttp's implicit text/plain default so we can
	// see whether a Content-Type was explicitly set.
	h.SetNoDefaultContentType(true)
	noCT := len(h.ContentType()) == 0
	h.SetNoDefaultContentType(false)
	if noCT {
		h.SetContentType("application/octet-stream")
	}
	return c.fiberCtx.Send(data)
}

func (c *FiberContext) SendStream(stream io.Reader) error {
	c.checkReleased()
	// fasthttp only pushes header bytes to the socket immediately when
	// ImmediateHeaderFlush is set; otherwise they sit buffered until the
	// body stream produces its first chunk (see writeBodyStream in
	// valyala/fasthttp). A client that waits for headers before writing
	// data (SSE/EventSource, or any reader fed from elsewhere, e.g. an
	// io.Pipe) would then deadlock waiting on a response that was never
	// actually sent. The flag is reset on the next request by fasthttp's
	// own Response.Reset(), so this only affects the current response.
	c.fiberCtx.Response().ImmediateHeaderFlush = true
	return c.fiberCtx.SendStream(stream)
}

func (c *FiberContext) SendFile(filePath string) error {
	c.checkReleased()
	return c.fiberCtx.SendFile(filePath)
}

func (c *FiberContext) Download(filePath string, filename string) error {
	c.checkReleased()
	return c.fiberCtx.Download(filePath, filename)
}

func (c *FiberContext) NoContent(status int) error {
	c.checkReleased()
	c.fiberCtx.Status(status)
	return nil
}
func (c *FiberContext) ResponseStatus() int {
	c.checkReleased()
	return c.fiberCtx.Response().StatusCode()
}

// ResponseBody returns a copy of the buffered response body, per the
// core.Context contract. fasthttp recycles the underlying buffer across
// requests, so returning it directly would let long-lived consumers (e.g.
// the Idempotency middleware's 24h cache) observe other requests' bytes.
//
// For a streamed response, fasthttp's Response.Body() is not a plain getter:
// when a body stream is set, calling it drains the stream to completion into
// a buffer and closes it — so calling it here would block until the stream
// ends and would destroy it for the real client. Return nil instead, which
// also matches gin's contract (ResponseBody() returns nil once a response
// has been streamed).
func (c *FiberContext) ResponseBody() []byte {
	c.checkReleased()
	if c.fiberCtx.Response().IsBodyStream() {
		return nil
	}
	return append([]byte(nil), c.fiberCtx.Response().Body()...)
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

// SetCookieSameSite implements core.SameSiteCookieSetter.
func (c *FiberContext) SetCookieSameSite(name, value string, maxAge int, path, domain string, secure, httpOnly bool, sameSite string) {
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
		SameSite: sameSite,
	})
}

// ResetResponse implements core.ResponseResetter: it discards the buffered
// response body and status so a middleware can replace the response (e.g.
// ETag converting a buffered 200 into a 304). It has no effect on bytes
// already streamed to the client.
func (c *FiberContext) ResetResponse() {
	c.checkReleased()
	resp := c.fiberCtx.Response()
	resp.ResetBody()
	resp.SetStatusCode(fiber.StatusOK)
}

// ResponseHeader implements core.ResponseHeaderReader.
func (c *FiberContext) ResponseHeader(key string) string {
	c.checkReleased()
	return string(c.fiberCtx.Response().Header.Peek(key))
}

func (c *FiberContext) Redirect(status int, url string) error {
	c.checkReleased()
	return c.fiberCtx.Redirect().Status(status).To(url)
}

// ─── Metadata ───────────────────────────────────────────────────────────────

func (c *FiberContext) ClientIP() string { c.checkReleased(); return c.fiberCtx.IP() }

// FullURL returns scheme://host/path?query (cross-adapter contract).
func (c *FiberContext) FullURL() string {
	c.checkReleased()
	// String concatenation allocates a fresh string, so the result does not
	// alias fasthttp's recycled buffers.
	return c.fiberCtx.Scheme() + "://" + c.fiberCtx.Host() + c.fiberCtx.OriginalURL()
}

// ─── Context Storage ────────────────────────────────────────────────────────

func (c *FiberContext) Set(key string, value interface{}) {
	c.checkReleased()
	c.fiberCtx.Locals(key, value)
}

func (c *FiberContext) Get(key string) interface{} {
	c.checkReleased()
	return c.fiberCtx.Locals(key)
}

// ─── Flow Control ───────────────────────────────────────────────────────────

func (c *FiberContext) Next() error             { c.checkReleased(); return c.fiberCtx.Next() }
func (c *FiberContext) Underlying() interface{} { c.checkReleased(); return c.fiberCtx }

func (c *FiberContext) RequestCtx() context.Context {
	c.checkReleased()
	// Seeded with the *fasthttp.RequestCtx at acquisition (see
	// seedRequestContext), so this carries server-shutdown cancellation
	// rather than a bare context.Background().
	return c.fiberCtx.Context()
}

func (c *FiberContext) SetRequestCtx(ctx context.Context) {
	c.checkReleased()
	c.fiberCtx.SetContext(ctx)
}

// Clone returns a snapshot of the FiberContext that is safe to use in
// goroutines. Fiber's context is NOT safe to use after the handler returns,
// so we deep-copy the essential request data into a standalone struct.
// Every string sourced from fiber is cloned: fiber v3 returns zero-copy
// aliases into fasthttp's per-connection buffers ("valid only within the
// handler"), which would silently mutate into the next request's bytes.
func (c *FiberContext) Clone() core.Context {
	c.checkReleased()
	s := acquireSnapshot()
	s.stdCtx = context.WithoutCancel(c.fiberCtx.Context())
	s.method = strings.Clone(c.fiberCtx.Method())
	s.path = strings.Clone(c.fiberCtx.Route().Path)
	s.ip = strings.Clone(c.fiberCtx.IP())
	s.fullURL = c.FullURL() // composed via concatenation — already a fresh string
	s.body = append(s.body[:0], c.fiberCtx.Body()...)

	// Reuse pooled maps instead of allocating new ones every Clone().
	c.copyHeadersInto(s)
	c.copyParamsInto(s)
	c.copyQueriesInto(s)
	return s
}

// copyHeadersInto stores headers under canonical MIME keys so snapshot
// lookups are case-insensitive like the live context. The FIRST occurrence
// of a duplicate header wins, matching fasthttp Peek semantics.
func (c *FiberContext) copyHeadersInto(s *FiberContextSnapshot) {
	if s.headers == nil {
		s.headers = make(map[string]string)
	} else {
		clear(s.headers)
	}
	for key, value := range c.fiberCtx.Request().Header.All() {
		k := textproto.CanonicalMIMEHeaderKey(string(key))
		if _, ok := s.headers[k]; !ok {
			s.headers[k] = string(value)
		}
	}
}

func (c *FiberContext) copyParamsInto(s *FiberContextSnapshot) {
	if s.params == nil {
		s.params = make(map[string]string)
	} else {
		clear(s.params)
	}
	for _, key := range c.fiberCtx.Route().Params {
		s.params[strings.Clone(key)] = strings.Clone(c.fiberCtx.Params(key))
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

// wrapHandler adapts a fully composed core.HandlerFunc into the single native
// fiber handler registered for a route. It:
//   - acquires the adapter context,
//   - recovers panics into a 500 through the error path (adapter-level safety
//     net, independent of middleware.Recovery()),
//   - runs the composed middleware+handler chain, and
//   - on a returned error invokes the configured error handler EXACTLY once,
//     and only if nothing has been written to the response yet.
//
// It always returns nil to fiber so fiber's own ErrorHandler never runs a
// second time for the same error.
func wrapHandler(handler core.HandlerFunc, errHandler core.ErrorHandler) fiber.Handler {
	return func(fc fiber.Ctx) error {
		ctx := acquireContext(fc)
		defer releaseContext(ctx)

		if err := safeInvoke(handler, ctx); err != nil {
			dispatchError(ctx, fc, err, errHandler)
		}
		return nil
	}
}

// safeInvoke runs the composed chain, converting panics into a 500 error so
// one panicking handler cannot kill the process (fiber/fasthttp do not
// recover handler panics themselves).
func safeInvoke(handler core.HandlerFunc, ctx *FiberContext) (err error) {
	defer func() {
		if r := recover(); r != nil {
			core.Log().Error("panic recovered in handler",
				core.F("panic", fmt.Sprintf("%v", r)),
				core.F("stack", string(debug.Stack())))
			err = core.ErrInternalServer("Internal Server Error")
		}
	}()
	return handler(ctx)
}

// dispatchError invokes the configured error handler exactly once, and only
// if nothing has been written to the response yet; otherwise it logs the
// error to avoid double-written responses.
func dispatchError(ctx *FiberContext, fc fiber.Ctx, err error, errHandler core.ErrorHandler) {
	resp := fc.Response()
	// IsBodyStream() must be checked FIRST: resp.Body() is not a cheap getter
	// when a body stream is set — it drains the stream to completion into a
	// buffer and closes it. Evaluating it first here would block this
	// dispatch (and destroy the stream) on every error returned after
	// SendStream, instead of just logging and returning.
	if resp.IsBodyStream() || len(resp.Body()) > 0 {
		core.Log().Error("handler returned error after response was written",
			core.F("error", err.Error()))
		return
	}
	if errHandler != nil {
		errHandler(ctx, err)
	} else {
		core.DefaultErrorHandler(ctx, err)
	}
}

// wrapMiddlewareChain composes core middleware in front of NATIVE fiber
// handlers (static files). The terminal handler continues the fiber chain
// via fc.Next(). Errors and panics go through the same single-dispatch error
// path as wrapHandler.
func wrapMiddlewareChain(mws []core.MiddlewareFunc, errHandler core.ErrorHandler) fiber.Handler {
	return func(fc fiber.Ctx) error {
		ctx := acquireContext(fc)
		defer releaseContext(ctx)

		next := core.HandlerFunc(func(core.Context) error { return fc.Next() })
		if err := safeInvoke(applyRouteMiddleware(next, mws), ctx); err != nil {
			dispatchError(ctx, fc, err, errHandler)
		}
		return nil
	}
}

// applyRouteMiddleware composes mws around handler with mws[0] outermost:
// final = mws[0](mws[1](...(handler))).
func applyRouteMiddleware(handler core.HandlerFunc, mws []core.MiddlewareFunc) core.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}
