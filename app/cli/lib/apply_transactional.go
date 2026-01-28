package lib

import (
	"fmt"
	"os"
	"plandex-cli/fs"
	"plandex-cli/term"
	"plandex-cli/types"
	"strings"

	shared "plandex-shared"

	"github.com/fatih/color"
)

// ApplyFilesWithTransaction applies file changes using the FileTransaction system
// for ACID guarantees. All changes are applied atomically - either all succeed
// or all are rolled back automatically on any failure.
//
// Benefits over non-transactional apply:
// - Atomicity: All files apply or none do
// - Automatic rollback on any error (file write, permissions, disk space, etc.)
// - Write-Ahead Log (WAL) for crash recovery
// - Progress tracking and user feedback
// - Consistent error handling
//
// Returns:
// - []string: List of successfully applied file paths
// - error: nil on success, or an error (changes already rolled back)
func ApplyFilesWithTransaction(
	planId, branch string,
	toApply map[string]string,
	toRemove map[string]bool,
	projectPaths *types.ProjectPaths,
) ([]string, error) {
	// Create transaction
	tx := shared.NewFileTransaction(planId, branch, fs.ProjectRoot)

	// Begin transaction (initializes WAL)
	if err := tx.Begin(); err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure rollback on panic
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback(fmt.Sprintf("panic during apply: %v", r))
			panic(r) // Re-panic after rollback
		}
	}()

	// Stage all operations (convert maps to transaction operations)
	if err := prepareOperations(tx, toApply, toRemove); err != nil {
		return nil, automaticRollback(tx, "staging failed", err)
	}

	// Apply operations sequentially with progress tracking
	updatedFiles, err := applyOperationsWithProgress(tx)
	if err != nil {
		return nil, automaticRollback(tx, "apply failed", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, automaticRollback(tx, "commit failed", err)
	}

	return updatedFiles, nil
}

// prepareOperations converts toApply and toRemove maps into FileTransaction operations.
// This stages all operations but doesn't apply them yet.
func prepareOperations(
	tx *shared.FileTransaction,
	toApply map[string]string,
	toRemove map[string]bool,
) error {
	// Stage file creations/modifications
	for path, content := range toApply {
		// Skip the apply script - it's handled separately
		if path == "_apply.sh" {
			continue
		}

		// Unescape escaped backticks
		content = strings.ReplaceAll(content, "\\`\\`\\`", "```")

		// Check if file exists to determine operation type
		fullPath := tx.BaseDir + "/" + path
		if fileExists(fullPath) {
			// Modify existing file
			if err := tx.ModifyFile(path, content); err != nil {
				return fmt.Errorf("failed to stage modification of %s: %w", path, err)
			}
		} else {
			// Create new file
			if err := tx.CreateFile(path, content); err != nil {
				return fmt.Errorf("failed to stage creation of %s: %w", path, err)
			}
		}
	}

	// Stage file deletions
	for path, shouldRemove := range toRemove {
		if !shouldRemove {
			continue
		}

		if err := tx.DeleteFile(path); err != nil {
			return fmt.Errorf("failed to stage deletion of %s: %w", path, err)
		}
	}

	return nil
}

// applyOperationsWithProgress applies all staged operations sequentially,
// showing progress to the user for each file.
func applyOperationsWithProgress(tx *shared.FileTransaction) ([]string, error) {
	var updatedFiles []string
	total := len(tx.Operations)

	if total == 0 {
		return updatedFiles, nil
	}

	// Apply operations with progress feedback
	err := tx.ApplyAllWithProgress(func(op *shared.FileOperation, current, total int) {
		// Track updated files
		updatedFiles = append(updatedFiles, op.Path)

		// Show progress to user
		term.StopSpinner()
		color.New(term.ColorHiCyan).Printf("🔄 Applying [%d/%d] %s\n", current, total, op.Path)
		term.ResumeSpinner()
	})

	if err != nil {
		return nil, err
	}

	return updatedFiles, nil
}

// automaticRollback performs automatic rollback and returns a user-friendly error.
// The transaction is rolled back, WAL entries are written, and snapshots are restored.
func automaticRollback(tx *shared.FileTransaction, reason string, originalErr error) error {
	term.StopSpinner()

	// Count operations that were applied vs pending
	var appliedCount, pendingCount int
	for _, op := range tx.Operations {
		if op.Status == shared.FileOpApplied {
			appliedCount++
		} else if op.Status == shared.FileOpPending {
			pendingCount++
		}
	}

	// Perform rollback
	rbErr := tx.Rollback(reason)

	// Format user-friendly message
	var msg strings.Builder
	fmt.Fprintf(&msg, "\n")
	color.New(term.ColorHiRed, color.Bold).Fprintf(&msg, "❌ %s: %v\n", reason, originalErr)
	fmt.Fprintf(&msg, "\n")

	if appliedCount > 0 {
		color.New(term.ColorHiYellow, color.Bold).Fprintf(&msg, "🚫 Rolled back %d applied file changes\n", appliedCount)
		fmt.Fprintf(&msg, "   All files have been restored to their original state\n")
	} else {
		fmt.Fprintf(&msg, "   No files were modified\n")
	}

	if rbErr != nil {
		color.New(term.ColorHiRed).Fprintf(&msg, "   ⚠️  Rollback encountered errors: %v\n", rbErr)
		fmt.Fprintf(&msg, "   Some files may not have been fully restored\n")
	}

	fmt.Print(msg.String())

	// Return wrapped error
	if rbErr != nil {
		return fmt.Errorf("rollback failed (%w) after %s: %w", rbErr, reason, originalErr)
	}
	return fmt.Errorf("%s (rolled back): %w", reason, originalErr)
}

// fileExists checks if a file exists at the given path
func fileExists(path string) bool {
	if path == "" {
		return false
	}
	// Use the same logic as the original ApplyFiles
	_, err := os.Stat(path)
	return err == nil
}
