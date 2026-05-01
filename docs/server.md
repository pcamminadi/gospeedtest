# `gospeedtest server` reference

Run the HTTP server that hosts the web UI and the speed-test endpoints.

```sh
gospeedtest server [--addr :8080] [--ipinfo-token TOKEN]
```

## Flags

| Flag             | Default  | Purpose                                                                 |
| ---------------- | -------- | ----------------------------------------------------------------------- |
| `--addr`         | `:8080`  | Listen address. Use `:0` for an ephemeral port, or `127.0.0.1:8080` to bind only to localhost. |
| `--ipinfo-token` | *(none)* | Optional [ipinfo.io](https://ipinfo.io) token — passed as `?token=…` on lookup. Without one, gospeedtest falls back to ipinfo's free tier (subject to undocumented per-source-IP rate limits). With one, you get a higher quota and more reliable results. See [Privacy](privacy.md). |

## HTTP endpoints

The server registers the protocol endpoints (described in
[How the test works](protocol.md)) plus serves the embedded web UI
(`index.html`, `style.css`, `app.js`, etc.) from `/`.

### `/ws` — WebSocket ping (browser-only)

The browser UI upgrades to a WebSocket on this path. The server then
drives a ping loop using WS Ping/Pong control frames — see
[How the test works](protocol.md) for why this is more accurate than
HTTP-based pinging from a browser. The CLI does not use this endpoint;
it pings via plain HTTP `/ping` because the Go HTTP client has no IPC
overhead.

The connection is bounded to 30 s of total lifetime so a misbehaving
client cannot pin a server goroutine indefinitely.

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
queries [ipinfo.io](https://ipinfo.io) with a 2-second timeout. On error
or timeout, those fields are omitted. Private and loopback IPs are
skipped — see [Privacy](privacy.md).

## Running behind a reverse proxy

`/api/info` honors the standard proxy headers when computing
`client_ip`:

1. The first entry of `X-Forwarded-For`, if present.
2. Otherwise, `X-Real-IP`.
3. Otherwise, the host portion of `r.RemoteAddr`.

So `nginx` / `Caddy` / `Traefik` reverse-proxy setups Just Work as long
as your proxy sets one of those headers.

The server intentionally has **no `WriteTimeout`** — long downloads
must not be cut off mid-stream — and a 60-second `ReadTimeout` to bound
slow uploads.

!!! example "nginx snippet"
    ```nginx
    location / {
        proxy_pass         http://127.0.0.1:8080;
        proxy_set_header   X-Real-IP        $remote_addr;
        proxy_set_header   X-Forwarded-For  $proxy_add_x_forwarded_for;
        proxy_buffering    off;   # keep download streams flowing
        proxy_request_buffering off; # keep upload streams flowing
    }
    ```

## Safety caps

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

## CORS and cache control

The server sets:

- `Access-Control-Allow-Origin: *` on every response. The speed test
  isn't authenticated; the UI is served from the same origin in normal
  use.
- `Cache-Control: no-store, no-cache, must-revalidate` on `/ping`,
  `/download`, `/upload`, and `/api/info` so intermediate proxies can't
  serve stale data and falsely inflate measurements.
- Static UI assets (`/`, `index.html`, `style.css`, `app.js`) **are**
  cacheable — they don't affect measurement accuracy.

`OPTIONS` requests return `204 No Content` for CORS preflight.
