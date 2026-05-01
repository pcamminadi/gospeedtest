# Releases & CI

## CI (`ci.yml`)

Triggers on every push to `main` and every pull request. Runs
`go vet`, `go build ./...`, and `go test -race ./...` on
`ubuntu-latest`, `macos-latest`, and `windows-latest`.

## Release (`release.yml`)

Triggered by pushing a `vX.Y.Z` git tag. The workflow:

1. **Cross-compiles** `cmd/gospeedtest` for six targets:
   `linux/{amd64,arm64}`, `darwin/{amd64,arm64}`,
   `windows/{amd64,arm64}`. Each binary is built with `-trimpath`,
   `-ldflags "-s -w -X main.versionTag=<tag>"`, and `CGO_ENABLED=0`.
2. **Packages** each binary alongside `README.md` and `LICENSE` into a
   `tar.gz` (Unix) or `.zip` (Windows).
3. **Computes SHA-256 checksums** for all archives.
4. **Publishes** a GitHub Release with the archives, the
   `checksums.txt`, and auto-generated release notes.

To cut a release:

```sh
git tag -a v0.1.1 -m "v0.1.1 — bug-fix release"
git push origin v0.1.1
```

…then watch the workflow at the
[Actions tab](https://github.com/pcamminadi/gospeedtest/actions).

## Docs (`docs.yml`)

Triggered on every push to `main`. Builds the MkDocs site (this site)
and publishes it to GitHub Pages.

If the workflow fails on the very first run, it's almost always because
GitHub Pages isn't yet enabled for the repo. Go to
**Settings → Pages**, set the source to **GitHub Actions**, and re-run
the workflow.
