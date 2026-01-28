package shared

import (
	"encoding/json"
	"fmt"
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

// Step represents a single unit of work in plan execution.
type Step struct {
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
	ParentID string `json:"parentId,omitempty"`
	Children []Step `json:"children,omitempty"`

	// Metrics
	TokensProcessed int `json:"tokensProcessed,omitempty"`
	BytesProcessed  int `json:"bytesProcessed,omitempty"`
}

// Duration returns the elapsed time for this step.
func (s *Step) Duration() time.Duration {
	if s.StartedAt.IsZero() {
		return 0
	}
	if s.CompletedAt.IsZero() {
		return time.Since(s.StartedAt)
	}
	return s.CompletedAt.Sub(s.StartedAt)
}

// IsStalled returns true if the step has been running longer than expected.
func (s *Step) IsStalled() bool {
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
	Steps         []Step `json:"steps"`
	CurrentStepID string `json:"currentStepId,omitempty"`

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
func (r *ProgressReport) CurrentStep() *Step {
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
	Step      *Step               `json:"step,omitempty"`
	Phase     ProgressPhase      `json:"phase,omitempty"`
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
