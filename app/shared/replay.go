package shared

import (
	"time"
)

// ReplaySessionStatus represents the current status of a replay session
type ReplaySessionStatus string

const (
	ReplaySessionStatusRecording ReplaySessionStatus = "recording"
	ReplaySessionStatusCompleted ReplaySessionStatus = "completed"
	ReplaySessionStatusFailed    ReplaySessionStatus = "failed"
	ReplaySessionStatusReplaying ReplaySessionStatus = "replaying"
	ReplaySessionStatusPaused    ReplaySessionStatus = "paused"
)

// ReplayStepType identifies what type of step this is
type ReplayStepType string

const (
	ReplayStepTypeModelRequest      ReplayStepType = "model_request"
	ReplayStepTypeModelResponse     ReplayStepType = "model_response"
	ReplayStepTypeFileDiff          ReplayStepType = "file_diff"
	ReplayStepTypeFileWrite         ReplayStepType = "file_write"
	ReplayStepTypeFileRemove        ReplayStepType = "file_remove"
	ReplayStepTypeFileMove          ReplayStepType = "file_move"
	ReplayStepTypeContextLoad       ReplayStepType = "context_load"
	ReplayStepTypeContextUpdate     ReplayStepType = "context_update"
	ReplayStepTypeBuildStart        ReplayStepType = "build_start"
	ReplayStepTypeBuildComplete     ReplayStepType = "build_complete"
	ReplayStepTypeBuildError        ReplayStepType = "build_error"
	ReplayStepTypeSubtaskStart      ReplayStepType = "subtask_start"
	ReplayStepTypeSubtaskComplete   ReplayStepType = "subtask_complete"
	ReplayStepTypePlanningPhase     ReplayStepType = "planning_phase"
	ReplayStepTypeImplementation    ReplayStepType = "implementation"
	ReplayStepTypeUserPrompt        ReplayStepType = "user_prompt"
	ReplayStepTypeMissingFilePrompt ReplayStepType = "missing_file_prompt"
	ReplayStepTypeError             ReplayStepType = "error"
)

// ReplayStepStatus represents the status of a single step during replay
type ReplayStepStatus string

const (
	ReplayStepStatusPending   ReplayStepStatus = "pending"
	ReplayStepStatusRunning   ReplayStepStatus = "running"
	ReplayStepStatusCompleted ReplayStepStatus = "completed"
	ReplayStepStatusSkipped   ReplayStepStatus = "skipped"
	ReplayStepStatusFailed    ReplayStepStatus = "failed"
)

// ReplayMode controls how the replay behaves
type ReplayMode string

const (
	// ReplayModeReadOnly inspects without making changes (default, safe)
	ReplayModeReadOnly ReplayMode = "read_only"
	// ReplayModeSimulate simulates changes without writing to disk
	ReplayModeSimulate ReplayMode = "simulate"
	// ReplayModeApply actually applies changes (requires explicit opt-in)
	ReplayModeApply ReplayMode = "apply"
)

// ReplayFileSnapshot captures the state of a file at a point in time
type ReplayFileSnapshot struct {
	Path        string    `json:"path"`
	Content     string    `json:"content,omitempty"`
	ContentHash string    `json:"contentHash,omitempty"`
	Size        int64     `json:"size"`
	Exists      bool      `json:"exists"`
	CapturedAt  time.Time `json:"capturedAt"`
}

// ReplayDiff represents a diff between two file states
type ReplayDiff struct {
	Path        string   `json:"path"`
	OldContent  string   `json:"oldContent,omitempty"`
	NewContent  string   `json:"newContent,omitempty"`
	UnifiedDiff string   `json:"unifiedDiff,omitempty"`
	Hunks       []string `json:"hunks,omitempty"`
}

// ReplayModelRequest captures the inputs to a model request
type ReplayModelRequest struct {
	ModelName       string                 `json:"modelName"`
	ModelId         string                 `json:"modelId"`
	Provider        string                 `json:"provider"`
	Temperature     float32                `json:"temperature,omitempty"`
	TopP            float32                `json:"topP,omitempty"`
	MaxTokens       int                    `json:"maxTokens,omitempty"`
	InputTokens     int                    `json:"inputTokens"`
	SystemPrompt    string                 `json:"systemPrompt,omitempty"`
	Messages        []ReplayMessage        `json:"messages,omitempty"`
	Stop            []string               `json:"stop,omitempty"`
	RequestMetadata map[string]interface{} `json:"requestMetadata,omitempty"`
}

// ReplayMessage represents a single message in the conversation
type ReplayMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ReplayModelResponse captures the model's response
type ReplayModelResponse struct {
	Content           string `json:"content"`
	FinishReason      string `json:"finishReason,omitempty"`
	InputTokens       int    `json:"inputTokens"`
	OutputTokens      int    `json:"outputTokens"`
	TotalTokens       int    `json:"totalTokens"`
	CachedInputTokens int    `json:"cachedInputTokens,omitempty"`
	Stopped           bool   `json:"stopped"`
	Error             string `json:"error,omitempty"`
}

// ReplayBuildInfo captures build operation details
type ReplayBuildInfo struct {
	Path           string             `json:"path"`
	NumTokens      int                `json:"numTokens"`
	Finished       bool               `json:"finished"`
	Removed        bool               `json:"removed,omitempty"`
	Success        bool               `json:"success"`
	Error          string             `json:"error,omitempty"`
	Replacements   []*Replacement     `json:"replacements,omitempty"`
	BeforeSnapshot ReplayFileSnapshot `json:"beforeSnapshot,omitempty"`
	AfterSnapshot  ReplayFileSnapshot `json:"afterSnapshot,omitempty"`
	Diff           *ReplayDiff        `json:"diff,omitempty"`
}

// ReplayStep represents a single step in the replay session
type ReplayStep struct {
	Id          string           `json:"id"`
	StepNumber  int              `json:"stepNumber"`
	Type        ReplayStepType   `json:"type"`
	Status      ReplayStepStatus `json:"status"`
	Description string           `json:"description"`
	StartedAt   time.Time        `json:"startedAt"`
	CompletedAt *time.Time       `json:"completedAt,omitempty"`
	DurationMs  int64            `json:"durationMs,omitempty"`

	// Type-specific data (only one will be populated based on Type)
	ModelRequest  *ReplayModelRequest  `json:"modelRequest,omitempty"`
	ModelResponse *ReplayModelResponse `json:"modelResponse,omitempty"`
	BuildInfo     *ReplayBuildInfo     `json:"buildInfo,omitempty"`
	FileDiff      *ReplayDiff          `json:"fileDiff,omitempty"`
	ContextFiles  []string             `json:"contextFiles,omitempty"`
	Subtask       *Subtask             `json:"subtask,omitempty"`
	UserPrompt    string               `json:"userPrompt,omitempty"`
	Error         string               `json:"error,omitempty"`

	// File snapshots for this step
	FileSnapshots map[string]*ReplayFileSnapshot `json:"fileSnapshots,omitempty"`

	// Metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ReplaySession represents a complete recorded session
type ReplaySession struct {
	Id            string              `json:"id"`
	OrgId         string              `json:"orgId"`
	PlanId        string              `json:"planId"`
	Branch        string              `json:"branch"`
	UserId        string              `json:"userId"`
	Status        ReplaySessionStatus `json:"status"`
	InitialPrompt string              `json:"initialPrompt"`
	Steps         []*ReplayStep       `json:"steps"`
	TotalSteps    int                 `json:"totalSteps"`
	CurrentStep   int                 `json:"currentStep"`

	// Timing
	StartedAt       time.Time  `json:"startedAt"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
	TotalDurationMs int64      `json:"totalDurationMs,omitempty"`

	// Initial state snapshots
	InitialFileSnapshots map[string]*ReplayFileSnapshot `json:"initialFileSnapshots,omitempty"`
	InitialContexts      []string                       `json:"initialContexts,omitempty"`

	// Model usage summary
	TotalInputTokens  int `json:"totalInputTokens"`
	TotalOutputTokens int `json:"totalOutputTokens"`
	TotalModelCalls   int `json:"totalModelCalls"`

	// Error tracking
	Errors []string `json:"errors,omitempty"`

	// Metadata
	PlanConfig *PlanConfig            `json:"planConfig,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`

	// Version for compatibility
	Version string `json:"version"`
}

// ReplaySessionSummary is a lightweight summary for listing sessions
type ReplaySessionSummary struct {
	Id              string              `json:"id"`
	PlanId          string              `json:"planId"`
	Branch          string              `json:"branch"`
	Status          ReplaySessionStatus `json:"status"`
	InitialPrompt   string              `json:"initialPrompt"`
	TotalSteps      int                 `json:"totalSteps"`
	StartedAt       time.Time           `json:"startedAt"`
	CompletedAt     *time.Time          `json:"completedAt,omitempty"`
	TotalDurationMs int64               `json:"totalDurationMs,omitempty"`
	HasErrors       bool                `json:"hasErrors"`
}

// ReplayExecutionState tracks the current state during replay execution
type ReplayExecutionState struct {
	SessionId      string                         `json:"sessionId"`
	Mode           ReplayMode                     `json:"mode"`
	CurrentStepIdx int                            `json:"currentStepIdx"`
	IsPaused       bool                           `json:"isPaused"`
	AutoAdvance    bool                           `json:"autoAdvance"`
	StepDelayMs    int                            `json:"stepDelayMs"`
	AppliedChanges []string                       `json:"appliedChanges,omitempty"`
	SkippedSteps   []int                          `json:"skippedSteps,omitempty"`
	Divergences    []ReplayDivergence             `json:"divergences,omitempty"`
	CurrentFiles   map[string]*ReplayFileSnapshot `json:"currentFiles,omitempty"`
}

// ReplayDivergence records where replay differs from original
type ReplayDivergence struct {
	StepNumber  int       `json:"stepNumber"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Expected    string    `json:"expected,omitempty"`
	Actual      string    `json:"actual,omitempty"`
	DetectedAt  time.Time `json:"detectedAt"`
}

// ReplayOptions configures replay behavior
type ReplayOptions struct {
	Mode            ReplayMode `json:"mode"`
	StartFromStep   int        `json:"startFromStep,omitempty"`
	EndAtStep       int        `json:"endAtStep,omitempty"`
	PauseBeforeStep []int      `json:"pauseBeforeStep,omitempty"`
	PauseAfterStep  []int      `json:"pauseAfterStep,omitempty"`
	SkipSteps       []int      `json:"skipSteps,omitempty"`
	AutoAdvance     bool       `json:"autoAdvance"`
	StepDelayMs     int        `json:"stepDelayMs,omitempty"`

	// CaptureSnapshots controls whether the replay engine validates that
	// per-file snapshots (.snapshot + .meta.json) exist on disk for every
	// file write step.  The underlying FileTransaction always persists
	// snapshots before writing — this flag does not control that behaviour.
	// When true the replay engine additionally verifies snapshot integrity
	// (hash match, completeness) and reports divergence if a snapshot is
	// missing or corrupt.  Default: true.
	CaptureSnapshots bool `json:"captureSnapshots"`

	ValidateChecksums bool `json:"validateChecksums"`
	StopOnDivergence  bool `json:"stopOnDivergence"`
}

// ReplayStepResult is returned after executing a single step
type ReplayStepResult struct {
	StepNumber  int               `json:"stepNumber"`
	Status      ReplayStepStatus  `json:"status"`
	Step        *ReplayStep       `json:"step"`
	Divergence  *ReplayDivergence `json:"divergence,omitempty"`
	FileChanges []*ReplayDiff     `json:"fileChanges,omitempty"`
	Error       string            `json:"error,omitempty"`
	ExecutionMs int64             `json:"executionMs"`
}

// API Request/Response types

// ListReplaySessionsRequest for listing replay sessions
type ListReplaySessionsRequest struct {
	PlanId string `json:"planId,omitempty"`
	Branch string `json:"branch,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// ListReplaySessionsResponse returns list of sessions
type ListReplaySessionsResponse struct {
	Sessions   []*ReplaySessionSummary `json:"sessions"`
	TotalCount int                     `json:"totalCount"`
}

// GetReplaySessionRequest for getting a specific session
type GetReplaySessionRequest struct {
	SessionId    string `json:"sessionId"`
	IncludeSteps bool   `json:"includeSteps"`
}

// StartReplayRequest for starting a replay
type StartReplayRequest struct {
	SessionId string         `json:"sessionId"`
	Options   *ReplayOptions `json:"options,omitempty"`
}

// StartReplayResponse returns initial replay state
type StartReplayResponse struct {
	State   *ReplayExecutionState `json:"state"`
	Session *ReplaySession        `json:"session"`
}

// ReplayStepRequest for executing the next step
type ReplayStepRequest struct {
	SessionId  string `json:"sessionId"`
	StepNumber int    `json:"stepNumber,omitempty"` // If specified, jump to this step
	Action     string `json:"action"`               // "next", "prev", "jump", "skip", "run_to_end"
}

// ReplayStepResponse returns result of step execution
type ReplayStepResponse struct {
	Result *ReplayStepResult     `json:"result"`
	State  *ReplayExecutionState `json:"state"`
}

// ReplayInspectRequest for inspecting state at a step
type ReplayInspectRequest struct {
	SessionId  string `json:"sessionId"`
	StepNumber int    `json:"stepNumber"`
}

// ReplayInspectResponse returns detailed step information
type ReplayInspectResponse struct {
	Step          *ReplayStep                    `json:"step"`
	FileSnapshots map[string]*ReplayFileSnapshot `json:"fileSnapshots"`
	Diffs         map[string]*ReplayDiff         `json:"diffs"`
}

// ReplayPauseRequest for pausing/resuming replay
type ReplayPauseRequest struct {
	SessionId string `json:"sessionId"`
	Pause     bool   `json:"pause"`
}

// ReplayStatusResponse returns current replay status
type ReplayStatusResponse struct {
	State   *ReplayExecutionState `json:"state"`
	Session *ReplaySessionSummary `json:"session"`
}

// Constants for replay session versioning
const ReplaySessionVersion = "1.0.0"

// Helper methods

// IsDestructive returns true if the step makes changes to files
func (s *ReplayStep) IsDestructive() bool {
	switch s.Type {
	case ReplayStepTypeFileDiff, ReplayStepTypeFileWrite, ReplayStepTypeFileRemove, ReplayStepTypeFileMove:
		return true
	case ReplayStepTypeBuildComplete:
		return s.BuildInfo != nil && s.BuildInfo.Success
	default:
		return false
	}
}

// IsModelInteraction returns true if the step involves model communication
func (s *ReplayStep) IsModelInteraction() bool {
	return s.Type == ReplayStepTypeModelRequest || s.Type == ReplayStepTypeModelResponse
}

// GetDuration returns the step duration
func (s *ReplayStep) GetDuration() time.Duration {
	if s.CompletedAt == nil {
		return 0
	}
	return s.CompletedAt.Sub(s.StartedAt)
}

// HasDivergences returns true if any divergences were detected
func (state *ReplayExecutionState) HasDivergences() bool {
	return len(state.Divergences) > 0
}

// IsSafeMode returns true if replay is in a non-destructive mode
func (opts *ReplayOptions) IsSafeMode() bool {
	return opts.Mode == ReplayModeReadOnly || opts.Mode == ReplayModeSimulate
}

// DefaultReplayOptions returns safe default options
func DefaultReplayOptions() *ReplayOptions {
	return &ReplayOptions{
		Mode:              ReplayModeReadOnly,
		AutoAdvance:       false,
		StepDelayMs:       0,
		CaptureSnapshots:  true,
		ValidateChecksums: true,
		StopOnDivergence:  false,
	}
}
