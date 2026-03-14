package store

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Reading is a single metric sample.
type Reading struct {
	Metric    string
	Value     float64
	Timestamp time.Time
}

// AlertRecord persists alert state to the DB.
type AlertRecord struct {
	Name        string
	Message     string
	FiredAt     time.Time
	Resolved    bool
	ResolvedAt  *time.Time
	DismissedAt *time.Time
}

// DB wraps the SQLite connection.
type DB struct {
	sql *sql.DB
}

// Open opens (or creates) the SQLite database at path and applies the schema.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Restrict database file to owner-only access.
	if err := os.Chmod(path, 0600); err != nil && !os.IsNotExist(err) {
		conn.Close()
		return nil, fmt.Errorf("chmod database: %w", err)
	}
	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	// Migrate: add dismissed_at column if missing (existing databases).
	if _, err := conn.Exec(`ALTER TABLE alerts ADD COLUMN dismissed_at INTEGER`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			conn.Close()
			return nil, fmt.Errorf("migrate alerts: %w", err)
		}
	}
	return &DB{sql: conn}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.sql.Close()
}

// WriteReadings inserts a batch of metric readings.
func (d *DB) WriteReadings(readings []Reading) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO metrics (metric, value, timestamp) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range readings {
		if _, err := stmt.Exec(r.Metric, r.Value, r.Timestamp.UnixNano()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// QueryRecent returns up to limit readings for the given metric, newest first.
func (d *DB) QueryRecent(metric string, limit int) ([]Reading, error) {
	rows, err := d.sql.Query(
		`SELECT metric, value, timestamp FROM metrics
		 WHERE metric = ? ORDER BY timestamp DESC LIMIT ?`,
		metric, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Reading
	for rows.Next() {
		var r Reading
		var ts int64
		if err := rows.Scan(&r.Metric, &r.Value, &ts); err != nil {
			return nil, err
		}
		r.Timestamp = time.Unix(0, ts)
		out = append(out, r)
	}
	return out, rows.Err()
}

// QueryRange returns all readings for the given metric within [from, to], oldest first.
func (d *DB) QueryRange(metric string, from, to time.Time) ([]Reading, error) {
	rows, err := d.sql.Query(
		`SELECT metric, value, timestamp FROM metrics
		 WHERE metric = ? AND timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`,
		metric, from.UnixNano(), to.UnixNano(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Reading
	for rows.Next() {
		var r Reading
		var ts int64
		if err := rows.Scan(&r.Metric, &r.Value, &ts); err != nil {
			return nil, err
		}
		r.Timestamp = time.Unix(0, ts)
		out = append(out, r)
	}
	return out, rows.Err()
}

// QueryRangePrefix returns all readings for metrics matching the given prefix within [from, to], oldest first.
// Useful for dynamic metric keys like "temp:cpu_thermal" where the sensor name varies.
func (d *DB) QueryRangePrefix(prefix string, from, to time.Time) ([]Reading, error) {
	rows, err := d.sql.Query(
		`SELECT metric, value, timestamp FROM metrics
		 WHERE metric LIKE ? AND timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`,
		prefix+"%", from.UnixNano(), to.UnixNano(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Reading
	for rows.Next() {
		var r Reading
		var ts int64
		if err := rows.Scan(&r.Metric, &r.Value, &ts); err != nil {
			return nil, err
		}
		r.Timestamp = time.Unix(0, ts)
		out = append(out, r)
	}
	return out, rows.Err()
}

// DistinctMetricNames returns distinct metric names matching the given prefix within [from, to].
func (d *DB) DistinctMetricNames(prefix string, from, to time.Time) ([]string, error) {
	rows, err := d.sql.Query(
		`SELECT DISTINCT metric FROM metrics
		 WHERE metric LIKE ? AND timestamp >= ? AND timestamp <= ?
		 ORDER BY metric ASC`,
		prefix+"%", from.UnixNano(), to.UnixNano(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// PurgeOlderThan deletes all metric readings with timestamps before cutoff.
func (d *DB) PurgeOlderThan(cutoff time.Time) error {
	_, err := d.sql.Exec(`DELETE FROM metrics WHERE timestamp < ?`, cutoff.UnixNano())
	return err
}

// WriteAlert inserts or updates an alert record.
func (d *DB) WriteAlert(a AlertRecord) error {
	_, err := d.sql.Exec(
		`INSERT INTO alerts (name, message, fired_at, resolved)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   message  = excluded.message,
		   fired_at = excluded.fired_at,
		   resolved = excluded.resolved`,
		a.Name, a.Message, a.FiredAt.UnixNano(), boolToInt(a.Resolved),
	)
	return err
}

// ResolveAlert marks a named alert as resolved.
func (d *DB) ResolveAlert(name string) error {
	_, err := d.sql.Exec(
		`UPDATE alerts SET resolved = 1, resolved_at = ? WHERE name = ?`,
		time.Now().UnixNano(), name,
	)
	return err
}

// DismissAlert marks a named alert as dismissed (manually cleared by user).
func (d *DB) DismissAlert(name string) error {
	_, err := d.sql.Exec(
		`UPDATE alerts SET dismissed_at = ? WHERE name = ?`,
		time.Now().UnixNano(), name,
	)
	return err
}

// ActiveAlerts returns all alerts that have not been resolved or dismissed.
func (d *DB) ActiveAlerts() ([]AlertRecord, error) {
	rows, err := d.sql.Query(
		`SELECT name, message, fired_at FROM alerts
		 WHERE resolved = 0 AND dismissed_at IS NULL
		 ORDER BY fired_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AlertRecord
	for rows.Next() {
		var a AlertRecord
		var ts int64
		if err := rows.Scan(&a.Name, &a.Message, &ts); err != nil {
			return nil, err
		}
		a.FiredAt = time.Unix(0, ts)
		out = append(out, a)
	}
	return out, rows.Err()
}

// ContainerReading is a single container stats sample.
type ContainerReading struct {
	Name        string
	ContainerID string
	Status      string
	CPUPercent  float64
	MemUsed     uint64
	MemLimit    uint64
	MemPercent  float64
	Timestamp   time.Time
}

// WriteContainerReadings inserts a batch of container metric readings.
func (d *DB) WriteContainerReadings(readings []ContainerReading) error {
	if len(readings) == 0 {
		return nil
	}
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO container_metrics
		(name, container_id, status, cpu_percent, mem_used, mem_limit, mem_percent, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range readings {
		if _, err := stmt.Exec(r.Name, r.ContainerID, r.Status,
			r.CPUPercent, r.MemUsed, r.MemLimit, r.MemPercent,
			r.Timestamp.UnixNano()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PurgeContainerMetrics deletes container readings older than cutoff.
func (d *DB) PurgeContainerMetrics(cutoff time.Time) error {
	_, err := d.sql.Exec(`DELETE FROM container_metrics WHERE timestamp < ?`, cutoff.UnixNano())
	return err
}

// ServiceCheckReading is a single service health check result.
type ServiceCheckReading struct {
	Name       string
	CheckType  string
	Up         bool
	StatusCode int
	LatencyMs  int64
	Error      string
	Timestamp  time.Time
}

// WriteServiceCheckReadings inserts a batch of service check results.
func (d *DB) WriteServiceCheckReadings(readings []ServiceCheckReading) error {
	if len(readings) == 0 {
		return nil
	}
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO service_checks
		(name, check_type, up, status_code, latency_ms, error, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range readings {
		if _, err := stmt.Exec(r.Name, r.CheckType, boolToInt(r.Up),
			r.StatusCode, r.LatencyMs, r.Error, r.Timestamp.UnixNano()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DistinctServiceNames returns distinct service check names within [from, to].
func (d *DB) DistinctServiceNames(from, to time.Time) ([]string, error) {
	rows, err := d.sql.Query(
		`SELECT DISTINCT name FROM service_checks
		 WHERE timestamp >= ? AND timestamp <= ?
		 ORDER BY name ASC`,
		from.UnixNano(), to.UnixNano(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// QueryServiceCheckRange returns service check readings for the named service within [from, to], oldest first.
func (d *DB) QueryServiceCheckRange(name string, from, to time.Time) ([]ServiceCheckReading, error) {
	rows, err := d.sql.Query(
		`SELECT name, check_type, up, status_code, latency_ms, error, timestamp
		 FROM service_checks
		 WHERE name = ? AND timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`,
		name, from.UnixNano(), to.UnixNano(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ServiceCheckReading
	for rows.Next() {
		var r ServiceCheckReading
		var ts int64
		var upInt int
		if err := rows.Scan(&r.Name, &r.CheckType, &upInt, &r.StatusCode, &r.LatencyMs, &r.Error, &ts); err != nil {
			return nil, err
		}
		r.Up = upInt != 0
		r.Timestamp = time.Unix(0, ts)
		out = append(out, r)
	}
	return out, rows.Err()
}

// PurgeServiceChecks deletes service check readings older than cutoff.
func (d *DB) PurgeServiceChecks(cutoff time.Time) error {
	_, err := d.sql.Exec(`DELETE FROM service_checks WHERE timestamp < ?`, cutoff.UnixNano())
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
