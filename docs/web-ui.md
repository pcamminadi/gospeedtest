# Web UI

Served at `/` from the embedded `web/assets/` bundle. It's vanilla
HTML + CSS + JavaScript — no build step, no framework. Inspect the source
from your browser if you want to tweak it.

## Behavior

- **GO button** — starts the test. Disabled while a run is in progress.
- **Ping → download → upload** phases, each ~10 seconds (matching the
  CLI defaults).
- **Auto-scaling gauge** — the maximum doubles when the live number gets
  close to the ceiling, so a 50 Mbps line and a 10 Gbps line both look
  meaningful.
- **Connection panel** — IP, ISP, location, server, all from
  `/api/info`.

The UI talks to the same origin it's served from. If you embed it
elsewhere (serving the speed-test endpoints from a different host),
the server already sends `Access-Control-Allow-Origin: *`.

## Customizing

The whole UI is three files in `web/assets/`:

- `index.html` — markup
- `style.css` — colors, gradients, glow filters, tick marks
- `app.js` — the test client (parallel `fetch` for download, multipart
  POSTs for upload, `requestAnimationFrame`-driven gauge)

Edits there are picked up next time you run `go build` — the assets are
embedded via [`//go:embed`](https://pkg.go.dev/embed) at compile time.
