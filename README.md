# gospeedtest

[![CI](https://github.com/pcamminadi/gospeedtest/actions/workflows/ci.yml/badge.svg)](https://github.com/pcamminadi/gospeedtest/actions/workflows/ci.yml)
[![Docs](https://github.com/pcamminadi/gospeedtest/actions/workflows/docs.yml/badge.svg)](https://pcamminadi.github.io/gospeedtest/)
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

📚 **Full documentation:** <https://pcamminadi.github.io/gospeedtest/>

## Features

- **Browser UI** at `/` — animated SVG gauge, live ping / download / upload,
  ISP and IP info, served from assets embedded in the binary.
- **CLI mode** with a Bubble Tea TUI — animated gauge, live numbers, works
  over SSH. `--json` for machine-readable output.
- **Default-mode CLI**: running the bare binary launches the CLI against
  `http://localhost:8080`.
- **Single Go binary**, no runtime deps; cross-compiled to linux / darwin /
  windows × amd64 / arm64.
- **HTTP-based protocol** (LibreSpeed-style) — works behind any reverse proxy.
- **MIT licensed**.

## Quick start

Pre-built binaries for every release at
[Releases](https://github.com/pcamminadi/gospeedtest/releases/latest), or:

```sh
go install github.com/pcamminadi/gospeedtest/cmd/gospeedtest@latest
```

Then:

```sh
gospeedtest server                     # listens on :8080
gospeedtest                            # run a test against localhost:8080
gospeedtest --server http://my-host    # …or against a remote server
gospeedtest --json                     # machine-readable output
```

That's the whole interface. For the full reference — every flag, the wire
protocol, reverse-proxy notes, the JSON schema, the privacy model, the CI &
release pipelines — see the **[documentation site](https://pcamminadi.github.io/gospeedtest/)**.

## Project layout

```
cmd/gospeedtest/   entry point + subcommand dispatch
internal/
  speedtest/       core measurement logic (shared)
  server/          HTTP handlers + embedded UI
  cli/             Bubble Tea TUI runner
web/               index.html, app.js, style.css (embedded via go:embed)
docs/              MkDocs source for the documentation site
```

## License

MIT — see [LICENSE](LICENSE).

---

**Built with [Claude Code](https://claude.com/claude-code).**
