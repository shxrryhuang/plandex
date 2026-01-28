package lib

import (
	"fmt"
	"os"
	"plandex-cli/fs"
	"plandex-cli/term"
	"plandex-cli/types"
	"plandex-cli/workspace"
	"strings"

	"github.com/fatih/color"

	shared "plandex-shared"
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

// ApplyFilesToWorkspace applies files to the isolated workspace atomically.
//
// Like ApplyFiles, all operations are staged first (snapshots of the current
// workspace state captured), then applied one at a time.  If any write fails
// every previously written file in this batch is restored via its snapshot.
// The workspace tracking maps are updated only after all writes succeed, so
// a failure leaves the workspace metadata consistent with what is actually
// on disk.
func ApplyFilesToWorkspace(
	ctx *WorkspaceApplyContext,
	toApply map[string]string,
	toRemove map[string]bool,
	projectPaths *types.ProjectPaths,
) ([]string, *types.ApplyRollbackPlan, error) {
	if ctx == nil || !ctx.Enabled || ctx.Workspace == nil {
		return ApplyFiles(toApply, toRemove, projectPaths)
	}

	ws := ctx.Workspace
	opLog := ctx.OperationLog

	// --- Recovery marker -----------------------------------------------------
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
		fmt.Printf("Warning: failed to start recovery point: %v\n", err)
	}

	// --- Staging: build operation list and capture snapshots -----------------
	// We use the workspace's files directory as the transaction base so
	// snapshots capture what is currently in the workspace (not the original
	// project).
	tx := shared.NewFileTransaction("ws-apply", ws.Branch, ws.GetFilesDir())
	if err := tx.Begin(); err != nil {
		workspace.EndRecoveryPoint(ws)
		return nil, nil, fmt.Errorf("failed to begin workspace transaction: %w", err)
	}

	normalise := func(s string) string {
		return strings.ReplaceAll(s, "\\`\\`\\`", "```")
	}

	// pendingTracking collects metadata updates to apply only after all writes
	// succeed.  This keeps workspace tracking consistent on failure.
	type trackEntry struct {
		path         string
		opType       string // "modify" | "create" | "delete"
		originalHash string
		contentHash  string
		mode         os.FileMode
		size         int64
	}
	var pendingTracking []trackEntry
	var updatedPaths []string

	for path, content := range toApply {
		if path == "_apply.sh" {
			continue
		}
		content = normalise(content)
		opLog.LogStart("apply_file", path)

		wsPath := ws.GetFilePath(path)
		originalPath := ws.GetOriginalPath(path)

		// Determine whether this is a create or modify relative to the
		// original project, and whether the content actually differs from
		// what is already in the workspace.
		var originalExists bool
		var originalMode os.FileMode
		var originalContent []byte
		if info, err := os.Stat(originalPath); err == nil {
			originalExists = true
			originalMode = info.Mode()
			originalContent, _ = os.ReadFile(originalPath)
		}

		// If workspace already has this file with identical content, skip.
		if wsContent, err := os.ReadFile(wsPath); err == nil {
			if string(wsContent) == content {
				opLog.LogComplete("apply_file", path, workspace.HashString(content))
				continue
			}
		} else if originalExists && string(originalContent) == content {
			// Not in workspace yet and identical to original — skip.
			opLog.LogComplete("apply_file", path, workspace.HashString(content))
			continue
		}

		// Stage the operation (snapshot of current workspace file captured).
		if originalExists {
			if err := tx.ModifyFile(path, content); err != nil {
				tx.Rollback("staging failed")
				workspace.EndRecoveryPoint(ws)
				return nil, nil, fmt.Errorf("failed to stage workspace modification for %s: %w", path, err)
			}
			pendingTracking = append(pendingTracking, trackEntry{
				path: path, opType: "modify",
				originalHash: workspace.HashString(string(originalContent)),
				contentHash:  workspace.HashString(content),
				mode:         originalMode, size: int64(len(content)),
			})
		} else {
			if err := tx.CreateFile(path, content); err != nil {
				tx.Rollback("staging failed")
				workspace.EndRecoveryPoint(ws)
				return nil, nil, fmt.Errorf("failed to stage workspace creation for %s: %w", path, err)
			}
			pendingTracking = append(pendingTracking, trackEntry{
				path: path, opType: "create",
				contentHash: workspace.HashString(content),
				mode:        0644, size: int64(len(content)),
			})
		}
		updatedPaths = append(updatedPaths, path)
	}

	for path, remove := range toRemove {
		if !remove {
			continue
		}
		opLog.LogStart("remove_file", path)
		if err := tx.DeleteFile(path); err != nil {
			tx.Rollback("staging failed")
			workspace.EndRecoveryPoint(ws)
			return nil, nil, fmt.Errorf("failed to stage workspace deletion for %s: %w", path, err)
		}
		pendingTracking = append(pendingTracking, trackEntry{path: path, opType: "delete"})
		updatedPaths = append(updatedPaths, path)
	}

	if len(tx.Operations) == 0 {
		tx.Commit()
		workspace.EndRecoveryPoint(ws)
		return updatedPaths, &types.ApplyRollbackPlan{
			PreviousProjectPaths: projectPaths,
			ToRevert:             map[string]types.ApplyReversion{},
		}, nil
	}

	// --- Applying: sequential writes to workspace directory ------------------
	for {
		op, err := tx.ApplyNext()
		if op == nil {
			break
		}
		if err != nil {
			opLog.LogFailed("apply_file", op.Path, err)
			tx.Rollback(fmt.Sprintf("workspace write failed for %s: %v", op.Path, err))
			workspace.EndRecoveryPoint(ws)
			return nil, nil, fmt.Errorf("workspace apply failed for %s: %w (all changes rolled back)", op.Path, err)
		}
	}

	// --- Commit: all writes succeeded, update tracking atomically -----------
	if err := tx.Commit(); err != nil {
		tx.Rollback("commit validation failed")
		workspace.EndRecoveryPoint(ws)
		return nil, nil, fmt.Errorf("workspace transaction commit failed: %w", err)
	}

	// Now that all writes are durable, update workspace metadata.
	for _, t := range pendingTracking {
		switch t.opType {
		case "modify":
			ws.TrackModifiedFile(t.path, t.originalHash, t.contentHash, t.mode, t.size)
		case "create":
			ws.TrackCreatedFile(t.path, t.contentHash, t.mode, t.size)
		case "delete":
			ws.TrackDeletedFile(t.path)
		}
		opLog.LogComplete("apply_file", t.path, t.contentHash)
	}

	if err := ws.Save(); err != nil {
		return nil, nil, fmt.Errorf("failed to save workspace state: %w", err)
	}
	workspace.EndRecoveryPoint(ws)

	// Build rollback plan from transaction snapshots for post-exec-script use.
	toRevert := map[string]types.ApplyReversion{}
	var toRemoveOnRollback []string
	for _, snap := range tx.Snapshots {
		if snap.Existed {
			toRevert[snap.Path] = types.ApplyReversion{Content: snap.Content, Mode: snap.Mode}
		} else {
			toRemoveOnRollback = append(toRemoveOnRollback, snap.Path)
		}
	}

	return updatedPaths, &types.ApplyRollbackPlan{
		PreviousProjectPaths: projectPaths,
		ToRevert:             toRevert,
		ToRemove:             toRemoveOnRollback,
	}, nil
}

// RollbackWorkspace reverts workspace files using the rollback plan produced
// by ApplyFilesToWorkspace.  Each entry in the plan corresponds to exactly
// one file that was written during the apply: ToRevert holds the pre-apply
// content for files that existed before, and ToRemove holds paths for files
// that were newly created.
//
// Workspace tracking is updated surgically — only the entries that were
// touched by this apply are removed.  Any earlier workspace modifications
// that were not part of this rollback plan remain intact.
//
// Output follows the same three-phase pattern as Rollback():
//
//	🔄 Rolling back workspace changes…
//	  ↩ restored  path/to/file.go
//	  ↩ removed   path/to/new.go
//	🚫 Rolled back N file(s)
func RollbackWorkspace(ctx *WorkspaceApplyContext, rollbackPlan *types.ApplyRollbackPlan, verbose bool) {
	if ctx == nil || !ctx.Enabled || ctx.Workspace == nil {
		Rollback(rollbackPlan, verbose)
		return
	}

	if rollbackPlan == nil || !rollbackPlan.HasChanges() {
		return
	}

	if verbose {
		fmt.Println()
		color.New(term.ColorHiYellow, color.Bold).Println("🔄 Rolling back workspace changes…")
	}

	ws := ctx.Workspace
	var errs []error
	restored := 0

	// 1. Restore files that existed before this apply (content revert).
	for path, revert := range rollbackPlan.ToRevert {
		if err := os.WriteFile(path, []byte(revert.Content), revert.Mode); err != nil {
			errs = append(errs, fmt.Errorf("failed to restore %s: %w", path, err))
			if verbose {
				color.New(term.ColorHiRed).Printf("  ✗ restore  %s  (%v)\n", path, err)
			}
			continue
		}
		restored++
		if verbose {
			color.New(term.ColorHiGreen).Printf("  ↩ restored  %s\n", path)
		}
		// Remove only this path from tracking — leave unrelated entries alone.
		relPath := workspaceRelPath(ws, path)
		if relPath != "" {
			ws.ModifiedFiles = deleteKey(ws.ModifiedFiles, relPath)
			ws.CreatedFiles = deleteKey(ws.CreatedFiles, relPath)
		}
	}

	// 2. Delete files that were newly created by this apply.
	for _, path := range rollbackPlan.ToRemove {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", path, err))
			if verbose {
				color.New(term.ColorHiRed).Printf("  ✗ remove   %s  (%v)\n", path, err)
			}
			continue
		}
		restored++
		if verbose {
			color.New(term.ColorHiGreen).Printf("  ↩ removed   %s\n", path)
		}
		relPath := workspaceRelPath(ws, path)
		if relPath != "" {
			ws.CreatedFiles = deleteKey(ws.CreatedFiles, relPath)
			ws.ModifiedFiles = deleteKey(ws.ModifiedFiles, relPath)
		}
	}

	ws.Save()

	if verbose {
		if len(errs) > 0 {
			color.New(term.ColorHiRed, color.Bold).Printf("⚠ Workspace rollback completed with %d error(s), %d file(s) restored\n", len(errs), restored)
		} else {
			color.New(term.ColorHiGreen, color.Bold).Printf("🚫 Rolled back %d workspace file(s)\n", restored)
		}
		fmt.Println()
	}
}

// workspaceRelPath converts an absolute workspace file path back to the
// relative path used as the key in the workspace tracking maps.
func workspaceRelPath(ws *workspace.Workspace, absPath string) string {
	filesDir := ws.GetFilesDir()
	if len(absPath) > len(filesDir)+1 && absPath[:len(filesDir)] == filesDir {
		return absPath[len(filesDir)+1:]
	}
	return ""
}

// deleteKey removes a single key from a FileEntry map without replacing the
// entire map.  Returns the same map for assignment convenience.
func deleteKey(m map[string]*workspace.FileEntry, key string) map[string]*workspace.FileEntry {
	if m != nil {
		delete(m, key)
	}
	return m
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
