# Plandex — Unit Test Registry & Execution Results

---

## How to Read This Document

- **Section 1** lists every unit test required to validate Plandex, grouped by subsystem. Each entry includes a rationale. Tests marked `[EXISTS]` are already implemented. Tests marked `[MISSING]` represent coverage gaps.
- **Section 2** describes the linting and formatting checks integrated into CI/CD.
- **Section 3** contains the live execution results: what ran, what was skipped, what passed, and an urgency-ranked assessment of every gap found.

---

## Pipeline Status — `2026-01-31` — commit `8460af0a`

| Module | Tests | gofmt | go vet | Overall |
|--------|-------|-------|--------|---------|
| `app/server` | PASS (77 executed) | PASS | PASS | PASS |
| `app/cli` | PASS (no test files) | PASS | PASS | PASS |
| `app/shared` | PASS (no test files) | PASS | PASS | PASS |

**Tests:** 77 executed, 77 passed, 0 failed. All previously-skipped subtests are now running (debug `only`/`Only` flags removed).

**gofmt:** All three modules clean.

**go vet:** All three modules clean. The only remaining compiler output is a warning from a vendored third-party C file (`go-tree-sitter/lua`); it is not owned by this repo and does not affect the build.

---

## 1. Required Unit Tests

---

### A. Subtask Parsing — `app/server/model/parse/`

Subtask parsing extracts structured task lists from AI model responses. Incorrect parsing here means the plan execution engine receives malformed task definitions, which silently produces wrong file operations.

| # | Test | Status | What It Validates |
|---|------|--------|-------------------|
| A1 | Empty input returns nil | EXISTS | No crash on empty or missing task section |
| A2 | Single task, no description | EXISTS | Minimal valid task is parsed correctly |
| A3 | Multiple tasks with descriptions and Uses fields | EXISTS | Full task structure including file references |
| A4 | Alternative header (`### Task` vs `### Tasks`) | EXISTS | Both singular and plural headers are recognized |
| A5 | Tasks separated by blank lines | EXISTS | Blank lines do not corrupt task boundaries |
| A6 | Real-world Makefile task (pong regression) | EXISTS | Regression test from actual production usage |
| A7 | `ParseRemoveSubtasks` — empty input | MISSING | The sibling function for task removal has zero coverage |
| A8 | `ParseRemoveSubtasks` — valid remove section | MISSING | Removal tasks must be parsed to prevent stale files |
| A9 | Malformed input — no numbered list | MISSING | Graceful degradation when AI produces unexpected format |
| A10 | Filenames with spaces in Uses field | MISSING | Paths like `src/my file.go` must survive parsing |

**Why these matter:** Subtask parsing is the bridge between AI output and deterministic file operations. Any parsing bug here propagates silently into incorrect edits applied to user code.

---

### B. Stream Processing & Tag Conversion — `app/server/model/plan/`

The stream processor converts `<PlandexBlock>` XML tags arriving in arbitrary chunk boundaries into markdown code fences for the TUI. Tags can arrive split across any number of network chunks, so every partial-state transition must be correct.

| # | Test | Status | What It Validates |
|---|------|--------|-------------------|
| B1 | Regular content streams through | EXISTS — SKIPPED | Baseline: non-tag content is not buffered |
| B2 | Partial opening tag is buffered | EXISTS — SKIPPED | `<Pland` mid-chunk does not stream prematurely |
| B3 | Full opening tag converted to code fence | EXISTS — SKIPPED | `<PlandexBlock lang="go">` becomes ` ```go ` |
| B4 | Opening tag without prior maybeFilePath | EXISTS — SKIPPED | Edge case where path is set directly |
| B5 | Partial backticks are buffered | EXISTS — SKIPPED | Triple backticks in code content must be escaped |
| B6 | Backticks in content are escaped | EXISTS — SKIPPED | Prevents premature code fence closure |
| B7 | Partial closing tag is buffered | EXISTS — SKIPPED | `</Plan` does not leak to output |
| B8 | Full closing tag with file still open | EXISTS — SKIPPED | Tag is held until file state resolves |
| B9 | Closing tag replaced when file is closed | EXISTS — SKIPPED | `</PlandexBlock>` becomes closing ` ``` ` |
| B10 | Closing tag with awaiting backticks | EXISTS — SKIPPED | Combined state: backtick ambiguity + tag close |
| B11 | Single backtick pass-through | EXISTS — SKIPPED | Inline code references are not falsely escaped |
| B12 | Close and re-open backtick sequence | EXISTS — SKIPPED | Adjacent inline code blocks handled correctly |
| B13 | End-of-file-ops tag buffered | EXISTS — SKIPPED | `<EndPlandexFileOps/>` triggers buffering |
| B14 | End-of-file-ops tag replaced | EXISTS — SKIPPED | Tag is stripped from output |
| B15 | Partial end-of-file-ops tag | EXISTS — SKIPPED | Partial tag does not leak |
| B16 | End-of-file-ops partial completion | EXISTS — SKIPPED | Tag completes across two chunks |
| B17 | Partial opening tag with no label | EXISTS — SKIPPED | Inline `<Pland` without preceding path label |
| B18 | Continued buffering of partial opening tag | EXISTS — SKIPPED | Multi-chunk tag accumulation |
| B19 | Opening tag completes with no label | EXISTS — SKIPPED | Buffered tag resolves when path becomes known |
| B20 | Full opening tag without label | EXISTS — SKIPPED | Single-chunk tag when no label preceded it |
| B21 | Stop tag in single chunk | EXISTS — SKIPPED | `<PlandexFinish/>` halts streaming correctly |
| B22 | Stop tag split across chunks — prefix | EXISTS — RUNS | Only subtest that executes; see Results |
| B23 | Stop tag split across chunks — completion | EXISTS — SKIPPED | Completes stop sequence across boundary |
| B24 | Stop prefix resolves to different tag | EXISTS — SKIPPED | `<PlandexFin` turns out to be `<PlandexBlock` |
| B25 | Stop prefix resolves to block tag (variant) | EXISTS — SKIPPED | Second variant of tag disambiguation |

**Why these matter:** The stream processor runs on every single chunk of every AI response. A bug here corrupts the user's real-time view and can cause the TUI to hang or display malformed output. 24 of 25 tests for this component are not running.

---

### C. Structured Code Edits — `app/server/syntax/`

This is the core file-editing engine. It takes an original file and a proposed diff containing reference comments (e.g., `// ... existing code ...`) and produces the merged result. This is responsible for every file modification Plandex makes.

| # | Test | Status | What It Validates |
|---|------|--------|-------------------|
| C1 | Single reference comment in function | EXISTS | `// ... existing code ...` preserves surrounding lines |
| C2 | Bad indentation formatting | EXISTS | Misaligned code still merges correctly |
| C3 | Multiple references in nested class | EXISTS | Struct field addition + method insertion |
| C4 | Single removal comment | EXISTS | `// Plandex: removed code` deletes correct lines |
| C5 | Multiple removal comments | EXISTS | Consecutive removals in same function |
| C6 | JSON update with references | EXISTS | Non-code structured files are handled |
| C7 | Method replacement with context | EXISTS | JS class method swap preserves sibling methods |
| C8 | Nested namespace class methods | EXISTS | TypeScript namespace + class nesting |
| C9 | Trailing comma handling | EXISTS | JS object method with trailing comma |
| C10 | Multiple structural updates | EXISTS | Constructor addition + method replacement + method append |
| C11 | JSON multi-level nested update | EXISTS | Deep JSON key insertion |
| C12 | JSON multi-level update variant | EXISTS | JSON icon object replacement |
| C13 | Scala complex class structures | EXISTS | Multi-parameter list class with implicits |
| C14 | Top-level ambiguous insertion | EXISTS | New function placed among existing top-level functions |
| C15 | Top-level with anchor functions | EXISTS | Anchor lines disambiguate insertion point |
| C16 | Extraneous newline cleanup | EXISTS | Extra blank lines between functions are removed |
| C17 | Insert between non-adjacent anchors | EXISTS | Insert mode: new line placed between known anchors |
| C18 | Insert with reference + non-adjacent anchors | EXISTS | Combined reference and insert semantics |
| C19 | Replacement with removal outside range | EXISTS | Explicit line range removal in description |
| C20 | Replacement with removal inside multi-line range | EXISTS | Multiple removal ranges in one edit |
| C21 | Append to end of full file | EXISTS | Insert mode with entire original included |
| C22 | Prepend to beginning of full file | EXISTS | New block before all existing content |
| C23 | Empty original file (new file creation) | MISSING | Creating a file via structured edit |
| C24 | Proposed identical to original (no-op) | MISSING | An edit that changes nothing must not corrupt |
| C25 | Python file edits | MISSING | Indentation-sensitive language not covered |
| C26 | Rust file edits | MISSING | Brace-based language with macro syntax not covered |

**Why these matter:** A failure here directly corrupts user source code. This is the highest-consequence component in the system.

---

### D. Unique Replacement Matching — `app/server/syntax/`

When the AI proposes a replacement for a code block, `FindUniqueReplacement` fuzzy-matches it against the original file to locate the correct target. Ambiguous matches are rejected to prevent editing the wrong location.

| # | Test | Status | What It Validates |
|---|------|--------|-------------------|
| D1 | Perfect single match | EXISTS | Exact substring is found |
| D2 | Match with error in middle | EXISTS | Fuzzy match tolerates AI hallucination in content |
| D3 | Multiple instances, unique boundaries | EXISTS | Correct block selected when others share prefix/suffix |
| D4 | No match at all | EXISTS | Returns empty string, does not crash |
| D5 | Multiple identical matches | EXISTS | Returns empty — ambiguous, cannot safely edit |
| D6 | Ambiguous boundaries | EXISTS | Multiple candidates with similar start and end |
| D7 | Very different middle content | EXISTS | Boundaries match but interior is completely wrong |
| D8 | Unique match near identical text | EXISTS | Correct block found despite similar neighbors |
| D9 | Identical start/end patterns | EXISTS | Symmetric patterns are flagged as ambiguous |
| D10 | Overlapping patterns | EXISTS | `ABCABCDEF` with target `ABCDEF` |
| D11 | Empty old string | MISSING | Edge case: searching for empty string |
| D12 | Empty original file | MISSING | Edge case: searching in empty file |
| D13 | Multiline replacement target | MISSING | Replacement spans multiple lines |

**Why these matter:** This is the safety mechanism that prevents edits from landing on the wrong code block. False positives silently corrupt files; false negatives block valid edits.

---

### E. Reply Parsing & Operation Detection — `app/server/types/`

The `ReplyParser` processes streaming AI output to detect file operations (create, move, remove, reset) as they arrive. It must handle arbitrary chunk boundaries without missing or duplicating operations.

| # | Test | Status | What It Validates |
|---|------|--------|-------------------|
| E1 | Two file operations (apply.go, checkout.go) | EXISTS — SKIPPED | Basic multi-file operation detection |
| E2 | Context remove + update operations | EXISTS — SKIPPED | Non-file command detection |
| E3 | Context remove + update (duplicate set) | EXISTS — SKIPPED | Duplicate operation handling |
| E4 | Single file operation | EXISTS — SKIPPED | Minimal valid single operation |
| E5 | Two files across packages | EXISTS — SKIPPED | Operations spanning different directories |
| E6 | Single deep-path file | EXISTS — SKIPPED | Nested directory path parsing |
| E7 | File map operation | EXISTS — SKIPPED | file_map path handling |
| E8 | Makefile + apply script | EXISTS — SKIPPED | Non-Go file type operations |
| E9 | Move + remove + reset + file (mixed ops) | EXISTS — SKIPPED | All four operation types in one response |
| E10 | File operation with description text | EXISTS — RUNS | Only example that executes; see Results |

**Why these matter:** Reply parsing drives the change-tracking UI and determines which files are modified. Missing an operation means a file change is invisible to the user before they accept it.

---

### F. Whitespace Normalization — `app/server/utils/`

`StripAddedBlankLines` prevents the AI from adding gratuitous leading or trailing blank lines to edited files, which would pollute diffs and version control history.

| # | Test | Status | What It Validates |
|---|------|--------|-------------------|
| F1 | No change (identity) | EXISTS | Unchanged content passes through unmodified |
| F2 | Leading newlines added | EXISTS | Extra leading blank lines are stripped |
| F3 | Trailing newline added | EXISTS | Extra trailing blank line is stripped |
| F4 | Both ends, original padding preserved | EXISTS | Original whitespace is the target, not zero |
| F5 | CRLF line endings | MISSING | Windows-style line endings must not break stripping |
| F6 | Tab-only lines at boundaries | MISSING | Lines containing only tabs are not blank-line equivalent |

**Why these matter:** Extra blank lines at file boundaries break diffs and confuse version control. This runs on every edited file.

---

### G. Model Error Classification — `app/server/model/` — NO TESTS EXIST

`ClassifyModelError` and `ClassifyErrMsg` categorize HTTP and message-based errors from every AI provider. Correct classification drives retry decisions, user-facing messages, and rate-limit handling.

| # | Test | What It Validates |
|---|------|-------------------|
| G1 | HTTP 429 classified as rate limited | Rate limit detection; triggers retry with backoff |
| G2 | HTTP 413 classified as payload too large | Context overflow detection |
| G3 | HTTP 501 classified as not implemented | Unsupported model feature |
| G4 | HTTP 505 classified as version not supported | Model version mismatch |
| G5 | Retry-After header extraction | Correct backoff duration is parsed |
| G6 | "context length" in error message | Context-too-long classification from message body |
| G7 | "overloaded" in error message | Transient overload classification |
| G8 | Unknown error code | Falls through to generic error without crash |
| G9 | Nil response body | Does not panic on empty response |

**Why these matter:** Misclassified errors either cause unnecessary retries (wasting tokens and money) or fail to retry when they should (causing user-visible failures that should have been transparent).

---

### H. Token Estimation — `app/server/model/` — NO TESTS EXIST

`GetMessagesTokenEstimate` estimates token counts for context window management. Over-estimation wastes context budget; under-estimation causes runtime context-overflow errors.

| # | Test | What It Validates |
|---|------|-------------------|
| H1 | Empty message list | Returns only overhead constants, not negative |
| H2 | Single system message | Overhead constants applied correctly per role |
| H3 | Multi-turn conversation | Token counts accumulate correctly across roles |
| H4 | Message with empty content | Role overhead is still counted |

**Why these matter:** Token estimation gates whether a plan can proceed. Wrong estimates cause silent truncation or unnecessary failures.

---

### I. Diff Generation — `app/server/diff/` — NO TESTS EXIST

`GetDiffs` and `GetDiffReplacements` produce unified diffs by writing to temp files and invoking git. These diffs are shown to users for review before changes are applied.

| # | Test | What It Validates |
|---|------|-------------------|
| I1 | Identical files produce empty diff | No false positives in the review surface |
| I2 | Single line change | Correct hunk output for minimal change |
| I3 | Multi-line addition | Hunk boundaries are correct |
| I4 | File deletion (empty new content) | Entire file shown as removed |
| I5 | Non-text or binary content | Does not crash |

**Why these matter:** Diffs are the primary review mechanism. Incorrect diffs hide or fabricate changes, undermining user trust in the tool.

---

### J. Syntax Validation — `app/server/syntax/` — NO TESTS EXIST

`ValidateFile` runs tree-sitter parsing to detect syntax errors in edited files before they are written to disk.

| # | Test | What It Validates |
|---|------|-------------------|
| J1 | Valid Go file passes | No false positives on correct code |
| J2 | Invalid Go file (missing brace) | Error reported with correct line number |
| J3 | Valid JavaScript file passes | Multi-language support works |
| J4 | Invalid JavaScript (unclosed string) | JS syntax error detected |
| J5 | Unsupported language | No crash; returns no errors |
| J6 | Empty file | Does not crash |

**Why these matter:** This is the last gate before a broken file is written to disk. Without it, syntactically invalid code is silently applied.

---

### K. API Retry & Error Handling — `app/cli/api/` — NO TESTS EXIST

The CLI retry transport uses exponential backoff with jitter for transient server errors (502, 503, 504). `HandleApiError` converts HTTP responses into structured errors.

| # | Test | What It Validates |
|---|------|-------------------|
| K1 | HTTP 502 triggers retry | Bad gateway is retried |
| K2 | HTTP 503 triggers retry | Service unavailable is retried |
| K3 | HTTP 504 triggers retry | Gateway timeout is retried |
| K4 | HTTP 400 does not retry | Client errors are not retried |
| K5 | HTTP 401 parsed as structured ApiError | Unauthorized is returned correctly |
| K6 | Max retries exhausted | Returns error after 3 attempts |
| K7 | Backoff duration increases | Exponential backoff is applied |

**Why these matter:** Retry logic is the difference between a transient hiccup and a user-visible failure. Retrying non-transient errors wastes time; not retrying transient errors causes failures that should be invisible.

---

### L. Comment Symbol Lookup — `app/server/syntax/` — NO TESTS EXIST

`GetCommentSymbols` returns language-specific comment syntax. The structured edit engine uses this to detect reference markers like `// ... existing code ...`.

| # | Test | What It Validates |
|---|------|-------------------|
| L1 | Go returns `//` | Single-line comment marker |
| L2 | Python returns `#` | Hash-based comment |
| L3 | JavaScript returns `//` | Same as Go |
| L4 | HTML returns `<!-- -->` | Block comment syntax |
| L5 | CSS returns `/* */` | Block comment syntax |
| L6 | Unknown language returns empty | Graceful fallback |

**Why these matter:** If the wrong comment syntax is returned, the edit engine fails to recognize reference comments and treats `// ... existing code ...` as literal content to write into the file.

---

### M. Remove Subtask Parsing — `app/server/model/parse/` — NO TESTS EXIST

`ParseRemoveSubtasks` is a sibling to `ParseSubtasks` but has zero test coverage. It parses the `### Remove Tasks` section that triggers file deletion.

| # | Test | What It Validates |
|---|------|-------------------|
| M1 | Empty input | No crash on missing section |
| M2 | Valid remove section with file paths | Correct file paths extracted |
| M3 | Missing Remove Tasks header | Returns nil gracefully |

**Why these matter:** Remove tasks delete files. An incorrectly parsed remove task either fails to clean up or targets the wrong file.

---

## 2. CI/CD: Linting & Formatting Integration

A new GitHub Actions workflow has been added at `.github/workflows/go-test-lint.yml`. It runs on every push to `main` and on every pull request targeting `main`.

The workflow uses a matrix strategy to test all three Go modules (`app/cli`, `app/server`, `app/shared`) in parallel. Each module runs the following steps in order:

| Step | Tool | What It Checks |
|------|------|----------------|
| 1 | `gofmt -l` | Formatting — fails if any `.go` file does not conform to standard `gofmt` output |
| 2 | `go vet ./...` | Static analysis — detects unreachable code, bad `printf` format verbs, shadowed variables, and other suspicious constructs |
| 3 | `go test ./...` | Unit tests — executes all test files in the module |

All three checks must pass for the workflow to succeed. A formatting or vet failure blocks the pull request.

---

## 3. Test Execution Results

*Executed on 2026-01-31 against commit `e2d77207` on branch `main`.*

---

### 3a. Summary

*Run: `2026-01-31` — commit `8460af0a` — all three modules (`app/cli`, `app/server`, `app/shared`)*

#### Unit Tests

| Metric | Count |
|--------|-------|
| Test functions executed | 6 |
| Total subtests defined in source | 77 |
| Subtests actually executed | 77 |
| Subtests silently skipped | 0 |
| **Passed** | **77** |
| **Failed** | **0** |
| Packages with at least one test file | 5 |
| Packages with zero test files | 22 |

All 77 subtests pass. The debug `only`/`Only` flags have been removed and three bugs in `bufferOrStream` (stop-sequence split, orphaned buffer flush, and opening-tag guard) have been fixed. 22 packages still have no test coverage at all (see gap analysis below).

#### Formatting (`gofmt`)

| Module | Status |
|--------|--------|
| `app/server` | PASS |
| `app/cli` | PASS |
| `app/shared` | PASS |

#### Static Analysis (`go vet`)

| Module | Status |
|--------|--------|
| `app/server` | PASS |
| `app/cli` | PASS |
| `app/shared` | PASS |

---

### 3b. Detailed Results by Test Function

---

#### TestParseSubtasks — `model/parse/` — 6 of 6 PASS

| Subtest | Result |
|---------|--------|
| empty\_input | PASS |
| single\_task\_without\_description | PASS |
| multiple\_tasks\_with\_descriptions\_and\_uses | PASS |
| alternative\_task\_header | PASS |
| tasks\_with\_empty\_lines\_between | PASS |
| single\_task\_from\_pong | PASS |

---

#### TestBufferOrStream — `model/plan/` — 25 of 25 PASS

**Fixed:** `only` flag removed. Three bugs in `bufferOrStream` fixed: (1) stop-sequence split applied to chunk instead of combined buffer+chunk; (2) orphaned stop-prefix buffer never flushed before tag detection; (3) opening-tag replacement guarded by `fileOpen` which was false for tags arriving via flushed buffer. A copy-paste typo in the #2 test case (`<Plandex` → `<Pland`) was also corrected.

| Subtest | Result |
|---------|--------|
| streams\_regular\_content | PASS |
| buffers\_partial\_opening\_tag | PASS |
| converts\_opening\_tag | PASS |
| converts\_opening\_tag\_without\_awaitingOpeningTag | PASS |
| buffers\_partial\_backticks | PASS |
| escapes\_backticks\_in\_content | PASS |
| buffers\_partial\_closing\_tag | PASS |
| buffers\_full\_closing\_tag\_with\_file\_open | PASS |
| replaces\_full\_closing\_tag\_with\_file\_closed | PASS |
| replaces\_full\_closing\_tag\_with\_file\_closed\_and\_awaiting\_backticks | PASS |
| handles\_single\_backticks | PASS |
| handles\_close\_and\_re-open\_backticks | PASS |
| buffers\_for\_end\_of\_file\_operations | PASS |
| replaces\_full\_end\_of\_file\_operations\_tag | PASS |
| buffers\_for\_end\_of\_file\_operations\_with\_partial\_tag | PASS |
| replaces\_end\_of\_file\_operation\_closing\_partial\_tag | PASS |
| buffers\_for\_partial\_opening\_tag\_with\_no\_file\_path\_label | PASS |
| continues\_buffering\_partial\_opening\_tag\_with\_no\_file\_path\_label | PASS |
| replaces\_opening\_tag\_with\_no\_file\_path\_label\_when\_it\_completes | PASS |
| replaces\_full\_opening\_tag\_without\_file\_path\_label | PASS |
| stop\_tag\_entirely\_in\_one\_chunk | PASS |
| stop\_tag\_split\_across\_two\_chunks\_(prefix\_+\_rest) | PASS |
| stop\_tag\_split\_across\_two\_chunks\_(completes) | PASS |
| stop\_prefix\_turns\_out\_to\_be\_different\_tag | PASS |
| stop\_prefix\_turns\_out\_to\_be\_different\_tag\_#2 | PASS |

---

#### TestStructuredReplacements — `syntax/` — 22 of 22 PASS

| Subtest | Result |
|---------|--------|
| single\_reference\_in\_function | PASS |
| bad\_formatting | PASS |
| multiple\_refs\_in\_class/nested\_structures | PASS |
| code\_removal\_comment | PASS |
| multiple\_code\_removal\_comments | PASS |
| json\_update\_with\_reference\_comments | PASS |
| method\_replacement\_with\_context | PASS |
| nested\_class\_methods\_update | PASS |
| update\_with\_trailing\_commas | PASS |
| multiple\_structural\_updates | PASS |
| json\_multi-level\_update | PASS |
| json\_multi-level\_update\_2 | PASS |
| scala\_complex\_structures | PASS |
| top-level\_ambiguous | PASS |
| top-level\_with\_anchors | PASS |
| clean\_up\_extraneous\_newlines | PASS |
| insert\_between\_non-adjacent\_anchors | PASS |
| insert\_with\_reference\_and\_non-adjacent\_anchors | PASS |
| replacement\_with\_removal\_outside\_of\_single\_line\_range | PASS |
| replacement\_with\_removal\_inside\_of\_multi\_line\_range | PASS |
| add\_to\_end\_with\_full\_file\_included | PASS |
| add\_to\_beginning\_with\_full\_file\_included | PASS |

---

#### TestFindUniqueReplacement — `syntax/` — 10 of 10 PASS

| Subtest | Result |
|---------|--------|
| perfect\_single\_match | PASS |
| match\_with\_error\_in\_middle | PASS |
| multiple\_instances\_but\_unique\_boundaries | PASS |
| no\_match\_at\_all | PASS |
| multiple\_complete\_matches | PASS |
| ambiguous\_boundaries | PASS |
| match\_with\_very\_different\_middle | PASS |
| unique\_match\_near\_identical\_text | PASS |
| identical\_start/end\_patterns | PASS |
| overlapping\_patterns | PASS |

---

#### TestReplyParser — `types/` — 10 of 10 PASS

**Fixed:** `Only` flag removed. All 10 examples now execute and pass.

| Example | Result |
|---------|--------|
| Example\_1 | PASS |
| Example\_2 | PASS |
| Example\_3 | PASS |
| Example\_4 | PASS |
| Example\_5 | PASS |
| Example\_6 | PASS |
| Example\_7 | PASS |
| Example\_8 | PASS |
| Example\_9 | PASS |
| Example\_10 | PASS |

---

#### TestStripAddedBlankLines — `utils/` — 4 of 4 PASS

| Subtest | Result |
|---------|--------|
| no\_change | PASS |
| leading\_newline\_added | PASS |
| trailing\_newline\_added | PASS |
| both\_ends,\_keep\_original\_padding | PASS |

---

### 3c. Packages with Zero Test Coverage

These 22 packages contain no `_test.go` files and are entirely unvalidated:

| Package | Risk | Key Untested Logic |
|---------|------|--------------------|
| `db` | HIGH | All database helpers, lock acquisition/release, transaction rollback, operation queue |
| `diff` | HIGH | Diff generation and hunk parsing shown to users |
| `handlers` | HIGH | All HTTP endpoint request/response logic |
| `model` | HIGH | Client factory, error classification, token estimation, summarization |
| `cli/api` | HIGH | HTTP retry transport, all 80+ API method calls |
| `cli/auth` | HIGH | Token management, auth refresh, session handling |
| `cli/lib` | HIGH | Context loading, plan execution, git operations |
| `syntax/file_map` | MEDIUM | Tree-sitter file mapping for 30+ languages |
| `shutdown` | MEDIUM | Graceful shutdown and orphaned lock cleanup |
| `email` | MEDIUM | Email delivery |
| `hooks` | MEDIUM | Hook invocation system |
| `cli/cmd` | MEDIUM | 41 CLI command handlers |
| `cli/fs` | MEDIUM | Filesystem traversal and exclusion |
| `cli/stream` | MEDIUM | Client-side SSE stream handling |
| `plandex-shared` | MEDIUM | All shared types and utility functions |
| `model/prompts` | LOW | Prompt templates (output-dependent, hard to unit test) |
| `host` | LOW | IP and host resolution |
| `notify` | LOW | Error notification dispatch |
| `routes` | LOW | Route registration (declarative, no logic) |
| `setup` | LOW | One-time server initialization |
| `plandex-server` (main) | LOW | Entry point only |
| `plandex-cli` (main) | LOW | Entry point only |

---

### 3d. Urgency Assessment

Issues are ranked by the combination of likelihood of causing a real bug and severity if that bug occurs.

---

#### CRITICAL — RESOLVED

All previously-critical issues have been fixed in commit `8460af0a`:

| Issue | Resolution |
|-------|------------|
| `only: true` flag in TestBufferOrStream | Flag removed. Three bugs in `bufferOrStream` discovered and fixed (stop-sequence split, orphaned buffer flush, opening-tag guard). Test-case typo (`<Plandex` → `<Pland`) corrected. All 25 subtests pass. |
| `Only: true` flag in TestReplyParser | Flag removed. All 10 examples pass. |

---

#### HIGH — Address Before Next Release

| Issue | Why It Is High Priority |
|-------|--------------------------|
| No tests for `ClassifyModelError` / `ClassifyErrMsg` | Error misclassification causes silent failures or wasteful retries across all AI providers. This function is called on every API error. |
| No tests for `ValidateFile` | This is the only automated gate preventing syntactically broken files from being written to disk. |
| No tests for `GetDiffs` / `GetDiffReplacements` | Diffs are the user's primary review surface before accepting changes. Incorrect diffs hide or fabricate modifications. |
| No tests for CLI retry transport | Retry behavior determines whether transient errors are invisible or user-visible. The backoff logic has no coverage. |
| No tests for `ParseRemoveSubtasks` | File deletion is irreversible. Zero test coverage on the parsing that triggers it. |

---

#### MEDIUM — Address in Near Term

| Issue | Why It Is Medium Priority |
|-------|----------------------------|
| No tests for `GetCommentSymbols` | Wrong comment symbol breaks reference detection for that language, causing edit failures silently. |
| No tests for `GetMessagesTokenEstimate` | Incorrect estimates cause context overflow or wasted budget. |
| No tests for `cli/auth` token refresh | Auth failures are highly visible to users but the refresh logic is untested. |
| Missing edge cases in UniqueReplacement (empty string, multiline) | Edge cases in fuzzy matching can cause edits to the wrong block. |
| Missing CRLF test in StripAddedBlankLines | Windows users would hit this silently. |
| `db` package has no tests | Lock and transaction logic is complex; requires integration test infrastructure (PostgreSQL) but the logic warrants it. |

---

#### LOW — Backlog

| Issue | Why It Is Low Priority |
|-------|-------------------------|
| No tests for `handlers/` | Requires HTTP test server; the handler logic is thin delegation to db and model layers. |
| No tests for `model/prompts/` | Prompt content is output-dependent; unit tests provide limited signal. |
| Missing Python and Rust structured edit tests | Covered by the generic engine; language-specific risk is lower. |
| No tests for entry point files | Entry points contain no business logic. |

---

### 3e. Lint & Format Findings Detail

These are the exact outputs produced by the CI pipeline steps. Each must be resolved for the workflow to pass.

#### gofmt — Formatting

All modules pass. Previously flagged files were resolved in commit `49ee57ca`:
- `app/server/model/prompts/describe.go` — removed leading and trailing blank lines.
- `app/cli/lib/log_format.go` — replaced blank lines containing trailing tabs with truly empty lines.

#### go vet — Static Analysis

All modules pass. Previously flagged context leaks were resolved in commit `49ee57ca`:
- `app/server/model/plan/build_structured_edits.go` — added `defer cancelBuild()` immediately after `context.WithCancel`. The cancel was previously only reachable through the `buildRace` path; the `autoApplyIsValid` path and the error return at line 200 both leaked the context.
- `app/cli/cmd/browser.go` — captured the cancel function from `context.WithTimeout` (was discarded with `_`) and deferred it.

#### Compiler Warning (non-blocking)

During the server build, the `go-tree-sitter` C dependency emits:
```
parser.c:254:18: warning: null character(s) preserved in string literal [-Wnull-character]
```
This is in a vendored third-party C file (`github.com/smacker/go-tree-sitter/lua`). It does not affect correctness and is not something this repo controls. It does not fail the build or any CI step.
