package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// CRASH RECOVERY SYSTEM
// =============================================================================
//
// This module handles recovery from crashes, partial runs, and unexpected
// failures. It provides:
//
//   - Recovery markers: Track in-flight operations
//   - Operation logging: Audit trail for debugging
//   - Automatic recovery: Restore consistent state on startup
//   - Checkpoint support: Named recovery points within operations
//
// Recovery flow:
//   1. On startup, scan for workspaces with recovery.json
//   2. Attempt to restore to last known good state
//   3. Mark workspace as recovered or failed
//
// =============================================================================

// RecoveryInfo tracks in-flight operations for crash recovery
type RecoveryInfo struct {
	WorkspaceId    string    `json:"workspaceId"`
	OperationType  string    `json:"operationType"`
	StartedAt      time.Time `json:"startedAt"`
	LastCheckpoint string    `json:"lastCheckpoint,omitempty"`
	PendingFiles   []string  `json:"pendingFiles,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// OperationLog tracks all operations for audit and recovery
type OperationLog struct {
	ws      *Workspace
	logPath string
	mu      sync.Mutex
}

// LogEntry represents a single operation in the log
type LogEntry struct {
	Timestamp  time.Time              `json:"timestamp"`
	Operation  string                 `json:"operation"`
	Path       string                 `json:"path,omitempty"`
	Status     OperationStatus        `json:"status"`
	Error      string                 `json:"error,omitempty"`
	Checksum   string                 `json:"checksum,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// OperationStatus indicates the status of an operation
type OperationStatus string

const (
	OpStatusStarted   OperationStatus = "started"
	OpStatusCompleted OperationStatus = "completed"
	OpStatusFailed    OperationStatus = "failed"
	OpStatusSkipped   OperationStatus = "skipped"
)

// RecoveryManager handles crash recovery for workspaces
type RecoveryManager struct {
	workspacesDir string
	mu            sync.Mutex
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(workspacesDir string) *RecoveryManager {
	return &RecoveryManager{
		workspacesDir: workspacesDir,
	}
}

// =============================================================================
// RECOVERY OPERATIONS
// =============================================================================

// CheckAndRecover scans for workspaces needing recovery
func (rm *RecoveryManager) CheckAndRecover() ([]*Workspace, []error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	entries, err := os.ReadDir(rm.workspacesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []error{fmt.Errorf("failed to read workspaces directory: %w", err)}
	}

	var recovered []*Workspace
	var errors []error

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		wsPath := filepath.Join(rm.workspacesDir, entry.Name(), "workspace.json")
		recoveryPath := filepath.Join(rm.workspacesDir, entry.Name(), "recovery.json")

		// Check for recovery file (indicates crash during operation)
		if _, err := os.Stat(recoveryPath); err == nil {
			ws, err := rm.recoverWorkspace(wsPath, recoveryPath)
			if err != nil {
				errors = append(errors, fmt.Errorf("failed to recover workspace %s: %w", entry.Name(), err))
				continue
			}
			recovered = append(recovered, ws)
		}
	}

	return recovered, errors
}

// recoverWorkspace attempts to restore workspace to consistent state
func (rm *RecoveryManager) recoverWorkspace(wsPath, recoveryPath string) (*Workspace, error) {
	// Load workspace metadata
	ws, err := LoadWorkspace(wsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace: %w", err)
	}

	// Load recovery info
	recoveryData, err := os.ReadFile(recoveryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read recovery info: %w", err)
	}

	var recovery RecoveryInfo
	if err := json.Unmarshal(recoveryData, &recovery); err != nil {
		return nil, fmt.Errorf("failed to parse recovery info: %w", err)
	}

	// Apply recovery based on operation type
	switch recovery.OperationType {
	case "apply":
		// Incomplete apply - files may be partially written
		// Remove any partially written files from workspace
		for _, path := range recovery.PendingFiles {
			wsFilePath := ws.GetFilePath(path)
			os.Remove(wsFilePath) // Ignore errors - file may not exist

			// Remove from tracking
			ws.mu.Lock()
			delete(ws.ModifiedFiles, path)
			delete(ws.CreatedFiles, path)
			ws.mu.Unlock()
		}

	case "commit":
		// Incomplete commit - main project may be partially updated
		// Mark workspace for manual review
		ws.SetState(WorkspaceStateRecovering)

	case "discard":
		// Incomplete discard - just re-discard
		ws.SetState(WorkspaceStateDiscarded)

	default:
		// Unknown operation type - mark for manual review
		ws.SetState(WorkspaceStateRecovering)
	}

	// Update recovery timestamp
	now := time.Now()
	ws.CrashRecoveryAt = &now

	// If we recovered to a known good state, mark as active
	if ws.State != WorkspaceStateRecovering {
		ws.SetState(WorkspaceStateActive)
	}

	// Clean up recovery file
	os.Remove(recoveryPath)

	// Save recovered state
	if err := ws.Save(); err != nil {
		return nil, fmt.Errorf("failed to save recovered workspace: %w", err)
	}

	return ws, nil
}

// =============================================================================
// RECOVERY MARKERS
// =============================================================================

// StartRecoveryPoint creates a recovery marker before risky operations
func StartRecoveryPoint(ws *Workspace, opType string, files []string) error {
	recovery := RecoveryInfo{
		WorkspaceId:   ws.Id,
		OperationType: opType,
		StartedAt:     time.Now(),
		PendingFiles:  files,
		Metadata:      make(map[string]interface{}),
	}

	if ws.LastCheckpoint != "" {
		recovery.LastCheckpoint = ws.LastCheckpoint
	}

	data, err := json.MarshalIndent(recovery, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal recovery info: %w", err)
	}

	recoveryPath := filepath.Join(ws.WorkspaceDir, "recovery.json")
	if err := os.WriteFile(recoveryPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write recovery marker: %w", err)
	}

	return nil
}

// UpdateRecoveryPoint updates the recovery marker with progress
func UpdateRecoveryPoint(ws *Workspace, completedFiles []string, pendingFiles []string) error {
	recoveryPath := filepath.Join(ws.WorkspaceDir, "recovery.json")

	data, err := os.ReadFile(recoveryPath)
	if err != nil {
		return fmt.Errorf("failed to read recovery marker: %w", err)
	}

	var recovery RecoveryInfo
	if err := json.Unmarshal(data, &recovery); err != nil {
		return fmt.Errorf("failed to parse recovery marker: %w", err)
	}

	recovery.PendingFiles = pendingFiles
	if recovery.Metadata == nil {
		recovery.Metadata = make(map[string]interface{})
	}
	recovery.Metadata["completedFiles"] = completedFiles

	data, err = json.MarshalIndent(recovery, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal recovery info: %w", err)
	}

	return os.WriteFile(recoveryPath, data, 0644)
}

// EndRecoveryPoint removes recovery marker after successful operation
func EndRecoveryPoint(ws *Workspace) error {
	recoveryPath := filepath.Join(ws.WorkspaceDir, "recovery.json")
	return os.Remove(recoveryPath)
}

// HasRecoveryPoint checks if a recovery marker exists
func HasRecoveryPoint(ws *Workspace) bool {
	recoveryPath := filepath.Join(ws.WorkspaceDir, "recovery.json")
	_, err := os.Stat(recoveryPath)
	return err == nil
}

// =============================================================================
// OPERATION LOGGING
// =============================================================================

// NewOperationLog creates an operation log for a workspace
func NewOperationLog(ws *Workspace) *OperationLog {
	return &OperationLog{
		ws:      ws,
		logPath: filepath.Join(ws.GetLogsDir(), "operations.log"),
	}
}

// Log writes an entry to the operation log
func (ol *OperationLog) Log(entry LogEntry) error {
	ol.mu.Lock()
	defer ol.mu.Unlock()

	// Ensure log directory exists
	if err := os.MkdirAll(filepath.Dir(ol.logPath), 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	f, err := os.OpenFile(ol.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	_, err = f.Write(append(data, '\n'))
	return err
}

// LogStart logs the start of an operation
func (ol *OperationLog) LogStart(operation, path string) error {
	return ol.Log(LogEntry{
		Timestamp: time.Now(),
		Operation: operation,
		Path:      path,
		Status:    OpStatusStarted,
	})
}

// LogComplete logs the completion of an operation
func (ol *OperationLog) LogComplete(operation, path, checksum string) error {
	return ol.Log(LogEntry{
		Timestamp: time.Now(),
		Operation: operation,
		Path:      path,
		Status:    OpStatusCompleted,
		Checksum:  checksum,
	})
}

// LogFailed logs a failed operation
func (ol *OperationLog) LogFailed(operation, path string, err error) error {
	return ol.Log(LogEntry{
		Timestamp: time.Now(),
		Operation: operation,
		Path:      path,
		Status:    OpStatusFailed,
		Error:     err.Error(),
	})
}

// GetPendingOperations returns operations that started but didn't complete
func (ol *OperationLog) GetPendingOperations() ([]LogEntry, error) {
	ol.mu.Lock()
	defer ol.mu.Unlock()

	data, err := os.ReadFile(ol.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	started := make(map[string]LogEntry) // key = operation:path

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // Skip malformed entries
		}

		key := entry.Operation + ":" + entry.Path

		switch entry.Status {
		case OpStatusStarted:
			started[key] = entry
		case OpStatusCompleted, OpStatusFailed, OpStatusSkipped:
			delete(started, key)
		}
	}

	var pending []LogEntry
	for _, entry := range started {
		pending = append(pending, entry)
	}

	return pending, nil
}

// GetRecentOperations returns the most recent N operations
func (ol *OperationLog) GetRecentOperations(limit int) ([]LogEntry, error) {
	ol.mu.Lock()
	defer ol.mu.Unlock()

	data, err := os.ReadFile(ol.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	var entries []LogEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		entries = append(entries, entry)
	}

	// Return last N entries
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	return entries, nil
}

// Clear removes all entries from the log
func (ol *OperationLog) Clear() error {
	ol.mu.Lock()
	defer ol.mu.Unlock()

	return os.Remove(ol.logPath)
}

// =============================================================================
// CHECKPOINT SUPPORT
// =============================================================================

// Checkpoint represents a named recovery point
type Checkpoint struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	FileHashes  map[string]string `json:"fileHashes"`
	FileList    []string          `json:"fileList"`
}

// CreateCheckpoint creates a named recovery point
func CreateCheckpoint(ws *Workspace, name, description string) (*Checkpoint, error) {
	checkpoint := &Checkpoint{
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		FileHashes:  make(map[string]string),
		FileList:    make([]string, 0),
	}

	// Record current state of workspace files
	for path, entry := range ws.ModifiedFiles {
		checkpoint.FileHashes[path] = entry.CurrentHash
		checkpoint.FileList = append(checkpoint.FileList, path)
	}
	for path, entry := range ws.CreatedFiles {
		checkpoint.FileHashes[path] = entry.CurrentHash
		checkpoint.FileList = append(checkpoint.FileList, path)
	}

	// Save checkpoint
	checkpointPath := filepath.Join(ws.GetCheckpointsDir(), name+".json")
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	if err := os.WriteFile(checkpointPath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write checkpoint: %w", err)
	}

	// Update workspace
	ws.LastCheckpoint = name

	return checkpoint, nil
}

// LoadCheckpoint loads a checkpoint by name
func LoadCheckpoint(ws *Workspace, name string) (*Checkpoint, error) {
	checkpointPath := filepath.Join(ws.GetCheckpointsDir(), name+".json")

	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to parse checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// ListCheckpoints returns all checkpoints for a workspace
func ListCheckpoints(ws *Workspace) ([]*Checkpoint, error) {
	entries, err := os.ReadDir(ws.GetCheckpointsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read checkpoints directory: %w", err)
	}

	var checkpoints []*Checkpoint
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			name := strings.TrimSuffix(entry.Name(), ".json")
			cp, err := LoadCheckpoint(ws, name)
			if err != nil {
				continue // Skip invalid checkpoints
			}
			checkpoints = append(checkpoints, cp)
		}
	}

	return checkpoints, nil
}

// DeleteCheckpoint removes a checkpoint
func DeleteCheckpoint(ws *Workspace, name string) error {
	checkpointPath := filepath.Join(ws.GetCheckpointsDir(), name+".json")
	return os.Remove(checkpointPath)
}
