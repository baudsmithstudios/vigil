# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o vigil ./cmd/vigil


# Runtime stage — pinned version for reproducible builds.
FROM alpine:3.21

# vcgencmd is used to read Pi throttle/voltage state.
# The package only exists on ARM — the install is skipped on other architectures.
RUN apk add --no-cache raspberrypi-utils 2>/dev/null || true

# Non-root user for running the process.
# Some /sys reads (temperature, throttle) may still need group permissions —
# add the container to the appropriate host group if needed.
RUN adduser -D -u 1000 vigil

WORKDIR /app
COPY --from=builder /build/vigil .

# Config is bind-mounted at runtime, not baked into the image.
# This prevents secrets (webhook URLs) from leaking into image layers.
# Mount your config.toml to /app/config.toml via docker-compose volumes.

# /data is where the SQLite DB lives — bind-mount this to your external drive.
RUN mkdir /data && chown vigil:vigil /data

USER vigil

ENTRYPOINT ["/app/vigil"]
CMD ["--config", "/app/config.toml"]
