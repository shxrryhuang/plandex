package shared

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// StepState represents the current state of an execution step.
// States are designed to clearly distinguish between guaranteed and best-effort information.
type StepState string

const (
	// Guaranteed states - these reflect committed, durable state changes
	StepStatePending   StepState = "pending"   // Step is queued but not started
	StepStateCompleted StepState = "completed" // Step finished successfully (guaranteed)
	StepStateFailed    StepState = "failed"    // Step failed with error (guaranteed)
	StepStateSkipped   StepState = "skipped"   // Step was skipped (guaranteed)

	// Best-effort states - these are signals about current activity, not guarantees
	StepStateRunning StepState = "running" // Step is actively executing (best-effort)
	StepStateWaiting StepState = "waiting" // Step is waiting on external resource (best-effort)
	StepStateStalled StepState = "stalled" // Step appears stuck, may need intervention (best-effort)
)

// IsGuaranteed returns true if this state represents a committed, durable change.
func (s StepState) IsGuaranteed() bool {
	switch s {
	case StepStateCompleted, StepStateFailed, StepStateSkipped:
		return true
	default:
		return false
	}
}

// IsBestEffort returns true if this state is a signal about current activity.
func (s StepState) IsBestEffort() bool {
	switch s {
	case StepStateRunning, StepStateWaiting, StepStateStalled:
		return true
	default:
		return false
	}
}

// IsTerminal returns true if no further state changes are expected.
func (s StepState) IsTerminal() bool {
	switch s {
	case StepStateCompleted, StepStateFailed, StepStateSkipped:
		return true
	default:
		return false
	}
}

// StepKind categorizes the type of operation being performed.
type StepKind string

const (
	StepKindLLMCall     StepKind = "llm_call"     // LLM API request
	StepKindFileRead    StepKind = "file_read"    // Reading file from disk
	StepKindFileWrite   StepKind = "file_write"   // Writing file to disk
	StepKindFileBuild   StepKind = "file_build"   // Building/generating file content
	StepKindToolExec    StepKind = "tool_exec"    // Executing external tool/command
	StepKindValidation  StepKind = "validation"   // Validating output/state
	StepKindContext     StepKind = "context"      // Loading context
	StepKindNetwork     StepKind = "network"      // Network operation
	StepKindUserInput   StepKind = "user_input"   // Waiting for user input
	StepKindInternal    StepKind = "internal"     // Internal processing
)

// ExpectedDuration returns the typical duration for this step kind.
// Used for stall detection and progress estimation.
func (k StepKind) ExpectedDuration() time.Duration {
	switch k {
	case StepKindLLMCall:
		return 30 * time.Second // LLM calls can be slow
	case StepKindFileRead:
		return 1 * time.Second
	case StepKindFileWrite:
		return 2 * time.Second
	case StepKindFileBuild:
		return 60 * time.Second // Building can involve LLM
	case StepKindToolExec:
		return 30 * time.Second
	case StepKindValidation:
		return 5 * time.Second
	case StepKindContext:
		return 10 * time.Second
	case StepKindNetwork:
		return 15 * time.Second
	case StepKindUserInput:
		return 0 // No timeout for user input
	default:
		return 10 * time.Second
	}
}

// ProgressStep represents a single unit of work in plan execution (ProgressReport system).
type ProgressStep struct {
	ID          string    `json:"id"`
	Kind        StepKind  `json:"kind"`
	State       StepState `json:"state"`
	Label       string    `json:"label"`       // Human-readable description
	Detail      string    `json:"detail"`      // Additional context (file path, token count, etc.)
	StartedAt   time.Time `json:"startedAt,omitempty"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
	Error       string    `json:"error,omitempty"`

	// Progress tracking for long-running operations
	Progress    float64 `json:"progress,omitempty"` // 0.0 to 1.0, -1 for indeterminate
	ProgressMsg string  `json:"progressMsg,omitempty"`

	// For nested operations
	ParentID string              `json:"parentId,omitempty"`
	Children []ProgressStep      `json:"children,omitempty"`

	// Metrics
	TokensProcessed int `json:"tokensProcessed,omitempty"`
	BytesProcessed  int `json:"bytesProcessed,omitempty"`
}

// Duration returns the elapsed time for this step.
func (s *ProgressStep) Duration() time.Duration {
	if s.StartedAt.IsZero() {
		return 0
	}
	if s.CompletedAt.IsZero() {
		return time.Since(s.StartedAt)
	}
	return s.CompletedAt.Sub(s.StartedAt)
}

// IsStalled returns true if the step has been running longer than expected.
func (s *ProgressStep) IsStalled() bool {
	if s.State != StepStateRunning && s.State != StepStateWaiting {
		return false
	}
	expected := s.Kind.ExpectedDuration()
	if expected == 0 {
		return false // No timeout
	}
	return s.Duration() > expected*2 // Consider stalled at 2x expected
}

// ProgressPhase represents a major phase of plan execution for progress tracking.
// Note: This is distinct from ProgressPhase in error_report.go which tracks error context.
type ProgressPhase string

const (
	PhaseInitializing ProgressPhase = "initializing" // Setting up context
	PhasePlanning     ProgressPhase = "planning"     // LLM reasoning about task
	PhaseDescribing   ProgressPhase = "describing"   // LLM describing changes
	PhaseBuilding     ProgressPhase = "building"     // Generating file content
	PhaseApplying     ProgressPhase = "applying"     // Applying changes
	PhaseValidating   ProgressPhase = "validating"   // Validating results
	PhaseCompleted    ProgressPhase = "completed"    // Execution finished
	PhaseFailed       ProgressPhase = "failed"       // Execution failed
	PhaseStopped      ProgressPhase = "stopped"      // User stopped execution
)

// PhaseOrder returns the expected order of this phase (lower = earlier).
func (p ProgressPhase) PhaseOrder() int {
	switch p {
	case PhaseInitializing:
		return 0
	case PhasePlanning:
		return 1
	case PhaseDescribing:
		return 2
	case PhaseBuilding:
		return 3
	case PhaseApplying:
		return 4
	case PhaseValidating:
		return 5
	case PhaseCompleted:
		return 6
	case PhaseFailed, PhaseStopped:
		return 7
	default:
		return 99
	}
}

// ProgressReport represents the current state of plan execution.
type ProgressReport struct {
	PlanID      string         `json:"planId"`
	Branch      string         `json:"branch"`
	Phase       ProgressPhase `json:"phase"`
	PhaseLabel  string         `json:"phaseLabel"`
	StartedAt   time.Time      `json:"startedAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	CompletedAt time.Time      `json:"completedAt,omitempty"`

	// Step tracking
	Steps         []ProgressStep `json:"steps"`
	CurrentStepID string         `json:"currentStepId,omitempty"`

	// Summary counts
	TotalSteps     int `json:"totalSteps"`
	CompletedSteps int `json:"completedSteps"`
	FailedSteps    int `json:"failedSteps"`
	SkippedSteps   int `json:"skippedSteps"`

	// Aggregate metrics
	TotalTokens int           `json:"totalTokens,omitempty"`
	TotalBytes  int           `json:"totalBytes,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`

	// Warnings and diagnostics
	Warnings   []string `json:"warnings,omitempty"`
	StalledIDs []string `json:"stalledIds,omitempty"` // IDs of potentially stalled steps

	// User guidance
	CanCancel    bool   `json:"canCancel"`
	CanRetry     bool   `json:"canRetry"`
	SuggestedAction string `json:"suggestedAction,omitempty"`
}

// CurrentStep returns the currently active step, if any.
func (r *ProgressReport) CurrentStep() *ProgressStep {
	for i := range r.Steps {
		if r.Steps[i].ID == r.CurrentStepID {
			return &r.Steps[i]
		}
	}
	return nil
}

// UpdateCounts recalculates summary counts from steps.
func (r *ProgressReport) UpdateCounts() {
	r.TotalSteps = len(r.Steps)
	r.CompletedSteps = 0
	r.FailedSteps = 0
	r.SkippedSteps = 0
	r.StalledIDs = nil
	r.TotalTokens = 0
	r.TotalBytes = 0

	for _, step := range r.Steps {
		switch step.State {
		case StepStateCompleted:
			r.CompletedSteps++
		case StepStateFailed:
			r.FailedSteps++
		case StepStateSkipped:
			r.SkippedSteps++
		}
		if step.IsStalled() {
			r.StalledIDs = append(r.StalledIDs, step.ID)
		}
		r.TotalTokens += step.TokensProcessed
		r.TotalBytes += step.BytesProcessed
	}

	r.UpdatedAt = time.Now()
}

// SetSuggestedAction updates the user guidance based on current state.
func (r *ProgressReport) SetSuggestedAction() {
	if len(r.StalledIDs) > 0 {
		r.SuggestedAction = "Operation appears stalled. Consider canceling (s) if no progress."
		return
	}

	current := r.CurrentStep()
	if current == nil {
		return
	}

	switch current.Kind {
	case StepKindLLMCall:
		if current.Duration() > 20*time.Second {
			r.SuggestedAction = "LLM is processing. Large tasks may take time."
		}
	case StepKindUserInput:
		r.SuggestedAction = "Waiting for your input."
	case StepKindToolExec:
		if current.Duration() > 30*time.Second {
			r.SuggestedAction = "External tool running. Cancel (s) if stuck."
		}
	}
}

// ToJSON serializes the progress report for logging.
func (r *ProgressReport) ToJSON() string {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"error": "marshal failed: %s"}`, err.Error())
	}
	return string(data)
}

// ProgressMessage is sent over the stream to update progress state.
type ProgressMessage struct {
	Type      ProgressMessageType `json:"type"`
	Step      *ProgressStep       `json:"step,omitempty"`
	Phase     ProgressPhase       `json:"phase,omitempty"`
	Report    *ProgressReport     `json:"report,omitempty"`
	Timestamp time.Time           `json:"timestamp"`
}

type ProgressMessageType string

const (
	ProgressMsgStepStart   ProgressMessageType = "step_start"
	ProgressMsgStepUpdate  ProgressMessageType = "step_update"
	ProgressMsgStepEnd     ProgressMessageType = "step_end"
	ProgressMsgPhaseChange ProgressMessageType = "phase_change"
	ProgressMsgFullReport  ProgressMessageType = "full_report"
	ProgressMsgHeartbeat   ProgressMessageType = "heartbeat"
)

// =============================================================================
// PROGRESS MODEL
//
// Two-tier confidence model for CLI progress reporting:
//
//   Guaranteed State  — steps that have completed or failed. These are facts
//                       sourced from server confirmations. They never regress.
//
//   Best-Effort       — steps currently running or waiting. The system believes
//                       they are in progress, but has not yet received a terminal
//                       confirmation. A heartbeat timeout or connection drop can
//                       cause these to revert to "stalled".
//
// This distinction lets the user make informed decisions:
//   • Guaranteed = safe to act on (logs, retries, etc.)
//   • Best-effort = informational only; treat as "probably fine"
//
// =============================================================================

// PhaseID identifies a top-level execution phase.
type PhaseID string

const (
	PhaseConnect      PhaseID = "connect"       // Establishing server connection
	PhaseContext      PhaseID = "context"       // Loading / auto-loading context files
	PhaseModel        PhaseID = "model"         // Sending prompt, receiving streamed reply
	PhaseBuild        PhaseID = "build"         // Generating file edits from reply
	PhaseApply        PhaseID = "apply"         // Writing files / running commands
	PhaseFinalize     PhaseID = "finalize"      // Cleanup, summary generation
)

// AllPhases is the canonical ordering of execution phases.
var AllPhases = []PhaseID{
	PhaseConnect,
	PhaseContext,
	PhaseModel,
	PhaseBuild,
	PhaseApply,
	PhaseFinalize,
}

// PhaseLabel returns a human-readable label for a phase.
func (p PhaseID) PhaseLabel() string {
	switch p {
	case PhaseConnect:
		return "connect"
	case PhaseContext:
		return "context"
	case PhaseModel:
		return "model"
	case PhaseBuild:
		return "build"
	case PhaseApply:
		return "apply"
	case PhaseFinalize:
		return "finalize"
	default:
		return string(p)
	}
}

// StepStatus represents the execution state of a single step.
type StepStatus string

const (
	StepPending   StepStatus = "pending"   // Not yet started
	StepRunning   StepStatus = "running"   // In progress (best-effort)
	StepCompleted StepStatus = "completed" // Confirmed finished (guaranteed)
	StepFailed    StepStatus = "failed"    // Confirmed failed (guaranteed)
	StepSkipped   StepStatus = "skipped"   // Intentionally skipped
	StepStalled   StepStatus = "stalled"   // Was running but no recent heartbeat
)

// IsTerminal returns true if the status is a final state (guaranteed).
func (s StepStatus) IsTerminal() bool {
	return s == StepCompleted || s == StepFailed || s == StepSkipped
}

// IsGuaranteed returns true if this status represents confirmed server state.
func (s StepStatus) IsGuaranteed() bool {
	return s == StepCompleted || s == StepFailed || s == StepSkipped
}

// Confidence indicates how reliable a status observation is.
type Confidence int

const (
	ConfidenceGuaranteed Confidence = iota // Server confirmed; immutable
	ConfidenceBestEffort                   // Client-side inference; may change
)

// Step is a single unit of work within a phase.
type Step struct {
	ID          string     `json:"id"`
	Phase       PhaseID    `json:"phase"`
	Label       string     `json:"label"`
	Status      StepStatus `json:"status"`
	Confidence  Confidence `json:"confidence"`
	Detail      string     `json:"detail,omitempty"`   // e.g. file path, token count
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
	DurationMs  int64      `json:"durationMs,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// Duration returns elapsed time for running steps, or recorded duration for
// completed steps.
func (s *Step) Duration() time.Duration {
	if s.FinishedAt != nil && s.StartedAt != nil {
		return s.FinishedAt.Sub(*s.StartedAt)
	}
	if s.StartedAt != nil {
		return time.Since(*s.StartedAt)
	}
	return 0
}

// Progress is the full snapshot of execution state at a point in time.
// The CLI renders this into terminal output; structured loggers serialize it
// directly.
type Progress struct {
	// Current phase the plan is executing
	ActivePhase PhaseID `json:"activePhase"`

	// Ordered list of all steps recorded so far
	Steps []*Step `json:"steps"`

	// Wall-clock start of the entire run
	StartedAt time.Time `json:"startedAt"`

	// Last heartbeat received from the server (nil = no heartbeat yet)
	LastHeartbeat *time.Time `json:"lastHeartbeat,omitempty"`

	// Terminal status if the run is done
	Finished bool   `json:"finished"`
	Error    string `json:"error,omitempty"`
}

// NewProgress creates a fresh Progress snapshot at the current time.
func NewProgress() *Progress {
	return &Progress{
		ActivePhase: PhaseConnect,
		Steps:       make([]*Step, 0, 16),
		StartedAt:   time.Now(),
	}
}

// AddStep appends a new step in the given phase with the given label.
// The step starts as Pending with BestEffort confidence.
func (p *Progress) AddStep(phase PhaseID, label, detail string) *Step {
	s := &Step{
		ID:         fmt.Sprintf("%s:%d", phase, len(p.Steps)),
		Phase:      phase,
		Label:      label,
		Status:     StepPending,
		Confidence: ConfidenceBestEffort,
		Detail:     detail,
	}
	p.Steps = append(p.Steps, s)
	return s
}

// StartStep marks a step as running, records the start time.
func (p *Progress) StartStep(step *Step) {
	now := time.Now()
	step.Status = StepRunning
	step.Confidence = ConfidenceBestEffort
	step.StartedAt = &now
	p.ActivePhase = step.Phase
}

// CompleteStep marks a step as completed with guaranteed confidence.
func (p *Progress) CompleteStep(step *Step) {
	now := time.Now()
	step.Status = StepCompleted
	step.Confidence = ConfidenceGuaranteed
	step.FinishedAt = &now
	if step.StartedAt != nil {
		step.DurationMs = now.Sub(*step.StartedAt).Milliseconds()
	}
}

// FailStep marks a step as failed with guaranteed confidence.
func (p *Progress) FailStep(step *Step, errMsg string) {
	now := time.Now()
	step.Status = StepFailed
	step.Confidence = ConfidenceGuaranteed
	step.FinishedAt = &now
	step.Error = errMsg
	if step.StartedAt != nil {
		step.DurationMs = now.Sub(*step.StartedAt).Milliseconds()
	}
	p.Error = errMsg
}

// SkipStep marks a step as intentionally skipped.  The reason is stored in
// Error (not Detail) so that a pre-existing Detail (e.g. a file path) is
// preserved for the renderer.
func (p *Progress) SkipStep(step *Step, reason string) {
	now := time.Now()
	step.Status = StepSkipped
	step.Confidence = ConfidenceGuaranteed
	step.FinishedAt = &now
	step.Error = reason
}

// MarkStalled transitions any running step in the given phase to stalled.
// Called when a heartbeat timeout fires for that phase.
func (p *Progress) MarkStalled(phase PhaseID) {
	for _, s := range p.Steps {
		if s.Phase == phase && s.Status == StepRunning {
			s.Status = StepStalled
			// Confidence stays BestEffort — we don't actually know if it failed
		}
	}
}

// RecordHeartbeat updates the last-seen heartbeat timestamp.
func (p *Progress) RecordHeartbeat() {
	now := time.Now()
	p.LastHeartbeat = &now
}

// ActiveSteps returns steps whose status is Running or Stalled.
func (p *Progress) ActiveSteps() []*Step {
	var out []*Step
	for _, s := range p.Steps {
		if s.Status == StepRunning || s.Status == StepStalled {
			out = append(out, s)
		}
	}
	return out
}

// CompletedSteps returns steps with guaranteed terminal status.
func (p *Progress) CompletedSteps() []*Step {
	var out []*Step
	for _, s := range p.Steps {
		if s.Status == StepCompleted {
			out = append(out, s)
		}
	}
	return out
}

// FailedSteps returns steps that failed.
func (p *Progress) FailedSteps() []*Step {
	var out []*Step
	for _, s := range p.Steps {
		if s.Status == StepFailed {
			out = append(out, s)
		}
	}
	return out
}

// PhaseSteps returns all steps belonging to the given phase, preserving order.
func (p *Progress) PhaseSteps(phase PhaseID) []*Step {
	var out []*Step
	for _, s := range p.Steps {
		if s.Phase == phase {
			out = append(out, s)
		}
	}
	return out
}

// Summary returns a one-line human-readable summary of progress.
// Running and stalled steps are reported separately so the user is not
// misled into thinking a stalled operation is actively making progress.
func (p *Progress) Summary() string {
	total := len(p.Steps)
	done := len(p.CompletedSteps())
	failed := len(p.FailedSteps())

	var running, stalled int
	for _, s := range p.Steps {
		switch s.Status {
		case StepRunning:
			running++
		case StepStalled:
			stalled++
		}
	}

	parts := []string{}
	if done > 0 {
		parts = append(parts, fmt.Sprintf("%d done", done))
	}
	if running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", running))
	}
	if stalled > 0 {
		parts = append(parts, fmt.Sprintf("%d stalled", stalled))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if total > done+running+stalled+failed {
		pending := total - done - running - stalled - failed
		parts = append(parts, fmt.Sprintf("%d pending", pending))
	}

	elapsed := time.Since(p.StartedAt)
	if elapsed < time.Second {
		elapsed = 0
	}
	elapsedStr := formatProgressDuration(elapsed)

	summary := strings.Join(parts, " | ")
	if summary == "" {
		summary = "starting"
	}
	return fmt.Sprintf("[%s] %s (%s elapsed)", p.ActivePhase.PhaseLabel(), summary, elapsedStr)
}

// formatProgressDuration produces a compact duration string.
func formatProgressDuration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) - m*60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) - h*60
	return fmt.Sprintf("%dh%dm", h, m)
}
