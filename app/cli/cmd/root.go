package cmd

import (
	"fmt"
	"os"
	"plandex-cli/fs"
	"plandex-cli/term"
	"path/filepath"
	"strings"

	shared "plandex-shared"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var helpShowAll bool
var debugMode bool
var debugLevel string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use: `plandex [command] [flags]`,
	// Short: "Plandex: iterative development with AI",
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Enable debug mode if --debug flag is set
		if debugMode && !shared.IsDebugEnabled() {
			level := shared.ParseDebugLevel(debugLevel)
			traceFile := filepath.Join(fs.HomePlandexDir, "debug_traces.log")
			if err := shared.EnableDebugMode(level, traceFile); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to enable debug mode: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "Debug mode enabled (level: %s)\n", level.String())
			}
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		run(cmd, args)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// if no arguments were passed, start the repl
	if len(os.Args) == 1 ||
		(len(os.Args) == 2 && strings.HasPrefix(os.Args[1], "--") && os.Args[1] != "--help") ||
		(len(os.Args) == 3 && strings.HasPrefix(os.Args[1], "--") && os.Args[1] != "--help" && strings.HasPrefix(os.Args[2], "--") && os.Args[2] != "--help") {

		// Instead of directly calling replCmd.Run, parse the flags first
		replCmd.ParseFlags(os.Args[1:])
		replCmd.Run(replCmd, []string{})
		return
	}

	if err := RootCmd.Execute(); err != nil {
		// term.OutputErrorAndExit("Error executing root command: %v", err)
		// log.Fatalf("Error executing root command: %v", err)

		// output the error message to stderr
		term.OutputSimpleError("Error: %v", err)

		fmt.Println()

		color.New(color.Bold, color.BgGreen, color.FgHiWhite).Println(" Usage ")
		color.New(color.Bold).Println("  plandex [command] [flags]")
		color.New(color.Bold).Println("  pdx [command] [flags]")
		fmt.Println()

		color.New(color.Bold, color.BgGreen, color.FgHiWhite).Println(" Help ")
		color.New(color.Bold).Println("  plandex help # show basic usage")
		color.New(color.Bold).Println("  plandex help --all # show all commands")
		color.New(color.Bold).Println("  plandex [command] --help")
		fmt.Println()

		color.New(color.Bold, color.BgGreen, color.FgHiWhite).Println(" Common Commands ")
		color.New(color.Bold).Println("  plandex new # create a new plan")
		color.New(color.Bold).Println("  plandex tell # tell the plan what to do")
		color.New(color.Bold).Println("  plandex continue # continue the current plan")
		color.New(color.Bold).Println("  plandex settings # show plan settings")
		color.New(color.Bold).Println("  plandex set # update plan settings")
		fmt.Println()

		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {

}

func init() {
	var helpCmd = &cobra.Command{
		Use:     "help",
		Aliases: []string{"h"},
		Short:   "Display help for Plandex",
		Long:    `Display help for Plandex.`,
		Run: func(cmd *cobra.Command, args []string) {
			term.PrintCustomHelp(helpShowAll)
		},
	}

	RootCmd.AddCommand(helpCmd)
	RootCmd.AddCommand(connectClaudeCmd)
	RootCmd.AddCommand(disconnectClaudeCmd)

	// add an --all/-a flag
	helpCmd.Flags().BoolVarP(&helpShowAll, "all", "a", false, "Show all commands")

	// Add global debug flags
	RootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "Enable debug mode for detailed tracing")
	RootCmd.PersistentFlags().StringVar(&debugLevel, "debug-level", "debug", "Debug verbosity level (info, debug, trace)")
}
