# Workspace Isolation System

## Overview

The Workspace Isolation System provides a safe environment for Plandex to execute file edits, patches, and shell commands without risking accidental corruption of your main project files. All changes are made in an isolated workspace until you explicitly commit them back to your project.

## Key Benefits

- **Safety**: Your main project files are protected until you explicitly apply changes
- **Review**: Preview and review all changes before they affect your project
- **Rollback**: Atomic, transactional rollback restores the workspace to its pre-apply state — all-or-nothing, with per-file status output
- **Resume**: Workspace state persists across sessions for continuity
- **Recovery**: Automatic recovery from crashes and partial runs

## Architecture

```
User's Project/                      Isolated Workspace
├── src/                             ~/.plandex-home-v2/workspaces/{ws-id}/
│   ├── main.go     ────────────→    ├── workspace.json    (metadata)
│   └── utils.go                     ├── files/            (modified files)
├── .plandex-v2/                     │   └── src/
│   └── workspaces-v2.json           │       └── main.go   (copy-on-write)
└── ...                              ├── checkpoints/      (recovery points)
                                     └── logs/             (audit trail)
```

### Copy-on-Write Strategy

The workspace uses a **copy-on-write (CoW)** approach for efficiency:

1. **Unchanged files**: Read directly from your project (no duplication)
2. **Modified files**: Copied to workspace on first modification
3. **New files**: Created directly in workspace
4. **Deleted files**: Marked in metadata (original untouched)

This means a large repository won't consume excessive disk space - only the files you actually modify are copied.

## User Guide

### Workspace Lifecycle

1. **Creation**: A workspace is automatically created when you first apply changes to a plan
2. **Active**: Files edited by Plandex go into the workspace, not your main project
3. **Review**: Use `plandex workspace diff` to see what changed
4. **Commit**: Apply changes to your main project with `plandex workspace commit`
5. **Discard**: Reject changes with `plandex workspace discard`

### CLI Commands

#### View Workspace Status

```bash
plandex workspace status
# or
plandex ws st
```

Shows:
- Workspace ID and state
- Plan and branch information
- List of modified, created, and deleted files
- Timestamps

#### Commit Changes to Main Project

```bash
plandex workspace commit
# or
plandex workspace commit --force  # Skip confirmation
```

This applies all workspace changes to your main project via a `CommitTransaction`:

1. **Stage** — snapshots of the current *project* files are captured to disk (WAL + `.snapshot` + `.meta.json`)
2. **Apply** — modified/created/deleted files are written sequentially to the project directory
3. **Rollback on failure** — if any write fails, all previously applied operations are reverted in reverse order; the project directory is left unchanged and the workspace remains intact for retry
4. **Finalise** — workspace is marked as committed and unregistered

The entire operation is all-or-nothing: either every file lands in the project or none do.

#### Discard Workspace Changes

```bash
plandex workspace discard
# or
plandex workspace discard --force  # Skip confirmation
```

Rejects all changes without affecting your main project.

#### View Differences

```bash
plandex workspace diff
```

Shows detailed differences between workspace and main project.

#### List All Workspaces

```bash
plandex workspace list
# or
plandex ws ls
```

Shows all workspaces for the current project with their status.

#### Clean Up Stale Workspaces

```bash
plandex workspace clean
plandex workspace clean --all       # Clean all eligible
plandex workspace clean --days 14   # Change stale threshold
```

Removes:
- Committed workspaces (changes already applied)
- Discarded workspaces
- Stale workspaces (not accessed for 7+ days by default)

### CLI Indicators

When a workspace is active, you'll see visual indicators:

**Status Banner:**
```
[WORKSPACE: abc123ef] Active - changes isolated
  Changes: 3 modified, 1 created, 0 deleted
```

**REPL Prompt Prefix:**
```
[ws:abc123] plandex >
```

**Status Output:**
```
Workspace Status
================
ID:          ws-abc123ef-4567-890a
State:       [ACTIVE]
Plan:        my-plan
Branch:      main
Created:     2024-01-15T10:00:00Z
Last Access: 5 mins ago

Modified Files:
  M  src/main.go
  M  src/utils.go

Created Files:
  +  src/new_feature.go

Deleted Files:
  -  src/deprecated.go
```

### Apply Command Integration

The standard `plandex apply` command works seamlessly with workspaces:

```bash
# Apply changes (goes to workspace by default)
plandex apply

# Apply directly to main project (bypass workspace)
plandex apply --no-workspace

# Apply and immediately commit to main project
plandex apply --commit
```

## Configuration

### Default Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `Enabled` | `true` | Master switch for workspace isolation |
| `AutoCommitThreshold` | `0` | Auto-commit after N applies (0 = disabled) |
| `RetainDiscarded` | `false` | Keep discarded workspaces for review |
| `MaxStaleWorkspaces` | `10` | Maximum stale workspaces to keep |
| `StaleAfterDays` | `7` | Days before workspace is considered stale |
| `CleanupBatchSize` | `5` | Max workspaces to clean per run |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `PLANDEX_WORKSPACE` | Path to current workspace directory |
| `PLANDEX_WORKSPACE_ID` | Current workspace ID |
| `PLANDEX_WORKSPACE_FILES` | Path to workspace files directory |

These are set automatically when running shell commands in workspace context.

## Failure Handling

### Crash Recovery

If Plandex crashes during an apply or commit operation:

1. On next startup, orphaned WAL files under `.plandex/wal/` are detected
2. `RecoverTransaction()` replays each WAL to determine which operations completed vs. which were pending
3. `loadSnapshotsFromDisk()` reloads every `.meta.json` and its `.snapshot` content into memory
4. The incomplete transaction is automatically rolled back, restoring every file to its pre-transaction state
5. User is notified of any recovered workspaces

Because snapshots are flushed to disk *before* any write begins, even a hard crash between staging and applying leaves all recovery data intact.

### User Cancellation (Ctrl+C)

When you cancel an operation:

1. Current workspace state is saved
2. Workspace remains in `active` state
3. You can resume or discard later

### Partial Runs

The operation log tracks all started/completed operations:
- `GetPendingOperations()` identifies incomplete work
- Resume can replay or skip partial operations

## Recovery Procedures

### Manual Recovery

If automatic recovery fails:

1. Check workspace status:
   ```bash
   plandex workspace list
   ```

2. Look for workspaces in `recovering` state

3. Options:
   - Try to commit if changes are valid: `plandex workspace commit`
   - Discard if changes are corrupted: `plandex workspace discard`
   - Manually inspect files in `~/.plandex-home-v2/workspaces/{ws-id}/files/`

### Workspace Files Location

```
~/.plandex-home-v2/workspaces/{workspace-id}/
├── workspace.json      # Metadata (state, file tracking)
├── recovery.json       # Present during in-flight operations
├── files/              # Modified/created files
│   └── {relative-path} # Mirrors project structure
├── checkpoints/        # Named recovery points
│   └── {name}.json
└── logs/
    └── operations.log  # Audit trail (JSON lines)
```

## Tradeoffs

### Performance

| Aspect | Impact | Notes |
|--------|--------|-------|
| First file modification | Slight delay | File copied from original |
| Subsequent modifications | Minimal | Writes directly to workspace |
| Reading unmodified files | None | Direct access to original |
| Commit operation | O(n) where n = changed files | Copies all changes to project |

### Disk Usage

| Scenario | Disk Usage |
|----------|------------|
| No changes | ~1 KB (metadata only) |
| Small changes | Proportional to changed files |
| Large file modified | Full file copied |
| Many small changes | Each file copied once |

**Tip**: The workspace only stores files you modify, not your entire project.

### Limitations

1. **Shell commands**: Currently execute in original project directory with environment variables indicating workspace context. Full sandboxing requires OS-level support (overlayfs on Linux).

2. **Binary files**: Tracked by hash only; full diff display not supported.

3. **Symlinks**: Original symlinks are followed; workspace does not preserve symlink structure.

4. **Concurrent workspaces**: One workspace per plan/branch combination. Switching branches preserves the workspace.

5. **Large files**: Files over 1MB may impact performance during commit.

## Troubleshooting

### "No active workspace" error

**Cause**: No workspace exists for current plan/branch.

**Solution**: Workspaces are created automatically on first `plandex apply`. Run an apply operation first.

### Workspace stuck in "recovering" state

**Cause**: Crash during operation, recovery couldn't complete.

**Solution**:
1. Inspect `~/.plandex-home-v2/workspaces/{ws-id}/`
2. Remove `recovery.json` if present
3. Manually verify/fix files in `files/` directory
4. Run `plandex workspace discard` to start fresh

### Disk space concerns

**Solution**:
```bash
# Clean up old workspaces
plandex workspace clean --all

# Remove specific workspace manually
rm -rf ~/.plandex-home-v2/workspaces/{ws-id}
```

### Changes not appearing in main project

**Cause**: Changes are in workspace, not committed.

**Solution**:
```bash
plandex workspace status  # Verify changes exist
plandex workspace commit  # Apply to main project
```

### Need to bypass workspace temporarily

**Solution**:
```bash
plandex apply --no-workspace
```

## API Reference

### Workspace States

| State | Description |
|-------|-------------|
| `pending` | Created but not yet used |
| `active` | In use, changes being tracked |
| `committed` | Changes applied to main project |
| `discarded` | Changes rejected |
| `recovering` | Crash recovery in progress |

### File Tracking

Files are tracked with:
- `originalHash`: SHA256 of original content
- `currentHash`: SHA256 of current content
- `mode`: File permissions
- `size`: File size in bytes
- `modifiedAt`: Timestamp of last modification

### Operation Log Format

```json
{"timestamp":"2024-01-15T10:30:00Z","operation":"apply_file","path":"src/main.go","status":"started"}
{"timestamp":"2024-01-15T10:30:01Z","operation":"apply_file","path":"src/main.go","status":"completed","checksum":"sha256:abc..."}
```

## Integration with Plandex Features

### Resume/Retry

- Workspaces persist across sessions
- Resuming a plan uses the same workspace
- Retry operations work within existing workspace

### Rollback

- `RollbackWorkspace()` restores workspace files using snapshots captured during the apply
- Operates in two phases: **restore** (revert files that existed before) then **remove** (delete files newly created by the apply)
- Workspace tracking is updated *surgically* — only the entries touched by this apply's rollback plan are removed; earlier workspace modifications remain intact
- Per-file coloured output: `↩ restored` / `↩ removed` (green) or `✗ restore` / `✗ remove` (red), with a summary footer
- Does not affect the main project unless workspace has been committed
- Original project remains pristine

### Branches

- Each plan/branch has its own workspace
- Switching branches preserves workspace state
- Independent workspace per branch allows parallel work

## Future Enhancements

Planned improvements:

1. **Full sandboxing**: Use overlayfs on Linux for complete isolation
2. **Partial commit**: Select specific files to commit
3. **Workspace sharing**: Export/import workspace state
4. **Visual diff**: Rich diff display in terminal
5. **Auto-cleanup**: Configurable automatic cleanup policies

## Contributing

The workspace isolation system is implemented in:

- `app/cli/workspace/` - Core workspace package
  - `workspace.go` - Types and data structures
  - `manager.go` - Lifecycle management
  - `cow.go` - Copy-on-write file access
  - `recovery.go` - Crash recovery
- `app/cli/cmd/workspace.go` - CLI commands
- `app/cli/term/workspace_indicators.go` - UI components
- `app/cli/lib/workspace_apply.go` - Apply integration
- `app/cli/fs/workspace_paths.go` - Path resolution

See the source code for detailed implementation notes.
