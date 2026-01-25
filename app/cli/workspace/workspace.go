package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// WORKSPACE ISOLATION SYSTEM
// =============================================================================
//
// This system provides isolated workspaces for Plandex plan execution,
// preventing accidental corruption of user's main project files.
//
// Key concepts:
//   - Workspace: An isolated directory where file edits and commands execute
//   - Copy-on-Write (CoW): Files only copied when modified, not on creation
//   - Commit: Apply workspace changes back to main project
//   - Discard: Reject workspace changes without affecting main project
//
// Benefits:
//   - Safety: User's files protected until explicit commit
//   - Review: Changes can be reviewed before applying
//   - Rollback: Easy to discard unwanted changes
//   - Resume: Workspace state persists across sessions
//
// =============================================================================

// WorkspaceState represents the lifecycle state of a workspace
type WorkspaceState string

const (
	WorkspaceStateActive     WorkspaceState = "active"     // Workspace in use
	WorkspaceStatePending    WorkspaceState = "pending"    // Created but not yet used
	WorkspaceStateCommitted  WorkspaceState = "committed"  // Changes applied to main project
	WorkspaceStateDiscarded  WorkspaceState = "discarded"  // Changes rejected
	WorkspaceStateRecovering WorkspaceState = "recovering" // Crash recovery in progress
)

// Workspace represents an isolated workspace for plan execution
type Workspace struct {
	mu sync.RWMutex

	// Identity
	Id        string         `json:"id"`
	PlanId    string         `json:"planId"`
	Branch    string         `json:"branch"`
	ProjectId string         `json:"projectId"`
	State     WorkspaceState `json:"state"`

	// Paths
	BaseDir      string `json:"baseDir"`      // Original project root
	WorkspaceDir string `json:"workspaceDir"` // Isolated workspace root
	MetadataPath string `json:"metadataPath"` // workspace.json location

	// Copy-on-Write tracking
	ModifiedFiles map[string]*FileEntry `json:"modifiedFiles"` // Files copied/modified
	CreatedFiles  map[string]*FileEntry `json:"createdFiles"`  // New files in workspace
	DeletedFiles  map[string]bool       `json:"deletedFiles"`  // Files marked deleted

	// State tracking
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	LastAccessedAt time.Time `json:"lastAccessedAt"`

	// Recovery info
	LastCheckpoint  string     `json:"lastCheckpoint,omitempty"`
	CrashRecoveryAt *time.Time `json:"crashRecoveryAt,omitempty"`

	// Transaction integration
	ActiveTxId string `json:"activeTxId,omitempty"`
}

// FileEntry tracks a file's status in the workspace
type FileEntry struct {
	Path         string      `json:"path"`
	OriginalHash string      `json:"originalHash,omitempty"` // Hash before modification
	CurrentHash  string      `json:"currentHash"`
	Mode         os.FileMode `json:"mode"`
	Size         int64       `json:"size"`
	ModifiedAt   time.Time   `json:"modifiedAt"`
	IsCopied     bool        `json:"isCopied"` // True if copied from original
}

// WorkspaceConfig holds user preferences for workspace behavior
type WorkspaceConfig struct {
	Enabled             bool `json:"enabled"`             // Master switch (default: true)
	AutoCommitThreshold int  `json:"autoCommitThreshold"` // Auto-commit after N successful applies (0 = disabled)
	RetainDiscarded     bool `json:"retainDiscarded"`     // Keep discarded workspaces for review
	MaxStaleWorkspaces  int  `json:"maxStaleWorkspaces"`  // Max stale workspaces to keep (default: 10)
	StaleAfterDays      int  `json:"staleAfterDays"`      // Days before workspace is stale (default: 7)
	CleanupBatchSize    int  `json:"cleanupBatchSize"`    // Max workspaces to clean per run (default: 5)
}

// DefaultWorkspaceConfig returns the default configuration
func DefaultWorkspaceConfig() *WorkspaceConfig {
	return &WorkspaceConfig{
		Enabled:             true,
		AutoCommitThreshold: 0, // Disabled by default
		RetainDiscarded:     false,
		MaxStaleWorkspaces:  10,
		StaleAfterDays:      7,
		CleanupBatchSize:    5,
	}
}

// WorkspaceSummary provides a brief overview for listing
type WorkspaceSummary struct {
	Id             string         `json:"id"`
	PlanId         string         `json:"planId"`
	Branch         string         `json:"branch"`
	State          WorkspaceState `json:"state"`
	ModifiedCount  int            `json:"modifiedCount"`
	CreatedCount   int            `json:"createdCount"`
	DeletedCount   int            `json:"deletedCount"`
	CreatedAt      time.Time      `json:"createdAt"`
	LastAccessedAt time.Time      `json:"lastAccessedAt"`
}

// WorkspaceReference maps plan/branch to workspace ID
type WorkspaceReference struct {
	Workspaces map[string]map[string]string `json:"workspaces"` // planId -> branch -> workspaceId
	UpdatedAt  time.Time                    `json:"updatedAt"`
}

// =============================================================================
// WORKSPACE METHODS
// =============================================================================

// NewWorkspace creates a new workspace instance
func NewWorkspace(planId, branch, projectId, baseDir, workspacesDir string) *Workspace {
	wsId := generateWorkspaceId()
	wsDir := filepath.Join(workspacesDir, wsId)

	return &Workspace{
		Id:             wsId,
		PlanId:         planId,
		Branch:         branch,
		ProjectId:      projectId,
		State:          WorkspaceStatePending,
		BaseDir:        baseDir,
		WorkspaceDir:   wsDir,
		MetadataPath:   filepath.Join(wsDir, "workspace.json"),
		ModifiedFiles:  make(map[string]*FileEntry),
		CreatedFiles:   make(map[string]*FileEntry),
		DeletedFiles:   make(map[string]bool),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		LastAccessedAt: time.Now(),
	}
}

// Save persists workspace metadata to disk
func (ws *Workspace) Save() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal workspace: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(ws.MetadataPath), 0755); err != nil {
		return fmt.Errorf("failed to create workspace directory: %w", err)
	}

	if err := os.WriteFile(ws.MetadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write workspace metadata: %w", err)
	}

	return nil
}

// Load reads workspace metadata from disk
func LoadWorkspace(metadataPath string) (*Workspace, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read workspace metadata: %w", err)
	}

	var ws Workspace
	if err := json.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workspace: %w", err)
	}

	// Initialize maps if nil (for backwards compatibility)
	if ws.ModifiedFiles == nil {
		ws.ModifiedFiles = make(map[string]*FileEntry)
	}
	if ws.CreatedFiles == nil {
		ws.CreatedFiles = make(map[string]*FileEntry)
	}
	if ws.DeletedFiles == nil {
		ws.DeletedFiles = make(map[string]bool)
	}

	return &ws, nil
}

// GetFilesDir returns the path to the files directory within the workspace
func (ws *Workspace) GetFilesDir() string {
	return filepath.Join(ws.WorkspaceDir, "files")
}

// GetCheckpointsDir returns the path to the checkpoints directory
func (ws *Workspace) GetCheckpointsDir() string {
	return filepath.Join(ws.WorkspaceDir, "checkpoints")
}

// GetLogsDir returns the path to the logs directory
func (ws *Workspace) GetLogsDir() string {
	return filepath.Join(ws.WorkspaceDir, "logs")
}

// GetFilePath returns the workspace path for a given relative file path
func (ws *Workspace) GetFilePath(relativePath string) string {
	return filepath.Join(ws.GetFilesDir(), relativePath)
}

// GetOriginalPath returns the original project path for a given relative path
func (ws *Workspace) GetOriginalPath(relativePath string) string {
	return filepath.Join(ws.BaseDir, relativePath)
}

// HasChanges returns true if there are any pending changes
func (ws *Workspace) HasChanges() bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	return len(ws.ModifiedFiles) > 0 || len(ws.CreatedFiles) > 0 || len(ws.DeletedFiles) > 0
}

// GetChangeCount returns the total number of changes
func (ws *Workspace) GetChangeCount() (modified, created, deleted int) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	return len(ws.ModifiedFiles), len(ws.CreatedFiles), len(ws.DeletedFiles)
}

// GetSummary returns a brief summary of the workspace
func (ws *Workspace) GetSummary() *WorkspaceSummary {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	return &WorkspaceSummary{
		Id:             ws.Id,
		PlanId:         ws.PlanId,
		Branch:         ws.Branch,
		State:          ws.State,
		ModifiedCount:  len(ws.ModifiedFiles),
		CreatedCount:   len(ws.CreatedFiles),
		DeletedCount:   len(ws.DeletedFiles),
		CreatedAt:      ws.CreatedAt,
		LastAccessedAt: ws.LastAccessedAt,
	}
}

// TrackModifiedFile records a file modification in the workspace
func (ws *Workspace) TrackModifiedFile(path string, originalHash, currentHash string, mode os.FileMode, size int64) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.ModifiedFiles[path] = &FileEntry{
		Path:         path,
		OriginalHash: originalHash,
		CurrentHash:  currentHash,
		Mode:         mode,
		Size:         size,
		ModifiedAt:   time.Now(),
		IsCopied:     true,
	}

	// Remove from created if it was there (file existed originally)
	delete(ws.CreatedFiles, path)
	// Remove from deleted if it was there (file being modified again)
	delete(ws.DeletedFiles, path)
}

// TrackCreatedFile records a new file in the workspace
func (ws *Workspace) TrackCreatedFile(path string, hash string, mode os.FileMode, size int64) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.CreatedFiles[path] = &FileEntry{
		Path:        path,
		CurrentHash: hash,
		Mode:        mode,
		Size:        size,
		ModifiedAt:  time.Now(),
		IsCopied:    false,
	}

	// Remove from deleted if it was there (file being re-created)
	delete(ws.DeletedFiles, path)
}

// TrackDeletedFile records a file deletion in the workspace
func (ws *Workspace) TrackDeletedFile(path string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.DeletedFiles[path] = true

	// Remove from modified/created - deletion takes precedence
	delete(ws.ModifiedFiles, path)
	delete(ws.CreatedFiles, path)
}

// IsFileModified checks if a file has been modified in the workspace
func (ws *Workspace) IsFileModified(path string) bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	_, exists := ws.ModifiedFiles[path]
	return exists
}

// IsFileCreated checks if a file was created in the workspace
func (ws *Workspace) IsFileCreated(path string) bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	_, exists := ws.CreatedFiles[path]
	return exists
}

// IsFileDeleted checks if a file was deleted in the workspace
func (ws *Workspace) IsFileDeleted(path string) bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	return ws.DeletedFiles[path]
}

// IsFileInWorkspace checks if a file exists in the workspace (modified or created)
func (ws *Workspace) IsFileInWorkspace(path string) bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	if _, ok := ws.ModifiedFiles[path]; ok {
		return true
	}
	if _, ok := ws.CreatedFiles[path]; ok {
		return true
	}
	return false
}

// GetShortId returns a truncated workspace ID for display
func (ws *Workspace) GetShortId() string {
	if len(ws.Id) > 8 {
		return ws.Id[:8]
	}
	return ws.Id
}

// Touch updates the last accessed timestamp
func (ws *Workspace) Touch() {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.LastAccessedAt = time.Now()
}

// SetState updates the workspace state
func (ws *Workspace) SetState(state WorkspaceState) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.State = state
	ws.UpdatedAt = time.Now()
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// generateWorkspaceId creates a unique workspace identifier
func generateWorkspaceId() string {
	return "ws-" + uuid.New().String()
}

// HashContent computes SHA256 hash of content
func HashContent(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// HashString computes SHA256 hash of a string
func HashString(content string) string {
	return HashContent([]byte(content))
}

// =============================================================================
// WORKSPACE REFERENCE MANAGEMENT
// =============================================================================

// NewWorkspaceReference creates a new reference tracker
func NewWorkspaceReference() *WorkspaceReference {
	return &WorkspaceReference{
		Workspaces: make(map[string]map[string]string),
		UpdatedAt:  time.Now(),
	}
}

// LoadWorkspaceReference loads references from a file
func LoadWorkspaceReference(path string) (*WorkspaceReference, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewWorkspaceReference(), nil
		}
		return nil, fmt.Errorf("failed to read workspace references: %w", err)
	}

	var ref WorkspaceReference
	if err := json.Unmarshal(data, &ref); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workspace references: %w", err)
	}

	if ref.Workspaces == nil {
		ref.Workspaces = make(map[string]map[string]string)
	}

	return &ref, nil
}

// Save persists references to disk
func (ref *WorkspaceReference) Save(path string) error {
	ref.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(ref, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal workspace references: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write workspace references: %w", err)
	}

	return nil
}

// SetWorkspace sets the workspace ID for a plan/branch
func (ref *WorkspaceReference) SetWorkspace(planId, branch, workspaceId string) {
	if ref.Workspaces[planId] == nil {
		ref.Workspaces[planId] = make(map[string]string)
	}
	ref.Workspaces[planId][branch] = workspaceId
}

// GetWorkspace returns the workspace ID for a plan/branch
func (ref *WorkspaceReference) GetWorkspace(planId, branch string) string {
	if branches, ok := ref.Workspaces[planId]; ok {
		return branches[branch]
	}
	return ""
}

// RemoveWorkspace removes a workspace reference
func (ref *WorkspaceReference) RemoveWorkspace(planId, branch string) {
	if branches, ok := ref.Workspaces[planId]; ok {
		delete(branches, branch)
		if len(branches) == 0 {
			delete(ref.Workspaces, planId)
		}
	}
}
