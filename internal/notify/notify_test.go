package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"vigil/internal/alert"
)

type failNotifier struct{ err error }

func (f failNotifier) Send(_ context.Context, _ alert.State, _ bool) error {
	return f.err
}

type succeedNotifier struct{}

func (succeedNotifier) Send(_ context.Context, _ alert.State, _ bool) error {
	return nil
}

func TestMultiSendReturnsErrors(t *testing.T) {
	t.Run("all succeed returns nil", func(t *testing.T) {
		m := Multi{succeedNotifier{}, succeedNotifier{}}
		err := m.Send(context.Background(), alert.State{Name: "test", FiredAt: time.Now()}, false)
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("one failure returns error", func(t *testing.T) {
		m := Multi{
			succeedNotifier{},
			failNotifier{err: fmt.Errorf("discord down")},
		}
		err := m.Send(context.Background(), alert.State{Name: "test", FiredAt: time.Now()}, false)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "discord down") {
			t.Fatalf("expected error to contain 'discord down', got %q", err.Error())
		}
	})

	t.Run("multiple failures joined", func(t *testing.T) {
		m := Multi{
			failNotifier{err: fmt.Errorf("discord down")},
			failNotifier{err: fmt.Errorf("webhook timeout")},
		}
		err := m.Send(context.Background(), alert.State{Name: "test", FiredAt: time.Now()}, false)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "discord down") || !strings.Contains(err.Error(), "webhook timeout") {
			t.Fatalf("expected both errors, got %q", err.Error())
		}
	})
}

func TestNtfySendPostsMessageToTopic(t *testing.T) {
	var gotMethod, gotPath, gotTitle, gotContentType, gotBody string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotTitle = r.Header.Get("Title")
		gotContentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	useTestHTTPClient(t, srv.Client())

	n := Ntfy{Server: srv.URL, Topic: "vigil-alerts"}
	a := alert.State{
		Name:    "cpu_percent",
		Message: "CPU usage above 90%",
		FiredAt: time.Date(2026, 5, 6, 14, 15, 16, 0, time.UTC),
	}

	if err := n.Send(context.Background(), a, false); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want %s", gotMethod, http.MethodPost)
	}
	if gotPath != "/vigil-alerts" {
		t.Errorf("path = %q, want /vigil-alerts", gotPath)
	}
	if gotTitle != "Vigil alert" {
		t.Errorf("Title header = %q, want Vigil alert", gotTitle)
	}
	if gotContentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type header = %q, want text/plain; charset=utf-8", gotContentType)
	}
	wantBody := "⚠ cpu_percent exceeded threshold\nCPU usage above 90%\nSince 14:15:16"
	if gotBody != wantBody {
		t.Errorf("body = %q, want %q", gotBody, wantBody)
	}
}

func TestNtfySendPostsResolvedMessage(t *testing.T) {
	var gotBody string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	useTestHTTPClient(t, srv.Client())

	n := Ntfy{Server: srv.URL, Topic: "vigil-alerts"}
	a := alert.State{Name: "cpu_percent", FiredAt: time.Now()}

	if err := n.Send(context.Background(), a, true); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	wantBody := "✓ cpu_percent resolved"
	if gotBody != wantBody {
		t.Errorf("body = %q, want %q", gotBody, wantBody)
	}
}

func TestNtfySendRejectsInvalidConfigBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	useTestHTTPClient(t, srv.Client())

	tests := []struct {
		name   string
		server string
		topic  string
	}{
		{"server path", srv.URL + "/base", "vigil-alerts"},
		{"server query", srv.URL + "?token=secret", "vigil-alerts"},
		{"server credentials", strings.Replace(srv.URL, "://", "://user:pass@", 1), "vigil-alerts"},
		{"topic slash", srv.URL, "vigil/alerts"},
		{"topic percent", srv.URL, "vigil%2Falerts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := Ntfy{Server: tt.server, Topic: tt.topic}
			err := n.Send(context.Background(), alert.State{Name: "cpu_percent", FiredAt: time.Now()}, false)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if requests.Load() != 0 {
				t.Fatal("expected validation to fail before sending a request")
			}
		})
	}
}

func useTestHTTPClient(t *testing.T, c *http.Client) {
	t.Helper()
	old := client
	client = c
	t.Cleanup(func() {
		client = old
	})
}
