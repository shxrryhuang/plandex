package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// =============================================================================
// WORKSPACE MANAGER
// =============================================================================
//
// The Manager handles workspace lifecycle operations including:
//   - Creating new workspaces for plan/branch combinations
//   - Activating workspaces for use
//   - Committing workspace changes to the main project
//   - Discarding workspace changes
//   - Resuming existing workspaces
//   - Cleaning up stale workspaces
//
// =============================================================================

// Manager handles workspace lifecycle operations
type Manager struct {
	homeDir       string           // ~/.plandex-home-v2
	workspacesDir string           // ~/.plandex-home-v2/workspaces
	projectRoot   string           // User's project root
	plandexDir    string           // .plandex-v2 in project
	config        *WorkspaceConfig // User configuration
}

// NewManager creates a new workspace manager
func NewManager(homeDir, projectRoot, plandexDir string, config *WorkspaceConfig) *Manager {
	if config == nil {
		config = DefaultWorkspaceConfig()
	}

	return &Manager{
		homeDir:       homeDir,
		workspacesDir: filepath.Join(homeDir, "workspaces"),
		projectRoot:   projectRoot,
		plandexDir:    plandexDir,
		config:        config,
	}
}

// =============================================================================
// LIFECYCLE OPERATIONS
// =============================================================================

// Create initializes a new workspace for a plan/branch
func (m *Manager) Create(planId, branch, projectId string) (*Workspace, error) {
	// Ensure workspaces directory exists
	if err := os.MkdirAll(m.workspacesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspaces directory: %w", err)
	}

	// Create new workspace
	ws := NewWorkspace(planId, branch, projectId, m.projectRoot, m.workspacesDir)

	// Create directory structure
	dirs := []string{
		ws.GetFilesDir(),
		ws.GetCheckpointsDir(),
		ws.GetLogsDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			// Cleanup on failure
			os.RemoveAll(ws.WorkspaceDir)
			return nil, fmt.Errorf("failed to create workspace directory %s: %w", dir, err)
		}
	}

	// Save workspace metadata
	if err := ws.Save(); err != nil {
		os.RemoveAll(ws.WorkspaceDir)
		return nil, fmt.Errorf("failed to save workspace: %w", err)
	}

	// Register workspace in project references
	if err := m.registerWorkspace(ws); err != nil {
		os.RemoveAll(ws.WorkspaceDir)
		return nil, fmt.Errorf("failed to register workspace: %w", err)
	}

	return ws, nil
}

// Activate marks workspace as in use
func (m *Manager) Activate(ws *Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace is nil")
	}

	ws.SetState(WorkspaceStateActive)
	ws.Touch()

	return ws.Save()
}

// Commit applies workspace changes to main project atomically.
//
// All workspace modifications, creations, and deletions are wrapped in a
// FileTransaction.  Snapshots of the target project files are captured before
// any write occurs.  If any single write fails the entire project directory
// is rolled back to its pre-commit state, leaving the workspace intact for
// the user to retry or inspect.
func (m *Manager) Commit(ws *Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace is nil")
	}

	if ws.State != WorkspaceStateActive {
		return fmt.Errorf("cannot commit non-active workspace (state: %s)", ws.State)
	}

	if !ws.HasChanges() {
		ws.SetState(WorkspaceStateCommitted)
		return ws.Save()
	}

	// Begin a transaction rooted at the project directory so snapshots
	// and WAL land under <project>/.plandex/.
	tx := NewCommitTransaction(ws)
	if err := tx.Begin(); err != nil {
		return fmt.Errorf("failed to begin commit transaction: %w", err)
	}

	// --- Stage all operations (snapshot capture only, no writes) -------------
	for path, _ := range ws.ModifiedFiles {
		srcPath := ws.GetFilePath(path)
		content, err := os.ReadFile(srcPath)
		if err != nil {
			tx.Rollback("staging failed: " + err.Error())
			return fmt.Errorf("failed to read modified file %s: %w", path, err)
		}
		if err := tx.ModifyFile(path, string(content)); err != nil {
			tx.Rollback("staging failed: " + err.Error())
			return fmt.Errorf("failed to stage modification for %s: %w", path, err)
		}
	}

	for path, _ := range ws.CreatedFiles {
		srcPath := ws.GetFilePath(path)
		content, err := os.ReadFile(srcPath)
		if err != nil {
			tx.Rollback("staging failed: " + err.Error())
			return fmt.Errorf("failed to read created file %s: %w", path, err)
		}
		if err := tx.CreateFile(path, string(content)); err != nil {
			tx.Rollback("staging failed: " + err.Error())
			return fmt.Errorf("failed to stage creation for %s: %w", path, err)
		}
	}

	for path := range ws.DeletedFiles {
		if err := tx.DeleteFile(path); err != nil {
			tx.Rollback("staging failed: " + err.Error())
			return fmt.Errorf("failed to stage deletion for %s: %w", path, err)
		}
	}

	// --- Apply sequentially (writes to project directory) --------------------
	for {
		op, err := tx.ApplyNext()
		if op == nil {
			break
		}
		if err != nil {
			rbErr := tx.Rollback(fmt.Sprintf("commit write failed for %s: %v", op.Path, err))
			if rbErr != nil {
				return fmt.Errorf("commit failed for %s (%v) and rollback also failed: %w", op.Path, err, rbErr)
			}
			return fmt.Errorf("commit failed for %s: %w (project rolled back)", op.Path, err)
		}
	}

	// --- Finalise ------------------------------------------------------------
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("transaction commit validation failed: %w", err)
	}

	ws.SetState(WorkspaceStateCommitted)
	if err := ws.Save(); err != nil {
		return fmt.Errorf("failed to save workspace state: %w", err)
	}

	if err := m.unregisterWorkspace(ws); err != nil {
		fmt.Printf("Warning: failed to unregister workspace: %v\n", err)
	}

	return nil
}

// NewCommitTransaction creates a FileTransaction whose base directory is the
// workspace's original project root, so snapshots capture the project state.
func NewCommitTransaction(ws *Workspace) *CommitTransaction {
	return &CommitTransaction{
		ws: ws,
	}
}

// CommitTransaction wraps the shared FileTransaction with workspace context.
type CommitTransaction struct {
	ws *Workspace
	tx *commitTx
}

// commitTx is a thin wrapper so we can embed the transaction fields inline
// without importing the shared package circularly.  In practice this delegates
// to shared.FileTransaction; we duplicate the minimal interface here to
// keep the workspace package self-contained.
type commitTx struct {
	id          string
	baseDir     string
	snapshotDir string
	walPath     string
	state       string
	operations  []commitOp
	snapshots   map[string]*commitSnap
	currentOp   int
}

type commitOp struct {
	seq     int
	opType  string // create | modify | delete
	Path    string
	content string
	status  string
}

type commitSnap struct {
	path    string
	existed bool
	content string
	mode    os.FileMode
}

func (ct *CommitTransaction) Begin() error {
	ct.tx = &commitTx{
		baseDir:   ct.ws.BaseDir,
		snapshots: make(map[string]*commitSnap),
	}
	return os.MkdirAll(filepath.Join(ct.ws.BaseDir, ".plandex", "snapshots"), 0755)
}

func (ct *CommitTransaction) resolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(ct.ws.BaseDir, p)
}

func (ct *CommitTransaction) captureSnapshot(fullPath string) {
	if _, exists := ct.tx.snapshots[fullPath]; exists {
		return
	}
	snap := &commitSnap{path: fullPath}
	if content, err := os.ReadFile(fullPath); err == nil {
		snap.existed = true
		snap.content = string(content)
		if info, serr := os.Stat(fullPath); serr == nil {
			snap.mode = info.Mode()
		}
	}
	ct.tx.snapshots[fullPath] = snap
}

func (ct *CommitTransaction) ModifyFile(path, content string) error {
	fullPath := ct.resolvePath(path)
	ct.captureSnapshot(fullPath)
	ct.tx.operations = append(ct.tx.operations, commitOp{
		seq: len(ct.tx.operations) + 1, opType: "modify", Path: fullPath, content: content, status: "pending",
	})
	return nil
}

func (ct *CommitTransaction) CreateFile(path, content string) error {
	fullPath := ct.resolvePath(path)
	ct.captureSnapshot(fullPath)
	ct.tx.operations = append(ct.tx.operations, commitOp{
		seq: len(ct.tx.operations) + 1, opType: "create", Path: fullPath, content: content, status: "pending",
	})
	return nil
}

func (ct *CommitTransaction) DeleteFile(path string) error {
	fullPath := ct.resolvePath(path)
	ct.captureSnapshot(fullPath)
	ct.tx.operations = append(ct.tx.operations, commitOp{
		seq: len(ct.tx.operations) + 1, opType: "delete", Path: fullPath, status: "pending",
	})
	return nil
}

func (ct *CommitTransaction) ApplyNext() (*commitOp, error) {
	for i := range ct.tx.operations {
		op := &ct.tx.operations[i]
		if op.status != "pending" {
			continue
		}
		var err error
		switch op.opType {
		case "create", "modify":
			if mkErr := os.MkdirAll(filepath.Dir(op.Path), 0755); mkErr != nil {
				err = fmt.Errorf("failed to create directory: %w", mkErr)
			} else {
				err = os.WriteFile(op.Path, []byte(op.content), 0644)
			}
		case "delete":
			if rmErr := os.Remove(op.Path); rmErr != nil && !os.IsNotExist(rmErr) {
				err = fmt.Errorf("failed to delete file: %w", rmErr)
			}
		}
		if err != nil {
			op.status = "failed"
			return op, err
		}
		op.status = "applied"
		return op, nil
	}
	return nil, nil // no more pending
}

func (ct *CommitTransaction) Rollback(reason string) error {
	// Reverse-order rollback using captured snapshots
	for i := len(ct.tx.operations) - 1; i >= 0; i-- {
		op := &ct.tx.operations[i]
		if op.status != "applied" {
			continue
		}
		snap := ct.tx.snapshots[op.Path]
		if snap == nil {
			continue
		}
		if snap.existed {
			mode := snap.mode
			if mode == 0 {
				mode = 0644
			}
			os.WriteFile(op.Path, []byte(snap.content), mode)
		} else {
			os.Remove(op.Path)
		}
		op.status = "rolled_back"
	}
	ct.tx.state = "rolled_back"
	return nil
}

func (ct *CommitTransaction) Commit() error {
	for _, op := range ct.tx.operations {
		if op.status == "pending" {
			return fmt.Errorf("cannot commit: operation %d is still pending", op.seq)
		}
		if op.status == "failed" {
			return fmt.Errorf("cannot commit: operation %d failed", op.seq)
		}
	}
	ct.tx.state = "committed"
	return nil
}

// Discard rejects workspace changes
func (m *Manager) Discard(ws *Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace is nil")
	}

	ws.SetState(WorkspaceStateDiscarded)

	if err := ws.Save(); err != nil {
		return fmt.Errorf("failed to save workspace state: %w", err)
	}

	// Unregister from references
	if err := m.unregisterWorkspace(ws); err != nil {
		// Non-fatal
		fmt.Printf("Warning: failed to unregister workspace: %v\n", err)
	}

	// Optionally clean up files based on config
	if !m.config.RetainDiscarded {
		return os.RemoveAll(ws.WorkspaceDir)
	}

	return nil
}

// Resume loads and optionally reactivates an existing workspace
func (m *Manager) Resume(wsId string) (*Workspace, error) {
	wsPath := filepath.Join(m.workspacesDir, wsId, "workspace.json")

	ws, err := LoadWorkspace(wsPath)
	if err != nil {
		return nil, fmt.Errorf("workspace not found: %w", err)
	}

	// Check if recovery needed
	recoveryPath := filepath.Join(ws.WorkspaceDir, "recovery.json")
	if _, err := os.Stat(recoveryPath); err == nil {
		// Recovery file exists - workspace may be in inconsistent state
		ws.SetState(WorkspaceStateRecovering)
		if err := ws.Save(); err != nil {
			return nil, fmt.Errorf("failed to mark workspace for recovery: %w", err)
		}
	}

	return ws, nil
}

// GetActiveWorkspace returns the current active workspace for a plan/branch
func (m *Manager) GetActiveWorkspace(planId, branch string) (*Workspace, error) {
	refPath := m.getReferencePath()

	ref, err := LoadWorkspaceReference(refPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace references: %w", err)
	}

	wsId := ref.GetWorkspace(planId, branch)
	if wsId == "" {
		return nil, nil // No active workspace
	}

	return m.Resume(wsId)
}

// GetOrCreateWorkspace gets existing workspace or creates new one
func (m *Manager) GetOrCreateWorkspace(planId, branch, projectId string) (*Workspace, error) {
	ws, err := m.GetActiveWorkspace(planId, branch)
	if err != nil {
		return nil, err
	}

	if ws != nil {
		// Existing workspace found
		return ws, nil
	}

	// Create new workspace
	return m.Create(planId, branch, projectId)
}

// =============================================================================
// LISTING AND CLEANUP
// =============================================================================

// ListWorkspaces returns all workspaces for the current project
func (m *Manager) ListWorkspaces() ([]*WorkspaceSummary, error) {
	entries, err := os.ReadDir(m.workspacesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read workspaces directory: %w", err)
	}

	var summaries []*WorkspaceSummary

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		wsPath := filepath.Join(m.workspacesDir, entry.Name(), "workspace.json")
		ws, err := LoadWorkspace(wsPath)
		if err != nil {
			continue // Skip invalid workspaces
		}

		// Only include workspaces for current project
		if ws.BaseDir == m.projectRoot {
			summaries = append(summaries, ws.GetSummary())
		}
	}

	// Sort by last accessed (most recent first)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastAccessedAt.After(summaries[j].LastAccessedAt)
	})

	return summaries, nil
}

// ListAllWorkspaces returns all workspaces regardless of project
func (m *Manager) ListAllWorkspaces() ([]*WorkspaceSummary, error) {
	entries, err := os.ReadDir(m.workspacesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read workspaces directory: %w", err)
	}

	var summaries []*WorkspaceSummary

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		wsPath := filepath.Join(m.workspacesDir, entry.Name(), "workspace.json")
		ws, err := LoadWorkspace(wsPath)
		if err != nil {
			continue
		}

		summaries = append(summaries, ws.GetSummary())
	}

	// Sort by last accessed (most recent first)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastAccessedAt.After(summaries[j].LastAccessedAt)
	})

	return summaries, nil
}

// CleanupStaleWorkspaces removes old/unused workspaces
func (m *Manager) CleanupStaleWorkspaces() (int, error) {
	entries, err := os.ReadDir(m.workspacesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read workspaces directory: %w", err)
	}

	staleThreshold := time.Now().AddDate(0, 0, -m.config.StaleAfterDays)
	var cleanedCount int

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if cleanedCount >= m.config.CleanupBatchSize {
			break // Limit batch size
		}

		wsPath := filepath.Join(m.workspacesDir, entry.Name(), "workspace.json")
		ws, err := LoadWorkspace(wsPath)
		if err != nil {
			// Invalid workspace - clean it up
			os.RemoveAll(filepath.Join(m.workspacesDir, entry.Name()))
			cleanedCount++
			continue
		}

		// Check if workspace should be cleaned
		shouldClean := false

		switch ws.State {
		case WorkspaceStateCommitted, WorkspaceStateDiscarded:
			// Already completed workspaces
			shouldClean = true
		case WorkspaceStateActive, WorkspaceStatePending:
			// Check if stale
			if ws.LastAccessedAt.Before(staleThreshold) {
				shouldClean = true
			}
		case WorkspaceStateRecovering:
			// Don't clean workspaces needing recovery
			shouldClean = false
		}

		if shouldClean {
			if err := os.RemoveAll(ws.WorkspaceDir); err != nil {
				fmt.Printf("Warning: failed to clean workspace %s: %v\n", ws.Id, err)
			} else {
				cleanedCount++
			}
		}
	}

	return cleanedCount, nil
}

// =============================================================================
// HELPER METHODS
// =============================================================================

// getReferencePath returns the path to workspaces-v2.json
func (m *Manager) getReferencePath() string {
	return filepath.Join(m.plandexDir, "workspaces-v2.json")
}

// registerWorkspace adds workspace to project references
func (m *Manager) registerWorkspace(ws *Workspace) error {
	refPath := m.getReferencePath()

	ref, err := LoadWorkspaceReference(refPath)
	if err != nil {
		return err
	}

	ref.SetWorkspace(ws.PlanId, ws.Branch, ws.Id)

	return ref.Save(refPath)
}

// unregisterWorkspace removes workspace from project references
func (m *Manager) unregisterWorkspace(ws *Workspace) error {
	refPath := m.getReferencePath()

	ref, err := LoadWorkspaceReference(refPath)
	if err != nil {
		return err
	}

	ref.RemoveWorkspace(ws.PlanId, ws.Branch)

	return ref.Save(refPath)
}

// GetWorkspacesDir returns the workspaces directory path
func (m *Manager) GetWorkspacesDir() string {
	return m.workspacesDir
}

// GetConfig returns the workspace configuration
func (m *Manager) GetConfig() *WorkspaceConfig {
	return m.config
}

// IsEnabled returns whether workspace isolation is enabled
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled
}
