# Roadmap

Last updated: May 7, 2026

Vigil is an active project. Priorities and planned features may change as the project matures, and nothing in this roadmap is on a defined release schedule.

This roadmap reflects current product direction, not guaranteed delivery commitments.

## Shipped

### v1

All v1 features have shipped:

- Docker container monitoring (per-container CPU %, memory, status)
- Under-voltage / throttle detection (Pi power supply alerts)
- USB / mount watchdog (debounced alerts, flap detection)
- HTTP health checks + TCP port reachability
- Network drop and error alerting (`net_drops`, `net_errors` prefix keys)
- Interactive setup (`vigil init`)
- Keyboard navigation and help screen
- Uptime display in TUI header
- Historical data persistence (SQLite)
- TUI/headless loop unification
- Historical view infrastructure (disk I/O, network, service latency)

### v2

- Disk I/O health metrics (disk latency, disk utilization, swap in/out rate, SD/MMC error deltas)
- ntfy.sh notifications with optional custom server

---

## v2 — Operational Completeness

Rounds out the core tool: better notifications, more Pi-relevant metrics, scripting support, and operational quality-of-life.

### TUI quit falls back to headless

Closing the dashboard should not interrupt collection or alerting.

- `q` transitions to headless mode instead of exiting — collection and alerting continue uninterrupted
- Container only restarts on explicit `Ctrl+C` or `docker compose down`
- `docker attach` remains the way to open the dashboard; `Ctrl+P Ctrl+Q` to detach without stopping

### Config hot-reload

Changing thresholds or webhook URLs currently requires a container restart.

- Reload and validate `config.toml` without a full container restart
- Apply valid config changes atomically and keep the current config if reload fails
- Support an explicit reload signal for operational use

### `vigil --once --json`

Single-shot output for scripting and integration.

- One collection tick, serialize to JSON on stdout, exit
- No TUI, no SQLite write, no background goroutines
- Useful for cron snapshots, watchdog scripts, Home Assistant integrations, feeding external monitoring

### `vigil doctor`

Diagnostic command for troubleshooting broken deployments.

- Runs environment checks, prints pass/fail report, exits
- Checks: config parsing, proc/sysfs access, storage writable, Docker socket, network, notification endpoints

### Wi-Fi signal quality

- Collect link quality, signal dBm, and noise dBm
- Surface in TUI network panel
- Configurable alert threshold for low signal strength

---

## v3 — Extended Monitoring

### Reboot detection

Silent reboots (power loss, kernel panic, watchdog) leave no trace.

- Detect reboots by tracking uptime changes
- Record reboot history in SQLite
- Optional alert on unexpected reboot

### TUI historical views

Historical data is already persisted. Surface it without leaving the terminal.

- Keybinding to enter historical mode from any panel
- Time-range navigation (last 1h / 6h / 24h)
- Downsampled rollups (min/avg/max at 5-minute intervals) for longer ranges beyond raw retention
- Separate rollup retention window for longer-term views with minimal storage overhead

---

## Lower Priority

### Export command

- `vigil export --last 24h --format csv` (also JSON)
- Pull data out for spreadsheets or external tools without touching SQLite directly

### Panel and collector toggles

- `[enable]` config section with boolean flags per subsystem
- TUI keybinding to toggle panel visibility for current session

### Configurable keybindings

- `[keybindings]` section in `config.toml` for remapping TUI keys
