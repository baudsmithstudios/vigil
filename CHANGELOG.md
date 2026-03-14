# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.1.0] - 2026-03-24

### Added

- Live TUI dashboard with CPU gauges, per-core bars, memory, disk, network, load, and temperature panels
- Per-container CPU %, memory, and status via Docker Engine API (opt-in)
- Mount watchdog with debounced alerts and flap detection (opt-in)
- Threshold and rate-of-change alert rules with Discord and generic webhook notifications
- Quiet hours support for alert notifications
- Service health checks: HTTP endpoint status/latency and TCP port reachability (opt-in)
- Persistent storage in SQLite with configurable retention and WAL mode for reliable writes
- Historical browser showing sparklines for CPU, memory, swap, load, temperature, disk I/O, network, and service latency over 1h/6h/12h ranges
- Alert persistence across restarts — active alerts are restored from the database on startup
- Headless mode (`--headless`) for background collection without a terminal
- `vigil init` for interactive config generation with environment detection
- `vigil mounts` to list mountable block devices for config setup
- `--version` flag
- ~20MB ARM64 Docker image; `docker compose up` one-liner deployment
- Hardened container defaults: read-only root filesystem, non-root user, no extra capabilities

[Unreleased]: https://github.com/baudsmithstudios/vigil/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/baudsmithstudios/vigil/releases/tag/v0.1.0
