package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempDB(t *testing.T) (*DB, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "vigil-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	db, err := Open(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatal(err)
	}

	return db, func() {
		db.Close()
		os.Remove(f.Name())
	}
}

func TestDB_WriteAndQuery(t *testing.T) {
	db, cleanup := tempDB(t)
	defer cleanup()

	now := time.Now()
	readings := []Reading{
		{Metric: "cpu_percent", Value: 42.5, Timestamp: now},
		{Metric: "mem_percent", Value: 60.0, Timestamp: now.Add(time.Second)},
	}

	if err := db.WriteReadings(readings); err != nil {
		t.Fatalf("WriteReadings: %v", err)
	}

	got, err := db.QueryRecent("cpu_percent", 10)
	if err != nil {
		t.Fatalf("QueryRecent: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 reading, got %d", len(got))
	}
	if got[0].Value != 42.5 {
		t.Errorf("expected value 42.5, got %f", got[0].Value)
	}
}

func TestOpen_CreatesDatabaseWithOwnerOnlyPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vigil.db")

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

func TestDB_Purge(t *testing.T) {
	db, cleanup := tempDB(t)
	defer cleanup()

	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now()

	readings := []Reading{
		{Metric: "cpu_percent", Value: 1.0, Timestamp: old},
		{Metric: "cpu_percent", Value: 2.0, Timestamp: recent},
	}

	if err := db.WriteReadings(readings); err != nil {
		t.Fatal(err)
	}

	cutoff := time.Now().Add(-1 * time.Hour)
	if err := db.PurgeOlderThan(cutoff); err != nil {
		t.Fatalf("PurgeOlderThan: %v", err)
	}

	got, err := db.QueryRecent("cpu_percent", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 reading after purge, got %d", len(got))
	}
	if got[0].Value != 2.0 {
		t.Errorf("expected value 2.0, got %f", got[0].Value)
	}
}

func TestDB_WriteAlert(t *testing.T) {
	db, cleanup := tempDB(t)
	defer cleanup()

	alert := AlertRecord{
		Name:    "high_cpu",
		Message: "CPU above 90%",
		FiredAt: time.Now(),
	}

	if err := db.WriteAlert(alert); err != nil {
		t.Fatalf("WriteAlert: %v", err)
	}

	alerts, err := db.ActiveAlerts()
	if err != nil {
		t.Fatalf("ActiveAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 active alert, got %d", len(alerts))
	}
	if alerts[0].Name != "high_cpu" {
		t.Errorf("expected name 'high_cpu', got %q", alerts[0].Name)
	}
}

func TestDB_DismissAlert(t *testing.T) {
	db, cleanup := tempDB(t)
	defer cleanup()

	alert := AlertRecord{
		Name:    "high_cpu",
		Message: "CPU above 90%",
		FiredAt: time.Now(),
	}
	if err := db.WriteAlert(alert); err != nil {
		t.Fatal(err)
	}

	if err := db.DismissAlert("high_cpu"); err != nil {
		t.Fatalf("DismissAlert: %v", err)
	}

	// Dismissed alert should not appear in active alerts.
	alerts, err := db.ActiveAlerts()
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 active alerts after dismiss, got %d", len(alerts))
	}
}

func TestDB_WriteAlert_ReactivatesDismissedAlert(t *testing.T) {
	db, cleanup := tempDB(t)
	defer cleanup()

	firstFire := AlertRecord{
		Name:    "high_cpu",
		Message: "CPU above 90%",
		FiredAt: time.Now(),
	}
	if err := db.WriteAlert(firstFire); err != nil {
		t.Fatal(err)
	}
	if err := db.DismissAlert("high_cpu"); err != nil {
		t.Fatalf("DismissAlert: %v", err)
	}

	// Re-fire the same alert name; it should become active again.
	refire := AlertRecord{
		Name:    "high_cpu",
		Message: "CPU above 90% again",
		FiredAt: time.Now().Add(time.Second),
	}
	if err := db.WriteAlert(refire); err != nil {
		t.Fatalf("WriteAlert refire: %v", err)
	}

	alerts, err := db.ActiveAlerts()
	if err != nil {
		t.Fatalf("ActiveAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 active alert after refire, got %d", len(alerts))
	}
	if alerts[0].Message != "CPU above 90% again" {
		t.Fatalf("expected updated message after refire, got %q", alerts[0].Message)
	}
}

func TestDB_ResolveAlert(t *testing.T) {
	db, cleanup := tempDB(t)
	defer cleanup()

	alert := AlertRecord{
		Name:    "high_cpu",
		Message: "CPU above 90%",
		FiredAt: time.Now(),
	}
	if err := db.WriteAlert(alert); err != nil {
		t.Fatal(err)
	}

	if err := db.ResolveAlert("high_cpu"); err != nil {
		t.Fatalf("ResolveAlert: %v", err)
	}

	alerts, err := db.ActiveAlerts()
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 active alerts after resolve, got %d", len(alerts))
	}
}

func TestWriteAndPurgeServiceCheckReadings(t *testing.T) {
	db, cleanup := tempDB(t)
	defer cleanup()

	readings := []ServiceCheckReading{
		{
			Name:       "web",
			CheckType:  "http",
			Up:         true,
			StatusCode: 200,
			LatencyMs:  23,
			Timestamp:  time.Now(),
		},
		{
			Name:       "mqtt",
			CheckType:  "tcp",
			Up:         false,
			StatusCode: 0,
			LatencyMs:  0,
			Error:      "connection refused",
			Timestamp:  time.Now(),
		},
	}
	if err := db.WriteServiceCheckReadings(readings); err != nil {
		t.Fatalf("write service check readings: %v", err)
	}

	var count int
	err := db.sql.QueryRow("SELECT COUNT(*) FROM service_checks").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows, got %d", count)
	}

	if err := db.PurgeServiceChecks(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("purge: %v", err)
	}
	err = db.sql.QueryRow("SELECT COUNT(*) FROM service_checks").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows after purge, got %d", count)
	}
}

func TestDB_DistinctMetricNames(t *testing.T) {
	db, cleanup := tempDB(t)
	defer cleanup()

	now := time.Now()
	readings := []Reading{
		{Metric: "disk_read:sda", Value: 100, Timestamp: now},
		{Metric: "disk_read:sdb", Value: 200, Timestamp: now},
		{Metric: "disk_write:sda", Value: 50, Timestamp: now},
		{Metric: "cpu_percent", Value: 42, Timestamp: now},
	}
	if err := db.WriteReadings(readings); err != nil {
		t.Fatal(err)
	}

	names, err := db.DistinctMetricNames("disk_read:", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("DistinctMetricNames: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d: %v", len(names), names)
	}
	for _, n := range names {
		if n != "disk_read:sda" && n != "disk_read:sdb" {
			t.Errorf("unexpected name: %q", n)
		}
	}
}

func TestDB_DistinctMetricNames_EmptyRange(t *testing.T) {
	db, cleanup := tempDB(t)
	defer cleanup()

	now := time.Now()
	readings := []Reading{
		{Metric: "disk_read:sda", Value: 100, Timestamp: now},
	}
	if err := db.WriteReadings(readings); err != nil {
		t.Fatal(err)
	}

	names, err := db.DistinctMetricNames("disk_read:", now.Add(time.Hour), now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 names for empty range, got %d", len(names))
	}
}

func TestDB_DistinctServiceNames(t *testing.T) {
	db, cleanup := tempDB(t)
	defer cleanup()

	now := time.Now()
	readings := []ServiceCheckReading{
		{Name: "web", CheckType: "http", Up: true, StatusCode: 200, LatencyMs: 23, Timestamp: now},
		{Name: "mqtt", CheckType: "tcp", Up: true, StatusCode: 0, LatencyMs: 5, Timestamp: now},
		{Name: "web", CheckType: "http", Up: true, StatusCode: 200, LatencyMs: 25, Timestamp: now.Add(time.Second)},
	}
	if err := db.WriteServiceCheckReadings(readings); err != nil {
		t.Fatal(err)
	}

	names, err := db.DistinctServiceNames(now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("DistinctServiceNames: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d: %v", len(names), names)
	}
}

func TestDB_QueryServiceCheckRange(t *testing.T) {
	db, cleanup := tempDB(t)
	defer cleanup()

	now := time.Now()
	readings := []ServiceCheckReading{
		{Name: "web", CheckType: "http", Up: true, StatusCode: 200, LatencyMs: 23, Timestamp: now},
		{Name: "web", CheckType: "http", Up: false, StatusCode: 0, LatencyMs: 0, Error: "timeout", Timestamp: now.Add(time.Second)},
		{Name: "mqtt", CheckType: "tcp", Up: true, StatusCode: 0, LatencyMs: 5, Timestamp: now},
	}
	if err := db.WriteServiceCheckReadings(readings); err != nil {
		t.Fatal(err)
	}

	got, err := db.QueryServiceCheckRange("web", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryServiceCheckRange: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 readings for 'web', got %d", len(got))
	}
	// Should be ordered by timestamp ascending.
	if got[0].LatencyMs != 23 {
		t.Errorf("expected first reading latency 23, got %d", got[0].LatencyMs)
	}
	if !got[0].Up {
		t.Error("expected first reading Up=true")
	}
	if got[1].Up {
		t.Error("expected second reading Up=false")
	}
	if got[1].Error != "timeout" {
		t.Errorf("expected error 'timeout', got %q", got[1].Error)
	}

	// mqtt should not appear in web query.
	gotMqtt, err := db.QueryServiceCheckRange("mqtt", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(gotMqtt) != 1 {
		t.Fatalf("expected 1 reading for 'mqtt', got %d", len(gotMqtt))
	}
}
