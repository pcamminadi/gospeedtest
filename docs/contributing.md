# Contributing

## Requirements

- Go 1.25 or newer (per `go.mod`).
- A POSIX shell for running the test scripts; on Windows use WSL or run
  the Go commands directly.

## Common commands

```sh
go build ./cmd/gospeedtest          # build the binary
go test -race ./...                 # full test suite, race detector on
go vet ./...                        # static checks
go run ./cmd/gospeedtest server     # run server in dev
go run ./cmd/gospeedtest            # run CLI against localhost:8080
```

## Test coverage (current)

| Package                | Coverage |
| ---------------------- | -------- |
| `internal/speedtest`   | ~89 %    |
| `internal/server`      | ~76 %    |
| `internal/cli`         | ~17 % (helpers; the Bubble Tea render path is intentionally not unit-tested) |
| `cmd/gospeedtest`      | 0 % (thin flag dispatch) |

CI runs `go vet`, `go build`, and `go test -race` on Linux, macOS, and
Windows for every push and pull request.

## Sending a PR

1. Fork the repo.
2. Run `go vet ./...` and `go test -race ./...` locally before pushing.
3. Keep changes scoped — the codebase is small enough that a "while
   I'm in there" cleanup is rarely worth the review noise.
4. Open the PR against `main`. CI must pass.

## Working on the docs

The docs site you're reading is built with
[MkDocs](https://www.mkdocs.org/) +
[Material for MkDocs](https://squidfunk.github.io/mkdocs-material/) and
deployed to GitHub Pages by `.github/workflows/docs.yml`.

To preview your changes locally:

```sh
pip install mkdocs-material
mkdocs serve
# open http://127.0.0.1:8000
```

Markdown lives under `docs/`, configuration is `mkdocs.yml`. Any push
to `main` rebuilds and redeploys the site.
