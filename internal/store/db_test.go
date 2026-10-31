package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "vigil.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func fireAlert(t *testing.T, db *DB, message string) {
	t.Helper()
	if err := db.WriteAlert(AlertRecord{Name: "high_cpu", Message: message, FiredAt: time.Now()}); err != nil {
		t.Fatalf("WriteAlert: %v", err)
	}
}

func activeAlerts(t *testing.T, db *DB) []AlertRecord {
	t.Helper()
	alerts, err := db.ActiveAlerts()
	if err != nil {
		t.Fatalf("ActiveAlerts: %v", err)
	}
	return alerts
}

func rowCount(t *testing.T, db *DB, table string) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

var clearAlert = map[string]func(*DB, string) error{
	"dismissed": (*DB).DismissAlert,
	"resolved":  (*DB).ResolveAlert,
}

func TestDB_QueryRecentReturnsNewestOfTheMetric(t *testing.T) {
	db := tempDB(t)
	now := time.Now()
	readings := []Reading{
		{Metric: "cpu_percent", Value: 42.5, Timestamp: now},
		{Metric: "cpu_percent", Value: 50.0, Timestamp: now.Add(time.Second)},
		{Metric: "mem_percent", Value: 60.0, Timestamp: now.Add(2 * time.Second)},
	}
	if err := db.WriteReadings(readings); err != nil {
		t.Fatalf("WriteReadings: %v", err)
	}

	got, err := db.QueryRecent("cpu_percent", 1)
	if err != nil {
		t.Fatalf("QueryRecent: %v", err)
	}
	if len(got) != 1 || got[0].Value != 50.0 {
		t.Fatalf("expected only the 50.0 reading, got %+v", got)
	}
}

func TestOpen_CreatesDatabaseWithOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vigil.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected mode 0600, got %04o", got)
	}
}

func TestOpen_ReopensExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vigil.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	fireAlert(t, db, "CPU above 90%")
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer db.Close()
	if len(activeAlerts(t, db)) != 1 {
		t.Error("expected the alert to survive a reopen")
	}
}

func TestOpen_AddsDismissedAtToDatabaseWithoutIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vigil.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`CREATE TABLE alerts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		message TEXT NOT NULL,
		fired_at INTEGER NOT NULL,
		resolved INTEGER NOT NULL DEFAULT 0,
		resolved_at INTEGER
	)`); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	fireAlert(t, db, "CPU above 90%")
	if err := db.DismissAlert("high_cpu"); err != nil {
		t.Fatalf("DismissAlert: %v", err)
	}
	if len(activeAlerts(t, db)) != 0 {
		t.Error("expected dismissal to work on the migrated table")
	}
}

func TestDB_Purge(t *testing.T) {
	db := tempDB(t)
	readings := []Reading{
		{Metric: "cpu_percent", Value: 1.0, Timestamp: time.Now().Add(-2 * time.Hour)},
		{Metric: "cpu_percent", Value: 2.0, Timestamp: time.Now()},
	}
	if err := db.WriteReadings(readings); err != nil {
		t.Fatal(err)
	}

	if err := db.PurgeOlderThan(time.Now().Add(-1 * time.Hour)); err != nil {
		t.Fatalf("PurgeOlderThan: %v", err)
	}

	got, err := db.QueryRecent("cpu_percent", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Value != 2.0 {
		t.Fatalf("expected only the 2.0 reading after purge, got %+v", got)
	}
}

func TestDB_WriteAlert(t *testing.T) {
	db := tempDB(t)
	fireAlert(t, db, "CPU above 90%")

	alerts := activeAlerts(t, db)
	if len(alerts) != 1 || alerts[0].Name != "high_cpu" {
		t.Fatalf("expected the high_cpu alert, got %+v", alerts)
	}
}

func TestDB_ClearedAlertIsInactiveUntilItFiresAgain(t *testing.T) {
	for name, clear := range clearAlert {
		t.Run(name, func(t *testing.T) {
			db := tempDB(t)
			fireAlert(t, db, "CPU above 90%")

			if err := clear(db, "high_cpu"); err != nil {
				t.Fatal(err)
			}
			if got := activeAlerts(t, db); len(got) != 0 {
				t.Fatalf("expected no active alerts, got %+v", got)
			}

			fireAlert(t, db, "CPU above 90% again")
			got := activeAlerts(t, db)
			if len(got) != 1 || got[0].Message != "CPU above 90% again" {
				t.Fatalf("expected the refired alert, got %+v", got)
			}
		})
	}
}

func TestWriteAndPurgeServiceCheckReadings(t *testing.T) {
	db := tempDB(t)
	readings := []ServiceCheckReading{
		{Name: "web", CheckType: "http", Up: true, StatusCode: 200, LatencyMs: 23, Timestamp: time.Now()},
		{Name: "mqtt", CheckType: "tcp", Error: "connection refused", Timestamp: time.Now()},
	}
	if err := db.WriteServiceCheckReadings(readings); err != nil {
		t.Fatalf("write service check readings: %v", err)
	}
	if got := rowCount(t, db, "service_checks"); got != 2 {
		t.Errorf("expected 2 rows, got %d", got)
	}

	if err := db.PurgeServiceChecks(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if got := rowCount(t, db, "service_checks"); got != 0 {
		t.Errorf("expected 0 rows after purge, got %d", got)
	}
}

func TestWriteAndPurgeContainerReadings(t *testing.T) {
	db := tempDB(t)
	readings := []ContainerReading{
		{Name: "hass", ContainerID: "3f9a1c2e7b4d", Status: "running", CPUPercent: 2.1, MemUsed: 1024, Timestamp: time.Now()},
	}
	if err := db.WriteContainerReadings(readings); err != nil {
		t.Fatalf("write container readings: %v", err)
	}
	if got := rowCount(t, db, "container_metrics"); got != 1 {
		t.Errorf("expected 1 row, got %d", got)
	}

	if err := db.PurgeContainerMetrics(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if got := rowCount(t, db, "container_metrics"); got != 0 {
		t.Errorf("expected 0 rows after purge, got %d", got)
	}
}
