# Feature Updates

**Date:** 2026-01-28
**Status:** Complete — all systems tested, CLI builds cleanly, pipelines verified, upstream merge bugs resolved

---

## Recent Additions

| # | Feature | Commit | Key Files |
|---|---------|--------|-----------|
| 1 | Progress Reporting System | `0f271ee5` | `app/shared/progress.go`, `app/cli/progress/` |
| 2 | Error Handling & Retry Strategy | `ec63f2a3` | `app/shared/retry_policy.go`, `app/server/model/` |
| 3 | Atomic Patch Application | `9f0109a8` | `app/shared/file_transaction.go`, `app/cli/lib/apply.go` |
| 4 | Startup & Provider Validation | `e588f997` | `app/shared/validation.go`, `app/cli/lib/startup_validation.go` |
| 5 | Build Bug Fixes (upstream merge) | `6f392fd5` | `app/shared/retry_context.go`, `app/cli/lib/apply.go`, `app/cli/progress/` |

---

## Build Bug Fixes — Upstream Merge Cleanup

### Summary

Three build-breaking issues introduced by incomplete upstream merges were identified and resolved. All stem from code that was partially grafted without the corresponding variable declarations or type updates.

### Fixes

| # | File | Root Cause | Fix |
|---|------|------------|-----|
| 1 | `app/shared/retry_context.go:173` | Called `FromProviderFailure()` — name does not exist in the package | Renamed call to `ErrorReportFromProviderFailure()` matching `error_report.go:252` |
| 2 | `app/cli/lib/apply.go:749–840` | Orphaned transactional block referencing undeclared `reporter`, `tx`, `updatedPaths` left over from incomplete merge of the atomic patch system | Removed dead block; restored original goroutine-based `toRemove` loop and `errCh` drain that completes the function correctly |
| 3 | `app/cli/progress/examples_test.go` + `pipeline_test.go` | Struct literals and callback signatures used `shared.Step` (the newer, simpler report struct) where `shared.ProgressStep` (the full progress-tracking struct with `Kind`, `State`, `CompletedAt`) is required by `ProgressReport.Steps` and `PipelineConfig` | Global replace `shared.Step` → `shared.ProgressStep` in both test files |

### Build State After Fixes

| Check | Result |
|-------|--------|
| `go build ./...` (shared) | PASS |
| `go build ./...` (cli) | PASS |
| `go vet ./...` (shared) | PASS |
| `go vet ./...` (cli) | PASS |
| Validation pipeline (format, vet, unit-tests, build) | All PASS |

---

## Progress Reporting System

### Summary

Added real-time, step-level execution tracking that surfaces every phase and operation as it happens. Output adapts automatically: animated progress bars and spinners in a terminal, structured log lines in CI/piped output.

### New Files

| File | Purpose |
|------|---------|
| `app/shared/progress.go` | Core types: `StepState`, `StepKind`, `Step`, `ProgressPhase`, `ProgressReport`, `ProgressMessage` |
| `app/cli/progress/tracker.go` | State coordinator with 100ms event loop, stall detection, phase callbacks |
| `app/cli/progress/renderer.go` | TTY (ANSI progress bars, spinners, color) and non-TTY (structured logs) output |
| `app/cli/progress/stream_adapter.go` | Bridges `StreamMessage` protocol → `Tracker` updates without modifying either side |
| `app/cli/progress/pipeline/pipeline.go` | Standalone callback-driven orchestrator for isolated testing |
| `app/cli/progress/pipeline/runner.go` | Visual executor with spinner animation for demos/scenarios |

### Key Design Decisions

- **Two rendering paths:** TTY uses ANSI escape codes to clear and redraw; non-TTY emits one log line per event. Detection is automatic.
- **Guaranteed vs best-effort states:** Terminal states (`completed`, `failed`, `skipped`) never change; live states (`running`, `waiting`, `stalled`) are indicators.
- **Per-kind stall thresholds:** LLM calls get 60 s, file builds 120 s, validation 30 s — avoids false positives while catching real hangs.
- **StreamAdapter pattern:** Decouples the legacy streaming protocol from the new progress model; either side can evolve independently.

---

## Error Handling & Retry Strategy

### Summary

Added a multi-layered resilience stack for provider interactions: configurable retry policies with exponential backoff, circuit breakers with per-provider state machines, graceful degradation that reduces scope under stress, a dead-letter queue for exhausted operations, health monitoring for smart routing, and stream recovery for partial response tracking.

### New Files

| File | Purpose |
|------|---------|
| `app/shared/retry_policy.go` | Per-failure-type policies, exponential backoff + jitter, `RetryState` tracking |
| `app/shared/idempotency.go` | Deduplication across retries via key tracking, file-change records, hash verification |
| `app/shared/run_journal.go` | Persistent execution log: entries, checkpoints, retry/circuit/fallback events, pause/resume |
| `app/shared/ai_models_errors.go` | `ModelError` ↔ `ProviderFailure` bridge + multi-level fallback routing |
| `app/server/model/circuit_breaker.go` | Per-provider Closed → Open → HalfOpen state machine |
| `app/server/model/dead_letter_queue.go` | Stores failed ops after all retries; supports auto-retry scheduling |
| `app/server/model/graceful_degradation.go` | 5-level degradation: adjusts context size, timeouts, retries, model selection |
| `app/server/model/health_check.go` | Proactive monitoring with latency percentiles and health scoring |
| `app/server/model/stream_recovery.go` | Partial stream buffering with token-based checkpoints |

### Key Design Decisions

- **Three-layer stack:** Failure Classification & Retry → Provider Health & Routing → Recovery & Audit. Each layer is independently testable.
- **Idempotency before retry:** Every operation gets a stable key before the first attempt; retries are safe by construction.
- **Circuit breaker excludes auth errors:** Authentication failures bypass the circuit so they surface immediately rather than being masked.
- **Degradation auto-triggers:** Error rate crossing a threshold activates the next level automatically; recovery requires explicit action or cool-off.

---

## Atomic Patch Application

### Summary

Wrapped file application in a transactional layer with all-or-nothing semantics. If any file operation fails mid-sequence, all previously applied changes roll back. A write-ahead log provides crash recovery. An optional workspace isolation layer lets changes be staged and reviewed before being committed to the project.

### New / Significantly Modified Files

| File | What changed |
|------|--------------|
| `app/shared/file_transaction.go` | Extended with WAL, snapshots, checkpoints, `RollbackOnProviderFailure()`, `RecoverTransaction()` |
| `app/shared/patch_status.go` | New — `PatchStatusReporter` interface, per-file and per-transaction event callbacks |
| `app/shared/patch_apply_test.go` | New — transaction integration tests |
| `app/cli/lib/apply.go` | Rewritten to use `FileTransaction`; added script execution with signal handling |
| `app/cli/lib/workspace_apply.go` | New — workspace-aware apply/rollback adapter |
| `app/cli/workspace/manager.go` | Extended with `Commit()` (transactional project writes), `Resume()`, stale cleanup |

### Key Design Decisions

- **Snapshot at Begin():** Original file contents captured once; any subsequent failure restores from snapshot.
- **WAL for crash safety:** Intent entries written before each operation; `RecoverTransaction()` can replay from WAL after an unexpected exit.
- **Workspace commit is also transactional:** `Manager.Commit()` snapshots project files before applying workspace changes; rolls back on failure.
- **Metadata updates are deferred:** In workspace mode, tracking maps are updated only after all file writes succeed — atomic from the caller's perspective.

---

## Startup & Provider Validation Phase

---

## Summary

Introduced an explicit two-phase validation system that runs before any plan execution begins. The validation catches common misconfigurations early — missing API keys, invalid filesystem paths, incompatible provider combinations, and unsupported environment values — and surfaces clear, actionable error messages in both CLI output and the run journal error registry.

---

## What Changed

### New Files

| File | Purpose |
|------|---------|
| `app/shared/validation.go` | Core validation framework: types, severity/category/phase enums, `FormatCLI()`, `ToErrorReport()`, helper functions |
| `app/shared/validation_test.go` | 18 unit tests covering all public APIs |
| `app/cli/lib/startup_validation.go` | CLI validation logic: synchronous startup checks, deferred provider checks, `MustRunDeferredValidation()` entry point |

### Modified Files

| File | What was added |
|------|----------------|
| `app/cli/main.go` | `runStartupChecks()` function wired into `main()` for all commands except `version`, `browser`, `help`, `sign-in`, `connect` |
| `app/cli/cmd/tell.go` | `lib.MustRunDeferredValidation()` call before `MustVerifyAuthVars` |
| `app/cli/cmd/build.go` | `lib.MustRunDeferredValidation()` call before `MustVerifyAuthVars` |
| `app/cli/cmd/continue.go` | `lib.MustRunDeferredValidation()` call before `MustVerifyAuthVars` |
| `docs/SYSTEM_DESIGN.md` | Added §12 (Startup & Provider Validation), updated §3.3 file listing, updated §6.1 flow diagram |
| `docs/docs/environment-variables.md` | Added "Startup Validation" section with validated vars and example output |
| `docs/docs/models/model-providers.md` | Added "Pre-Execution Validation" section with check table and example output |

---

## Validation Phases

### Phase 1 — Synchronous (Startup)

Runs once per CLI invocation before any network call. All commands except version/help/sign-in are covered.

| Check | What it validates | Severity |
|-------|-------------------|----------|
| Home directory exists | `fs.HomePlandexDir` is set, exists, is a directory, and is writable | Fatal |
| Auth file integrity | `auth.json` and `accounts.json` are valid JSON if present | Fatal |
| Auth file emptiness | Warns if config files exist but contain no data | Warning |
| `PLANDEX_ENV` value | Must be `""`, `"development"`, or `"production"` | Fatal |
| `PLANDEX_DEBUG_LEVEL` value | Must be `"verbose"`, `"normal"`, `"minimal"`, or unset | Warning |
| `PLANDEX_TRACE_FILE` parent | Parent directory must exist if env var is set | Warning |

### Phase 2 — Deferred (Provider-Scoped)

Runs after plan settings are loaded, triggered by `tell`, `build`, or `continue`. Only checks providers the current plan actually uses.

| Check | What it validates | Severity |
|-------|-------------------|----------|
| API key env var | Required key is set and non-empty | Fatal |
| Extra required auth vars | e.g. `VERTEXAI_PROJECT`, `VERTEXAI_LOCATION` | Fatal |
| Credential file paths | File-path values (starting with `/`, `.`, `~`) point to existing files | Fatal |
| AWS profile reachability | `PLANDEX_AWS_PROFILE` has a backing `~/.aws/credentials` or `~/.aws/config` | Fatal |
| Claude Max credentials | Creds file exists when subscription is active | Warning |
| Dual Anthropic providers | Both `ANTHROPIC_API_KEY` and Claude Max configured simultaneously | Warning |

---

## Error Output Format

Errors are grouped by category and printed with severity badges:

```
── Configuration errors must be fixed before running ──

  FILESYSTEM
    ✗  Plandex home directory is not writable: /home/user/.plandex
       → Fix permissions: chmod 755 /home/user/.plandex

  PROVIDER
    ✗  Missing required environment variable OPENAI_API_KEY (needed for openai)
       → export OPENAI_API_KEY='your-api-key'
    ⚠  Both Claude Max subscription and ANTHROPIC_API_KEY are configured
       → If you intend to use Claude Max as your primary Anthropic provider, you can unset ANTHROPIC_API_KEY.
```

Category display order: Filesystem → Environment → Authentication → Provider → Configuration.

---

## Integration with Error Registry

When fatal validation errors occur, `ToErrorReport()` converts them into an `ErrorReport` and persists via `shared.StoreError()`. This ensures:
- Failures appear in run journals for post-mortem diagnosis
- The `plandex doctor` workflow can surface stale validation failures

---

## Test Coverage

| Area | Tests | Status |
|------|-------|--------|
| ValidationResult Add/Merge | 6 | PASS |
| ValidationError interface | 1 | PASS |
| FormatCLI output | 4 | PASS |
| ToErrorReport conversion | 2 | PASS |
| ValidateEnvVarSet helper | 3 | PASS |
| ValidateProviderCompatibility | 4 | PASS |
| ValidateFilePath helper | 2 | PASS |
| Timestamp freshness | 1 | PASS |
| **Total new tests** | **18** | **All passing** |

Build (`go build ./...`) and static analysis (`go vet ./...`) both pass cleanly on `app/shared` and `app/cli`.

---

## CI Pipeline

A dedicated GitHub Actions workflow (`.github/workflows/validation-tests.yml`) runs independently from the main CI suite. Triggers only on changes to validation source files or on a daily schedule.

| Job | What it does |
|-----|--------------|
| `format` | `gofmt` on validation source files |
| `vet` | `go vet` on `shared` and `cli` modules |
| `unit-tests` | 23 tests with `-race`, coverage to Codecov, per-area step summaries |
| `build` | Full CLI compile + grep-verification of all integration entry points |
| `summary` | Aggregated pass/fail table |
| `notify-on-failure` | Opens labeled GitHub issue on scheduled-run failure |

Local mirror: `test/run_validation_tests.sh` — supports `all`, `format`, `vet`, `unit`, `build` targets.

---

## Error Message Scan Results

A post-implementation scan identified 12 additional error conditions not yet surfaced by the validation system.

### High Priority — Catchable at Startup Now

| # | Condition | Location | Impact |
|---|-----------|----------|--------|
| 1 | `SHELL` env var empty, `/bin/bash` fallback missing | `lib/apply.go:375` | Apply scripts fail silently |
| 2 | `PLANDEX_API_HOST` invalid URL/hostname | `api/clients.go:25` | Cryptic network errors |
| 3 | `PLANDEX_REPL_OUTPUT_FILE` parent directory missing | `stream_tui/run.go:102` | Write fails at runtime |
| 4 | `projects-v2.json` malformed JSON | `fs/projects.go:31`, `lib/current.go:96` | Bare unmarshal exit |
| 5 | `settings-v2.json` malformed JSON | `lib/current.go:198` | Bare unmarshal exit |

### Medium Priority — Deferred Validation Gaps

| # | Condition | Location | Impact |
|---|-----------|----------|--------|
| 6 | `GOOGLE_APPLICATION_CREDENTIALS` file has invalid JSON content | `lib/startup_validation.go:372` | Exists but unparseable |
| 7 | `PLANDEX_AWS_PROFILE` names undefined profile | `lib/startup_validation.go:342` | Reachability stops at file existence |
| 8 | `PLANDEX_COLUMNS` not a valid integer | `term/utils.go:88` | Silently ignored |
| 9 | `PLANDEX_STREAM_FOREGROUND_COLOR` invalid ANSI code | `term/utils.go:125` | Silently ignored |

### Lower Priority — Requires Runtime Context

| # | Condition | Location | Why deferred |
|---|-----------|----------|--------------|
| 10 | Project root not writable | `lib/apply.go:356` | Only known after plan resolution |
| 11 | Custom editor not on PATH | `lib/editor.go:75` | Only known after editor selection |
| 12 | `less` pager not available | `term/utils.go:49` | Only needed during output display |

---

## Documentation Updated

| Document | Changes |
|----------|---------|
| `docs/docs/environment-variables.md` | "Startup Validation" subsection with validated vars, example output, config file checks; new "Additional Variables" subsection documenting scan-discovered env vars |
| `docs/docs/models/model-providers.md` | "Pre-Execution Validation" section with check table, example output; new "Known Gaps" subsection listing provider validation candidates from the scan |
| `docs/SYSTEM_DESIGN.md` | §3.3 file listing includes `validation.go`; §6.1 flow diagram shows validation steps; full §12 with architecture diagram, severity/category tables, integration points, key files, test coverage, CI pipeline (§12.8), and error scan findings (§12.9) |
