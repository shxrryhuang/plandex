# Bug-Fix Registry — Schema & Data

---

## Schema Definition

Every row in the table below conforms to the following fields.

| Field | Type | Description |
|-------|------|-------------|
| `id` | `BF-NNN` | Monotonic identifier, assigned in detection order |
| `commit` | short hash | The commit that introduced the fix |
| `severity` | `CRITICAL \| HIGH \| MEDIUM \| LOW` | Impact if the bug triggers in production |
| `bug_class` | see enum below | The root-cause category of the defect |
| `detected_by` | see enum below | The mechanism that surfaced the bug |
| `module` | `server \| cli \| shared` | Go module containing the defect |
| `package` | string | Go package path relative to the module root |
| `file` | string | Source file path relative to repo root |
| `function` | string | Function or method where the defect lives |
| `line` | int or range | Line number(s) at the time of detection |
| `description` | string | What was wrong and why it mattered |
| `fix` | string | What was changed to resolve it |

### `bug_class` enum

| Value | Meaning |
|-------|---------|
| `division-by-zero` | Divisor is always zero at runtime |
| `missing-return` | `http.Error` or similar without a subsequent `return` |
| `pointer-compare` | Two `*T` pointers compared by address instead of dereferenced value |
| `swallowed-panic` | `recover()` catches a panic but the caller receives no error |
| `index-oob` | Slice or array index access without a length guard |
| `nil-deref` | Pointer or interface used after a code path that leaves it nil |
| `resource-leak` | Database cursor, file handle, or similar not closed |
| `goroutine-leak` | Goroutine that cannot terminate because its exit condition is unreachable |
| `context-leak` | `context.CancelFunc` not called on all paths |
| `logic-error` | Incorrect condition, wrong variable, or dead code |
| `naming` | Variable or function name that misrepresents its value |
| `formatting` | Code that does not conform to `gofmt` |
| `test-skip` | Debug flag that silently excludes test cases from a run |

### `detected_by` enum

| Value | Meaning |
|-------|---------|
| `gofmt` | Flagged by `gofmt -l` in the CI pipeline |
| `go-vet` | Flagged by `go vet ./...` in the CI pipeline |
| `test-restoration` | Exposed when debug `only`/`Only` flags were removed and previously-skipped test cases re-ran |
| `static-analysis` | Found by manual deep-read of packages with zero test coverage |

---

## Fix Data

### Group A — Formatting and lint (`49ee57ca`)

| id | severity | bug_class | detected_by | module | file | function | line | description | fix |
|----|----------|-----------|-------------|--------|------|----------|------|-------------|-----|
| BF-001 | LOW | formatting | gofmt | server | `app/server/model/prompts/describe.go` | — | 1–end | Extraneous leading and trailing blank lines made the file non-canonical per `gofmt`. | Ran `gofmt -w`; blank lines removed. |
| BF-002 | LOW | formatting | gofmt | cli | `app/cli/lib/log_format.go` | — | scattered | Blank lines contained trailing tab characters, making them non-empty to the formatter. | Ran `gofmt -w`; trailing whitespace stripped. |

### Group B — Context leaks (`49ee57ca`)

| id | severity | bug_class | detected_by | module | file | function | line | description | fix |
|----|----------|-----------|-------------|--------|------|----------|------|-------------|-----|
| BF-003 | HIGH | context-leak | go-vet | server | `app/server/model/plan/build_structured_edits.go` | `buildStructuredEdits` | 39 | `cancelBuild` from `context.WithCancel` was only reachable through the `buildRace` error path. The successful auto-apply path and the early error return both exited without calling it, leaking the context and any goroutines it held. | Added `defer cancelBuild()` immediately after the `WithCancel` call. |
| BF-004 | HIGH | context-leak | go-vet | cli | `app/cli/cmd/browser.go` | tab loop | — | `context.WithTimeout` returned a cancel function that was discarded with `_` on every iteration of the per-URL tab loop. The timeout context leaked until the parent was cancelled or the timer fired. | Captured the cancel function and deferred it. |

### Group C — Stream-processor bugs and test restoration (`8460af0a`)

| id | severity | bug_class | detected_by | module | file | function | line | description | fix |
|----|----------|-----------|-------------|--------|------|----------|------|-------------|-----|
| BF-005 | HIGH | logic-error | test-restoration | server | `app/server/model/plan/tell_stream_processor.go` | `bufferOrStream` | 187–208 | Stop-sequence detection split was applied to `content` (the current chunk) instead of `combined` (buffer + chunk). Sequences that started in a prior buffered chunk were never found. | Changed the split target to `combined` and updated the buffer to retain everything up through the matched stop sequence. |
| BF-006 | HIGH | logic-error | test-restoration | server | `app/server/model/plan/tell_stream_processor.go` | `bufferOrStream` | after stop loop | When a speculatively-buffered stop-sequence prefix turned out not to be a stop sequence (e.g. it was actually a `<PlandexBlock>` tag), the buffer was never prepended back into `content` before normal tag detection ran. The tag was invisible to the replacement logic. | Added a flush block that prepends `contentBuffer` into `content` and clears it when no stop-sequence state flags are active. |
| BF-007 | HIGH | logic-error | test-restoration | server | `app/server/model/plan/tell_stream_processor.go` | `bufferOrStream` | 482, 499 | `replaceCodeBlockOpeningTag` was only entered when `fileOpen` was already `true`. A full opening tag arriving via a flushed buffer had `fileOpen=false`, so the replacement was skipped entirely. | Widened the guard to `fileOpen || awaitingBlockOpeningTag` and set `fileOpen = true` on a successful match. |
| BF-008 | MEDIUM | logic-error | test-restoration | server | `app/server/model/plan/tell_stream_processor_test.go` | `TestBufferOrStream` | 341-area | Copy-paste typo: `contentBuffer` ended with `<Plandex` instead of `<Pland`. When concatenated with the chunk `exBlock…`, the result was `<PlandexexBlock` — an impossible tag that could never match. The test silently passed because the `only` flag hid it. | Changed `contentBuffer` from `"…\n<Plandex"` to `"…\n<Pland"`. |
| BF-009 | MEDIUM | test-skip | test-restoration | server | `app/server/types/reply_test.go` | `TestReplyParser` | example 9 | `Only: true` on example 9 caused all other `TestReplyParser` cases to be silently skipped. | Removed the `Only` flag; all 10 examples now run. |

### Group D — Static-analysis critical and high (`1cc6c4d6`)

| id | severity | bug_class | detected_by | module | file | function | line | description | fix |
|----|----------|-----------|-------------|--------|------|----------|------|-------------|-----|
| BF-010 | CRITICAL | division-by-zero | static-analysis | shared | `app/shared/utils.go` | `looksTextish` | 171–182 | The `for len(b) > 0` loop consumed the input slice via `b = b[size:]`. After the loop `len(b)` was always 0. The division `float64(printable)/float64(len(b))` produced `+Inf` (or `NaN` when `printable == 0`), making the 90 % threshold entirely dead code. The function returned `true` for any valid UTF-8 with at least one printable rune. | Added a `total` rune counter incremented inside the loop. Division now uses `total`. Added an explicit `total == 0` guard that returns `false`. |
| BF-011 | CRITICAL | missing-return | static-analysis | server | `app/server/handlers/invites.go` | `InviteUserHandler` | 83–86 | When `org.AutoAddDomainUsers` matched, `http.Error` wrote a 400 but the handler did not return. Execution fell through into the next block, which looked up the user by email and potentially continued processing the invite or sent a second response. | Added `return` after `http.Error`. |
| BF-012 | CRITICAL | pointer-compare | static-analysis | server | `app/server/handlers/invites.go` | `InviteUserHandler` | 83 | `org.Domain` is `*string`; `domain` is `*string` (address of a `strings.Split` element). The condition `org.Domain == domain` compared the two pointer addresses — always unequal across separate allocations — so the domain-match guard never fired. | Changed to `org.Domain != nil && *org.Domain == *domain`. |
| BF-013 | CRITICAL | swallowed-panic | static-analysis | server | `app/server/db/transactions.go` | `withTx` | 33–39 | `recover()` in the deferred function caught panics from `fn(tx)` and logged them, but `withTx` had no named return value. After recovery the function returned the zero `error` — `nil` — so the caller believed the transaction succeeded even though it was rolled back. | Changed the signature to a named return `(retErr error)`. Set `retErr` in the recover block before the rollback runs. |
| BF-014 | HIGH | logic-error | static-analysis | cli | `app/cli/cmd/diffs.go` | `getNewListener` | 119–128 | `http.HandleFunc` registered routes on the process-wide default `ServeMux`. Each time the user toggled between side-by-side and line-by-line view, a new listener was created and a new route was registered on the same mux. Toggling back to a previously registered format caused Go to panic on duplicate pattern registration. | Created a per-listener `http.NewServeMux()` inside `getNewListener`; routes are registered on it instead of the default mux. |
| BF-015 | HIGH | index-oob | static-analysis | server | `app/server/handlers/plans_exec.go` | `RespondMissingFileHandler` | 406 | `loadContexts` can return a non-nil response with an empty context slice in edge cases. The code checked `res == nil` but not `len(dbContexts) == 0`, so accessing index 0 panicked. | Added a `len(dbContexts) == 0` guard that returns a 500 error. |
| BF-016 | HIGH | nil-deref | static-analysis | server | `app/server/handlers/auth_helpers.go` | `execAuthenticate` | 498–513 | `db.GetUser` can return `(nil, nil)` when the user record is missing. The code checked `err != nil` but not `user == nil`. Subsequent field accesses (`user.Id`, `user.Email`) panicked. | Added a `user == nil` check that returns 401 Unauthorized. |

### Group E — Static-analysis open items (`6f5db915`)

| id | severity | bug_class | detected_by | module | file | function | line | description | fix |
|----|----------|-----------|-------------|--------|------|----------|------|-------------|-----|
| BF-017 | MEDIUM | context-leak | static-analysis | server | `app/server/model/plan/build_exec.go` | `execPlanBuild` | 152–156 | The goroutine proceeded through parser initialisation and plan-state fetch even when the plan context was already cancelled. Cancellation was only caught later inside model calls, wasting work. | Added a `select` on `activePlan.Ctx.Done()` immediately after the active-plan nil check, before any setup begins. |
| BF-018 | MEDIUM | nil-deref | static-analysis | server | `app/server/model/plan/build_finish.go` | `onBuildFileError` | 307 | `loadBuildFile` assigns `state.build` only on its success path (line 316 of `build_load.go`). Every error return before that — including `StorePlanBuild` failure and `ExecRepoOperation` failure — leaves `fileState.build` nil. `onBuildFileError` then accessed `build.Error` unconditionally, panicking. | Wrapped the `build.Error` assignment and `SetBuildError` call in an `if build != nil` guard. |
| BF-019 | MEDIUM | logic-error | static-analysis | server | `app/server/model/plan/build_whole_file.go` | `buildWholeFileFallback` | 117–127 | `BuildWholeFileFinishedAt` was set at line 127, after the `if err != nil` early return. On the error path the timestamp was never recorded (the `AfterReq` callback is not guaranteed to run on all error paths from `ModelRequest`). | Moved `BuildWholeFileFinishedAt = time.Now()` to immediately after `ModelRequest` returns, before the error check. Removed the now-redundant duplicate on the success path. |
| BF-020 | MEDIUM | resource-leak | static-analysis | server | `app/server/handlers/projects.go` | `ListProjectsHandler` | 92–111 | `db.Conn.Query` returned `*sql.Rows` with no `defer rows.Close()`. A scan error inside the `rows.Next()` loop caused an early `return` that leaked the database cursor. | Added `defer rows.Close()` immediately after the error check on `Query`. |
| BF-021 | MEDIUM | goroutine-leak | static-analysis | server | `app/server/diff/diff.go` | `GetDiffs` | 26–28 | Deferred cleanup wrapped `os.RemoveAll` in a goroutine (`go os.RemoveAll(...)`). If the process exited immediately after `GetDiffs` returned, the goroutine had no chance to run and the temp directory was orphaned. | Removed the goroutine wrapper; cleanup is now a direct `defer os.RemoveAll(tempDirPath)`. |
| BF-022 | MEDIUM | index-oob | static-analysis | server | `app/server/diff/diff.go` | `GetDiffReplacements` | 93 | `strings.Split(line, " ")[1:]` on a `@@` hunk header with no trailing fields produced an empty slice. The immediate access to `lineInfo[0]` panicked. | Added a `len(lineInfo) == 0` guard that skips the malformed header with `continue`. |
| BF-023 | LOW | naming | static-analysis | server | `app/server/handlers/models.go` | `UpsertCustomModelsHandler` | 61–64 | `hasDuplicates` was named the exact inverse of what `CheckNoDuplicates()` returns (`true` = no duplicates). The logic was accidentally correct because the negation and the wrong name cancelled each other out, but the code was misleading. | Renamed to `noDuplicates` so `if !noDuplicates` reads as intended. |

---

## Summary

| Metric | Count |
|--------|-------|
| Total fixes | 23 |
| CRITICAL | 4 |
| HIGH | 7 |
| MEDIUM | 8 |
| LOW | 4 |
| Commits | 4 (`49ee57ca`, `8460af0a`, `1cc6c4d6`, `6f5db915`) |
| Modules touched | 3 (`server`, `cli`, `shared`) |
| Unique `bug_class` values used | 11 of 13 |
| False positives identified | 2 (O7 `convo.go`, O9 `build_race.go`) |
