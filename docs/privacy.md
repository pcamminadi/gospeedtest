# Privacy and external services

gospeedtest is **self-hosted by default**: speed measurements never
leave your server. Two pieces of data flow externally, both server-side
and both best-effort.

## `ipinfo.io` lookup

When a client hits `/api/info`, the server queries
`https://ipinfo.io/<client_ip>/json` (with a 2-second timeout) to enrich
the response with ISP and location. The lookup is **skipped
automatically** for:

- Loopback addresses (`127.0.0.0/8`, `::1`)
- RFC 1918 private ranges (`10/8`, `172.16/12`, `192.168/16`)
- Link-local (`169.254/16`)
- The unspecified address (`0.0.0.0`)

So a server bound to localhost or used inside a LAN never makes
outbound calls. For a public-facing instance, lookups go to ipinfo.io.

!!! warning "Rate limits on the free tier"
    ipinfo's free tier has rate limits per source IP (the **server's**
    IP, *not* the client's), so busy public instances should provide a
    token via `--ipinfo-token`. ipinfo no longer publishes the exact
    free-tier number; historically it has been ~1 000 requests/day
    unauthenticated.

### Disabling the lookup

The cleanest current option is to **block outbound traffic to
`ipinfo.io`** from the server's network — the handler already
gracefully falls back to returning just `client_ip`, `server_host`,
and `server_time`. A `--no-ip-lookup` flag is a reasonable feature
request; please open an issue if you want one.

## What is **not** sent anywhere

- The actual speed-test bytes — random data generated locally on each
  side and discarded.
- Any `/api/info` field besides what comes back from ipinfo.
- The CLI never calls ipinfo directly. The "public IP", "ISP", and
  "location" rows in the TUI are populated from your `gospeedtest
  server`'s `/api/info` endpoint.
- No telemetry, analytics, or "phone home" of any kind.
