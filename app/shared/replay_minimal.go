package shared

import (
	"time"
)

// =============================================================================
// MINIMAL REPLAY DATA MODEL
// =============================================================================
//
// For deterministic replay, we only need to capture NON-DETERMINISTIC inputs:
//
// 1. Model Responses    - AI outputs are non-deterministic (can't re-call)
// 2. Initial File State - Starting state before any changes
// 3. User Decisions     - Any user input during execution
// 4. External Data      - Anything fetched from outside (URLs, APIs)
//
// Everything else can be RECOMPUTED from these inputs:
// - Diffs (computed from before/after state)
// - Token counts (computed from content)
// - Build results (computed by re-running build logic)
// - Timing (informational only, not needed for determinism)
//
// =============================================================================

// MinimalReplaySession contains only the data required for deterministic replay
type MinimalReplaySession struct {
	// Identity
	Id        string    `json:"id"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`

	// Plan context
	PlanId string `json:"planId"`
	Branch string `json:"branch"`
	OrgId  string `json:"orgId"`

	// Initial state (REQUIRED - can't be recomputed)
	InitialFileStates map[string]FileState `json:"initialFileStates"`
	InitialContextIds []string             `json:"initialContextIds,omitempty"`

	// The sequence of non-deterministic events
	Events []ReplayEvent `json:"events"`
}

// FileState captures the minimal state of a file
type FileState struct {
	Path    string `json:"path"`
	Content string `json:"content"` // Empty string if file doesn't exist
	Exists  bool   `json:"exists"`
	// SHA256 hash for verification without storing full content
	Hash string `json:"hash,omitempty"`
}

// ReplayEventType identifies what non-deterministic event occurred
type ReplayEventType string

const (
	// EventUserPrompt - User provided input (non-deterministic)
	EventUserPrompt ReplayEventType = "user_prompt"

	// EventModelResponse - AI model returned a response (non-deterministic)
	EventModelResponse ReplayEventType = "model_response"

	// EventUserDecision - User made a choice (missing file, confirm, etc.)
	EventUserDecision ReplayEventType = "user_decision"

	// EventExternalFetch - Data fetched from external source (URL, API)
	EventExternalFetch ReplayEventType = "external_fetch"

	// EventFileStateChange - File changed outside our control (rare)
	EventFileStateChange ReplayEventType = "file_state_change"
)

// ReplayEvent represents a single non-deterministic event
type ReplayEvent struct {
	// Sequence number (1-indexed, monotonically increasing)
	Seq  int             `json:"seq"`
	Type ReplayEventType `json:"type"`

	// Type-specific payload (only one will be set)
	UserPrompt      *UserPromptEvent      `json:"userPrompt,omitempty"`
	ModelResponse   *ModelResponseEvent   `json:"modelResponse,omitempty"`
	UserDecision    *UserDecisionEvent    `json:"userDecision,omitempty"`
	ExternalFetch   *ExternalFetchEvent   `json:"externalFetch,omitempty"`
	FileStateChange *FileStateChangeEvent `json:"fileStateChange,omitempty"`
}

// =============================================================================
// EVENT PAYLOADS - Only the essential data for each event type
// =============================================================================

// UserPromptEvent captures a user's input prompt
type UserPromptEvent struct {
	Prompt string `json:"prompt"`

	// Optional: which iteration this prompt was for (for continue/auto-continue)
	Iteration int `json:"iteration,omitempty"`
}

// ModelResponseEvent captures the AI model's response
// This is the MOST CRITICAL data - without it, replay is impossible
type ModelResponseEvent struct {
	// The complete model output (REQUIRED)
	Content string `json:"content"`

	// Model identification (for verification/debugging)
	ModelId string `json:"modelId,omitempty"`

	// Was the response stopped early?
	Stopped bool `json:"stopped,omitempty"`

	// Finish reason from the API
	FinishReason string `json:"finishReason,omitempty"`

	// Input context hash - to verify we're replaying with same context
	// (SHA256 of the messages sent to the model)
	InputHash string `json:"inputHash,omitempty"`
}

// UserDecisionEvent captures a decision the user made during execution
type UserDecisionEvent struct {
	// What type of decision
	DecisionType string `json:"decisionType"` // "missing_file", "confirm_overwrite", "auto_context", etc.

	// The choice made
	Choice string `json:"choice"` // The actual selection

	// Context about what was being decided
	Context string `json:"context,omitempty"` // e.g., the file path for missing_file
}

// ExternalFetchEvent captures data fetched from external sources
type ExternalFetchEvent struct {
	// What was fetched
	Source string `json:"source"` // URL or API endpoint

	// The fetched content (REQUIRED for determinism)
	Content string `json:"content"`

	// Content hash for verification
	Hash string `json:"hash,omitempty"`
}

// FileStateChangeEvent captures when a file changed unexpectedly
// (e.g., external process modified it during execution)
type FileStateChangeEvent struct {
	Path       string `json:"path"`
	NewContent string `json:"newContent"`
	NewHash    string `json:"newHash,omitempty"`
}

// =============================================================================
// REPLAY EXECUTION STATE
// =============================================================================

// MinimalReplayState tracks position during replay
type MinimalReplayState struct {
	SessionId    string `json:"sessionId"`
	CurrentEvent int    `json:"currentEvent"` // 0-indexed into Events array
	TotalEvents  int    `json:"totalEvents"`

	// Current file states (updated as we replay)
	CurrentFiles map[string]FileState `json:"currentFiles"`

	// Divergences detected
	Divergences []string `json:"divergences,omitempty"`
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// NewMinimalReplaySession creates a new session with required fields
func NewMinimalReplaySession(planId, branch, orgId string) *MinimalReplaySession {
	return &MinimalReplaySession{
		Id:                generateId(),
		Version:           "1.0",
		CreatedAt:         time.Now(),
		PlanId:            planId,
		Branch:            branch,
		OrgId:             orgId,
		InitialFileStates: make(map[string]FileState),
		Events:            []ReplayEvent{},
	}
}

// AddEvent appends an event with auto-incremented sequence number
func (s *MinimalReplaySession) AddEvent(eventType ReplayEventType) *ReplayEvent {
	event := ReplayEvent{
		Seq:  len(s.Events) + 1,
		Type: eventType,
	}
	s.Events = append(s.Events, event)
	return &s.Events[len(s.Events)-1]
}

// RecordUserPrompt adds a user prompt event
func (s *MinimalReplaySession) RecordUserPrompt(prompt string, iteration int) {
	event := s.AddEvent(EventUserPrompt)
	event.UserPrompt = &UserPromptEvent{
		Prompt:    prompt,
		Iteration: iteration,
	}
}

// RecordModelResponse adds a model response event
func (s *MinimalReplaySession) RecordModelResponse(content, modelId, finishReason string, stopped bool, inputHash string) {
	event := s.AddEvent(EventModelResponse)
	event.ModelResponse = &ModelResponseEvent{
		Content:      content,
		ModelId:      modelId,
		Stopped:      stopped,
		FinishReason: finishReason,
		InputHash:    inputHash,
	}
}

// RecordUserDecision adds a user decision event
func (s *MinimalReplaySession) RecordUserDecision(decisionType, choice, context string) {
	event := s.AddEvent(EventUserDecision)
	event.UserDecision = &UserDecisionEvent{
		DecisionType: decisionType,
		Choice:       choice,
		Context:      context,
	}
}

// SetInitialFileState records the initial state of a file
func (s *MinimalReplaySession) SetInitialFileState(path, content string, exists bool) {
	s.InitialFileStates[path] = FileState{
		Path:    path,
		Content: content,
		Exists:  exists,
		Hash:    hashContent(content),
	}
}

// GetEventAt returns the event at the given index (0-indexed)
func (s *MinimalReplaySession) GetEventAt(idx int) *ReplayEvent {
	if idx < 0 || idx >= len(s.Events) {
		return nil
	}
	return &s.Events[idx]
}

// GetNextModelResponse returns the next model response event starting from idx
func (s *MinimalReplaySession) GetNextModelResponse(startIdx int) (*ModelResponseEvent, int) {
	for i := startIdx; i < len(s.Events); i++ {
		if s.Events[i].Type == EventModelResponse && s.Events[i].ModelResponse != nil {
			return s.Events[i].ModelResponse, i
		}
	}
	return nil, -1
}

// hashContent computes SHA256 hash (implementation would use crypto/sha256)
func hashContent(content string) string {
	// Placeholder - actual implementation uses crypto/sha256
	return ""
}

// generateId generates a UUID (implementation would use github.com/google/uuid)
func generateId() string {
	// Placeholder - actual implementation uses uuid.New().String()
	return ""
}

// =============================================================================
// DATA SIZE ANALYSIS
// =============================================================================
//
// Minimal data per event type (approximate):
//
// UserPromptEvent:     ~100 bytes - 10KB (depends on prompt length)
// ModelResponseEvent:  ~1KB - 100KB (depends on response length)
// UserDecisionEvent:   ~50 - 200 bytes
// ExternalFetchEvent:  ~100 bytes - 1MB (depends on fetched content)
// FileStateChangeEvent: ~100 bytes - 1MB (depends on file size)
//
// For a typical plan execution:
// - 1 user prompt:           ~1KB
// - 5-20 model responses:    ~50KB - 500KB
// - 0-5 user decisions:      ~500 bytes
// - Initial file states:     Varies (can be large for big codebases)
//
// OPTIMIZATION: For large files, store only the hash in InitialFileStates
// and reference the actual content from the plan's git history or context store.
//
// =============================================================================

// =============================================================================
// WHY THIS IS MINIMAL
// =============================================================================
//
// NOT NEEDED for deterministic replay:
//
// 1. Diffs - Can be computed by replaying the build logic with model responses
// 2. Token counts - Can be recomputed from content
// 3. Timing data - Only informational, doesn't affect determinism
// 4. Build intermediate states - Recomputed during replay
// 5. Context loading details - Determined by the model responses and prompts
// 6. Subtask metadata - Extracted from model responses
// 7. Error details - Reproduced when errors occur during replay
//
// REQUIRED for deterministic replay:
//
// 1. Model responses - Non-deterministic, can't reproduce
// 2. User prompts - The input that started each iteration
// 3. User decisions - Choices made during execution
// 4. Initial file states - Starting point for computing changes
// 5. External fetches - Any data from outside the system
//
// =============================================================================
