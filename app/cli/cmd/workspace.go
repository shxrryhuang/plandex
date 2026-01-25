package cmd

import (
	"fmt"
	"os"
	"plandex-cli/auth"
	"plandex-cli/fs"
	"plandex-cli/lib"
	"plandex-cli/term"
	"plandex-cli/workspace"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func init() {
	// Add subcommands
	workspaceCmd.AddCommand(workspaceStatusCmd)
	workspaceCmd.AddCommand(workspaceCommitCmd)
	workspaceCmd.AddCommand(workspaceDiscardCmd)
	workspaceCmd.AddCommand(workspaceDiffCmd)
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceCleanCmd)

	// Add commit confirmation flag
	workspaceCommitCmd.Flags().BoolVarP(&wsCommitForce, "force", "f", false, "Skip confirmation prompt")

	// Add discard confirmation flag
	workspaceDiscardCmd.Flags().BoolVarP(&wsDiscardForce, "force", "f", false, "Skip confirmation prompt")

	// Add clean flags
	workspaceCleanCmd.Flags().BoolVarP(&wsCleanAll, "all", "a", false, "Clean all completed/stale workspaces")
	workspaceCleanCmd.Flags().IntVarP(&wsCleanDays, "days", "d", 7, "Clean workspaces older than N days")

	// Register with root
	RootCmd.AddCommand(workspaceCmd)
}

var wsCommitForce bool
var wsDiscardForce bool
var wsCleanAll bool
var wsCleanDays int

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "Manage isolated workspaces",
	Long: `Manage isolated workspaces for plan execution.

Workspaces provide a safe environment for file edits and commands,
preventing accidental changes to your main project files until you
explicitly commit them.

Commands:
  status   - Show current workspace status
  commit   - Apply workspace changes to main project
  discard  - Reject workspace changes
  diff     - Show differences between workspace and main project
  list     - List all workspaces for current project
  clean    - Remove stale/completed workspaces`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default to status if no subcommand
		workspaceStatus(cmd, args)
	},
}

var workspaceStatusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st"},
	Short:   "Show current workspace status",
	Run:     workspaceStatus,
}

var workspaceCommitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Apply workspace changes to main project",
	Long: `Apply all changes from the isolated workspace to your main project.

This will:
  - Copy modified files back to your project
  - Create new files in your project
  - Delete files marked for removal

Use --force to skip the confirmation prompt.`,
	Run: workspaceCommit,
}

var workspaceDiscardCmd = &cobra.Command{
	Use:   "discard",
	Short: "Reject workspace changes",
	Long: `Discard all changes in the isolated workspace without affecting
your main project. The workspace will be marked as discarded and
may be cleaned up automatically.

Use --force to skip the confirmation prompt.`,
	Run: workspaceDiscard,
}

var workspaceDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show differences between workspace and main project",
	Run:   workspaceDiff,
}

var workspaceListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all workspaces for current project",
	Run:     workspaceList,
}

var workspaceCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove stale/completed workspaces",
	Long: `Remove old, completed, or discarded workspaces to free disk space.

By default, removes workspaces that are:
  - Committed (changes already applied)
  - Discarded (changes rejected)
  - Stale (not accessed for 7+ days)

Use --all to clean all eligible workspaces.
Use --days N to change the stale threshold.`,
	Run: workspaceClean,
}

// =============================================================================
// COMMAND IMPLEMENTATIONS
// =============================================================================

func workspaceStatus(cmd *cobra.Command, args []string) {
	auth.MustResolveAuthWithOrg()
	lib.MustResolveProject()

	if lib.CurrentPlanId == "" {
		term.OutputSimpleError("No current plan. Create or select a plan first.")
		fmt.Println()
		term.PrintCmds("", "new", "plans")
		return
	}

	mgr := getWorkspaceManager()
	ws, err := mgr.GetActiveWorkspace(lib.CurrentPlanId, lib.CurrentBranch)
	if err != nil {
		term.OutputErrorAndExit("Error loading workspace: %v", err)
	}

	if ws == nil {
		fmt.Println("No active workspace for current plan/branch.")
		fmt.Println()
		fmt.Println("A workspace will be created automatically when you apply changes.")
		fmt.Println()
		term.PrintCmds("", "apply")
		return
	}

	printWorkspaceStatus(ws)
}

func workspaceCommit(cmd *cobra.Command, args []string) {
	auth.MustResolveAuthWithOrg()
	lib.MustResolveProject()

	if lib.CurrentPlanId == "" {
		term.OutputNoCurrentPlanErrorAndExit()
	}

	mgr := getWorkspaceManager()
	ws, err := mgr.GetActiveWorkspace(lib.CurrentPlanId, lib.CurrentBranch)
	if err != nil {
		term.OutputErrorAndExit("Error loading workspace: %v", err)
	}

	if ws == nil {
		fmt.Println("No active workspace to commit.")
		return
	}

	if !ws.HasChanges() {
		fmt.Println("No changes to commit.")
		ws.SetState(workspace.WorkspaceStateCommitted)
		ws.Save()
		return
	}

	// Show what will be committed
	printWorkspaceChanges(ws)

	if !wsCommitForce {
		fmt.Println()
		confirmed, err := term.ConfirmYesNo("Apply these changes to your main project?")
		if err != nil {
			term.OutputErrorAndExit("Error getting confirmation: %v", err)
		}
		if !confirmed {
			fmt.Println("Commit cancelled.")
			return
		}
	}

	term.StartSpinner("Committing workspace changes...")

	if err := mgr.Commit(ws); err != nil {
		term.StopSpinner()
		term.OutputErrorAndExit("Error committing workspace: %v", err)
	}

	term.StopSpinner()

	color.New(color.FgGreen, color.Bold).Println("Workspace changes committed successfully!")
	fmt.Println()
	fmt.Println("Your main project files have been updated.")
}

func workspaceDiscard(cmd *cobra.Command, args []string) {
	auth.MustResolveAuthWithOrg()
	lib.MustResolveProject()

	if lib.CurrentPlanId == "" {
		term.OutputNoCurrentPlanErrorAndExit()
	}

	mgr := getWorkspaceManager()
	ws, err := mgr.GetActiveWorkspace(lib.CurrentPlanId, lib.CurrentBranch)
	if err != nil {
		term.OutputErrorAndExit("Error loading workspace: %v", err)
	}

	if ws == nil {
		fmt.Println("No active workspace to discard.")
		return
	}

	if ws.HasChanges() {
		printWorkspaceChanges(ws)

		if !wsDiscardForce {
			fmt.Println()
			color.New(color.FgYellow, color.Bold).Println("Warning: This will permanently discard all workspace changes!")
			confirmed, err := term.ConfirmYesNo("Discard all changes?")
			if err != nil {
				term.OutputErrorAndExit("Error getting confirmation: %v", err)
			}
			if !confirmed {
				fmt.Println("Discard cancelled.")
				return
			}
		}
	}

	term.StartSpinner("Discarding workspace...")

	if err := mgr.Discard(ws); err != nil {
		term.StopSpinner()
		term.OutputErrorAndExit("Error discarding workspace: %v", err)
	}

	term.StopSpinner()

	color.New(color.FgYellow).Println("Workspace discarded.")
	fmt.Println()
	fmt.Println("Your main project files remain unchanged.")
}

func workspaceDiff(cmd *cobra.Command, args []string) {
	auth.MustResolveAuthWithOrg()
	lib.MustResolveProject()

	if lib.CurrentPlanId == "" {
		term.OutputNoCurrentPlanErrorAndExit()
	}

	mgr := getWorkspaceManager()
	ws, err := mgr.GetActiveWorkspace(lib.CurrentPlanId, lib.CurrentBranch)
	if err != nil {
		term.OutputErrorAndExit("Error loading workspace: %v", err)
	}

	if ws == nil {
		fmt.Println("No active workspace.")
		return
	}

	if !ws.HasChanges() {
		fmt.Println("No changes in workspace.")
		return
	}

	// Get diffs
	fa := workspace.NewLazyFileAccess(ws)
	diffs, err := fa.GetDiffs()
	if err != nil {
		term.OutputErrorAndExit("Error getting diffs: %v", err)
	}

	for _, diff := range diffs {
		printFileDiff(fa, diff)
	}
}

func workspaceList(cmd *cobra.Command, args []string) {
	auth.MustResolveAuthWithOrg()
	lib.MustResolveProject()

	mgr := getWorkspaceManager()
	summaries, err := mgr.ListWorkspaces()
	if err != nil {
		term.OutputErrorAndExit("Error listing workspaces: %v", err)
	}

	if len(summaries) == 0 {
		fmt.Println("No workspaces found for this project.")
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Plan", "Branch", "State", "Changes", "Last Access"})
	table.SetBorder(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetTablePadding("\t")
	table.SetNoWhiteSpace(true)

	for _, s := range summaries {
		changes := fmt.Sprintf("%dM %dA %dD", s.ModifiedCount, s.CreatedCount, s.DeletedCount)
		lastAccess := formatTimeAgo(s.LastAccessedAt)

		// Highlight active workspace
		id := s.Id[:8]
		if s.State == workspace.WorkspaceStateActive {
			id = color.New(color.FgGreen, color.Bold).Sprint(id)
		}

		table.Append([]string{
			id,
			truncate(s.PlanId, 12),
			s.Branch,
			string(s.State),
			changes,
			lastAccess,
		})
	}

	table.Render()
}

func workspaceClean(cmd *cobra.Command, args []string) {
	auth.MustResolveAuthWithOrg()
	lib.MustResolveProject()

	mgr := getWorkspaceManager()

	// Update config with custom days if specified
	if cmd.Flags().Changed("days") {
		mgr.GetConfig().StaleAfterDays = wsCleanDays
	}

	if wsCleanAll {
		mgr.GetConfig().CleanupBatchSize = 100 // Higher limit for --all
	}

	term.StartSpinner("Cleaning stale workspaces...")

	cleaned, err := mgr.CleanupStaleWorkspaces()
	if err != nil {
		term.StopSpinner()
		term.OutputErrorAndExit("Error cleaning workspaces: %v", err)
	}

	term.StopSpinner()

	if cleaned == 0 {
		fmt.Println("No workspaces to clean.")
	} else {
		fmt.Printf("Cleaned %d workspace(s).\n", cleaned)
	}
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func getWorkspaceManager() *workspace.Manager {
	config := workspace.DefaultWorkspaceConfig()
	return workspace.NewManager(fs.HomePlandexDir, fs.ProjectRoot, fs.PlandexDir, config)
}

func printWorkspaceStatus(ws *workspace.Workspace) {
	fmt.Println()
	color.New(color.Bold).Println("Workspace Status")
	fmt.Println(strings.Repeat("=", 40))

	// State indicator with color
	stateColor := color.New(color.FgWhite)
	stateIcon := ""
	switch ws.State {
	case workspace.WorkspaceStateActive:
		stateColor = color.New(color.FgGreen, color.Bold)
		stateIcon = "[ACTIVE]"
	case workspace.WorkspaceStatePending:
		stateColor = color.New(color.FgCyan)
		stateIcon = "[PENDING]"
	case workspace.WorkspaceStateCommitted:
		stateColor = color.New(color.FgBlue)
		stateIcon = "[COMMITTED]"
	case workspace.WorkspaceStateDiscarded:
		stateColor = color.New(color.FgYellow)
		stateIcon = "[DISCARDED]"
	case workspace.WorkspaceStateRecovering:
		stateColor = color.New(color.FgRed, color.Bold)
		stateIcon = "[RECOVERING]"
	}

	fmt.Printf("%-14s %s\n", "ID:", ws.GetShortId())
	fmt.Printf("%-14s ", "State:")
	stateColor.Println(stateIcon)
	fmt.Printf("%-14s %s\n", "Plan:", ws.PlanId)
	fmt.Printf("%-14s %s\n", "Branch:", ws.Branch)
	fmt.Printf("%-14s %s\n", "Created:", ws.CreatedAt.Format(time.RFC3339))
	fmt.Printf("%-14s %s\n", "Last Access:", formatTimeAgo(ws.LastAccessedAt))
	fmt.Println()

	modified, created, deleted := ws.GetChangeCount()
	if modified+created+deleted == 0 {
		fmt.Println("No changes in workspace.")
	} else {
		printWorkspaceChanges(ws)
	}

	fmt.Println()
	term.PrintCmds("", "workspace commit", "workspace discard", "workspace diff")
}

func printWorkspaceChanges(ws *workspace.Workspace) {
	modified, created, deleted := ws.GetChangeCount()

	color.New(color.Bold).Println("Changes:")

	if modified > 0 {
		color.New(color.FgYellow).Printf("  Modified: %d file(s)\n", modified)
		for path := range ws.ModifiedFiles {
			fmt.Printf("    M  %s\n", path)
		}
	}

	if created > 0 {
		color.New(color.FgGreen).Printf("  Created: %d file(s)\n", created)
		for path := range ws.CreatedFiles {
			fmt.Printf("    +  %s\n", path)
		}
	}

	if deleted > 0 {
		color.New(color.FgRed).Printf("  Deleted: %d file(s)\n", deleted)
		for path := range ws.DeletedFiles {
			fmt.Printf("    -  %s\n", path)
		}
	}
}

func printFileDiff(fa *workspace.LazyFileAccess, diff *workspace.FileDiff) {
	fmt.Println()

	// Header with color based on type
	switch diff.Type {
	case workspace.DiffTypeModified:
		color.New(color.FgYellow, color.Bold).Printf("Modified: %s\n", diff.Path)
	case workspace.DiffTypeCreated:
		color.New(color.FgGreen, color.Bold).Printf("Created: %s\n", diff.Path)
	case workspace.DiffTypeDeleted:
		color.New(color.FgRed, color.Bold).Printf("Deleted: %s\n", diff.Path)
	}

	fmt.Println(strings.Repeat("-", 60))

	// Get content
	original, current, _, err := fa.GetFileContent(diff.Path)
	if err != nil {
		fmt.Printf("  Error reading file: %v\n", err)
		return
	}

	switch diff.Type {
	case workspace.DiffTypeModified:
		// Simple diff display
		fmt.Printf("Original (%d bytes) -> Current (%d bytes)\n", len(original), len(current))
		if diff.OriginalHash != diff.CurrentHash {
			fmt.Printf("Hash: %s -> %s\n", truncate(diff.OriginalHash, 16), truncate(diff.CurrentHash, 16))
		}

	case workspace.DiffTypeCreated:
		fmt.Printf("New file: %d bytes\n", len(current))

	case workspace.DiffTypeDeleted:
		fmt.Printf("Removed: %d bytes\n", len(original))
	}
}

func formatTimeAgo(t time.Time) string {
	d := time.Since(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
