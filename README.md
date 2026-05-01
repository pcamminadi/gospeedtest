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

## Features

- **Browser UI** at `/` — animated SVG gauge, live ping/download/upload, ISP
  and IP info, all served from the embedded assets (no Node, no build step).
- **CLI mode** with a Bubble Tea TUI — animated ASCII gauge, live numbers,
  works over SSH. Falls back to plain text with `--json`.
- **Single Go binary**, no runtime dependencies.
- **HTTP-based protocol** (LibreSpeed-style): parallel `GET /download` and
  `POST /upload` streams plus a `/ping` endpoint.
- **MIT licensed**.

## Install

Pre-built binaries for Linux, macOS, and Windows (amd64 + arm64) are produced
on every tagged release — grab the latest from
[Releases](https://github.com/pcamminadi/gospeedtest/releases/latest).

Or install with Go:

```sh
go install github.com/pcamminadi/gospeedtest/cmd/gospeedtest@latest
```

…or build from a checkout:

```sh
git clone https://github.com/pcamminadi/gospeedtest
cd gospeedtest
go build ./cmd/gospeedtest
```

## Run the server

```sh
./gospeedtest server --addr :8080
```

Then open <http://localhost:8080> in a browser.

## Run the CLI

The CLI is the default mode — running the binary with no subcommand is the
same as running `cli`:

```sh
./gospeedtest                                       # default ⇒ CLI
./gospeedtest --server http://my-host:8080          # against a remote server
./gospeedtest --server http://localhost:8080 --json # machine-readable output
./gospeedtest cli --server http://localhost:8080    # explicit form, identical
```

Useful flags: `--duration 10s`, `--streams 4`, `--json`.

## Releases

CI runs `vet`, `build`, and `go test -race` on Linux, macOS, and Windows for
every push and PR.

Tagging `vX.Y.Z` triggers the release workflow, which cross-compiles the
binary for `linux/{amd64,arm64}`, `darwin/{amd64,arm64}`, and
`windows/{amd64,arm64}`, packages each into a tarball or zip alongside
`README.md` and `LICENSE`, computes SHA-256 checksums, and publishes them all
to a GitHub Release with auto-generated notes.

## Layout

```
cmd/gospeedtest/   entry point + subcommand dispatch
internal/
  speedtest/       core measurement logic (shared)
  server/          HTTP handlers + embedded UI
  cli/             Bubble Tea TUI runner
web/               index.html, app.js, style.css (embedded via go:embed)
```

## License

MIT — see [LICENSE](LICENSE).
