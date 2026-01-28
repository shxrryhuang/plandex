// Pipeline runner - standalone entry point for testing progress reporting features
// Run with: go run ./progress/pipeline/cmd/
//
// This runs all progress reporting scenarios WITHOUT modifying any original code.
// The original stream_tui remains untouched.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"plandex-cli/progress/pipeline"
)

func main() {
	// Parse flags
	scenario := flag.String("scenario", "all", "Scenario to run: normal, slow_llm, stalled, failure, user_input, large_task, quick_task, mixed, all")
	tty := flag.Bool("tty", true, "Enable TTY mode (animated output)")
	logFormat := flag.Bool("log", false, "Use log format (non-TTY output)")
	flag.Parse()

	if *logFormat {
		*tty = false
	}

	// Force color output
	color.NoColor = false
	os.Setenv("TERM", "xterm-256color")

	fmt.Println(strings.Repeat("=", 70))
	fmt.Println(color.New(color.Bold, color.FgCyan).Sprint("PLANDEX PROGRESS PIPELINE - STANDALONE RUNNER"))
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println()
	fmt.Println(color.HiBlackString("This pipeline runs independently of the original stream_tui code."))
	fmt.Println(color.HiBlackString("No original code is modified or affected."))
	fmt.Println()

	scenarios := []pipeline.Scenario{
		pipeline.ScenarioNormal,
		pipeline.ScenarioSlowLLM,
		pipeline.ScenarioStalled,
		pipeline.ScenarioFailure,
		pipeline.ScenarioUserInput,
		pipeline.ScenarioLargeTask,
		pipeline.ScenarioQuickTask,
		pipeline.ScenarioMixed,
	}

	// Filter to specific scenario if requested
	if *scenario != "all" {
		found := false
		for _, s := range scenarios {
			if string(s) == *scenario {
				scenarios = []pipeline.Scenario{s}
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("Unknown scenario: %s\n", *scenario)
			fmt.Println("Available scenarios: normal, slow_llm, stalled, failure, user_input, large_task, quick_task, mixed, all")
			os.Exit(1)
		}
	}

	// Run each scenario
	for i, s := range scenarios {
		if i > 0 {
			fmt.Println()
			fmt.Println(strings.Repeat("-", 70))
		}

		runScenario(s, *tty)
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println(color.New(color.Bold).Sprint("Pipeline Summary"))
	fmt.Println(strings.Repeat("-", 70))
	fmt.Println()
	fmt.Println("State Legend:")
	fmt.Println("  " + color.CyanString("⠹") + " Running   (best-effort signal)")
	fmt.Println("  " + color.GreenString("●") + " Completed (guaranteed state)")
	fmt.Println("  " + color.RedString("✗") + " Failed    (guaranteed state)")
	fmt.Println("  " + color.YellowString("◔") + " Waiting   (best-effort signal)")
	fmt.Println("  " + color.New(color.FgRed, color.Bold).Sprint("⚠") + " Stalled   (needs attention)")
	fmt.Println("  " + color.HiBlackString("○") + " Pending   (not started)")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  go run ./progress/pipeline/cmd/ -scenario=normal    # Run specific scenario")
	fmt.Println("  go run ./progress/pipeline/cmd/ -log                # Use log format (CI/non-TTY)")
	fmt.Println("  go run ./progress/pipeline/cmd/ -scenario=all       # Run all scenarios")
	fmt.Println(strings.Repeat("=", 70))
}

func runScenario(scenario pipeline.Scenario, isTTY bool) {
	title := getScenarioTitle(scenario)
	fmt.Printf("\n%s %s\n",
		color.New(color.Bold, color.FgCyan).Sprint("▶"),
		color.New(color.Bold).Sprintf("Scenario: %s", title))
	fmt.Println(strings.Repeat("-", 40))

	// Create pipeline
	config := pipeline.DefaultConfig()
	config.IsTTY = isTTY
	config.StallThreshold = 2 * time.Second // 2 seconds for demo (faster stall detection)

	p := pipeline.New(config)

	// Create runner
	runnerConfig := pipeline.DefaultRunnerConfig()
	runnerConfig.IsTTY = isTTY

	runner := pipeline.NewRunner(p, runnerConfig)

	// Run scenario
	if err := runner.Run(scenario); err != nil {
		fmt.Printf("Scenario error: %v\n", err)
	}
}

func getScenarioTitle(s pipeline.Scenario) string {
	switch s {
	case pipeline.ScenarioNormal:
		return "Normal Execution"
	case pipeline.ScenarioSlowLLM:
		return "Slow LLM Response"
	case pipeline.ScenarioStalled:
		return "Stalled Operation"
	case pipeline.ScenarioFailure:
		return "Build Failure"
	case pipeline.ScenarioUserInput:
		return "Waiting for User Input"
	case pipeline.ScenarioLargeTask:
		return "Large Task (Many Files)"
	case pipeline.ScenarioQuickTask:
		return "Quick Task"
	case pipeline.ScenarioMixed:
		return "Mixed Results"
	default:
		return string(s)
	}
}
