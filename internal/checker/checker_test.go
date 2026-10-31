package checker

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vigil/internal/config"
)

func checkOnce(t *testing.T, httpChecks []config.HTTPCheck, portChecks []config.PortCheck) ServiceStatus {
	t.Helper()
	sc := New(config.Services{}, httpChecks, portChecks)
	sc.CheckOnce()
	results, _ := sc.Snapshot()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	return results[0]
}

func statusServer(t *testing.T, code int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func listen(t *testing.T, addr string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skipf("cannot listen on %s: %v", addr, err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

func port(ln net.Listener) int {
	return ln.Addr().(*net.TCPAddr).Port
}

func TestHTTPCheck_Success(t *testing.T) {
	r := checkOnce(t, []config.HTTPCheck{{Name: "web", URL: statusServer(t, 200), ExpectedStatus: 200}}, nil)
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
	r := checkOnce(t, []config.HTTPCheck{{Name: "api", URL: statusServer(t, 503), ExpectedStatus: 200}}, nil)
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

func TestHTTPCheck_NoExpectedStatusMeansAny2xx(t *testing.T) {
	tests := []struct {
		code int
		up   bool
	}{
		{204, true},
		{299, true},
		{300, false},
	}
	for _, tt := range tests {
		r := checkOnce(t, []config.HTTPCheck{{Name: "web", URL: statusServer(t, tt.code)}}, nil)
		if r.Up != tt.up {
			t.Errorf("status %d: Up = %v, want %v", tt.code, r.Up, tt.up)
		}
	}
}

func TestHTTPCheck_ConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	r := checkOnce(t, []config.HTTPCheck{{Name: "dead", URL: url, ExpectedStatus: 200}}, nil)
	if r.Up {
		t.Error("expected Up=false for connection refused")
	}
	if r.Error == "" {
		t.Error("expected non-empty error for connection refused")
	}
}

func TestTCPCheck_Success(t *testing.T) {
	ln := listen(t, "127.0.0.1:0")
	r := checkOnce(t, nil, []config.PortCheck{{Name: "listener", Host: "127.0.0.1", Port: port(ln)}})
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
	ln := listen(t, "127.0.0.1:0")
	closed := port(ln)
	ln.Close()

	r := checkOnce(t, nil, []config.PortCheck{{Name: "dead-port", Host: "127.0.0.1", Port: closed}})
	if r.Up {
		t.Error("expected Up=false for connection refused")
	}
	if r.Error == "" {
		t.Error("expected non-empty error for connection refused")
	}
}

func TestTCPCheck_IPv6(t *testing.T) {
	ln := listen(t, "[::1]:0")
	r := checkOnce(t, nil, []config.PortCheck{{Name: "ipv6-listener", Host: "::1", Port: port(ln)}})
	if !r.Up {
		t.Errorf("expected Up=true for IPv6 TCP check, got false; error: %s", r.Error)
	}
}

func TestRun_ChecksBeforeTheFirstTick(t *testing.T) {
	sc := New(config.Services{Interval: config.TestDuration(time.Hour)}, nil, nil)
	if _, cycle := sc.Snapshot(); !cycle.IsZero() {
		t.Error("expected zero cycle time before Run")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sc.Run(ctx)

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, cycle := sc.Snapshot(); !cycle.IsZero() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("expected a completed cycle without waiting for the ticker")
}

func TestSnapshot_ReturnsCopyAndCycleTime(t *testing.T) {
	now := time.Now()
	sc := &ServiceChecker{results: []ServiceStatus{{Name: "web"}}, cycleTime: now}

	got, cycle := sc.Snapshot()
	if !cycle.Equal(now) {
		t.Fatalf("expected cycle time %v, got %v", now, cycle)
	}

	got[0].Name = "mutated"
	after, _ := sc.Snapshot()
	if after[0].Name != "web" {
		t.Fatal("expected Snapshot to return a defensive copy")
	}
}
