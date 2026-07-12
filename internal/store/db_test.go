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
