# Preflight Validation Integration Guide

This guide shows how to integrate the new preflight validation phase into CLI commands to ensure all systems are ready before any work begins.

## Overview

The preflight validation phase provides comprehensive checks before execution:

1. **Config Files** - Validates .env file format
2. **Environment Variables** - Checks for required vars and conflicts
3. **Database** - Verifies connectivity
4. **LiteLLM Proxy** - Confirms proxy health
5. **AI Providers** - Validates all configured provider credentials
6. **System Resources** - Checks system readiness

## When to Use Preflight Validation

Use preflight validation for commands that:
- Execute plans or apply changes
- Interact with AI providers
- Modify files or databases
- Run long-running operations

**Examples:**
- `plandex tell` - Before starting a new conversation
- `plandex continue` - Before continuing execution
- `plandex build` - Before building a plan
- `plandex apply` - Before applying changes

## Integration Examples

### Example 1: Basic Integration in CLI Command

```go
package cmd

import (
    "context"
    "time"
    "plandex-shared/validation"
)

func executePlan(args []string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Run preflight validation
    result, err := validation.RunPreflightValidation(ctx)
    if err != nil {
        // Critical failures - execution blocked
        log.Println("\n⛔ Preflight validation failed - cannot proceed")
        result.PrintDetailedReport(true)
        return err
    }

    // Print summary
    log.Println(result.Summary())

    // All systems ready - proceed with execution
    return performExecution(args)
}
```

### Example 2: Feature Flag Aware Integration

```go
package cmd

import (
    "context"
    "plandex-shared/validation"
)

func executePlanWithValidation(args []string) error {
    ctx := context.Background()

    // Use safe wrapper - respects feature flags
    if err := validation.SafeValidatePreflight(ctx); err != nil {
        log.Printf("❌ Preflight validation failed: %v\n", err)
        return err
    }

    // Proceed with execution
    return performExecution(args)
}
```

### Example 3: Quick Preflight for Fast Commands

```go
package cmd

import (
    "context"
    "plandex-shared/validation"
)

func quickExecution(args []string) error {
    ctx := context.Background()

    // Use quick preflight check (10s timeout, no file checks)
    result, err := validation.QuickPreflightCheck(ctx)
    if err != nil {
        log.Printf("⚠️  Quick preflight failed: %v\n", err)
        // Optionally allow execution with warnings in non-strict mode
        if !isStrictMode() {
            log.Println("Continuing despite validation warnings...")
            return performExecution(args)
        }
        return err
    }

    return performExecution(args)
}
```

### Example 4: Custom Preflight Checks

```go
package cmd

import (
    "context"
    "time"
    "plandex-shared/validation"
)

func customPreflight(requiredProviders []string) error {
    // Create custom preflight options
    opts := validation.DefaultPreflightOptions()
    opts.ProviderNames = requiredProviders  // Only validate specific providers
    opts.Timeout = 15 * time.Second         // Shorter timeout
    opts.SkipLiteLLM = true                 // Skip LiteLLM if not needed

    // Create and run custom preflight validator
    pv := validation.NewPreflightValidator(opts)
    result := pv.RunPreflight(context.Background())

    if !result.IsReadyToExecute() {
        return fmt.Errorf("custom preflight failed with %d critical errors", result.CriticalFailures)
    }

    return nil
}
```

### Example 5: Progressive Validation with Fallback

```go
package cmd

import (
    "context"
    "plandex-shared/validation"
)

func progressiveValidation(args []string) error {
    ctx := context.Background()

    // Try full preflight first
    result, err := validation.RunPreflightValidation(ctx)

    if err != nil {
        // Critical failures
        if result.CriticalFailures > 0 {
            // Cannot proceed
            result.PrintDetailedReport(true)
            return fmt.Errorf("critical preflight failures - execution blocked")
        }

        // Non-critical failures - warn and ask user
        log.Println("\n⚠️  Non-critical validation issues detected:")
        result.PrintDetailedReport(false)

        if !promptUserToContinue() {
            return fmt.Errorf("user aborted due to validation warnings")
        }
    }

    return performExecution(args)
}
```

## Output Examples

### Successful Preflight

```
╔════════════════════════════════════════════════════════════════════╗
║                    PREFLIGHT VALIDATION                            ║
╚════════════════════════════════════════════════════════════════════╝

[1/6] Running: Config Files
        Validate configuration file format and loading
        ✅ PASSED

[2/6] Running: Environment Variables
        Validate required environment variables and detect conflicts
        ✅ PASSED

[3/6] Running: Database Connectivity
        Verify database configuration and test connection
        ✅ PASSED

[4/6] Running: LiteLLM Proxy
        Check LiteLLM proxy availability and health
        ✅ PASSED

[5/6] Running: AI Provider Credentials
        Validate credentials for all configured providers
        ✅ PASSED

[6/6] Running: System Resources
        Check system readiness and resource availability
        ✅ PASSED

╔════════════════════════════════════════════════════════════════════╗
║                    PREFLIGHT VALIDATION SUMMARY                    ║
╚════════════════════════════════════════════════════════════════════╝

Status:     ✅ READY
Duration:   245ms
Total Checks: 6

Results:
  ✅ Passed:              6
  ⚠️  Warnings:            0
  ❌ Critical Failures:   0
  ❌ Non-Critical Fails:  0

✅ ALL SYSTEMS GO
   Configuration validated - ready to execute.
```

### Failed Preflight (Critical)

```
╔════════════════════════════════════════════════════════════════════╗
║                    PREFLIGHT VALIDATION                            ║
╚════════════════════════════════════════════════════════════════════╝

[1/6] Running: Config Files
        Validate configuration file format and loading
        ⚠️  PASSED with 1 warning(s)

[2/6] Running: Environment Variables
        Validate required environment variables and detect conflicts
        ✅ PASSED

[3/6] Running: Database Connectivity
        Verify database configuration and test connection
        ❌ FAILED (critical) - 1 error(s)

⛔ Critical preflight check failed - execution blocked

╔════════════════════════════════════════════════════════════════════╗
║                    PREFLIGHT VALIDATION SUMMARY                    ║
╚════════════════════════════════════════════════════════════════════╝

Status:     ⛔ BLOCKED
Duration:   156ms
Total Checks: 3

Results:
  ✅ Passed:              1
  ⚠️  Warnings:            1
  ❌ Critical Failures:   1
  ❌ Non-Critical Fails:  0

⛔ EXECUTION BLOCKED
   Fix critical errors before proceeding.

❌ FAILED CHECKS:
════════════════════════════════════════════════════════════════════

Database Connectivity (12ms):

❌ Database: No database configuration found

📋 Details:
Neither DATABASE_URL nor individual DB_* environment variables are set.
The server requires database configuration to function.

⚠️  Impact:
The server will fail to start without database configuration.
All database-dependent features will be unavailable.

✅ Solution:
Set DATABASE_URL or configure individual DB_* variables:

Option 1: Use DATABASE_URL (recommended)
  export DATABASE_URL=postgresql://user:password@localhost:5432/plandex

Option 2: Use individual variables
  export DB_HOST=localhost
  export DB_PORT=5432
  export DB_USER=plandex
  export DB_PASSWORD=your_password
  export DB_NAME=plandex
```

## Best Practices

### 1. Use SafeValidatePreflight for Feature Flag Compatibility

```go
// Respects PLANDEX_ENABLE_VALIDATION flag
err := validation.SafeValidatePreflight(ctx)
```

### 2. Set Appropriate Timeouts

```go
// For quick commands
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

// For thorough validation
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
```

### 3. Handle Results Appropriately

```go
result, err := validation.RunPreflightValidation(ctx)

// Always check if ready to execute
if !result.IsReadyToExecute() {
    // Handle based on failure type
    if result.CriticalFailures > 0 {
        // Must block execution
        return err
    }

    if result.NonCriticalFailures > 0 {
        // Can continue with user consent
        if !promptUser("Continue despite warnings?") {
            return err
        }
    }
}
```

### 4. Provide Verbose Output When Helpful

```go
// Use verbose output for debugging
result.PrintDetailedReport(true)

// Use concise output for normal operation
result.PrintDetailedReport(false)

// Or just print summary
log.Println(result.Summary())
```

### 5. Cache Validation Results When Appropriate

```go
var cachedPreflightResult *validation.PreflightResult
var preflightCacheTTL = 5 * time.Minute

func getCachedPreflight(ctx context.Context) (*validation.PreflightResult, error) {
    // Check cache
    if cachedPreflightResult != nil && time.Since(cachedPreflightResult.StartTime) < preflightCacheTTL {
        return cachedPreflightResult, nil
    }

    // Run new preflight
    result, err := validation.RunPreflightValidation(ctx)
    if err == nil {
        cachedPreflightResult = result
    }

    return result, err
}
```

## Integration Checklist

When integrating preflight validation into a CLI command:

- [ ] Determine if validation should run (command modifies state or uses resources)
- [ ] Choose validation type (full preflight vs quick check)
- [ ] Set appropriate timeout (10s for quick, 30s for comprehensive)
- [ ] Use SafeValidatePreflight for feature flag compatibility
- [ ] Handle critical vs non-critical failures appropriately
- [ ] Provide clear output to user on failure
- [ ] Consider caching results for frequently-run commands
- [ ] Test with and without feature flags enabled
- [ ] Test with various failure scenarios
- [ ] Document the validation behavior in command help text

## Testing Preflight Integration

```go
func TestPreflightIntegration(t *testing.T) {
    t.Run("Preflight blocks on critical failure", func(t *testing.T) {
        // Unset database config to trigger critical failure
        os.Unsetenv("DATABASE_URL")

        ctx := context.Background()
        result, err := validation.RunPreflightValidation(ctx)

        if err == nil {
            t.Error("Should fail with missing database config")
        }

        if result.IsReadyToExecute() {
            t.Error("Should not be ready with critical failures")
        }
    })

    t.Run("Preflight succeeds with valid config", func(t *testing.T) {
        // Set up valid config
        os.Setenv("DATABASE_URL", "postgresql://localhost:5432/test")
        os.Setenv("OPENAI_API_KEY", "sk-test-key")
        defer os.Unsetenv("DATABASE_URL")
        defer os.Unsetenv("OPENAI_API_KEY")

        ctx := context.Background()
        result, err := validation.QuickPreflightCheck(ctx)

        // May succeed or fail based on actual connectivity
        // But result should always be non-nil
        if result == nil {
            t.Error("Should return result")
        }
    })
}
```

## Migration from Existing Validation

If you're currently using `ValidateExecution`:

```go
// Before
err := validation.ValidateExecution(ctx, providers)

// After - more comprehensive
err := validation.SafeValidatePreflight(ctx)
```

The preflight validation includes everything from execution validation plus:
- Config file validation
- More thorough environment checks
- Detailed progress reporting
- Better error categorization

## Additional Resources

- [VALIDATION_SYSTEM.md](docs/VALIDATION_SYSTEM.md) - Complete system documentation
- [VALIDATION_EXAMPLES.md](docs/VALIDATION_EXAMPLES.md) - Example failure scenarios
- [README.md](app/shared/validation/README.md) - Package documentation
- [preflight.go](app/shared/validation/preflight.go) - Implementation source
