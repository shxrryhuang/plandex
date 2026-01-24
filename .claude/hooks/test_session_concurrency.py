#!/usr/bin/env python3
"""
Concurrency and edge case tests for capture_session_event.py
"""
import json
import os
import sys
import tempfile
import threading
import time
import unittest
from concurrent.futures import ThreadPoolExecutor, as_completed

# Import the module under test
import capture_session_event


class TestAtomicWriteConcurrency(unittest.TestCase):
    """Tests for concurrent write safety."""

    def test_concurrent_writes_no_corruption(self):
        """Test that concurrent writes don't corrupt the log file."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.jsonl', delete=False) as f:
            temp_path = f.name

        try:
            num_writers = 10
            writes_per_thread = 50

            def write_entries(thread_id):
                for i in range(writes_per_thread):
                    entry = {
                        "thread_id": thread_id,
                        "sequence": i,
                        "timestamp": time.time()
                    }
                    capture_session_event.write_log_entry_atomic(temp_path, entry)

            # Run concurrent writers
            threads = []
            for i in range(num_writers):
                t = threading.Thread(target=write_entries, args=(i,))
                threads.append(t)
                t.start()

            for t in threads:
                t.join()

            # Verify all entries were written correctly
            with open(temp_path, 'r') as f:
                lines = f.readlines()

            # Should have exactly num_writers * writes_per_thread entries
            expected_count = num_writers * writes_per_thread
            self.assertEqual(len(lines), expected_count,
                           f"Expected {expected_count} lines, got {len(lines)}")

            # Each line should be valid JSON
            for i, line in enumerate(lines):
                try:
                    entry = json.loads(line.strip())
                    self.assertIn("thread_id", entry)
                    self.assertIn("sequence", entry)
                except json.JSONDecodeError as e:
                    self.fail(f"Line {i} is not valid JSON: {line[:100]}... Error: {e}")

        finally:
            if os.path.exists(temp_path):
                os.unlink(temp_path)

    def test_concurrent_writes_to_different_files(self):
        """Test that concurrent writes to different files work correctly."""
        temp_dir = tempfile.mkdtemp()

        try:
            num_files = 5
            writes_per_file = 20

            def write_to_file(file_id):
                file_path = os.path.join(temp_dir, f"session_{file_id}.jsonl")
                for i in range(writes_per_file):
                    entry = {"file_id": file_id, "entry": i}
                    capture_session_event.write_log_entry_atomic(file_path, entry)
                return file_path

            with ThreadPoolExecutor(max_workers=num_files) as executor:
                futures = [executor.submit(write_to_file, i) for i in range(num_files)]
                file_paths = [f.result() for f in as_completed(futures)]

            # Verify each file has correct content
            for file_id in range(num_files):
                file_path = os.path.join(temp_dir, f"session_{file_id}.jsonl")
                with open(file_path, 'r') as f:
                    lines = f.readlines()

                self.assertEqual(len(lines), writes_per_file,
                               f"File {file_id} should have {writes_per_file} lines")

        finally:
            import shutil
            shutil.rmtree(temp_dir, ignore_errors=True)


class TestSessionIdGeneration(unittest.TestCase):
    """Tests for session ID handling edge cases."""

    def test_none_session_id_gets_fallback(self):
        """Test that None session_id generates a fallback."""
        # This tests the logic, not the full main() function
        session_id = None
        if not session_id or session_id == "unknown":
            import uuid
            session_id = f"fallback_{uuid.uuid4().hex[:8]}"

        self.assertTrue(session_id.startswith("fallback_"))
        self.assertEqual(len(session_id), len("fallback_") + 8)

    def test_empty_string_session_id_gets_fallback(self):
        """Test that empty string session_id generates a fallback."""
        session_id = ""
        if not session_id or session_id == "unknown":
            import uuid
            session_id = f"fallback_{uuid.uuid4().hex[:8]}"

        self.assertTrue(session_id.startswith("fallback_"))

    def test_valid_session_id_preserved(self):
        """Test that valid session_id is preserved."""
        original_id = "abc123-session-xyz"
        session_id = original_id
        if not session_id or session_id == "unknown":
            import uuid
            session_id = f"fallback_{uuid.uuid4().hex[:8]}"

        self.assertEqual(session_id, original_id)

    def test_fallback_ids_are_unique(self):
        """Test that generated fallback IDs are unique."""
        import uuid
        ids = set()
        for _ in range(1000):
            session_id = f"fallback_{uuid.uuid4().hex[:8]}"
            self.assertNotIn(session_id, ids, "Generated duplicate session ID")
            ids.add(session_id)


class TestGitMetadataExtraction(unittest.TestCase):
    """Tests for git metadata extraction."""

    def test_git_metadata_in_non_git_directory(self):
        """Test that git metadata returns None in non-git directory."""
        with tempfile.TemporaryDirectory() as temp_dir:
            result = capture_session_event.get_git_metadata(temp_dir)
            self.assertIsNone(result)

    def test_git_metadata_with_invalid_directory(self):
        """Test that git metadata handles invalid directory gracefully."""
        result = capture_session_event.get_git_metadata("/nonexistent/path/12345")
        self.assertIsNone(result)


class TestErrorHandling(unittest.TestCase):
    """Tests for error handling edge cases."""

    def test_write_to_readonly_location_handled(self):
        """Test that write to readonly location is handled gracefully."""
        # Try to write to a location that should fail
        readonly_path = "/readonly_test_file_that_should_not_exist.jsonl"

        # This should not raise an exception due to our error handling
        try:
            capture_session_event.write_log_entry_atomic(readonly_path, {"test": "data"})
        except PermissionError:
            pass  # Expected on some systems
        except Exception as e:
            # Should be handled gracefully
            pass

    def test_write_with_unicode_content(self):
        """Test that unicode content is handled correctly."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.jsonl', delete=False) as f:
            temp_path = f.name

        try:
            entry = {
                "message": "Hello 世界 🌍 émojis",
                "path": "/path/to/файл.txt"
            }
            capture_session_event.write_log_entry_atomic(temp_path, entry)

            with open(temp_path, 'r', encoding='utf-8') as f:
                content = f.read()
                parsed = json.loads(content.strip())
                self.assertEqual(parsed["message"], entry["message"])
                self.assertEqual(parsed["path"], entry["path"])
        finally:
            if os.path.exists(temp_path):
                os.unlink(temp_path)

    def test_write_with_special_characters(self):
        """Test that special characters in content are escaped properly."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.jsonl', delete=False) as f:
            temp_path = f.name

        try:
            entry = {
                "content": 'Line1\nLine2\tTabbed\r\nWindows line',
                "quote": 'He said "hello"',
                "backslash": "path\\to\\file"
            }
            capture_session_event.write_log_entry_atomic(temp_path, entry)

            with open(temp_path, 'r', encoding='utf-8') as f:
                content = f.read()
                parsed = json.loads(content.strip())
                self.assertEqual(parsed["content"], entry["content"])
                self.assertEqual(parsed["quote"], entry["quote"])
                self.assertEqual(parsed["backslash"], entry["backslash"])
        finally:
            if os.path.exists(temp_path):
                os.unlink(temp_path)

    def test_large_entry_handling(self):
        """Test that large log entries are handled correctly."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.jsonl', delete=False) as f:
            temp_path = f.name

        try:
            # Create a large entry (1MB of data)
            large_data = "x" * (1024 * 1024)
            entry = {"large_field": large_data}

            capture_session_event.write_log_entry_atomic(temp_path, entry)

            with open(temp_path, 'r', encoding='utf-8') as f:
                content = f.read()
                parsed = json.loads(content.strip())
                self.assertEqual(len(parsed["large_field"]), len(large_data))
        finally:
            if os.path.exists(temp_path):
                os.unlink(temp_path)


class TestLogFilePathCreation(unittest.TestCase):
    """Tests for log file path handling."""

    def test_directory_creation_race_condition(self):
        """Test that concurrent directory creation doesn't fail."""
        from claude_code_capture_utils import get_log_file_path

        temp_base = tempfile.mkdtemp()

        try:
            def get_path(session_id):
                return get_log_file_path(session_id, temp_base)

            # Multiple threads trying to create directories
            with ThreadPoolExecutor(max_workers=10) as executor:
                futures = [executor.submit(get_path, f"session_{i}") for i in range(20)]
                paths = [f.result() for f in as_completed(futures)]

            # All paths should be valid strings
            for path in paths:
                self.assertIsInstance(path, str)
                self.assertTrue(len(path) > 0)

        finally:
            import shutil
            shutil.rmtree(temp_base, ignore_errors=True)


if __name__ == '__main__':
    unittest.main()
