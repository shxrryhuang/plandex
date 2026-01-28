# Atomic Patch Application & Rollback

## Overview

Every time Plandex applies a patch (the set of file writes produced by a
plan), it wraps the entire operation in a **FileTransaction**.  This means
patches are either applied in full or not applied at all — the user's working
directory is never left in a half-written state.

The same transactional guarantee extends to workspace isolation: both
`ApplyFilesToWorkspace` and `Manager.Commit` route their writes through a
`FileTransaction` (or its `CommitTransaction` equivalent) so that workspace
→ project copies are also all-or-nothing.

---

## How It Works

### 1. Staging (snapshot capture)

Before any file is written, the transaction captures a **snapshot** of each
target path:

* If the file exists, its current content, permissions, and SHA-256 hash are
  recorded both in memory and on disk (`<project>/.plandex/snapshots/<txId>/`).
* If the file does not exist, a metadata marker records that fact.
* Files whose content is identical to what the plan wants to write are
  **skipped entirely** — they are never staged or written.  The content
  equality check happens *before* `ModifyFile` / `CreateFile` is called, so
  unchanged files consume no snapshot space and produce no WAL entries.

Snapshots are persisted to disk *before* any writes occur.  Every snapshot
produces two files:

| File | Purpose |
|---|---|
| `<hash>.snapshot` | Raw original file content |
| `<hash>.meta.json` | Metadata: path, existed flag, permissions, content hash |

This is the key durability guarantee: even a hard crash between staging and
applying leaves the snapshots intact for recovery.

### 2. Applying (sequential writes)

Operations are applied **one at a time**, in the order they were staged.
After each successful write, the transaction's Write-Ahead Log (WAL) records
an `op_complete` entry.  If a write fails:

1. The transaction immediately enters **rolling back** state.
2. Every previously applied operation is reverted in reverse order using its
   snapshot.
3. The WAL records a `tx_rollback` entry.
4. The caller receives an error that includes which file failed and confirms
   that all changes have been undone.

### 3. Committing

After all operations succeed, the transaction is **committed**:

* The WAL records `tx_commit`.
* Snapshot files and WAL are cleaned up (no longer needed).
* The caller receives a success response and the list of files that changed.
* A `PatchStatusReporter` (if wired) receives a `PhaseDone` event.

### 4. Crash Recovery

If Plandex exits (crash, kill signal, power loss) between staging and
committing:

1. On next startup, orphaned WAL files under `.plandex/wal/` are detected.
2. `RecoverTransaction` replays the WAL to reconstruct which operations were
   applied vs. pending.
3. `loadSnapshotsFromDisk()` reads every `.meta.json` in the snapshot
   directory and reloads the corresponding `.snapshot` content into the
   in-memory `Snapshots` map.
4. The incomplete transaction is **automatically rolled back**, restoring
   every file to its pre-transaction state.

---

## Rollback Mechanics

### Transaction-internal rollback (automatic)

When a write fails during `ApplyFiles`, the `FileTransaction` rolls back
internally using its own snapshots.  The caller receives an error; no
`ApplyRollbackPlan` is returned.  The project is already restored.

### Manual rollback via `ApplyRollbackPlan` (post-exec)

When all file writes succeed but a subsequent `_apply.sh` script fails (or
the user cancels), the caller holds an `ApplyRollbackPlan` built from the
transaction's snapshots.  Invoking `Rollback(plan, verbose)` processes the
plan in **three sequential phases**:

| Phase | What it does | Example output |
|---|---|---|
| **restore** | Writes pre-apply content back from snapshots for every file that existed before | `  ↩ restored  src/main.go` |
| **remove** | Deletes files that were *created* by the apply | `  ↩ removed   src/generated.go` |
| **sweep** | Removes files that appeared on disk after the apply but are not in `PreviousProjectPaths` (side-effect stragglers) | `  ↩ swept     src/stale.go` |

On failure within any phase the error is collected but processing continues
— a partially-restored project is worse than a partially-failed rollback.
A summary footer reports the outcome:

```
🚫 Rolled back 4 file(s)                       # all succeeded
⚠  Rollback completed with 1 error(s), 3 file(s) restored   # partial
```

### Workspace rollback (`RollbackWorkspace`)

`RollbackWorkspace` mirrors the restore + remove phases of `Rollback` but
additionally performs **surgical tracking updates**: only the workspace
metadata keys touched by *this* apply's rollback plan are deleted.  Earlier
workspace modifications that were not part of this batch remain intact.

Output format is identical to the main rollback (coloured per-file lines,
error collection, summary footer), with a workspace-specific header:

```
🔄 Rolling back workspace changes…
  ↩ restored  src/config.go
  ↩ removed   src/temp.go
🚫 Rolled back 2 workspace file(s)
```

### Exec-script failure handler (`apply_exec.go`)

When an exec script fails, the user is presented with four choices:

| Choice | What happens |
|---|---|
| Debug and retry once | `Rollback` restores pre-apply state, then re-tells + re-applies |
| Debug in full auto mode | Same, but resets the attempt counter and enables full-auto flags |
| Rollback changes and exit | `Rollback` runs with verbose output, then `os.Exit(1)` |
| Apply changes and exit | Keeps the file changes as-is |

In every path that invokes `Rollback`, the function itself owns the banner
(`🔄 Rolling back changes…`) and per-file output.  Call sites do not print
their own header — this prevents duplicate banners.

---

## CLI Status Output

The user sees structured lifecycle events during patch application:

| Phase | What the user sees |
|---|---|
| `preparing` | `📋 Staging changes (capturing snapshots)…` (when exec script present) or `📋 Applying changes atomically…` |
| `staging` | Per-file reporter events: skip / modify / create |
| `applying` | Sequential file writes |
| `committing` | `✔ Changes staged (all-or-nothing applied to disk)` |
| `rolling_back` | Per-file `↩` or `✗` lines via `Rollback()` |
| `done` | `✅ Applied changes, N file(s) updated` or rollback summary |

Each event includes a timestamp so progress bars or duration reporting can be
built on top.

### `PatchStatusReporter` summary

`LoggingReporter.Summary()` returns aggregate counts after a patch run.  It
keeps only the *last* event per path and classifies it exactly once:

* Error field non-empty → **Failed**
* Phase is `PhaseStaging` (a skip event, never followed by applying) → **Skipped**
* Otherwise → **Applied**

This avoids inflating counts from intermediate events (e.g. a file that
emits staging → applying → done contributes only one count).

---

## Workspace Isolation

### Apply (`ApplyFilesToWorkspace`)

File writes are routed to the isolated workspace directory, not the main
project.  The function uses a `FileTransaction` with the workspace's files
directory as the base so snapshots capture workspace state.

A `pendingTracking` slice accumulates metadata entries (modify / create /
delete) during staging.  The tracking maps (`ModifiedFiles`, `CreatedFiles`,
`DeletedFiles`) are updated only after *all* writes succeed and the
transaction commits.  A failure at any point rolls back writes and leaves
the tracking maps untouched — consistent with pre-apply state.

### Commit (`Manager.Commit` / `CommitTransaction`)

`Manager.Commit` wraps the workspace → project copy in a `CommitTransaction`:

1. **Stage** — read each modified/created/deleted file from the workspace;
   capture snapshots of the *project* files (not workspace files).
2. **Apply** — write sequentially to the project directory.
3. **Rollback on failure** — reverse-order restore using project snapshots;
   project directory is left unchanged, workspace remains intact for retry.
4. **Finalise** — mark workspace as committed, unregister from references.

### Discard

`Manager.Discard` simply marks the workspace as discarded, unregisters it,
and optionally removes the workspace directory.  No file writes to the
project occur.

---

## Guarantees

| Property | Guarantee |
|---|---|
| Atomicity | All files in a patch are written, or none are. |
| Consistency | No file is left in a partial/corrupt state.  Workspace metadata stays in sync with disk. |
| Durability | Snapshots (`.snapshot` + `.meta.json`) are flushed to disk before any write begins. |
| Isolation | Concurrent runs use separate transactions; workspace isolation prevents conflicts at the file-system level. |
| Idempotency | Unchanged files are skipped; re-running a patch has no additional effect. |
| Surgical rollback | `RollbackWorkspace` deletes only the tracking keys touched by the current rollback plan; earlier workspace state survives. |

---

## Limitations

1. **Snapshot size**.  Each modified file's full content is stored in the
   snapshot directory.  For very large generated files (hundreds of MB) this
   temporarily doubles disk usage for that file.  Snapshots are cleaned up
   immediately on commit or rollback.

2. **External modifications between staging and applying**.  The snapshot
   captures the file state at *staging* time.  If another process writes to
   the same file after staging but before the transaction applies, that
   external change will be overwritten.  Rolling back restores the *snapshot*
   state (pre-staging), not the external write.  For repos under version
   control, `git diff` after apply will surface any such conflicts.

3. **Directory-level atomicity**.  Individual file writes are atomic at the
   OS level (write-then-rename pattern is not used; `os.WriteFile` is used
   directly).  On some filesystems a crash *during* a single `WriteFile` call
   could theoretically corrupt that one file.  The snapshot guarantees
   recovery on the *next* Plandex run, but the file may be unreadable until
   then.

4. **Exec scripts run after file writes**.  When a plan includes an
   `_apply.sh` script, files are written first, then the script executes.  If
   the script fails, the user is prompted to keep or roll back the file
   changes.  The script's own side effects (network calls, package installs)
   are **not** rolled back — only file changes are.  The rollback path prints
   per-file status lines (restore / remove / sweep) so the user can verify
   exactly what was undone.

5. **Workspace apply is sequential and transactional**.  `ApplyFilesToWorkspace`
   processes files one at a time through a `FileTransaction`.  Workspace
   metadata (tracking maps for modified/created/deleted files) is updated only
   after *all* writes succeed, keeping the workspace state consistent on
   failure.  Rollback (`RollbackWorkspace`) surgically removes only the
   tracking entries that this apply touched — earlier workspace modifications
   remain intact.

6. **Workspace commit is also transactional**.  When workspace isolation is
   enabled, `workspace commit` wraps the workspace→project copy in its own
   `CommitTransaction` with the same snapshot/rollback guarantees.  A mid-commit
   failure leaves the project directory unchanged and the workspace intact
   for retry.

7. **Snapshot-missing is a narrow failure window**.  Because snapshots are
   persisted to disk before any write begins, the `UnrecoverableSnapshotMissing`
   condition can only occur if the process crashes *during* the
   `captureSnapshot` write itself, if the `.plandex/snapshots` directory is
   manually deleted, or if storage corruption occurs after the snapshot was
   written.  "Snapshot pruned to save space" is no longer a valid cause.

---

## File Layout

```
<project>/
  .plandex/
    wal/
      <txId>.wal          # Write-ahead log (newline-delimited JSON)
    snapshots/
      <txId>/
        <hash>.snapshot   # Original file content
        <hash>.meta.json  # Snapshot metadata (path, existed, mode, hash)
```

Both the WAL directory and snapshot directory are created at transaction
`Begin()` and removed at `Commit()` or `Rollback()`.  Orphaned directories
indicate an incomplete transaction that will be recovered on next startup.

```
~/.plandex-home-v2/
  workspaces/
    <wsId>/
      workspace.json        # Workspace metadata + tracking maps
      files/                # Isolated copy of modified project files
      checkpoints/          # Named checkpoints within the workspace
      logs/                 # Operation log
```

The workspace directory lives in the user's home Plandex directory, not
inside the project.  This keeps the project tree clean and prevents
workspace metadata from being committed to version control.
