package collector

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"
)

func TestRunnerCollect_ReadsTheHost(t *testing.T) {
	r, _ := New(time.Second, "", nil, nil)
	snap := r.Collect(time.Now())
	if snap.Memory.TotalBytes == 0 {
		t.Error("expected non-zero memory total")
	}
	if snap.UptimeSec == 0 {
		t.Error("expected non-zero uptime")
	}
}

func TestRunnerDeliver_LogsStallOnceAndDropCountOnResume(t *testing.T) {
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	out := make(chan Snapshot, 1)
	r := &Runner{out: out}

	r.deliver(Snapshot{})
	r.deliver(Snapshot{})
	r.deliver(Snapshot{})
	if n := strings.Count(logs.String(), "\n"); n != 1 || !strings.Contains(logs.String(), "stalled") {
		t.Fatalf("want one stall line while stalled, got %d: %q", n, logs.String())
	}

	<-out
	logs.Reset()
	r.deliver(Snapshot{})
	if !strings.Contains(logs.String(), "dropped 2 snapshots") {
		t.Fatalf("got %q, want drop count of 2", logs.String())
	}

	<-out
	logs.Reset()
	r.deliver(Snapshot{})
	if logs.Len() != 0 {
		t.Fatalf("count not reset after logging: %q", logs.String())
	}
}
