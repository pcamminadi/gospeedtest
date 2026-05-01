# gospeedtest

[![CI](https://github.com/pcamminadi/gospeedtest/actions/workflows/ci.yml/badge.svg)](https://github.com/pcamminadi/gospeedtest/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/pcamminadi/gospeedtest?sort=semver)](https://github.com/pcamminadi/gospeedtest/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/pcamminadi/gospeedtest.svg)](https://pkg.go.dev/github.com/pcamminadi/gospeedtest)
[![Go Report Card](https://goreportcard.com/badge/github.com/pcamminadi/gospeedtest)](https://goreportcard.com/report/github.com/pcamminadi/gospeedtest)
[![Go version](https://img.shields.io/github/go-mod/go-version/pcamminadi/gospeedtest)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

An open-source, self-hosted internet speed test, written in Go. Inspired by
speedtest.net — keeps the fancy gauge in the browser and adds a rich terminal
UI for headless boxes.

A single binary serves both the speed-test endpoints and the web UI, and the
same binary ships a CLI mode that runs the test from a terminal.

## Contents

- [Features](#features)
- [Install](#install)
- [Quick start](#quick-start)
- [How the test works](#how-the-test-works)
- [`gospeedtest server` reference](#gospeedtest-server-reference)
- [`gospeedtest cli` reference](#gospeedtest-cli-reference)
- [Web UI](#web-ui)
- [Privacy and external services](#privacy-and-external-services)
- [Building and contributing](#building-and-contributing)
- [Project layout](#project-layout)
- [Releases and CI](#releases-and-ci)
- [License](#license)

## Features

- **Browser UI** at `/` — animated SVG gauge, live ping / download / upload,
  ISP and IP info, served from assets embedded in the binary (no Node, no
  build step).
- **CLI mode** with a [Bubble Tea](https://github.com/charmbracelet/bubbletea)
  TUI — animated gauge, live numbers, works over SSH. `--json` for
  machine-readable output.
- **Default-mode CLI**: running the bare binary (no subcommand) launches the
  CLI against `http://localhost:8080`.
- **Single Go binary**, no runtime dependencies. Cross-compiled to
  linux / darwin / windows × amd64 / arm64 by CI.
- **HTTP-based protocol** (LibreSpeed-style): parallel `GET /download` and
  `POST /upload` streams plus a `/ping` endpoint. No proprietary protocol —
  any reverse proxy / load balancer works without special handling.
- **MIT licensed**.

## Install

### Pre-built binaries (recommended)

Pre-built binaries for Linux, macOS, and Windows (amd64 + arm64) are produced
on every tagged release. Grab the latest from the
[Releases page](https://github.com/pcamminadi/gospeedtest/releases/latest).
Each release also publishes a `checksums.txt` (SHA-256) so you can verify the
download.

```sh
# Example: linux/amd64
curl -L -o gospeedtest.tar.gz \
  https://github.com/pcamminadi/gospeedtest/releases/latest/download/gospeedtest_v0.1.0_linux_amd64.tar.gz
tar -xzf gospeedtest.tar.gz
./gospeedtest version
```

### `go install`

```sh
go install github.com/pcamminadi/gospeedtest/cmd/gospeedtest@latest
```

### From source

```sh
git clone https://github.com/pcamminadi/gospeedtest
cd gospeedtest
go build ./cmd/gospeedtest
```

## Quick start

Start a server on port 8080 in one terminal:

```sh
./gospeedtest server
```

…then open <http://localhost:8080> in a browser, **or** run the CLI in
another terminal:

```sh
./gospeedtest                          # defaults to http://localhost:8080
./gospeedtest --server http://my-host  # against a remote gospeedtest server
./gospeedtest --json                   # one-shot, machine-readable output
```

## How the test works

### Wire protocol

| Endpoint                | Method   | Purpose                                                               |
| ----------------------- | -------- | --------------------------------------------------------------------- |
| `GET  /ping`            | GET/HEAD | Returns `204 No Content`. Used to sample round-trip latency.           |
| `GET  /download?bytes=N` | GET      | Streams `N` bytes of incompressible random data with `Content-Length`. |
| `POST /upload`          | POST/PUT | Drains the request body and replies `{"bytes": N}` with the count.    |
| `GET  /api/info`        | GET      | Returns client IP, server hostname, server time, and best-effort ISP/location. |

Random bytes for `/download` come from `crypto/rand`, so transparent proxies
and accelerators can't compress them away.

### Measurement methodology

- **Latency** — the client sends 10 GETs to `/ping` (default) spaced 50 ms
  apart. The reported value is the **median** RTT; jitter is the mean of the
  absolute deltas between consecutive samples (loosely RFC 3550-style).
- **Download / upload** — the client opens **4 parallel HTTP streams**
  (default) and either reads from `/download` or POSTs random bytes to
  `/upload` for **10 seconds** (default).
- The first **1.5 seconds are discarded as ramp-up** while TCP congestion
  control scales up. The reported throughput is computed over the remaining
  stable window. If the test is shorter than the ramp-up, the whole window
  is used.
- All defaults are tunable: `--duration`, `--streams`, plus the package-level
  defaults in `internal/speedtest`.

## `gospeedtest server` reference

Run the HTTP server that hosts the web UI and the speed-test endpoints.

```sh
gospeedtest server [--addr :8080] [--ipinfo-token TOKEN]
```

### Flags

| Flag             | Default  | Purpose                                                                 |
| ---------------- | -------- | ----------------------------------------------------------------------- |
| `--addr`         | `:8080`  | Listen address. Use `:0` for an ephemeral port, or `127.0.0.1:8080` to bind only to localhost. |
| `--ipinfo-token` | *(none)* | Optional [ipinfo.io](https://ipinfo.io) token — passed as `?token=…` on lookup. Without one, gospeedtest falls back to ipinfo's free tier (subject to undocumented per-source-IP rate limits). With one, you get a higher quota and more reliable results. See [Privacy and external services](#privacy-and-external-services). |

### HTTP endpoints

The server registers the protocol endpoints (above) plus serves the embedded
web UI (`index.html`, `style.css`, `app.js`, etc.) from `/`.

`/api/info` returns:

```json
{
  "client_ip": "203.0.113.5",
  "isp":       "Acme Internet (AS64500)",
  "city":      "Berlin",
  "region":    "Berlin",
  "country":   "DE",
  "server_host": "gst-01.example.com",
  "server_time": "2026-05-01T11:11:40Z"
}
```

`isp`, `city`, `region`, and `country` are **best-effort**: the server
queries [ipinfo.io](https://ipinfo.io) with a 2-second timeout. On error or
timeout, those fields are omitted. Private and loopback IPs are skipped — see
[Privacy](#privacy-and-external-services).

### Running behind a reverse proxy

`/api/info` honors the standard proxy headers when computing `client_ip`:

1. The first entry of `X-Forwarded-For`, if present.
2. Otherwise, `X-Real-IP`.
3. Otherwise, the host portion of `r.RemoteAddr`.

So `nginx`/`Caddy`/`Traefik` reverse-proxy setups Just Work as long as your
proxy sets one of those headers.

The server intentionally has **no `WriteTimeout`** — long downloads must not
be cut off mid-stream — and a 60-second `ReadTimeout` to bound slow uploads.

### Safety caps

The handlers cap individual request sizes so a malicious or buggy client
cannot make the server stream forever:

| Limit             | Default    | Purpose                                                                 |
| ----------------- | ---------- | ----------------------------------------------------------------------- |
| `MaxDownloadMiB`  | `1024` MiB | A `?bytes=` value above this is silently capped.                         |
| `MaxUploadMiB`    | `1024` MiB | Body bytes beyond this are dropped (`http.MaxBytesReader`).              |
| `ReadTimeout`     | `60s`      | Per-request read deadline.                                              |

These are not currently exposed as CLI flags. To change them, set the
corresponding fields on `server.Config` from your own embedding of the
package, or open an issue / PR.

### CORS and cache control

The server sets:

- `Access-Control-Allow-Origin: *` on every response (the speed test isn't
  authenticated; the UI is served from the same origin in normal use).
- `Cache-Control: no-store, no-cache, must-revalidate` on `/ping`,
  `/download`, `/upload`, and `/api/info` so intermediate proxies can't
  serve stale data and falsely inflate measurements.
- Static UI assets (`/`, `index.html`, `style.css`, `app.js`) **are**
  cacheable — they don't affect measurement accuracy.

`OPTIONS` requests return `204 No Content` for CORS preflight.

## `gospeedtest cli` reference

Run the speed test from a terminal.

```sh
gospeedtest                                       # default mode
gospeedtest [--server URL] [--json] ...           # default mode with flags
gospeedtest cli    [--server URL] ...             # explicit form, identical
```

The CLI is the **default subcommand** — invoking the binary with no argument
(or with arguments that don't start with `server`/`cli`/`version`/`help`)
falls through to CLI mode against `http://localhost:8080`.

### Flags

| Flag         | Default                   | Purpose                                                  |
| ------------ | ------------------------- | -------------------------------------------------------- |
| `--server`   | `http://localhost:8080`   | URL of a `gospeedtest server` instance.                  |
| `--json`     | `false`                   | Emit a single JSON object summarizing the run, then exit. Useful for scripting or non-tty environments. |
| `--duration` | `10s`                     | Per-phase duration (download and upload). Accepts any `time.Duration`. |
| `--streams`  | `4`                       | Number of parallel HTTP streams used for download and upload. |

### Keybindings (TUI mode)

| Key              | Action       |
| ---------------- | ------------ |
| `q`              | Quit         |
| `Esc`            | Quit         |
| `Ctrl+C`         | Quit         |

The TUI runs **inline** (not in an alt-screen), so the final frame stays in
your scrollback after the program exits — you can review the results.

### JSON output schema

`gospeedtest --json` writes a single object to stdout:

```json
{
  "ping_ms":        14.3,
  "jitter_ms":      0.9,
  "download_mbps":  942.7,
  "upload_mbps":    412.1,
  "bytes_down":     1184932864,
  "bytes_up":       515325952,
  "duration":       21500000000,
  "started_at":     "2026-05-01T12:48:27.894010+02:00"
}
```

Notes:
- `duration` is nanoseconds (Go's default `time.Duration` JSON encoding).
- `*_mbps` fields are megabits per second (1 Mb = 1,000,000 bits).
- `bytes_*` are total bytes transferred during the corresponding phase
  (including ramp-up).
- `started_at` is RFC 3339 with the local timezone.

### Network info shown in the TUI

The CLI panel shows:

- **public IP** — from the server's `/api/info`.
- **ISP**, **location** — best-effort from ipinfo.io (server-side lookup).
- **server** — the server's `os.Hostname()`.
- **local IP** — the local source IP that would be used to reach the
  internet (computed via a UDP "dial" to `8.8.8.8:53` — no packet is sent;
  the kernel just consults the route table).
- **gateway** — the system default route, parsed from `route -n get default`
  on macOS/BSD, `ip route show default` (with `route -n` fallback) on Linux,
  and `route print 0.0.0.0` on Windows.

`local IP` and `gateway` are detected **client-side** and never leave your
machine.

## Web UI

Served at `/` from the embedded `web/assets/` bundle. It's vanilla HTML +
CSS + JavaScript — no build step, no framework. Inspect the source from your
browser if you want to tweak it.

Behavior:

- **GO button** — starts the test. Disabled while a run is in progress.
- **Ping → download → upload** phases, each ~10 seconds (matching the CLI
  defaults).
- **Auto-scaling gauge** — the maximum doubles when the live number gets
  close to the ceiling, so a 50 Mbps line and a 10 Gbps line both look
  meaningful.
- **Connection panel** — IP, ISP, location, server, all from `/api/info`.

The UI talks to the same origin it's served from. If you embed it elsewhere
(serving the speed-test endpoints from a different host), the server already
sends `Access-Control-Allow-Origin: *`.

## Privacy and external services

gospeedtest is **self-hosted by default**: speed measurements never leave
your server. Two pieces of data flow externally, both server-side and both
best-effort:

### `ipinfo.io` lookup

When a client hits `/api/info`, the server queries
`https://ipinfo.io/<client_ip>/json` (with a 2-second timeout) to enrich the
response with ISP and location. The lookup is **skipped automatically** for:

- Loopback addresses (`127.0.0.0/8`, `::1`)
- RFC 1918 private ranges (`10/8`, `172.16/12`, `192.168/16`)
- Link-local (`169.254/16`)
- The unspecified address (`0.0.0.0`)

So a server bound to localhost or used inside a LAN never makes outbound
calls. For a public-facing instance, lookups go to ipinfo.io. ipinfo's free
tier has rate limits per source IP (the server's IP, *not* the client's), so
busy public instances should provide a token via `--ipinfo-token`.

If you want to disable the lookup entirely, the cleanest current option is
to block outbound traffic to `ipinfo.io` from the server's network — the
handler already gracefully falls back to returning just `client_ip`,
`server_host`, and `server_time`. A flag for this is a reasonable feature
request.

### What is **not** sent anywhere

- The actual speed-test bytes — random data generated locally on each side
  and discarded.
- Any `/api/info` field besides what comes back from ipinfo.
- The CLI never calls ipinfo directly. The "public IP", "ISP", and
  "location" rows in the TUI are populated from your `gospeedtest server`'s
  `/api/info` endpoint.
- No telemetry, analytics, or "phone home" of any kind.

## Building and contributing

### Requirements

- Go 1.25 or newer (per `go.mod`).
- A POSIX shell for running the test scripts; on Windows use WSL or run the
  Go commands directly.

### Common commands

```sh
go build ./cmd/gospeedtest          # build the binary
go test -race ./...                 # full test suite, race detector on
go vet ./...                        # static checks
go run ./cmd/gospeedtest server     # run server in dev
go run ./cmd/gospeedtest            # run CLI against localhost:8080
```

### Test coverage (current)

- `internal/speedtest`: ~89 %
- `internal/server`:    ~76 %
- `internal/cli`:       ~17 % (helpers; the Bubble Tea render path is
  intentionally not unit-tested)
- `cmd/gospeedtest`: 0 % (thin flag dispatch)

CI runs `go vet`, `go build`, and `go test -race` on Linux, macOS, and
Windows for every push and pull request.

### Sending a PR

1. Fork the repo.
2. Run `go vet ./...` and `go test -race ./...` locally before pushing.
3. Keep changes scoped — the codebase is small enough that a "while I'm in
   there" cleanup is rarely worth the review noise.
4. Open the PR against `main`. CI must pass.

## Project layout

```
cmd/gospeedtest/         entry point; subcommand dispatch + flags
internal/
  speedtest/             core measurement logic (shared by server + CLI)
  server/                HTTP handlers, /api/info, ipinfo lookup
  cli/                   Bubble Tea TUI runner; gateway parsers per-OS
web/
  embed.go               //go:embed wrapper
  assets/                index.html, style.css, app.js (vanilla)
.github/workflows/
  ci.yml                 vet + build + test on linux/mac/windows per push/PR
  release.yml            cross-platform binary build + GitHub Release on tag
```

## Releases and CI

### CI (`ci.yml`)

Triggers on every push to `main` and every pull request. Runs `go vet`,
`go build ./...`, and `go test -race ./...` on `ubuntu-latest`,
`macos-latest`, and `windows-latest`.

### Release (`release.yml`)

Triggered by pushing a `vX.Y.Z` git tag. The workflow:

1. Cross-compiles `cmd/gospeedtest` for six targets:
   `linux/{amd64,arm64}`, `darwin/{amd64,arm64}`, `windows/{amd64,arm64}`.
   Each binary is built with `-trimpath`, `-ldflags "-s -w -X
   main.versionTag=<tag>"`, and `CGO_ENABLED=0`.
2. Packages each binary alongside `README.md` and `LICENSE` into a
   `tar.gz` (Unix) or `.zip` (Windows).
3. Computes SHA-256 checksums for all archives.
4. Publishes a GitHub Release with the archives, the `checksums.txt`, and
   auto-generated release notes.

To cut a release:

```sh
git tag -a v0.1.1 -m "v0.1.1 — bug-fix release"
git push origin v0.1.1
```

…then watch the workflow at the
[Actions tab](https://github.com/pcamminadi/gospeedtest/actions).

## License

MIT — see [LICENSE](LICENSE).

---

**Built with [Claude Code](https://claude.com/claude-code).**
