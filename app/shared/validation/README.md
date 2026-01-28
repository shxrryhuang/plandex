# Plandex Configuration Validation System

This package provides comprehensive configuration validation for Plandex, catching misconfigurations early and providing clear, actionable error messages.

## Overview

Configuration errors often only surface deep into execution, after time and resources have been wasted. The validation system addresses this by:

1. **Early Detection**: Validates configuration before execution begins
2. **Phased Validation**: Synchronous checks at startup, deferred checks when features are used
3. **Clear Messages**: Specific, actionable error messages instead of cryptic stack traces
4. **Multiple Integrations**: Works in server startup, CLI execution, and runtime

## Architecture

### Validation Phases

The system supports four validation phases:

#### 1. Startup Phase (`PhaseStartup`)
Runs synchronously at server startup to catch critical issues before services start.

**Validates:**
- Database connectivity and configuration
- Environment variable format and conflicts
- Port availability (e.g., LiteLLM port 4000)

**Skips:**
- Provider credentials (deferred to execution)
- File access checks (performance optimization)

**Example:**
```go
err := validation.ValidateStartup(context.Background())
if err != nil {
    log.Fatal("Startup validation failed: ", err)
}
```

#### 2. Preflight Phase (`PhasePreflight`) **NEW**
Comprehensive validation that runs before ANY work begins. This is the most thorough validation phase, checking all systems to ensure readiness for execution.

**Validates:**
- Configuration file format and loading (.env files)
- Environment variables and conflicts
- Database connectivity
- LiteLLM proxy health
- All AI provider credentials
- System resources (placeholder for future enhancements)

**Features:**
- Detailed progress reporting with check-by-check status
- Critical vs non-critical failure distinction
- Blocks execution only on critical failures
- Comprehensive summary report

**Example:**
```go
result, err := validation.RunPreflightValidation(context.Background())
if err != nil {
    // Critical failure - execution blocked
    result.PrintDetailedReport(true)
    log.Fatal("Preflight validation failed: ", err)
}

log.Println(result.Summary())
// Proceed with execution
```

**Quick Preflight Check:**
```go
// Faster check with reduced timeout
result, err := validation.QuickPreflightCheck(context.Background())
```

#### 3. Execution Phase (`PhaseExecution`)
Runs before plan execution to validate all required resources are available.

**Validates:**
- Provider credentials for models being used
- File paths and permissions
- LiteLLM proxy health
- Environment configuration

**Skips:**
- Database (already validated at startup)

**Example:**
```go
providers := []string{"openai", "anthropic"}
err := validation.ValidateExecution(context.Background(), providers)
if err != nil {
    return fmt.Errorf("cannot execute plan: %w", err)
}
```

#### 4. Runtime Phase (`PhaseRuntime`)
Deferred validation when specific features are used.

**Use Cases:**
- Validate AWS credentials when Bedrock is first accessed
- Check file permissions when loading context
- Verify network connectivity to external services

## Components

### `errors.go` - Error Types and Formatting

Defines structured error types with rich metadata:

```go
type ValidationError struct {
    Category    ValidationErrorCategory // database, provider, environment, etc.
    Severity    ValidationErrorSeverity // critical, error, warning
    Summary     string                  // Short description
    Details     string                  // What went wrong
    Impact      string                  // Why it matters
    Solution    string                  // How to fix it
    Example     string                  // Example configuration
    RelatedVars []string               // Affected environment variables
    Err         error                   // Underlying error
}
```

**Error Categories:**
- `CategoryDatabase` - Database connection and configuration
- `CategoryProvider` - AI provider credentials and setup
- `CategoryEnvironment` - Environment variables and configuration
- `CategoryFilePath` - File paths and permissions
- `CategoryNetwork` - Network connectivity and services
- `CategoryPermission` - Access permissions
- `CategoryFormat` - Data format validation (JSON, etc.)

**Severity Levels:**
- `SeverityCritical` - Blocks startup (database, required services)
- `SeverityError` - Blocks execution (missing credentials, invalid config)
- `SeverityWarning` - Informational (conflicts, suboptimal configuration)

### `database.go` - Database Validation

Validates database configuration and connectivity:

**Checks:**
- Environment variables (DATABASE_URL or DB_* variables)
- Incomplete configuration (partial DB_* variables)
- Connection string format
- Database accessibility and connectivity
- Query execution capability

**Example:**
```go
result := validation.ValidateDatabase(ctx)
if result.HasErrors() {
    // Handle validation errors
}
```

**Common Errors Detected:**
- Missing database configuration
- Incomplete DB_* variables
- Invalid URL format
- Connection refused
- Authentication failures
- Database doesn't exist

### `provider.go` - Provider Credential Validation

Validates AI provider credentials for all supported providers:

**Supported Providers:**
- OpenAI
- Anthropic
- OpenRouter
- Google AI Studio
- Google Vertex AI
- Azure OpenAI
- DeepSeek
- Perplexity
- AWS Bedrock

**Validation:**
- Required environment variables
- Optional variables (if set)
- File paths for credential files
- JSON format validation
- Base64-encoded credentials

**Example:**
```go
// Validate a specific provider
result := validation.ValidateProviderCredentials("openai", true)

// Validate all providers
result := validation.ValidateAllProviders(true)
```

### `environment.go` - Environment Variable Validation

Validates general environment configuration:

**Checks:**
- PORT format and range
- GOENV value
- Debug configuration (PLANDEX_DEBUG, PLANDEX_DEBUG_LEVEL)
- Trace file paths
- Conflicting variables
- LiteLLM proxy availability and health

**Example:**
```go
result := validation.ValidateEnvironment()
if result.HasWarnings() {
    // Log warnings but continue
}
```

### `validator.go` - Validation Orchestrator

Coordinates all validation checks with configurable options:

```go
type ValidationOptions struct {
    Phase           ValidationPhase
    CheckFileAccess bool
    Verbose         bool
    Timeout         time.Duration
    ProviderNames   []string
    SkipDatabase    bool
    SkipProvider    bool
    SkipEnvironment bool
    SkipLiteLLM     bool
}
```

**Example:**
```go
opts := validation.ValidationOptions{
    Phase:           validation.PhaseStartup,
    CheckFileAccess: false,  // Fast startup
    Verbose:         true,
    Timeout:         30 * time.Second,
    SkipProvider:    true,   // Defer to execution
}

validator := validation.NewValidator(opts)
result := validator.ValidateAll(ctx)
```

## Usage Examples

### Server Startup

```go
// In server/main.go
func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := validation.ValidateStartup(ctx); err != nil {
        log.Fatal("Startup validation failed: ", err)
    }

    // Continue with server startup...
}
```

### CLI Execution

```go
// In cli/lib/validation.go
func ValidateExecutionEnvironment(providerNames []string) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    opts := validation.ValidationOptions{
        Phase:           validation.PhaseExecution,
        CheckFileAccess: true,
        ProviderNames:   providerNames,
        SkipDatabase:    true,
    }

    validator := validation.NewValidator(opts)
    result := validator.ValidateAll(ctx)

    if result.HasErrors() {
        fmt.Println(validation.FormatResult(result, true))
        os.Exit(1)
    }
}
```

### Runtime Validation

```go
// Validate specific provider when needed
func useBedrockProvider() error {
    if err := validation.ValidateProvider(ctx, "aws-bedrock", true); err != nil {
        return fmt.Errorf("AWS Bedrock not configured: %w", err)
    }
    // Use provider...
}
```

## Error Message Format

Validation errors are formatted for maximum clarity:

```
🗄️ CRITICAL: Cannot connect to database

📋 Details:
  Database server is not accepting connections.

⚠️  Impact:
  Plandex server cannot start without a working database connection.

✅ Solution:
  The database server may not be running or is not accessible:
    1. Check if PostgreSQL is running:
       systemctl status postgresql  # Linux
       brew services list            # macOS
    2. Verify the host and port are correct
    3. Check firewall settings if connecting to a remote host

🔑 Related variables:
  DATABASE_URL

🐛 Underlying error:
  dial tcp 127.0.0.1:5432: connect: connection refused
```

### Format Components

- **Emoji Icon**: Visual category identifier
- **Severity Level**: CRITICAL, ERROR, or WARNING
- **Summary**: One-line description
- **Details**: What specifically went wrong
- **Impact**: Why this matters and what will fail
- **Solution**: Step-by-step fix instructions
- **Example**: (Optional) Working configuration example
- **Related Variables**: Environment variables involved
- **Underlying Error**: (Optional) Technical error details

## Integration Points

### 1. Server Main (`app/server/main.go`)

Validates before starting LiteLLM and database:

```go
// Pre-startup validation
if err := validateStartupConfiguration(ctx); err != nil {
    log.Fatal("Startup validation failed: ", err)
}

// Start services with better error handling
if err := model.EnsureLiteLLM(2); err != nil {
    log.Fatal(formatLiteLLMError(err))
}
```

### 2. CLI Commands

Validates before plan execution:

```go
// In cmd/build.go, cmd/tell.go, etc.
func executePlan() {
    // Validate environment before execution
    lib.ValidateExecutionEnvironment(providerNames)

    // Execute with validated configuration
    authVars := lib.MustVerifyAuthVars(integratedModels)
    // ...
}
```

### 3. Error Registry Integration

Validation errors can be stored in the error registry:

```go
if result.HasErrors() {
    for _, err := range result.Errors {
        shared.ReportError(ctx, shared.ErrorReport{
            Category:    string(err.Category),
            Message:     err.Summary,
            Details:     err.Details,
            Remediation: err.Solution,
        })
    }
}
```

## Configuration Best Practices

### 1. Startup Validation

Keep startup validation fast:
- Skip expensive file access checks
- Defer provider validation to execution
- Use reasonable timeouts (30s default)

### 2. Execution Validation

Be thorough before execution:
- Validate all required providers
- Check file access and permissions
- Verify network connectivity

### 3. Error Handling

Handle validation results appropriately:
```go
result := validator.ValidateAll(ctx)

// Always log warnings
if result.HasWarnings() {
    log.Println(validation.FormatResult(result, false))
}

// Exit on errors
if result.HasErrors() {
    fmt.Fprintln(os.Stderr, validation.FormatResult(result, true))
    os.Exit(1)
}
```

## Testing Validation

### Manual Testing

Test different failure scenarios:

```bash
# Test missing database config
unset DATABASE_URL DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME
./plandex server

# Test invalid database URL
export DATABASE_URL="invalid://url"
./plandex server

# Test missing provider credentials
unset OPENAI_API_KEY ANTHROPIC_API_KEY
plandex tell "build a web app"

# Test malformed JSON credentials
export GOOGLE_APPLICATION_CREDENTIALS='{"invalid": json'
plandex build
```

### Automated Testing

```go
func TestDatabaseValidation(t *testing.T) {
    // Test missing configuration
    os.Unsetenv("DATABASE_URL")
    result := validation.ValidateDatabase(context.Background())
    if !result.HasErrors() {
        t.Error("Expected error for missing database config")
    }

    // Verify error message quality
    if len(result.Errors) > 0 {
        err := result.Errors[0]
        if err.Solution == "" {
            t.Error("Error missing solution")
        }
    }
}
```

## Future Enhancements

Potential improvements to the validation system:

1. **Dry-Run Mode**: Validate without actually starting services
2. **Config File Validation**: Validate YAML/JSON config files
3. **Network Tests**: Test connectivity to external APIs
4. **Performance Validation**: Check system resources (memory, disk)
5. **Compatibility Checks**: Verify versions and dependencies
6. **Automated Fixes**: Suggest or apply automatic fixes for common issues
7. **Validation Reports**: Export validation results to JSON/HTML

## See Also

- [VALIDATION_EXAMPLES.md](../../../docs/VALIDATION_EXAMPLES.md) - Detailed examples of common failures
- [Error Reporting System](../error_report.go) - Error reporting and diagnostics
- [Model Providers](../ai_models_providers.go) - Provider configuration schemas

## Contributing

When adding new validation checks:

1. Use appropriate error categories and severity
2. Provide specific, actionable solutions
3. Include examples of correct configuration
4. List all related environment variables
5. Test with real failure scenarios
6. Update VALIDATION_EXAMPLES.md with examples
