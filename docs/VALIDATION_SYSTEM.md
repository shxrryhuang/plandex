# Plandex Configuration Validation System

## Overview

Plandex now includes a comprehensive configuration validation system that catches misconfigurations early, before execution begins. This system provides clear, actionable error messages to help users quickly diagnose and fix issues.

## Problem Statement

Previously, configuration errors would only surface deep into execution:
- Missing API tokens discovered when first using a provider
- Invalid database credentials causing startup panics
- File paths resolved at runtime, failing after time invested
- Cryptic stack traces instead of helpful guidance

## Solution

The validation system introduces an explicit validation phase that runs before execution:

### Key Features

1. **Early Detection**: Validates configuration before services start
2. **Phased Validation**:
   - Synchronous checks at startup (database, environment)
   - Deferred checks when features are used (provider credentials)
3. **Clear Error Messages**: Structured errors with solutions and examples
4. **Multiple Integration Points**: Server startup, CLI execution, runtime
5. **Performance Optimized**: Fast startup checks, thorough execution checks

## Architecture

### Validation Phases

```
┌─────────────────────────────────────────────────────────────┐
│                      Validation Phases                       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Phase 1: STARTUP (Synchronous)                             │
│  ├─ Database connectivity                                   │
│  ├─ Environment variables                                   │
│  └─ Port availability                                       │
│                                                              │
│  Phase 2: EXECUTION (Before plan execution)                 │
│  ├─ Provider credentials                                    │
│  ├─ File paths and permissions                              │
│  └─ LiteLLM proxy health                                    │
│                                                              │
│  Phase 3: RUNTIME (Deferred)                                │
│  └─ Feature-specific checks when accessed                   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Package Structure

```
app/shared/validation/
├── errors.go          # Error types and formatting
├── database.go        # Database validation
├── provider.go        # Provider credential validation
├── environment.go     # Environment variable validation
├── validator.go       # Validation orchestrator
├── validator_test.go  # Comprehensive tests
└── README.md          # Package documentation
```

## Implementation Details

### 1. Error Types (`errors.go`)

Structured errors with rich context:

```go
type ValidationError struct {
    Category    ValidationErrorCategory  // What failed
    Severity    ValidationErrorSeverity  // How critical
    Summary     string                   // Quick description
    Details     string                   // What went wrong
    Impact      string                   // Why it matters
    Solution    string                   // How to fix
    Example     string                   // Working configuration
    RelatedVars []string                 // Affected variables
    Err         error                    // Underlying error
}
```

**Categories**: Database, Provider, Environment, FilePath, Network, Permission, Format

**Severities**: Critical (blocks startup), Error (blocks execution), Warning (informational)

### 2. Database Validation (`database.go`)

Validates database configuration and connectivity:

```go
// Checks performed:
// - Environment variables present
// - Configuration completeness
// - Connection string format
// - Database connectivity
// - Query execution capability

result := validation.ValidateDatabase(ctx)
```

**Common Errors Detected**:
- No configuration set
- Incomplete DB_* variables
- Invalid URL format
- Connection refused
- Authentication failed
- Database doesn't exist
- Too many connections

### 3. Provider Validation (`provider.go`)

Validates credentials for all AI providers:

```go
// Supported providers:
// OpenAI, Anthropic, OpenRouter, Google AI Studio,
// Google Vertex AI, Azure OpenAI, DeepSeek,
// Perplexity, AWS Bedrock

result := validation.ValidateProviderCredentials("openai", true)
```

**Validation Includes**:
- Required environment variables
- Optional variables (if set)
- File path existence and readability
- JSON format validation
- Base64-encoded credentials

### 4. Environment Validation (`environment.go`)

Validates general environment configuration:

```go
// Checks:
// - PORT format and range
// - GOENV value
// - Debug configuration
// - Conflicting variables
// - LiteLLM proxy availability

result := validation.ValidateEnvironment()
```

### 5. Validation Orchestrator (`validator.go`)

Coordinates all validation checks:

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

## Integration Points

### Server Startup (`app/server/main.go`)

```go
func main() {
    log.Println("Starting Plandex server...")

    // Run startup validation
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := validateStartupConfiguration(ctx); err != nil {
        log.Fatal("Startup validation failed: ", err)
    }
    log.Println("✅ Startup validation passed")

    // Start LiteLLM with better error handling
    if err := model.EnsureLiteLLM(2); err != nil {
        log.Fatal(formatLiteLLMError(err))
    }

    // Continue startup...
}
```

**What's Validated at Startup**:
- Database configuration and connectivity
- Environment variable format
- Port availability (LiteLLM port 4000)
- Configuration conflicts

**What's Deferred**:
- Provider credentials (checked at execution time)
- File access (performance optimization)

### CLI Execution (`app/cli/lib/validation.go`)

```go
// Before plan execution
func ValidateExecutionEnvironment(providerNames []string) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    opts := validation.ValidationOptions{
        Phase:           validation.PhaseExecution,
        CheckFileAccess: true,
        ProviderNames:   providerNames,
        SkipDatabase:    true,  // Already validated
    }

    validator := validation.NewValidator(opts)
    result := validator.ValidateAll(ctx)

    if result.HasErrors() {
        fmt.Println(validation.FormatResult(result, true))
        term.OutputErrorAndExit("Configuration validation failed.")
    }
}
```

**What's Validated Before Execution**:
- Provider credentials for models being used
- File paths and permissions
- LiteLLM proxy health
- Environment configuration

### Error Registry Integration

Validation errors integrate with Plandex's existing error reporting:

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

## Error Message Format

Validation errors use a consistent, helpful format:

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

- **📋 Details**: Specific information about what failed
- **⚠️ Impact**: Why this matters and what will fail
- **✅ Solution**: Step-by-step instructions to fix
- **💡 Example**: (Optional) Working configuration example
- **🔑 Related Variables**: Environment variables involved
- **🐛 Underlying Error**: (Optional) Technical details

## Usage Examples

### Example 1: Successful Startup

```bash
$ plandex server
Starting Plandex server...
Running pre-startup validation checks...
✅ Startup validation passed
Starting LiteLLM proxy...
✅ LiteLLM proxy started successfully
Connecting to database...
✅ Database connection established
Running database migrations...
migration state is up to date
✅ Database initialization complete
Started Plandex server on port 8099
```

### Example 2: Missing Database Configuration

```bash
$ plandex server
Starting Plandex server...
Running pre-startup validation checks...
❌ Configuration validation failed
================================================================================

🗄️ CRITICAL: No database configuration found

📋 Details:
  Neither DATABASE_URL nor individual DB_* environment variables are set.

⚠️  Impact:
  Plandex server cannot start without a database connection.

✅ Solution:
  Set either DATABASE_URL or all of DB_HOST, DB_PORT, DB_USER,
  DB_PASSWORD, and DB_NAME.

💡 Example:
  Option 1: Using DATABASE_URL
    export DATABASE_URL="postgres://user:password@localhost:5432/plandex"

  Option 2: Using individual variables
    export DB_HOST="localhost"
    export DB_PORT="5432"
    export DB_USER="plandex_user"
    export DB_PASSWORD="secure_password"
    export DB_NAME="plandex"

🔑 Related variables:
  DATABASE_URL, DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME

================================================================================
Found 1 error(s)
Please fix the errors above before continuing.

FATAL: Startup validation failed: configuration validation failed with 1 error(s)
```

### Example 3: Missing Provider Credentials

```bash
$ plandex tell "build a web app"

🔌 ERROR: No provider credentials configured

📋 Details:
  No valid credentials found for any AI model provider.

⚠️  Impact:
  Cannot execute plans without at least one configured provider.

✅ Solution:
  Configure credentials for at least one provider:

  Quick start with OpenRouter (supports multiple models):
    1. Sign up at https://openrouter.ai
    2. Generate an API key
    3. Set: export OPENROUTER_API_KEY=your_key

  Or configure a specific provider:
    - OpenAI: export OPENAI_API_KEY=your_key
    - Anthropic: export ANTHROPIC_API_KEY=your_key
    - Google: export GEMINI_API_KEY=your_key

  See https://docs.plandex.ai/models/model-providers for full details.

Configuration validation failed. Please fix the errors above before continuing.
```

### Example 4: Configuration Warnings

```bash
$ plandex server
Starting Plandex server...
Running pre-startup validation checks...

⚠️  Configuration warnings
================================================================================

⚙️ WARNING: Both DATABASE_URL and DB_* variables are set

📋 Details:
  You have both DATABASE_URL and individual DB_* variables configured.
  DATABASE_URL will take precedence.

⚠️  Impact:
  DB_* variables will be ignored, which may be confusing.

✅ Solution:
  Use either DATABASE_URL or DB_* variables, but not both.
  Remove the unused configuration.

================================================================================

✅ Startup validation passed (with warnings)
# Server continues starting...
```

## Performance Considerations

### Startup Validation (Fast)

- Database: Connection test only (~100ms)
- Environment: Format validation only (~1ms)
- Port checks: Quick TCP dial (~10ms)
- **Total**: ~100-200ms overhead

**Optimizations**:
- Skip file access checks
- Defer provider validation
- Use short timeouts

### Execution Validation (Thorough)

- Providers: Credential validation (~50ms per provider)
- Files: Existence and readability checks (~10ms per file)
- LiteLLM: Health check (~100ms)
- **Total**: ~200-500ms overhead

**Trade-off**: Slightly longer validation time prevents wasted execution time

## Testing

### Unit Tests

Comprehensive test coverage in `validator_test.go`:

```bash
# Run all validation tests
go test ./app/shared/validation/...

# Run specific test
go test ./app/shared/validation/ -run TestDatabaseValidation

# Run with verbose output
go test -v ./app/shared/validation/...

# Run benchmarks
go test -bench=. ./app/shared/validation/...
```

### Manual Testing

Test different failure scenarios:

```bash
# Test missing database config
unset DATABASE_URL DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME
plandex server

# Test invalid database URL
export DATABASE_URL="invalid://url"
plandex server

# Test missing provider credentials
unset OPENAI_API_KEY ANTHROPIC_API_KEY
plandex tell "build a web app"

# Test malformed JSON credentials
export GOOGLE_APPLICATION_CREDENTIALS='{"invalid": json'
plandex build
```

## Monitoring and Debugging

### Validation Logs

Validation results are logged to server logs:

```bash
# View validation in logs
tail -f /path/to/plandex.log | grep validation

# Check for validation errors
grep "validation failed" /path/to/plandex.log
```

### Error Registry

Validation errors are stored in the error registry:

```bash
# View error registry
cat ~/.plandex/errors.json | jq .

# Filter for validation errors
cat ~/.plandex/errors.json | jq '.errors[] | select(.category | contains("validation"))'
```

### Debug Mode

Enable debug mode for detailed validation traces:

```bash
export PLANDEX_DEBUG=1
export PLANDEX_DEBUG_LEVEL=debug
export PLANDEX_TRACE_FILE=~/plandex-trace.log

plandex server
```

## Common Issues and Solutions

### Issue: Validation Too Slow

**Solution**: Adjust validation options

```go
opts := validation.ValidationOptions{
    CheckFileAccess: false,  // Skip file checks
    Timeout:         5 * time.Second,  // Shorter timeout
    SkipProvider:    true,   // Defer provider checks
}
```

### Issue: False Positives

**Solution**: Review validation logic and add exceptions

```go
// Skip validation for local-only providers
if provider.LocalOnly {
    return nil  // No validation needed
}
```

### Issue: Missing Validation

**Solution**: Add new validation checks

```go
// Add validation for new configuration
func ValidateNewFeature() *ValidationResult {
    result := &ValidationResult{}

    // Check configuration
    if value := os.Getenv("NEW_FEATURE_VAR"); value == "" {
        result.AddError(NewEnvironmentError(
            "Missing NEW_FEATURE_VAR",
            "Feature requires NEW_FEATURE_VAR to be set",
            "Set: export NEW_FEATURE_VAR=value",
            "export NEW_FEATURE_VAR=\"example\"",
            []string{"NEW_FEATURE_VAR"},
        ))
    }

    return result
}
```

## Future Enhancements

Potential improvements to the validation system:

1. **Dry-Run Mode**: Validate without starting services
2. **Config File Validation**: Validate YAML/JSON config files
3. **Network Tests**: Test connectivity to external APIs
4. **Performance Validation**: Check system resources
5. **Compatibility Checks**: Verify versions and dependencies
6. **Automated Fixes**: Apply automatic fixes for common issues
7. **Validation Reports**: Export results to JSON/HTML
8. **Interactive Validation**: Guide users through fixing issues
9. **Pre-commit Validation**: Validate config before git commit
10. **Health Dashboard**: Web UI showing validation status

## Documentation

- **[VALIDATION_EXAMPLES.md](VALIDATION_EXAMPLES.md)**: Detailed examples of common failures
- **[validation/README.md](../app/shared/validation/README.md)**: Package documentation
- **[validation/validator_test.go](../app/shared/validation/validator_test.go)**: Test examples

## Contributing

When adding new validation checks:

1. **Use appropriate error categories and severity**
2. **Provide specific, actionable solutions**
3. **Include examples of correct configuration**
4. **List all related environment variables**
5. **Test with real failure scenarios**
6. **Update documentation with examples**
7. **Add unit tests for new validations**
8. **Consider performance impact**

## Benefits

### For Users

- **Faster troubleshooting**: Clear error messages with solutions
- **Less frustration**: Catch issues before wasting time
- **Better onboarding**: Helpful guidance for new users
- **Confidence**: Know configuration is correct before execution

### For Developers

- **Better debugging**: Structured error information
- **Easier maintenance**: Consistent error handling
- **Improved testing**: Validation as part of test suite
- **Reduced support load**: Users can self-diagnose issues

## Conclusion

The Plandex configuration validation system transforms error handling from reactive to proactive. By catching issues early and providing clear guidance, it significantly improves the user experience and reduces time spent debugging configuration problems.

The system is designed to be:
- **Fast**: Minimal startup overhead
- **Thorough**: Comprehensive checks when needed
- **Clear**: Actionable error messages
- **Extensible**: Easy to add new validations
- **Integrated**: Works with existing error reporting

This validation system ensures users can focus on building with Plandex, not debugging configuration issues.
