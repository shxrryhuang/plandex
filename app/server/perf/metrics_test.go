package perf

import (
	"strings"
	"testing"
	"time"
)

func TestTimerRecordsElapsed(t *testing.T) {
	ResetForTest()

	done := Timer("test", "sleep10")
	time.Sleep(10 * time.Millisecond)
	done()

	sums := Summaries()
	var found bool
	for _, s := range sums {
		if s.Operation == "test.sleep10" {
			found = true
			if s.Count != 1 {
				t.Errorf("count: want 1, got %d", s.Count)
			}
			if s.AvgMs < 5 {
				t.Errorf("avg: want >= 5ms, got %.2f", s.AvgMs)
			}
		}
	}
	if !found {
		t.Fatal("test.sleep10 not in Summaries()")
	}
}

func TestRecordCountAccumulates(t *testing.T) {
	ResetForTest()

	RecordCount("test.ctr", 5)
	RecordCount("test.ctr", 3)

	c := Counters()
	if c["test.ctr"] != 8 {
		t.Errorf("counter: want 8, got %d", c["test.ctr"])
	}
}

func TestMultipleSamplesPercentiles(t *testing.T) {
	ResetForTest()

	// Record 200 samples: 1ms … 200ms
	for i := 1; i <= 200; i++ {
		Record("test", "pctl", time.Duration(i)*time.Millisecond)
	}

	sums := Summaries()
	var s Summary
	for _, candidate := range sums {
		if candidate.Operation == "test.pctl" {
			s = candidate
			break
		}
	}
	if s.Count != 200 {
		t.Fatalf("count: want 200, got %d", s.Count)
	}
	if s.MinMs > 1.5 {
		t.Errorf("min: want ~1ms, got %.2f", s.MinMs)
	}
	if s.MaxMs < 199 {
		t.Errorf("max: want ~200ms, got %.2f", s.MaxMs)
	}
	if s.P50Ms < 90 || s.P50Ms > 110 {
		t.Errorf("p50: want ~100ms, got %.2f", s.P50Ms)
	}
	if s.P95Ms < 180 {
		t.Errorf("p95: want >= 180ms, got %.2f", s.P95Ms)
	}
}

func TestReportContainsKeys(t *testing.T) {
	ResetForTest()

	Record("cat", "op42", 7*time.Millisecond)
	RecordCount("cat.counter42", 11)

	r := Report()
	if !strings.Contains(r, "cat.op42") {
		t.Errorf("Report missing histogram key; got:\n%s", r)
	}
	if !strings.Contains(r, "cat.counter42") {
		t.Errorf("Report missing counter key; got:\n%s", r)
	}
}

func TestEmptyReport(t *testing.T) {
	ResetForTest()
	r := Report()
	if !strings.Contains(r, "No metrics recorded yet") {
		t.Errorf("empty report unexpected text:\n%s", r)
	}
}
