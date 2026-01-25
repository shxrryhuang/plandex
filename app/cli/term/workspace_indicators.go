package term

import (
	"fmt"
	"os"
	"strings"
	"time"

	"plandex-cli/workspace"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
)

// =============================================================================
// WORKSPACE UI INDICATORS
// =============================================================================
//
// This module provides visual indicators and status displays for workspace
// isolation. It helps users understand when they're working in an isolated
// environment and the current state of their workspace.
//
// =============================================================================

// ShowWorkspaceIndicator displays a banner showing workspace status
// This should be called at the start of operations that use a workspace
func ShowWorkspaceIndicator(ws *workspace.Workspace) {
	if ws == nil {
		return
	}

	fmt.Println()

	// Workspace ID and state
	stateColor := getWorkspaceStateColor(ws.State)
	stateText := getWorkspaceStateText(ws.State)

	color.New(ColorHiYellow, color.Bold).Printf("[WORKSPACE: %s] ", ws.GetShortId())
	stateColor.Println(stateText)

	// Show change summary if any
	modified, created, deleted := ws.GetChangeCount()
	if modified+created+deleted > 0 {
		fmt.Printf("  Changes: %d modified, %d created, %d deleted\n",
			modified, created, deleted)
	}

	fmt.Println()
}

// ShowWorkspaceBanner displays a prominent banner for workspace mode
func ShowWorkspaceBanner(ws *workspace.Workspace) {
	if ws == nil {
		return
	}

	fmt.Println()
	color.New(color.BgYellow, color.FgBlack, color.Bold).Println(" ISOLATED WORKSPACE ")
	fmt.Println()

	fmt.Printf("  ID:     %s\n", ws.GetShortId())
	fmt.Printf("  State:  %s\n", ws.State)
	fmt.Printf("  Plan:   %s\n", ws.PlanId)
	fmt.Printf("  Branch: %s\n", ws.Branch)

	modified, created, deleted := ws.GetChangeCount()
	if modified+created+deleted > 0 {
		fmt.Println()
		fmt.Printf("  Changes: %dM %dA %dD\n", modified, created, deleted)
	}

	fmt.Println()
	color.New(color.FgYellow).Println("  All file changes are isolated from your main project.")
	color.New(color.FgYellow).Println("  Use 'plandex workspace commit' to apply changes.")
	fmt.Println()
}

// GetWorkspacePromptPrefix returns a prefix string for REPL prompts
func GetWorkspacePromptPrefix(ws *workspace.Workspace) string {
	if ws == nil || ws.State != workspace.WorkspaceStateActive {
		return ""
	}

	return color.New(ColorHiYellow).Sprintf("[ws:%s] ", ws.GetShortId()[:6])
}

// GetWorkspaceStatusLine returns a one-line status for display
func GetWorkspaceStatusLine(ws *workspace.Workspace) string {
	if ws == nil {
		return ""
	}

	stateIcon := getWorkspaceStateIcon(ws.State)
	modified, created, deleted := ws.GetChangeCount()

	if modified+created+deleted == 0 {
		return fmt.Sprintf("%s Workspace %s (no changes)", stateIcon, ws.GetShortId())
	}

	return fmt.Sprintf("%s Workspace %s (%dM %dA %dD)",
		stateIcon, ws.GetShortId(), modified, created, deleted)
}

// PrintWorkspaceStatus displays detailed workspace status
func PrintWorkspaceStatus(ws *workspace.Workspace) {
	if ws == nil {
		fmt.Println("No active workspace for current plan/branch")
		fmt.Println()
		PrintCmds("", "apply")
		return
	}

	fmt.Println()
	color.New(color.Bold).Println("Workspace Status")
	fmt.Println(strings.Repeat("=", 50))

	// Identity info
	fmt.Printf("%-16s %s\n", "ID:", ws.Id)
	fmt.Printf("%-16s %s\n", "Short ID:", ws.GetShortId())
	fmt.Printf("%-16s ", "State:")
	getWorkspaceStateColor(ws.State).Println(getWorkspaceStateText(ws.State))
	fmt.Printf("%-16s %s\n", "Plan:", ws.PlanId)
	fmt.Printf("%-16s %s\n", "Branch:", ws.Branch)
	fmt.Println()

	// Timestamps
	fmt.Printf("%-16s %s\n", "Created:", ws.CreatedAt.Format(time.RFC3339))
	fmt.Printf("%-16s %s\n", "Updated:", ws.UpdatedAt.Format(time.RFC3339))
	fmt.Printf("%-16s %s (%s)\n", "Last Access:",
		ws.LastAccessedAt.Format(time.RFC3339),
		formatDuration(time.Since(ws.LastAccessedAt)))

	if ws.CrashRecoveryAt != nil {
		fmt.Printf("%-16s %s\n", "Recovered:", ws.CrashRecoveryAt.Format(time.RFC3339))
	}

	fmt.Println()

	// File changes
	PrintWorkspaceFiles(ws)

	fmt.Println()
	PrintCmds("", "workspace commit", "workspace discard", "workspace diff")
}

// PrintWorkspaceFiles displays the files in a workspace
func PrintWorkspaceFiles(ws *workspace.Workspace) {
	modified, created, deleted := ws.GetChangeCount()

	if modified+created+deleted == 0 {
		fmt.Println("No changes in workspace.")
		return
	}

	color.New(color.Bold).Println("Workspace Files:")
	fmt.Println()

	if len(ws.ModifiedFiles) > 0 {
		color.New(color.FgYellow).Println("  Modified:")
		for path, entry := range ws.ModifiedFiles {
			fmt.Printf("    M  %-40s (%s)\n", truncatePath(path, 40), formatBytes(entry.Size))
		}
	}

	if len(ws.CreatedFiles) > 0 {
		color.New(color.FgGreen).Println("  Created:")
		for path, entry := range ws.CreatedFiles {
			fmt.Printf("    +  %-40s (%s)\n", truncatePath(path, 40), formatBytes(entry.Size))
		}
	}

	if len(ws.DeletedFiles) > 0 {
		color.New(color.FgRed).Println("  Deleted:")
		for path := range ws.DeletedFiles {
			fmt.Printf("    -  %s\n", path)
		}
	}
}

// PrintWorkspaceSummaryTable displays a table of workspaces
func PrintWorkspaceSummaryTable(summaries []*workspace.WorkspaceSummary) {
	if len(summaries) == 0 {
		fmt.Println("No workspaces found.")
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "State", "Plan", "Branch", "Modified", "Created", "Deleted", "Last Access"})
	table.SetBorder(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for _, s := range summaries {
		stateColor := getWorkspaceStateColor(s.State)
		stateText := stateColor.Sprint(string(s.State))

		table.Append([]string{
			s.Id[:8],
			stateText,
			truncatePath(s.PlanId, 15),
			s.Branch,
			fmt.Sprintf("%d", s.ModifiedCount),
			fmt.Sprintf("%d", s.CreatedCount),
			fmt.Sprintf("%d", s.DeletedCount),
			formatDuration(time.Since(s.LastAccessedAt)) + " ago",
		})
	}

	table.Render()
}

// =============================================================================
// INLINE INDICATORS
// =============================================================================

// WorkspaceFilePrefix returns a prefix indicating file is in workspace
func WorkspaceFilePrefix() string {
	return color.New(ColorHiYellow).Sprint("[ws] ")
}

// WorkspaceModifiedIndicator returns indicator for modified files
func WorkspaceModifiedIndicator() string {
	return color.New(ColorHiYellow).Sprint("M ")
}

// WorkspaceCreatedIndicator returns indicator for created files
func WorkspaceCreatedIndicator() string {
	return color.New(ColorHiGreen).Sprint("+ ")
}

// WorkspaceDeletedIndicator returns indicator for deleted files
func WorkspaceDeletedIndicator() string {
	return color.New(ColorHiRed).Sprint("- ")
}

// =============================================================================
// CONFIRMATION PROMPTS
// =============================================================================

// ConfirmWorkspaceCommit prompts for commit confirmation
func ConfirmWorkspaceCommit(ws *workspace.Workspace) (bool, error) {
	if ws == nil {
		return false, fmt.Errorf("no workspace")
	}

	modified, created, deleted := ws.GetChangeCount()

	fmt.Println()
	color.New(color.Bold).Println("Commit Workspace Changes")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("  Modified: %d file(s)\n", modified)
	fmt.Printf("  Created:  %d file(s)\n", created)
	fmt.Printf("  Deleted:  %d file(s)\n", deleted)
	fmt.Println()

	return ConfirmYesNo("Apply these changes to your main project?")
}

// ConfirmWorkspaceDiscard prompts for discard confirmation
func ConfirmWorkspaceDiscard(ws *workspace.Workspace) (bool, error) {
	if ws == nil {
		return false, fmt.Errorf("no workspace")
	}

	if !ws.HasChanges() {
		return true, nil // No changes, no confirmation needed
	}

	modified, created, deleted := ws.GetChangeCount()

	fmt.Println()
	color.New(color.FgYellow, color.Bold).Println("Discard Workspace Changes")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("  Modified: %d file(s)\n", modified)
	fmt.Printf("  Created:  %d file(s)\n", created)
	fmt.Printf("  Deleted:  %d file(s)\n", deleted)
	fmt.Println()

	color.New(color.FgRed).Println("Warning: This action cannot be undone!")
	fmt.Println()

	return ConfirmYesNo("Permanently discard all changes?")
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func getWorkspaceStateColor(state workspace.WorkspaceState) *color.Color {
	switch state {
	case workspace.WorkspaceStateActive:
		return color.New(ColorHiGreen, color.Bold)
	case workspace.WorkspaceStatePending:
		return color.New(ColorHiCyan)
	case workspace.WorkspaceStateCommitted:
		return color.New(ColorHiBlue)
	case workspace.WorkspaceStateDiscarded:
		return color.New(ColorHiYellow)
	case workspace.WorkspaceStateRecovering:
		return color.New(ColorHiRed, color.Bold)
	default:
		return color.New(color.FgWhite)
	}
}

func getWorkspaceStateText(state workspace.WorkspaceState) string {
	switch state {
	case workspace.WorkspaceStateActive:
		return "Active - changes isolated"
	case workspace.WorkspaceStatePending:
		return "Pending - ready for use"
	case workspace.WorkspaceStateCommitted:
		return "Committed - changes applied"
	case workspace.WorkspaceStateDiscarded:
		return "Discarded - changes rejected"
	case workspace.WorkspaceStateRecovering:
		return "Recovering - needs attention"
	default:
		return string(state)
	}
}

func getWorkspaceStateIcon(state workspace.WorkspaceState) string {
	switch state {
	case workspace.WorkspaceStateActive:
		return color.New(ColorHiGreen).Sprint("[*]")
	case workspace.WorkspaceStatePending:
		return color.New(ColorHiCyan).Sprint("[ ]")
	case workspace.WorkspaceStateCommitted:
		return color.New(ColorHiBlue).Sprint("[+]")
	case workspace.WorkspaceStateDiscarded:
		return color.New(ColorHiYellow).Sprint("[x]")
	case workspace.WorkspaceStateRecovering:
		return color.New(ColorHiRed).Sprint("[!]")
	default:
		return "[ ]"
	}
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min"
		}
		return fmt.Sprintf("%d mins", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	// Keep the end of the path (filename is usually more important)
	return "..." + path[len(path)-maxLen+3:]
}
