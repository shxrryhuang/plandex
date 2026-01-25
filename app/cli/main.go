package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"plandex-cli/api"
	"plandex-cli/auth"
	"plandex-cli/cmd"
	"plandex-cli/fs"
	"plandex-cli/lib"
	"plandex-cli/plan_exec"
	"plandex-cli/term"
	"plandex-cli/types"
	"plandex-cli/ui"
	"time"

	shared "plandex-shared"

	"gopkg.in/natefinch/lumberjack.v2"
)

func init() {
	// inter-package dependency injections to avoid circular imports
	auth.SetApiClient(api.Client)

	auth.SetOpenUnauthenticatedCloudURLFn(ui.OpenUnauthenticatedCloudURL)
	auth.SetOpenAuthenticatedURLFn(ui.OpenAuthenticatedURL)

	term.SetOpenAuthenticatedURLFn(ui.OpenAuthenticatedURL)
	term.SetOpenUnauthenticatedCloudURLFn(ui.OpenUnauthenticatedCloudURL)
	term.SetConvertTrialFn(auth.ConvertTrial)

	plan_exec.SetPromptSyncModelsIfNeeded(lib.PromptSyncModelsIfNeeded)

	lib.SetBuildPlanInlineFn(func(autoConfirm bool, maybeContexts []*shared.Context) (bool, error) {
		authVars := lib.MustVerifyAuthVars(auth.Current.IntegratedModelsMode)
		return plan_exec.Build(plan_exec.ExecParams{
			CurrentPlanId: lib.CurrentPlanId,
			CurrentBranch: lib.CurrentBranch,
			AuthVars:      authVars,
			CheckOutdatedContext: func(maybeContexts []*shared.Context, projectPaths *types.ProjectPaths) (bool, bool, error) {
				return lib.CheckOutdatedContextWithOutput(true, autoConfirm, maybeContexts, projectPaths)
			},
		}, types.BuildFlags{})
	})

	// set up a rotating file logger
	logger := &lumberjack.Logger{
		Filename:   filepath.Join(fs.HomePlandexDir, "plandex.log"),
		MaxSize:    10,   // megabytes before rotation
		MaxBackups: 3,    // number of backups to keep
		MaxAge:     28,   // days to keep old logs
		Compress:   true, // compress rotated files
	}

	// Set the output of the logger
	log.SetOutput(logger)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	// log.Println("Starting Plandex - logging initialized")

	// Initialize global error registry for diagnostics
	sessionId := fmt.Sprintf("cli_%d", time.Now().UnixNano())
	if err := shared.InitGlobalRegistry(fs.HomePlandexDir, sessionId); err != nil {
		log.Printf("Warning: Failed to initialize error registry: %v", err)
	}

	// Check for debug mode environment variable
	if os.Getenv("PLANDEX_DEBUG") == "1" || os.Getenv("PLANDEX_DEBUG") == "true" {
		level := shared.ParseDebugLevel(os.Getenv("PLANDEX_DEBUG_LEVEL"))
		traceFile := os.Getenv("PLANDEX_TRACE_FILE")
		if traceFile == "" {
			traceFile = filepath.Join(fs.HomePlandexDir, "debug_traces.log")
		}
		if err := shared.EnableDebugMode(level, traceFile); err != nil {
			log.Printf("Warning: Failed to enable debug mode: %v", err)
		}
	}
}

func main() {
	// Manually check for help flags at the root level
	if len(os.Args) == 2 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		// Display your custom help here
		term.PrintCustomHelp(true)
		os.Exit(0)
	}

	var firstArg string
	if len(os.Args) > 1 {
		firstArg = os.Args[1]
	}

	if firstArg != "version" && firstArg != "browser" && firstArg != "help" && firstArg != "h" {
		checkForUpgrade()
	}

	cmd.Execute()
}
