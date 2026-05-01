# `gospeedtest server` reference

Run the HTTP server that hosts the web UI and the speed-test endpoints.

```sh
gospeedtest server [--addr :8080] [--ipinfo-token TOKEN]
```

## Flags

| Flag                    | Default  | Purpose                                                                 |
| ----------------------- | -------- | ----------------------------------------------------------------------- |
| `--addr`                | `:8080`  | Listen address. Use `:0` for an ephemeral port, or `127.0.0.1:8080` to bind only to localhost. |
| `--ipinfo-token`        | *(none)* | Optional [ipinfo.io](https://ipinfo.io) token — passed as `?token=…` on lookup. Without one, gospeedtest falls back to ipinfo's free tier (subject to undocumented per-source-IP rate limits). With one, you get a higher quota and more reliable results. See [Privacy](privacy.md). |
| `--trust-proxy-headers` | `false`  | When set, the server reads the client IP from `X-Forwarded-For` (first entry) or `X-Real-IP` instead of the TCP peer address. **Off by default** — see the warning below. |

!!! danger "Don't enable `--trust-proxy-headers` on a directly-exposed server"
    Without a sanitizing reverse proxy in front, any caller can send
    `X-Forwarded-For: <any.public.ip>` and force the server to issue an
    outbound `ipinfo.io` lookup for an IP of their choosing — draining
    your ipinfo quota and turning `/api/info` into an SSRF-style oracle
    that returns the lookup result. Only enable this flag when the
    server sits behind an nginx / Caddy / Traefik / cloud-LB instance
    that strips and re-sets these headers itself.

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

By default the server reads the client IP from the TCP peer address
(`r.RemoteAddr`) — proxy headers are *ignored* so that a malicious
direct caller can't spoof their IP. To honor them, run with
`--trust-proxy-headers`. With the flag set, `/api/info` resolves
`client_ip` in this order:

1. The first entry of `X-Forwarded-For`, if present.
2. Otherwise, `X-Real-IP`.
3. Otherwise, the host portion of `r.RemoteAddr`.

So `nginx` / `Caddy` / `Traefik` reverse-proxy setups Just Work — as
long as your proxy strips and re-sets these headers itself.

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

## Hardening posture and public-internet exposure

This binary intentionally serves **bandwidth-heavy, unauthenticated
endpoints with open CORS** — that's what a speed-test server *is*.
Read this section before exposing an instance to the public internet.

What's already in place:

- `ReadHeaderTimeout: 5s` — bounds slowloris-style header-dribbling.
- `ReadTimeout: 60s` — bounds slow uploads.
- `IdleTimeout: 60s` — bounds idle keep-alive connections.
- `MaxDownloadMiB` / `MaxUploadMiB` — per-request byte caps (see
  below). Each *individual* request can't stream forever, but the
  server has **no global concurrency cap and no per-IP rate limit**.

What this binary **does not** do, and why it matters on the open
internet:

- **No authentication.** Anyone who can reach `:8080` can run a test.
  At ~1 GiB per `/download?bytes=` and 4 parallel browser streams, a
  single visitor draws ~4 GiB of egress per test. A modest pool of
  abusers can saturate your uplink.
- **No global rate limiting.** A reverse proxy (nginx/Caddy/Traefik)
  with `limit_req` / a token bucket is the recommended fix.
- **`X-Forwarded-For` is not trusted by default.** See
  [`--trust-proxy-headers`](#flags) — only enable it when behind a
  proxy that strips and re-sets the header itself, otherwise any
  caller can spoof their IP into your `/api/info` response and your
  ipinfo.io quota.

If you're running this on a homelab / LAN: ignore all of the above.
The defaults are fine.

If you're publishing it on the internet: put it behind a reverse
proxy with rate limits and (ideally) a WAF, or restrict access by
firewall to known networks.

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
