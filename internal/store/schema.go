package store

const schema = `
CREATE TABLE IF NOT EXISTS metrics (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    metric    TEXT    NOT NULL,
    value     REAL    NOT NULL,
    timestamp INTEGER NOT NULL  -- Unix nanoseconds
);
CREATE INDEX IF NOT EXISTS idx_metrics_metric_ts ON metrics (metric, timestamp);

CREATE TABLE IF NOT EXISTS container_metrics (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    container_id TEXT   NOT NULL,
    status      TEXT    NOT NULL,
    cpu_percent REAL    NOT NULL,
    mem_used    INTEGER NOT NULL,
    mem_limit   INTEGER NOT NULL,
    mem_percent REAL    NOT NULL,
    timestamp   INTEGER NOT NULL  -- Unix nanoseconds
);
CREATE INDEX IF NOT EXISTS idx_container_metrics_name_ts ON container_metrics (name, timestamp);

CREATE TABLE IF NOT EXISTS alerts (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT    NOT NULL UNIQUE,
    message      TEXT    NOT NULL,
    fired_at     INTEGER NOT NULL,  -- Unix nanoseconds
    resolved     INTEGER NOT NULL DEFAULT 0,
    resolved_at  INTEGER,
    dismissed_at INTEGER           -- NULL until manually cleared
);

CREATE TABLE IF NOT EXISTS service_checks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    check_type  TEXT    NOT NULL,
    up          INTEGER NOT NULL,
    status_code INTEGER NOT NULL,
    latency_ms  INTEGER NOT NULL,
    error       TEXT    NOT NULL DEFAULT '',
    timestamp   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_service_checks_name_ts ON service_checks (name, timestamp);
`
