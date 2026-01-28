package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// STREAM RECOVERY MANAGER
// =============================================================================
//
// This file provides stream recovery functionality to handle partial streaming
// responses. When a stream is interrupted, the manager tracks the partial
// content and allows for potential recovery or debugging.
//
// =============================================================================

// StreamRecoveryManager handles partial streaming response recovery
type StreamRecoveryManager struct {
	mu       sync.RWMutex
	sessions map[string]*StreamSession

	// Configuration
	config StreamRecoveryConfig

	// Optional callback when a session ends
	onSessionEnd func(session *StreamSession)
}

// StreamRecoveryConfig configures stream recovery behavior
type StreamRecoveryConfig struct {
	// MaxSessions is the maximum number of concurrent sessions to track
	MaxSessions int `json:"maxSessions"`

	// SessionTimeout is how long to keep inactive sessions
	SessionTimeout time.Duration `json:"sessionTimeout"`

	// CheckpointInterval is how many tokens between checkpoints
	CheckpointInterval int `json:"checkpointInterval"`

	// MaxCheckpoints is the maximum checkpoints to keep per session
	MaxCheckpoints int `json:"maxCheckpoints"`
}

// DefaultStreamRecoveryConfig provides sensible defaults
var DefaultStreamRecoveryConfig = StreamRecoveryConfig{
	MaxSessions:        100,
	SessionTimeout:     30 * time.Minute,
	CheckpointInterval: 1000,
	MaxCheckpoints:     10,
}

// StreamSession tracks a single streaming session
type StreamSession struct {
	// Identity
	Id        string    `json:"id"`
	StartedAt time.Time `json:"startedAt"`

	// Provider context
	Provider string `json:"provider"`
	Model    string `json:"model"`
	RequestId string `json:"requestId,omitempty"`

	// Accumulated content
	ContentBuffer  string `json:"contentBuffer"`
	TokensReceived int    `json:"tokensReceived"`
	ChunksReceived int    `json:"chunksReceived"`

	// Timing
	LastChunkAt time.Time `json:"lastChunkAt"`
	LastChunkSeq int      `json:"lastChunkSeq"`

	// Checkpoints for potential recovery
	Checkpoints []StreamCheckpoint `json:"checkpoints,omitempty"`

	// Status
	Status       StreamSessionStatus `json:"status"`
	EndedAt      *time.Time          `json:"endedAt,omitempty"`
	EndReason    string              `json:"endReason,omitempty"`
	FinalError   string              `json:"finalError,omitempty"`

	// For idempotency
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
}

// StreamSessionStatus represents the session state
type StreamSessionStatus string

const (
	StreamSessionActive      StreamSessionStatus = "active"
	StreamSessionCompleted   StreamSessionStatus = "completed"
	StreamSessionInterrupted StreamSessionStatus = "interrupted"
	StreamSessionFailed      StreamSessionStatus = "failed"
	StreamSessionTimedOut    StreamSessionStatus = "timed_out"
)

// StreamCheckpoint marks a recovery point within a streaming session
type StreamCheckpoint struct {
	// Sequence number of this checkpoint
	Seq int `json:"seq"`

	// Content state at checkpoint
	ContentLength int    `json:"contentLength"`
	TokenCount    int    `json:"tokenCount"`
	ContentHash   string `json:"contentHash"`

	// Timing
	Timestamp time.Time `json:"timestamp"`

	// For recovery
	ChunkSeq int `json:"chunkSeq"`
}

// GlobalStreamRecoveryManager is the singleton instance
var GlobalStreamRecoveryManager *StreamRecoveryManager

// InitGlobalStreamRecoveryManager initializes the global manager
func InitGlobalStreamRecoveryManager() {
	GlobalStreamRecoveryManager = NewStreamRecoveryManager(nil)
}

// InitGlobalStreamRecoveryManagerWithConfig initializes with custom config
func InitGlobalStreamRecoveryManagerWithConfig(config *StreamRecoveryConfig) {
	GlobalStreamRecoveryManager = NewStreamRecoveryManager(config)
}

// NewStreamRecoveryManager creates a new stream recovery manager
func NewStreamRecoveryManager(config *StreamRecoveryConfig) *StreamRecoveryManager {
	if config == nil {
		config = &DefaultStreamRecoveryConfig
	}

	manager := &StreamRecoveryManager{
		sessions: make(map[string]*StreamSession),
		config:   *config,
	}

	// Start cleanup goroutine
	go manager.cleanupLoop()

	return manager
}

// SetSessionEndCallback sets a callback for when sessions end
func (m *StreamRecoveryManager) SetSessionEndCallback(callback func(session *StreamSession)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onSessionEnd = callback
}

// =============================================================================
// SESSION LIFECYCLE
// =============================================================================

// StartSession begins tracking a new streaming session
func (m *StreamRecoveryManager) StartSession(id, provider, model string) *StreamSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clean up if at capacity
	if len(m.sessions) >= m.config.MaxSessions {
		m.pruneOldestSessionLocked()
	}

	session := &StreamSession{
		Id:          id,
		StartedAt:   time.Now(),
		Provider:    provider,
		Model:       model,
		LastChunkAt: time.Now(),
		Status:      StreamSessionActive,
		Checkpoints: make([]StreamCheckpoint, 0, m.config.MaxCheckpoints),
	}

	m.sessions[id] = session
	return session
}

// RecordChunk records a received chunk
func (m *StreamRecoveryManager) RecordChunk(sessionId string, content string, tokens int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionId]
	if !exists || session.Status != StreamSessionActive {
		return
	}

	// Update session state
	session.ContentBuffer += content
	session.TokensReceived += tokens
	session.ChunksReceived++
	session.LastChunkAt = time.Now()
	session.LastChunkSeq++

	// Create checkpoint if needed
	if m.config.CheckpointInterval > 0 && tokens > 0 {
		// Check if we've crossed a checkpoint threshold
		prevCheckpointTokens := 0
		if len(session.Checkpoints) > 0 {
			prevCheckpointTokens = session.Checkpoints[len(session.Checkpoints)-1].TokenCount
		}

		if session.TokensReceived-prevCheckpointTokens >= m.config.CheckpointInterval {
			m.createCheckpointLocked(session)
		}
	}
}

// createCheckpointLocked creates a checkpoint (caller must hold lock)
func (m *StreamRecoveryManager) createCheckpointLocked(session *StreamSession) {
	checkpoint := StreamCheckpoint{
		Seq:           len(session.Checkpoints) + 1,
		ContentLength: len(session.ContentBuffer),
		TokenCount:    session.TokensReceived,
		ContentHash:   hashContent(session.ContentBuffer),
		Timestamp:     time.Now(),
		ChunkSeq:      session.LastChunkSeq,
	}

	session.Checkpoints = append(session.Checkpoints, checkpoint)

	// Prune old checkpoints if needed
	if len(session.Checkpoints) > m.config.MaxCheckpoints {
		session.Checkpoints = session.Checkpoints[1:]
	}
}

// EndSession ends a streaming session
func (m *StreamRecoveryManager) EndSession(sessionId string, status StreamSessionStatus, reason string) *StreamSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionId]
	if !exists {
		return nil
	}

	now := time.Now()
	session.Status = status
	session.EndedAt = &now
	session.EndReason = reason

	// Create final checkpoint
	if session.ContentBuffer != "" && len(session.Checkpoints) == 0 {
		m.createCheckpointLocked(session)
	}

	// Call callback if set
	if m.onSessionEnd != nil {
		// Make a copy to avoid holding the lock during callback
		sessionCopy := *session
		go m.onSessionEnd(&sessionCopy)
	}

	// Remove from active sessions
	delete(m.sessions, sessionId)

	return session
}

// EndSessionWithError ends a session with an error
func (m *StreamRecoveryManager) EndSessionWithError(sessionId string, err error) *StreamSession {
	m.mu.Lock()
	session, exists := m.sessions[sessionId]
	if exists {
		session.FinalError = err.Error()
	}
	m.mu.Unlock()

	return m.EndSession(sessionId, StreamSessionFailed, "error")
}

// =============================================================================
// QUERIES
// =============================================================================

// GetSession retrieves a session by ID
func (m *StreamRecoveryManager) GetSession(sessionId string) *StreamSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionId]
	if !exists {
		return nil
	}

	// Return a copy
	copy := *session
	return &copy
}

// GetPartialContent returns accumulated content for a session
func (m *StreamRecoveryManager) GetPartialContent(sessionId string) (string, int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionId]
	if !exists {
		return "", 0
	}

	return session.ContentBuffer, session.TokensReceived
}

// GetActiveSessions returns all active session IDs
func (m *StreamRecoveryManager) GetActiveSessions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.sessions))
	for id, session := range m.sessions {
		if session.Status == StreamSessionActive {
			ids = append(ids, id)
		}
	}
	return ids
}

// GetLastCheckpoint returns the most recent checkpoint for a session
func (m *StreamRecoveryManager) GetLastCheckpoint(sessionId string) *StreamCheckpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionId]
	if !exists || len(session.Checkpoints) == 0 {
		return nil
	}

	// Return a copy of the last checkpoint
	checkpoint := session.Checkpoints[len(session.Checkpoints)-1]
	return &checkpoint
}

// =============================================================================
// RECOVERY
// =============================================================================

// RecoveryInfo contains information needed to potentially resume a stream
type RecoveryInfo struct {
	SessionId       string           `json:"sessionId"`
	Provider        string           `json:"provider"`
	Model           string           `json:"model"`
	PartialContent  string           `json:"partialContent"`
	TokensReceived  int              `json:"tokensReceived"`
	LastCheckpoint  *StreamCheckpoint `json:"lastCheckpoint,omitempty"`
	InterruptedAt   time.Time        `json:"interruptedAt"`
	CanResume       bool             `json:"canResume"`
	ResumeReason    string           `json:"resumeReason,omitempty"`
}

// GetRecoveryInfo returns information needed to potentially resume a stream
func (m *StreamRecoveryManager) GetRecoveryInfo(sessionId string) *RecoveryInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionId]
	if !exists {
		return nil
	}

	info := &RecoveryInfo{
		SessionId:      sessionId,
		Provider:       session.Provider,
		Model:          session.Model,
		PartialContent: session.ContentBuffer,
		TokensReceived: session.TokensReceived,
		InterruptedAt:  session.LastChunkAt,
	}

	if len(session.Checkpoints) > 0 {
		checkpoint := session.Checkpoints[len(session.Checkpoints)-1]
		info.LastCheckpoint = &checkpoint
	}

	// Determine if we can resume
	// Currently, resuming mid-stream isn't typically supported by LLM APIs,
	// but we track the information for debugging and potential future use
	info.CanResume = false
	info.ResumeReason = "mid-stream resumption not supported by provider"

	return info
}

// =============================================================================
// CLEANUP
// =============================================================================

// cleanupLoop periodically removes stale sessions
func (m *StreamRecoveryManager) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.cleanupStaleSessions()
	}
}

// cleanupStaleSessions removes sessions that have timed out
func (m *StreamRecoveryManager) cleanupStaleSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-m.config.SessionTimeout)

	for id, session := range m.sessions {
		if session.LastChunkAt.Before(cutoff) {
			session.Status = StreamSessionTimedOut
			now := time.Now()
			session.EndedAt = &now
			session.EndReason = "timeout"
			delete(m.sessions, id)
		}
	}
}

// pruneOldestSessionLocked removes the oldest session (caller must hold lock)
func (m *StreamRecoveryManager) pruneOldestSessionLocked() {
	var oldestId string
	var oldestTime time.Time

	for id, session := range m.sessions {
		if oldestId == "" || session.StartedAt.Before(oldestTime) {
			oldestId = id
			oldestTime = session.StartedAt
		}
	}

	if oldestId != "" {
		delete(m.sessions, oldestId)
	}
}

// =============================================================================
// STATISTICS
// =============================================================================

// StreamRecoveryStats provides statistics about stream recovery
type StreamRecoveryStats struct {
	ActiveSessions     int            `json:"activeSessions"`
	TotalTokens        int            `json:"totalTokens"`
	TotalChunks        int            `json:"totalChunks"`
	TotalCheckpoints   int            `json:"totalCheckpoints"`
	SessionsByProvider map[string]int `json:"sessionsByProvider"`
	SessionsByStatus   map[string]int `json:"sessionsByStatus"`
}

// GetStats returns statistics about the stream recovery manager
func (m *StreamRecoveryManager) GetStats() StreamRecoveryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := StreamRecoveryStats{
		ActiveSessions:     len(m.sessions),
		SessionsByProvider: make(map[string]int),
		SessionsByStatus:   make(map[string]int),
	}

	for _, session := range m.sessions {
		stats.TotalTokens += session.TokensReceived
		stats.TotalChunks += session.ChunksReceived
		stats.TotalCheckpoints += len(session.Checkpoints)
		stats.SessionsByProvider[session.Provider]++
		stats.SessionsByStatus[string(session.Status)]++
	}

	return stats
}

// =============================================================================
// HELPERS
// =============================================================================

// hashContent creates a short hash of content for verification
func hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:8]) // First 8 bytes = 16 hex chars
}

// GenerateSessionId creates a unique session ID
func GenerateSessionId(provider, model string) string {
	return fmt.Sprintf("stream_%s_%s_%d", provider, model, time.Now().UnixNano())
}
