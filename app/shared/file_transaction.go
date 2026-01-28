package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// TRANSACTIONAL FILE-WRITE MECHANISM
// =============================================================================
//
// This system provides ACID-like guarantees for file operations during plan
// execution, with automatic rollback on provider failure.
//
// Key concepts:
//   - Transaction: A group of file operations that succeed or fail together
//   - Write-Ahead Log (WAL): Records intended changes before applying them
//   - Snapshot: Backup of original file state for rollback
//   - Checkpoint: Named recovery point within a transaction
//
// Guarantees:
//   - Atomicity: All changes in a transaction apply, or none do
//   - Consistency: Files are never left in partial/corrupt state
//   - Isolation: Concurrent transactions don't interfere (via locking)
//   - Durability: Committed changes persist; uncommitted can be rolled back
//
// =============================================================================

// TransactionState represents the current state of a transaction
type TransactionState string

const (
	TxStateActive     TransactionState = "active"      // Transaction in progress
	TxStatePreparing  TransactionState = "preparing"   // Preparing to commit
	TxStateCommitted  TransactionState = "committed"   // Successfully committed
	TxStateRolledBack TransactionState = "rolled_back" // Rolled back
	TxStateFailed     TransactionState = "failed"      // Failed, needs cleanup
)

// FileOperationType identifies the type of file operation
type FileOperationType string

const (
	FileOpCreate FileOperationType = "create" // Create new file
	FileOpModify FileOperationType = "modify" // Modify existing file
	FileOpDelete FileOperationType = "delete" // Delete file
	FileOpRename FileOperationType = "rename" // Rename/move file
)

// =============================================================================
// CORE TYPES
// =============================================================================

// FileTransaction manages a group of file operations atomically
type FileTransaction struct {
	mu sync.Mutex

	// Identity
	Id        string           `json:"id"`
	PlanId    string           `json:"planId"`
	Branch    string           `json:"branch"`
	CreatedAt time.Time        `json:"createdAt"`
	State     TransactionState `json:"state"`

	// Configuration
	BaseDir     string `json:"baseDir"`     // Root directory for operations
	SnapshotDir string `json:"snapshotDir"` // Where to store backups
	WALPath     string `json:"walPath"`     // Write-ahead log file

	// Operations tracking
	Operations  []FileOperation          `json:"operations"`
	Snapshots   map[string]*FileSnapshot `json:"snapshots"`
	Checkpoints map[string]*TxCheckpoint `json:"checkpoints"`

	// Execution state
	CurrentOp      int        `json:"currentOp"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	RollbackReason string     `json:"rollbackReason,omitempty"`

	// Provider failure tracking
	ProviderError *ProviderFailure `json:"providerError,omitempty"`
}

// FileOperation represents a single file operation within a transaction
type FileOperation struct {
	// Identity
	Seq       int               `json:"seq"` // Sequence number (1-indexed)
	Type      FileOperationType `json:"type"`
	Timestamp time.Time         `json:"timestamp"`

	// Target
	Path    string `json:"path"`              // Target file path
	NewPath string `json:"newPath,omitempty"` // For rename operations

	// Content (for create/modify)
	Content     string `json:"content,omitempty"`
	ContentHash string `json:"contentHash,omitempty"`

	// Execution state
	Status       FileOpStatus `json:"status"`
	AppliedAt    *time.Time   `json:"appliedAt,omitempty"`
	RolledBackAt *time.Time   `json:"rolledBackAt,omitempty"`
	Error        string       `json:"error,omitempty"`

	// Snapshot reference (for rollback)
	SnapshotId string `json:"snapshotId,omitempty"`
}

// FileOpStatus tracks the status of an operation
type FileOpStatus string

const (
	FileOpPending    FileOpStatus = "pending"     // Not yet applied
	FileOpApplied    FileOpStatus = "applied"     // Successfully applied
	FileOpRolledBack FileOpStatus = "rolled_back" // Rolled back
	FileOpFailed     FileOpStatus = "failed"      // Failed to apply
	FileOpSkipped    FileOpStatus = "skipped"     // Skipped (e.g., file doesn't exist for delete)
)

// FileSnapshot stores the original state of a file for rollback
type FileSnapshot struct {
	Id          string      `json:"id"`
	Path        string      `json:"path"`
	Existed     bool        `json:"existed"`           // Did file exist before?
	Content     string      `json:"content,omitempty"` // Original content (if existed)
	ContentHash string      `json:"contentHash,omitempty"`
	Mode        os.FileMode `json:"mode,omitempty"` // Original permissions
	CapturedAt  time.Time   `json:"capturedAt"`

	// For large files, store to disk instead of memory
	StoredOnDisk bool   `json:"storedOnDisk"`
	DiskPath     string `json:"diskPath,omitempty"`
}

// TxCheckpoint represents a named recovery point
type TxCheckpoint struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	AfterOp     int       `json:"afterOp"` // Checkpoint is after this operation

	// State at checkpoint
	AppliedOps   []int             `json:"appliedOps"`
	FileHashes   map[string]string `json:"fileHashes"`
	FileContents map[string]string `json:"fileContents,omitempty"` // Actual content at checkpoint
}

// =============================================================================
// WRITE-AHEAD LOG (WAL)
// =============================================================================

// WALEntry represents an entry in the write-ahead log
type WALEntry struct {
	Seq         int             `json:"seq"`
	Timestamp   time.Time       `json:"timestamp"`
	TxId        string          `json:"txId"`
	EntryType   WALEntryType    `json:"entryType"`
	Operation   *FileOperation  `json:"operation,omitempty"`
	Checkpoint  *TxCheckpoint   `json:"checkpoint,omitempty"`
	StateChange *WALStateChange `json:"stateChange,omitempty"`
}

// WALEntryType identifies the type of WAL entry
type WALEntryType string

const (
	WALEntryTxStart    WALEntryType = "tx_start"
	WALEntryOpIntent   WALEntryType = "op_intent"   // About to apply operation
	WALEntryOpComplete WALEntryType = "op_complete" // Operation applied
	WALEntryOpRollback WALEntryType = "op_rollback" // Operation rolled back
	WALEntryCheckpoint WALEntryType = "checkpoint"
	WALEntryTxCommit   WALEntryType = "tx_commit"
	WALEntryTxRollback WALEntryType = "tx_rollback"
	WALEntryTxFailed   WALEntryType = "tx_failed"
)

// WALStateChange records a transaction state change
type WALStateChange struct {
	OldState TransactionState `json:"oldState"`
	NewState TransactionState `json:"newState"`
	Reason   string           `json:"reason,omitempty"`
}

// =============================================================================
// TRANSACTION MANAGEMENT
// =============================================================================

// NewFileTransaction creates a new transaction
func NewFileTransaction(planId, branch, baseDir string) *FileTransaction {
	txId := generateTxId()
	snapshotDir := filepath.Join(baseDir, ".plandex", "snapshots", txId)
	walPath := filepath.Join(baseDir, ".plandex", "wal", txId+".wal")

	return &FileTransaction{
		Id:          txId,
		PlanId:      planId,
		Branch:      branch,
		CreatedAt:   time.Now(),
		State:       TxStateActive,
		BaseDir:     baseDir,
		SnapshotDir: snapshotDir,
		WALPath:     walPath,
		Operations:  []FileOperation{},
		Snapshots:   make(map[string]*FileSnapshot),
		Checkpoints: make(map[string]*TxCheckpoint),
	}
}

// Begin starts the transaction
func (tx *FileTransaction) Begin() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.State != TxStateActive {
		return fmt.Errorf("transaction already started (state: %s)", tx.State)
	}

	// Create directories
	if err := os.MkdirAll(tx.SnapshotDir, 0755); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(tx.WALPath), 0755); err != nil {
		return fmt.Errorf("failed to create WAL directory: %w", err)
	}

	now := time.Now()
	tx.StartedAt = &now

	// Write WAL entry
	return tx.writeWAL(WALEntry{
		Seq:       1,
		Timestamp: now,
		TxId:      tx.Id,
		EntryType: WALEntryTxStart,
	})
}

// =============================================================================
// FILE OPERATIONS
// =============================================================================

// CreateFile stages a file creation
func (tx *FileTransaction) CreateFile(path, content string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.State != TxStateActive {
		return fmt.Errorf("transaction not active (state: %s)", tx.State)
	}

	fullPath := tx.resolvePath(path)

	// Snapshot existing state (in case file already exists)
	if err := tx.captureSnapshot(fullPath); err != nil {
		return fmt.Errorf("failed to capture snapshot: %w", err)
	}

	op := FileOperation{
		Seq:         len(tx.Operations) + 1,
		Type:        FileOpCreate,
		Timestamp:   time.Now(),
		Path:        fullPath,
		Content:     content,
		ContentHash: hashString(content),
		Status:      FileOpPending,
		SnapshotId:  fullPath,
	}

	tx.Operations = append(tx.Operations, op)

	// Write intent to WAL
	return tx.writeWAL(WALEntry{
		Seq:       len(tx.Operations) + 1,
		Timestamp: time.Now(),
		TxId:      tx.Id,
		EntryType: WALEntryOpIntent,
		Operation: &op,
	})
}

// ModifyFile stages a file modification
func (tx *FileTransaction) ModifyFile(path, newContent string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.State != TxStateActive {
		return fmt.Errorf("transaction not active (state: %s)", tx.State)
	}

	fullPath := tx.resolvePath(path)

	// Must capture snapshot before modification
	if err := tx.captureSnapshot(fullPath); err != nil {
		return fmt.Errorf("failed to capture snapshot: %w", err)
	}

	op := FileOperation{
		Seq:         len(tx.Operations) + 1,
		Type:        FileOpModify,
		Timestamp:   time.Now(),
		Path:        fullPath,
		Content:     newContent,
		ContentHash: hashString(newContent),
		Status:      FileOpPending,
		SnapshotId:  fullPath,
	}

	tx.Operations = append(tx.Operations, op)

	return tx.writeWAL(WALEntry{
		Seq:       len(tx.Operations) + 1,
		Timestamp: time.Now(),
		TxId:      tx.Id,
		EntryType: WALEntryOpIntent,
		Operation: &op,
	})
}

// DeleteFile stages a file deletion
func (tx *FileTransaction) DeleteFile(path string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.State != TxStateActive {
		return fmt.Errorf("transaction not active (state: %s)", tx.State)
	}

	fullPath := tx.resolvePath(path)

	// Must capture snapshot before deletion
	if err := tx.captureSnapshot(fullPath); err != nil {
		return fmt.Errorf("failed to capture snapshot: %w", err)
	}

	op := FileOperation{
		Seq:        len(tx.Operations) + 1,
		Type:       FileOpDelete,
		Timestamp:  time.Now(),
		Path:       fullPath,
		Status:     FileOpPending,
		SnapshotId: fullPath,
	}

	tx.Operations = append(tx.Operations, op)

	return tx.writeWAL(WALEntry{
		Seq:       len(tx.Operations) + 1,
		Timestamp: time.Now(),
		TxId:      tx.Id,
		EntryType: WALEntryOpIntent,
		Operation: &op,
	})
}

// RenameFile stages a file rename/move
func (tx *FileTransaction) RenameFile(oldPath, newPath string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.State != TxStateActive {
		return fmt.Errorf("transaction not active (state: %s)", tx.State)
	}

	fullOldPath := tx.resolvePath(oldPath)
	fullNewPath := tx.resolvePath(newPath)

	// Capture snapshots for both paths
	if err := tx.captureSnapshot(fullOldPath); err != nil {
		return fmt.Errorf("failed to capture snapshot for source: %w", err)
	}
	if err := tx.captureSnapshot(fullNewPath); err != nil {
		return fmt.Errorf("failed to capture snapshot for destination: %w", err)
	}

	op := FileOperation{
		Seq:        len(tx.Operations) + 1,
		Type:       FileOpRename,
		Timestamp:  time.Now(),
		Path:       fullOldPath,
		NewPath:    fullNewPath,
		Status:     FileOpPending,
		SnapshotId: fullOldPath,
	}

	tx.Operations = append(tx.Operations, op)

	return tx.writeWAL(WALEntry{
		Seq:       len(tx.Operations) + 1,
		Timestamp: time.Now(),
		TxId:      tx.Id,
		EntryType: WALEntryOpIntent,
		Operation: &op,
	})
}

// =============================================================================
// APPLY OPERATIONS
// =============================================================================

// ApplyNext applies the next pending operation
func (tx *FileTransaction) ApplyNext() (*FileOperation, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.State != TxStateActive {
		return nil, fmt.Errorf("transaction not active (state: %s)", tx.State)
	}

	// Find next pending operation
	var op *FileOperation
	for i := range tx.Operations {
		if tx.Operations[i].Status == FileOpPending {
			op = &tx.Operations[i]
			tx.CurrentOp = i
			break
		}
	}

	if op == nil {
		return nil, nil // No more pending operations
	}

	// Apply the operation
	if err := tx.applyOperation(op); err != nil {
		op.Status = FileOpFailed
		op.Error = err.Error()
		return op, err
	}

	now := time.Now()
	op.Status = FileOpApplied
	op.AppliedAt = &now

	// Write completion to WAL
	tx.writeWAL(WALEntry{
		Seq:       op.Seq,
		Timestamp: now,
		TxId:      tx.Id,
		EntryType: WALEntryOpComplete,
		Operation: op,
	})

	return op, nil
}

// ApplyAll applies all pending operations
func (tx *FileTransaction) ApplyAll() error {
	for {
		op, err := tx.ApplyNext()
		if err != nil {
			return fmt.Errorf("operation %d failed: %w", op.Seq, err)
		}
		if op == nil {
			break // No more operations
		}
	}
	return nil
}

// applyOperation performs the actual file system operation
func (tx *FileTransaction) applyOperation(op *FileOperation) error {
	switch op.Type {
	case FileOpCreate, FileOpModify:
		// Ensure directory exists
		dir := filepath.Dir(op.Path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		// Write file
		if err := os.WriteFile(op.Path, []byte(op.Content), 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}

	case FileOpDelete:
		if err := os.Remove(op.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete file: %w", err)
		}

	case FileOpRename:
		// Ensure destination directory exists
		dir := filepath.Dir(op.NewPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create destination directory: %w", err)
		}
		if err := os.Rename(op.Path, op.NewPath); err != nil {
			return fmt.Errorf("failed to rename file: %w", err)
		}

	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}

	return nil
}

// =============================================================================
// COMMIT AND ROLLBACK
// =============================================================================

// Commit finalizes the transaction
func (tx *FileTransaction) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.State != TxStateActive {
		return fmt.Errorf("cannot commit: transaction not active (state: %s)", tx.State)
	}

	// Verify all operations are applied
	for _, op := range tx.Operations {
		if op.Status == FileOpPending {
			return fmt.Errorf("cannot commit: operation %d is still pending", op.Seq)
		}
		if op.Status == FileOpFailed {
			return fmt.Errorf("cannot commit: operation %d failed", op.Seq)
		}
	}

	tx.State = TxStateCommitted
	now := time.Now()
	tx.CompletedAt = &now

	// Write commit to WAL
	if err := tx.writeWAL(WALEntry{
		Seq:       len(tx.Operations) + 100,
		Timestamp: now,
		TxId:      tx.Id,
		EntryType: WALEntryTxCommit,
		StateChange: &WALStateChange{
			OldState: TxStateActive,
			NewState: TxStateCommitted,
		},
	}); err != nil {
		return err
	}

	// Clean up snapshots (no longer needed after commit)
	tx.cleanupSnapshots()

	return nil
}

// Rollback reverts all applied operations
func (tx *FileTransaction) Rollback(reason string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.State == TxStateCommitted {
		return fmt.Errorf("cannot rollback: transaction already committed")
	}
	if tx.State == TxStateRolledBack {
		return nil // Already rolled back
	}

	tx.RollbackReason = reason

	// Write rollback intent to WAL
	tx.writeWAL(WALEntry{
		Seq:       len(tx.Operations) + 200,
		Timestamp: time.Now(),
		TxId:      tx.Id,
		EntryType: WALEntryTxRollback,
		StateChange: &WALStateChange{
			OldState: tx.State,
			NewState: TxStateRolledBack,
			Reason:   reason,
		},
	})

	// Rollback operations in reverse order
	for i := len(tx.Operations) - 1; i >= 0; i-- {
		op := &tx.Operations[i]
		if op.Status != FileOpApplied {
			continue // Only rollback applied operations
		}

		if err := tx.rollbackOperation(op); err != nil {
			// Log but continue - try to rollback as much as possible
			op.Error = fmt.Sprintf("rollback failed: %v", err)
		} else {
			now := time.Now()
			op.Status = FileOpRolledBack
			op.RolledBackAt = &now
		}
	}

	tx.State = TxStateRolledBack
	now := time.Now()
	tx.CompletedAt = &now

	// Clean up snapshots
	tx.cleanupSnapshots()

	return nil
}

// RollbackOnProviderFailure handles rollback due to provider failure
func (tx *FileTransaction) RollbackOnProviderFailure(failure *ProviderFailure) error {
	tx.ProviderError = failure

	reason := fmt.Sprintf("Provider failure: %s - %s", failure.Type, failure.Message)
	return tx.Rollback(reason)
}

// rollbackOperation restores a file to its original state
func (tx *FileTransaction) rollbackOperation(op *FileOperation) error {
	snapshot, exists := tx.Snapshots[op.SnapshotId]
	if !exists {
		return fmt.Errorf("snapshot not found for operation %d", op.Seq)
	}

	switch op.Type {
	case FileOpCreate:
		// File was created - delete it if it didn't exist before
		if !snapshot.Existed {
			return os.Remove(op.Path)
		}
		// If file existed, restore original content
		return tx.restoreFromSnapshot(snapshot)

	case FileOpModify:
		// Restore original content
		return tx.restoreFromSnapshot(snapshot)

	case FileOpDelete:
		// File was deleted - restore it
		return tx.restoreFromSnapshot(snapshot)

	case FileOpRename:
		// Rename back
		if err := os.Rename(op.NewPath, op.Path); err != nil {
			return err
		}
		// Also restore destination if something was there
		destSnapshot, exists := tx.Snapshots[op.NewPath]
		if exists && destSnapshot.Existed {
			return tx.restoreFromSnapshot(destSnapshot)
		}
		return nil

	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}

// restoreFromSnapshot restores a file from its snapshot
func (tx *FileTransaction) restoreFromSnapshot(snapshot *FileSnapshot) error {
	if !snapshot.Existed {
		// File didn't exist - ensure it's deleted
		return os.Remove(snapshot.Path)
	}

	var content string
	if snapshot.StoredOnDisk {
		data, err := os.ReadFile(snapshot.DiskPath)
		if err != nil {
			return fmt.Errorf("failed to read snapshot from disk: %w", err)
		}
		content = string(data)
	} else {
		content = snapshot.Content
	}

	// Ensure directory exists
	dir := filepath.Dir(snapshot.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write original content
	mode := snapshot.Mode
	if mode == 0 {
		mode = 0644
	}
	return os.WriteFile(snapshot.Path, []byte(content), mode)
}

// =============================================================================
// CHECKPOINTS
// =============================================================================

// CreateCheckpoint creates a named recovery point
func (tx *FileTransaction) CreateCheckpoint(name, description string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.State != TxStateActive {
		return fmt.Errorf("cannot create checkpoint: transaction not active")
	}

	// Collect applied operations
	appliedOps := []int{}
	for _, op := range tx.Operations {
		if op.Status == FileOpApplied {
			appliedOps = append(appliedOps, op.Seq)
		}
	}

	// Capture current file state (hashes AND content for rollback)
	fileHashes := make(map[string]string)
	fileContents := make(map[string]string)
	for path := range tx.Snapshots {
		if content, err := os.ReadFile(path); err == nil {
			fileHashes[path] = hashString(string(content))
			fileContents[path] = string(content)
		}
	}

	checkpoint := &TxCheckpoint{
		Name:         name,
		Description:  description,
		CreatedAt:    time.Now(),
		AfterOp:      len(tx.Operations),
		AppliedOps:   appliedOps,
		FileHashes:   fileHashes,
		FileContents: fileContents,
	}

	tx.Checkpoints[name] = checkpoint

	// Write to WAL
	return tx.writeWAL(WALEntry{
		Seq:        len(tx.Operations) + 50,
		Timestamp:  time.Now(),
		TxId:       tx.Id,
		EntryType:  WALEntryCheckpoint,
		Checkpoint: checkpoint,
	})
}

// RollbackToCheckpoint rolls back to a named checkpoint
func (tx *FileTransaction) RollbackToCheckpoint(name string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	checkpoint, exists := tx.Checkpoints[name]
	if !exists {
		return fmt.Errorf("checkpoint not found: %s", name)
	}

	// Rollback operations after the checkpoint (in reverse order)
	// We need to restore files to their state AT the checkpoint, not their original state
	for i := len(tx.Operations) - 1; i >= checkpoint.AfterOp; i-- {
		op := &tx.Operations[i]
		if op.Status != FileOpApplied {
			continue
		}

		// For checkpoint rollback, we restore to checkpoint state, not original
		if err := tx.rollbackOperationToCheckpoint(op, checkpoint); err != nil {
			op.Error = fmt.Sprintf("rollback failed: %v", err)
		} else {
			now := time.Now()
			op.Status = FileOpRolledBack
			op.RolledBackAt = &now
		}
	}

	return nil
}

// rollbackOperationToCheckpoint restores a file to its checkpoint state
func (tx *FileTransaction) rollbackOperationToCheckpoint(op *FileOperation, checkpoint *TxCheckpoint) error {
	// Get the content at checkpoint time
	checkpointContent, hasCheckpointContent := checkpoint.FileContents[op.Path]

	if !hasCheckpointContent {
		// File wasn't tracked at checkpoint - use full rollback
		return tx.rollbackOperation(op)
	}

	// Restore to checkpoint state
	return os.WriteFile(op.Path, []byte(checkpointContent), 0644)
}

// =============================================================================
// SNAPSHOT MANAGEMENT
// =============================================================================

// captureSnapshot captures the current state of a file and persists it to the
// snapshot directory so that crash recovery can restore state without
// relying on in-memory data.
func (tx *FileTransaction) captureSnapshot(path string) error {
	// Don't re-capture if we already have a snapshot
	if _, exists := tx.Snapshots[path]; exists {
		return nil
	}

	if err := os.MkdirAll(tx.SnapshotDir, 0755); err != nil {
		return err
	}

	snapshot := &FileSnapshot{
		Id:         path,
		Path:       path,
		CapturedAt: time.Now(),
	}

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		snapshot.Existed = false
		tx.Snapshots[path] = snapshot
		return tx.writeSnapshotMeta(snapshot)
	}
	if err != nil {
		return err
	}

	snapshot.Existed = true
	snapshot.Mode = info.Mode()

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	snapshot.ContentHash = hashString(string(content))

	// Always persist content to disk for crash recovery durability.
	snapshot.StoredOnDisk = true
	snapshot.DiskPath = filepath.Join(tx.SnapshotDir, hashString(path)+".snapshot")
	if err := os.WriteFile(snapshot.DiskPath, content, 0644); err != nil {
		return err
	}

	// Keep small files in memory as well for fast rollback without disk I/O.
	const maxInMemorySize = 1024 * 1024
	if len(content) <= maxInMemorySize {
		snapshot.Content = string(content)
	}

	tx.Snapshots[path] = snapshot
	return tx.writeSnapshotMeta(snapshot)
}

// writeSnapshotMeta persists snapshot metadata as JSON so RecoverTransaction
// can reload it without the in-memory Snapshots map.
func (tx *FileTransaction) writeSnapshotMeta(snap *FileSnapshot) error {
	metaPath := filepath.Join(tx.SnapshotDir, hashString(snap.Path)+".meta.json")
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0644)
}

// cleanupSnapshots removes snapshot files after transaction completes
func (tx *FileTransaction) cleanupSnapshots() {
	// Remove snapshot directory
	os.RemoveAll(tx.SnapshotDir)

	// Remove WAL file
	os.Remove(tx.WALPath)
}

// =============================================================================
// WRITE-AHEAD LOG
// =============================================================================

// writeWAL appends an entry to the write-ahead log
func (tx *FileTransaction) writeWAL(entry WALEntry) error {
	f, err := os.OpenFile(tx.WALPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open WAL: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal WAL entry: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write WAL entry: %w", err)
	}

	return f.Sync() // Ensure durability
}

// =============================================================================
// RECOVERY
// =============================================================================

// RecoverTransaction attempts to recover a transaction from its WAL
func RecoverTransaction(walPath string) (*FileTransaction, error) {
	data, err := os.ReadFile(walPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read WAL: %w", err)
	}

	// Parse WAL entries
	var entries []WALEntry
	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var entry WALEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // Skip malformed entries
		}
		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no valid WAL entries found")
	}

	// Reconstruct transaction state
	tx := &FileTransaction{
		WALPath:     walPath,
		State:       TxStateActive,
		Operations:  []FileOperation{},
		Snapshots:   make(map[string]*FileSnapshot),
		Checkpoints: make(map[string]*TxCheckpoint),
	}

	for _, entry := range entries {
		switch entry.EntryType {
		case WALEntryTxStart:
			tx.Id = entry.TxId
			tx.StartedAt = &entry.Timestamp

		case WALEntryOpIntent:
			if entry.Operation != nil {
				tx.Operations = append(tx.Operations, *entry.Operation)
			}

		case WALEntryOpComplete:
			if entry.Operation != nil {
				for i := range tx.Operations {
					if tx.Operations[i].Seq == entry.Operation.Seq {
						tx.Operations[i].Status = FileOpApplied
						tx.Operations[i].AppliedAt = entry.Operation.AppliedAt
						break
					}
				}
			}

		case WALEntryTxCommit:
			tx.State = TxStateCommitted

		case WALEntryTxRollback:
			tx.State = TxStateRolledBack
		}
	}

	// Derive snapshot directory from WAL path: wal/<txId>.wal -> snapshots/<txId>
	walDir := filepath.Dir(walPath)
	plandexDir := filepath.Dir(walDir)
	tx.SnapshotDir = filepath.Join(plandexDir, "snapshots", tx.Id)
	tx.BaseDir = filepath.Dir(plandexDir)

	// Reload persisted snapshots so rollback can restore files
	tx.loadSnapshotsFromDisk()

	// If transaction is still active, we need to decide whether to rollback or continue
	if tx.State == TxStateActive {
		// By default, rollback incomplete transactions on recovery
		tx.Rollback("Recovered from crash - rolling back incomplete transaction")
	}

	return tx, nil
}

// loadSnapshotsFromDisk reloads snapshot metadata and content from the
// snapshot directory.  This is used during crash recovery when the
// in-memory Snapshots map is empty.
func (tx *FileTransaction) loadSnapshotsFromDisk() {
	entries, err := os.ReadDir(tx.SnapshotDir)
	if err != nil {
		return // snapshot dir may not exist if nothing was staged
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".meta.json") {
			continue
		}
		metaPath := filepath.Join(tx.SnapshotDir, entry.Name())
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var snap FileSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		// If content was stored on disk, reload it
		if snap.StoredOnDisk && snap.DiskPath != "" {
			if content, err := os.ReadFile(snap.DiskPath); err == nil {
				snap.Content = string(content)
			}
		}
		tx.Snapshots[snap.Path] = &snap
	}
}

// =============================================================================
// HELPERS
// =============================================================================

func (tx *FileTransaction) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(tx.BaseDir, path)
}

func generateTxId() string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// =============================================================================
// USAGE EXAMPLE
// =============================================================================
//
// tx := NewFileTransaction(planId, branch, "/project/root")
// tx.Begin()
//
// // Stage operations (nothing written yet)
// tx.CreateFile("src/new_file.go", "package main...")
// tx.ModifyFile("src/existing.go", "modified content...")
// tx.DeleteFile("src/deprecated.go")
//
// // Create checkpoint before risky operations
// tx.CreateCheckpoint("before_refactor", "Before major refactor")
//
// // Apply operations one at a time
// for {
//     op, err := tx.ApplyNext()
//     if err != nil {
//         // Provider failed mid-operation
//         tx.RollbackOnProviderFailure(&ProviderFailure{...})
//         break
//     }
//     if op == nil {
//         break // All done
//     }
// }
//
// // If all succeeded, commit
// if tx.State == TxStateActive {
//     tx.Commit()
// }
//
// =============================================================================
