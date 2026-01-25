package fs

import (
	"os"
	"path/filepath"
	"plandex-cli/types"
	"plandex-cli/workspace"
)

// =============================================================================
// WORKSPACE-AWARE PATH RESOLUTION
// =============================================================================
//
// This module provides workspace-aware path resolution, merging the original
// project paths with any modifications tracked in the workspace.
//
// =============================================================================

// WorkspacesDir is the global path to the workspaces directory
var WorkspacesDir string

// InitWorkspacesDir initializes the workspaces directory path
// Should be called after HomePlandexDir is initialized
func InitWorkspacesDir() {
	if HomePlandexDir != "" {
		WorkspacesDir = filepath.Join(HomePlandexDir, "workspaces")
	}
}

// GetWorkspaceProjectPaths returns project paths merged with workspace modifications
func GetWorkspaceProjectPaths(baseDir string, ws *workspace.Workspace) (*types.ProjectPaths, error) {
	if ws == nil {
		return GetProjectPaths(baseDir)
	}

	// Get original project paths
	originalPaths, err := GetProjectPaths(baseDir)
	if err != nil {
		return nil, err
	}

	// Create merged paths
	mergedPaths := &types.ProjectPaths{
		ActivePaths:    make(map[string]bool),
		AllPaths:       make(map[string]bool),
		ActiveDirs:     make(map[string]bool),
		AllDirs:        make(map[string]bool),
		PlandexIgnored: originalPaths.PlandexIgnored,
		IgnoredPaths:   make(map[string]string),
		GitIgnoredDirs: originalPaths.GitIgnoredDirs,
	}

	// Copy original paths
	for k, v := range originalPaths.ActivePaths {
		mergedPaths.ActivePaths[k] = v
	}
	for k, v := range originalPaths.AllPaths {
		mergedPaths.AllPaths[k] = v
	}
	for k, v := range originalPaths.ActiveDirs {
		mergedPaths.ActiveDirs[k] = v
	}
	for k, v := range originalPaths.AllDirs {
		mergedPaths.AllDirs[k] = v
	}
	for k, v := range originalPaths.IgnoredPaths {
		mergedPaths.IgnoredPaths[k] = v
	}

	// Add workspace modifications (these override original)
	for path := range ws.ModifiedFiles {
		mergedPaths.ActivePaths[path] = true
		mergedPaths.AllPaths[path] = true
		// Add parent directories
		addParentDirs(mergedPaths, path)
	}

	// Add workspace created files
	for path := range ws.CreatedFiles {
		mergedPaths.ActivePaths[path] = true
		mergedPaths.AllPaths[path] = true
		// Add parent directories
		addParentDirs(mergedPaths, path)
	}

	// Remove deleted files from active paths
	for path := range ws.DeletedFiles {
		delete(mergedPaths.ActivePaths, path)
		// Keep in AllPaths for reference
	}

	return mergedPaths, nil
}

// addParentDirs adds all parent directories of a path to the merged paths
func addParentDirs(paths *types.ProjectPaths, filePath string) {
	dir := filepath.Dir(filePath)
	for dir != "." && dir != "/" && dir != "" {
		paths.ActiveDirs[dir] = true
		paths.AllDirs[dir] = true
		dir = filepath.Dir(dir)
	}
}

// GetEffectiveFilePath returns the actual file path to read from
// considering workspace overrides
func GetEffectiveFilePath(relativePath string, ws *workspace.Workspace) string {
	if ws == nil {
		return filepath.Join(ProjectRoot, relativePath)
	}

	// Check if file is in workspace
	if ws.IsFileInWorkspace(relativePath) {
		return ws.GetFilePath(relativePath)
	}

	// Check if file is deleted in workspace
	if ws.IsFileDeleted(relativePath) {
		return "" // File doesn't exist in workspace view
	}

	// Fall back to original
	return filepath.Join(ProjectRoot, relativePath)
}

// IsFileInWorkspace checks if a file has been modified or created in workspace
func IsFileInWorkspace(relativePath string, ws *workspace.Workspace) bool {
	if ws == nil {
		return false
	}
	return ws.IsFileInWorkspace(relativePath)
}

// IsFileDeletedInWorkspace checks if a file is marked as deleted in workspace
func IsFileDeletedInWorkspace(relativePath string, ws *workspace.Workspace) bool {
	if ws == nil {
		return false
	}
	return ws.IsFileDeleted(relativePath)
}

// WorkspacePathInfo provides information about a path in workspace context
type WorkspacePathInfo struct {
	RelativePath    string
	EffectivePath   string
	IsInWorkspace   bool
	IsDeleted       bool
	IsModified      bool
	IsCreated       bool
	OriginalExists  bool
	WorkspaceExists bool
}

// GetWorkspacePathInfo returns detailed information about a path
func GetWorkspacePathInfo(relativePath string, ws *workspace.Workspace) *WorkspacePathInfo {
	info := &WorkspacePathInfo{
		RelativePath: relativePath,
	}

	originalPath := filepath.Join(ProjectRoot, relativePath)
	info.OriginalExists = fileExists(originalPath)

	if ws == nil {
		info.EffectivePath = originalPath
		return info
	}

	wsPath := ws.GetFilePath(relativePath)
	info.WorkspaceExists = fileExists(wsPath)
	info.IsDeleted = ws.IsFileDeleted(relativePath)
	info.IsModified = ws.IsFileModified(relativePath)
	info.IsCreated = ws.IsFileCreated(relativePath)
	info.IsInWorkspace = info.IsModified || info.IsCreated

	if info.IsDeleted {
		info.EffectivePath = ""
	} else if info.IsInWorkspace {
		info.EffectivePath = wsPath
	} else {
		info.EffectivePath = originalPath
	}

	return info
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
