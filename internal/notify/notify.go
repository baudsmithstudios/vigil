package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"vigil/internal/alert"
	"vigil/internal/ntfy"
)

// Mute is a thread-safe toggle for temporarily suppressing notifications.
type Mute struct {
	muted atomic.Bool
}

// Toggle flips the mute state and returns the new value.
func (m *Mute) Toggle() bool {
	for {
		old := m.muted.Load()
		if m.muted.CompareAndSwap(old, !old) {
			return !old
		}
	}
}

// IsMuted returns true if notifications are currently muted.
func (m *Mute) IsMuted() bool {
	return m.muted.Load()
}

var client = &http.Client{Timeout: 10 * time.Second}

// ValidateURL checks that a webhook URL is well-formed and uses HTTPS.
func ValidateURL(raw string) error {
	return validateHTTPSURL(raw, "webhook URL")
}

func validateHTTPSURL(raw, label string) error {
	_, err := parseHTTPSURL(raw, label)
	return err
}

func parseHTTPSURL(raw, label string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", label, err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return nil, fmt.Errorf("%s must use HTTPS (got %s)", label, u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%s missing host", label)
	}
	return u, nil
}

// Notifier sends alert notifications to an external service.
type Notifier interface {
	Send(ctx context.Context, a alert.State, resolved bool) error
}

// Multi fans out to multiple notifiers.
type Multi []Notifier

func (m Multi) Send(ctx context.Context, a alert.State, resolved bool) error {
	var errs []string
	for _, n := range m {
		if err := n.Send(ctx, a, resolved); err != nil {
			log.Printf("notify error: %v", err)
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("notify: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Discord sends notifications to a Discord webhook.
type Discord struct {
	WebhookURL string
}

func (d Discord) Send(ctx context.Context, a alert.State, resolved bool) error {
	var msg string
	if resolved {
		msg = fmt.Sprintf("✓ **%s** resolved", a.Name)
	} else {
		msg = fmt.Sprintf("⚠ **%s** exceeded threshold — %s (since %s)",
			a.Name, a.Message, a.FiredAt.Format("15:04:05"))
	}
	return postJSON(ctx, d.WebhookURL, map[string]string{"content": msg})
}

// Webhook sends notifications to a generic HTTP endpoint.
type Webhook struct {
	URL string
}

func (w Webhook) Send(ctx context.Context, a alert.State, resolved bool) error {
	status := "fired"
	if resolved {
		status = "resolved"
	}
	payload := map[string]string{
		"name":    a.Name,
		"message": a.Message,
		"status":  status,
		"since":   a.FiredAt.Format(time.RFC3339),
	}
	return postJSON(ctx, w.URL, payload)
}

// Ntfy sends notifications to an ntfy topic.
type Ntfy struct {
	Server string
	Topic  string
}

func (n Ntfy) Send(ctx context.Context, a alert.State, resolved bool) error {
	if err := ntfy.ValidateServerURL(n.Server); err != nil {
		return fmt.Errorf("ntfy server: %w", err)
	}
	if err := ntfy.ValidateTopic(n.Topic); err != nil {
		return fmt.Errorf("ntfy topic: %w", err)
	}
	endpoint, err := url.JoinPath(n.Server, n.Topic)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("⚠ %s exceeded threshold\n%s\nSince %s",
		a.Name, a.Message, a.FiredAt.Format("15:04:05"))
	if resolved {
		msg = fmt.Sprintf("✓ %s resolved", a.Name)
	}
	return postText(ctx, endpoint, "Vigil alert", msg)
}

// QuietWindow represents a daily time-of-day suppression window.
type QuietWindow struct {
	Start, End int // minutes since midnight
}

// ParseQuietHours parses strings like "02:00-06:00" into QuietWindow values.
func ParseQuietHours(specs []string) ([]QuietWindow, error) {
	var windows []QuietWindow
	for _, s := range specs {
		parts := strings.SplitN(s, "-", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid quiet_hours format %q (expected HH:MM-HH:MM)", s)
		}
		start, err := parseTimeOfDay(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("quiet_hours start: %w", err)
		}
		end, err := parseTimeOfDay(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("quiet_hours end: %w", err)
		}
		windows = append(windows, QuietWindow{Start: start, End: end})
	}
	return windows, nil
}

func parseTimeOfDay(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("invalid time %q (expected HH:MM)", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

// IsQuiet returns true if the given time falls within any quiet window.
func IsQuiet(now time.Time, windows []QuietWindow) bool {
	mins := now.Hour()*60 + now.Minute()
	for _, w := range windows {
		if w.Start <= w.End {
			// Same-day window: e.g. 02:00-06:00
			if mins >= w.Start && mins < w.End {
				return true
			}
		} else {
			// Overnight window: e.g. 23:00-05:00
			if mins >= w.Start || mins < w.End {
				return true
			}
		}
	}
	return false
}

// Quiet wraps a Notifier and suppresses sends during quiet windows or when muted.
type Quiet struct {
	Inner   Notifier
	Windows []QuietWindow
	Mute    *Mute
}

func (q Quiet) Send(ctx context.Context, a alert.State, resolved bool) error {
	if q.Mute != nil && q.Mute.IsMuted() {
		log.Printf("notification suppressed (muted): %s", a.Name)
		return nil
	}
	if IsQuiet(time.Now(), q.Windows) {
		log.Printf("notification suppressed (quiet hours): %s", a.Name)
		return nil
	}
	return q.Inner.Send(ctx, a, resolved)
}

func postJSON(ctx context.Context, url string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if len(respBody) > 0 {
			return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
		}
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

func postText(ctx context.Context, url, title, body string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", title)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if len(respBody) > 0 {
			return fmt.Errorf("ntfy returned %d: %s", resp.StatusCode, string(respBody))
		}
		return fmt.Errorf("ntfy returned %d", resp.StatusCode)
	}
	return nil
}
