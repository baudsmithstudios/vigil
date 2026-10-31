package notify

import (
	"context"
	"encoding/json"
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
		{"server query", srv.URL + "?mode=test", "vigil-alerts"},
		{"server credentials", strings.Replace(srv.URL, "://", "://:@", 1), "vigil-alerts"},
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

type countNotifier struct{ sent *int }

func (c countNotifier) Send(_ context.Context, _ alert.State, _ bool) error {
	*c.sent++
	return nil
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		raw     string
		wantErr bool
	}{
		{"https://hooks.example.com/alert", false},
		{"HTTPS://hooks.example.com/alert", false},
		{"http://hooks.example.com/alert", true},
		{"https://", true},
		{"://bad", true},
	}
	for _, tt := range tests {
		if err := ValidateURL(tt.raw); (err != nil) != tt.wantErr {
			t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
		}
	}
}

func TestParseQuietHours(t *testing.T) {
	got, err := ParseQuietHours([]string{"02:00-06:30", "23:00 - 05:00"})
	if err != nil {
		t.Fatalf("ParseQuietHours: %v", err)
	}
	want := []QuietWindow{{Start: 120, End: 390}, {Start: 1380, End: 300}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %+v, want %+v", got, want)
	}

	for _, spec := range []string{"02:00", "25:00-06:00", "02:00-24:00", "never"} {
		if _, err := ParseQuietHours([]string{spec}); err == nil {
			t.Errorf("ParseQuietHours(%q): expected error", spec)
		}
	}
}

func TestIsQuiet(t *testing.T) {
	sameDay := []QuietWindow{{Start: 2 * 60, End: 6 * 60}}
	overnight := []QuietWindow{{Start: 23 * 60, End: 5 * 60}}
	tests := []struct {
		name    string
		hour    int
		minute  int
		windows []QuietWindow
		want    bool
	}{
		{"before a same-day window", 1, 59, sameDay, false},
		{"start is inclusive", 2, 0, sameDay, true},
		{"last minute of a same-day window", 5, 59, sameDay, true},
		{"end is exclusive", 6, 0, sameDay, false},
		{"before an overnight window", 22, 59, overnight, false},
		{"overnight start", 23, 0, overnight, true},
		{"overnight after midnight", 0, 0, overnight, true},
		{"overnight end is exclusive", 5, 0, overnight, false},
		{"equal start and end is never quiet", 2, 0, []QuietWindow{{Start: 120, End: 120}}, false},
		{"no windows", 2, 0, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, 10, 31, tt.hour, tt.minute, 0, 0, time.UTC)
			if got := IsQuiet(now, tt.windows); got != tt.want {
				t.Errorf("IsQuiet = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQuietSendSuppresses(t *testing.T) {
	muted := &Mute{}
	muted.Toggle()
	allDay := []QuietWindow{{Start: 0, End: 24 * 60}}
	tests := []struct {
		name    string
		mute    *Mute
		windows []QuietWindow
		want    int
	}{
		{"no mute and no windows", nil, nil, 1},
		{"unmuted", &Mute{}, nil, 1},
		{"muted", muted, nil, 0},
		{"inside a quiet window", nil, allDay, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sent := 0
			q := Quiet{Inner: countNotifier{&sent}, Windows: tt.windows, Mute: tt.mute}
			if err := q.Send(context.Background(), alert.State{Name: "cpu_percent"}, false); err != nil {
				t.Fatalf("Send() error = %v", err)
			}
			if sent != tt.want {
				t.Errorf("sent %d, want %d", sent, tt.want)
			}
		})
	}
}

func TestMuteToggleReportsNewState(t *testing.T) {
	var m Mute
	if !m.Toggle() || !m.IsMuted() {
		t.Error("expected the first toggle to mute")
	}
	if m.Toggle() || m.IsMuted() {
		t.Error("expected the second toggle to unmute")
	}
}

func TestWebhookSendPostsAlertJSON(t *testing.T) {
	var got map[string]string
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
	}))
	defer srv.Close()

	a := alert.State{Name: "cpu_percent", Message: "CPU high", FiredAt: time.Date(2026, 5, 6, 14, 15, 16, 0, time.UTC)}
	if err := (Webhook{URL: srv.URL}).Send(context.Background(), a, true); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
	want := map[string]string{"name": "cpu_percent", "message": "CPU high", "status": "resolved", "since": "2026-05-06T14:15:16Z"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestDiscordSendReportsServerRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad token", http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := Discord{WebhookURL: srv.URL}.Send(context.Background(), alert.State{Name: "cpu_percent"}, false)
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "bad token") {
		t.Fatalf("expected the status and body in the error, got %v", err)
	}
}
