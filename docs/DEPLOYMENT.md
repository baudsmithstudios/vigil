# Raspberry Pi Deployment Guide

> **This is a developer reference.** It covers cross-compilation, buildx workflows, and Pi-specific troubleshooting. If you are setting up Vigil for the first time, the [README Quick Start](../README.md#quick-start) is the right place to begin.

## Prerequisites

- Raspberry Pi (tested on Pi 4) running a 64-bit OS (Raspberry Pi OS Lite 64-bit recommended)
- Docker and Docker Compose installed on the Pi
- An external USB drive

## Building

### Option A — Build on the Pi

```sh
git clone https://github.com/baudsmithstudios/vigil.git
cd vigil
docker build -t vigil:latest --load .
./vigil.sh init
docker compose up -d
```

This builds natively on the Pi — slow on older models but straightforward.

### Option B — Cross-compile on a dev machine

Requires Docker with `buildx` and the `linux/arm64` builder available.

```sh
# One-time: create a multi-platform builder
docker buildx create --use --name pibuilder

# Build for ARM64 and export the image as a tar (~20MB)
docker buildx build --platform linux/arm64 -t vigil:latest \
  --output type=docker,dest=vigil.tar .

# Copy the image and supporting files to the Pi
scp vigil.tar docker-compose.yml vigil.sh user@<pi-ip>:~/vigil/

# On the Pi: load the image, generate config, and start
cd ~/vigil
docker load < vigil.tar
./vigil.sh init
docker compose up -d
```

### Redeploying after changes (Option B)

```sh
docker buildx build --platform linux/arm64 -t vigil:latest \
  --output type=docker,dest=vigil.tar .

scp vigil.tar user@<pi-ip>:~/vigil/ && \
  ssh user@<pi-ip> "cd ~/vigil && docker compose down && docker load < vigil.tar && docker compose up -d"
```

## External Drive Setup

The database should live on the external drive, not the SD card.

Find the mount point:

```sh
lsblk -o NAME,MOUNTPOINT,FSTYPE,SIZE,LABEL
```

Create the data directory:

```sh
mkdir -p /your/mount/point/vigil
chown 1000:1000 /your/mount/point/vigil
```

Update the volume in `docker-compose.yml`:

```yaml
volumes:
  - /your/mount/point/vigil:/data
```

## Viewing the Dashboard

The container runs in TUI mode by default. When started with `docker compose up -d` it runs detached in the background — attach to open the dashboard:

```sh
docker attach vigil
```

Detach without stopping: `Ctrl+P` then `Ctrl+Q`.

## Headless Mode

For background metric collection with no terminal UI, add to `docker-compose.yml`:

```yaml
command: ["--headless", "--config", "/app/config.toml"]
```

## Single-Shot JSON

For scripts, cron snapshots, or external monitoring integrations, run one collection tick and print JSON:

```sh
docker compose run --rm vigil --once --json
```

This uses the same host mounts and network settings as the daemon service, but exits after one snapshot. It does not write SQLite data or start the TUI/background collection loop.

## Troubleshooting

### Verifying host metrics

After starting the container, confirm it sees host processes and interfaces:

```sh
# Should show all host processes, not just the container's
docker exec vigil cat /proc/1/comm

# Should show host network interfaces (eth0, wlan0, etc.)
docker exec vigil cat /proc/net/dev
```

## Health Checklist

- [ ] External drive mounted and `vigil/` directory created with uid 1000 ownership
- [ ] `docker-compose.yml` volume path updated to actual mount point
- [ ] Video group GID verified for Pi throttle detection (`getent group video | cut -d: -f3`)
- [ ] cgroup memory accounting enabled if needed (optional, see README)
- [ ] `docker compose up` shows TUI with all sections populated
- [ ] `docker compose up -d` runs cleanly in background
- [ ] DB file appears on external drive after ~5 minutes
- [ ] `docker attach vigil` opens the dashboard from a detached session
- [ ] `docker compose logs vigil` shows no errors
