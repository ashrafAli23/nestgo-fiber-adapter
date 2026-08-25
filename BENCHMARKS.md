# Benchmarks

In-process request benchmarks (no network): the raw engine vs NestGo running on it.
They measure NestGo's overhead over the engine, not absolute throughput.

Run them yourself:

```bash
go test -run '^$' -bench . -benchmem -count 3 ./...
```

## Results

Machine: 12th Gen Intel(R) Core(TM) i7-12850HX, Go go1.26.7 linux/amd64, 2026-08-25

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| RawFiber_HelloJSON | 1239 | 432 | 6 |
| NestGoFiber_HelloJSON | 1534 | 409 | 8 |
| RawFiber_Middleware3 | 1697 | 432 | 6 |
| NestGoFiber_Middleware3 | 2617 | 490 | 12 |

Values are the median of 3 runs (`go test -count 3`); raw output in `/tmp/bench-fiber.txt`.

`Middleware3` = recovery + request-id header + an allow-all guard on both sides, so the
comparison is like-for-like.
