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

// Commit applies workspace changes to main project atomically
func (m *Manager) Commit(ws *Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace is nil")
	}

	if ws.State != WorkspaceStateActive {
		return fmt.Errorf("cannot commit non-active workspace (state: %s)", ws.State)
	}

	if !ws.HasChanges() {
		// No changes to commit - just mark as committed
		ws.SetState(WorkspaceStateCommitted)
		return ws.Save()
	}

	// Apply modified files
	for path, entry := range ws.ModifiedFiles {
		srcPath := ws.GetFilePath(path)
		dstPath := ws.GetOriginalPath(path)

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("failed to read modified file %s: %w", path, err)
		}

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", path, err)
		}

		if err := os.WriteFile(dstPath, content, entry.Mode); err != nil {
			return fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}

	// Apply created files
	for path, entry := range ws.CreatedFiles {
		srcPath := ws.GetFilePath(path)
		dstPath := ws.GetOriginalPath(path)

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("failed to read created file %s: %w", path, err)
		}

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", path, err)
		}

		if err := os.WriteFile(dstPath, content, entry.Mode); err != nil {
			return fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}

	// Apply deletions
	for path := range ws.DeletedFiles {
		dstPath := ws.GetOriginalPath(path)

		// Only delete if file exists
		if _, err := os.Stat(dstPath); err == nil {
			if err := os.Remove(dstPath); err != nil {
				return fmt.Errorf("failed to delete file %s: %w", path, err)
			}
		}
	}

	// Mark as committed
	ws.SetState(WorkspaceStateCommitted)
	if err := ws.Save(); err != nil {
		return fmt.Errorf("failed to save workspace state: %w", err)
	}

	// Update reference to remove this workspace (it's done)
	if err := m.unregisterWorkspace(ws); err != nil {
		// Non-fatal: workspace is committed, just log warning
		fmt.Printf("Warning: failed to unregister workspace: %v\n", err)
	}

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
