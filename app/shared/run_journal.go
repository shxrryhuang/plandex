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
