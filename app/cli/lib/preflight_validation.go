package lib

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"plandex-cli/fs"

	shared "plandex-shared"
)

// =============================================================================
// PREFLIGHT VALIDATION — execution-readiness gate
// =============================================================================
//
// MustRunPreflightChecks is the last validation gate before plan_exec starts
// real work (LLM calls, file writes, streaming).  It runs every registered
// preflightCheck in order, collects all failures, and surfaces them together
// so the user can fix everything in one pass.
//
// Checks are intentionally cheap: no API calls, no file scanning.  They target
// conditions that would cause a cryptic mid-execution failure if left uncaught.
//
// =============================================================================

// preflightCheck is a single preflight check function.
type preflightCheck struct {
	Name     string                   // machine-readable label for logging / registry
	Category shared.ValidationCategory // reuses existing category enum
	Run      func() *shared.ValidationError
}

// preflightChecks is the ordered registry of all preflight checks.
var preflightChecks = []preflightCheck{
	{"project_root_writable", shared.CategoryFilesystem, checkProjectRootWritable},
	{"shell_available", shared.CategoryConfig, checkShellAvailable},
	{"repl_output_dir", shared.CategoryFilesystem, checkReplOutputDir},
	{"api_host_valid", shared.CategoryConfig, checkAPIHostValid},
	{"projects_file_valid", shared.CategoryFilesystem, checkProjectsFileValid},
	{"settings_file_valid", shared.CategoryFilesystem, checkSettingsFileValid},
}

// MustRunPreflightChecks runs all preflight checks for the current plan.
// It exits with a formatted error report if any fatal checks fail.
func MustRunPreflightChecks() {
	if CurrentPlanId == "" {
		return // no plan loaded — nothing to preflight
	}

	result := shared.NewValidationResult(shared.PhasePreflight)

	for _, check := range preflightChecks {
		if ve := check.Run(); ve != nil {
			result.Add(ve)
		}
	}

	// Persist preflight-specific ErrorReport so the registry distinguishes
	// preflight failures from startup-phase failures.
	report := buildPreflightErrorReport(result)
	if report != nil {
		shared.StoreError(report)
	}

	// Warnings only: print but continue.
	if len(result.Warnings) > 0 && len(result.FatalErrors) == 0 {
		fmt.Fprint(os.Stderr, "\n")
		fmt.Fprint(os.Stderr, result.FormatCLI())
		return
	}

	// Fatal errors stop execution.
	if !result.Passed {
		fmt.Fprint(os.Stderr, "\n")
		fmt.Fprint(os.Stderr, result.FormatCLI())
		if shared.GlobalErrorRegistry != nil {
			shared.GlobalErrorRegistry.Persist()
		}
		os.Exit(1)
	}
}

// buildPreflightErrorReport wraps a failed preflight result into an ErrorReport
// with labels distinct from startup validation so the error registry can
// distinguish the two phases.
func buildPreflightErrorReport(r *shared.ValidationResult) *shared.ErrorReport {
	if r.Passed {
		return nil
	}

	var msgs []string
	for _, e := range r.FatalErrors {
		msgs = append(msgs, fmt.Sprintf("[%s] %s", e.Code, e.Message))
	}

	report := shared.NewErrorReport(
		shared.ErrorCategoryValidation,
		"preflight_validation_failed",
		"PREFLIGHT_VALIDATION",
		strings.Join(msgs, "; "),
	)

	report.StepContext = &shared.StepContext{Phase: shared.PhaseValidation}
	report.Recovery = &shared.RecoveryAction{
		CanAutoRecover: false,
		ManualActions:  make([]shared.ManualAction, 0, len(r.FatalErrors)),
	}
	for _, e := range r.FatalErrors {
		report.Recovery.ManualActions = append(report.Recovery.ManualActions, shared.ManualAction{
			Description: e.Fix,
			Priority:    "critical",
		})
	}

	return report
}

// =============================================================================
// CHECK IMPLEMENTATIONS
// =============================================================================

// checkProjectRootWritable verifies that the project root directory exists,
// is a directory, and is writable.  Apply scripts and file writes will fail
// at runtime if this is not true.
func checkProjectRootWritable() *shared.ValidationError {
	root := fs.ProjectRoot
	if root == "" {
		return &shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityFatal,
			Phase:    shared.PhasePreflight,
			Code:     "PROJECT_ROOT_UNSET",
			Message:  "Project root directory path is empty",
			Fix:      "Run plandex from within a project directory",
		}
	}

	info, err := os.Stat(root)
	if err != nil {
		return &shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityFatal,
			Phase:    shared.PhasePreflight,
			Code:     "PROJECT_ROOT_MISSING",
			Message:  fmt.Sprintf("Project root directory does not exist: %s", root),
			Fix:      fmt.Sprintf("Create the directory or cd into an existing project: mkdir -p %s", root),
			Path:     root,
		}
	}

	if !info.IsDir() {
		return &shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityFatal,
			Phase:    shared.PhasePreflight,
			Code:     "PROJECT_ROOT_NOT_DIR",
			Message:  fmt.Sprintf("Project root path exists but is not a directory: %s", root),
			Fix:      fmt.Sprintf("Remove the file and create a directory: rm %s && mkdir -p %s", root, root),
			Path:     root,
		}
	}

	// Write-check via temp file.
	testPath := filepath.Join(root, ".plandex_preflight_check")
	if err := os.WriteFile(testPath, []byte("ok"), 0600); err != nil {
		return &shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityFatal,
			Phase:    shared.PhasePreflight,
			Code:     "PROJECT_ROOT_NOT_WRITABLE",
			Message:  fmt.Sprintf("Project root directory is not writable: %s", root),
			Fix:      fmt.Sprintf("Fix permissions: chmod 755 %s", root),
			Path:     root,
		}
	}
	os.Remove(testPath)
	return nil
}

// checkShellAvailable verifies that a shell is available for apply scripts.
// The SHELL env var must be set, or /bin/bash must exist as a fallback.
func checkShellAvailable() *shared.ValidationError {
	if os.Getenv("SHELL") != "" {
		return nil
	}

	// Fallback: /bin/bash
	if _, err := os.Stat("/bin/bash"); err == nil {
		return nil
	}

	return &shared.ValidationError{
		Category: shared.CategoryConfig,
		Severity: shared.ValidationSeverityFatal,
		Phase:    shared.PhasePreflight,
		Code:     "SHELL_UNAVAILABLE",
		Message:  "No shell available: SHELL env var is empty and /bin/bash does not exist",
		Fix:      "Set the SHELL environment variable to a valid shell path (e.g. export SHELL=/bin/bash)",
		EnvVar:   "SHELL",
	}
}

// checkReplOutputDir verifies that PLANDEX_REPL_OUTPUT_FILE, if set, has a
// writable parent directory.
func checkReplOutputDir() *shared.ValidationError {
	outputFile := os.Getenv("PLANDEX_REPL_OUTPUT_FILE")
	if outputFile == "" {
		return nil // not configured — nothing to check
	}

	dir := filepath.Dir(outputFile)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return &shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityWarn,
			Phase:    shared.PhasePreflight,
			Code:     "REPL_OUTPUT_DIR_MISSING",
			Message:  fmt.Sprintf("PLANDEX_REPL_OUTPUT_FILE parent directory does not exist: %s", dir),
			Fix:      fmt.Sprintf("Create the directory: mkdir -p %s", dir),
			EnvVar:   "PLANDEX_REPL_OUTPUT_FILE",
			Path:     outputFile,
		}
	}

	return nil
}

// checkAPIHostValid verifies that PLANDEX_API_HOST, if set, is a valid URL
// with a host component.
func checkAPIHostValid() *shared.ValidationError {
	apiHost := os.Getenv("PLANDEX_API_HOST")
	if apiHost == "" {
		return nil // not configured — uses default
	}

	parsed, err := url.Parse(apiHost)
	if err != nil || parsed.Host == "" {
		return &shared.ValidationError{
			Category: shared.CategoryConfig,
			Severity: shared.ValidationSeverityWarn,
			Phase:    shared.PhasePreflight,
			Code:     "API_HOST_INVALID",
			Message:  fmt.Sprintf("PLANDEX_API_HOST is not a valid URL: %q", apiHost),
			Fix:      "Set PLANDEX_API_HOST to a valid URL with scheme and host (e.g. https://api.example.com)",
			EnvVar:   "PLANDEX_API_HOST",
		}
	}

	return nil
}

// checkProjectsFileValid verifies that projects-v2.json, if present, is valid
// JSON.  A malformed file causes a bare unmarshal exit downstream.
func checkProjectsFileValid() *shared.ValidationError {
	return checkPreflightJSONFile(
		filepath.Join(fs.HomePlandexDir, "projects-v2.json"),
		"projects-v2.json",
		"PROJECTS_FILE_MALFORMED",
	)
}

// checkSettingsFileValid verifies that settings-v2.json, if present, is valid
// JSON.  A malformed file causes a bare unmarshal exit downstream.
func checkSettingsFileValid() *shared.ValidationError {
	return checkPreflightJSONFile(
		filepath.Join(fs.HomePlandexDir, "settings-v2.json"),
		"settings-v2.json",
		"SETTINGS_FILE_MALFORMED",
	)
}

// checkPreflightJSONFile is a helper that validates a JSON config file if
// present.  Missing files are fine (first-time state); malformed content is
// fatal.
func checkPreflightJSONFile(path, label, code string) *shared.ValidationError {
	data, err := os.ReadFile(path)
	if err != nil {
		// File missing is acceptable.
		return nil
	}

	if len(data) == 0 {
		// Empty file is a warning, not fatal — downstream may re-create it.
		return &shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityWarn,
			Phase:    shared.PhasePreflight,
			Code:     strings.Replace(code, "_MALFORMED", "_EMPTY", 1),
			Message:  fmt.Sprintf("%s exists but is empty — Plandex may need to reinitialize", label),
			Fix:      fmt.Sprintf("Delete and let Plandex recreate: rm %s", path),
			Path:     path,
		}
	}

	var target interface{}
	if err := json.Unmarshal(data, &target); err != nil {
		return &shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityFatal,
			Phase:    shared.PhasePreflight,
			Code:     code,
			Message:  fmt.Sprintf("%s contains invalid JSON: %v", label, err),
			Fix:      fmt.Sprintf("Delete the file and let Plandex recreate it: rm %s", path),
			Path:     path,
		}
	}

	return nil
}
