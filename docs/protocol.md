# How the test works

## Wire protocol

| Endpoint                 | Method   | Purpose                                                               |
| ------------------------ | -------- | --------------------------------------------------------------------- |
| `GET  /ping`             | GET/HEAD | Returns `204 No Content`. Used by the CLI to sample round-trip latency. |
| `GET  /ws`               | Upgrade  | WebSocket. Server-driven ping using WS Ping/Pong control frames; results streamed back as JSON text messages. Used by the browser UI for accurate sub-millisecond ping. |
| `GET  /download?bytes=N` | GET      | Streams `N` bytes of incompressible random data with `Content-Length`. |
| `POST /upload`           | POST/PUT | Drains the request body and replies `{"bytes": N}` with the count.    |
| `GET  /api/info`         | GET      | Returns client IP, server hostname, server time, and best-effort ISP/location. |

Random bytes for `/download` come from `crypto/rand`, so transparent
proxies and accelerators can't compress them away. The client is
expected to read until the server closes the body.

`/upload` reads until the body ends or `MaxUploadMiB` is reached
(default 1 GiB), then replies with the byte count it actually drained.
This is useful for verifying that no traffic was silently dropped along
the way.

## Measurement methodology

### Latency

There are two paths depending on which client you're using:

#### CLI path (`/ping`)

The CLI sends 10 GETs to `/ping` spaced 50 ms apart and measures the
duration with Go's monotonic clock. Each round-trip is a thin syscall
+ kernel TCP exchange — overhead is microseconds, so the wall-clock
measurement is accurate.

#### Browser path (`/ws`)

The browser uses a WebSocket because *fetch + `performance.now()`*
can't measure sub-millisecond RTT in the browser:

1. Browsers clamp `performance.now()` and Resource Timing entries for
   Spectre mitigation: 1 ms in Firefox / Safari, ~100 µs in Chrome.
   On loopback the actual RTT is below those clamps.
2. Each `fetch()` traverses the renderer process → IPC → network
   process → kernel → and back. That IPC + microtask scheduling adds
   1–3 ms even with HTTP keep-alive.

WebSocket Ping/Pong frames (RFC 6455 §5.5.2) are handled by the
browser's network process **without involving the JS event loop**.
The server sends a Ping, the browser auto-Pongs at protocol level,
the server measures the round-trip with its own nanosecond clock,
and streams the result back as a JSON text message:

```json
{"type": "ping", "seq": 0, "rtt_ms": 0.21}
```

The final message in the stream is `{"type": "done"}`.

For both paths:

- **Reported value:** the **median** RTT of the samples.
- **Jitter:** the mean of the absolute deltas between consecutive
  samples (loosely
  [RFC 3550](https://datatracker.ietf.org/doc/html/rfc3550) style — a
  measure of how "smooth" the latency is, not the standard deviation).

### Download / upload

The client opens **4 parallel HTTP streams** (default) and either reads
from `/download` or POSTs random bytes to `/upload` for **10 seconds**
(default).

The first **1.5 seconds are discarded as ramp-up** while TCP congestion
control scales up. The reported throughput is computed over the
remaining stable window.

```
0s              1.5s                          11s
|<-- ramp -->|<------- measured window ------>|
└── ignored ┘└─── bytes / seconds = Mbps ────┘
```

If the test is shorter than the ramp-up (e.g. `--duration 1s`), the
whole window is used and the result is necessarily noisier.

All of these defaults are tunable: `--duration`, `--streams`, plus the
package-level defaults in `internal/speedtest`.

### Why parallel streams?

A single TCP connection is bottlenecked by congestion-control AIMD
behavior and per-flow scheduling on intermediate hops. Multiple
concurrent flows saturate a high-bandwidth path much more reliably and
match what real-world workloads (browsers, CDN downloads) do. 4 streams
is a sensible default; bump `--streams 8` on links where you suspect
single-flow throughput is being capped.

### Mbps formula

```
Mbps = (bytes × 8) ÷ seconds ÷ 1 000 000
```

i.e. **megabits per second**, decimal (1 Mb = 1 000 000 bits), matching
how ISPs advertise plans. Not mebibits, not megabytes.
