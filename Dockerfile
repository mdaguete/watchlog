# Go builder stage - always runs on the build host's native platform
FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.26-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Cache dependencies layer (runs natively, no emulation)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Cross-compile natively using Go's built-in support (no QEMU needed)
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s" \
    -trimpath \
    -o watchlog ./cmd/server/

# Create data directory for the final image
RUN mkdir -p /data && chown 65532:65532 /data

# Final stage - using distroless for minimal attack surface
FROM gcr.io/distroless/static-debian12:nonroot

# Copy binary and templates
COPY --from=builder /app/watchlog /usr/local/bin/
COPY --from=builder /app/web /usr/local/share/watchlog/web
COPY --from=builder --chown=nonroot:nonroot /data /data

# Container metadata following OCI standards
LABEL org.opencontainers.image.title="WatchLog"
LABEL org.opencontainers.image.description="Personal TV show and movie tracking app"
LABEL org.opencontainers.image.source="https://github.com/mdaguete/watchlog"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.vendor="mdaguete"

# Configuration
EXPOSE 8080
ENV TMDB_API_KEY=""

# Data volume for SQLite database
VOLUME /data

WORKDIR /usr/local/share/watchlog

# Use unprivileged user (provided by distroless)
USER nonroot

ENTRYPOINT ["/usr/local/bin/watchlog", "-datadir", "/data"]
