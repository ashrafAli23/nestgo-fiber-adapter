package fiberadapter

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"sync"

	core "github.com/ashrafAli23/nestgo/core"
)

var _ core.Context = (*FiberContextSnapshot)(nil)

// snapshotPool reuses FiberContextSnapshot structs to reduce GC pressure
// when Clone() is called frequently (e.g. fanning out to goroutines).
var snapshotPool = sync.Pool{
	New: func() interface{} {
		return &FiberContextSnapshot{
			locals: make(map[string]interface{}),
		}
	},
}

func acquireSnapshot() *FiberContextSnapshot {
	s := snapshotPool.Get().(*FiberContextSnapshot)
	// Reset locals map
	for k := range s.locals {
		delete(s.locals, k)
	}
	return s
}

// ReleaseSnapshot returns the snapshot to the pool for reuse.
// Call this when you're done with a cloned context in a goroutine.
// Optional — if not called, the GC will collect it normally.
func ReleaseSnapshot(c core.Context) {
	if s, ok := c.(*FiberContextSnapshot); ok {
		s.method = ""
		s.path = ""
		s.ip = ""
		s.fullURL = ""
		s.body = s.body[:0]
		// Keep maps allocated for reuse — Clone will clear() them.
		snapshotPool.Put(s)
	}
}

// FiberContextSnapshot is a read-only copy of FiberContext that is safe
// to use in goroutines. It holds copied request data, not a reference
// to the original fiber.Ctx (which is recycled after the handler returns).
type FiberContextSnapshot struct {
	method  string
	path    string
	ip      string
	fullURL string
	body    []byte
	headers map[string]string
	params  map[string]string
	queries map[string]string
	locals  map[string]interface{}
	stdCtx  context.Context
}

// ─── Request (read from snapshot) ──────────────────────────────────────────

func (c *FiberContextSnapshot) Method() string          { return c.method }
func (c *FiberContextSnapshot) Path() string            { return c.path }
func (c *FiberContextSnapshot) Param(key string) string { return c.params[key] }
func (c *FiberContextSnapshot) Query(key string) string { return c.queries[key] }

func (c *FiberContextSnapshot) QueryDefault(key, def string) string {
	if v, ok := c.queries[key]; ok && v != "" {
		return v
	}
	return def
}

func (c *FiberContextSnapshot) GetHeader(key string) string { return c.headers[key] }
func (c *FiberContextSnapshot) Cookie(name string) string   { return "" }
func (c *FiberContextSnapshot) Body() ([]byte, error)       { return c.body, nil }
func (c *FiberContextSnapshot) Bind(v interface{}) error {
	return core.ErrInternalServer("Bind() not supported on cloned context")
}
func (c *FiberContextSnapshot) FormValue(key string) string { return "" }
func (c *FiberContextSnapshot) FormFile(key string) (*multipart.FileHeader, error) {
	return nil, core.ErrInternalServer("FormFile() not supported on cloned context")
}
func (c *FiberContextSnapshot) ContentType() string { return c.headers["Content-Type"] }
func (c *FiberContextSnapshot) IsWebSocket() bool   { return false }

// ─── Response (not supported on snapshot) ──────────────────────────────────
// Snapshots are read-only. Response methods are no-ops or return errors.

func (c *FiberContextSnapshot) Status(code int) core.Context { return c }
func (c *FiberContextSnapshot) JSON(status int, data interface{}) error {
	return core.ErrInternalServer("JSON() not supported on cloned context")
}
func (c *FiberContextSnapshot) XML(status int, data interface{}) error {
	return core.ErrInternalServer("XML() not supported on cloned context")
}
func (c *FiberContextSnapshot) String(status int, format string, vals ...interface{}) error {
	return core.ErrInternalServer("String() not supported on cloned context")
}
func (c *FiberContextSnapshot) SendBytes(status int, data []byte) error {
	return core.ErrInternalServer("SendBytes() not supported on cloned context")
}
func (c *FiberContextSnapshot) SendStream(stream io.Reader) error {
	return core.ErrInternalServer("SendStream() not supported on cloned context")
}
func (c *FiberContextSnapshot) SendFile(filePath string) error {
	return core.ErrInternalServer("SendFile() not supported on cloned context")
}
func (c *FiberContextSnapshot) Download(filePath string, filename string) error {
	return core.ErrInternalServer("Download() not supported on cloned context")
}
func (c *FiberContextSnapshot) NoContent(status int) error {
	return core.ErrInternalServer("NoContent() not supported on cloned context")
}
func (c *FiberContextSnapshot) ResponseStatus() int    { return 0 }
func (c *FiberContextSnapshot) ResponseBody() []byte   { return nil }
func (c *FiberContextSnapshot) SetHeader(k, v string) {}
func (c *FiberContextSnapshot) SetCookie(name, value string, maxAge int, path, domain string, secure, httpOnly bool) {
}
func (c *FiberContextSnapshot) Redirect(status int, url string) error {
	return core.ErrInternalServer("Redirect() not supported on cloned context")
}

// ─── Metadata ──────────────────────────────────────────────────────────────

func (c *FiberContextSnapshot) ClientIP() string { return c.ip }
func (c *FiberContextSnapshot) FullURL() string {
	if c.fullURL != "" {
		return c.fullURL
	}
	return fmt.Sprintf("%s", c.path)
}

// ─── Context Storage ───────────────────────────────────────────────────────

func (c *FiberContextSnapshot) Set(key string, value interface{}) {
	c.locals[key] = value
}

func (c *FiberContextSnapshot) Get(key string) interface{} {
	return c.locals[key]
}

// ─── Flow Control ──────────────────────────────────────────────────────────

func (c *FiberContextSnapshot) Next() error                 { return nil }
func (c *FiberContextSnapshot) Underlying() interface{}     { return nil }
func (c *FiberContextSnapshot) Clone() core.Context         { return c }
func (c *FiberContextSnapshot) RequestCtx() context.Context { return c.stdCtx }
func (c *FiberContextSnapshot) SetRequestCtx(ctx context.Context) { c.stdCtx = ctx }
