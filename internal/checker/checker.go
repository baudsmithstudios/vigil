package checker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"vigil/internal/config"
)

const requestTimeout = 10 * time.Second

type ServiceStatus struct {
	Name       string
	CheckType  string // "http" or "tcp"
	Up         bool
	StatusCode int // HTTP only, 0 for TCP
	Latency    time.Duration
	CheckedAt  time.Time
	Error      string // empty when healthy
}

// ServiceChecker runs periodic HTTP and TCP health checks.
type ServiceChecker struct {
	interval   time.Duration
	httpChecks []config.HTTPCheck
	portChecks []config.PortCheck
	client     *http.Client

	mu        sync.Mutex
	results   []ServiceStatus
	cycleTime time.Time
}

func New(svc config.Services, httpChecks []config.HTTPCheck, portChecks []config.PortCheck) *ServiceChecker {
	return &ServiceChecker{
		interval:   svc.Interval.Duration,
		httpChecks: httpChecks,
		portChecks: portChecks,
		client: &http.Client{
			Timeout: requestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Run executes health checks immediately, then on each tick until ctx is cancelled.
func (sc *ServiceChecker) Run(ctx context.Context) {
	sc.runCycle()
	ticker := time.NewTicker(sc.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sc.runCycle()
		}
	}
}

func (sc *ServiceChecker) CheckOnce() ([]ServiceStatus, time.Time) {
	sc.runCycle()
	return sc.Snapshot()
}

// Snapshot returns the latest results and cycle completion time as a consistent pair.
func (sc *ServiceChecker) Snapshot() ([]ServiceStatus, time.Time) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	out := make([]ServiceStatus, len(sc.results))
	copy(out, sc.results)
	return out, sc.cycleTime
}

func (sc *ServiceChecker) runCycle() {
	now := time.Now()
	results := make([]ServiceStatus, 0, len(sc.httpChecks)+len(sc.portChecks))
	for _, hc := range sc.httpChecks {
		results = append(results, sc.checkHTTP(hc, now))
	}
	for _, pc := range sc.portChecks {
		results = append(results, sc.checkTCP(pc, now))
	}
	sc.mu.Lock()
	sc.results = results
	sc.cycleTime = time.Now()
	sc.mu.Unlock()
}

func (sc *ServiceChecker) checkHTTP(hc config.HTTPCheck, checkedAt time.Time) ServiceStatus {
	status := ServiceStatus{
		Name:      hc.Name,
		CheckType: "http",
		CheckedAt: checkedAt,
	}
	start := time.Now()
	resp, err := sc.client.Get(hc.URL)
	status.Latency = time.Since(start)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	resp.Body.Close()
	status.StatusCode = resp.StatusCode
	if hc.ExpectedStatus == 0 {
		status.Up = resp.StatusCode >= 200 && resp.StatusCode < 300
	} else {
		status.Up = resp.StatusCode == hc.ExpectedStatus
	}
	if !status.Up && status.Error == "" {
		if hc.ExpectedStatus == 0 {
			status.Error = fmt.Sprintf("expected 2xx, got %d", resp.StatusCode)
		} else {
			status.Error = fmt.Sprintf("expected status %d, got %d", hc.ExpectedStatus, resp.StatusCode)
		}
	}
	return status
}

func (sc *ServiceChecker) checkTCP(pc config.PortCheck, checkedAt time.Time) ServiceStatus {
	status := ServiceStatus{
		Name:      pc.Name,
		CheckType: "tcp",
		CheckedAt: checkedAt,
	}
	addr := net.JoinHostPort(pc.Host, fmt.Sprintf("%d", pc.Port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, requestTimeout)
	status.Latency = time.Since(start)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	conn.Close()
	status.Up = true
	return status
}
