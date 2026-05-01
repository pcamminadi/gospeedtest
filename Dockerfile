# syntax=docker/dockerfile:1
#
# Multi-stage build:
#  - "build" stage produces a static, stripped Go binary
#  - final image is `scratch` so the resulting container is the binary
#    plus a CA bundle for the outbound ipinfo.io lookup. Total image
#    size ends up roughly the size of the binary itself.
#
# Build:   docker build -t gospeedtest .
# Run:     docker run --rm -p 8080:8080 gospeedtest
# CLI:     docker run --rm gospeedtest cli --server http://your-host:8080

ARG GO_VERSION=1.25

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# Cache module downloads in their own layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO disabled + -trimpath + -s -w match the release builds.
# The binary is statically linked, so the final image needs no libc.
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=docker
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
      -trimpath \
      -ldflags "-s -w -X main.versionTag=${VERSION}" \
      -o /out/gospeedtest \
      ./cmd/gospeedtest

# The final image bundles the CA roots from alpine so /api/info's
# outbound HTTPS call to ipinfo.io can verify the certificate.
FROM alpine:3.21 AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
COPY --from=certs  /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build  /out/gospeedtest /gospeedtest

EXPOSE 8080
ENTRYPOINT ["/gospeedtest"]
CMD ["server", "--addr", ":8080"]
