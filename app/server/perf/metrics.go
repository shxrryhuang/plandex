// Package perf provides a lightweight, lock-contention-minimal metrics
// collector for the Plandex server.  It records duration histograms and
// monotonic counters keyed by (category, operation) and exposes a
// human-readable report.  Overhead per sample is a single mutex-guarded
// append; counters use sync/atomic with no lock.
package perf

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Well-known category constants used throughout the server.
const (
	CatFileIO      = "file_io"
	CatGitOps      = "git_ops"
	CatProviderCall = "provider_call"
	CatPatchApply  = "patch_apply"
	CatStream      = "stream"
	CatLock        = "lock"
	CatDiff        = "diff"
	CatTellExec    = "tell_exec"
	CatBuildExec   = "build_exec"
)

// maxSamples is how many raw durations we keep per histogram for
// percentile calculation.  After this many the oldest samples are
// silently dropped; the count / total / min / max accumulators remain
// exact.
const maxSamples = 1024

// histogram tracks duration statistics for a single named metric.
type histogram struct {
	mu      sync.Mutex
	count   int64
	totalNs int64
	minNs   int64
	maxNs   int64
	samples []int64 // up to maxSamples raw durations in ns
}

// Summary is an exported snapshot of a single histogram.
type Summary struct {
	Operation string  `json:"operation"`
	Count     int64   `json:"count"`
	TotalMs   float64 `json:"total_ms"`
	AvgMs     float64 `json:"avg_ms"`
	MinMs     float64 `json:"min_ms"`
	MaxMs     float64 `json:"max_ms"`
	P50Ms     float64 `json:"p50_ms"`
	P95Ms     float64 `json:"p95_ms"`
	P99Ms     float64 `json:"p99_ms"`
}

var (
	registry   = make(map[string]*histogram)
	registryMu sync.RWMutex

	counters   = make(map[string]*int64)
	countersMu sync.RWMutex
)

func getOrCreate(key string) *histogram {
	registryMu.RLock()
	h, ok := registry[key]
	registryMu.RUnlock()
	if ok {
		return h
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if h, ok := registry[key]; ok {
		return h
	}
	h = &histogram{minNs: math.MaxInt64}
	registry[key] = h
	return h
}

// Record adds a single duration observation under category.operation.
func Record(category, operation string, d time.Duration) {
	key := category + "." + operation
	h := getOrCreate(key)
	ns := int64(d)

	h.mu.Lock()
	h.count++
	h.totalNs += ns
	if ns < h.minNs {
		h.minNs = ns
	}
	if ns > h.maxNs {
		h.maxNs = ns
	}
	if len(h.samples) < maxSamples {
		h.samples = append(h.samples, ns)
	}
	h.mu.Unlock()
}

// Timer returns a stop-function; calling it records the elapsed wall time.
//
//	done := perf.Timer("category", "op")
//	defer done()
func Timer(category, operation string) func() {
	start := time.Now()
	return func() {
		Record(category, operation, time.Since(start))
	}
}

// RecordCount atomically adds n to the named counter.
func RecordCount(key string, n int) {
	countersMu.RLock()
	ptr, ok := counters[key]
	countersMu.RUnlock()
	if !ok {
		countersMu.Lock()
		if p, exists := counters[key]; exists {
			ptr = p
		} else {
			v := int64(0)
			ptr = &v
			counters[key] = ptr
		}
		countersMu.Unlock()
	}
	atomic.AddInt64(ptr, int64(n))
}

func (h *histogram) snapshot(name string) Summary {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.count == 0 {
		return Summary{Operation: name}
	}
	s := Summary{
		Operation: name,
		Count:     h.count,
		TotalMs:   float64(h.totalNs) / 1e6,
		AvgMs:     float64(h.totalNs) / float64(h.count) / 1e6,
		MinMs:     float64(h.minNs) / 1e6,
		MaxMs:     float64(h.maxNs) / 1e6,
	}
	if len(h.samples) > 0 {
		sorted := make([]int64, len(h.samples))
		copy(sorted, h.samples)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		n := len(sorted)
		s.P50Ms = float64(sorted[n/2]) / 1e6
		idx95 := int(float64(n) * 0.95)
		if idx95 >= n {
			idx95 = n - 1
		}
		s.P95Ms = float64(sorted[idx95]) / 1e6
		idx99 := int(float64(n) * 0.99)
		if idx99 >= n {
			idx99 = n - 1
		}
		s.P99Ms = float64(sorted[idx99]) / 1e6
	}
	return s
}

// Summaries returns all histograms that have at least one sample,
// sorted by total time descending (heaviest first).
func Summaries() []Summary {
	registryMu.RLock()
	defer registryMu.RUnlock()

	out := make([]Summary, 0, len(registry))
	for key, h := range registry {
		s := h.snapshot(key)
		if s.Count > 0 {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].TotalMs > out[j].TotalMs
	})
	return out
}

// Counters returns a snapshot of every counter.
func Counters() map[string]int64 {
	countersMu.RLock()
	defer countersMu.RUnlock()

	out := make(map[string]int64, len(counters))
	for k, ptr := range counters {
		out[k] = atomic.LoadInt64(ptr)
	}
	return out
}

// Report returns a human-readable table of all metrics.
func Report() string {
	var sb strings.Builder

	sums := Summaries()
	sb.WriteString("=== Plandex Performance Metrics ===\n\n")
	if len(sums) == 0 {
		sb.WriteString("No metrics recorded yet.\n")
	} else {
		sb.WriteString(fmt.Sprintf("%-44s %8s %10s %8s %8s %8s %8s %8s\n",
			"operation", "count", "total_ms", "avg_ms", "min_ms", "max_ms", "p50_ms", "p95_ms"))
		sb.WriteString(strings.Repeat("-", 108) + "\n")
		for _, s := range sums {
			sb.WriteString(fmt.Sprintf("%-44s %8d %10.2f %8.2f %8.2f %8.2f %8.2f %8.2f\n",
				s.Operation, s.Count, s.TotalMs, s.AvgMs, s.MinMs, s.MaxMs, s.P50Ms, s.P95Ms))
		}
	}

	ctrs := Counters()
	if len(ctrs) > 0 {
		sb.WriteString("\n=== Counters ===\n")
		var keys []string
		for k := range ctrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("  %-44s %d\n", k, ctrs[k]))
		}
	}
	return sb.String()
}

// ResetForTest clears all state.  Only for use in tests.
func ResetForTest() {
	registryMu.Lock()
	registry = make(map[string]*histogram)
	registryMu.Unlock()

	countersMu.Lock()
	counters = make(map[string]*int64)
	countersMu.Unlock()
}
