// Demo program to show progress rendering output
// Run with: go run ./progress/demo/
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
)

func main() {
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("PLANDEX PROGRESS REPORTING DEMO")
	fmt.Println(strings.Repeat("=", 70))

	// Demo 1: Normal Execution
	fmt.Println("\n" + color.New(color.Bold, color.FgCyan).Sprint("1. NORMAL EXECUTION"))
	fmt.Println(strings.Repeat("-", 70))
	renderNormalExecution()

	// Demo 2: Slow LLM Call
	fmt.Println("\n" + color.New(color.Bold, color.FgCyan).Sprint("2. SLOW LLM CALL"))
	fmt.Println(strings.Repeat("-", 70))
	renderSlowLLMCall()

	// Demo 3: Stalled Operation
	fmt.Println("\n" + color.New(color.Bold, color.FgCyan).Sprint("3. STALLED OPERATION (needs attention)"))
	fmt.Println(strings.Repeat("-", 70))
	renderStalledOperation()

	// Demo 4: Failure
	fmt.Println("\n" + color.New(color.Bold, color.FgCyan).Sprint("4. FAILURE SCENARIO"))
	fmt.Println(strings.Repeat("-", 70))
	renderFailure()

	// Demo 5: User Input
	fmt.Println("\n" + color.New(color.Bold, color.FgCyan).Sprint("5. WAITING FOR USER INPUT"))
	fmt.Println(strings.Repeat("-", 70))
	renderUserInput()

	// Demo 6: Completed
	fmt.Println("\n" + color.New(color.Bold, color.FgCyan).Sprint("6. COMPLETED SUCCESSFULLY"))
	fmt.Println(strings.Repeat("-", 70))
	renderCompleted()

	// Demo 7: Non-TTY Log Format
	fmt.Println("\n" + color.New(color.Bold, color.FgCyan).Sprint("7. NON-TTY LOG FORMAT"))
	fmt.Println(strings.Repeat("-", 70))
	renderNonTTYLogs()

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("State Legend:")
	fmt.Println("  " + color.CyanString("◐/⠹") + " Running   (best-effort signal)")
	fmt.Println("  " + color.GreenString("●") + "   Completed (guaranteed state)")
	fmt.Println("  " + color.RedString("✗") + "   Failed    (guaranteed state)")
	fmt.Println("  " + color.YellowString("◔") + "   Waiting   (best-effort signal)")
	fmt.Println("  " + color.New(color.FgRed, color.Bold).Sprint("⚠") + "   Stalled   (needs attention)")
	fmt.Println("  " + color.HiBlackString("○") + "   Pending   (not started)")
	fmt.Println(strings.Repeat("=", 70))
}

func renderProgressBar(completed, total, width int) string {
	progress := float64(completed) / float64(total)
	filled := int(progress * float64(width))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("[%s] %3.0f%%", bar, progress*100)
}

func renderNormalExecution() {
	fmt.Println(color.New(color.Bold).Sprint("🏗 Building files") + color.HiBlackString(" [1m12s]"))
	fmt.Println("  " + renderProgressBar(3, 5, 40))
	fmt.Println(color.CyanString("⠹") + " 📄 Building file " + color.HiBlackString("(src/api/handlers.go)") + color.HiBlackString(" 847🪙"))
	fmt.Println(color.GreenString("●") + " 📄 Building file " + color.HiBlackString("(src/models/user.go)") + color.HiBlackString(" 15s"))
	fmt.Println(color.GreenString("●") + " 📄 Building file " + color.HiBlackString("(src/config/config.go)") + color.HiBlackString(" 8s"))
	fmt.Println(color.HiBlackString("(s)top • (b)ackground • (j/k) scroll • (p)rogress"))
}

func renderSlowLLMCall() {
	fmt.Println(color.New(color.Bold).Sprint("🧠 Planning task") + color.HiBlackString(" [1m45s]"))
	fmt.Println("  " + renderProgressBar(1, 2, 40))
	fmt.Println(color.CyanString("⠧") + " 🤖 Calling LLM " + color.HiBlackString("(gpt-4-turbo)") + color.HiBlackString(" [1m30s]"))
	fmt.Println(color.GreenString("●") + " 📚 Loading context " + color.HiBlackString("(12 files)") + color.HiBlackString(" 10s"))
	fmt.Println(color.HiBlackString("💡 LLM is processing. Large tasks may take time."))
	fmt.Println(color.HiBlackString("(s)top • (b)ackground • (j/k) scroll • (p)rogress"))
}

func renderStalledOperation() {
	fmt.Println(color.New(color.Bold).Sprint("🧠 Planning task") + color.HiBlackString(" [2m30s]"))
	fmt.Println("  " + renderProgressBar(1, 2, 40))
	fmt.Println(color.New(color.FgRed, color.Bold).Sprint("⚠") + " 🤖 Calling LLM " + color.HiBlackString("(gpt-4-turbo)") + color.HiBlackString(" [2m15s]"))
	fmt.Println(color.GreenString("●") + " 📚 Loading context " + color.HiBlackString("(12 files)") + color.HiBlackString(" 10s"))
	fmt.Println(color.New(color.FgRed, color.Bold).Sprint("⚠ Operation may be stalled"))
	fmt.Println(color.HiBlackString("💡 Operation appears stalled. Consider canceling (s) if no progress."))
	fmt.Println(color.HiBlackString("(s)top • (b)ackground • (j/k) scroll • (p)rogress"))
}

func renderFailure() {
	fmt.Println(color.New(color.Bold).Sprint("🏗 Building files") + color.HiBlackString(" [32s]"))
	fmt.Println("  " + renderProgressBar(2, 4, 40))
	fmt.Println(color.RedString("✗") + " 📄 Building file " + color.HiBlackString("(src/broken.go)") + color.RedString(" [syntax error at line 42]"))
	fmt.Println(color.GreenString("●") + " 📄 Building file " + color.HiBlackString("(src/api.go)") + color.HiBlackString(" 12s"))
	fmt.Println(color.GreenString("●") + " 📄 Building file " + color.HiBlackString("(src/models.go)") + color.HiBlackString(" 8s"))
	fmt.Println(color.YellowString("⚠ Build failed: 1 error, 2 completed, 1 remaining"))
	fmt.Println(color.HiBlackString("(s)top • (r)etry • view (l)ogs"))
}

func renderUserInput() {
	fmt.Println(color.New(color.Bold).Sprint("🏗 Building files") + color.HiBlackString(" [45s]"))
	fmt.Println("  " + renderProgressBar(2, 5, 40))
	fmt.Println(color.YellowString("◔") + " ⌨ Waiting for input " + color.HiBlackString("(missing file: src/config.json)"))
	fmt.Println(color.GreenString("●") + " 📄 Building file " + color.HiBlackString("(src/api.go)") + color.HiBlackString(" 12s"))
	fmt.Println(color.GreenString("●") + " 📄 Building file " + color.HiBlackString("(src/models.go)") + color.HiBlackString(" 8s"))
	fmt.Println(color.HiBlackString("💡 Waiting for your input."))
	fmt.Println(color.HiBlackString("(s)top • (b)ackground • (j/k) scroll • (p)rogress"))
}

func renderCompleted() {
	fmt.Println(color.New(color.Bold).Sprint("✅ Completed") + color.HiBlackString(" [2m34s]"))
	fmt.Println("  " + renderProgressBar(4, 4, 40))
	fmt.Println(color.GreenString("●") + " 📄 Writing file " + color.HiBlackString("(src/api/handlers.go)") + color.HiBlackString(" 2s"))
	fmt.Println(color.GreenString("●") + " 🔍 Validating " + color.HiBlackString("(syntax check)") + color.HiBlackString(" 3s"))
	fmt.Println(color.GreenString("●") + " 📦 Applied 4 files")
}

func renderNonTTYLogs() {
	now := time.Now()
	fmt.Println(color.HiBlackString("Structured log output for CI/piped environments:"))
	fmt.Println()
	fmt.Printf("[%s] [%s] %s > %s %s\n",
		now.Add(-30*time.Second).Format("15:04:05"),
		color.CyanString("RUN "),
		"planning",
		"🤖",
		"Calling LLM (gpt-4-turbo)",
	)
	fmt.Printf("[%s] [%s] %s > %s %s %s\n",
		now.Format("15:04:05"),
		color.GreenString("DONE"),
		"planning",
		"🤖",
		"Calling LLM (gpt-4-turbo)",
		color.HiBlackString("[30s]"),
	)
	fmt.Printf("[%s] [%s] %s > %s %s\n",
		now.Format("15:04:05"),
		color.CyanString("RUN "),
		"building",
		"📄",
		"Building file (src/api.go)",
	)
	fmt.Printf("[%s] [%s] %s > %s %s %s\n",
		now.Add(3*time.Second).Format("15:04:05"),
		color.GreenString("DONE"),
		"building",
		"📄",
		"Building file (src/api.go)",
		color.HiBlackString("[3s]"),
	)
	fmt.Printf("[%s] [%s] %s > %s %s %s\n",
		now.Add(10*time.Second).Format("15:04:05"),
		color.RedString("FAIL"),
		"building",
		"📄",
		"Building file (src/broken.go)",
		color.RedString("ERROR: syntax error"),
	)
	fmt.Println()
	fmt.Println(color.HiBlackString("Format: [TIME] [STATE] PHASE > ICON LABEL (DETAIL) [DURATION]"))
	fmt.Println(color.HiBlackString("States: RUN (running), DONE (completed), FAIL (failed), WAIT (waiting), STAL (stalled)"))
}

func init() {
	// Force color output
	color.NoColor = false
	os.Setenv("TERM", "xterm-256color")
}
