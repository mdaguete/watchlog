# Go builder stage
FROM docker.io/library/golang:1.26-alpine AS builder

WORKDIR /app

# Cache dependencies layer
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build with optimizations
RUN CGO_ENABLED=0 go build \
    -ldflags="-w -s" \
    -trimpath \
    -o watchlog ./cmd/server/

# Final stage - using distroless for minimal attack surface
FROM gcr.io/distroless/static-debian12:nonroot

# Copy binary and templates
COPY --from=builder /app/watchlog /usr/local/bin/
COPY --from=builder /app/web /usr/local/share/watchlog/web

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

ENTRYPOINT ["/usr/local/bin/watchlog", "-db", "/data/watchlog.db"]
