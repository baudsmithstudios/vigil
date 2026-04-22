package checker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vigil/internal/config"
)

func newTestChecker(httpChecks []config.HTTPCheck, portChecks []config.PortCheck) *ServiceChecker {
	svc := config.Services{
		Interval:            config.TestDuration(time.Second),
		FailuresBeforeAlert: 2,
	}
	return New(svc, httpChecks, portChecks)
}

func waitForResults(t *testing.T, sc *ServiceChecker, n int) []ServiceStatus {
	t.Helper()
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		results := sc.Results()
		if len(results) >= n {
			return results
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d results", n)
	return nil
}

func TestHTTPCheck_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	sc := newTestChecker(
		[]config.HTTPCheck{{Name: "web", URL: srv.URL, ExpectedStatus: 200}},
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sc.Run(ctx)

	results := waitForResults(t, sc, 1)
	r := results[0]
	if r.Name != "web" {
		t.Errorf("expected name 'web', got %q", r.Name)
	}
	if r.CheckType != "http" {
		t.Errorf("expected check type 'http', got %q", r.CheckType)
	}
	if !r.Up {
		t.Errorf("expected Up=true, got false")
	}
	if r.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", r.StatusCode)
	}
	if r.Latency <= 0 {
		t.Errorf("expected positive latency, got %s", r.Latency)
	}
	if r.CheckedAt.IsZero() {
		t.Error("expected non-zero CheckedAt")
	}
	if r.Error != "" {
		t.Errorf("expected empty error, got %q", r.Error)
	}
}

func TestHTTPCheck_WrongStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	sc := newTestChecker(
		[]config.HTTPCheck{{Name: "api", URL: srv.URL, ExpectedStatus: 200}},
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sc.Run(ctx)

	results := waitForResults(t, sc, 1)
	r := results[0]
	if r.Up {
		t.Error("expected Up=false for wrong status")
	}
	if r.StatusCode != 503 {
		t.Errorf("expected status 503, got %d", r.StatusCode)
	}
	if r.Error == "" {
		t.Error("expected non-empty error for wrong status")
	}
}

func TestHTTPCheck_DefaultExpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer srv.Close()

	sc := newTestChecker(
		[]config.HTTPCheck{{Name: "no-content", URL: srv.URL, ExpectedStatus: 0}},
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sc.Run(ctx)

	results := waitForResults(t, sc, 1)
	r := results[0]
	if !r.Up {
		t.Error("expected Up=true for 204 with default expected status (any 2xx)")
	}
}

func TestHTTPCheck_ConnectionRefused(t *testing.T) {
	sc := newTestChecker(
		[]config.HTTPCheck{{Name: "dead", URL: "http://127.0.0.1:1", ExpectedStatus: 200}},
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sc.Run(ctx)

	results := waitForResults(t, sc, 1)
	r := results[0]
	if r.Up {
		t.Error("expected Up=false for connection refused")
	}
	if r.Error == "" {
		t.Error("expected non-empty error for connection refused")
	}
}

func TestTCPCheck_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	sc := newTestChecker(
		nil,
		[]config.PortCheck{{Name: "listener", Host: "127.0.0.1", Port: port}},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sc.Run(ctx)

	results := waitForResults(t, sc, 1)
	r := results[0]
	if r.Name != "listener" {
		t.Errorf("expected name 'listener', got %q", r.Name)
	}
	if r.CheckType != "tcp" {
		t.Errorf("expected check type 'tcp', got %q", r.CheckType)
	}
	if !r.Up {
		t.Errorf("expected Up=true, got false; error: %s", r.Error)
	}
}

func TestTCPCheck_ConnectionRefused(t *testing.T) {
	sc := newTestChecker(
		nil,
		[]config.PortCheck{{Name: "dead-port", Host: "127.0.0.1", Port: 1}},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sc.Run(ctx)

	results := waitForResults(t, sc, 1)
	r := results[0]
	if r.Up {
		t.Error("expected Up=false for connection refused")
	}
	if r.Error == "" {
		t.Error("expected non-empty error for connection refused")
	}
}

func TestTCPCheck_IPv6(t *testing.T) {
	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 loopback not available")
	}
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	sc := newTestChecker(
		nil,
		[]config.PortCheck{{Name: "ipv6-listener", Host: "::1", Port: port}},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sc.Run(ctx)

	results := waitForResults(t, sc, 1)
	r := results[0]
	if !r.Up {
		t.Errorf("expected Up=true for IPv6 TCP check, got false; error: %s", r.Error)
	}
}

func TestCycleTime_AdvancesAfterCycle(t *testing.T) {
	sc := newTestChecker(
		[]config.HTTPCheck{{Name: "dummy", URL: "http://127.0.0.1:1", ExpectedStatus: 200}},
		nil,
	)

	if !sc.CycleTime().IsZero() {
		t.Error("expected zero CycleTime before Run")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sc.Run(ctx)

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !sc.CycleTime().IsZero() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("expected non-zero CycleTime after Run")
}

func TestSnapshot_ReturnsResultsAndCycleTime(t *testing.T) {
	sc := newTestChecker(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sc.Run(ctx)

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !sc.CycleTime().IsZero() {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	results := sc.Results()
	gotResults, gotCycle := sc.Snapshot()

	if gotCycle.IsZero() {
		t.Fatal("expected non-zero cycle time from Snapshot")
	}
	if len(gotResults) != len(results) {
		t.Fatalf("expected %d results, got %d", len(results), len(gotResults))
	}

	// Ensure Snapshot returns a copy.
	if len(gotResults) > 0 {
		origName := gotResults[0].Name
		gotResults[0].Name = "mutated"
		after, _ := sc.Snapshot()
		if after[0].Name != origName {
			t.Fatal("expected Snapshot results to be copied, but mutation leaked")
		}
	}
}
