package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"plandex-cli/auth"
	"plandex-cli/term"
	"plandex-cli/version"

	shared "plandex-shared"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// diagnoseCmd is the parent command for diagnostics
var diagnoseCmd = &cobra.Command{
	Use:     "diagnose",
	Aliases: []string{"diag", "dx"},
	Short:   "Diagnostics and error inspection",
	Long: `Access error history, export debug info, and troubleshoot issues.

Use subcommands to:
  - View recent errors: diagnose errors
  - Export debug bundle: diagnose export
  - Check system status: diagnose status
  - Toggle debug mode: diagnose debug-mode`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// diagnose errors flags
var (
	diagnoseErrorsLast     int
	diagnoseErrorsCategory string
	diagnoseErrorsPlanId   string
	diagnoseErrorsAll      bool
)

// diagnose export flags
var (
	diagnoseExportOutput string
	diagnoseExportFormat string
)

// diagnose debug-mode flags
var (
	debugModeLevel     string
	debugModeTraceFile string
)

func init() {
	RootCmd.AddCommand(diagnoseCmd)

	// diagnose errors
	errorsCmd := &cobra.Command{
		Use:   "errors [error-id]",
		Short: "View recent errors",
		Long: `View recent errors from the error registry.

Without arguments, shows a summary of recent errors.
With an error ID, shows full details for that error.`,
		Run: runDiagnoseErrors,
	}
	errorsCmd.Flags().IntVarP(&diagnoseErrorsLast, "last", "n", 10, "Number of recent errors to show")
	errorsCmd.Flags().StringVarP(&diagnoseErrorsCategory, "category", "c", "", "Filter by category (provider, file_system, validation, internal)")
	errorsCmd.Flags().StringVarP(&diagnoseErrorsPlanId, "plan", "p", "", "Filter by plan ID")
	errorsCmd.Flags().BoolVarP(&diagnoseErrorsAll, "all", "a", false, "Show all errors (ignores --last)")
	diagnoseCmd.AddCommand(errorsCmd)

	// diagnose export
	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export debug information bundle",
		Long: `Export a debug bundle containing:
  - System information
  - Error history (sanitized)
  - Configuration (no secrets)
  - Debug traces (if enabled)

The export is safe to share - all sensitive data is automatically sanitized.`,
		Run: runDiagnoseExport,
	}
	exportCmd.Flags().StringVarP(&diagnoseExportOutput, "output", "o", "", "Output file path (default: plandex-debug-<timestamp>.json)")
	exportCmd.Flags().StringVarP(&diagnoseExportFormat, "format", "f", "json", "Output format: json or text")
	diagnoseCmd.AddCommand(exportCmd)

	// diagnose status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show system health and status",
		Long:  `Display system diagnostics including connection status, provider configuration, and recent issues.`,
		Run:   runDiagnoseStatus,
	}
	diagnoseCmd.AddCommand(statusCmd)

	// diagnose debug-mode
	debugModeCmd := &cobra.Command{
		Use:   "debug-mode [on|off]",
		Short: "Enable or disable debug mode",
		Long: `Toggle debug mode for capturing detailed traces.

When enabled, debug mode captures:
  - Execution traces at the specified level
  - Timing information
  - Error context

Levels:
  - info:  Important events only
  - debug: Detailed debugging information
  - trace: Everything including step-by-step traces`,
		Run: runDiagnoseDebugMode,
	}
	debugModeCmd.Flags().StringVarP(&debugModeLevel, "level", "l", "debug", "Debug level: info, debug, or trace")
	debugModeCmd.Flags().StringVarP(&debugModeTraceFile, "trace-file", "t", "", "Write traces to this file")
	diagnoseCmd.AddCommand(debugModeCmd)

	// diagnose clear
	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear error history",
		Long:  `Clear all stored errors from the error registry.`,
		Run:   runDiagnoseClear,
	}
	diagnoseCmd.AddCommand(clearCmd)
}

// =============================================================================
// DIAGNOSE ERRORS
// =============================================================================

func runDiagnoseErrors(cmd *cobra.Command, args []string) {
	// Initialize registry if needed
	ensureErrorRegistry()

	// If an error ID is provided, show details for that error
	if len(args) > 0 {
		showErrorDetails(args[0])
		return
	}

	// Build filter
	filter := shared.ErrorFilter{}

	if diagnoseErrorsCategory != "" {
		filter.Category = shared.ErrorCategory(diagnoseErrorsCategory)
	}

	if diagnoseErrorsPlanId != "" {
		filter.PlanId = diagnoseErrorsPlanId
	}

	if !diagnoseErrorsAll {
		filter.Limit = diagnoseErrorsLast
	}

	// Get errors
	errors := shared.ListErrors(filter)

	if len(errors) == 0 {
		fmt.Println("No errors found.")
		fmt.Println()
		term.PrintCmds("", "diagnose status")
		return
	}

	// Display errors
	fmt.Println()
	printErrorsHeader(len(errors))

	for i, err := range errors {
		printErrorSummary(i+1, err)
	}

	fmt.Println()
	fmt.Println("Use 'plandex diagnose errors <id>' for full details")
	fmt.Println()
	term.PrintCmds("", "diagnose export", "diagnose clear")
}

func showErrorDetails(id string) {
	err := shared.GetError(id)
	if err == nil {
		term.OutputErrorAndExit("Error not found: %s", id)
	}

	// Use the existing Format() method for detailed output
	fmt.Println(err.ErrorReport.Format())

	// Show additional stored error metadata
	fmt.Println()
	color.New(color.Bold).Println("▌ STORAGE METADATA")
	fmt.Println("├─────────────────────────────────────────────────────────────────")
	fmt.Printf("│ Stored At:  %s\n", err.StoredAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("│ Session:    %s\n", err.SessionId)
	if err.Resolved {
		fmt.Printf("│ Resolved:   Yes (%s)\n", err.ResolvedBy)
		if err.ResolvedAt != nil {
			fmt.Printf("│ Resolved At: %s\n", err.ResolvedAt.Format("2006-01-02 15:04:05"))
		}
	} else {
		fmt.Println("│ Resolved:   No")
	}
	if err.RetryCount > 0 {
		fmt.Printf("│ Retries:    %d\n", err.RetryCount)
	}
	fmt.Println()
}

func printErrorsHeader(count int) {
	header := fmt.Sprintf("                     RECENT ERRORS (%d)", count)
	fmt.Println("╔═══════════════════════════════════════════════════════════════════╗")
	fmt.Printf("║%s║\n", centerString(header, 67))
	fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func printErrorSummary(num int, err *shared.StoredError) {
	// [1] 2024-01-25 14:32:15 | err_1706193135000000000
	fmt.Printf("[%d] %s | %s\n",
		num,
		err.StoredAt.Format("2006-01-02 15:04:05"),
		err.Id)

	fmt.Printf("    Category: %s\n", err.RootCause.Category)
	fmt.Printf("    Type:     %s\n", err.RootCause.Type)

	// Truncate long messages
	msg := err.RootCause.Message
	if len(msg) > 60 {
		msg = msg[:57] + "..."
	}
	fmt.Printf("    Message:  %s\n", msg)

	// Plan info if available
	if err.StepContext != nil && err.StepContext.PlanId != "" {
		planInfo := err.StepContext.PlanId
		if err.StepContext.Branch != "" {
			planInfo += fmt.Sprintf(" (branch: %s)", err.StepContext.Branch)
		}
		fmt.Printf("    Plan:     %s\n", planInfo)
	}

	// Recovery status
	if err.Resolved {
		resolvedInfo := "Yes"
		if err.ResolvedBy != "" {
			resolvedInfo += fmt.Sprintf(" (%s)", err.ResolvedBy)
		}
		fmt.Printf("    Recovery: ✓ %s\n", resolvedInfo)
	} else {
		fmt.Println("    Recovery: ✗ Manual action required")

		// Show first action if available
		if err.Recovery != nil && len(err.Recovery.ManualActions) > 0 {
			action := err.Recovery.ManualActions[0]
			fmt.Printf("    Actions:\n")
			fmt.Printf("      1. [%s] %s\n", strings.ToUpper(action.Priority), action.Description)
			if action.Command != "" {
				fmt.Printf("         Run: %s\n", action.Command)
			}
		}
	}

	fmt.Println()
}

// =============================================================================
// DIAGNOSE EXPORT
// =============================================================================

func runDiagnoseExport(cmd *cobra.Command, args []string) {
	ensureErrorRegistry()

	term.StartSpinner("Exporting debug information...")

	// Build export bundle
	bundle := buildExportBundle()

	term.StopSpinner()

	// Determine output path
	outputPath := diagnoseExportOutput
	if outputPath == "" {
		outputPath = fmt.Sprintf("plandex-debug-%s.%s",
			time.Now().Format("20060102-150405"),
			diagnoseExportFormat)
	}

	// Export based on format
	var data []byte
	var err error

	switch diagnoseExportFormat {
	case "json":
		data, err = json.MarshalIndent(bundle, "", "  ")
	case "text":
		data = formatBundleAsText(bundle)
	default:
		term.OutputErrorAndExit("Unknown format: %s (use json or text)", diagnoseExportFormat)
	}

	if err != nil {
		term.OutputErrorAndExit("Failed to export: %v", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		term.OutputErrorAndExit("Failed to write file: %v", err)
	}

	// Report success
	fmt.Println()
	color.New(color.Bold, color.FgGreen).Println("✓ Debug bundle exported successfully")
	fmt.Println()
	fmt.Printf("  File:     %s\n", outputPath)
	fmt.Printf("  Size:     %s\n", formatSize(len(data)))
	fmt.Printf("  Format:   %s\n", diagnoseExportFormat)
	fmt.Println()
	fmt.Println("  Sensitive data has been automatically sanitized.")
	fmt.Println("  This file is safe to share when reporting issues.")
	fmt.Println()
}

type ExportBundle struct {
	ExportedAt     string                 `json:"exportedAt"`
	PlandexVersion string                 `json:"plandexVersion"`
	System         map[string]interface{} `json:"system"`
	Errors         []*shared.StoredError  `json:"errors"`
	DebugMode      map[string]interface{} `json:"debugMode"`
	Traces         []shared.TraceEntry    `json:"traces,omitempty"`
	Stats          shared.RegistryStats   `json:"stats"`
}

func buildExportBundle() *ExportBundle {
	// Get errors with sanitization
	errors := shared.ListErrors(shared.ErrorFilter{Limit: 50})

	// Sanitize errors
	sanitizedErrors := make([]*shared.StoredError, len(errors))
	for i, err := range errors {
		sanitized := shared.SanitizeError(err.ErrorReport, shared.SanitizeLevelStandard)
		sanitizedErrors[i] = &shared.StoredError{
			ErrorReport: sanitized,
			StoredAt:    err.StoredAt,
			SessionId:   err.SessionId,
			Resolved:    err.Resolved,
			ResolvedAt:  err.ResolvedAt,
			ResolvedBy:  err.ResolvedBy,
			RetryCount:  err.RetryCount,
		}
	}

	// Get traces if debug mode was enabled
	var traces []shared.TraceEntry
	if shared.IsDebugEnabled() {
		tracesData, _ := shared.ExportTraces(shared.SanitizeLevelStandard)
		json.Unmarshal(tracesData, &traces)
	}

	return &ExportBundle{
		ExportedAt:     time.Now().Format(time.RFC3339),
		PlandexVersion: version.Version,
		System:         shared.CaptureEnvironment(shared.SanitizeLevelStandard),
		Errors:         sanitizedErrors,
		DebugMode:      shared.GetDebugModeState(),
		Traces:         traces,
		Stats:          shared.GlobalErrorRegistry.Stats(),
	}
}

func formatBundleAsText(bundle *ExportBundle) []byte {
	var sb strings.Builder

	sb.WriteString("═══════════════════════════════════════════════════════════════════\n")
	sb.WriteString("                    PLANDEX DEBUG EXPORT\n")
	sb.WriteString("═══════════════════════════════════════════════════════════════════\n\n")

	sb.WriteString(fmt.Sprintf("Exported:  %s\n", bundle.ExportedAt))
	sb.WriteString(fmt.Sprintf("Version:   %s\n", bundle.PlandexVersion))
	sb.WriteString("\n")

	// System info
	sb.WriteString("▌ SYSTEM\n")
	sb.WriteString("├─────────────────────────────────────────────────────────────────\n")
	for k, v := range bundle.System {
		sb.WriteString(fmt.Sprintf("│ %s: %v\n", k, v))
	}
	sb.WriteString("\n")

	// Stats
	sb.WriteString("▌ ERROR STATISTICS\n")
	sb.WriteString("├─────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("│ Total:      %d\n", bundle.Stats.TotalErrors))
	sb.WriteString(fmt.Sprintf("│ Resolved:   %d\n", bundle.Stats.ResolvedErrors))
	sb.WriteString(fmt.Sprintf("│ Unresolved: %d\n", bundle.Stats.UnresolvedErrors))
	sb.WriteString("\n")

	// Errors
	sb.WriteString("▌ RECENT ERRORS\n")
	sb.WriteString("├─────────────────────────────────────────────────────────────────\n")
	for _, err := range bundle.Errors {
		sb.WriteString(fmt.Sprintf("│ [%s] %s: %s\n",
			err.StoredAt.Format("2006-01-02 15:04:05"),
			err.RootCause.Type,
			truncateString(err.RootCause.Message, 50)))
	}
	sb.WriteString("\n")

	return []byte(sb.String())
}

// =============================================================================
// DIAGNOSE STATUS
// =============================================================================

func runDiagnoseStatus(cmd *cobra.Command, args []string) {
	ensureErrorRegistry()

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                     SYSTEM DIAGNOSTICS                            ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Connection status
	printConnectionStatus()

	// Local state
	printLocalState()

	// Debug mode
	printDebugModeStatus()

	// Recent issues summary
	printRecentIssues()

	fmt.Println()
	term.PrintCmds("", "diagnose errors", "diagnose export")
}

func printConnectionStatus() {
	color.New(color.Bold).Println("▌ CONNECTION")
	fmt.Println("├─────────────────────────────────────────────────────────────────")

	// Check if authenticated
	if auth.Current != nil {
		fmt.Printf("│ Auth:       ✓ Authenticated\n")
		if auth.Current.Email != "" {
			fmt.Printf("│ User:       %s\n", auth.Current.Email)
		}
	} else {
		fmt.Println("│ Auth:       ✗ Not authenticated")
	}

	// API host
	host := os.Getenv("PLANDEX_API_HOST")
	if host == "" {
		host = "https://api.plandex.ai"
	}
	fmt.Printf("│ Server:     %s\n", host)

	fmt.Println()
}

func printLocalState() {
	color.New(color.Bold).Println("▌ LOCAL STATE")
	fmt.Println("├─────────────────────────────────────────────────────────────────")

	// Data directory
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".plandex")

	if _, err := os.Stat(dataDir); err == nil {
		size := getDirSize(dataDir)
		fmt.Printf("│ Data dir:   %s (%s)\n", dataDir, formatSize(size))
	} else {
		fmt.Printf("│ Data dir:   %s (not found)\n", dataDir)
	}

	// Log file
	logFile := filepath.Join(dataDir, "plandex.log")
	if info, err := os.Stat(logFile); err == nil {
		fmt.Printf("│ Log file:   %s (%s)\n", logFile, formatSize(int(info.Size())))
	} else {
		fmt.Println("│ Log file:   Not found")
	}

	// Error registry stats
	if shared.GlobalErrorRegistry != nil {
		stats := shared.GlobalErrorRegistry.Stats()
		fmt.Printf("│ Errors:     %d total (%d unresolved)\n",
			stats.TotalErrors, stats.UnresolvedErrors)
	}

	fmt.Println()
}

func printDebugModeStatus() {
	color.New(color.Bold).Println("▌ DEBUG MODE")
	fmt.Println("├─────────────────────────────────────────────────────────────────")

	state := shared.GetDebugModeState()
	if enabled, ok := state["enabled"].(bool); ok && enabled {
		fmt.Printf("│ Status:     ✓ ENABLED\n")
		if level, ok := state["level"].(string); ok {
			fmt.Printf("│ Level:      %s\n", level)
		}
		if traceCount, ok := state["traceCount"].(int); ok {
			fmt.Printf("│ Traces:     %d captured\n", traceCount)
		}
		if duration, ok := state["duration"].(string); ok {
			fmt.Printf("│ Duration:   %s\n", duration)
		}
	} else {
		fmt.Println("│ Status:     OFF")
	}

	fmt.Println()
}

func printRecentIssues() {
	color.New(color.Bold).Println("▌ RECENT ISSUES")
	fmt.Println("├─────────────────────────────────────────────────────────────────")

	// Get unresolved errors from last 24 hours
	since := time.Now().Add(-24 * time.Hour)
	unresolvedFilter := false
	filter := shared.ErrorFilter{
		Since:    &since,
		Resolved: &unresolvedFilter,
		Limit:    5,
	}

	errors := shared.ListErrors(filter)

	if len(errors) == 0 {
		fmt.Println("│ No recent issues")
	} else {
		for _, err := range errors {
			age := formatAge(time.Since(err.StoredAt))
			fmt.Printf("│ • %s (%s) - %s\n",
				err.RootCause.Type,
				truncateString(err.RootCause.Message, 30),
				age)
		}
	}

	fmt.Println()
}

// =============================================================================
// DIAGNOSE DEBUG-MODE
// =============================================================================

func runDiagnoseDebugMode(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		// Show current status
		state := shared.GetDebugModeState()
		if enabled, ok := state["enabled"].(bool); ok && enabled {
			fmt.Printf("Debug mode is currently ENABLED (level: %s)\n", state["level"])
			if traceFile, ok := state["traceFile"].(string); ok && traceFile != "" {
				fmt.Printf("Trace file: %s\n", traceFile)
			}
			fmt.Printf("Traces captured: %v\n", state["traceCount"])
		} else {
			fmt.Println("Debug mode is currently OFF")
		}
		fmt.Println()
		fmt.Println("Use 'plandex diagnose debug-mode on' to enable")
		fmt.Println("Use 'plandex diagnose debug-mode off' to disable")
		return
	}

	switch strings.ToLower(args[0]) {
	case "on", "enable", "1":
		level := shared.ParseDebugLevel(debugModeLevel)
		if err := shared.EnableDebugMode(level, debugModeTraceFile); err != nil {
			term.OutputErrorAndExit("Failed to enable debug mode: %v", err)
		}

		fmt.Printf("✓ Debug mode enabled (level: %s)\n", level.String())
		if debugModeTraceFile != "" {
			fmt.Printf("  Traces will be written to: %s\n", debugModeTraceFile)
		}
		fmt.Println()
		fmt.Println("Debug mode will capture detailed traces during operations.")
		fmt.Println("Use 'plandex diagnose export' to export captured traces.")

	case "off", "disable", "0":
		shared.DisableDebugMode()
		fmt.Println("✓ Debug mode disabled")

	default:
		term.OutputErrorAndExit("Unknown argument: %s (use 'on' or 'off')", args[0])
	}
}

// =============================================================================
// DIAGNOSE CLEAR
// =============================================================================

func runDiagnoseClear(cmd *cobra.Command, args []string) {
	ensureErrorRegistry()

	count := shared.GlobalErrorRegistry.Count()

	if count == 0 {
		fmt.Println("No errors to clear.")
		return
	}

	res, err := term.ConfirmYesNo(fmt.Sprintf("Clear %d stored errors?", count))
	if err != nil {
		term.OutputErrorAndExit("Error: %v", err)
	}

	if !res {
		fmt.Println("Cancelled.")
		return
	}

	shared.GlobalErrorRegistry.Clear()
	if err := shared.GlobalErrorRegistry.Persist(); err != nil {
		term.OutputErrorAndExit("Failed to persist: %v", err)
	}

	fmt.Printf("✓ Cleared %d errors\n", count)
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func ensureErrorRegistry() {
	if shared.GlobalErrorRegistry == nil {
		homeDir, _ := os.UserHomeDir()
		dataDir := filepath.Join(homeDir, ".plandex")
		sessionId := fmt.Sprintf("cli_%d", time.Now().UnixNano())

		if err := shared.InitGlobalRegistry(dataDir, sessionId); err != nil {
			// Non-fatal, just warn
			fmt.Fprintf(os.Stderr, "Warning: Could not load error registry: %v\n", err)
		}
	}
}

func centerString(s string, width int) string {
	if len(s) >= width {
		return s
	}
	leftPad := (width - len(s)) / 2
	rightPad := width - len(s) - leftPad
	return strings.Repeat(" ", leftPad) + s + strings.Repeat(" ", rightPad)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatSize(bytes int) string {
	if bytes < 1024 {
		return strconv.Itoa(bytes) + "B"
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	} else {
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	} else if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	} else {
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

func getDirSize(path string) int {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return int(size)
}

// getGoVersion returns the Go version
func getGoVersion() string {
	return runtime.Version()
}
