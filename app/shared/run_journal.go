package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// =============================================================================
// RUN JOURNAL FORMAT
// =============================================================================
//
// A run journal is a persistent record of execution that supports:
//   - Pausing: Save state at any point
//   - Resuming: Continue from saved state
//   - Skipping: Mark steps to skip
//   - Checkpoints: Named save points for recovery
//   - Branching: Fork execution from any checkpoint
//
// =============================================================================

// JournalVersion for format compatibility
const JournalVersion = "1.0.0"

// =============================================================================
// CORE TYPES
// =============================================================================

// RunJournal is the complete execution record
type RunJournal struct {
	// Header
	Header JournalHeader `json:"header"`

	// Execution state
	State JournalState `json:"state"`

	// The sequence of entries (immutable log)
	Entries []JournalEntry `json:"entries"`

	// Named checkpoints for resume points
	Checkpoints map[string]*Checkpoint `json:"checkpoints,omitempty"`

	// Steps marked to skip
	SkipList map[int]SkipReason `json:"skipList,omitempty"`

	// File state tracking
	FileStates map[string]*FileStateRecord `json:"fileStates,omitempty"`
}

// JournalHeader contains metadata about the journal
type JournalHeader struct {
	// Identity
	Id        string    `json:"id"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// Plan context
	PlanId string `json:"planId"`
	Branch string `json:"branch"`
	OrgId  string `json:"orgId"`
	UserId string `json:"userId"`

	// Initial prompt that started this run
	InitialPrompt string `json:"initialPrompt"`

	// Parent journal (if this is a fork/branch)
	ParentJournalId string `json:"parentJournalId,omitempty"`
	ForkedFromEntry int    `json:"forkedFromEntry,omitempty"`
}

// JournalState tracks the current execution position
type JournalState struct {
	// Current position
	Status       JournalStatus `json:"status"`
	CurrentEntry int           `json:"currentEntry"` // Index of current/next entry to process
	TotalEntries int           `json:"totalEntries"`

	// Pause/Resume tracking
	IsPaused    bool       `json:"isPaused"`
	PausedAt    *time.Time `json:"pausedAt,omitempty"`
	PauseReason string     `json:"pauseReason,omitempty"`
	ResumeCount int        `json:"resumeCount"` // How many times resumed

	// Progress tracking
	EntriesExecuted int `json:"entriesExecuted"`
	EntriesSkipped  int `json:"entriesSkipped"`
	EntriesFailed   int `json:"entriesFailed"`
	EntriesPending  int `json:"entriesPending"`

	// Last checkpoint (for quick resume)
	LastCheckpoint   string     `json:"lastCheckpoint,omitempty"`
	LastCheckpointAt *time.Time `json:"lastCheckpointAt,omitempty"`

	// Error state
	LastError      string     `json:"lastError,omitempty"`
	LastErrorEntry int        `json:"lastErrorEntry,omitempty"`
	LastErrorAt    *time.Time `json:"lastErrorAt,omitempty"`
}

// JournalStatus represents the overall journal status
type JournalStatus string

const (
	JournalStatusRecording JournalStatus = "recording" // Currently executing
	JournalStatusPaused    JournalStatus = "paused"    // Paused, can resume
	JournalStatusCompleted JournalStatus = "completed" // Finished successfully
	JournalStatusFailed    JournalStatus = "failed"    // Stopped due to error
	JournalStatusAborted   JournalStatus = "aborted"   // User cancelled
	JournalStatusReplaying JournalStatus = "replaying" // Being replayed
)

// =============================================================================
// JOURNAL ENTRIES
// =============================================================================

// JournalEntry represents a single step in the execution
type JournalEntry struct {
	// Identity and ordering
	Index     int       `json:"index"`     // 0-indexed position
	Id        string    `json:"id"`        // Unique ID for this entry
	Timestamp time.Time `json:"timestamp"` // When this entry was created

	// Entry type and status
	Type   EntryType   `json:"type"`
	Status EntryStatus `json:"status"`

	// Execution timing
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	DurationMs  int64      `json:"durationMs,omitempty"`

	// The actual data (type-specific)
	Data *EntryData `json:"data"`

	// For resume: hash of inputs to verify consistency
	InputHash string `json:"inputHash,omitempty"`

	// Skip tracking
	Skipped    bool   `json:"skipped,omitempty"`
	SkipReason string `json:"skipReason,omitempty"`

	// Dependencies (entries that must complete first)
	DependsOn []int `json:"dependsOn,omitempty"`

	// Error information
	Error *EntryError `json:"error,omitempty"`
}

// EntryType identifies what kind of step this is
type EntryType string

const (
	EntryTypeUserPrompt      EntryType = "user_prompt"
	EntryTypeModelRequest    EntryType = "model_request"
	EntryTypeModelResponse   EntryType = "model_response"
	EntryTypeFileBuild       EntryType = "file_build"
	EntryTypeFileWrite       EntryType = "file_write"
	EntryTypeFileDelete      EntryType = "file_delete"
	EntryTypeFileMove        EntryType = "file_move"
	EntryTypeContextLoad     EntryType = "context_load"
	EntryTypeUserDecision    EntryType = "user_decision"
	EntryTypeSubtaskStart    EntryType = "subtask_start"
	EntryTypeSubtaskComplete EntryType = "subtask_complete"
	EntryTypeCheckpoint      EntryType = "checkpoint"
	EntryTypeError           EntryType = "error"

	// Retry tracking entry types
	EntryTypeRetryAttempt  EntryType = "retry_attempt"   // Records a retry attempt
	EntryTypeRetryExhaust  EntryType = "retry_exhaust"   // Records when retries are exhausted
	EntryTypeCircuitEvent  EntryType = "circuit_event"   // Records circuit breaker state changes
	EntryTypeFallbackEvent EntryType = "fallback_event"  // Records fallback activations
)

// EntryStatus tracks execution state of an entry
type EntryStatus string

const (
	EntryStatusPending   EntryStatus = "pending"   // Not yet executed
	EntryStatusRunning   EntryStatus = "running"   // Currently executing
	EntryStatusCompleted EntryStatus = "completed" // Finished successfully
	EntryStatusFailed    EntryStatus = "failed"    // Failed with error
	EntryStatusSkipped   EntryStatus = "skipped"   // Skipped by user
	EntryStatusBlocked   EntryStatus = "blocked"   // Waiting on dependencies
)

// EntryData contains the type-specific payload
type EntryData struct {
	// User input
	UserPrompt *UserPromptData `json:"userPrompt,omitempty"`

	// Model interaction
	ModelRequest  *ModelRequestData  `json:"modelRequest,omitempty"`
	ModelResponse *ModelResponseData `json:"modelResponse,omitempty"`

	// File operations
	FileBuild  *FileBuildData  `json:"fileBuild,omitempty"`
	FileWrite  *FileWriteData  `json:"fileWrite,omitempty"`
	FileDelete *FileDeleteData `json:"fileDelete,omitempty"`
	FileMove   *FileMoveData   `json:"fileMove,omitempty"`

	// Context
	ContextLoad *ContextLoadData `json:"contextLoad,omitempty"`

	// User decisions
	UserDecision *UserDecisionData `json:"userDecision,omitempty"`

	// Subtasks
	Subtask *SubtaskData `json:"subtask,omitempty"`

	// Checkpoints
	Checkpoint *CheckpointData `json:"checkpoint,omitempty"`

	// Retry tracking
	RetryAttempt  *RetryAttemptData  `json:"retryAttempt,omitempty"`
	RetryExhaust  *RetryExhaustData  `json:"retryExhaust,omitempty"`
	CircuitEvent  *CircuitEventData  `json:"circuitEvent,omitempty"`
	FallbackEvent *FallbackEventData `json:"fallbackEvent,omitempty"`
}

// EntryError captures error details
type EntryError struct {
	Message    string `json:"message"`
	Type       string `json:"type,omitempty"`
	Retryable  bool   `json:"retryable"`
	StackTrace string `json:"stackTrace,omitempty"`
}

// =============================================================================
// ENTRY DATA TYPES
// =============================================================================

// UserPromptData captures user input
type UserPromptData struct {
	Prompt    string `json:"prompt"`
	Iteration int    `json:"iteration"`
}

// ModelRequestData captures what was sent to the model
type ModelRequestData struct {
	ModelId     string `json:"modelId"`
	ModelName   string `json:"modelName"`
	InputTokens int    `json:"inputTokens"`
	InputHash   string `json:"inputHash"` // Hash of full input for verification
}

// ModelResponseData captures the model's response
type ModelResponseData struct {
	Content      string `json:"content"`
	OutputTokens int    `json:"outputTokens"`
	FinishReason string `json:"finishReason"`
	Stopped      bool   `json:"stopped"`
}

// FileBuildData captures a file build operation
type FileBuildData struct {
	Path          string `json:"path"`
	BeforeHash    string `json:"beforeHash"`              // Hash of file before changes
	AfterHash     string `json:"afterHash"`               // Hash of file after changes
	BeforeContent string `json:"beforeContent,omitempty"` // Full content (optional, for small files)
	AfterContent  string `json:"afterContent,omitempty"`
	DiffPatch     string `json:"diffPatch,omitempty"` // Unified diff
}

// FileWriteData captures a new file creation
type FileWriteData struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Hash    string `json:"hash"`
}

// FileDeleteData captures file deletion
type FileDeleteData struct {
	Path          string `json:"path"`
	BeforeContent string `json:"beforeContent,omitempty"`
	BeforeHash    string `json:"beforeHash"`
}

// FileMoveData captures file rename/move
type FileMoveData struct {
	FromPath string `json:"fromPath"`
	ToPath   string `json:"toPath"`
}

// ContextLoadData captures context loading
type ContextLoadData struct {
	Files  []string `json:"files"`
	Tokens int      `json:"tokens"`
}

// UserDecisionData captures a user choice
type UserDecisionData struct {
	DecisionType string   `json:"decisionType"`
	Prompt       string   `json:"prompt"`
	Options      []string `json:"options"`
	Selected     string   `json:"selected"`
}

// SubtaskData captures subtask information
type SubtaskData struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Files       []string `json:"files,omitempty"`
	IsComplete  bool     `json:"isComplete"`
}

// CheckpointData captures checkpoint information
type CheckpointData struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	FileHashes  map[string]string `json:"fileHashes"` // Current state of all tracked files
	Auto        bool              `json:"auto"`       // Auto-created vs user-created
}

// =============================================================================
// RETRY TRACKING DATA TYPES
// =============================================================================

// RetryAttemptData captures a retry attempt for provider failures
type RetryAttemptData struct {
	// Attempt tracking
	AttemptNumber int `json:"attemptNumber"` // Current attempt (1-indexed)
	TotalAttempts int `json:"totalAttempts"` // Total attempts so far

	// Failure information
	FailureType  string `json:"failureType"`            // From FailureType
	ErrorMessage string `json:"errorMessage,omitempty"` // Sanitized error message
	HTTPCode     int    `json:"httpCode,omitempty"`     // HTTP status code if applicable

	// Provider context
	Provider string `json:"provider"` // Provider that failed
	Model    string `json:"model"`    // Model being used

	// Retry policy
	PolicyUsed string `json:"policyUsed"`        // Name of retry policy applied
	DelayMs    int64  `json:"delayMs"`           // Delay before this retry in milliseconds
	WillRetry  bool   `json:"willRetry"`         // Whether another retry will be attempted
	Retryable  bool   `json:"retryable"`         // Whether the error is retryable

	// Idempotency tracking
	IdempotencyKey string `json:"idempotencyKey,omitempty"` // Key for preventing duplicates
	RequestId      string `json:"requestId,omitempty"`      // Unique request identifier

	// Partial response tracking (for stream interruptions)
	HasPartialResponse bool   `json:"hasPartialResponse,omitempty"`
	PartialTokens      int    `json:"partialTokens,omitempty"`
	PartialContentHash string `json:"partialContentHash,omitempty"`
}

// RetryExhaustData captures when all retry attempts are exhausted
type RetryExhaustData struct {
	// Summary
	TotalAttempts   int   `json:"totalAttempts"`   // Total attempts made
	TotalDurationMs int64 `json:"totalDurationMs"` // Total time spent in retry loop

	// Failure history
	FailureTypes []string `json:"failureTypes"` // All failure types encountered
	FinalError   string   `json:"finalError"`   // The last error that caused exhaustion

	// Provider context
	Provider string `json:"provider"` // Provider that failed
	Model    string `json:"model"`    // Model being used

	// Fallback information
	FallbackUsed bool   `json:"fallbackUsed"`          // Whether fallback was attempted
	FallbackType string `json:"fallbackType,omitempty"` // Type of fallback used

	// Resolution
	Resolution string `json:"resolution"` // What happened: "failed", "fallback_success", "user_intervention"
}

// CircuitEventData captures circuit breaker state transitions
type CircuitEventData struct {
	// Provider
	Provider string `json:"provider"`

	// State transition
	OldState string `json:"oldState"` // Previous circuit state
	NewState string `json:"newState"` // New circuit state

	// Trigger information
	TriggerReason   string `json:"triggerReason,omitempty"`   // Why the transition happened
	ConsecFailures  int    `json:"consecFailures,omitempty"`  // Consecutive failures count
	RecentFailures  int    `json:"recentFailures,omitempty"`  // Failures in sliding window

	// Recovery information (for half-open -> closed)
	RecoverySuccesses int `json:"recoverySuccesses,omitempty"` // Successes in half-open
}

// FallbackEventData captures fallback activations
type FallbackEventData struct {
	// Provider transition
	FromProvider string `json:"fromProvider"` // Original provider
	ToProvider   string `json:"toProvider"`   // Fallback provider

	// Model transition
	FromModel string `json:"fromModel,omitempty"` // Original model
	ToModel   string `json:"toModel,omitempty"`   // Fallback model

	// Fallback type
	FallbackType string `json:"fallbackType"` // "error", "context", "provider"

	// Reason for fallback
	Reason       string `json:"reason"`                 // Human-readable reason
	FailureType  string `json:"failureType,omitempty"`  // Failure that triggered fallback
	ErrorMessage string `json:"errorMessage,omitempty"` // Error message

	// Outcome
	Success bool   `json:"success"`          // Whether fallback succeeded
	Error   string `json:"error,omitempty"`  // Error if fallback failed
}

// =============================================================================
// CHECKPOINTS
// =============================================================================

// Checkpoint represents a named resume point
type Checkpoint struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`

	// Position in the journal
	EntryIndex int `json:"entryIndex"`

	// State snapshot
	StateSnapshot *JournalState `json:"stateSnapshot"`

	// File states at this checkpoint
	FileStates map[string]string `json:"fileStates"` // path -> hash

	// File contents for restoration (optional, can be large)
	FileContents map[string]string `json:"fileContents,omitempty"` // path -> content

	// For verification
	JournalHash string `json:"journalHash"` // Hash of all entries up to this point
}

// =============================================================================
// SKIP LIST
// =============================================================================

// SkipReason explains why a step is skipped
type SkipReason struct {
	Reason    string    `json:"reason"`
	SkippedAt time.Time `json:"skippedAt"`
	SkippedBy string    `json:"skippedBy,omitempty"` // "user" or "auto"
}

// =============================================================================
// FILE STATE TRACKING
// =============================================================================

// FileStateRecord tracks a file's state through execution
type FileStateRecord struct {
	Path         string    `json:"path"`
	InitialHash  string    `json:"initialHash"`
	CurrentHash  string    `json:"currentHash"`
	LastModified time.Time `json:"lastModified"`
	ModifiedBy   int       `json:"modifiedBy"` // Entry index that last modified it
}

// =============================================================================
// JOURNAL OPERATIONS
// =============================================================================

// NewRunJournal creates a new journal
func NewRunJournal(planId, branch, orgId, userId, prompt string) *RunJournal {
	now := time.Now()
	return &RunJournal{
		Header: JournalHeader{
			Id:            generateUUID(),
			Version:       JournalVersion,
			CreatedAt:     now,
			UpdatedAt:     now,
			PlanId:        planId,
			Branch:        branch,
			OrgId:         orgId,
			UserId:        userId,
			InitialPrompt: prompt,
		},
		State: JournalState{
			Status:       JournalStatusRecording,
			CurrentEntry: 0,
		},
		Entries:     []JournalEntry{},
		Checkpoints: make(map[string]*Checkpoint),
		SkipList:    make(map[int]SkipReason),
		FileStates:  make(map[string]*FileStateRecord),
	}
}

// =============================================================================
// ENTRY OPERATIONS
// =============================================================================

// AppendEntry adds a new entry to the journal
func (j *RunJournal) AppendEntry(entryType EntryType, data *EntryData) *JournalEntry {
	entry := JournalEntry{
		Index:     len(j.Entries),
		Id:        generateUUID(),
		Timestamp: time.Now(),
		Type:      entryType,
		Status:    EntryStatusPending,
		Data:      data,
	}

	j.Entries = append(j.Entries, entry)
	j.State.TotalEntries = len(j.Entries)
	j.State.EntriesPending++
	j.Header.UpdatedAt = time.Now()

	return &j.Entries[len(j.Entries)-1]
}

// StartEntry marks an entry as running
func (j *RunJournal) StartEntry(index int) error {
	if index < 0 || index >= len(j.Entries) {
		return fmt.Errorf("invalid entry index: %d", index)
	}

	entry := &j.Entries[index]
	if entry.Status != EntryStatusPending && entry.Status != EntryStatusBlocked {
		return fmt.Errorf("entry %d is not pending (status: %s)", index, entry.Status)
	}

	now := time.Now()
	entry.Status = EntryStatusRunning
	entry.StartedAt = &now
	j.State.CurrentEntry = index
	j.State.EntriesPending--
	j.Header.UpdatedAt = now

	return nil
}

// CompleteEntry marks an entry as completed
func (j *RunJournal) CompleteEntry(index int) error {
	if index < 0 || index >= len(j.Entries) {
		return fmt.Errorf("invalid entry index: %d", index)
	}

	entry := &j.Entries[index]
	now := time.Now()
	entry.Status = EntryStatusCompleted
	entry.CompletedAt = &now

	if entry.StartedAt != nil {
		entry.DurationMs = now.Sub(*entry.StartedAt).Milliseconds()
	}

	j.State.EntriesExecuted++
	j.State.CurrentEntry = index + 1
	j.Header.UpdatedAt = now

	return nil
}

// FailEntry marks an entry as failed
func (j *RunJournal) FailEntry(index int, err *EntryError) error {
	if index < 0 || index >= len(j.Entries) {
		return fmt.Errorf("invalid entry index: %d", index)
	}

	entry := &j.Entries[index]
	now := time.Now()
	entry.Status = EntryStatusFailed
	entry.CompletedAt = &now
	entry.Error = err

	if entry.StartedAt != nil {
		entry.DurationMs = now.Sub(*entry.StartedAt).Milliseconds()
	}

	j.State.EntriesFailed++
	j.State.LastError = err.Message
	j.State.LastErrorEntry = index
	j.State.LastErrorAt = &now
	j.Header.UpdatedAt = now

	return nil
}

// =============================================================================
// RETRY TRACKING OPERATIONS
// =============================================================================

// AppendRetryAttempt records a retry attempt in the journal
func (j *RunJournal) AppendRetryAttempt(data *RetryAttemptData) *JournalEntry {
	entry := j.AppendEntry(EntryTypeRetryAttempt, &EntryData{
		RetryAttempt: data,
	})
	// Auto-complete retry attempt entries since they're just logs
	now := time.Now()
	entry.Status = EntryStatusCompleted
	entry.StartedAt = &now
	entry.CompletedAt = &now
	j.State.EntriesPending--
	j.State.EntriesExecuted++
	return entry
}

// AppendRetryExhaust records when retries are exhausted
func (j *RunJournal) AppendRetryExhaust(data *RetryExhaustData) *JournalEntry {
	entry := j.AppendEntry(EntryTypeRetryExhaust, &EntryData{
		RetryExhaust: data,
	})
	// Auto-complete with failed status since retries were exhausted
	now := time.Now()
	entry.Status = EntryStatusFailed
	entry.StartedAt = &now
	entry.CompletedAt = &now
	entry.Error = &EntryError{
		Message:   data.FinalError,
		Type:      "retry_exhausted",
		Retryable: false,
	}
	j.State.EntriesPending--
	j.State.EntriesFailed++
	j.State.LastError = data.FinalError
	j.State.LastErrorAt = &now
	return entry
}

// AppendCircuitEvent records a circuit breaker state change
func (j *RunJournal) AppendCircuitEvent(data *CircuitEventData) *JournalEntry {
	entry := j.AppendEntry(EntryTypeCircuitEvent, &EntryData{
		CircuitEvent: data,
	})
	// Auto-complete circuit events
	now := time.Now()
	entry.Status = EntryStatusCompleted
	entry.StartedAt = &now
	entry.CompletedAt = &now
	j.State.EntriesPending--
	j.State.EntriesExecuted++
	return entry
}

// AppendFallbackEvent records a fallback activation
func (j *RunJournal) AppendFallbackEvent(data *FallbackEventData) *JournalEntry {
	entry := j.AppendEntry(EntryTypeFallbackEvent, &EntryData{
		FallbackEvent: data,
	})
	// Auto-complete fallback events with status based on outcome
	now := time.Now()
	entry.StartedAt = &now
	entry.CompletedAt = &now
	j.State.EntriesPending--

	if data.Success {
		entry.Status = EntryStatusCompleted
		j.State.EntriesExecuted++
	} else {
		entry.Status = EntryStatusFailed
		entry.Error = &EntryError{
			Message:   data.Error,
			Type:      "fallback_failed",
			Retryable: false,
		}
		j.State.EntriesFailed++
	}
	return entry
}

// GetRetryAttempts returns all retry attempt entries for analysis
func (j *RunJournal) GetRetryAttempts() []*JournalEntry {
	var attempts []*JournalEntry
	for i := range j.Entries {
		if j.Entries[i].Type == EntryTypeRetryAttempt {
			attempts = append(attempts, &j.Entries[i])
		}
	}
	return attempts
}

// GetRetryStats returns statistics about retry attempts in this journal
func (j *RunJournal) GetRetryStats() RetryJournalStats {
	stats := RetryJournalStats{
		ByProvider:    make(map[string]int),
		ByFailureType: make(map[string]int),
	}

	for i := range j.Entries {
		entry := &j.Entries[i]
		switch entry.Type {
		case EntryTypeRetryAttempt:
			if entry.Data != nil && entry.Data.RetryAttempt != nil {
				data := entry.Data.RetryAttempt
				stats.TotalAttempts++
				stats.TotalDelayMs += data.DelayMs
				stats.ByProvider[data.Provider]++
				stats.ByFailureType[data.FailureType]++
			}
		case EntryTypeRetryExhaust:
			stats.ExhaustedCount++
		case EntryTypeFallbackEvent:
			if entry.Data != nil && entry.Data.FallbackEvent != nil {
				stats.FallbackCount++
				if entry.Data.FallbackEvent.Success {
					stats.FallbackSuccesses++
				}
			}
		case EntryTypeCircuitEvent:
			if entry.Data != nil && entry.Data.CircuitEvent != nil {
				if entry.Data.CircuitEvent.NewState == "open" {
					stats.CircuitOpenCount++
				}
			}
		}
	}

	return stats
}

// RetryJournalStats provides statistics about retry operations in a journal
type RetryJournalStats struct {
	TotalAttempts    int            `json:"totalAttempts"`
	TotalDelayMs     int64          `json:"totalDelayMs"`
	ExhaustedCount   int            `json:"exhaustedCount"`
	FallbackCount    int            `json:"fallbackCount"`
	FallbackSuccesses int           `json:"fallbackSuccesses"`
	CircuitOpenCount int            `json:"circuitOpenCount"`
	ByProvider       map[string]int `json:"byProvider"`
	ByFailureType    map[string]int `json:"byFailureType"`
}

// =============================================================================
// SKIP OPERATIONS
// =============================================================================

// SkipEntry marks an entry to be skipped
func (j *RunJournal) SkipEntry(index int, reason string) error {
	if index < 0 || index >= len(j.Entries) {
		return fmt.Errorf("invalid entry index: %d", index)
	}

	entry := &j.Entries[index]
	if entry.Status == EntryStatusCompleted {
		return fmt.Errorf("cannot skip completed entry %d", index)
	}

	now := time.Now()
	entry.Status = EntryStatusSkipped
	entry.Skipped = true
	entry.SkipReason = reason
	entry.CompletedAt = &now

	j.SkipList[index] = SkipReason{
		Reason:    reason,
		SkippedAt: now,
		SkippedBy: "user",
	}

	if entry.Status == EntryStatusPending {
		j.State.EntriesPending--
	}
	j.State.EntriesSkipped++
	j.Header.UpdatedAt = now

	return nil
}

// SkipRange marks a range of entries to be skipped
func (j *RunJournal) SkipRange(start, end int, reason string) error {
	for i := start; i <= end && i < len(j.Entries); i++ {
		if err := j.SkipEntry(i, reason); err != nil {
			// Continue skipping others even if one fails
			continue
		}
	}
	return nil
}

// IsSkipped checks if an entry is skipped
func (j *RunJournal) IsSkipped(index int) bool {
	_, exists := j.SkipList[index]
	return exists
}

// UnskipEntry removes an entry from the skip list
func (j *RunJournal) UnskipEntry(index int) error {
	if index < 0 || index >= len(j.Entries) {
		return fmt.Errorf("invalid entry index: %d", index)
	}

	entry := &j.Entries[index]
	if entry.Status != EntryStatusSkipped {
		return fmt.Errorf("entry %d is not skipped", index)
	}

	entry.Status = EntryStatusPending
	entry.Skipped = false
	entry.SkipReason = ""
	entry.CompletedAt = nil

	delete(j.SkipList, index)

	j.State.EntriesSkipped--
	j.State.EntriesPending++
	j.Header.UpdatedAt = time.Now()

	return nil
}

// =============================================================================
// PAUSE/RESUME OPERATIONS
// =============================================================================

// Pause pauses the journal at the current position
func (j *RunJournal) Pause(reason string) error {
	if j.State.IsPaused {
		return fmt.Errorf("journal is already paused")
	}

	now := time.Now()
	j.State.Status = JournalStatusPaused
	j.State.IsPaused = true
	j.State.PausedAt = &now
	j.State.PauseReason = reason
	j.Header.UpdatedAt = now

	// Auto-create a checkpoint
	checkpointName := fmt.Sprintf("pause_%d", j.State.CurrentEntry)
	j.CreateCheckpoint(checkpointName, "Auto-checkpoint on pause", true)

	return nil
}

// Resume resumes from paused state
func (j *RunJournal) Resume() error {
	if !j.State.IsPaused {
		return fmt.Errorf("journal is not paused")
	}

	j.State.Status = JournalStatusRecording
	j.State.IsPaused = false
	j.State.PausedAt = nil
	j.State.PauseReason = ""
	j.State.ResumeCount++
	j.Header.UpdatedAt = time.Now()

	return nil
}

// ResumeFrom resumes from a specific checkpoint
func (j *RunJournal) ResumeFrom(checkpointName string) error {
	checkpoint, exists := j.Checkpoints[checkpointName]
	if !exists {
		return fmt.Errorf("checkpoint not found: %s", checkpointName)
	}

	// Restore state from checkpoint
	j.State = *checkpoint.StateSnapshot
	j.State.Status = JournalStatusRecording
	j.State.IsPaused = false
	j.State.ResumeCount++
	j.Header.UpdatedAt = time.Now()

	return nil
}

// ResumeFromEntry resumes from a specific entry index
func (j *RunJournal) ResumeFromEntry(index int) error {
	if index < 0 || index >= len(j.Entries) {
		return fmt.Errorf("invalid entry index: %d", index)
	}

	j.State.CurrentEntry = index
	j.State.Status = JournalStatusRecording
	j.State.IsPaused = false
	j.State.ResumeCount++
	j.Header.UpdatedAt = time.Now()

	// Mark all entries from index onwards as pending (if not skipped)
	for i := index; i < len(j.Entries); i++ {
		if !j.IsSkipped(i) && j.Entries[i].Status != EntryStatusCompleted {
			j.Entries[i].Status = EntryStatusPending
		}
	}

	return nil
}

// =============================================================================
// CHECKPOINT OPERATIONS
// =============================================================================

// CreateCheckpoint creates a named checkpoint at the current position
func (j *RunJournal) CreateCheckpoint(name, description string, auto bool) *Checkpoint {
	now := time.Now()

	// Compute file state hashes
	fileStates := make(map[string]string)
	for path, state := range j.FileStates {
		fileStates[path] = state.CurrentHash
	}

	// Copy current state
	stateCopy := j.State

	checkpoint := &Checkpoint{
		Name:          name,
		Description:   description,
		CreatedAt:     now,
		EntryIndex:    j.State.CurrentEntry,
		StateSnapshot: &stateCopy,
		FileStates:    fileStates,
		JournalHash:   j.ComputeHashUpTo(j.State.CurrentEntry),
	}

	j.Checkpoints[name] = checkpoint
	j.State.LastCheckpoint = name
	j.State.LastCheckpointAt = &now
	j.Header.UpdatedAt = now

	// Add checkpoint entry to journal
	j.AppendEntry(EntryTypeCheckpoint, &EntryData{
		Checkpoint: &CheckpointData{
			Name:        name,
			Description: description,
			FileHashes:  fileStates,
			Auto:        auto,
		},
	})

	return checkpoint
}

// GetCheckpoint retrieves a checkpoint by name
func (j *RunJournal) GetCheckpoint(name string) *Checkpoint {
	return j.Checkpoints[name]
}

// ListCheckpoints returns all checkpoint names
func (j *RunJournal) ListCheckpoints() []string {
	names := make([]string, 0, len(j.Checkpoints))
	for name := range j.Checkpoints {
		names = append(names, name)
	}
	return names
}

// =============================================================================
// NAVIGATION
// =============================================================================

// GetCurrentEntry returns the current entry
func (j *RunJournal) GetCurrentEntry() *JournalEntry {
	if j.State.CurrentEntry >= len(j.Entries) {
		return nil
	}
	return &j.Entries[j.State.CurrentEntry]
}

// GetEntry returns an entry by index
func (j *RunJournal) GetEntry(index int) *JournalEntry {
	if index < 0 || index >= len(j.Entries) {
		return nil
	}
	return &j.Entries[index]
}

// GetNextPendingEntry returns the next entry that needs execution
func (j *RunJournal) GetNextPendingEntry() *JournalEntry {
	for i := j.State.CurrentEntry; i < len(j.Entries); i++ {
		if j.Entries[i].Status == EntryStatusPending {
			return &j.Entries[i]
		}
	}
	return nil
}

// HasMoreEntries returns true if there are more entries to process
func (j *RunJournal) HasMoreEntries() bool {
	return j.State.CurrentEntry < len(j.Entries)
}

// =============================================================================
// VERIFICATION
// =============================================================================

// ComputeHashUpTo computes a hash of all entries up to (not including) index
func (j *RunJournal) ComputeHashUpTo(index int) string {
	if index <= 0 || index > len(j.Entries) {
		return ""
	}

	h := sha256.New()
	for i := 0; i < index; i++ {
		entryBytes, _ := json.Marshal(j.Entries[i])
		h.Write(entryBytes)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyCheckpoint verifies a checkpoint's integrity
func (j *RunJournal) VerifyCheckpoint(name string) (bool, string) {
	checkpoint := j.Checkpoints[name]
	if checkpoint == nil {
		return false, "checkpoint not found"
	}

	currentHash := j.ComputeHashUpTo(checkpoint.EntryIndex)
	if currentHash != checkpoint.JournalHash {
		return false, fmt.Sprintf("journal hash mismatch at entry %d", checkpoint.EntryIndex)
	}

	return true, ""
}

// =============================================================================
// FILE STATE OPERATIONS
// =============================================================================

// TrackFile starts tracking a file's state
func (j *RunJournal) TrackFile(path, content string) {
	hash := computeHash(content)
	j.FileStates[path] = &FileStateRecord{
		Path:         path,
		InitialHash:  hash,
		CurrentHash:  hash,
		LastModified: time.Now(),
		ModifiedBy:   -1,
	}
}

// UpdateFileState updates a tracked file's state
func (j *RunJournal) UpdateFileState(path, content string, modifiedByEntry int) {
	hash := computeHash(content)
	if state, exists := j.FileStates[path]; exists {
		state.CurrentHash = hash
		state.LastModified = time.Now()
		state.ModifiedBy = modifiedByEntry
	} else {
		j.FileStates[path] = &FileStateRecord{
			Path:         path,
			InitialHash:  hash,
			CurrentHash:  hash,
			LastModified: time.Now(),
			ModifiedBy:   modifiedByEntry,
		}
	}
}

// GetFileState returns the tracked state for a file
func (j *RunJournal) GetFileState(path string) *FileStateRecord {
	return j.FileStates[path]
}

// =============================================================================
// RETRY RECORDING
// =============================================================================

// RetryRecord captures a single retry attempt for journal auditing.
type RetryRecord struct {
	AttemptNumber  int       `json:"attemptNumber"`
	StartedAt      time.Time `json:"startedAt"`
	DurationMs     int64     `json:"durationMs"`
	FailureType    string    `json:"failureType,omitempty"`
	ErrorMessage   string    `json:"errorMessage,omitempty"`
	Retriable      bool      `json:"retriable"`
	DelayAppliedMs int64     `json:"delayAppliedMs,omitempty"`
	UsedFallback   bool      `json:"usedFallback,omitempty"`
	FallbackType   string    `json:"fallbackType,omitempty"`
}

// RecordRetryAttempt appends a retry record to the given journal entry's error
// context.  If the entry has no error yet, one is created.  The retry history
// is stored as a JSON-encoded list in the StackTrace field (which is
// otherwise unused for provider errors) so existing serialisation works
// without schema changes.
func (j *RunJournal) RecordRetryAttempt(entryIndex int, record RetryRecord) error {
	if entryIndex < 0 || entryIndex >= len(j.Entries) {
		return fmt.Errorf("invalid entry index: %d", entryIndex)
	}

	entry := &j.Entries[entryIndex]
	if entry.Error == nil {
		entry.Error = &EntryError{
			Message:   record.ErrorMessage,
			Type:      record.FailureType,
			Retryable: record.Retriable,
		}
	}

	// Append to the StackTrace field as structured retry history
	existing := entry.Error.StackTrace
	recordJSON, _ := json.Marshal(record)
	if existing == "" {
		entry.Error.StackTrace = string(recordJSON)
	} else {
		entry.Error.StackTrace = existing + "\n" + string(recordJSON)
	}

	j.Header.UpdatedAt = time.Now()
	return nil
}

// RecordRetryOutcome finalises the retry history for an entry.  If succeeded
// is true, the entry error is cleared (the retry ultimately worked).
// Otherwise the entry remains in failed state with the full retry trace.
func (j *RunJournal) RecordRetryOutcome(entryIndex int, succeeded bool, totalAttempts int) error {
	if entryIndex < 0 || entryIndex >= len(j.Entries) {
		return fmt.Errorf("invalid entry index: %d", entryIndex)
	}

	entry := &j.Entries[entryIndex]
	now := time.Now()

	if succeeded {
		entry.Status = EntryStatusCompleted
		entry.CompletedAt = &now
		if entry.StartedAt != nil {
			entry.DurationMs = now.Sub(*entry.StartedAt).Milliseconds()
		}
		// Preserve retry trace but mark as resolved
		if entry.Error != nil {
			entry.Error.Message = fmt.Sprintf("resolved after %d attempt(s): %s", totalAttempts, entry.Error.Message)
			entry.Error.Retryable = false
		}
	} else {
		entry.Status = EntryStatusFailed
		entry.CompletedAt = &now
		if entry.StartedAt != nil {
			entry.DurationMs = now.Sub(*entry.StartedAt).Milliseconds()
		}
		if entry.Error != nil {
			entry.Error.Message = fmt.Sprintf("failed after %d attempt(s): %s", totalAttempts, entry.Error.Message)
		}

		j.State.EntriesFailed++
		if entry.Error != nil {
			j.State.LastError = entry.Error.Message
			j.State.LastErrorEntry = entryIndex
			j.State.LastErrorAt = &now
		}
	}

	j.Header.UpdatedAt = now
	return nil
}

// =============================================================================
// SERIALIZATION
// =============================================================================

// ToJSON serializes the journal to JSON
func (j *RunJournal) ToJSON() ([]byte, error) {
	return json.MarshalIndent(j, "", "  ")
}

// FromJSON deserializes a journal from JSON
func FromJSON(data []byte) (*RunJournal, error) {
	var journal RunJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return nil, err
	}
	return &journal, nil
}

// =============================================================================
// HELPERS
// =============================================================================

func computeHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

func generateUUID() string {
	// Placeholder - use github.com/google/uuid in real implementation
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// =============================================================================
// USAGE EXAMPLES (in comments)
// =============================================================================
//
// RECORDING:
//
//   journal := NewRunJournal(planId, branch, orgId, userId, prompt)
//
//   // Add entries as they happen
//   entry := journal.AppendEntry(EntryTypeModelResponse, &EntryData{...})
//   journal.StartEntry(entry.Index)
//   // ... execute ...
//   journal.CompleteEntry(entry.Index)
//
// PAUSING:
//
//   journal.Pause("User requested pause")
//   data, _ := journal.ToJSON()
//   // Save to disk
//
// RESUMING:
//
//   data, _ := os.ReadFile("journal.json")
//   journal, _ := FromJSON(data)
//   journal.Resume()
//   // Continue from journal.GetNextPendingEntry()
//
// SKIPPING:
//
//   journal.SkipEntry(5, "User chose to skip")
//   journal.SkipRange(10, 15, "Skipping problematic section")
//
// CHECKPOINTS:
//
//   journal.CreateCheckpoint("before_refactor", "State before major refactor", false)
//   // ... do work ...
//   // If something goes wrong:
//   journal.ResumeFrom("before_refactor")
//
// =============================================================================
