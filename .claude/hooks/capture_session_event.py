#!/usr/bin/env python3
"""
Hook to capture session start/end events.
Cross-platform support for Windows, macOS, and Linux.
"""
import json
import sys
import os
import subprocess
import fcntl
from datetime import datetime, timezone
from claude_code_capture_utils import get_log_file_path, add_ab_metadata


def write_log_entry_atomic(log_file, log_entry):
    """Write a log entry atomically with file locking to prevent race conditions."""
    try:
        with open(log_file, "a", encoding="utf-8") as f:
            # Acquire exclusive lock to prevent concurrent writes
            fcntl.flock(f.fileno(), fcntl.LOCK_EX)
            try:
                f.write(json.dumps(log_entry) + "\n")
                f.flush()
                os.fsync(f.fileno())
            finally:
                # Release the lock
                fcntl.flock(f.fileno(), fcntl.LOCK_UN)
    except (IOError, OSError) as e:
        print(f"[WARN] Failed to write log entry: {e}", file=sys.stderr)

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
    event_type = "unknown"
    try:
        if len(sys.argv) < 2:
            print("Usage: capture_session_event.py [start|end]", file=sys.stderr)
            sys.exit(1)

        event_type = sys.argv[1].lower()
        if event_type not in ["start", "end"]:
            print("Event type must be 'start' or 'end'", file=sys.stderr)
            sys.exit(1)

        input_data = json.load(sys.stdin)

        session_id = input_data.get("session_id")
        # Ensure session_id is valid to prevent file overwrites between sessions
        if not session_id or session_id == "unknown":
            import uuid
            session_id = f"fallback_{uuid.uuid4().hex[:8]}"
            print(f"[WARN] No valid session_id provided, using: {session_id}", file=sys.stderr)

        transcript_path = input_data.get("transcript_path", "")
        cwd = input_data.get("cwd", "")

        if event_type == "start":
            # Session start: capture git metadata
            git_metadata = get_git_metadata(cwd)

            log_entry = {
                "type": "session_start",
                "timestamp": datetime.now(timezone.utc).isoformat(),
                "session_id": session_id,
                "transcript_path": transcript_path,
                "cwd": cwd,
                "git_metadata": git_metadata
            }

            log_entry = add_ab_metadata(log_entry, cwd)

            if git_metadata:
                print(f"[OK] Captured git metadata: {git_metadata['base_commit'][:8]} on {git_metadata['branch']}")

            # Write session_start event with atomic locking
            log_file = get_log_file_path(session_id, cwd)
            write_log_entry_atomic(log_file, log_entry)

        elif event_type == "end":
            # Session end: log the event with duration info
            end_time = datetime.now(timezone.utc)

            log_entry = {
                "type": "session_end",
                "timestamp": end_time.isoformat(),
                "session_id": session_id,
                "transcript_path": transcript_path,
                "cwd": cwd,
                "reason": input_data.get("reason", "")
            }

            log_entry = add_ab_metadata(log_entry, cwd)

            # Write session_end event with atomic locking
            log_file = get_log_file_path(session_id, cwd)
            write_log_entry_atomic(log_file, log_entry)

            # Report session end
            reason = input_data.get("reason", "completed")
            print(f"[OK] Session ended: {session_id[:16]}... ({reason})")

    except Exception as e:
        import traceback
        print(f"[ERROR] Session {event_type} failed: {e}", file=sys.stderr)
        print(f"[DEBUG] Stack trace:\n{traceback.format_exc()}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
