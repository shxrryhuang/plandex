package lib

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"plandex-cli/api"
	"plandex-cli/auth"
	"plandex-cli/fs"
	"plandex-cli/term"
	"plandex-cli/types"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	shared "plandex-shared"

	"github.com/fatih/color"
)

type ApplyPlanParams struct {
	PlanId      string
	Branch      string
	ApplyFlags  types.ApplyFlags
	TellFlags   types.TellFlags
	OnExecFail  types.OnApplyExecFailFn
	ExecCommand string
}

func MustApplyPlan(
	params ApplyPlanParams,
) {
	MustApplyPlanAttempt(params, 0)
}

func MustApplyPlanAttempt(
	params ApplyPlanParams,
	attempt int,
) {
	log.Println("Applying plan")

	applyFlags := params.ApplyFlags
	planId := params.PlanId
	branch := params.Branch
	onExecFail := params.OnExecFail

	autoConfirm := applyFlags.AutoConfirm
	autoCommit := applyFlags.AutoCommit
	noCommit := applyFlags.NoCommit
	noExec := applyFlags.NoExec

	term.StartSpinner("")

	err := PromptSyncModelsIfNeeded()
	if err != nil {
		term.OutputErrorAndExit("Error syncing models: %v", err)
	}

	term.StartSpinner("")

	currentPlanState, apiErr := api.Client.GetCurrentPlanState(planId, branch)

	if apiErr != nil {
		term.StopSpinner()
		term.OutputErrorAndExit("Error getting current plan state: %v", apiErr)
	}

	if currentPlanState.HasPendingBuilds() {
		plansRunningRes, apiErr := api.Client.ListPlansRunning([]string{CurrentProjectId}, false)

		if apiErr != nil {
			term.StopSpinner()
			term.OutputErrorAndExit("Error getting running plans: %v", apiErr)
		}

		for _, b := range plansRunningRes.Branches {
			if b.PlanId == planId && b.Name == branch {
				fmt.Println("This plan is currently active. Please wait for it to finish before applying.")
				fmt.Println()
				term.PrintCmds("", "ps", "connect")
				return
			}
		}

		term.StopSpinner()

		fmt.Println("This plan has changes that need to be built before applying")
		fmt.Println()

		shouldBuild, err := term.ConfirmYesNo("Build changes now?")

		if err != nil {
			term.OutputErrorAndExit("failed to get confirmation user input: %s", err)
		}

		if !shouldBuild {
			fmt.Println("Apply plan canceled")
			os.Exit(0)
		}

		_, err = buildPlanInlineFn(autoConfirm, nil)

		if err != nil {
			term.OutputErrorAndExit("failed to build plan: %v", err)
		}
	}

	paths, err := fs.GetProjectPaths(fs.ProjectRoot)

	if err != nil {
		term.OutputErrorAndExit("error getting project paths: %v", err)
	}

	anyOutdated, didUpdate, err := CheckOutdatedContextWithOutput(true, autoConfirm, nil, paths)

	if err != nil {
		term.OutputErrorAndExit("error checking outdated context: %v", err)
	}

	if anyOutdated && !didUpdate {
		term.StopSpinner()
		fmt.Println("Apply plan canceled")
		os.Exit(0)
	}

	term.ResumeSpinner()

	currentPlanFiles := currentPlanState.CurrentPlanFiles
	isRepo := fs.ProjectRootIsGitRepo()

	toApply := currentPlanFiles.Files
	toRemove := currentPlanFiles.Removed
	hasExec := currentPlanFiles.Files["_apply.sh"] != ""

	log.Printf("Files to apply: %d, Has exec script: %v", len(toApply), hasExec)

	if len(toApply) == 0 && !hasExec {
		term.StopSpinner()
		fmt.Println("🤷‍♂️ No changes to apply")
		return
	}

	hasFileChanges := !hasExec || len(toApply) > 1

	var toRollback *types.ApplyRollbackPlan
	var updatedFiles []string

	onErr := func(errMsg string, errArgs ...interface{}) {
		term.StopSpinner()
		// ApplyFiles now rolls back internally on failure via FileTransaction;
		// no manual rollback is needed here.
		term.OutputErrorAndExit(errMsg, errArgs...)
	}

	onGitErr := func(errMsg, unformattedErrMsg string) {
		term.StopSpinner()
		fmt.Println()
		term.OutputSimpleError(errMsg, unformattedErrMsg)
	}

	log.Println("Has file changes:", hasFileChanges)

	if hasFileChanges {
		if !autoConfirm {
			log.Println("Asking user to confirm applying changes")

			term.StopSpinner()
			numToApply := len(toApply)
			suffix := ""
			if numToApply > 1 {
				suffix = "s"
			}
			shouldContinue, err := term.ConfirmYesNo("Apply changes to %d file%s?", numToApply, suffix)

			if err != nil {
				term.OutputErrorAndExit("failed to get confirmation user input: %s", err)
			}

			if !shouldContinue {
				os.Exit(0)
			}
			term.ResumeSpinner()
		}

		log.Println("Applying plan files")

		term.StopSpinner()
		if hasExec {
			color.New(term.ColorHiCyan).Println("📋 Staging changes (capturing snapshots)…")
		} else {
			color.New(term.ColorHiCyan).Println("📋 Applying changes atomically…")
		}
		term.ResumeSpinner()

		updatedFiles, toRollback, err = ApplyFiles(toApply, toRemove, paths)

		if err != nil {
			// ApplyFiles rolled back internally; surface the error.
			onErr("failed to apply files: %s", err)
		}

		term.StopSpinner()
		if hasExec {
			color.New(term.ColorHiCyan).Println("✔ Changes staged (all-or-nothing applied to disk)")
		}
		term.ResumeSpinner()

		log.Println("Applying plan files complete")
	}

	onExecSuccess := func() {
		term.StartSpinner("")
		commitSummary, err := apiApplyPlan(planId, branch)

		if err != nil {
			onErr("apply plan server error: %s", err)
		}

		if len(updatedFiles) == 0 {
			term.StopSpinner()
			fmt.Println("✅ Applied changes, but no files were updated")
		} else {
			appliedMsgFn := func() {
				suffix := ""
				if len(updatedFiles) > 1 {
					suffix = "s"
				}
				fmt.Printf("✅ Applied changes, %d file%s updated\n", len(updatedFiles), suffix)
				for _, file := range updatedFiles {
					fmt.Println(" • 📄 " + file)
				}
			}

			if isRepo && !noCommit {
				term.StopSpinner()
				gitErr := commitApplied(autoCommit, commitSummary, updatedFiles, currentPlanState)
				appliedMsgFn()
				if gitErr != nil {
					onGitErr("Failed to commit changes:", gitErr.Error())
				}
			} else {
				term.StopSpinner()
				appliedMsgFn()
			}
		}
	}

	if _, ok := toApply["_apply.sh"]; ok && !noExec {
		handleApplyScript(params, toApply, onErr, toRollback, onExecFail, attempt, onExecSuccess)
	} else {
		onExecSuccess()
	}
}

func handleApplyScript(
	params ApplyPlanParams,
	toApply map[string]string,
	onErr types.OnErrFn,
	toRollback *types.ApplyRollbackPlan,
	onExecFail types.OnApplyExecFailFn,
	attempt int,
	onSuccess func(),
) {
	log.Println("Handling apply script")

	term.StopSpinner()

	color.New(term.ColorHiCyan, color.Bold).Println("🚀 Commands to execute 👇")

	var content string
	if params.ExecCommand != "" {
		content = params.ExecCommand
	} else {
		content = toApply["_apply.sh"]
	}

	md, err := term.GetMarkdown("```bash\n" + content + "\n```")

	if err != nil {
		onErr("failed to get markdown representation: %s", err)
	}

	fmt.Println(strings.TrimSpace(md))

	log.Println("Asking user to confirm executing apply script")

	var confirmed bool
	if params.ApplyFlags.AutoExec {
		confirmed = true
	} else {
		confirmed, err = term.ConfirmYesNo("Execute now?")
		if err != nil {
			onErr("failed to get confirmation user input: %s", err)
		}
	}

	if confirmed {
		log.Println("Executing apply script")
		execApplyScript(params, toApply, onErr, toRollback, onExecFail, attempt, onSuccess)
	} else {
		if toRollback != nil && toRollback.HasChanges() {
			res, err := term.SelectFromList("Skipping execution. Apply file changes or roll back?", []string{string(types.ApplyRollbackOptionKeep), string(types.ApplyRollbackOptionRollback)})

			if err != nil {
				onErr("failed to get rollback confirmation user input: %s", err)
			}

			if res == string(types.ApplyRollbackOptionRollback) {
				if rbErr := Rollback(toRollback, true); rbErr != nil {
					onErr("rollback failed: %s", rbErr)
				}
				os.Exit(0)
			} else {
				onSuccess()
			}
		} else {
			fmt.Println("🙅‍♂️ Skipped execution")
			fmt.Println("🤷‍♂️ No changes to apply")
		}
	}
}

var shellShebangs = map[string]string{
	"/bin/bash": `#!/bin/bash
`,
	"/bin/zsh": `#!/bin/zsh
`,
}

var applyScriptErrorHandling = map[string]string{
	"/bin/bash": `set -euo pipefail`,
	"/bin/zsh":  `set -euo pipefail`,
}

func execApplyScript(
	params ApplyPlanParams,
	toApply map[string]string,
	onErr types.OnErrFn,
	toRollback *types.ApplyRollbackPlan,
	onExecFail types.OnApplyExecFailFn,
	attempt int,
	onSuccess func(),
) {
	log.Println("Executing apply script")

	color.New(term.ColorHiYellow, color.Bold).Println("👉 For long-running commands, use ctrl+c to exit")
	color.New(term.ColorHiCyan, color.Bold).Println("🚀 Executing... output below 👇")

	fmt.Println()

	var content string

	if params.ExecCommand != "" {
		content = params.ExecCommand
	} else {
		content = toApply["_apply.sh"]
	}

	scriptPath := filepath.Join(fs.ProjectRoot, "_apply.sh")
	lines := strings.Split(content, "\n")
	filteredLines := []string{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#!/") {
			continue
		}
		if strings.HasPrefix(trimmed, "set -") || strings.HasSuffix(trimmed, "pipefail") {
			continue
		}
		if strings.HasPrefix(trimmed, "trap") {
			continue
		}
		filteredLines = append(filteredLines, line)
	}

	// Detect shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash" // fallback
	}

	// Get appropriate header
	shebang := shellShebangs[shell]
	if shebang == "" {
		shebang = shellShebangs["/bin/bash"] // fallback if shell not supported
	}
	errorHandling := applyScriptErrorHandling[shell]

	if errorHandling == "" {
		errorHandling = applyScriptErrorHandling["/bin/bash"] // fallback if shell not supported
	}

	header := shebang + "\n" + errorHandling
	content = header + "\n" + strings.Join(filteredLines, "\n")
	err := os.WriteFile(scriptPath, []byte(content), 0755)

	if err != nil {
		onErr("failed to write _apply.sh: %s", err)
	}

	execCmd := exec.Command(shell, "-c", scriptPath)
	execCmd.Dir = fs.ProjectRoot
	execCmd.Env = os.Environ()
	execCmd.Stdin = os.Stdin

	// Create a pipe for both stdout and stderr
	pipe, err := execCmd.StdoutPipe()
	if err != nil {
		// best effort cleanup
		os.Remove(scriptPath)
		onErr("failed to create stdout pipe: %s", err)
	}
	execCmd.Stderr = execCmd.Stdout

	// Set platform-specific process attributes
	SetPlatformSpecificAttrs(execCmd)

	if err := execCmd.Start(); err != nil {
		// best effort cleanup
		os.Remove(scriptPath)
		onErr("failed to start command: %s", err)
	}

	maybeDeleteCgroup := MaybeIsolateCgroup(execCmd)

	pgid, err := syscall.Getpgid(execCmd.Process.Pid)
	if err != nil {
		log.Printf("Getpgid error: %v", err)
	} else {
		log.Printf("Child PID=%d PGID=%d", execCmd.Process.Pid, pgid)
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use atomic variable to prevent data races
	var interrupted atomic.Bool

	// Handle SIGINT and SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)

	var interruptHandled atomic.Bool
	var interruptWG sync.WaitGroup

	// Start the interrupt handler goroutine
	interruptWG.Add(1)
	go func() {
		defer interruptWG.Done()

		for {
			select {
			case sig := <-sigChan:
				if interruptHandled.CompareAndSwap(false, true) {
					fmt.Println()
					color.New(term.ColorHiYellow, color.Bold).Println("👉 Caught interrupt. Exiting gracefully...")
					interrupted.Store(true)

					var sysSig syscall.Signal

					switch sig {
					case os.Interrupt:
						// user pressed Ctrl+C
						sysSig = syscall.SIGINT
					case syscall.SIGTERM:
						// a polite "kill" request
						sysSig = syscall.SIGTERM
					case syscall.SIGHUP:
						sysSig = syscall.SIGHUP
					case syscall.SIGQUIT:
						sysSig = syscall.SIGQUIT
					default:
						sysSig = syscall.SIGINT
					}

					if err := KillProcessGroup(execCmd, sysSig); err != nil {
						log.Printf("Failed to send signal %s to process group: %v", sysSig, err)
					}

					select {
					case <-time.After(2 * time.Second):
						color.New(term.ColorHiYellow, color.Bold).Println("👉 Commands didn't exit after 2 seconds. Sending SIGKILL.")
						if err := KillProcessGroup(execCmd, syscall.SIGKILL); err != nil {
							log.Printf("Failed to terminate process group: %v", err)
						}
						pipe.Close()
						if maybeDeleteCgroup != nil {
							maybeDeleteCgroup()
						}
					case <-ctx.Done():
						if maybeDeleteCgroup != nil {
							maybeDeleteCgroup()
						}
						return
					}
				}

			case <-ctx.Done():
				// If no interrupts occurred, this will be the normal exit path
				if maybeDeleteCgroup != nil {
					maybeDeleteCgroup()
				}
				return
			}
		}
	}()

	// Read and display output in real-time
	scanner := bufio.NewScanner(pipe)
	var outputBuilder strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line)
		outputBuilder.WriteString(line + "\n")
	}

	// Check for scanner errors
	if scanErr := scanner.Err(); scanErr != nil {
		log.Printf("⚠️ Scanner error reading subprocess output: %v", scanErr)
	}

	err = execCmd.Wait()

	// Ensure interrupt handler fully completes before proceeding
	cancel()           // cancel the context, if not already
	interruptWG.Wait() // wait until the interrupt handler goroutine finishes
	signal.Stop(sigChan)
	close(sigChan)

	success := err == nil

	if interrupted.Load() {
		os.Remove(scriptPath)

		fmt.Println()
		color.New(term.ColorHiYellow, color.Bold).Println("👉 Execution interrupted")

		didSucceed, canceled, err := term.ConfirmYesNoCancel("Did the commands succeed?")

		if err != nil {
			onErr("failed to get confirmation user input: %s", err)
		}

		success = didSucceed

		if canceled {
			if rbErr := Rollback(toRollback, true); rbErr != nil {
				onErr("rollback after interrupt failed: %s", rbErr)
			}
			os.Exit(0)
		}
	}

	// remove _apply.sh without overwriting err val
	{
		err := os.Remove(scriptPath)
		if err != nil && !os.IsNotExist(err) {
			onErr("failed to remove _apply.sh: %s", err)
		}
	}

	if !success {
		fmt.Println()
		color.New(term.ColorHiRed, color.Bold).Println("🚨 Commands failed")

		exitErr, ok := err.(*exec.ExitError)
		status := -1
		if ok {
			status = exitErr.ExitCode()
		}
		onExecFail(status, outputBuilder.String(), attempt, toRollback, onErr, onSuccess)
	} else {
		fmt.Println()
		fmt.Println("✅ Commands succeeded")
		onSuccess()
	}
}

func apiApplyPlan(planId, branch string) (string, error) {
	authVars := MustVerifyAuthVarsSilent(auth.Current.IntegratedModelsMode)

	var commitSummary string

	log.Println("Applying plan with API call")

	commitSummary, apiErr := api.Client.ApplyPlan(planId, branch, shared.ApplyPlanRequest{
		AuthVars: authVars,
	})

	if apiErr != nil {
		return "", fmt.Errorf("failed to set pending results applied: %s", apiErr.Msg)
	}

	return commitSummary, nil
}

func commitApplied(autoCommit bool, commitSummary string, updatedFiles []string, currentPlanState *shared.CurrentPlanState) (err error) {
	confirmed := autoCommit
	if !autoCommit {
		fmt.Println("✏️  Plandex can commit these updates with an automatically generated message.")
		fmt.Println()
		// fmt.Println("ℹ️  Only the files that Plandex is updating will be included the commit. Any other changes, staged or unstaged, will remain exactly as they are.")
		// fmt.Println()
		confirmed, err = term.ConfirmYesNo("Commit Plandex updates now?")
		if err != nil {
			return fmt.Errorf("failed to get confirmation user input: %s", err)
		}
	}

	if confirmed {
		// Commit the changes
		msg := currentPlanState.PendingChangesSummaryForApply(commitSummary)
		// log.Println("Committing changes with message:")
		// log.Println(msg)
		// spew.Dump(currentPlanState)
		err = GitAddAndCommitPaths(fs.ProjectRoot, msg, updatedFiles, true)
		if err != nil {
			return fmt.Errorf("failed to commit changes: %s", err.Error())
		}
	}

	return nil
}

// ApplyFiles applies file changes atomically via FileTransaction.
//
// All operations are staged first (snapshots captured), then applied
// sequentially.  If any single write fails the entire set is rolled back
// to the pre-apply state via the captured snapshots.  A PatchStatusReporter
// (if non-nil on the global ActiveReporter) receives lifecycle events so
// the CLI can display staging / applying / rolled-back status per file.
//
// The returned ApplyRollbackPlan is populated from the transaction's own
// snapshot data so that callers who need a manual rollback (e.g. after an
// exec-script failure) can still invoke Rollback() without re-reading disk.
func ApplyFiles(toApply map[string]string, toRemove map[string]bool, projectPaths *types.ProjectPaths) ([]string, *types.ApplyRollbackPlan, error) {
	reporter := shared.NewLoggingReporter()

	// --- Phase: preparing ----------------------------------------------------
	reporter.OnPatchEvent(shared.PatchEvent{
		TxId:      "",
		Phase:     shared.PhasePreparing,
		FileCount: len(toApply) + len(toRemove),
		Timestamp: time.Now(),
	})

	tx := shared.NewFileTransaction("apply", "main", fs.ProjectRoot)
	if err := tx.Begin(); err != nil {
		return nil, nil, fmt.Errorf("failed to begin apply transaction: %w", err)
	}

	reporter.OnPatchEvent(shared.PatchEvent{
		TxId:      tx.Id,
		Phase:     shared.PhaseStaging,
		FileCount: len(toApply) + len(toRemove),
		Timestamp: time.Now(),
	})

	// --- Phase: staging (capture snapshots, enqueue ops) ---------------------
	// Normalise escape sequences used by the plan renderer.
	normalise := func(s string) string {
		return strings.ReplaceAll(s, "\\`\\`\\`", "```")
	}

	// Track which relative paths actually have content changes so we can
	// skip unchanged files without erroring.
	var updatedPaths []string

	for path, content := range toApply {
		if path == "_apply.sh" {
			continue
		}
		content = normalise(content)
		dstPath := filepath.Join(fs.ProjectRoot, path)

		// Check whether content actually differs from what's on disk.
		if existing, err := os.ReadFile(dstPath); err == nil {
			if string(existing) == content {
				reporter.OnFileStatus(shared.FileStatus{
					Path:      path,
					Phase:     shared.PhaseStaging,
					OpType:    "skip",
					Timestamp: time.Now(),
				})
				continue // unchanged – skip entirely
			}
			// File exists and differs → modify
			reporter.OnFileStatus(shared.FileStatus{
				Path:      path,
				Phase:     shared.PhaseStaging,
				OpType:    "modify",
				Timestamp: time.Now(),
			})
			if err := tx.ModifyFile(path, content); err != nil {
				tx.Rollback("staging failed")
				return nil, nil, fmt.Errorf("failed to stage modification for %s: %w", path, err)
			}
		} else {
			// File does not exist → create
			reporter.OnFileStatus(shared.FileStatus{
				Path:      path,
				Phase:     shared.PhaseStaging,
				OpType:    "create",
				Timestamp: time.Now(),
			})
			if err := tx.CreateFile(path, content); err != nil {
				tx.Rollback("staging failed")
				return nil, nil, fmt.Errorf("failed to stage creation for %s: %w", path, err)
			}
		}
		updatedPaths = append(updatedPaths, path)
	}

	for path, remove := range toRemove {
		if !remove {
			continue
		}
		reporter.OnFileStatus(shared.FileStatus{
			Path:      path,
			Phase:     shared.PhaseStaging,
			OpType:    "delete",
			Timestamp: time.Now(),
		})
		if err := tx.DeleteFile(path); err != nil {
			tx.Rollback("staging failed")
			return nil, nil, fmt.Errorf("failed to stage deletion for %s: %w", path, err)
		}
		updatedPaths = append(updatedPaths, path)
	}

	if len(tx.Operations) == 0 {
		// Nothing to do – commit the empty transaction cleanly.
		tx.Commit()
		reporter.OnPatchEvent(shared.PatchEvent{
			TxId:      tx.Id,
			Phase:     shared.PhaseDone,
			Timestamp: time.Now(),
		})
		return updatedPaths, &types.ApplyRollbackPlan{
			PreviousProjectPaths: projectPaths,
			ToRevert:             map[string]types.ApplyReversion{},
		}, nil
	}

	// --- Phase: applying (write files sequentially via transaction) ----------
	reporter.OnPatchEvent(shared.PatchEvent{
		TxId:      tx.Id,
		Phase:     shared.PhaseApplying,
		FileCount: len(tx.Operations),
		Timestamp: time.Now(),
	})

	for {
		op, err := tx.ApplyNext()
		if op == nil {
			break // no more pending operations
		}

		reporter.OnFileStatus(shared.FileStatus{
			Path:      op.Path,
			Phase:     shared.PhaseApplying,
			OpType:    string(op.Type),
			Timestamp: time.Now(),
		})

		if err != nil {
			// A single file failed – rollback the entire transaction.
			reporter.OnFileStatus(shared.FileStatus{
				Path:      op.Path,
				Phase:     shared.PhaseDone,
				OpType:    string(op.Type),
				Error:     err.Error(),
				Timestamp: time.Now(),
			})
			reporter.OnPatchEvent(shared.PatchEvent{
				TxId:      tx.Id,
				Phase:     shared.PhaseRollingBack,
				Reason:    fmt.Sprintf("write failed for %s: %v", op.Path, err),
				Timestamp: time.Now(),
			})

			rbErr := tx.Rollback(fmt.Sprintf("apply failed: %v", err))
			if rbErr != nil {
				return nil, nil, fmt.Errorf("apply failed for %s (%v) and rollback also failed: %w", op.Path, err, rbErr)
			}

			reporter.OnPatchEvent(shared.PatchEvent{
				TxId:      tx.Id,
				Phase:     shared.PhaseDone,
				Reason:    "rolled back",
				Timestamp: time.Now(),
			})
			return nil, nil, fmt.Errorf("apply failed for %s: %w (all changes rolled back)", op.Path, err)
		}
	}

	// --- Phase: committing ---------------------------------------------------
	reporter.OnPatchEvent(shared.PatchEvent{
		TxId:      tx.Id,
		Phase:     shared.PhaseCommitting,
		FileCount: len(tx.Operations),
		Timestamp: time.Now(),
	})

	if err := tx.Commit(); err != nil {
		// Commit guards against pending ops; should not happen here, but
		// roll back defensively.
		tx.Rollback("commit validation failed")
		return nil, nil, fmt.Errorf("transaction commit failed: %w", err)
	}

	reporter.OnPatchEvent(shared.PatchEvent{
		TxId:      tx.Id,
		Phase:     shared.PhaseDone,
		FileCount: len(updatedPaths),
		Timestamp: time.Now(),
	})

	// Build rollback plan from snapshots so callers can still revert manually
	// (e.g. after an exec-script failure that happens post-apply).
	toRevert := map[string]types.ApplyReversion{}
	var toRemoveOnRollback []string
	for _, snap := range tx.Snapshots {
		if snap.Existed {
			toRevert[snap.Path] = types.ApplyReversion{Content: snap.Content, Mode: snap.Mode}
		} else {
			toRemoveOnRollback = append(toRemoveOnRollback, snap.Path)
		}
	}

	return updatedPaths, &types.ApplyRollbackPlan{
		PreviousProjectPaths: projectPaths,
		ToRevert:             toRevert,
		ToRemove:             toRemoveOnRollback,
	}, nil
}

// Rollback restores every file in rollbackPlan to its pre-apply state.
//
// Operations are performed sequentially in reverse snapshot order so that
// each restore is confirmed before the next begins.  If any single restore
// fails the function continues with the remaining files and collects all
// errors into the returned value — the caller can surface them, but we
// never stop mid-rollback because a partially-restored project is worse
// than a partially-failed rollback.
//
// When msg is true the function prints a per-phase status line:
//
//	🔄 Rolling back changes…
//	  ↩ restored  src/foo.go
//	  ✗ removed   src/bar.go  (failed: …)
//	🚫 Rolled back N file(s)
func Rollback(rollbackPlan *types.ApplyRollbackPlan, msg bool) error {
	if rollbackPlan == nil || !rollbackPlan.HasChanges() {
		return nil
	}

	if msg {
		fmt.Println()
		color.New(term.ColorHiYellow, color.Bold).Println("🔄 Rolling back changes…")
	}

	var errs []error
	restored := 0

	// 1. Restore files that existed before the apply (content revert).
	for path, revert := range rollbackPlan.ToRevert {
		if err := os.WriteFile(path, []byte(revert.Content), revert.Mode); err != nil {
			errs = append(errs, fmt.Errorf("failed to restore %s: %w", path, err))
			if msg {
				color.New(term.ColorHiRed).Printf("  ✗ restore  %s  (%v)\n", path, err)
			}
			continue
		}
		restored++
		if msg {
			color.New(term.ColorHiGreen).Printf("  ↩ restored  %s\n", path)
		}
	}

	// 2. Remove files that were newly created by the apply.
	for _, path := range rollbackPlan.ToRemove {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", path, err))
			if msg {
				color.New(term.ColorHiRed).Printf("  ✗ remove   %s  (%v)\n", path, err)
			}
			continue
		}
		restored++
		if msg {
			color.New(term.ColorHiGreen).Printf("  ↩ removed   %s\n", path)
		}
	}

	// 3. Sweep any files that appeared in the project after the apply but
	//    were not part of the explicit apply set (e.g. side-effect files
	//    created by a plan renderer).  Only run when PreviousProjectPaths is
	//    available.
	if rollbackPlan.PreviousProjectPaths != nil {
		updatedPaths, pathErr := fs.GetProjectPaths(fs.ProjectRoot)
		if pathErr != nil {
			errs = append(errs, fmt.Errorf("failed to scan project paths for sweep: %w", pathErr))
		} else {
			for path := range updatedPaths.AllPaths {
				if _, existed := rollbackPlan.PreviousProjectPaths.AllPaths[path]; !existed {
					if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
						errs = append(errs, fmt.Errorf("failed to sweep %s: %w", path, rmErr))
						if msg {
							color.New(term.ColorHiRed).Printf("  ✗ sweep    %s  (%v)\n", path, rmErr)
						}
						continue
					}
					restored++
					if msg {
						color.New(term.ColorHiGreen).Printf("  ↩ swept    %s\n", path)
					}
				}
			}
		}
	}

	if msg {
		if len(errs) > 0 {
			color.New(term.ColorHiRed, color.Bold).Printf("⚠ Rollback completed with %d error(s), %d file(s) restored\n", len(errs), restored)
		} else {
			color.New(term.ColorHiGreen, color.Bold).Printf("🚫 Rolled back %d file(s)\n", restored)
		}
		fmt.Println()
	}

	if len(errs) > 0 {
		return fmt.Errorf("rollback completed with errors: %v", errs)
	}
	return nil
}
