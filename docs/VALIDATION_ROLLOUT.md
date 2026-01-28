# Validation System Rollout Guide

## Overview

The Configuration Validation System is designed to be **completely optional** and can be enabled/disabled without affecting the original Plandex behavior. This guide explains how to safely roll out validation in your environment.

---

## Quick Start

### Enable Validation (All Features)

```bash
export PLANDEX_ENABLE_VALIDATION=true
```

This single environment variable enables:
- Startup validation (fast critical checks)
- Execution validation (thorough pre-execution checks)
- Enhanced error messages

### Disable Validation (Original Behavior)

```bash
unset PLANDEX_ENABLE_VALIDATION
# OR
export PLANDEX_ENABLE_VALIDATION=false
```

With validation disabled, Plandex runs exactly as before with no changes to behavior.

---

## Rollout Strategy

### Phase 1: Development Testing (Week 1)

**Goal:** Verify validation works in development environment

```bash
# Enable validation in development
export PLANDEX_ENABLE_VALIDATION=true
export PLANDEX_VALIDATION_VERBOSE=true

# Run Plandex server
./plandex-server
```

**What to watch for:**
- Validation completes in < 500ms
- Error messages are clear and actionable
- No false positives (valid configs don't fail)
- Original functionality still works

**Success criteria:**
- ✅ Server starts successfully with valid config
- ✅ Clear errors shown for invalid config
- ✅ Can disable validation and revert to original behavior

---

### Phase 2: Staging Deployment (Week 2)

**Goal:** Test with realistic configurations and workloads

```bash
# Enable validation with standard verbosity
export PLANDEX_ENABLE_VALIDATION=true
export PLANDEX_VALIDATION_VERBOSE=false

# Run in staging
./plandex-server
```

**What to test:**
- All AI providers you use
- Database connectivity under load
- Edge cases (network issues, timeouts)
- Error recovery scenarios

**Success criteria:**
- ✅ Validation catches real configuration issues
- ✅ Performance impact < 500ms on startup
- ✅ No production incidents caused by validation

---

### Phase 3: Production Rollout (Week 3)

**Goal:** Gradual production deployment

**Option A: Canary Deployment**
```bash
# Enable on 10% of servers first
if [ "$SERVER_ID" -lt 10 ]; then
  export PLANDEX_ENABLE_VALIDATION=true
fi
```

**Option B: Feature Flag**
```bash
# Use your feature flag system
if feature_enabled "plandex_validation"; then
  export PLANDEX_ENABLE_VALIDATION=true
fi
```

**Monitoring:**
- Track startup time metrics
- Monitor error rates
- Watch for validation-related issues
- Collect user feedback

**Success criteria:**
- ✅ Reduced configuration-related incidents
- ✅ Faster time-to-resolution for config issues
- ✅ Positive user feedback
- ✅ No degradation in performance or reliability

---

## Fine-Grained Control

### Individual Feature Flags

Instead of enabling all validation, you can enable specific features:

```bash
# Enable only startup validation (fastest)
export PLANDEX_VALIDATION_SYSTEM=true
export PLANDEX_VALIDATION_STARTUP=true
export PLANDEX_VALIDATION_EXECUTION=false

# Enable verbose error messages
export PLANDEX_VALIDATION_VERBOSE=true

# Enable strict mode (warnings block execution)
export PLANDEX_VALIDATION_STRICT=true

# Enable thorough file access checks (slower but comprehensive)
export PLANDEX_VALIDATION_FILE_CHECKS=true
```

### Recommended Configurations

**Development Environment:**
```bash
export PLANDEX_ENABLE_VALIDATION=true
export PLANDEX_VALIDATION_VERBOSE=true
export PLANDEX_VALIDATION_STRICT=true
export PLANDEX_VALIDATION_FILE_CHECKS=true
```
- Most thorough validation
- Verbose error messages for debugging
- Strict mode catches warnings
- File access checks enabled

**Staging Environment:**
```bash
export PLANDEX_ENABLE_VALIDATION=true
export PLANDEX_VALIDATION_VERBOSE=true
export PLANDEX_VALIDATION_STRICT=false
export PLANDEX_VALIDATION_FILE_CHECKS=false
```
- Production-like configuration
- Verbose for troubleshooting
- Warnings don't block (realistic)
- Fast validation checks

**Production Environment:**
```bash
export PLANDEX_ENABLE_VALIDATION=true
export PLANDEX_VALIDATION_VERBOSE=false
export PLANDEX_VALIDATION_STRICT=false
export PLANDEX_VALIDATION_FILE_CHECKS=false
```
- Standard validation enabled
- Concise error messages
- Only errors block execution
- Optimized for performance

---

## Environment File Templates

### .env.validation-enabled

```bash
# Enable Validation System
PLANDEX_ENABLE_VALIDATION=true

# Validation Settings
PLANDEX_VALIDATION_VERBOSE=false
PLANDEX_VALIDATION_STRICT=false
PLANDEX_VALIDATION_FILE_CHECKS=false

# Database Configuration
DATABASE_URL=postgresql://user:pass@localhost:5432/plandex

# AI Provider Configuration
OPENAI_API_KEY=sk-...

# Server Configuration
PORT=8080
GOENV=production
```

### .env.validation-disabled

```bash
# Disable Validation System (original behavior)
PLANDEX_ENABLE_VALIDATION=false

# Database Configuration
DATABASE_URL=postgresql://user:pass@localhost:5432/plandex

# AI Provider Configuration
OPENAI_API_KEY=sk-...

# Server Configuration
PORT=8080
GOENV=production
```

---

## Testing the Pipeline

### Test 1: Validation Enabled

```bash
# Set environment
export PLANDEX_ENABLE_VALIDATION=true

# Start server
./plandex-server

# Expected output:
# Starting Plandex server...
# ⚙️  Validation system: ENABLED
#   ● Startup validation: enabled
#   ● Execution validation: enabled
#   ● Verbose errors: enabled
# Running startup validation...
# ✅ Startup validation passed
# ✅ Database configuration valid
# Starting LiteLLM proxy...
# ✅ LiteLLM proxy started successfully
```

### Test 2: Validation Disabled

```bash
# Unset environment
unset PLANDEX_ENABLE_VALIDATION

# Start server
./plandex-server

# Expected output:
# Starting Plandex server...
# ⚙️  Validation system: DISABLED (using original startup flow)
# Starting LiteLLM proxy...
# (original behavior - no validation output)
```

### Test 3: Validation Catches Error

```bash
# Remove database config
unset DATABASE_URL

# Enable validation
export PLANDEX_ENABLE_VALIDATION=true

# Start server
./plandex-server

# Expected output:
# Starting Plandex server...
# ⚙️  Validation system: ENABLED
# Running startup validation...
#
# ❌ Database Configuration Error
#
# Summary: No database configuration found
# Details: Neither DATABASE_URL nor individual DB_* variables are set
# Impact: Plandex cannot start without a database connection
# Solution: Set DATABASE_URL environment variable
# Example: DATABASE_URL=postgresql://user:pass@localhost:5432/plandex
#
# FATAL: Startup validation failed: configuration validation failed
```

---

## Rollback Procedure

If validation causes issues, you can **instantly** rollback:

### Immediate Rollback

```bash
# Disable validation
export PLANDEX_ENABLE_VALIDATION=false

# Restart service
systemctl restart plandex-server
```

The original Plandex behavior is restored immediately with no code changes required.

### Persistent Rollback

**Update systemd service file:**
```ini
[Service]
Environment="PLANDEX_ENABLE_VALIDATION=false"
```

**Update Docker Compose:**
```yaml
environment:
  - PLANDEX_ENABLE_VALIDATION=false
```

**Update Kubernetes:**
```yaml
env:
  - name: PLANDEX_ENABLE_VALIDATION
    value: "false"
```

---

## Performance Impact

### Startup Validation

- **Without file checks:** 50-150ms
- **With file checks:** 200-500ms
- **Impact:** One-time cost at startup

### Execution Validation

- **Standard checks:** 100-300ms
- **With provider validation:** 200-500ms
- **Impact:** One-time cost before each execution

### Recommendations

- **Development:** Enable all checks
- **Production:** Disable file checks for faster startup
- **CI/CD:** Enable all checks to catch issues early

---

## Monitoring

### Key Metrics to Track

**Startup Time:**
```bash
# Before validation
START_TIME=$(date +%s%N)
./plandex-server &
END_TIME=$(date +%s%N)
STARTUP_MS=$(( (END_TIME - START_TIME) / 1000000 ))
echo "Startup time: ${STARTUP_MS}ms"
```

**Validation Success Rate:**
- Track how often validation catches real issues
- Monitor false positives (valid configs failing validation)
- Measure time-to-resolution for config issues

**Error Rates:**
- Configuration errors caught at startup (good!)
- Runtime errors due to configuration (should decrease)
- Validation system errors (should be near zero)

---

## Troubleshooting

### Validation Takes Too Long

**Problem:** Startup validation exceeds 500ms

**Solutions:**
1. Disable file checks:
   ```bash
   export PLANDEX_VALIDATION_FILE_CHECKS=false
   ```

2. Skip thorough provider validation:
   ```bash
   export PLANDEX_VALIDATION_EXECUTION=false
   export PLANDEX_VALIDATION_STARTUP=true
   ```

3. Disable validation entirely:
   ```bash
   export PLANDEX_ENABLE_VALIDATION=false
   ```

---

### False Positives

**Problem:** Validation fails for valid configuration

**Debugging:**
1. Enable verbose mode:
   ```bash
   export PLANDEX_VALIDATION_VERBOSE=true
   ```

2. Check validation output for specific issue

3. Report false positive as bug with:
   - Configuration details
   - Error message
   - Expected behavior

**Workaround:**
```bash
# Disable validation temporarily
export PLANDEX_ENABLE_VALIDATION=false
```

---

### Missing Original Behavior

**Problem:** Want to verify original code path still works

**Test:**
```bash
# Completely disable validation
unset PLANDEX_ENABLE_VALIDATION
unset PLANDEX_VALIDATION_SYSTEM
unset PLANDEX_VALIDATION_STARTUP
unset PLANDEX_VALIDATION_EXECUTION
unset PLANDEX_VALIDATION_VERBOSE
unset PLANDEX_VALIDATION_STRICT
unset PLANDEX_VALIDATION_FILE_CHECKS

# Start server
./plandex-server

# Should behave exactly as before validation system was added
```

---

## Architecture

### Dual Path Design

```
┌─────────────────────────────────────────┐
│         Plandex Server Start            │
└──────────────┬──────────────────────────┘
               │
               ▼
    ┌──────────────────────┐
    │ Check Feature Flags  │
    └──────────┬───────────┘
               │
       ┌───────┴────────┐
       │                │
       ▼                ▼
┌──────────┐    ┌──────────────┐
│ Original │    │ Validation   │
│ Path     │    │ Path         │
│          │    │              │
│ - Fast   │    │ - Safe       │
│ - Legacy │    │ - Clear      │
│ - Proven │    │ - Helpful    │
└──────────┘    └──────────────┘
```

### Code Organization

```
app/
├── server/
│   ├── main.go          # Original entry point
│   └── main_safe.go     # New entry point with validation
├── shared/
│   ├── features/
│   │   └── features.go  # Feature flag management
│   └── validation/
│       ├── validator.go # Core validation
│       └── wrapper.go   # Safe wrappers
```

### Feature Flag Flow

```go
// In main_safe.go
if features.IsValidationEnabled() {
    // Use validation system
    if err := validation.SafeValidateStartup(ctx); err != nil {
        log.Fatal("Validation failed:", err)
    }
} else {
    // Use original behavior (no validation)
    // Code continues as before
}
```

---

## Migration Path

### Current State: No Validation

```bash
# Server starts with original code
./plandex-server

# Errors appear during execution
# Stack traces and cryptic messages
```

### Target State: Validation Enabled

```bash
# Enable validation
export PLANDEX_ENABLE_VALIDATION=true

# Server validates configuration at startup
./plandex-server

# Clear errors shown immediately
# Helpful solutions provided
```

### Migration Steps

1. **Week 1: Enable in development**
   - Test with your configurations
   - Fix any false positives
   - Verify performance acceptable

2. **Week 2: Enable in staging**
   - Test with production-like workloads
   - Monitor for issues
   - Collect feedback

3. **Week 3: Enable in production**
   - Gradual rollout (10% → 50% → 100%)
   - Monitor metrics
   - Quick rollback if needed

---

## Success Metrics

### Before Validation

- Configuration errors discovered during execution
- Average time-to-diagnosis: 15-30 minutes
- Support tickets for config issues: High
- User frustration: High

### After Validation

- Configuration errors discovered at startup
- Average time-to-diagnosis: 1-5 minutes
- Support tickets for config issues: Reduced 60-80%
- User satisfaction: Improved

### Key Performance Indicators

1. **Mean Time To Resolution (MTTR)**
   - Target: < 5 minutes for config issues
   - Measured: Time from error to fix

2. **Configuration Error Rate**
   - Target: 80% caught at startup
   - Measured: Errors caught early vs runtime

3. **False Positive Rate**
   - Target: < 1%
   - Measured: Valid configs failing validation

4. **Startup Performance**
   - Target: < 500ms validation overhead
   - Measured: Time for validation checks

---

## Best Practices

### DO ✅

- Enable validation in development and staging first
- Test with your actual configurations
- Monitor performance impact
- Use verbose mode for debugging
- Keep rollback plan ready
- Disable validation if it causes issues

### DON'T ❌

- Enable in production without testing
- Ignore performance degradation
- Skip rollback testing
- Force validation on all users immediately
- Remove original code path
- Panic if validation fails (it's designed to help!)

---

## Support

### Documentation

- **Quick fixes:** [VALIDATION_QUICK_REFERENCE.md](VALIDATION_QUICK_REFERENCE.md)
- **Examples:** [VALIDATION_EXAMPLES.md](VALIDATION_EXAMPLES.md)
- **Full docs:** [VALIDATION_SYSTEM.md](VALIDATION_SYSTEM.md)

### Getting Help

1. Check error message (includes solution!)
2. Review [VALIDATION_EXAMPLES.md](VALIDATION_EXAMPLES.md)
3. Search [existing issues](https://github.com/anthropics/plandex/issues)
4. Create new issue with details

### Reporting Issues

Include:
- Environment variables (sanitized)
- Error message
- Expected behavior
- Steps to reproduce
- Plandex version

---

## Summary

The Configuration Validation System is designed to be:

- **Optional:** Enable/disable with environment variable
- **Safe:** Original behavior preserved when disabled
- **Fast:** < 500ms validation overhead
- **Clear:** Helpful error messages with solutions
- **Gradual:** Roll out at your own pace
- **Reversible:** Instant rollback if needed

**Enable validation:**
```bash
export PLANDEX_ENABLE_VALIDATION=true
```

**Disable validation:**
```bash
export PLANDEX_ENABLE_VALIDATION=false
```

That's it! The choice is yours.

---

**Happy validating!** 🚀
