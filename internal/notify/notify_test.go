package notify

import (
	"context"
	"fmt"
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
