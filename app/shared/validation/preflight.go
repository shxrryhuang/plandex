package validation

import (
	"context"
	"fmt"
	"log"
	"time"
)

// PreflightCheck represents a single preflight validation check
type PreflightCheck struct {
	Name        string
	Description string
	Execute     func(ctx context.Context) *ValidationResult
	Critical    bool // If true, failure blocks execution
	Priority    int  // Higher priority runs first
}

// PreflightValidator performs comprehensive validation before any work begins
type PreflightValidator struct {
	checks  []PreflightCheck
	options ValidationOptions
}

// NewPreflightValidator creates a new preflight validator
func NewPreflightValidator(opts ValidationOptions) *PreflightValidator {
	pv := &PreflightValidator{
		options: opts,
	}
	pv.initializeChecks()
	return pv
}

// initializeChecks sets up all preflight checks in priority order
func (pv *PreflightValidator) initializeChecks() {
	pv.checks = []PreflightCheck{
		{
			Name:        "Config Files",
			Description: "Validate configuration file format and loading",
			Priority:    100,
			Critical:    false, // Warnings only
			Execute: func(ctx context.Context) *ValidationResult {
				return ValidateAllConfigFiles()
			},
		},
		{
			Name:        "Environment Variables",
			Description: "Validate required environment variables and detect conflicts",
			Priority:    95,
			Critical:    true,
			Execute: func(ctx context.Context) *ValidationResult {
				return ValidateEnvironment()
			},
		},
		{
			Name:        "Database Connectivity",
			Description: "Verify database configuration and test connection",
			Priority:    90,
			Critical:    true,
			Execute: func(ctx context.Context) *ValidationResult {
				return ValidateDatabase(ctx)
			},
		},
		{
			Name:        "LiteLLM Proxy",
			Description: "Check LiteLLM proxy availability and health",
			Priority:    85,
			Critical:    true,
			Execute: func(ctx context.Context) *ValidationResult {
				return ValidateLiteLLMProxyHealth(ctx)
			},
		},
		{
			Name:        "AI Provider Credentials",
			Description: "Validate credentials for all configured providers",
			Priority:    80,
			Critical:    true,
			Execute: func(ctx context.Context) *ValidationResult {
				return ValidateAllProviders(pv.options.CheckFileAccess)
			},
		},
		{
			Name:        "System Resources",
			Description: "Check system readiness and resource availability",
			Priority:    70,
			Critical:    false,
			Execute: func(ctx context.Context) *ValidationResult {
				return ValidateSystemResources()
			},
		},
	}
}

// RunPreflight executes all preflight checks with detailed progress reporting
func (pv *PreflightValidator) RunPreflight(ctx context.Context) *PreflightResult {
	startTime := time.Now()

	log.Println("\n╔════════════════════════════════════════════════════════════════════╗")
	log.Println("║                    PREFLIGHT VALIDATION                            ║")
	log.Println("╚════════════════════════════════════════════════════════════════════╝")
	log.Println()

	result := &PreflightResult{
		StartTime: startTime,
		Checks:    make(map[string]*CheckResult),
	}

	// Apply timeout
	if pv.options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, pv.options.Timeout)
		defer cancel()
	}

	// Run checks in priority order
	for i, check := range pv.checks {
		log.Printf("[%d/%d] Running: %s\n", i+1, len(pv.checks), check.Name)
		log.Printf("        %s\n", check.Description)

		checkStart := time.Now()
		validationResult := check.Execute(ctx)
		checkDuration := time.Since(checkStart)

		checkResult := &CheckResult{
			Name:     check.Name,
			Duration: checkDuration,
			Result:   validationResult,
			Critical: check.Critical,
		}

		result.Checks[check.Name] = checkResult

		// Report check status
		if validationResult.HasErrors() {
			if check.Critical {
				log.Printf("        ❌ FAILED (critical) - %d error(s)\n", len(validationResult.Errors))
				result.CriticalFailures++
			} else {
				log.Printf("        ❌ FAILED (non-critical) - %d error(s)\n", len(validationResult.Errors))
				result.NonCriticalFailures++
			}
		} else if validationResult.HasWarnings() {
			log.Printf("        ⚠️  PASSED with %d warning(s)\n", len(validationResult.Warnings))
			result.Warnings++
		} else {
			log.Printf("        ✅ PASSED\n")
			result.Passed++
		}

		// Stop on critical failure
		if check.Critical && validationResult.HasErrors() {
			log.Println()
			log.Println("⛔ Critical preflight check failed - execution blocked")
			result.Duration = time.Since(startTime)
			result.Blocked = true
			return result
		}

		log.Println()
	}

	result.Duration = time.Since(startTime)
	return result
}

// PreflightResult contains the results of all preflight checks
type PreflightResult struct {
	StartTime            time.Time
	Duration             time.Duration
	Checks               map[string]*CheckResult
	Passed               int
	Warnings             int
	CriticalFailures     int
	NonCriticalFailures  int
	Blocked              bool // True if a critical check failed
}

// CheckResult contains the result of a single preflight check
type CheckResult struct {
	Name     string
	Duration time.Duration
	Result   *ValidationResult
	Critical bool
}

// IsReadyToExecute returns true if all critical checks passed
func (pr *PreflightResult) IsReadyToExecute() bool {
	return !pr.Blocked && pr.CriticalFailures == 0
}

// Summary returns a formatted summary of preflight results
func (pr *PreflightResult) Summary() string {
	status := "READY"
	icon := "✅"

	if pr.Blocked || pr.CriticalFailures > 0 {
		status = "BLOCKED"
		icon = "⛔"
	} else if pr.NonCriticalFailures > 0 || pr.Warnings > 0 {
		status = "READY (with warnings)"
		icon = "⚠️"
	}

	return fmt.Sprintf(`
╔════════════════════════════════════════════════════════════════════╗
║                    PREFLIGHT VALIDATION SUMMARY                    ║
╚════════════════════════════════════════════════════════════════════╝

Status:     %s %s
Duration:   %dms
Total Checks: %d

Results:
  ✅ Passed:              %d
  ⚠️  Warnings:            %d
  ❌ Critical Failures:   %d
  ❌ Non-Critical Fails:  %d

%s
`,
		icon, status,
		pr.Duration.Milliseconds(),
		len(pr.Checks),
		pr.Passed,
		pr.Warnings,
		pr.CriticalFailures,
		pr.NonCriticalFailures,
		pr.executionAdvice(),
	)
}

// executionAdvice returns advice based on preflight results
func (pr *PreflightResult) executionAdvice() string {
	if pr.Blocked || pr.CriticalFailures > 0 {
		return "⛔ EXECUTION BLOCKED\n   Fix critical errors before proceeding."
	}

	if pr.NonCriticalFailures > 0 {
		return "⚠️  NON-CRITICAL ISSUES DETECTED\n   You may proceed, but some features may not work correctly."
	}

	if pr.Warnings > 0 {
		return "⚠️  WARNINGS DETECTED\n   Review warnings but execution can proceed normally."
	}

	return "✅ ALL SYSTEMS GO\n   Configuration validated - ready to execute."
}

// GetFailedChecks returns all checks that had errors
func (pr *PreflightResult) GetFailedChecks() []*CheckResult {
	var failed []*CheckResult
	for _, check := range pr.Checks {
		if check.Result.HasErrors() {
			failed = append(failed, check)
		}
	}
	return failed
}

// GetWarningChecks returns all checks that had warnings
func (pr *PreflightResult) GetWarningChecks() []*CheckResult {
	var warnings []*CheckResult
	for _, check := range pr.Checks {
		if check.Result.HasWarnings() {
			warnings = append(warnings, check)
		}
	}
	return warnings
}

// PrintDetailedReport prints a detailed report of all checks
func (pr *PreflightResult) PrintDetailedReport(verbose bool) {
	log.Println(pr.Summary())

	// Print failed checks
	failed := pr.GetFailedChecks()
	if len(failed) > 0 {
		log.Println("\n❌ FAILED CHECKS:")
		log.Println("════════════════════════════════════════════════════════════════════")
		for _, check := range failed {
			log.Printf("\n%s (%s):\n", check.Name, check.Duration)
			log.Println(FormatResult(check.Result, verbose))
		}
	}

	// Print warning checks
	warnings := pr.GetWarningChecks()
	if len(warnings) > 0 && verbose {
		log.Println("\n⚠️  WARNINGS:")
		log.Println("════════════════════════════════════════════════════════════════════")
		for _, check := range warnings {
			log.Printf("\n%s (%s):\n", check.Name, check.Duration)
			log.Println(FormatResult(check.Result, verbose))
		}
	}
}

// ValidateSystemResources checks system readiness
func ValidateSystemResources() *ValidationResult {
	result := &ValidationResult{}

	// This is a placeholder for system resource validation
	// In a full implementation, this would check:
	// - Available memory
	// - Available disk space
	// - CPU load
	// - Network connectivity
	// - File descriptor limits
	// - Process limits

	// For now, just return success
	// Future enhancement: implement actual resource checks

	return result
}

// RunPreflightValidation is a convenience function that creates and runs a preflight validator
func RunPreflightValidation(ctx context.Context) (*PreflightResult, error) {
	opts := DefaultPreflightOptions()
	pv := NewPreflightValidator(opts)
	result := pv.RunPreflight(ctx)

	if !result.IsReadyToExecute() {
		return result, fmt.Errorf("preflight validation failed: %d critical error(s)", result.CriticalFailures)
	}

	return result, nil
}

// QuickPreflightCheck performs a fast preflight check with reduced timeout
func QuickPreflightCheck(ctx context.Context) (*PreflightResult, error) {
	opts := DefaultPreflightOptions()
	opts.Timeout = 10 * time.Second
	opts.CheckFileAccess = false // Skip slow file checks

	pv := NewPreflightValidator(opts)
	result := pv.RunPreflight(ctx)

	if !result.IsReadyToExecute() {
		return result, fmt.Errorf("quick preflight check failed")
	}

	return result, nil
}
