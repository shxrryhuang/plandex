package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// =============================================================================
// COPY-ON-WRITE FILE ACCESS
// =============================================================================
//
// This module provides copy-on-write semantics for file operations within
// a workspace. Files are only copied from the original project when they
// are first modified, reducing disk usage and improving performance.
//
// Read operations:
//   - Check workspace first for modified/created files
//   - Fall back to original project for unmodified files
//   - Respect deleted file markers
//
// Write operations:
//   - Create file in workspace directory
//   - Track original hash for modified files
//   - Update workspace metadata
//
// =============================================================================

// LazyFileAccess provides copy-on-write semantics for file operations
type LazyFileAccess struct {
	ws *Workspace
	mu sync.RWMutex
}

// NewLazyFileAccess creates a new LazyFileAccess for a workspace
func NewLazyFileAccess(ws *Workspace) *LazyFileAccess {
	return &LazyFileAccess{
		ws: ws,
	}
}

// Read returns file content, preferring workspace version over original
func (l *LazyFileAccess) Read(path string) ([]byte, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Check if file is deleted in workspace
	if l.ws.IsFileDeleted(path) {
		return nil, os.ErrNotExist
	}

	// Check if file exists in workspace (modified or created)
	wsPath := l.ws.GetFilePath(path)
	if content, err := os.ReadFile(wsPath); err == nil {
		return content, nil
	}

	// Fall back to original project
	originalPath := l.ws.GetOriginalPath(path)
	return os.ReadFile(originalPath)
}

// Write writes content to workspace, implementing copy-on-write
func (l *LazyFileAccess) Write(path string, content []byte, mode os.FileMode) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Compute new content hash
	newHash := HashContent(content)

	// Determine workspace destination path
	wsPath := l.ws.GetFilePath(path)
	originalPath := l.ws.GetOriginalPath(path)

	// Ensure directory exists in workspace
	if err := os.MkdirAll(filepath.Dir(wsPath), 0755); err != nil {
		return fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Check if this is a modification of existing file
	if !l.ws.IsFileInWorkspace(path) {
		// First modification - check if file exists in original
		if originalContent, err := os.ReadFile(originalPath); err == nil {
			// File exists in original - this is a modification
			originalHash := HashContent(originalContent)
			info, _ := os.Stat(originalPath)
			originalMode := info.Mode()

			// Write to workspace
			if err := os.WriteFile(wsPath, content, mode); err != nil {
				return fmt.Errorf("failed to write to workspace: %w", err)
			}

			// Track as modified
			l.ws.TrackModifiedFile(path, originalHash, newHash, originalMode, int64(len(content)))
			return nil
		}
	}

	// Write to workspace
	if err := os.WriteFile(wsPath, content, mode); err != nil {
		return fmt.Errorf("failed to write to workspace: %w", err)
	}

	// Track as created or update existing
	if l.ws.IsFileModified(path) {
		// Update existing modified entry
		l.ws.mu.Lock()
		if entry, ok := l.ws.ModifiedFiles[path]; ok {
			entry.CurrentHash = newHash
			entry.Size = int64(len(content))
		}
		l.ws.mu.Unlock()
	} else if l.ws.IsFileCreated(path) {
		// Update existing created entry
		l.ws.mu.Lock()
		if entry, ok := l.ws.CreatedFiles[path]; ok {
			entry.CurrentHash = newHash
			entry.Size = int64(len(content))
		}
		l.ws.mu.Unlock()
	} else {
		// New file
		l.ws.TrackCreatedFile(path, newHash, mode, int64(len(content)))
	}

	return nil
}

// Delete marks a file as deleted in the workspace
func (l *LazyFileAccess) Delete(path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if file exists (either in workspace or original)
	wsPath := l.ws.GetFilePath(path)
	originalPath := l.ws.GetOriginalPath(path)

	existsInWorkspace := false
	existsInOriginal := false

	if _, err := os.Stat(wsPath); err == nil {
		existsInWorkspace = true
	}
	if _, err := os.Stat(originalPath); err == nil {
		existsInOriginal = true
	}

	if !existsInWorkspace && !existsInOriginal && !l.ws.IsFileInWorkspace(path) {
		return os.ErrNotExist
	}

	// Remove from workspace if it exists there
	if existsInWorkspace {
		if err := os.Remove(wsPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove file from workspace: %w", err)
		}
	}

	// Track deletion (only matters if file exists in original)
	if existsInOriginal {
		l.ws.TrackDeletedFile(path)
	} else {
		// File only existed in workspace - just remove from tracking
		l.ws.mu.Lock()
		delete(l.ws.ModifiedFiles, path)
		delete(l.ws.CreatedFiles, path)
		delete(l.ws.DeletedFiles, path)
		l.ws.mu.Unlock()
	}

	return nil
}

// Exists checks if a file exists (considering workspace state)
func (l *LazyFileAccess) Exists(path string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Check if deleted in workspace
	if l.ws.IsFileDeleted(path) {
		return false
	}

	// Check workspace
	wsPath := l.ws.GetFilePath(path)
	if _, err := os.Stat(wsPath); err == nil {
		return true
	}

	// Check original
	originalPath := l.ws.GetOriginalPath(path)
	if _, err := os.Stat(originalPath); err == nil {
		return true
	}

	return false
}

// Stat returns file info, preferring workspace version
func (l *LazyFileAccess) Stat(path string) (os.FileInfo, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Check if deleted
	if l.ws.IsFileDeleted(path) {
		return nil, os.ErrNotExist
	}

	// Check workspace first
	wsPath := l.ws.GetFilePath(path)
	if info, err := os.Stat(wsPath); err == nil {
		return info, nil
	}

	// Fall back to original
	originalPath := l.ws.GetOriginalPath(path)
	return os.Stat(originalPath)
}

// Copy copies a file within the workspace view
func (l *LazyFileAccess) Copy(src, dst string) error {
	content, err := l.Read(src)
	if err != nil {
		return fmt.Errorf("failed to read source: %w", err)
	}

	info, err := l.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source: %w", err)
	}

	return l.Write(dst, content, info.Mode())
}

// Rename renames a file within the workspace view
func (l *LazyFileAccess) Rename(oldPath, newPath string) error {
	// Read old file
	content, err := l.Read(oldPath)
	if err != nil {
		return fmt.Errorf("failed to read source: %w", err)
	}

	info, err := l.Stat(oldPath)
	if err != nil {
		return fmt.Errorf("failed to stat source: %w", err)
	}

	// Write to new location
	if err := l.Write(newPath, content, info.Mode()); err != nil {
		return fmt.Errorf("failed to write destination: %w", err)
	}

	// Delete old location
	if err := l.Delete(oldPath); err != nil {
		return fmt.Errorf("failed to delete source: %w", err)
	}

	return nil
}

// GetEffectivePath returns the actual file path to use for reading
// This is useful when external tools need direct file access
func (l *LazyFileAccess) GetEffectivePath(path string) (string, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Check if deleted
	if l.ws.IsFileDeleted(path) {
		return "", os.ErrNotExist
	}

	// Check workspace first
	wsPath := l.ws.GetFilePath(path)
	if _, err := os.Stat(wsPath); err == nil {
		return wsPath, nil
	}

	// Fall back to original
	originalPath := l.ws.GetOriginalPath(path)
	if _, err := os.Stat(originalPath); err == nil {
		return originalPath, nil
	}

	return "", os.ErrNotExist
}

// =============================================================================
// DIFF SUPPORT
// =============================================================================

// FileDiff represents the difference between workspace and original
type FileDiff struct {
	Path         string
	Type         DiffType
	OriginalHash string
	CurrentHash  string
	OriginalSize int64
	CurrentSize  int64
}

// DiffType indicates the type of change
type DiffType string

const (
	DiffTypeModified DiffType = "modified"
	DiffTypeCreated  DiffType = "created"
	DiffTypeDeleted  DiffType = "deleted"
)

// GetDiffs returns all differences between workspace and original project
func (l *LazyFileAccess) GetDiffs() ([]*FileDiff, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var diffs []*FileDiff

	// Modified files
	for path, entry := range l.ws.ModifiedFiles {
		diffs = append(diffs, &FileDiff{
			Path:         path,
			Type:         DiffTypeModified,
			OriginalHash: entry.OriginalHash,
			CurrentHash:  entry.CurrentHash,
			CurrentSize:  entry.Size,
		})
	}

	// Created files
	for path, entry := range l.ws.CreatedFiles {
		diffs = append(diffs, &FileDiff{
			Path:        path,
			Type:        DiffTypeCreated,
			CurrentHash: entry.CurrentHash,
			CurrentSize: entry.Size,
		})
	}

	// Deleted files
	for path := range l.ws.DeletedFiles {
		// Get original file info if possible
		var originalSize int64
		var originalHash string

		originalPath := l.ws.GetOriginalPath(path)
		if content, err := os.ReadFile(originalPath); err == nil {
			originalSize = int64(len(content))
			originalHash = HashContent(content)
		}

		diffs = append(diffs, &FileDiff{
			Path:         path,
			Type:         DiffTypeDeleted,
			OriginalHash: originalHash,
			OriginalSize: originalSize,
		})
	}

	return diffs, nil
}

// GetFileContent returns both original and current content for diff display
func (l *LazyFileAccess) GetFileContent(path string) (original, current []byte, diffType DiffType, err error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	originalPath := l.ws.GetOriginalPath(path)
	wsPath := l.ws.GetFilePath(path)

	// Determine diff type and get content
	if l.ws.IsFileDeleted(path) {
		original, err = os.ReadFile(originalPath)
		if err != nil {
			return nil, nil, "", fmt.Errorf("failed to read original: %w", err)
		}
		return original, nil, DiffTypeDeleted, nil
	}

	if l.ws.IsFileCreated(path) {
		current, err = os.ReadFile(wsPath)
		if err != nil {
			return nil, nil, "", fmt.Errorf("failed to read created file: %w", err)
		}
		return nil, current, DiffTypeCreated, nil
	}

	if l.ws.IsFileModified(path) {
		original, _ = os.ReadFile(originalPath)
		current, err = os.ReadFile(wsPath)
		if err != nil {
			return nil, nil, "", fmt.Errorf("failed to read modified file: %w", err)
		}
		return original, current, DiffTypeModified, nil
	}

	// File not tracked in workspace - read from original
	original, err = os.ReadFile(originalPath)
	if err != nil {
		return nil, nil, "", os.ErrNotExist
	}

	return original, original, "", nil
}

// =============================================================================
// EXECUTION SUPPORT
// =============================================================================

// PrepareExecutionDir ensures workspace is ready for shell command execution
// Returns the directory to use as working directory for commands
func (l *LazyFileAccess) PrepareExecutionDir() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// For execution, we use the workspace's files directory as overlay
	// Commands will see workspace files when they exist, original otherwise
	// This requires setting up proper symlinks or using the original dir

	// Simple approach: return original project dir but with env vars set
	// to indicate workspace context. More sophisticated: use overlayfs on Linux
	return l.ws.BaseDir, nil
}

// CopyUnmodifiedFile copies a file from original to workspace for direct access
// This is useful when a file needs to be in the workspace for command execution
func (l *LazyFileAccess) CopyUnmodifiedFile(path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Skip if already in workspace
	if l.ws.IsFileInWorkspace(path) {
		return nil
	}

	// Read original
	originalPath := l.ws.GetOriginalPath(path)
	content, err := os.ReadFile(originalPath)
	if err != nil {
		return err
	}

	info, err := os.Stat(originalPath)
	if err != nil {
		return err
	}

	// Copy to workspace without tracking as modified
	wsPath := l.ws.GetFilePath(path)
	if err := os.MkdirAll(filepath.Dir(wsPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(wsPath, content, info.Mode())
}

// StreamCopy copies a file using streaming (for large files)
func (l *LazyFileAccess) StreamCopy(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}
