package fiberadapter

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	core "github.com/ashrafAli23/nestgo/core"
	"github.com/gofiber/fiber/v3"
)

func newTestServer(t *testing.T, cfg *core.Config) (core.Server, *fiber.App) {
	t.Helper()
	if cfg == nil {
		cfg = core.DefaultConfig()
	}
	srv := New(cfg)
	app, ok := srv.Underlying().(*fiber.App)
	if !ok {
		t.Fatalf("Underlying() is not *fiber.App")
	}
	return srv, app
}

func doGet(t *testing.T, app *fiber.App, target string) (int, string, map[string]string) {
	t.Helper()
	req := httptest.NewRequest("GET", target, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	headers := map[string]string{}
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}
	return resp.StatusCode, string(body), headers
}

// ─── Error propagation & single dispatch ────────────────────────────────────

func TestHandlerErrorVisibleToMiddlewareAndHandledOnce(t *testing.T) {
	cfg := core.DefaultConfig()
	errCalls := 0
	cfg.ErrorHandler = func(c core.Context, err error) {
		errCalls++
		core.DefaultErrorHandler(c, err)
	}
	srv, app := newTestServer(t, cfg)

	var seen error
	srv.Use(func(next core.HandlerFunc) core.HandlerFunc {
		return func(c core.Context) error {
			err := next(c)
			seen = err // interceptor-style middleware must observe handler errors
			return err
		}
	})
	srv.GET("/boom", func(c core.Context) error {
		return core.ErrInternalServer("kaboom")
	})

	status, body, _ := doGet(t, app, "/boom")
	if seen == nil || !strings.Contains(seen.Error(), "kaboom") {
		t.Fatalf("middleware did not observe handler error, got: %v", seen)
	}
	if status != 500 {
		t.Fatalf("status = %d, want 500", status)
	}
	if errCalls != 1 {
		t.Fatalf("error handler invoked %d times, want exactly 1", errCalls)
	}
	if !strings.Contains(body, "kaboom") {
		t.Fatalf("body = %q, want error message", body)
	}
}

func TestErrorHandlerSkippedWhenResponseAlreadyWritten(t *testing.T) {
	cfg := core.DefaultConfig()
	errCalls := 0
	cfg.ErrorHandler = func(c core.Context, err error) {
		errCalls++
		core.DefaultErrorHandler(c, err)
	}
	srv, app := newTestServer(t, cfg)

	srv.GET("/partial", func(c core.Context) error {
		if err := c.String(200, "partial"); err != nil {
			return err
		}
		return core.ErrInternalServer("late failure")
	})

	status, body, _ := doGet(t, app, "/partial")
	if errCalls != 0 {
		t.Fatalf("error handler ran after response was written (%d calls)", errCalls)
	}
	if status != 200 || body != "partial" {
		t.Fatalf("got %d %q, want 200 %q", status, body, "partial")
	}
}

// ─── Panic recovery ─────────────────────────────────────────────────────────

func TestPanicRecoveredInto500(t *testing.T) {
	srv, app := newTestServer(t, nil)
	srv.GET("/panic", func(c core.Context) error {
		panic("nil map write, probably")
	})
	srv.GET("/ok", func(c core.Context) error {
		return c.String(200, "alive")
	})

	status, _, _ := doGet(t, app, "/panic")
	if status != 500 {
		t.Fatalf("panic status = %d, want 500", status)
	}
	// Process (and app) must still serve requests.
	status, body, _ := doGet(t, app, "/ok")
	if status != 200 || body != "alive" {
		t.Fatalf("server not alive after panic: %d %q", status, body)
	}
}

func TestPanicInMiddlewareRecovered(t *testing.T) {
	srv, app := newTestServer(t, nil)
	srv.Use(func(next core.HandlerFunc) core.HandlerFunc {
		return func(c core.Context) error {
			panic("middleware panic")
		}
	})
	srv.GET("/mw-panic", func(c core.Context) error { return c.String(200, "unreachable") })

	status, _, _ := doGet(t, app, "/mw-panic")
	if status != 500 {
		t.Fatalf("status = %d, want 500", status)
	}
}

// ─── ResponseBody / Body copy semantics ─────────────────────────────────────

func TestResponseBodyReturnsCopy(t *testing.T) {
	srv, app := newTestServer(t, nil)
	var captured []byte
	srv.GET("/rb", func(c core.Context) error {
		if err := c.String(200, "original"); err != nil {
			return err
		}
		captured = c.ResponseBody()
		// Mutate the live fasthttp buffer; the captured copy must not change.
		live := c.Underlying().(fiber.Ctx).Response().Body()
		for i := range live {
			live[i] = 'X'
		}
		return nil
	})

	doGet(t, app, "/rb")
	if string(captured) != "original" {
		t.Fatalf("ResponseBody aliased the live buffer: %q", captured)
	}
}

func TestBodyStableAfterHandler(t *testing.T) {
	srv, app := newTestServer(t, nil)
	var captured []byte
	srv.POST("/echo", func(c core.Context) error {
		b, err := c.Body()
		if err != nil {
			return err
		}
		if captured == nil { // keep only the FIRST request's body
			captured = b
			// Mutate the live fasthttp request buffer; the adapter copy
			// must not change.
			live := c.Underlying().(fiber.Ctx).Body()
			for i := range live {
				live[i] = 'Z'
			}
		}
		return c.NoContent(204)
	})

	req := httptest.NewRequest("POST", "/echo", strings.NewReader("payload-one"))
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	// Second request would reuse fasthttp buffers if Body() aliased them.
	req2 := httptest.NewRequest("POST", "/echo", strings.NewReader("XXXXXXXXXXX"))
	if _, err := app.Test(req2); err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if string(captured) != "payload-one" {
		t.Fatalf("Body() aliased fasthttp memory: %q, want payload-one", captured)
	}
}

// ─── Clone snapshot ─────────────────────────────────────────────────────────

func TestCloneSnapshotIndependence(t *testing.T) {
	srv, app := newTestServer(t, nil)
	var snap core.Context
	srv.POST("/users/:id", func(c core.Context) error {
		if snap == nil { // only snapshot the first request
			snap = c.Clone()
		}
		return c.NoContent(204)
	})

	req := httptest.NewRequest("POST", "/users/alice?x=1&empty=", strings.NewReader("body-one"))
	req.Header.Set("Authorization", "Bearer token-1")
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	// Fire a second, different request so any aliased fasthttp memory would
	// have been recycled/overwritten.
	req2 := httptest.NewRequest("POST", "/users/bob?x=2", strings.NewReader("body-two"))
	req2.Header.Set("Authorization", "Bearer token-2")
	if _, err := app.Test(req2); err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if got := snap.Param("id"); got != "alice" {
		t.Errorf("snapshot Param(id) = %q, want alice", got)
	}
	if got := snap.Method(); got != "POST" {
		t.Errorf("snapshot Method = %q, want POST", got)
	}
	wantURL := "http://example.com/users/alice?x=1&empty="
	if got := snap.FullURL(); got != wantURL {
		t.Errorf("snapshot FullURL = %q, want %q", got, wantURL)
	}
	// Case-insensitive header lookup on the snapshot.
	if got := snap.GetHeader("authorization"); got != "Bearer token-1" {
		t.Errorf("snapshot GetHeader(authorization) = %q, want Bearer token-1", got)
	}
	if got := snap.GetHeader("AUTHORIZATION"); got != "Bearer token-1" {
		t.Errorf("snapshot GetHeader(AUTHORIZATION) = %q, want Bearer token-1", got)
	}
	b, _ := snap.Body()
	if string(b) != "body-one" {
		t.Errorf("snapshot Body = %q, want body-one", b)
	}
	// QueryDefault on snapshot: present-but-empty returns "", absent returns default.
	if got := snap.QueryDefault("empty", "def"); got != "" {
		t.Errorf("snapshot QueryDefault(empty) = %q, want \"\"", got)
	}
	if got := snap.QueryDefault("missing", "def"); got != "def" {
		t.Errorf("snapshot QueryDefault(missing) = %q, want def", got)
	}

	// Double release must be a no-op (no panic, no double-pooling).
	ReleaseSnapshot(snap)
	ReleaseSnapshot(snap)
}

func TestReleaseSnapshotClearsState(t *testing.T) {
	s := acquireSnapshot()
	s.stdCtx = context.Background()
	s.headers = map[string]string{"Authorization": "secret"}
	s.params = map[string]string{"id": "1"}
	s.queries = map[string]string{"q": "v"}
	s.Set("k", "v")
	ReleaseSnapshot(s)

	if s.stdCtx != nil {
		t.Errorf("stdCtx not cleared on release")
	}
	if len(s.headers) != 0 || len(s.params) != 0 || len(s.queries) != 0 || len(s.locals) != 0 {
		t.Errorf("maps not cleared on release: h=%d p=%d q=%d l=%d",
			len(s.headers), len(s.params), len(s.queries), len(s.locals))
	}
}

// ─── Use-after-release ──────────────────────────────────────────────────────

func TestUseAfterReleasePanics(t *testing.T) {
	srv, app := newTestServer(t, nil)
	var leaked core.Context
	srv.GET("/leak", func(c core.Context) error {
		leaked = c
		return c.NoContent(204)
	})
	doGet(t, app, "/leak")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected use-after-release panic, got none")
		}
		if !strings.Contains(r.(string), "use-after-release") {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	leaked.Method()
}

// ─── Parity behaviors ───────────────────────────────────────────────────────

func TestQueryDefaultPresenceSemantics(t *testing.T) {
	srv, app := newTestServer(t, nil)
	srv.GET("/q", func(c core.Context) error {
		return c.String(200, "[%s]", c.QueryDefault("flag", "def"))
	})

	_, body, _ := doGet(t, app, "/q")
	if body != "[def]" {
		t.Errorf("absent key: body = %q, want [def]", body)
	}
	_, body, _ = doGet(t, app, "/q?flag=")
	if body != "[]" {
		t.Errorf("present-but-empty key: body = %q, want []", body)
	}
	_, body, _ = doGet(t, app, "/q?flag=x")
	if body != "[x]" {
		t.Errorf("present key: body = %q, want [x]", body)
	}
}

func TestStringVerbatimWithoutArgs(t *testing.T) {
	srv, app := newTestServer(t, nil)
	srv.GET("/sale", func(c core.Context) error {
		return c.String(200, "Sale: 50% off")
	})
	srv.GET("/fmt", func(c core.Context) error {
		return c.String(200, "n=%d", 42)
	})

	_, body, _ := doGet(t, app, "/sale")
	if body != "Sale: 50% off" {
		t.Errorf("literal %% mangled: %q", body)
	}
	_, body, _ = doGet(t, app, "/fmt")
	if body != "n=42" {
		t.Errorf("formatted output = %q, want n=42", body)
	}
}

func TestSendBytesDefaultContentType(t *testing.T) {
	srv, app := newTestServer(t, nil)
	srv.GET("/raw", func(c core.Context) error {
		return c.SendBytes(200, []byte{0x1, 0x2})
	})
	srv.GET("/typed", func(c core.Context) error {
		c.SetHeader("Content-Type", "application/pdf")
		return c.SendBytes(200, []byte{0x1, 0x2})
	})

	_, _, headers := doGet(t, app, "/raw")
	if ct := headers["Content-Type"]; ct != "application/octet-stream" {
		t.Errorf("default Content-Type = %q, want application/octet-stream", ct)
	}
	_, _, headers = doGet(t, app, "/typed")
	if ct := headers["Content-Type"]; ct != "application/pdf" {
		t.Errorf("explicit Content-Type = %q, want application/pdf", ct)
	}
}

func TestFullURLIncludesSchemeAndHost(t *testing.T) {
	srv, app := newTestServer(t, nil)
	var got string
	srv.GET("/things/:id", func(c core.Context) error {
		got = c.FullURL()
		return c.NoContent(204)
	})
	doGet(t, app, "/things/7?a=b")
	want := "http://example.com/things/7?a=b"
	if got != want {
		t.Errorf("FullURL = %q, want %q", got, want)
	}
}

// ─── Middleware composition ─────────────────────────────────────────────────

func TestMiddlewareCompositionOrder(t *testing.T) {
	srv, app := newTestServer(t, nil)
	var order []string
	mk := func(name string) core.MiddlewareFunc {
		return func(next core.HandlerFunc) core.HandlerFunc {
			return func(c core.Context) error {
				order = append(order, name)
				return next(c)
			}
		}
	}
	srv.Use(mk("use"))
	g := srv.Group("/g", mk("group"))
	g.GET("/r", func(c core.Context) error {
		order = append(order, "handler")
		return c.NoContent(204)
	}, mk("route"))

	status, _, _ := doGet(t, app, "/g/r")
	if status != 204 {
		t.Fatalf("status = %d, want 204", status)
	}
	want := []string{"use", "group", "route", "handler"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

// ─── Request context ────────────────────────────────────────────────────────

func TestRequestCtxIsNotBackground(t *testing.T) {
	srv, app := newTestServer(t, nil)
	var isBackground bool
	var overlaid interface{}
	type key struct{}
	srv.GET("/ctx", func(c core.Context) error {
		isBackground = c.RequestCtx() == context.Background()
		c.SetRequestCtx(context.WithValue(c.RequestCtx(), key{}, "v"))
		overlaid = c.RequestCtx().Value(key{})
		return c.NoContent(204)
	})
	doGet(t, app, "/ctx")
	if isBackground {
		t.Errorf("RequestCtx() returned context.Background(); want request-scoped context")
	}
	if overlaid != "v" {
		t.Errorf("SetRequestCtx overlay not visible, got %v", overlaid)
	}
}

// ─── Response introspection capabilities ────────────────────────────────────

func TestResponseIntrospectionCapabilities(t *testing.T) {
	srv, app := newTestServer(t, nil)
	srv.GET("/etag", func(c core.Context) error {
		if err := c.String(200, "full body"); err != nil {
			return err
		}
		rr, ok := c.(core.ResponseResetter)
		if !ok {
			t.Errorf("FiberContext does not implement ResponseResetter")
			return c.NoContent(500)
		}
		c.SetHeader("X-Probe", "yes")
		hr, ok := c.(core.ResponseHeaderReader)
		if !ok {
			t.Errorf("FiberContext does not implement ResponseHeaderReader")
		} else if got := hr.ResponseHeader("X-Probe"); got != "yes" {
			t.Errorf("ResponseHeader(X-Probe) = %q, want yes", got)
		}
		rr.ResetResponse()
		return c.NoContent(304)
	})

	status, body, _ := doGet(t, app, "/etag")
	if status != 304 || body != "" {
		t.Errorf("after ResetResponse: got %d %q, want 304 with empty body", status, body)
	}
}

func TestSetCookieSameSite(t *testing.T) {
	srv, app := newTestServer(t, nil)
	srv.GET("/cookie", func(c core.Context) error {
		ss, ok := c.(core.SameSiteCookieSetter)
		if !ok {
			t.Errorf("FiberContext does not implement SameSiteCookieSetter")
			return c.NoContent(500)
		}
		ss.SetCookieSameSite("sid", "v", 60, "/", "", true, true, "Strict")
		return c.NoContent(204)
	})
	_, _, headers := doGet(t, app, "/cookie")
	sc := headers["Set-Cookie"]
	if !strings.Contains(sc, "sid=v") || !strings.Contains(strings.ToLower(sc), "samesite=strict") {
		t.Errorf("Set-Cookie = %q, want sid=v with SameSite=Strict", sc)
	}
}
