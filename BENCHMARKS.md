# Benchmarks

In-process request benchmarks (no network): the raw engine vs NestGo running on it.
They measure NestGo's overhead over the engine, not absolute throughput.

Run them yourself:

```bash
go test -run '^$' -bench . -benchmem -count 10 ./...
```

## Results

Machine: 12th Gen Intel(R) Core(TM) i7-12850HX, Go 1.26.7 linux/amd64 (module pins 1.25.x;
benchmarks run with the local toolchain), 2026-08-25

| Benchmark | ns/op median | ns/op range | B/op | allocs/op |
|---|---|---|---|---|
| RawFiber_HelloJSON | 1046 | 848–1346 | 337 | 6 |
| NestGoFiber_HelloJSON | 1184 | 1104–1944 | 409 | 8 |
| RawFiber_Middleware3 | 1302 | 1262–1379 | 369 | 7 |
| NestGoFiber_Middleware3 | 1980 | 1882–3602 | 490 | 12 |

Values are the median and min–max range of 10 runs (`go test -count 10`).

Ratio (NestGo/raw, median ns/op): HelloJSON 1.1x, Middleware3 1.5x.

Deltas that fall inside the raw side's own run-to-run range are noise, not signal — only a
difference that clears both sides' ranges reflects real overhead. `NestGoFiber_Middleware3`'s
own range (1882–3602) is unusually wide across these 10 runs (likely scheduler/GC jitter);
its median is still a >1x delta over `RawFiber_Middleware3`'s much tighter 1262–1379 range.

**What's identical** between the raw and NestGo columns: the JSON payload
(`map[string]bool{"ok": true}`), the routed path and method, the `RequestCtx` itself — built
once via fasthttp's own `RequestCtx.Init` before the timed loop starts, then reused every
iteration with only `Response.Reset()`/`ResetUserValues()` clearing per-request state (on both
sides) — the number of recovery layers (one each — `fiberadapter.New` already installs
`recover.New()`, so the raw side adds its own to match rather than NestGo running two), and
doing real request-ID work (16 random bytes, hex-encoded into the header) instead of a literal
string on the raw side.

**What still differs**, counted as NestGo's overhead by design: NestGo's per-handler safety
wrapper and its `core.Context` adaptation over the native `fiber.Ctx`/`RequestCtx`; and in
`Middleware3`, the "guard" is a real `core.Guard` dispatched through NestGo's guard pipeline
on the NestGo side, versus a no-op middleware closure on the raw side — that gap is the
guard-dispatch mechanism's cost, not noise.

Gin and Fiber numbers are **not comparable to each other**: the Gin adapter's harness
measures through `net/http/httptest` (an `httptest.ResponseRecorder` is allocated per
iteration on both the raw and NestGo side there), while this harness measures through a
fasthttp `RequestCtx` directly, with no such recorder. Only the ratio between the two
columns within one adapter's table is meaningful.

`Middleware3` = recovery + request-id header + an allow-all guard on both sides, so the
comparison is like-for-like on payload, recovery count, and request-ID cost — see above for
what's deliberately still asymmetric.
