# Hooks System Design Document

**Version:** 1.0
**Last Updated:** January 2026

---

## Table of Contents

1. [Overview](#1-overview)
2. [Architecture](#2-architecture)
3. [Session Capture Flow](#3-session-capture-flow)
4. [File Locking Strategy](#4-file-locking-strategy)
5. [A/B Testing Integration](#5-ab-testing-integration)
6. [Error Handling](#6-error-handling)
7. [Testing Strategy](#7-testing-strategy)

---

## 1. Overview

### 1.1 Purpose

The hooks system provides event capture and logging capabilities for Claude Code sessions. It enables:

- Session start/end tracking
- Git metadata capture
- A/B testing support for model comparison experiments
- Atomic log file writes to prevent corruption

### 1.2 Components

| Component | File | Purpose |
|-----------|------|---------|
| Session Capture | `capture_session_event.py` | Main hook entry point |
| Utilities | `claude_code_capture_utils.py` | Shared helper functions |
| Unit Tests | `test_capture_session_event.py` | Core functionality tests |
| Integration Tests | `test_integration.py` | End-to-end flow tests |
| Concurrency Tests | `test_session_concurrency.py` | Thread safety tests |

---

## 2. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Claude Code CLI                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│    Session Start ──────┐          ┌────── Session End           │
│                        │          │                              │
│                        ▼          ▼                              │
│              ┌─────────────────────────────┐                    │
│              │   capture_session_event.py  │                    │
│              │   (Hook Entry Point)        │                    │
│              └──────────────┬──────────────┘                    │
│                             │                                    │
│              ┌──────────────┴──────────────┐                    │
│              │                             │                     │
│              ▼                             ▼                     │
│   ┌──────────────────┐         ┌──────────────────────┐        │
│   │ Git Metadata     │         │ A/B Test Metadata    │        │
│   │ Extraction       │         │ (Utils Module)       │        │
│   └────────┬─────────┘         └──────────┬───────────┘        │
│            │                              │                     │
│            └──────────────┬───────────────┘                    │
│                           │                                     │
│                           ▼                                     │
│              ┌─────────────────────────────┐                    │
│              │   write_log_entry_atomic()  │                    │
│              │   (File Locking)            │                    │
│              └──────────────┬──────────────┘                    │
│                             │                                    │
│                             ▼                                    │
│              ┌─────────────────────────────┐                    │
│              │   session_{id}.jsonl        │                    │
│              │   (Log File)                │                    │
│              └─────────────────────────────┘                    │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. Session Capture Flow

### 3.1 Session Start

```python
# Input (via stdin)
{
    "session_id": "abc123",
    "transcript_path": "/path/to/transcript",
    "cwd": "/working/directory"
}

# Processing Steps
1. Validate session_id (generate fallback if missing)
2. Extract git metadata (branch, commit hash)
3. Add A/B testing metadata if in experiment
4. Write log entry with atomic file locking

# Output (to log file)
{
    "type": "session_start",
    "timestamp": "2026-01-24T12:00:00+00:00",
    "session_id": "abc123",
    "git_metadata": {
        "branch": "main",
        "base_commit": "f32cd41f...",
        "timestamp": "2026-01-24T12:00:00+00:00"
    },
    "task_id": "TASK_3447",
    "model_lane": "model_b"
}
```

### 3.2 Session End

```python
# Input
{
    "session_id": "abc123",
    "reason": "user_exit"
}

# Output
{
    "type": "session_end",
    "timestamp": "2026-01-24T12:30:00+00:00",
    "session_id": "abc123",
    "reason": "user_exit"
}
```

### 3.3 Session ID Fallback

To prevent file overwrites between sessions with missing IDs:

```python
if not session_id or session_id == "unknown":
    session_id = f"fallback_{uuid.uuid4().hex[:8]}"
```

---

## 4. File Locking Strategy

### 4.1 Problem

Multiple concurrent sessions or processes may attempt to write to log files simultaneously, causing:
- Data corruption (interleaved writes)
- Lost entries
- Malformed JSON

### 4.2 Solution: Atomic Writes with File Locking

```python
def write_log_entry_atomic(log_file, log_entry):
    """Write a log entry atomically with file locking."""
    with open(log_file, "a", encoding="utf-8") as f:
        # Acquire exclusive lock
        fcntl.flock(f.fileno(), fcntl.LOCK_EX)
        try:
            f.write(json.dumps(log_entry) + "\n")
            f.flush()
            os.fsync(f.fileno())  # Force write to disk
        finally:
            # Release lock
            fcntl.flock(f.fileno(), fcntl.LOCK_UN)
```

### 4.3 Lock Guarantees

| Property | Guarantee |
|----------|-----------|
| Atomicity | Each entry is written completely or not at all |
| Ordering | Entries appear in acquisition order |
| Durability | `fsync()` ensures write to disk before release |
| Safety | `LOCK_EX` prevents concurrent writes |

---

## 5. A/B Testing Integration

### 5.1 Directory Structure

```
experiment_root/
├── manifest.json          # Task ID and model assignments
├── model_a/               # First model's codebase
│   └── .claude/hooks/
├── model_b/               # Second model's codebase
│   └── .claude/hooks/
└── logs/
    ├── model_a/           # Model A session logs
    │   └── session_*.jsonl
    └── model_b/           # Model B session logs
        └── session_*.jsonl
```

### 5.2 Lane Detection

```python
def detect_model_lane(cwd):
    """Detect if we're in model_a or model_b directory."""
    path_parts = Path(cwd).parts
    if 'model_a' in path_parts:
        return 'model_a'
    elif 'model_b' in path_parts:
        return 'model_b'
    return None
```

### 5.3 Manifest Format

```json
{
    "task_id": "TASK_3447",
    "assignments": {
        "model_a": "claude-3-opus",
        "model_b": "gpt-4-turbo"
    }
}
```

---

## 6. Error Handling

### 6.1 Error Categories

| Error Type | Handling |
|------------|----------|
| Missing session_id | Generate fallback UUID |
| Invalid event type | Exit with error message |
| Git command failure | Return None, continue |
| File write failure | Log warning to stderr |
| JSON parse error | Log error with stack trace |

### 6.2 Graceful Degradation

The system prioritizes session capture over perfect metadata:

```python
try:
    git_metadata = get_git_metadata(cwd)
except Exception:
    git_metadata = None  # Continue without git info

# Session still logged even if git metadata unavailable
```

---

## 7. Testing Strategy

### 7.1 Test Pyramid

```
                    ┌───────────────┐
                    │  Integration  │  (11 tests)
                    │    Tests      │  End-to-end flows
                    └───────┬───────┘
                            │
               ┌────────────┴────────────┐
               │     Concurrency Tests   │  (13 tests)
               │   Thread safety, races  │
               └────────────┬────────────┘
                            │
        ┌───────────────────┴───────────────────┐
        │           Unit Tests                   │  (7 tests)
        │   Individual function behavior         │
        └────────────────────────────────────────┘
```

### 7.2 Key Test Scenarios

| Test Category | Scenarios |
|---------------|-----------|
| Atomic Writes | Concurrent writes, large entries, unicode |
| Session IDs | Fallback generation, uniqueness |
| Git Metadata | Non-git directories, invalid paths |
| Error Recovery | Partial writes, missing directories |
| Concurrency | Multiple sessions, race conditions |

### 7.3 Running Tests

```bash
# All tests
python3 -m unittest discover -v

# Specific test file
python3 -m unittest test_integration -v

# With coverage
python3 -m coverage run -m unittest discover
python3 -m coverage report
```

---

## Appendix A: Log File Format

Each line in a `.jsonl` file is a complete JSON object:

```jsonl
{"type":"session_start","timestamp":"2026-01-24T12:00:00Z","session_id":"abc123",...}
{"type":"tool_use","timestamp":"2026-01-24T12:01:00Z","tool_name":"Read",...}
{"type":"session_end","timestamp":"2026-01-24T12:30:00Z","session_id":"abc123",...}
```

## Appendix B: Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `CLAUDE_PROJECT_DIR` | Log file base directory | `os.getcwd()` |

---

*Document generated: January 2026*
