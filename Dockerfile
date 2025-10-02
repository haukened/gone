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

# Install and run minify pipeline. Use GOBIN so binary lands in a known directory, then invoke absolute path.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOBIN=/usr/local/bin go install github.com/tdewolff/minify/v2/cmd/minify@latest && \
    mkdir -p web/dist/css web/dist/js && \
    cp web/*.tmpl.html web/dist/ || true && \
    /usr/local/bin/minify -r -o web/dist/css/ web/css/ && \
    /usr/local/bin/minify -r -o web/dist/js/ web/js/

# Build (attempt fully static) linked binary with CGO for sqlite. Using external link mode and static flags.
RUN mkdir -p bin
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=1
RUN --mount=type=cache,target=/root/.cache/go-build \
        --mount=type=cache,target=/go/pkg/mod \
        GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
        go build -trimpath -tags='prod sqlite_omit_load_extension netgo osusergo' \
            -ldflags='-s -w -linkmode external -extldflags "-static"' \
            -o ./bin/gone ./cmd/gone || \
        (echo 'Falling back to dynamic link (static link failed)'; \
         go build -trimpath -tags='prod sqlite_omit_load_extension netgo osusergo' -ldflags='-s -w' -o ./bin/gone ./cmd/gone)

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