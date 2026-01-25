#!/usr/bin/env python3
"""
Hook to capture session start/end events.
Cross-platform support for Windows, macOS, and Linux.

Concurrency Safety:
- Uses fcntl.flock() for atomic file writes on Unix systems
- Generates fallback session IDs to prevent overwrites
- See docs/CONCURRENCY_SAFETY.md for details
"""
import json
import sys
import os
import subprocess
import uuid
from datetime import datetime, timezone
from claude_code_capture_utils import get_log_file_path, add_ab_metadata

# Platform-specific file locking
try:
    import fcntl
    HAS_FCNTL = True
except ImportError:
    HAS_FCNTL = False  # Windows doesn't have fcntl


def write_log_entry_atomic(log_file, log_entry):
    """Write a log entry atomically with file locking.

    Uses fcntl.flock() on Unix systems to ensure atomic writes
    when multiple sessions write to the same file concurrently.
    """
    os.makedirs(os.path.dirname(log_file), exist_ok=True)

    with open(log_file, "a", encoding="utf-8") as f:
        if HAS_FCNTL:
            # Acquire exclusive lock (blocks until available)
            fcntl.flock(f.fileno(), fcntl.LOCK_EX)
            try:
                f.write(json.dumps(log_entry) + "\n")
                f.flush()
                os.fsync(f.fileno())  # Force write to disk
            finally:
                # Release lock
                fcntl.flock(f.fileno(), fcntl.LOCK_UN)
        else:
            # Windows fallback: no locking, but still atomic write
            f.write(json.dumps(log_entry) + "\n")
            f.flush()


def get_safe_session_id(session_id):
    """Generate a safe session ID, with fallback for missing/unknown IDs.

    Prevents file overwrites when multiple sessions have missing session IDs.
    """
    if not session_id or session_id == "unknown":
        return f"fallback_{uuid.uuid4().hex[:8]}"
    return session_id

def get_git_metadata(repo_dir):
    """Get current git commit and branch."""
    try:
        # Get current commit hash
        commit_result = subprocess.run(
            ['git', 'rev-parse', 'HEAD'],
            cwd=repo_dir,
            capture_output=True,
            text=True,
            timeout=10
        )

        # Get current branch
        branch_result = subprocess.run(
            ['git', 'branch', '--show-current'],
            cwd=repo_dir,
            capture_output=True,
            text=True,
            timeout=10
        )

        git_metadata = {
            "base_commit": commit_result.stdout.strip() if commit_result.returncode == 0 else None,
            "branch": branch_result.stdout.strip() if branch_result.returncode == 0 else None,
            "timestamp": datetime.now(timezone.utc).isoformat()
        }

        if git_metadata["base_commit"]:
            return git_metadata
        else:
            return None

    except Exception as e:
        print(f"Warning: Could not capture git metadata: {e}", file=sys.stderr)
        return None

def main():
    try:
        if len(sys.argv) < 2:
            print("Usage: capture_session_event.py [start|end]", file=sys.stderr)
            sys.exit(1)

        event_type = sys.argv[1].lower()
        if event_type not in ["start", "end"]:
            print("Event type must be 'start' or 'end'", file=sys.stderr)
            sys.exit(1)

        input_data = json.load(sys.stdin)

        # Use safe session ID to prevent file overwrites
        raw_session_id = input_data.get("session_id", "unknown")
        session_id = get_safe_session_id(raw_session_id)
        transcript_path = input_data.get("transcript_path", "")
        cwd = input_data.get("cwd", "")

        if event_type == "start":
            # Session start: capture git metadata
            git_metadata = get_git_metadata(cwd)

            log_entry = {
                "type": "session_start",
                "timestamp": datetime.now(timezone.utc).isoformat(),
                "session_id": session_id,
                "original_session_id": raw_session_id if raw_session_id != session_id else None,
                "transcript_path": transcript_path,
                "cwd": cwd,
                "git_metadata": git_metadata
            }

            # Remove None values for cleaner output
            log_entry = {k: v for k, v in log_entry.items() if v is not None}
            log_entry = add_ab_metadata(log_entry, cwd)

            if git_metadata:
                print(f"[OK] Captured git metadata: {git_metadata['base_commit'][:8]} on {git_metadata['branch']}")

            # Write session_start event with atomic file locking
            log_file = get_log_file_path(session_id, cwd)
            write_log_entry_atomic(log_file, log_entry)

        elif event_type == "end":
            # Session end: log the event
            log_entry = {
                "type": "session_end",
                "timestamp": datetime.now(timezone.utc).isoformat(),
                "session_id": session_id,
                "transcript_path": transcript_path,
                "cwd": cwd,
                "reason": input_data.get("reason", "")
            }

            log_entry = add_ab_metadata(log_entry, cwd)

            # Write session_end event with atomic file locking
            log_file = get_log_file_path(session_id, cwd)
            write_log_entry_atomic(log_file, log_entry)

    except Exception as e:
        print(f"[ERROR] Session {event_type}: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
