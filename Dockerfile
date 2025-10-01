# syntax=docker/dockerfile:1.7

# ----------- Builder Stage -----------
FROM cgr.dev/chainguard/go:latest AS builder

WORKDIR /app

# Leverage build cache for deps
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source
COPY . .

# Install minify (v2 path) and prepare/minify static assets (HTML copied like task prod)
RUN --mount=type=cache,target=/go/pkg/mod \
    go install github.com/tdewolff/minify/v2/cmd/minify@latest && \
    mkdir -p web/dist/css web/dist/js && \
    cp web/*.html web/dist/ && \
    minify -r -o web/dist/css/ web/css/ && \
    minify -r -o web/dist/js/ web/js/

# build static linked binary
RUN mkdir -p bin
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -tags=prod -o ./bin/gone ./cmd/gone

# Runtime doesn't have mkdir, so create data dir at build time
# and copy it to final image.
RUN mkdir -p /app/data

# ----------- Final Stage -----------
FROM cgr.dev/chainguard/static:latest

# copy the data dir from builder
COPY --from=builder --chown=nonroot:nonroot /app/data /data

# copy the binary from builder
COPY --from=builder --chown=nonroot:nonroot /app/bin/gone /usr/local/bin/gone

# OCI Labels
LABEL org.opencontainers.image.title="Gone" \
      org.opencontainers.image.description="A secure, encrypted, self-destructing pastebin service" \
      org.opencontainers.image.url="https://github.com/haukened/gone" \
      org.opencontainers.image.source="https://github.com/haukened/gone" \
      org.opencontainers.image.licenses="AGPL-3.0"

# Expose doesn't do anything anymore, but it's documentation for users
EXPOSE 8080
EXPOSE 9090

# Allow mounting /data at runtime
VOLUME ["/data"]

# Run as non-root user
USER nonroot:nonroot

# Entrypoint
ENTRYPOINT ["/usr/local/bin/gone"]