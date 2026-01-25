package shared

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// DEBUG MODE - On-demand deep tracing for diagnostics
// =============================================================================

// DebugLevel specifies the verbosity of debug output
type DebugLevel int

const (
	// DebugLevelInfo shows only important information
	DebugLevelInfo DebugLevel = iota

	// DebugLevelDebug shows detailed debugging information
	DebugLevelDebug

	// DebugLevelTrace shows everything including step-by-step traces
	DebugLevelTrace
)

// DebugMode contains the configuration and state for debug mode
type DebugMode struct {
	// Enabled indicates if debug mode is active
	Enabled bool `json:"enabled"`

	// Level specifies the verbosity level
	Level DebugLevel `json:"level"`

	// TraceFile is the optional file path for writing traces
	TraceFile string `json:"traceFile,omitempty"`

	// CaptureStack enables stack trace capture
	CaptureStack bool `json:"captureStack"`

	// CaptureEnv enables (sanitized) environment capture
	CaptureEnv bool `json:"captureEnv"`

	// StartTime is when debug mode was enabled
	StartTime time.Time `json:"startTime"`

	// SessionId identifies the debug session
	SessionId string `json:"sessionId"`

	// traces stores captured trace entries
	traces []TraceEntry

	// mu protects concurrent access
	mu sync.RWMutex

	// traceFile is the open file handle for trace output
	traceFileHandle *os.File

	// maxTraces is the maximum number of traces to keep in memory
	maxTraces int
}

// TraceEntry represents a single trace event
type TraceEntry struct {
	// Timestamp is when the trace was recorded
	Timestamp time.Time `json:"timestamp"`

	// Level is the debug level of this trace
	Level DebugLevel `json:"level"`

	// Phase is the execution phase
	Phase ExecutionPhase `json:"phase,omitempty"`

	// Operation is the specific operation being traced
	Operation string `json:"operation"`

	// Message is the trace message
	Message string `json:"message,omitempty"`

	// Data contains additional trace data
	Data map[string]interface{} `json:"data,omitempty"`

	// Duration is the time taken by the operation (if applicable)
	Duration time.Duration `json:"duration,omitempty"`

	// Stack is the stack trace (if CaptureStack is enabled)
	Stack string `json:"stack,omitempty"`

	// Source is the source file and line number
	Source string `json:"source,omitempty"`
}

// DefaultMaxTraces is the default maximum number of traces to keep in memory
const DefaultMaxTraces = 1000

// CurrentDebugMode is the global debug mode state
var CurrentDebugMode = &DebugMode{
	Enabled:   false,
	Level:     DebugLevelInfo,
	maxTraces: DefaultMaxTraces,
}

// =============================================================================
// DEBUG MODE CONTROL
// =============================================================================

// EnableDebugMode activates debug mode with the specified settings
func EnableDebugMode(level DebugLevel, traceFile string) error {
	CurrentDebugMode.mu.Lock()
	defer CurrentDebugMode.mu.Unlock()

	CurrentDebugMode.Enabled = true
	CurrentDebugMode.Level = level
	CurrentDebugMode.TraceFile = traceFile
	CurrentDebugMode.StartTime = time.Now()
	CurrentDebugMode.SessionId = fmt.Sprintf("debug_%d", time.Now().UnixNano())
	CurrentDebugMode.traces = make([]TraceEntry, 0, DefaultMaxTraces)

	// Open trace file if specified
	if traceFile != "" {
		// Ensure directory exists
		dir := filepath.Dir(traceFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create trace directory: %w", err)
		}

		f, err := os.OpenFile(traceFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open trace file: %w", err)
		}
		CurrentDebugMode.traceFileHandle = f

		// Write session header
		header := fmt.Sprintf("\n=== DEBUG SESSION %s ===\nStarted: %s\nLevel: %s\n\n",
			CurrentDebugMode.SessionId,
			CurrentDebugMode.StartTime.Format(time.RFC3339),
			level.String())
		f.WriteString(header)
	}

	return nil
}

// DisableDebugMode deactivates debug mode
func DisableDebugMode() {
	CurrentDebugMode.mu.Lock()
	defer CurrentDebugMode.mu.Unlock()

	if CurrentDebugMode.traceFileHandle != nil {
		// Write session footer
		footer := fmt.Sprintf("\n=== END SESSION %s ===\nEnded: %s\nDuration: %s\nTraces: %d\n\n",
			CurrentDebugMode.SessionId,
			time.Now().Format(time.RFC3339),
			time.Since(CurrentDebugMode.StartTime),
			len(CurrentDebugMode.traces))
		CurrentDebugMode.traceFileHandle.WriteString(footer)
		CurrentDebugMode.traceFileHandle.Close()
		CurrentDebugMode.traceFileHandle = nil
	}

	CurrentDebugMode.Enabled = false
}

// IsDebugEnabled returns true if debug mode is active
func IsDebugEnabled() bool {
	CurrentDebugMode.mu.RLock()
	defer CurrentDebugMode.mu.RUnlock()
	return CurrentDebugMode.Enabled
}

// GetDebugLevel returns the current debug level
func GetDebugLevel() DebugLevel {
	CurrentDebugMode.mu.RLock()
	defer CurrentDebugMode.mu.RUnlock()
	return CurrentDebugMode.Level
}

// =============================================================================
// TRACE CAPTURE
// =============================================================================

// TraceStep records a trace entry for an execution step
func TraceStep(phase ExecutionPhase, operation string, data map[string]interface{}) {
	if !IsDebugEnabled() {
		return
	}

	trace := TraceEntry{
		Timestamp: time.Now(),
		Level:     DebugLevelDebug,
		Phase:     phase,
		Operation: operation,
		Data:      data,
	}

	addTrace(trace)
}

// TraceInfo records an informational trace
func TraceInfo(operation string, message string) {
	if !IsDebugEnabled() {
		return
	}

	trace := TraceEntry{
		Timestamp: time.Now(),
		Level:     DebugLevelInfo,
		Operation: operation,
		Message:   message,
	}

	addTrace(trace)
}

// TraceDebug records a debug-level trace
func TraceDebug(operation string, message string, data map[string]interface{}) {
	if !IsDebugEnabled() || GetDebugLevel() < DebugLevelDebug {
		return
	}

	trace := TraceEntry{
		Timestamp: time.Now(),
		Level:     DebugLevelDebug,
		Operation: operation,
		Message:   message,
		Data:      data,
	}

	addTrace(trace)
}

// TraceVerbose records a trace-level (most verbose) trace
func TraceVerbose(operation string, message string, data map[string]interface{}) {
	if !IsDebugEnabled() || GetDebugLevel() < DebugLevelTrace {
		return
	}

	trace := TraceEntry{
		Timestamp: time.Now(),
		Level:     DebugLevelTrace,
		Operation: operation,
		Message:   message,
		Data:      data,
	}

	addTrace(trace)
}

// TraceError records an error with trace information
func TraceError(operation string, err error, data map[string]interface{}) {
	if !IsDebugEnabled() {
		return
	}

	if data == nil {
		data = make(map[string]interface{})
	}
	data["error"] = err.Error()

	trace := TraceEntry{
		Timestamp: time.Now(),
		Level:     DebugLevelInfo, // Errors always show at info level
		Operation: operation,
		Message:   fmt.Sprintf("ERROR: %v", err),
		Data:      data,
	}

	// Always capture stack for errors
	trace.Stack = captureStack(3)
	trace.Source = getCaller(2)

	addTrace(trace)
}

// TraceDuration records a timed operation
func TraceDuration(operation string, start time.Time, data map[string]interface{}) {
	if !IsDebugEnabled() {
		return
	}

	duration := time.Since(start)

	trace := TraceEntry{
		Timestamp: time.Now(),
		Level:     DebugLevelDebug,
		Operation: operation,
		Duration:  duration,
		Message:   fmt.Sprintf("Completed in %v", duration),
		Data:      data,
	}

	addTrace(trace)
}

// addTrace adds a trace entry to the buffer and optionally writes to file
func addTrace(trace TraceEntry) {
	CurrentDebugMode.mu.Lock()
	defer CurrentDebugMode.mu.Unlock()

	// Add source info if in trace level
	if CurrentDebugMode.Level >= DebugLevelTrace && trace.Source == "" {
		trace.Source = getCaller(3)
	}

	// Capture stack if enabled and not already captured
	if CurrentDebugMode.CaptureStack && trace.Stack == "" && CurrentDebugMode.Level >= DebugLevelDebug {
		trace.Stack = captureStack(4)
	}

	// Ring buffer: remove oldest if at capacity
	if len(CurrentDebugMode.traces) >= CurrentDebugMode.maxTraces {
		CurrentDebugMode.traces = CurrentDebugMode.traces[1:]
	}

	CurrentDebugMode.traces = append(CurrentDebugMode.traces, trace)

	// Write to file if configured
	if CurrentDebugMode.traceFileHandle != nil {
		writeTraceToFile(CurrentDebugMode.traceFileHandle, trace)
	}

	// Print to stdout if trace level
	if CurrentDebugMode.Level >= DebugLevelTrace {
		printTrace(trace)
	}
}

// writeTraceToFile writes a trace entry to the trace file
func writeTraceToFile(f *os.File, trace TraceEntry) {
	line := formatTraceCompact(trace)
	f.WriteString(line + "\n")
}

// printTrace prints a trace entry to stdout
func printTrace(trace TraceEntry) {
	fmt.Println(formatTraceCompact(trace))
}

// formatTraceCompact returns a compact single-line format for traces
func formatTraceCompact(trace TraceEntry) string {
	var sb strings.Builder

	// Timestamp and level prefix
	sb.WriteString(fmt.Sprintf("[%s] [%s]",
		trace.Timestamp.Format("15:04:05.000"),
		trace.Level.Short()))

	// Phase if present
	if trace.Phase != "" {
		sb.WriteString(fmt.Sprintf(" [%s]", trace.Phase))
	}

	// Operation
	sb.WriteString(fmt.Sprintf(" %s", trace.Operation))

	// Message
	if trace.Message != "" {
		sb.WriteString(fmt.Sprintf(": %s", trace.Message))
	}

	// Duration
	if trace.Duration > 0 {
		sb.WriteString(fmt.Sprintf(" (%v)", trace.Duration))
	}

	// Data summary (just keys for compact format)
	if len(trace.Data) > 0 {
		keys := make([]string, 0, len(trace.Data))
		for k := range trace.Data {
			keys = append(keys, k)
		}
		sb.WriteString(fmt.Sprintf(" {%s}", strings.Join(keys, ", ")))
	}

	// Source
	if trace.Source != "" {
		sb.WriteString(fmt.Sprintf(" @ %s", trace.Source))
	}

	return sb.String()
}

// =============================================================================
// TRACE RETRIEVAL
// =============================================================================

// GetTraces returns all captured traces
func GetTraces() []TraceEntry {
	CurrentDebugMode.mu.RLock()
	defer CurrentDebugMode.mu.RUnlock()

	result := make([]TraceEntry, len(CurrentDebugMode.traces))
	copy(result, CurrentDebugMode.traces)
	return result
}

// GetTracesFiltered returns traces matching the filter criteria
func GetTracesFiltered(minLevel DebugLevel, phase ExecutionPhase, operation string) []TraceEntry {
	CurrentDebugMode.mu.RLock()
	defer CurrentDebugMode.mu.RUnlock()

	var result []TraceEntry
	for _, trace := range CurrentDebugMode.traces {
		if trace.Level < minLevel {
			continue
		}
		if phase != "" && trace.Phase != phase {
			continue
		}
		if operation != "" && !strings.Contains(trace.Operation, operation) {
			continue
		}
		result = append(result, trace)
	}

	return result
}

// ExportTraces exports traces in JSON format
func ExportTraces(sanitizeLevel SanitizeLevel) ([]byte, error) {
	traces := GetTraces()

	// Sanitize if requested
	if sanitizeLevel != SanitizeLevelNone {
		for i := range traces {
			traces[i].Message = SanitizeString(traces[i].Message, sanitizeLevel)
			traces[i].Operation = SanitizeString(traces[i].Operation, sanitizeLevel)
			if traces[i].Data != nil {
				traces[i].Data = SanitizeMap(traces[i].Data, sanitizeLevel)
			}
		}
	}

	return json.MarshalIndent(traces, "", "  ")
}

// ClearTraces clears all captured traces
func ClearTraces() {
	CurrentDebugMode.mu.Lock()
	defer CurrentDebugMode.mu.Unlock()

	CurrentDebugMode.traces = make([]TraceEntry, 0, CurrentDebugMode.maxTraces)
}

// =============================================================================
// ENVIRONMENT CAPTURE
// =============================================================================

// CaptureEnvironment captures sanitized environment information for debugging
func CaptureEnvironment(sanitizeLevel SanitizeLevel) map[string]interface{} {
	env := map[string]interface{}{
		"goVersion": runtime.Version(),
		"os":        runtime.GOOS,
		"arch":      runtime.GOARCH,
		"numCPU":    runtime.NumCPU(),
		"timestamp": time.Now().Format(time.RFC3339),
	}

	// Capture select environment variables (sanitized)
	safeEnvVars := []string{
		"SHELL",
		"TERM",
		"LANG",
		"HOME",
		"EDITOR",
		"PATH",
		"PLANDEX_DEBUG",
		"PLANDEX_API_HOST",
	}

	envVars := make(map[string]string)
	for _, key := range safeEnvVars {
		if val := os.Getenv(key); val != "" {
			envVars[key] = SanitizeString(val, sanitizeLevel)
		}
	}
	env["envVars"] = envVars

	// Memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	env["memory"] = map[string]interface{}{
		"allocMB":      memStats.Alloc / 1024 / 1024,
		"totalAllocMB": memStats.TotalAlloc / 1024 / 1024,
		"sysMB":        memStats.Sys / 1024 / 1024,
		"numGC":        memStats.NumGC,
	}

	return env
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// captureStack captures the current stack trace
func captureStack(skip int) string {
	const maxFrames = 20
	pcs := make([]uintptr, maxFrames)
	n := runtime.Callers(skip, pcs)
	if n == 0 {
		return ""
	}

	frames := runtime.CallersFrames(pcs[:n])
	var sb strings.Builder

	for {
		frame, more := frames.Next()
		// Skip runtime internals
		if strings.Contains(frame.File, "runtime/") {
			if !more {
				break
			}
			continue
		}

		sb.WriteString(fmt.Sprintf("  %s\n    %s:%d\n",
			frame.Function, frame.File, frame.Line))

		if !more {
			break
		}
	}

	return sb.String()
}

// getCaller returns the source file and line of the caller
func getCaller(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

// =============================================================================
// DEBUG LEVEL HELPERS
// =============================================================================

// ParseDebugLevel parses a string into a DebugLevel
func ParseDebugLevel(s string) DebugLevel {
	switch strings.ToLower(s) {
	case "info", "0":
		return DebugLevelInfo
	case "debug", "1":
		return DebugLevelDebug
	case "trace", "2":
		return DebugLevelTrace
	default:
		return DebugLevelDebug
	}
}

// String returns the string representation of a DebugLevel
func (l DebugLevel) String() string {
	switch l {
	case DebugLevelInfo:
		return "info"
	case DebugLevelDebug:
		return "debug"
	case DebugLevelTrace:
		return "trace"
	default:
		return "unknown"
	}
}

// Short returns a short string representation of a DebugLevel
func (l DebugLevel) Short() string {
	switch l {
	case DebugLevelInfo:
		return "INFO"
	case DebugLevelDebug:
		return "DEBUG"
	case DebugLevelTrace:
		return "TRACE"
	default:
		return "???"
	}
}

// =============================================================================
// DEBUG MODE STATE
// =============================================================================

// GetDebugModeState returns the current debug mode configuration
func GetDebugModeState() map[string]interface{} {
	CurrentDebugMode.mu.RLock()
	defer CurrentDebugMode.mu.RUnlock()

	state := map[string]interface{}{
		"enabled":      CurrentDebugMode.Enabled,
		"level":        CurrentDebugMode.Level.String(),
		"captureStack": CurrentDebugMode.CaptureStack,
		"captureEnv":   CurrentDebugMode.CaptureEnv,
		"traceCount":   len(CurrentDebugMode.traces),
	}

	if CurrentDebugMode.Enabled {
		state["sessionId"] = CurrentDebugMode.SessionId
		state["startTime"] = CurrentDebugMode.StartTime.Format(time.RFC3339)
		state["duration"] = time.Since(CurrentDebugMode.StartTime).String()
	}

	if CurrentDebugMode.TraceFile != "" {
		state["traceFile"] = CurrentDebugMode.TraceFile
	}

	return state
}

// SetDebugOptions sets additional debug options
func SetDebugOptions(captureStack, captureEnv bool) {
	CurrentDebugMode.mu.Lock()
	defer CurrentDebugMode.mu.Unlock()

	CurrentDebugMode.CaptureStack = captureStack
	CurrentDebugMode.CaptureEnv = captureEnv
}
