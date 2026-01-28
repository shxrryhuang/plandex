# Unit Test Documentation for Plandex

> **Document Version:** 2.0
> **Last Updated:** January 23, 2026
> **Project:** Plandex - Terminal-based AI Development Tool

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Test Results Summary](#test-results-summary)
3. [Test Files Overview](#test-files-overview)
4. [Bug Fixes Applied](#bug-fixes-applied)
5. [Linting and Formatting CI/CD Integration](#linting-and-formatting-cicd-integration)
6. [Test Coverage by Module](#test-coverage-by-module)
7. [Running Tests](#running-tests)

---

## Executive Summary

**Project Statistics:**
- **Total Go Code:** ~77,404 lines across 335 files
- **Test Files:** 20 Go unit test files + 4 shell integration scripts
- **Test Status:** **ALL 651 TESTS PASSING** (0 failing)

### Key Improvements Made
1. **Stream Processor Bug Fixed** - `bufferOrStream()` now correctly handles stop sequences split across chunks
2. **Context Leak Fixed** - Added `defer cancelBuild()` in `build_structured_edits.go`
3. **Test Coverage Expanded** - From 64 tests to 651 tests (917% increase)
4. **New Test Packages** - Added tests for model errors, tokens, diff, hooks, file mapping, and the retry/safety subsystem
5. **Retry & Safety Tests** - 28 new tests covering configurable backoff, jitter bounds, operation safety classification, and retry context lifecycle

---

## Test Results Summary

### Overall Results

| Status | Count | Percentage |
|--------|-------|------------|
| **PASSED** | 651 | 100% |
| **FAILED** | 0 | 0% |
| **TOTAL** | 651 | 100% |

### Results by Module

| Module | Tests | Status |
|--------|-------|--------|
| `plandex-server/db` | 36 | ✅ PASS |
| `plandex-server/diff` | 25 | ✅ PASS |
| `plandex-server/handlers` | 53 | ✅ PASS |
| `plandex-server/hooks` | 25 | ✅ PASS |
| `plandex-server/model` | 70 | ✅ PASS |
| `plandex-server/model/parse` | 6 | ✅ PASS |
| `plandex-server/model/plan` | 26 | ✅ PASS |
| `plandex-server/syntax` | 83 | ✅ PASS |
| `plandex-server/syntax/file_map` | 75 | ✅ PASS |
| `plandex-server/types` | 102 | ✅ PASS |
| `plandex-server/utils` | 122 | ✅ PASS |
| `plandex-shared` (retry/safety) | 28 | ✅ PASS |

---

## Test Files Overview

### Existing Test Files (Enhanced)
| File | Location | Description |
|------|----------|-------------|
| `tell_stream_processor_test.go` | `model/plan/` | Stream processing, stop tags, buffering |
| `structured_edits_test.go` | `syntax/` | Code replacement with reference comments |
| `unique_replacement_test.go` | `syntax/` | Unique replacement detection |
| `subtasks_test.go` | `model/parse/` | Subtask parsing |
| `reply_test.go` | `types/` | Reply parsing |
| `whitespace_test.go` | `utils/` | Whitespace handling |

### New Test Files Created
| File | Location | Tests | Description |
|------|----------|-------|-------------|
| `data_models_test.go` | `db/` | 36 | Plan status, roles, context types, operations |
| `safe_map_test.go` | `types/` | 56 | Concurrent map operations |
| `validation_test.go` | `handlers/` | 53 | Email, plan name, path, token validation |
| `active_plan_test.go` | `types/` | 45 | Build state, file ops, token tracking |
| `xml_test.go` | `utils/` | 122 | XML parsing with fixture files |
| `parsers_test.go` | `syntax/` | 50 | Language detection, comments, code blocks |
| `model_error_test.go` | `model/` | 55 | Error classification, retry logic |
| `tokens_test.go` | `model/` | 15 | Token estimation |
| `diff_test.go` | `diff/` | 25 | Diff generation, replacements |
| `hooks_test.go` | `hooks/` | 25 | Hook registration and execution |
| `markup_test.go` | `syntax/file_map/` | 45 | HTML markup parsing |
| `svelte_test.go` | `syntax/file_map/` | 30 | Svelte component parsing |
| `retry_config_test.go` | `shared/` | 12 | Backoff math, jitter bounds, max clamping, env loading, Retry-After cap |
| `operation_safety_test.go` | `shared/` | 3 | Safety strings, IsOperationSafe combos, ClassifyOperation |
| `retry_context_test.go` | `shared/` | 13 | Attempt lifecycle, CanRetry caps, fallback recording, timing, Summary |

### Test Fixture Files
| File | Location | Purpose |
|------|----------|---------|
| `simple_tags.xml` | `utils/xml_test_examples/` | Basic XML tag tests |
| `attributes.xml` | `utils/xml_test_examples/` | Attribute parsing tests |
| `entities.xml` | `utils/xml_test_examples/` | XML entity tests |
| `cdata.xml` | `utils/xml_test_examples/` | CDATA section tests |
| `nested_tags.xml` | `utils/xml_test_examples/` | Nested structure tests |

---

## Bug Fixes Applied

### 1. Stream Processor Bug (FIXED)
**Location:** `app/server/model/plan/tell_stream_processor.go:187-208`

**Problem:** Stop sequences split across network chunks weren't handled correctly.

**Fix:**
```go
// BEFORE (buggy):
if strings.Contains(processor.contentBuffer+content, stopSequence) {
    split := strings.Split(content, stopSequence)  // Wrong: splits only content

// AFTER (fixed):
combined := processor.contentBuffer + content
if strings.Contains(combined, stopSequence) {
    split := strings.Split(combined, stopSequence)  // Correct: splits combined
    processor.contentBuffer = stopSequence  // Store complete tag
```

### 2. Context Leak Bug (FIXED)
**Location:** `app/server/model/plan/build_structured_edits.go:39`

**Problem:** `cancelBuild` function not called on all exit paths, causing context leaks.

**Fix:**
```go
buildCtx, cancelBuild := context.WithCancel(activePlan.Ctx)
defer cancelBuild() // Added: Ensure context is cancelled on all exit paths
```

### 3. Test Data Bug (FIXED)
**Location:** `app/server/model/plan/tell_stream_processor_test.go:434-449`

**Problem:** Malformed test case had `"exBlock"` instead of `"Block"` causing doubled "ex".

**Fix:** Corrected chunk data and added missing initial state fields.

---

## Linting and Formatting CI/CD Integration

### CI/CD Workflow
**File:** `.github/workflows/ci.yml`

The workflow includes:
- Go formatting checks (`gofmt`)
- Linting with `golangci-lint`
- Unit tests with race detection
- Code coverage reporting
- Security scanning with `gosec`

### Linter Configuration
**File:** `.golangci.yml`

Enabled linters:
- `errcheck` - Unchecked error returns
- `govet` - Suspicious constructs
- `staticcheck` - Static analysis
- `ineffassign` - Unused assignments
- `unused` - Unused code
- `misspell` - Spelling errors
- `gosec` - Security issues

### Current Linter Status
| Check | Status |
|-------|--------|
| go vet | ✅ Clean |
| gofmt | ✅ Clean |
| Tests | ✅ 623 passing |
| Race Detection | ✅ Clean |

---

## Test Coverage by Module

### Critical Modules (Fully Tested)
| Module | Coverage | Key Tests |
|--------|----------|-----------|
| Stream Processing | High | Stop tags, buffering, chunk handling |
| Syntax Editing | High | Structured replacements, language detection |
| Type Safety | High | SafeMap concurrency, validation |
| XML Parsing | High | Tags, attributes, entities, CDATA |

### Modules with New Tests
| Module | New Tests | Coverage Focus |
|--------|-----------|----------------|
| `model/` | 70 | Error classification, token estimation |
| `diff/` | 25 | Diff generation, replacements |
| `hooks/` | 25 | Hook lifecycle |
| `syntax/file_map/` | 75 | HTML/Svelte parsing |
| `shared/ (retry)` | 12 | Backoff math, jitter, env loading, Retry-After cap |
| `shared/ (safety)` | 3 | OperationSafety classification, IsOperationSafe |
| `shared/ (context)` | 13 | RetryContext lifecycle, CanRetry, attempt tracing |

---

## Running Tests

### Run All Tests
```bash
cd app/server && go test ./... -v
```

### Run Tests with Coverage
```bash
cd app/server && go test ./... -v -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### Run Tests with Race Detection
```bash
cd app/server && go test ./... -v -race
```

### Run Specific Module Tests
```bash
# Model tests
go test -v ./model/...

# Syntax tests
go test -v ./syntax/...

# Utils tests (including XML)
go test -v ./utils/...

# New test packages
go test -v ./diff/...
go test -v ./hooks/...
go test -v ./syntax/file_map/...
```

### Run Linter
```bash
golangci-lint run ./...
```

### Check Formatting
```bash
gofmt -l .
```

---

## Test History

| Date | Tests | Change |
|------|-------|--------|
| Initial | 64 | Baseline |
| Jan 22, 2026 | 215 | Added db, handlers, types tests |
| Jan 22, 2026 | 352 | Added active_plan, xml, parsers tests |
| Jan 23, 2026 | 416 | Added XML fixture tests |
| Jan 23, 2026 | 623 | Added model, diff, hooks, file_map tests |
| Jan 28, 2026 | 651 | Added retry_config, operation_safety, retry_context tests (28 new) |

---

*This document is auto-updated when tests are added or modified.*
