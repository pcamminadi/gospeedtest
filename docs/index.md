---
title: Home
---

# gospeedtest

An open-source, self-hosted internet speed test, written in Go. Inspired by
speedtest.net — keeps the fancy gauge in the browser and adds a rich
terminal UI for headless boxes.

A single binary serves both the speed-test endpoints and the web UI, and
the same binary ships a CLI mode that runs the test from a terminal.

[Get started :material-arrow-right:](getting-started.md){ .md-button .md-button--primary }
[View on GitHub :material-github:](https://github.com/pcamminadi/gospeedtest){ .md-button }

## Features

- **Browser UI** at `/` — animated SVG gauge, live ping / download /
  upload, ISP and IP info, served from assets embedded in the binary.
  No Node, no build step.
- **CLI mode** with a [Bubble Tea](https://github.com/charmbracelet/bubbletea)
  TUI — animated gauge, live numbers, works over SSH. `--json` for
  machine-readable output.
- **Default-mode CLI**: running the bare binary (no subcommand) launches
  the CLI against `http://localhost:8080`.
- **Single Go binary**, no runtime dependencies. Cross-compiled to
  linux / darwin / windows × amd64 / arm64 by CI.
- **HTTP-based protocol** (LibreSpeed-style): parallel `GET /download`
  and `POST /upload` streams plus a `/ping` endpoint. No proprietary
  protocol — any reverse proxy / load balancer works without special
  handling.
- **MIT licensed**.

## Where to next

<div class="grid cards" markdown>

-   :material-rocket-launch: **[Getting started](getting-started.md)**

    Install, run the server, run a test in 30 seconds.

-   :material-server: **[`gospeedtest server`](server.md)**

    Every flag, every endpoint, reverse-proxy notes, safety caps.

-   :material-console: **[`gospeedtest cli`](cli.md)**

    Default-mode dispatch, flags, JSON output schema, keybindings.

-   :material-graph: **[How the test works](protocol.md)**

    Wire protocol, measurement methodology, ramp-up trim, jitter math.

-   :material-shield-lock: **[Privacy](privacy.md)**

    What stays local, what goes to ipinfo.io, when lookups are skipped.

-   :material-source-branch: **[Contributing](contributing.md)**

    Build, test, send a PR.

</div>
