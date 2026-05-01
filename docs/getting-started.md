# Getting started

## Install

### Pre-built binaries (recommended)

Pre-built binaries for Linux, macOS, and Windows (amd64 + arm64) are
produced on every tagged release. Grab the latest from the
[Releases page](https://github.com/pcamminadi/gospeedtest/releases/latest).
Each release also publishes a `checksums.txt` (SHA-256) so you can verify
the download.

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

### Docker

Pre-built multi-arch images (`linux/amd64` + `linux/arm64`) are
published to **GitHub Container Registry** by
`.github/workflows/docker.yml` on every push to `main` and on every
release tag.

| Tag | What it points at |
| --- | --- |
| `:latest`    | the highest semver release (e.g. `v0.2.0`) |
| `:0.2.0`     | a specific release |
| `:0.2`       | the most recent patch release on the 0.2 line |
| `:edge`      | the latest commit on `main` |
| `:sha-<n>`   | a specific commit |

```sh
# Run the latest release
docker run --rm -p 8080:8080 ghcr.io/pcamminadi/gospeedtest:latest

# Pin a specific version (recommended for production)
docker run --rm -p 8080:8080 ghcr.io/pcamminadi/gospeedtest:0.2.0

# Track main
docker run --rm -p 8080:8080 ghcr.io/pcamminadi/gospeedtest:edge

# Pass flags through after the image name
docker run --rm -p 9000:9000 ghcr.io/pcamminadi/gospeedtest:latest \
  server --addr :9000 --trust-proxy-headers

# Run the CLI in a one-off container
docker run --rm ghcr.io/pcamminadi/gospeedtest:latest \
  cli --server http://host.docker.internal:8080 --json
```

…or build it yourself from a checkout:

```sh
docker build -t gospeedtest .
docker run --rm -p 8080:8080 gospeedtest
```

The image is **~8 MB** with a `scratch` final stage; it bundles
`ca-certificates` so the server's `/api/info` outbound HTTPS lookup
to ipinfo.io can verify the certificate. The binary itself is built
with `CGO_ENABLED=0` and the same `-trimpath -ldflags "-s -w"` as the
release tarballs — statically linked, no runtime dependencies.

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

That's it. Continue to [`gospeedtest server`](server.md) or
[`gospeedtest cli`](cli.md) for the full reference.
