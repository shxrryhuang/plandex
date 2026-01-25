package shared

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDebugLevel_String(t *testing.T) {
	tests := []struct {
		level    DebugLevel
		expected string
	}{
		{DebugLevelInfo, "info"},
		{DebugLevelDebug, "debug"},
		{DebugLevelTrace, "trace"},
		{DebugLevel(99), "unknown"},
	}

	for _, tc := range tests {
		result := tc.level.String()
		if result != tc.expected {
			t.Errorf("DebugLevel(%d).String() = %q, want %q", tc.level, result, tc.expected)
		}
	}
}

func TestDebugLevel_Short(t *testing.T) {
	tests := []struct {
		level    DebugLevel
		expected string
	}{
		{DebugLevelInfo, "INFO"},
		{DebugLevelDebug, "DEBUG"},
		{DebugLevelTrace, "TRACE"},
		{DebugLevel(99), "???"},
	}

	for _, tc := range tests {
		result := tc.level.Short()
		if result != tc.expected {
			t.Errorf("DebugLevel(%d).Short() = %q, want %q", tc.level, result, tc.expected)
		}
	}
}

func TestParseDebugLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected DebugLevel
	}{
		{"info", DebugLevelInfo},
		{"Info", DebugLevelInfo},
		{"INFO", DebugLevelInfo},
		{"0", DebugLevelInfo},
		{"debug", DebugLevelDebug},
		{"Debug", DebugLevelDebug},
		{"1", DebugLevelDebug},
		{"trace", DebugLevelTrace},
		{"Trace", DebugLevelTrace},
		{"2", DebugLevelTrace},
		{"unknown", DebugLevelDebug}, // Default
	}

	for _, tc := range tests {
		result := ParseDebugLevel(tc.input)
		if result != tc.expected {
			t.Errorf("ParseDebugLevel(%q) = %v, want %v", tc.input, result, tc.expected)
		}
	}
}

func TestEnableDisableDebugMode(t *testing.T) {
	// Ensure disabled initially
	DisableDebugMode()

	if IsDebugEnabled() {
		t.Error("Debug mode should be disabled initially")
	}

	// Enable
	err := EnableDebugMode(DebugLevelDebug, "")
	if err != nil {
		t.Fatalf("EnableDebugMode failed: %v", err)
	}

	if !IsDebugEnabled() {
		t.Error("Debug mode should be enabled after EnableDebugMode")
	}

	if GetDebugLevel() != DebugLevelDebug {
		t.Errorf("GetDebugLevel() = %v, want debug", GetDebugLevel())
	}

	// Disable
	DisableDebugMode()

	if IsDebugEnabled() {
		t.Error("Debug mode should be disabled after DisableDebugMode")
	}
}

func TestEnableDebugMode_WithTraceFile(t *testing.T) {
	// Create temp file
	tmpDir, err := os.MkdirTemp("", "debug-mode-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	traceFile := filepath.Join(tmpDir, "traces.log")

	err = EnableDebugMode(DebugLevelTrace, traceFile)
	if err != nil {
		t.Fatalf("EnableDebugMode with trace file failed: %v", err)
	}

	// Record a trace
	TraceInfo("test_op", "Test message")

	// Disable to flush
	DisableDebugMode()

	// Check file exists and has content
	data, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("Failed to read trace file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "DEBUG SESSION") {
		t.Error("Trace file should contain session header")
	}
	if !strings.Contains(content, "test_op") {
		t.Error("Trace file should contain trace operation")
	}
}

func TestTraceStep(t *testing.T) {
	// Ensure completely disabled and cleared
	DisableDebugMode()
	ClearTraces()

	// Should not record when disabled
	TraceStep(PhaseExecution, "test_op_disabled", nil)
	if IsDebugEnabled() {
		t.Error("Debug mode should be disabled")
	}
	if len(GetTraces()) > 0 {
		t.Error("Should not record traces when disabled")
	}

	// Enable and record
	EnableDebugMode(DebugLevelDebug, "")
	defer DisableDebugMode()

	ClearTraces() // Ensure clean slate

	TraceStep(PhaseExecution, "test_op", map[string]interface{}{
		"key": "value",
	})

	traces := GetTraces()
	if len(traces) != 1 {
		t.Fatalf("Expected 1 trace, got %d", len(traces))
	}

	trace := traces[0]
	if trace.Phase != PhaseExecution {
		t.Errorf("Phase = %v, want execution", trace.Phase)
	}
	if trace.Operation != "test_op" {
		t.Errorf("Operation = %q, want test_op", trace.Operation)
	}
	if trace.Data["key"] != "value" {
		t.Error("Data should contain key=value")
	}
}

func TestTraceInfo(t *testing.T) {
	EnableDebugMode(DebugLevelInfo, "")
	defer DisableDebugMode()

	ClearTraces()
	TraceInfo("info_op", "Info message")

	traces := GetTraces()
	if len(traces) != 1 {
		t.Fatalf("Expected 1 trace, got %d", len(traces))
	}

	if traces[0].Level != DebugLevelInfo {
		t.Errorf("Level = %v, want info", traces[0].Level)
	}
	if traces[0].Message != "Info message" {
		t.Errorf("Message = %q, want 'Info message'", traces[0].Message)
	}
}

func TestTraceDebug(t *testing.T) {
	// At info level, debug traces should not be recorded
	EnableDebugMode(DebugLevelInfo, "")
	ClearTraces()
	TraceDebug("debug_op", "Debug message", nil)

	if len(GetTraces()) > 0 {
		t.Error("Debug traces should not be recorded at info level")
	}

	// At debug level, debug traces should be recorded
	DisableDebugMode()
	EnableDebugMode(DebugLevelDebug, "")
	defer DisableDebugMode()

	ClearTraces()
	TraceDebug("debug_op", "Debug message", map[string]interface{}{"test": true})

	traces := GetTraces()
	if len(traces) != 1 {
		t.Fatalf("Expected 1 trace at debug level, got %d", len(traces))
	}
}

func TestTraceVerbose(t *testing.T) {
	// At debug level, verbose traces should not be recorded
	EnableDebugMode(DebugLevelDebug, "")
	ClearTraces()
	TraceVerbose("verbose_op", "Verbose message", nil)

	if len(GetTraces()) > 0 {
		t.Error("Verbose traces should not be recorded at debug level")
	}

	// At trace level, verbose traces should be recorded
	DisableDebugMode()
	EnableDebugMode(DebugLevelTrace, "")
	defer DisableDebugMode()

	ClearTraces()
	TraceVerbose("verbose_op", "Verbose message", nil)

	traces := GetTraces()
	if len(traces) != 1 {
		t.Fatalf("Expected 1 trace at trace level, got %d", len(traces))
	}
}

func TestTraceError(t *testing.T) {
	EnableDebugMode(DebugLevelInfo, "")
	defer DisableDebugMode()

	ClearTraces()
	testErr := errors.New("test error")
	TraceError("error_op", testErr, map[string]interface{}{"context": "test"})

	traces := GetTraces()
	if len(traces) != 1 {
		t.Fatalf("Expected 1 trace, got %d", len(traces))
	}

	trace := traces[0]
	if !strings.Contains(trace.Message, "ERROR") {
		t.Errorf("Message should contain ERROR, got: %s", trace.Message)
	}
	if trace.Data["error"] != "test error" {
		t.Errorf("Data should contain error message")
	}
	if trace.Stack == "" {
		t.Error("Error traces should capture stack")
	}
}

func TestTraceDuration(t *testing.T) {
	EnableDebugMode(DebugLevelDebug, "")
	defer DisableDebugMode()

	ClearTraces()
	start := time.Now()
	time.Sleep(10 * time.Millisecond)
	TraceDuration("timed_op", start, nil)

	traces := GetTraces()
	if len(traces) != 1 {
		t.Fatalf("Expected 1 trace, got %d", len(traces))
	}

	trace := traces[0]
	if trace.Duration < 10*time.Millisecond {
		t.Errorf("Duration = %v, should be >= 10ms", trace.Duration)
	}
	if !strings.Contains(trace.Message, "Completed") {
		t.Errorf("Message should indicate completion, got: %s", trace.Message)
	}
}

func TestGetTracesFiltered(t *testing.T) {
	EnableDebugMode(DebugLevelTrace, "")
	defer DisableDebugMode()

	ClearTraces()
	TraceStep(PhaseExecution, "exec_op", nil)
	TraceStep(PhaseStreaming, "stream_op", nil)
	TraceStep(PhaseExecution, "exec_op2", nil)
	TraceInfo("info_op", "Info message")

	// Filter by phase
	execTraces := GetTracesFiltered(DebugLevelInfo, PhaseExecution, "")
	if len(execTraces) != 2 {
		t.Errorf("Expected 2 execution traces, got %d", len(execTraces))
	}

	// Filter by operation
	streamTraces := GetTracesFiltered(DebugLevelInfo, "", "stream")
	if len(streamTraces) != 1 {
		t.Errorf("Expected 1 stream trace, got %d", len(streamTraces))
	}

	// Filter by level
	infoTraces := GetTracesFiltered(DebugLevelDebug, "", "")
	// This should include debug and trace level traces
	if len(infoTraces) < 3 {
		t.Errorf("Expected at least 3 traces at debug level filter, got %d", len(infoTraces))
	}
}

func TestExportTraces(t *testing.T) {
	EnableDebugMode(DebugLevelDebug, "")
	defer DisableDebugMode()

	ClearTraces()
	TraceInfo("test_op", "Test with key sk-1234567890abcdefghijklmnopqrstuv")

	// Export with sanitization
	data, err := ExportTraces(SanitizeLevelStandard)
	if err != nil {
		t.Fatalf("ExportTraces failed: %v", err)
	}

	content := string(data)
	if strings.Contains(content, "sk-1234") {
		t.Error("Exported traces should be sanitized")
	}
	if !strings.Contains(content, "[REDACTED_OPENAI_KEY]") {
		t.Error("Exported traces should contain redaction marker")
	}
}

func TestClearTraces(t *testing.T) {
	EnableDebugMode(DebugLevelDebug, "")
	defer DisableDebugMode()

	TraceInfo("test", "Test 1")
	TraceInfo("test", "Test 2")

	if len(GetTraces()) != 2 {
		t.Fatalf("Expected 2 traces before clear, got %d", len(GetTraces()))
	}

	ClearTraces()

	if len(GetTraces()) != 0 {
		t.Errorf("Expected 0 traces after clear, got %d", len(GetTraces()))
	}
}

func TestCaptureEnvironment(t *testing.T) {
	env := CaptureEnvironment(SanitizeLevelStandard)

	// Check required fields
	if _, ok := env["goVersion"]; !ok {
		t.Error("Should include goVersion")
	}
	if _, ok := env["os"]; !ok {
		t.Error("Should include os")
	}
	if _, ok := env["arch"]; !ok {
		t.Error("Should include arch")
	}
	if _, ok := env["memory"]; !ok {
		t.Error("Should include memory")
	}

	// Memory should have required fields
	if mem, ok := env["memory"].(map[string]interface{}); ok {
		if _, ok := mem["allocMB"]; !ok {
			t.Error("Memory should include allocMB")
		}
	} else {
		t.Error("Memory should be a map")
	}
}

func TestGetDebugModeState(t *testing.T) {
	DisableDebugMode()

	state := GetDebugModeState()
	if enabled, ok := state["enabled"].(bool); !ok || enabled {
		t.Error("State should show disabled")
	}

	EnableDebugMode(DebugLevelTrace, "")
	defer DisableDebugMode()

	state = GetDebugModeState()
	if enabled, ok := state["enabled"].(bool); !ok || !enabled {
		t.Error("State should show enabled")
	}
	if level, ok := state["level"].(string); !ok || level != "trace" {
		t.Errorf("State level = %v, want trace", state["level"])
	}
	if _, ok := state["sessionId"]; !ok {
		t.Error("State should include sessionId when enabled")
	}
	if _, ok := state["startTime"]; !ok {
		t.Error("State should include startTime when enabled")
	}
}

func TestSetDebugOptions(t *testing.T) {
	EnableDebugMode(DebugLevelDebug, "")
	defer DisableDebugMode()

	SetDebugOptions(true, true)

	state := GetDebugModeState()
	if captureStack, ok := state["captureStack"].(bool); !ok || !captureStack {
		t.Error("captureStack should be true")
	}
	if captureEnv, ok := state["captureEnv"].(bool); !ok || !captureEnv {
		t.Error("captureEnv should be true")
	}
}

func TestTraceRingBuffer(t *testing.T) {
	EnableDebugMode(DebugLevelDebug, "")
	defer DisableDebugMode()

	ClearTraces()

	// Set a small max for testing (this requires accessing internal state)
	CurrentDebugMode.mu.Lock()
	originalMax := CurrentDebugMode.maxTraces
	CurrentDebugMode.maxTraces = 5
	CurrentDebugMode.mu.Unlock()

	defer func() {
		CurrentDebugMode.mu.Lock()
		CurrentDebugMode.maxTraces = originalMax
		CurrentDebugMode.mu.Unlock()
	}()

	// Add more traces than the limit
	for i := 0; i < 10; i++ {
		TraceInfo("test", "Trace")
	}

	traces := GetTraces()
	if len(traces) != 5 {
		t.Errorf("Expected 5 traces (ring buffer limit), got %d", len(traces))
	}
}

func TestTraceWithSource(t *testing.T) {
	EnableDebugMode(DebugLevelTrace, "")
	defer DisableDebugMode()

	ClearTraces()
	TraceInfo("test_op", "Test message")

	traces := GetTraces()
	if len(traces) != 1 {
		t.Fatalf("Expected 1 trace, got %d", len(traces))
	}

	// At trace level, source should be captured
	if traces[0].Source == "" {
		t.Error("Source should be captured at trace level")
	}
	// Should contain file:line format
	if !strings.Contains(traces[0].Source, ".go:") {
		t.Errorf("Source should contain .go: format, got: %s", traces[0].Source)
	}
}

func TestTraceTimestamp(t *testing.T) {
	EnableDebugMode(DebugLevelDebug, "")
	defer DisableDebugMode()

	ClearTraces()

	before := time.Now()
	TraceInfo("test", "Test")
	after := time.Now()

	traces := GetTraces()
	if len(traces) != 1 {
		t.Fatalf("Expected 1 trace, got %d", len(traces))
	}

	ts := traces[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("Timestamp %v should be between %v and %v", ts, before, after)
	}
}

func TestDisableDebugMode_FlushesTraceFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "debug-flush-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	traceFile := filepath.Join(tmpDir, "traces.log")

	EnableDebugMode(DebugLevelDebug, traceFile)
	TraceInfo("test", "Before disable")

	// Disable should flush and close file
	DisableDebugMode()

	// Check file has footer
	data, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("Failed to read trace file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "END SESSION") {
		t.Error("Trace file should contain session footer after disable")
	}
}

func TestConcurrentTracing(t *testing.T) {
	EnableDebugMode(DebugLevelDebug, "")
	defer DisableDebugMode()

	ClearTraces()

	// Spawn multiple goroutines to write traces
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				TraceInfo("concurrent", "Test")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have some traces (not necessarily all 1000 due to ring buffer)
	traces := GetTraces()
	if len(traces) == 0 {
		t.Error("Should have captured some traces")
	}
}
