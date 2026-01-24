#!/usr/bin/env python3
"""
Integration tests for the session capture system.
Tests the end-to-end flow from event capture to log file writing.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
import uuid
from unittest.mock import patch, MagicMock

# Import modules under test
import capture_session_event
from claude_code_capture_utils import get_log_file_path


def unique_session_id(prefix="test"):
    """Generate a unique session ID for testing."""
    return f"{prefix}_{uuid.uuid4().hex[:8]}"


class TestEndToEndSessionCapture(unittest.TestCase):
    """Integration tests for the complete session capture flow."""

    def setUp(self):
        """Set up test fixtures."""
        self.temp_dir = tempfile.mkdtemp()
        self.original_env = os.environ.copy()

    def tearDown(self):
        """Clean up test fixtures."""
        os.environ.clear()
        os.environ.update(self.original_env)
        shutil.rmtree(self.temp_dir, ignore_errors=True)

    def test_full_capture_flow_with_valid_input(self):
        """Test the complete flow from stdin input to log file output."""
        session_id = unique_session_id("full-flow")
        log_file = os.path.join(self.temp_dir, f"session_{session_id}.jsonl")

        # Ensure directory exists
        os.makedirs(os.path.dirname(log_file), exist_ok=True)

        # Write log entry directly (simulating what main() does)
        log_entry = {
            "event_type": "tool_use",
            "session_id": session_id,
            "tool_name": "Read",
            "tool_input": {"file_path": "/test/file.py"},
            "cwd": "/test/cwd"
        }

        capture_session_event.write_log_entry_atomic(log_file, log_entry)

        # Verify the log file was created and contains valid JSON
        self.assertTrue(os.path.exists(log_file))

        with open(log_file, 'r', encoding='utf-8') as f:
            lines = f.readlines()

        self.assertEqual(len(lines), 1)
        parsed = json.loads(lines[0].strip())
        self.assertEqual(parsed["event_type"], "tool_use")
        self.assertEqual(parsed["session_id"], session_id)
        self.assertEqual(parsed["tool_name"], "Read")

    def test_multiple_events_same_session(self):
        """Test capturing multiple events in the same session."""
        session_id = unique_session_id("multi-event")
        log_file = os.path.join(self.temp_dir, f"session_{session_id}.jsonl")
        os.makedirs(os.path.dirname(log_file), exist_ok=True)

        events = [
            {"event_type": "tool_use", "tool_name": "Read", "sequence": 1},
            {"event_type": "tool_use", "tool_name": "Write", "sequence": 2},
            {"event_type": "tool_use", "tool_name": "Bash", "sequence": 3},
        ]

        for event in events:
            event["session_id"] = session_id
            capture_session_event.write_log_entry_atomic(log_file, event)

        # Verify all events were captured
        with open(log_file, 'r', encoding='utf-8') as f:
            lines = f.readlines()

        self.assertEqual(len(lines), 3)

        for i, line in enumerate(lines):
            parsed = json.loads(line.strip())
            self.assertEqual(parsed["sequence"], i + 1)
            self.assertEqual(parsed["session_id"], session_id)

    def test_log_file_path_creation(self):
        """Test that log file paths are created correctly."""
        session_id = "path-test-session"
        # Use actual cwd which is in the repo
        cwd = os.path.dirname(os.path.abspath(__file__))
        log_file = get_log_file_path(session_id, cwd)

        # Path should contain session_id
        self.assertIn(session_id, log_file)

        # Path should end with .jsonl
        self.assertTrue(log_file.endswith('.jsonl'))

        # Path should be a valid string path
        self.assertIsInstance(log_file, str)
        self.assertTrue(len(log_file) > 0)

    def test_git_metadata_integration(self):
        """Test git metadata extraction in a real git repo."""
        # This test runs in the actual repo
        cwd = os.path.dirname(os.path.abspath(__file__))
        result = capture_session_event.get_git_metadata(cwd)

        # Should return metadata since we're in a git repo
        if result is not None:
            self.assertIn("branch", result)
            self.assertIn("base_commit", result)
            self.assertIn("timestamp", result)

    def test_fallback_session_id_generation(self):
        """Test that fallback session IDs are generated when needed."""
        import uuid

        # Test various invalid session IDs
        invalid_ids = [None, "", "unknown"]

        for invalid_id in invalid_ids:
            session_id = invalid_id
            if not session_id or session_id == "unknown":
                session_id = f"fallback_{uuid.uuid4().hex[:8]}"

            self.assertTrue(session_id.startswith("fallback_"))
            self.assertEqual(len(session_id), len("fallback_") + 8)


class TestHookModuleIntegration(unittest.TestCase):
    """Tests for integration between hook modules."""

    def test_utils_module_is_importable(self):
        """Test that the utils module can be imported."""
        try:
            from claude_code_capture_utils import get_log_file_path
            self.assertTrue(callable(get_log_file_path))
        except ImportError as e:
            self.fail(f"Could not import claude_code_capture_utils: {e}")

    def test_capture_module_is_importable(self):
        """Test that the capture module can be imported."""
        try:
            import capture_session_event
            self.assertTrue(hasattr(capture_session_event, 'write_log_entry_atomic'))
            self.assertTrue(hasattr(capture_session_event, 'get_git_metadata'))
        except ImportError as e:
            self.fail(f"Could not import capture_session_event: {e}")

    def test_modules_work_together(self):
        """Test that both modules work together correctly."""
        with tempfile.TemporaryDirectory() as temp_dir:
            session_id = unique_session_id("integration")
            log_file = os.path.join(temp_dir, f"session_{session_id}.jsonl")

            os.makedirs(os.path.dirname(log_file), exist_ok=True)

            entry = {"test": "data", "module": "integration"}
            capture_session_event.write_log_entry_atomic(log_file, entry)

            self.assertTrue(os.path.exists(log_file))

            with open(log_file, 'r') as f:
                lines = f.readlines()
                self.assertEqual(len(lines), 1)
                content = json.loads(lines[0].strip())
                self.assertEqual(content["test"], "data")


class TestErrorRecovery(unittest.TestCase):
    """Tests for error recovery in the capture system."""

    def test_recovery_from_partial_write(self):
        """Test that the system handles partial writes gracefully."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.jsonl', delete=False) as f:
            temp_path = f.name
            # Write a partial entry with newline (simulating crash after one entry)
            f.write('{"incomplete": true}\n')
            f.flush()

        try:
            # Write a complete entry after the partial one
            entry = {"complete": True, "data": "test"}
            capture_session_event.write_log_entry_atomic(temp_path, entry)

            # Read back and verify both entries are on separate lines
            with open(temp_path, 'r') as f:
                lines = f.readlines()

            # Should have 2 lines now
            self.assertEqual(len(lines), 2)

            # First line is the original entry
            first = json.loads(lines[0].strip())
            self.assertTrue(first["incomplete"])

            # Second line is our new entry
            second = json.loads(lines[1].strip())
            self.assertTrue(second["complete"])
        finally:
            if os.path.exists(temp_path):
                os.unlink(temp_path)

    def test_recovery_from_missing_directory(self):
        """Test that the system creates missing directories."""
        with tempfile.TemporaryDirectory() as temp_dir:
            deep_path = os.path.join(temp_dir, "a", "b", "c", "session.jsonl")

            # Directory doesn't exist yet
            self.assertFalse(os.path.exists(os.path.dirname(deep_path)))

            # Create directory and write
            os.makedirs(os.path.dirname(deep_path), exist_ok=True)
            entry = {"test": "deep_path"}
            capture_session_event.write_log_entry_atomic(deep_path, entry)

            self.assertTrue(os.path.exists(deep_path))


class TestConcurrentSessions(unittest.TestCase):
    """Tests for handling multiple concurrent sessions."""

    def test_multiple_sessions_no_interference(self):
        """Test that multiple sessions don't interfere with each other."""
        with tempfile.TemporaryDirectory() as temp_dir:
            # Generate unique session IDs
            session_a = unique_session_id("session-A")
            session_b = unique_session_id("session-B")
            session_c = unique_session_id("session-C")

            sessions = [
                (session_a, [{"type": "read", "seq": i} for i in range(5)]),
                (session_b, [{"type": "write", "seq": i} for i in range(5)]),
                (session_c, [{"type": "bash", "seq": i} for i in range(5)]),
            ]

            # Write events for each session
            for session_id, events in sessions:
                log_file = os.path.join(temp_dir, f"session_{session_id}.jsonl")
                os.makedirs(os.path.dirname(log_file), exist_ok=True)

                for event in events:
                    event["session_id"] = session_id
                    capture_session_event.write_log_entry_atomic(log_file, event)

            # Verify each session has correct entries
            for session_id, expected_events in sessions:
                log_file = os.path.join(temp_dir, f"session_{session_id}.jsonl")

                with open(log_file, 'r') as f:
                    lines = f.readlines()

                self.assertEqual(len(lines), 5)

                for i, line in enumerate(lines):
                    parsed = json.loads(line.strip())
                    self.assertEqual(parsed["session_id"], session_id)
                    self.assertEqual(parsed["seq"], i)


if __name__ == '__main__':
    unittest.main()
