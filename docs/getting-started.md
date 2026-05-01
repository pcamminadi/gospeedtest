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
