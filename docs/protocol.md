# How the test works

## Wire protocol

| Endpoint                | Method   | Purpose                                                               |
| ----------------------- | -------- | --------------------------------------------------------------------- |
| `GET  /ping`            | GET/HEAD | Returns `204 No Content`. Used to sample round-trip latency.           |
| `GET  /download?bytes=N` | GET      | Streams `N` bytes of incompressible random data with `Content-Length`. |
| `POST /upload`          | POST/PUT | Drains the request body and replies `{"bytes": N}` with the count.    |
| `GET  /api/info`        | GET      | Returns client IP, server hostname, server time, and best-effort ISP/location. |

Random bytes for `/download` come from `crypto/rand`, so transparent
proxies and accelerators can't compress them away. The client is
expected to read until the server closes the body.

`/upload` reads until the body ends or `MaxUploadMiB` is reached
(default 1 GiB), then replies with the byte count it actually drained.
This is useful for verifying that no traffic was silently dropped along
the way.

## Measurement methodology

### Latency

The client sends 10 GETs to `/ping` (default) spaced 50 ms apart.

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
