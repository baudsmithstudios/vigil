package collector

import (
	"errors"
	"testing"
)

func TestParseMMCErrStats(t *testing.T) {
	data := `# Command Timeout Occurred:	 10
# Command CRC Errors Occurred:	 2
# Data Timeout Occurred:	 0
`
	total, err := parseMMCErrStats(data)
	if err != nil {
		t.Fatalf("parseMMCErrStats error: %v", err)
	}
	if total != 12 {
		t.Errorf("expected total 12, got %d", total)
	}
}

func TestParseMMCErrStats_Invalid(t *testing.T) {
	if _, err := parseMMCErrStats("not a counter file"); err == nil {
		t.Fatal("expected parse error for invalid file")
	}
}

func TestSDErrorCollector_DeltaAcrossTicks(t *testing.T) {
	calls := 0
	c := &sdErrorCollector{
		prev: make(map[string]uint64),
		glob: func(pattern string) ([]string, error) {
			return []string{"/sys/kernel/debug/mmc0/err_stats"}, nil
		},
		readFile: func(name string) ([]byte, error) {
			calls++
			if calls == 1 {
				return []byte("# err a: 10\n# err b: 2\n"), nil
			}
			return []byte("# err a: 10\n# err b: 3\n"), nil
		},
	}

	first := c.collect()
	if len(first) != 1 {
		t.Fatalf("expected 1 host on first collect, got %d", len(first))
	}
	if first[0].Host != "mmc0" {
		t.Fatalf("expected host mmc0, got %q", first[0].Host)
	}
	if first[0].Delta != 0 {
		t.Fatalf("expected zero first delta, got %d", first[0].Delta)
	}

	second := c.collect()
	if len(second) != 1 {
		t.Fatalf("expected 1 host on second collect, got %d", len(second))
	}
	if second[0].Delta != 1 {
		t.Fatalf("expected delta 1 on second collect, got %d", second[0].Delta)
	}
}

func TestSDErrorCollector_NoReadableStats(t *testing.T) {
	c := &sdErrorCollector{
		prev: make(map[string]uint64),
		glob: func(pattern string) ([]string, error) {
			return []string{"/sys/kernel/debug/mmc0/err_stats"}, nil
		},
		readFile: func(name string) ([]byte, error) {
			return nil, errors.New("permission denied")
		},
	}

	got := c.collect()
	if len(got) != 0 {
		t.Fatalf("expected no snapshots when stats cannot be read, got %d", len(got))
	}
}
