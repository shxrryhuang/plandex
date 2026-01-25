package cmd

import (
	"fmt"
	"os"
	"plandex-cli/api"
	"plandex-cli/auth"
	"plandex-cli/lib"
	"plandex-cli/term"

	shared "plandex-shared"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var doctorFix bool
var doctorVerbose bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose concurrency issues and stale locks",
	Long: `Check for concurrency issues, stale locks, and other system health problems.

This command helps identify and fix issues that may cause operations to hang or fail:
- Stale locks from crashed operations
- Long-running operations blocking other work
- Database connectivity issues
- High memory usage

Use --fix to automatically clean up stale locks.
Use --verbose for detailed diagnostic information.`,
	Run: doctor,
}

func init() {
	RootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Automatically fix issues (clean up stale locks)")
	doctorCmd.Flags().BoolVarP(&doctorVerbose, "verbose", "v", false, "Show detailed diagnostic information")
}

func doctor(cmd *cobra.Command, args []string) {
	auth.MustResolveAuthWithOrg()

	fmt.Println("Checking system health...")
	fmt.Println()

	term.StartSpinner("")
	res, apiErr := api.Client.Doctor(shared.DoctorRequest{
		Fix: doctorFix,
	})
	term.StopSpinner()

	if apiErr != nil {
		term.OutputErrorAndExit("Error running diagnostics: %v", apiErr.Msg)
		return
	}

	// Display health checks
	fmt.Println(color.New(color.Bold).Sprint("Health Checks"))
	fmt.Println()

	for _, check := range res.Checks {
		var statusIcon string
		var statusColor *color.Color

		switch check.Status {
		case shared.CheckStatusOK:
			statusIcon = "\u2713" // checkmark
			statusColor = color.New(color.FgGreen)
		case shared.CheckStatusWarning:
			statusIcon = "\u26a0" // warning
			statusColor = color.New(color.FgYellow)
		case shared.CheckStatusError:
			statusIcon = "\u2717" // X
			statusColor = color.New(color.FgRed)
		}

		fmt.Printf("%s %s: %s\n",
			statusColor.Sprint(statusIcon),
			check.Name,
			check.Message,
		)
	}
	fmt.Println()

	// Display stale locks
	if len(res.StaleLocks) > 0 {
		fmt.Println(color.New(color.FgYellow, color.Bold).Sprint("\u26a0 Stale Locks Found"))
		fmt.Println()

		table := tablewriter.NewWriter(os.Stdout)
		table.SetAutoWrapText(false)
		table.SetHeader([]string{"Plan", "Scope", "Branch", "Age"})

		for _, lock := range res.StaleLocks {
			branch := lock.Branch
			if branch == "" {
				branch = "(root)"
			}
			table.Append([]string{
				lock.PlanName,
				lock.Scope,
				branch,
				lock.Age,
			})
		}
		table.Render()
		fmt.Println()

		if !doctorFix {
			fmt.Println("These locks appear to be from crashed operations.")
			fmt.Println()
			fmt.Println("Options:")
			fmt.Println("  1. Clean up stale locks: " + color.New(color.FgCyan).Sprint("plandex doctor --fix"))
			fmt.Println("  2. View details: " + color.New(color.FgCyan).Sprint("plandex doctor --verbose"))
			fmt.Println("  3. Wait for locks to expire automatically (60 seconds from last heartbeat)")
			fmt.Println()
		}
	}

	// Display active locks
	if len(res.ActiveLocks) > 0 && doctorVerbose {
		fmt.Println(color.New(color.Bold).Sprint("Active Locks"))
		fmt.Println()

		table := tablewriter.NewWriter(os.Stdout)
		table.SetAutoWrapText(false)
		table.SetHeader([]string{"Plan", "Scope", "Branch", "Started"})

		for _, lock := range res.ActiveLocks {
			branch := lock.Branch
			if branch == "" {
				branch = "(root)"
			}
			table.Append([]string{
				lock.PlanName,
				lock.Scope,
				branch,
				lock.CreatedAt.Format("15:04:05"),
			})
		}
		table.Render()
		fmt.Println()
	}

	// Display fixed issues
	if len(res.FixedIssues) > 0 {
		fmt.Println(color.New(color.FgGreen, color.Bold).Sprint("\u2713 Issues Fixed"))
		fmt.Println()
		for _, fixed := range res.FixedIssues {
			fmt.Printf("  %s %s\n", color.New(color.FgGreen).Sprint("\u2713"), fixed)
		}
		fmt.Println()
	}

	// Display issues
	if len(res.Issues) > 0 {
		hasErrors := false
		for _, issue := range res.Issues {
			if issue.Severity == shared.SeverityError || issue.Severity == shared.SeverityCritical {
				hasErrors = true
				break
			}
		}

		if hasErrors {
			fmt.Println(color.New(color.FgRed, color.Bold).Sprint("\u2717 Issues Detected"))
		} else {
			fmt.Println(color.New(color.FgYellow, color.Bold).Sprint("\u26a0 Warnings"))
		}
		fmt.Println()

		for _, issue := range res.Issues {
			var severityColor *color.Color
			switch issue.Severity {
			case shared.SeverityInfo:
				severityColor = color.New(color.FgBlue)
			case shared.SeverityWarning:
				severityColor = color.New(color.FgYellow)
			case shared.SeverityError, shared.SeverityCritical:
				severityColor = color.New(color.FgRed)
			}

			fmt.Printf("  %s %s\n", severityColor.Sprint("\u2022"), issue.Description)
			if doctorVerbose && issue.Suggestion != "" {
				fmt.Printf("    %s %s\n", color.New(color.Faint).Sprint("\u2192"), issue.Suggestion)
			}
		}
		fmt.Println()
	}

	// Display server metrics (verbose only)
	if doctorVerbose && res.ServerMetrics != nil {
		fmt.Println(color.New(color.Bold).Sprint("Server Metrics"))
		fmt.Println()
		fmt.Printf("  Goroutines: %d\n", res.ServerMetrics.GoroutineCount)
		fmt.Printf("  Memory: %dMB\n", res.ServerMetrics.MemoryUsageMB)
		fmt.Println()
	}

	// Summary
	if res.Healthy && len(res.Issues) == 0 && len(res.StaleLocks) == 0 {
		fmt.Println(color.New(color.FgGreen, color.Bold).Sprint("\u2713 System is healthy"))
		fmt.Println()
	}

	// Suggest next steps
	if lib.CurrentPlanId != "" && len(res.StaleLocks) == 0 && len(res.ActiveLocks) == 0 {
		fmt.Println("No concurrency issues detected for the current plan.")
		term.PrintCmds("", "tell", "build", "apply")
	}
}
