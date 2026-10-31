FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o vigil ./cmd/vigil


# Pinned for reproducible builds.
FROM alpine:3.21

# vcgencmd reads Pi throttle state. The package only exists on ARM, so other
# architectures skip the install.
RUN apk add --no-cache raspberrypi-utils 2>/dev/null || true

# Some /sys reads may still need the container in a host group.
RUN adduser -D -u 1000 vigil

WORKDIR /app
COPY --from=builder /build/vigil .

# config.toml is bind-mounted at runtime so webhook URLs stay out of image layers.
# /data holds the SQLite DB; bind-mount it to your external drive.
RUN mkdir /data && chown vigil:vigil /data

USER vigil

ENTRYPOINT ["/app/vigil"]
CMD ["--config", "/app/config.toml"]
