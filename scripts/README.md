# Plandex Validation System - Helper Scripts

This directory contains helper scripts for managing the Configuration Validation System.

---

## Quick Start

```bash
# Enable validation with development settings
./scripts/enable-validation.sh enable-dev

# Check status
./scripts/enable-validation.sh status

# Test configuration
./scripts/enable-validation.sh test

# Verify both paths work
./scripts/test-dual-path.sh

# Disable validation
./scripts/enable-validation.sh disable
```

---

## Scripts Overview

### enable-validation.sh

**Purpose:** Enable/disable validation system and manage configuration profiles.

**Usage:**
```bash
./scripts/enable-validation.sh [COMMAND]
```

**Commands:**

| Command | Description |
|---------|-------------|
| `enable` | Enable validation (standard profile) |
| `enable-dev` | Enable with development settings (verbose, strict, thorough) |
| `enable-staging` | Enable with staging settings (verbose, non-strict, fast) |
| `enable-prod` | Enable with production settings (concise, non-strict, fast) |
| `disable` | Disable validation (original behavior) |
| `rollback` | Alias for disable |
| `test` | Test validation with current settings |
| `status` | Show current validation status |
| `help` | Show help message |

**Examples:**

```bash
# Enable validation for development
./scripts/enable-validation.sh enable-dev
# Output:
# ✅ Validation system enabled
# Profile: Development (verbose, strict, thorough)
# Configuration saved to: .env.validation

# Check what's enabled
./scripts/enable-validation.sh status
# Output:
# ● Master switch: enabled
# ● Verbose errors: enabled
# ● Strict mode: enabled
# ● File checks: enabled

# Test configuration
./scripts/enable-validation.sh test
# Output: Server starts with validation enabled

# Disable validation
./scripts/enable-validation.sh disable
# Output:
# ✅ Validation system disabled (original behavior restored)
```

---

### test-dual-path.sh

**Purpose:** Verify both validation and original code paths work correctly.

**Usage:**
```bash
./scripts/test-dual-path.sh
```

**What it tests:**

1. Validation package compiles
2. Features package compiles
3. Server compiles with validation disabled
4. Server compiles with validation enabled
5. Feature flags work correctly
6. Validation tests pass
7. Safe wrappers work correctly

**Example output:**

```bash
$ ./scripts/test-dual-path.sh

═══════════════════════════════════════════════════════════════
  ⚙️  Plandex Dual-Path Verification
═══════════════════════════════════════════════════════════════

Test: Validation package compiles
✅ PASSED: Validation package compiles

Test: Features package compiles
✅ PASSED: Features package compiles

Test: Server compiles (validation disabled)
✅ PASSED: Server compiles (validation disabled)

Test: Server compiles (validation enabled)
✅ PASSED: Server compiles (validation enabled)

Test: Feature flags work correctly
✅ PASSED: Feature flags work correctly

Test: Validation tests pass
✅ PASSED: Validation tests pass

Test: Safe wrappers work correctly
✅ PASSED: Safe wrappers work correctly

═══════════════════════════════════════════════════════════════
  Test Results
═══════════════════════════════════════════════════════════════

Total tests: 7
✅ Passed: 7

✅ All tests passed! Both paths working correctly.

You can safely:
  • Enable validation: ./scripts/enable-validation.sh enable
  • Disable validation: ./scripts/enable-validation.sh disable
  • Check status: ./scripts/enable-validation.sh status
```

---

## Configuration Profiles

### Development Profile

**Enable:**
```bash
./scripts/enable-validation.sh enable-dev
```

**Settings:**
```bash
PLANDEX_ENABLE_VALIDATION=true
PLANDEX_VALIDATION_VERBOSE=true
PLANDEX_VALIDATION_STRICT=true
PLANDEX_VALIDATION_FILE_CHECKS=true
```

**Best for:**
- Local development
- Catching all issues
- Debugging configuration
- Testing new features

---

### Staging Profile

**Enable:**
```bash
./scripts/enable-validation.sh enable-staging
```

**Settings:**
```bash
PLANDEX_ENABLE_VALIDATION=true
PLANDEX_VALIDATION_VERBOSE=true
PLANDEX_VALIDATION_STRICT=false
PLANDEX_VALIDATION_FILE_CHECKS=false
```

**Best for:**
- Pre-production testing
- Production-like environment
- Performance testing
- Integration testing

---

### Production Profile

**Enable:**
```bash
./scripts/enable-validation.sh enable-prod
```

**Settings:**
```bash
PLANDEX_ENABLE_VALIDATION=true
PLANDEX_VALIDATION_VERBOSE=false
PLANDEX_VALIDATION_STRICT=false
PLANDEX_VALIDATION_FILE_CHECKS=false
```

**Best for:**
- Production deployment
- Minimal overhead
- Clear but concise errors
- Optimal performance

---

## Common Workflows

### Workflow 1: First Time Setup

```bash
# 1. Enable validation in development
./scripts/enable-validation.sh enable-dev

# 2. Test it works
./scripts/enable-validation.sh test

# 3. Check status
./scripts/enable-validation.sh status

# 4. If issues, disable and investigate
./scripts/enable-validation.sh disable
```

---

### Workflow 2: Deploy to Staging

```bash
# 1. Verify both paths work
./scripts/test-dual-path.sh

# 2. Enable staging profile
./scripts/enable-validation.sh enable-staging

# 3. Make permanent in staging environment
cat .env.validation >> /etc/plandex/environment

# 4. Restart service
systemctl restart plandex-server

# 5. Monitor logs
journalctl -u plandex-server -f
```

---

### Workflow 3: Production Rollout

```bash
# 1. Verify everything works in staging
./scripts/enable-validation.sh status

# 2. Enable production profile
./scripts/enable-validation.sh enable-prod

# 3. Deploy to canary servers (10%)
# ... deployment process ...

# 4. Monitor for issues
# ... monitoring ...

# 5. Gradual rollout (50%, then 100%)
# ... deployment process ...
```

---

### Workflow 4: Rollback

```bash
# 1. Disable validation immediately
./scripts/enable-validation.sh disable

# 2. Restart service
systemctl restart plandex-server

# 3. Verify original behavior restored
./scripts/enable-validation.sh status
# Should show: "Validation System: DISABLED"
```

---

### Workflow 5: Troubleshooting

```bash
# 1. Check current status
./scripts/enable-validation.sh status

# 2. Enable verbose mode
export PLANDEX_VALIDATION_VERBOSE=true

# 3. Test configuration
./scripts/enable-validation.sh test

# 4. Review error messages
# Errors include solutions!

# 5. Fix issues and retest
./scripts/enable-validation.sh test
```

---

## Environment Files

The scripts create `.env.validation` file with current configuration.

**Example .env.validation (development):**
```bash
# Plandex Validation System - Development Profile
PLANDEX_ENABLE_VALIDATION=true
PLANDEX_VALIDATION_VERBOSE=true
PLANDEX_VALIDATION_STRICT=true
PLANDEX_VALIDATION_FILE_CHECKS=true
```

**To use in your shell:**
```bash
# Load into current shell
source .env.validation

# Or export manually
export PLANDEX_ENABLE_VALIDATION=true
export PLANDEX_VALIDATION_VERBOSE=true
```

**To make permanent:**
```bash
# Add to ~/.bashrc or ~/.zshrc
cat .env.validation >> ~/.bashrc
source ~/.bashrc
```

---

## Integration with CI/CD

### GitHub Actions

```yaml
name: Validate Configuration

on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Enable validation
        run: ./scripts/enable-validation.sh enable-dev

      - name: Run tests
        run: ./scripts/test-dual-path.sh

      - name: Test server startup
        run: timeout 30s ./plandex-server || true
```

---

### GitLab CI

```yaml
validate-config:
  script:
    - ./scripts/enable-validation.sh enable-dev
    - ./scripts/test-dual-path.sh
    - timeout 30s ./plandex-server || true
```

---

### Jenkins

```groovy
stage('Validate Configuration') {
    steps {
        sh './scripts/enable-validation.sh enable-dev'
        sh './scripts/test-dual-path.sh'
        sh 'timeout 30s ./plandex-server || true'
    }
}
```

---

## Troubleshooting Scripts

### Script doesn't run

**Problem:** Permission denied

**Solution:**
```bash
chmod +x scripts/*.sh
```

---

### Test fails

**Problem:** test-dual-path.sh shows failures

**Solution:**
```bash
# Check which test failed
./scripts/test-dual-path.sh 2>&1 | grep FAILED

# Run validation tests directly
cd app/shared/validation
go test -v

# Check compilation
cd app/server
go build
```

---

### Can't find server

**Problem:** "plandex-server not found"

**Solution:**
```bash
# Build server first
cd app/server
go build -o plandex-server

# Or run from correct directory
cd /path/to/plandex
./scripts/enable-validation.sh test
```

---

## Best Practices

### DO ✅

- Use `enable-dev` in development
- Use `enable-prod` in production
- Run `test-dual-path.sh` before deploying
- Check `status` regularly
- Keep `.env.validation` in version control (template)
- Test rollback procedure before production

### DON'T ❌

- Don't use production profile in development
- Don't skip testing before production
- Don't ignore test failures
- Don't commit actual secrets in `.env.validation`
- Don't enable strict mode in production (unless intended)

---

## Additional Resources

### Documentation

- **Quick Reference:** [../docs/VALIDATION_QUICK_REFERENCE.md](../docs/VALIDATION_QUICK_REFERENCE.md)
- **Rollout Guide:** [../docs/VALIDATION_ROLLOUT.md](../docs/VALIDATION_ROLLOUT.md)
- **Migration Guide:** [../docs/VALIDATION_MIGRATION_GUIDE.md](../docs/VALIDATION_MIGRATION_GUIDE.md)
- **Examples:** [../docs/VALIDATION_EXAMPLES.md](../docs/VALIDATION_EXAMPLES.md)
- **Full Documentation:** [../docs/VALIDATION_SYSTEM.md](../docs/VALIDATION_SYSTEM.md)

### Getting Help

1. Check error message (includes solution!)
2. Review documentation
3. Search existing issues
4. Create new issue with details

---

## Summary

**Quick commands:**

```bash
# Enable (dev)
./scripts/enable-validation.sh enable-dev

# Check status
./scripts/enable-validation.sh status

# Test everything
./scripts/test-dual-path.sh

# Disable
./scripts/enable-validation.sh disable
```

**That's it!** Simple scripts for powerful validation.

---

**Happy scripting!** 🚀
