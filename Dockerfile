# syntax=docker/dockerfile:1.7

# Base images are Docker Hardened Images (https://dhi.io), pinned by digest.
# Dependabot (docker ecosystem) keeps the tags and digests current.
# Pulling from dhi.io requires `docker login dhi.io` with Docker Hub credentials.

# ----------- Builder Stage -----------
# Build on the native platform and cross-compile; CGO is disabled so no target toolchain is needed.
FROM --platform=$BUILDPLATFORM dhi.io/golang:1.27-debian13-dev@sha256:b2c53244752dcb35fea192b1e7015635696c8456130e5b82bb0306f89e1144eb AS builder

# The image sets GOTOOLCHAIN=local, so it can only build with its own Go. When
# go.mod needs a newer patch release than the image has yet (for a security
# fix), "auto" fetches exactly that toolchain from proxy.golang.org, checked
# against sum.golang.org. Once the image catches up, its own Go is used.
ENV GOTOOLCHAIN=auto

ARG MINIFY_VERSION=v2.24.17

WORKDIR /app

# Leverage build cache for deps
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Install minify (version pinned) for the production asset pipeline.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOBIN=/usr/local/bin go install github.com/tdewolff/minify/v2/cmd/minify@${MINIFY_VERSION}

# Copy source
COPY . .

# Minify assets into web/dist (embedded via -tags=prod)
RUN mkdir -p web/dist/css web/dist/js web/dist/fonts web/dist/img && \
    cp web/*.html web/dist/ && \
    cp web/fonts/* web/dist/fonts/ && \
    cp web/img/* web/dist/img/ && \
    minify -r -o web/dist/css/ web/css/ && \
    minify -r -o web/dist/js/ web/js/

# Build a fully static, reproducible binary. SQLite is provided by the pure-Go modernc.org/sqlite driver.
# VERSION is the release tag shown in the page footer; local builds say "dev".
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -tags=prod -ldflags="-s -w -buildid= -X main.version=${VERSION}" -o /app/bin/goned ./cmd/goned

# The runtime image has no shell, so create the data dir here and copy it across.
RUN mkdir -p /app/data

# ----------- Final Stage -----------
# Distroless static runtime: CA certs, tzdata and a nonroot (65532) user only. No shell or package manager.
FROM dhi.io/static:20250419-debian13@sha256:98ef7a853608577e8d66dad1d25ada75d745d782f28d84e9ecfb85dfeb1f9c98

COPY --from=builder --chown=65532:65532 /app/data /data
COPY --from=builder --chown=0:0 --chmod=0555 /app/bin/goned /usr/local/bin/goned

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
USER 65532:65532

# The image has no shell or curl, so goned probes its own /readyz.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/goned", "healthcheck"]

ENTRYPOINT ["/usr/local/bin/goned"]
