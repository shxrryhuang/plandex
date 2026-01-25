package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"plandex-cli/fs"
	"plandex-cli/term"
	"plandex-cli/types"
	"plandex-cli/workspace"
	"strings"
	"sync"
)

// =============================================================================
// WORKSPACE-AWARE APPLY FUNCTIONS
// =============================================================================
//
// This module provides workspace-aware versions of the apply functions.
// When a workspace is active, file operations are redirected to the
// isolated workspace instead of the main project directory.
//
// =============================================================================

// WorkspaceApplyContext holds workspace state for apply operations
type WorkspaceApplyContext struct {
	Workspace    *workspace.Workspace
	Manager      *workspace.Manager
	FileAccess   *workspace.LazyFileAccess
	OperationLog *workspace.OperationLog
	Enabled      bool
}

// NewWorkspaceApplyContext creates a new workspace context for apply operations
func NewWorkspaceApplyContext(planId, branch, projectId string) (*WorkspaceApplyContext, error) {
	config := workspace.DefaultWorkspaceConfig()
	mgr := workspace.NewManager(fs.HomePlandexDir, fs.ProjectRoot, fs.PlandexDir, config)

	if !mgr.IsEnabled() {
		return &WorkspaceApplyContext{Enabled: false}, nil
	}

	// Get or create workspace
	ws, err := mgr.GetOrCreateWorkspace(planId, branch, projectId)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create workspace: %w", err)
	}

	// Activate workspace
	if err := mgr.Activate(ws); err != nil {
		return nil, fmt.Errorf("failed to activate workspace: %w", err)
	}

	return &WorkspaceApplyContext{
		Workspace:    ws,
		Manager:      mgr,
		FileAccess:   workspace.NewLazyFileAccess(ws),
		OperationLog: workspace.NewOperationLog(ws),
		Enabled:      true,
	}, nil
}

// ApplyFilesToWorkspace applies files to the isolated workspace instead of main project
func ApplyFilesToWorkspace(
	ctx *WorkspaceApplyContext,
	toApply map[string]string,
	toRemove map[string]bool,
	projectPaths *types.ProjectPaths,
) ([]string, *types.ApplyRollbackPlan, error) {
	if ctx == nil || !ctx.Enabled || ctx.Workspace == nil {
		// Fall back to regular apply
		return ApplyFiles(toApply, toRemove, projectPaths)
	}

	ws := ctx.Workspace
	opLog := ctx.OperationLog

	// Start recovery point
	var filesToApply []string
	for path := range toApply {
		if path != "_apply.sh" {
			filesToApply = append(filesToApply, path)
		}
	}
	for path := range toRemove {
		filesToApply = append(filesToApply, path)
	}

	if err := workspace.StartRecoveryPoint(ws, "apply", filesToApply); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: failed to start recovery point: %v\n", err)
	}

	var updatedFiles []string
	toRevert := map[string]types.ApplyReversion{}
	var toRemoveOnRollback []string

	var mu sync.Mutex
	totalOps := len(toApply) + len(toRemove)
	errCh := make(chan error, totalOps)

	for path, content := range toApply {
		if path == "_apply.sh" {
			errCh <- nil
			continue
		}

		go func(path, content string) {
			// Log operation start
			opLog.LogStart("apply_file", path)

			// Compute paths
			wsPath := ws.GetFilePath(path)
			originalPath := ws.GetOriginalPath(path)
			content = strings.ReplaceAll(content, "\\`\\`\\`", "```")

			// Check if file exists in original project
			var originalExists bool
			var originalMode os.FileMode
			var originalContent []byte

			info, err := os.Stat(originalPath)
			if err == nil {
				originalExists = true
				originalMode = info.Mode()
				originalContent, _ = os.ReadFile(originalPath)
			} else if !os.IsNotExist(err) {
				errCh <- fmt.Errorf("failed to check if %s exists: %w", originalPath, err)
				return
			}

			// Check if file is already in workspace
			wsInfo, wsErr := os.Stat(wsPath)
			if wsErr == nil {
				// File already in workspace - read current content for rollback
				wsContent, _ := os.ReadFile(wsPath)
				mu.Lock()
				toRevert[wsPath] = types.ApplyReversion{Content: string(wsContent), Mode: wsInfo.Mode()}
				mu.Unlock()
			}

			// Ensure workspace directory exists
			if err := os.MkdirAll(filepath.Dir(wsPath), 0755); err != nil {
				errCh <- fmt.Errorf("failed to create workspace directory: %w", err)
				return
			}

			// Check if content actually changed
			if originalExists && string(originalContent) == content && wsErr != nil {
				// No change from original and not already in workspace
				errCh <- nil
				return
			}

			// Write to workspace
			if err := os.WriteFile(wsPath, []byte(content), 0644); err != nil {
				opLog.LogFailed("apply_file", path, err)
				errCh <- fmt.Errorf("failed to write to workspace: %w", err)
				return
			}

			// Track in workspace metadata
			contentHash := workspace.HashString(content)
			if originalExists {
				originalHash := workspace.HashString(string(originalContent))
				ws.TrackModifiedFile(path, originalHash, contentHash, originalMode, int64(len(content)))
			} else {
				ws.TrackCreatedFile(path, contentHash, 0644, int64(len(content)))
				mu.Lock()
				toRemoveOnRollback = append(toRemoveOnRollback, wsPath)
				mu.Unlock()
			}

			mu.Lock()
			updatedFiles = append(updatedFiles, path)
			mu.Unlock()

			opLog.LogComplete("apply_file", path, contentHash)
			errCh <- nil
		}(path, content)
	}

	// Handle file removals
	for path, remove := range toRemove {
		go func(path string, remove bool) {
			if !remove {
				errCh <- nil
				return
			}

			opLog.LogStart("remove_file", path)

			// Track deletion in workspace
			ws.TrackDeletedFile(path)

			// If file was in workspace, remove it
			wsPath := ws.GetFilePath(path)
			if _, err := os.Stat(wsPath); err == nil {
				content, _ := os.ReadFile(wsPath)
				info, _ := os.Stat(wsPath)
				mu.Lock()
				toRevert[wsPath] = types.ApplyReversion{Content: string(content), Mode: info.Mode()}
				mu.Unlock()

				if err := os.Remove(wsPath); err != nil && !os.IsNotExist(err) {
					errCh <- fmt.Errorf("failed to remove file from workspace: %w", err)
					return
				}
			}

			opLog.LogComplete("remove_file", path, "")
			errCh <- nil
		}(path, remove)
	}

	// Wait for all operations
	for i := 0; i < totalOps; i++ {
		if err := <-errCh; err != nil {
			workspace.EndRecoveryPoint(ws)
			return nil, nil, err
		}
	}

	// Save workspace state
	if err := ws.Save(); err != nil {
		return nil, nil, fmt.Errorf("failed to save workspace state: %w", err)
	}

	// End recovery point
	workspace.EndRecoveryPoint(ws)

	return updatedFiles, &types.ApplyRollbackPlan{
		PreviousProjectPaths: projectPaths,
		ToRevert:             toRevert,
		ToRemove:             toRemoveOnRollback,
	}, nil
}

// RollbackWorkspace rolls back changes in the workspace
func RollbackWorkspace(ctx *WorkspaceApplyContext, rollbackPlan *types.ApplyRollbackPlan, verbose bool) {
	if ctx == nil || !ctx.Enabled || ctx.Workspace == nil {
		// Fall back to regular rollback
		Rollback(rollbackPlan, verbose)
		return
	}

	if rollbackPlan == nil || !rollbackPlan.HasChanges() {
		return
	}

	if verbose {
		fmt.Println("🔄 Rolling back workspace changes...")
	}

	var wg sync.WaitGroup

	// Revert modified files
	for path, revert := range rollbackPlan.ToRevert {
		wg.Add(1)
		go func(path string, revert types.ApplyReversion) {
			defer wg.Done()
			os.WriteFile(path, []byte(revert.Content), revert.Mode)
		}(path, revert)
	}

	// Remove created files
	for _, path := range rollbackPlan.ToRemove {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			os.Remove(path)
		}(path)
	}

	wg.Wait()

	// Update workspace tracking
	ws := ctx.Workspace
	ws.ModifiedFiles = make(map[string]*workspace.FileEntry)
	ws.CreatedFiles = make(map[string]*workspace.FileEntry)
	ws.DeletedFiles = make(map[string]bool)
	ws.Save()

	if verbose {
		fmt.Println("✅ Workspace rolled back")
	}
}

// GetWorkspaceExecutionDir returns the directory to use for command execution
// For now, returns original project root with workspace env vars set
func GetWorkspaceExecutionDir(ctx *WorkspaceApplyContext) string {
	if ctx == nil || !ctx.Enabled || ctx.Workspace == nil {
		return fs.ProjectRoot
	}

	// For command execution, we use the original project directory
	// but set environment variables to indicate workspace context
	return fs.ProjectRoot
}

// GetWorkspaceEnvVars returns environment variables for workspace context
func GetWorkspaceEnvVars(ctx *WorkspaceApplyContext) []string {
	if ctx == nil || !ctx.Enabled || ctx.Workspace == nil {
		return nil
	}

	ws := ctx.Workspace
	return []string{
		fmt.Sprintf("PLANDEX_WORKSPACE=%s", ws.WorkspaceDir),
		fmt.Sprintf("PLANDEX_WORKSPACE_ID=%s", ws.Id),
		fmt.Sprintf("PLANDEX_WORKSPACE_FILES=%s", ws.GetFilesDir()),
	}
}

// ShowWorkspaceIndicator displays workspace status before operations
func ShowWorkspaceIndicator(ctx *WorkspaceApplyContext) {
	if ctx == nil || !ctx.Enabled || ctx.Workspace == nil {
		return
	}

	term.ShowWorkspaceIndicator(ctx.Workspace)
}

// CommitWorkspace commits workspace changes to main project
func CommitWorkspace(ctx *WorkspaceApplyContext) error {
	if ctx == nil || !ctx.Enabled || ctx.Workspace == nil {
		return nil
	}

	return ctx.Manager.Commit(ctx.Workspace)
}

// DiscardWorkspace discards workspace changes
func DiscardWorkspace(ctx *WorkspaceApplyContext) error {
	if ctx == nil || !ctx.Enabled || ctx.Workspace == nil {
		return nil
	}

	return ctx.Manager.Discard(ctx.Workspace)
}

// GetWorkspaceStatus returns a status string for display
func GetWorkspaceStatus(ctx *WorkspaceApplyContext) string {
	if ctx == nil || !ctx.Enabled || ctx.Workspace == nil {
		return ""
	}

	return term.GetWorkspaceStatusLine(ctx.Workspace)
}

// HasWorkspaceChanges returns true if workspace has uncommitted changes
func HasWorkspaceChanges(ctx *WorkspaceApplyContext) bool {
	if ctx == nil || !ctx.Enabled || ctx.Workspace == nil {
		return false
	}

	return ctx.Workspace.HasChanges()
}

// SaveWorkspaceState saves current workspace state
func SaveWorkspaceState(ctx *WorkspaceApplyContext) error {
	if ctx == nil || !ctx.Enabled || ctx.Workspace == nil {
		return nil
	}

	ctx.Workspace.Touch()
	return ctx.Workspace.Save()
}
