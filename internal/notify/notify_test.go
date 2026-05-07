package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	var gotMethod, gotPath, gotTitle, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotTitle = r.Header.Get("Title")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

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
	if !strings.Contains(gotBody, "cpu_percent") || !strings.Contains(gotBody, "CPU usage above 90%") {
		t.Errorf("body = %q, want alert name and message", gotBody)
	}
}
