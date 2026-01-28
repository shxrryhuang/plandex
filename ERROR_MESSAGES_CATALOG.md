# Transactional Apply - Error Messages Catalog

This document catalogs all error messages that users may encounter when using the transactional patch application system.

## Table of Contents
1. [User-Facing CLI Messages](#user-facing-cli-messages)
2. [Transaction Lifecycle Errors](#transaction-lifecycle-errors)
3. [File Operation Errors](#file-operation-errors)
4. [Staging Errors](#staging-errors)
5. [WAL and Snapshot Errors](#wal-and-snapshot-errors)
6. [Rollback Errors](#rollback-errors)
7. [Error Recovery Guide](#error-recovery-guide)

---

## User-Facing CLI Messages

These are the primary messages users will see in the terminal.

### Success Messages

```
✅ All changes committed successfully
```
**When:** All file operations completed successfully and transaction committed.

```
📦 Staging changes...
```
**When:** Beginning to prepare operations for transactional apply.

```
🔄 Applying changes [N/Total] filename
```
**When:** Applying each file sequentially during transaction.

### Rollback Messages

```
🚫 Rolled back N applied file changes
   All files have been restored to their original state
```
**When:** Automatic rollback occurred after some files were applied but an error occurred.
**User Action:** Check the error message above this for the cause. All changes have been reverted.

```
   No files were modified
```
**When:** Rollback occurred before any files were successfully applied.
**User Action:** Check the error message for why the transaction failed to start.

### Error Messages

```
❌ Error: [error details]
```
**When:** User-declined script execution in transactional mode.
**User Action:** If you declined the script execution, the file changes have been rolled back.

```
Transaction failed: [error details]
```
**When:** The transaction failed for any reason (see specific errors below).
**User Action:** Read the specific error, fix the issue, and retry `plandex apply --tx`.

---

## Transaction Lifecycle Errors

### 1. Transaction Initialization

```
failed to begin transaction: transaction already started (state: Active)
```
**Cause:** Attempted to start a transaction when one is already active.
**Resolution:** This is an internal error. Report as a bug if seen.

```
failed to begin transaction: failed to create snapshot directory: [OS error]
```
**Cause:** Cannot create `.plandex/snapshots/<txid>/` directory.
**Common Reasons:**
- Insufficient permissions in project directory
- Disk full
- Read-only filesystem

**Resolution:**
```bash
# Check permissions
ls -la .plandex/
chmod u+w .plandex/

# Check disk space
df -h .

# Check if filesystem is read-only
touch test.txt && rm test.txt
```

```
failed to begin transaction: failed to create WAL directory: [OS error]
```
**Cause:** Cannot create `.plandex/wal/` directory.
**Resolution:** Same as snapshot directory error above.

### 2. Transaction Commit

```
cannot commit: transaction not active (state: [state])
```
**Cause:** Attempted to commit when transaction is not active.
**Possible States:** `Idle`, `RolledBack`, `Committed`
**Resolution:** This is an internal error. Report as a bug.

```
cannot commit: operation N is still pending
```
**Cause:** Trying to commit when some operations haven't been applied yet.
**Resolution:** This is an internal error indicating incomplete operation application.

```
cannot commit: operation N failed
```
**Cause:** Trying to commit when some operations failed.
**Resolution:** This should trigger automatic rollback instead. Report as a bug if seen.

---

## File Operation Errors

### 1. File Write Errors

```
failed to write file: [OS error]
```
**Cause:** Cannot write to target file during operation application.
**Common Reasons:**
- Permission denied (file or parent directory)
- Disk full
- Path too long
- Invalid filename characters

**Resolution:**
```bash
# Check file permissions
ls -la path/to/file

# Check parent directory permissions
ls -la path/to/

# Fix permissions
chmod u+w path/to/file
chmod u+w path/to/

# Check disk space
df -h .

# Check for invalid characters in filename
# On Windows: < > : " / \ | ? *
# Generally problematic: null bytes, control characters
```

### 2. Directory Creation Errors

```
failed to create directory: [OS error]
```
**Cause:** Cannot create parent directory for nested file.
**Common Reasons:**
- Permission denied on parent path
- Path component is a file (not a directory)
- Path too long

**Resolution:**
```bash
# Check parent path
ls -la path/to/parent/

# Check if path component is a file
file path/to/parent/some_component

# Fix permissions
chmod u+w path/to/parent/

# If component is a file, remove it first
rm path/to/parent/conflicting_file
```

### 3. File Delete Errors

```
failed to delete file: [OS error]
```
**Cause:** Cannot delete file during delete operation.
**Common Reasons:**
- Permission denied
- File is locked by another process
- File doesn't exist (should not happen with snapshots)

**Resolution:**
```bash
# Check file permissions
ls -la path/to/file

# Check if file is open
lsof path/to/file  # macOS/Linux
handle.exe path/to/file  # Windows

# Fix permissions
chmod u+w path/to/file

# Close applications using the file
```

### 4. File Rename Errors

```
failed to rename file: [OS error]
```
**Cause:** Cannot rename file during rename operation.
**Common Reasons:**
- Permission denied
- Source or destination locked
- Cross-device rename (different filesystems)

**Resolution:**
```bash
# Check permissions on both paths
ls -la source/path
ls -la dest/path

# Check filesystem
df -h source/path
df -h dest/path

# If cross-device, this is unsupported
# Report as a bug if using rename operations
```

### 5. Unknown Operation Type

```
unknown operation type: [type]
```
**Cause:** Internal error - operation has unrecognized type.
**Valid Types:** `create`, `modify`, `delete`, `rename`
**Resolution:** This is a bug. Report it.

---

## Staging Errors

These errors occur during the staging phase, before any files are applied.

```
failed to stage creation of <path>: [error]
```
**Cause:** Cannot stage a file creation operation.
**Common Reasons:**
- Path validation failed
- Snapshot capture failed

```
failed to stage modification of <path>: [error]
```
**Cause:** Cannot stage a file modification operation.
**Common Reasons:**
- Original file doesn't exist (expected for modify)
- Cannot capture snapshot of original

```
failed to stage deletion of <path>: [error]
```
**Cause:** Cannot stage a file deletion operation.
**Common Reasons:**
- File doesn't exist
- Cannot capture snapshot

---

## WAL and Snapshot Errors

### 1. Snapshot Capture Errors

```
failed to capture snapshot: [error]
```
**Cause:** Cannot create a snapshot of a file's current state.
**Common Reasons:**
- Cannot read original file (permission denied)
- Cannot write snapshot file (disk full, permissions)
- File too large (memory issue)

**Resolution:**
```bash
# Check original file permissions
ls -la path/to/original

# Check snapshot directory permissions
ls -la .plandex/snapshots/<txid>/

# Check disk space
df -h .

# Check file size
ls -lh path/to/original
```

```
snapshot not found for operation N
```
**Cause:** During rollback, cannot find snapshot to restore.
**Resolution:** This indicates snapshot file was deleted or corrupted. Manual recovery may be needed:
```bash
# Check if snapshot exists
ls .plandex/snapshots/<txid>/

# If missing, restore from git
git checkout path/to/file

# Or restore from backup
```

### 2. WAL Errors

```
failed to open WAL: [error]
```
**Cause:** Cannot open write-ahead log file for writing.
**Common Reasons:**
- Permission denied
- Disk full
- Path doesn't exist

**Resolution:**
```bash
# Check WAL directory
ls -la .plandex/wal/

# Fix permissions
chmod u+w .plandex/wal/

# Check disk space
df -h .
```

```
failed to marshal WAL entry: [error]
```
**Cause:** Cannot serialize operation to JSON for WAL.
**Resolution:** This is an internal error. Report as a bug.

```
failed to write WAL entry: [error]
```
**Cause:** Cannot write serialized entry to WAL file.
**Common Reasons:**
- Disk full
- I/O error

**Resolution:**
```bash
# Check disk space
df -h .

# Check for I/O errors
dmesg | tail  # Linux
tail /var/log/system.log  # macOS
```

```
failed to read WAL: [error]
```
**Cause:** Cannot read WAL during recovery.
**Resolution:** WAL may be corrupted. Safe to delete:
```bash
rm .plandex/wal/*.wal
```

```
no valid WAL entries found
```
**Cause:** WAL file exists but contains no valid entries.
**Resolution:** Safe to delete orphaned WAL:
```bash
rm .plandex/wal/*.wal
```

---

## Rollback Errors

### 1. Rollback State Errors

```
cannot rollback: transaction already committed
```
**Cause:** Attempted to rollback after successful commit.
**Resolution:** This is a logic error. Report as a bug.

### 2. Snapshot Restoration Errors

```
failed to read snapshot from disk: [error]
```
**Cause:** Cannot read snapshot file during rollback.
**Common Reasons:**
- Snapshot file deleted
- Snapshot file corrupted
- Permission denied

**Resolution:**
```bash
# Check if snapshots exist
ls -la .plandex/snapshots/<txid>/

# Check permissions
chmod u+r .plandex/snapshots/<txid>/*

# If missing, manual recovery needed
git checkout path/to/file
```

### 3. Compound Rollback Errors

```
rollback failed ([rollback error]) after [original error]
```
**Cause:** The rollback itself failed after the original operation failed.
**Impact:** **CRITICAL** - Files may be in inconsistent state.
**Resolution:**
1. Note both errors
2. Check file states manually
3. Restore from git or backup:
```bash
# Check current file state
git status
git diff

# If needed, restore from git
git checkout .

# Or restore from last commit
git reset --hard HEAD
```

---

## Error Recovery Guide

### Recovery by Error Type

#### Permission Errors
```bash
# Fix directory permissions
find .plandex -type d -exec chmod u+w {} \;

# Fix file permissions
find .plandex -type f -exec chmod u+rw {} \;

# Fix project permissions (careful!)
chmod -R u+w .
```

#### Disk Space Errors
```bash
# Check available space
df -h .

# Clean up old snapshots (if safe)
rm -rf .plandex/snapshots/old-*

# Clean up old WAL files
rm .plandex/wal/*.wal

# Clear temp files
rm -rf /tmp/plandex-*
```

#### Transaction Stuck/Corrupted
```bash
# Remove all transaction artifacts
rm -rf .plandex/wal/*.wal
rm -rf .plandex/snapshots/*

# Restore from git if needed
git status
git checkout .
```

#### File Conflicts
```bash
# Check what files are in conflict
git status

# Restore all from git
git checkout .

# Or restore specific file
git checkout path/to/file

# Then retry transaction
plandex apply --tx
```

### Manual Recovery Steps

1. **Identify the error**
   - Read the full error message
   - Note which file(s) caused the issue

2. **Check transaction state**
   ```bash
   # Check for orphaned WAL files
   ls .plandex/wal/

   # Check for orphaned snapshots
   ls .plandex/snapshots/
   ```

3. **Verify file states**
   ```bash
   # Check git status
   git status

   # Check file contents
   cat path/to/file
   ```

4. **Clean up if needed**
   ```bash
   # Remove transaction artifacts
   rm -rf .plandex/wal/*.wal
   rm -rf .plandex/snapshots/*
   ```

5. **Restore from backup**
   ```bash
   # From git
   git checkout path/to/file

   # Or restore entire working tree
   git checkout .
   ```

6. **Retry the operation**
   ```bash
   # After fixing the cause, retry
   plandex apply --tx
   ```

---

## Error Prevention

### Best Practices

1. **Check before applying**
   ```bash
   # Review changes
   plandex diff

   # Check permissions
   ls -la .

   # Check disk space
   df -h .
   ```

2. **Ensure clean git state**
   ```bash
   # Commit or stash changes first
   git status
   git stash  # or git commit
   ```

3. **Use transactions for safety**
   ```bash
   # Always use --tx for important patches
   plandex apply --tx

   # Or enable globally
   export PLANDEX_USE_TRANSACTIONS=1
   ```

4. **Monitor for warnings**
   - Watch for permission warnings
   - Note disk space warnings
   - Check for file lock messages

5. **Keep backups**
   ```bash
   # Git commit before large patches
   git add .
   git commit -m "Before plandex apply"

   # Or create a branch
   git checkout -b before-plandex-patch
   ```

---

## Debugging

### Enable Verbose Logging

```bash
# Set log level
export PLANDEX_LOG_LEVEL=debug

# Run with verbose output
plandex apply --tx 2>&1 | tee apply.log
```

### Check Transaction State

```bash
# List WAL files
ls -la .plandex/wal/

# View WAL contents (JSON)
cat .plandex/wal/*.wal | jq .

# List snapshots
ls -la .plandex/snapshots/

# Check snapshot contents
ls -la .plandex/snapshots/<txid>/
```

### Verify File Integrity

```bash
# Check file checksums before/after
sha256sum path/to/file > before.sha256
# ... apply patch ...
sha256sum path/to/file > after.sha256
diff before.sha256 after.sha256
```

---

## Error Statistics

Based on test runs, error frequency estimates:

| Error Type | Frequency | Impact | Auto-Recovery |
|-----------|-----------|--------|---------------|
| Permission denied | Common | High | Yes (rollback) |
| Disk full | Rare | High | Yes (rollback) |
| File not found | Rare | Medium | Yes (rollback) |
| Snapshot failure | Very Rare | Critical | Partial |
| WAL corruption | Very Rare | Medium | Yes (cleanup) |
| Rollback failure | Extremely Rare | Critical | No |

---

## Reporting Issues

When reporting transaction errors, include:

1. **Full error message** (from terminal)
2. **Transaction ID** (from `.plandex/wal/` filename)
3. **WAL contents** (if exists):
   ```bash
   cat .plandex/wal/*.wal
   ```
4. **System info**:
   ```bash
   uname -a
   df -h .
   ls -la .plandex/
   ```
5. **Git status**:
   ```bash
   git status
   git log --oneline -5
   ```
6. **Reproduction steps** if possible

---

## Summary

The transactional apply system provides:
- ✅ **Automatic rollback** on all errors
- ✅ **Clear error messages** with context
- ✅ **Safe recovery** via snapshots and WAL
- ✅ **Manual recovery** options when needed
- ⚠️ **Critical errors** are rare but require attention

**Key Takeaway:** Most errors result in automatic rollback, leaving your codebase in the original state. Only rollback failures require manual intervention.
