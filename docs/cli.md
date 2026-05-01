# `gospeedtest cli` reference

Run the speed test from a terminal.

```sh
gospeedtest                                       # default mode
gospeedtest [--server URL] [--json] ...           # default mode with flags
gospeedtest cli    [--server URL] ...             # explicit form, identical
```

The CLI is the **default subcommand** — invoking the binary with no
argument (or with arguments that don't start with
`server` / `cli` / `version` / `help`) falls through to CLI mode against
`http://localhost:8080`.

## Flags

| Flag         | Default                   | Purpose                                                  |
| ------------ | ------------------------- | -------------------------------------------------------- |
| `--server`   | `http://localhost:8080`   | URL of a `gospeedtest server` instance.                  |
| `--json`     | `false`                   | Emit a single JSON object summarizing the run, then exit. Useful for scripting or non-tty environments. |
| `--duration` | `10s`                     | Per-phase duration (download and upload). Accepts any `time.Duration`. |
| `--streams`  | `4`                       | Number of parallel HTTP streams used for download and upload. |

## Keybindings (TUI mode)

| Key              | Action       |
| ---------------- | ------------ |
| `q`              | Quit         |
| `Esc`            | Quit         |
| `Ctrl+C`         | Quit         |

The TUI runs **inline** (not in an alt-screen), so the final frame stays
in your scrollback after the program exits — you can review the results.

## JSON output schema

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

!!! note "Units and conventions"
    - `duration` is **nanoseconds** (Go's default `time.Duration` JSON
      encoding).
    - `*_mbps` fields are **megabits per second** (1 Mb = 1 000 000 bits).
    - `bytes_*` are total bytes transferred during the corresponding
      phase, including ramp-up.
    - `started_at` is RFC 3339 with the local timezone.

## Network info shown in the TUI

The CLI panel shows:

- **public IP** — from the server's `/api/info`.
- **ISP**, **location** — best-effort from ipinfo.io (server-side
  lookup).
- **server** — the server's `os.Hostname()`.
- **local IP** — the local source IP that would be used to reach the
  internet (computed via a UDP "dial" to `8.8.8.8:53` — no packet is
  sent; the kernel just consults the route table).
- **gateway** — the system default route, parsed from
  `route -n get default` on macOS / BSD, `ip route show default` (with
  `route -n` fallback) on Linux, and `route print 0.0.0.0` on Windows.

`local IP` and `gateway` are detected **client-side** and never leave
your machine.
