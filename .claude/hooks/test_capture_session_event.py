#!/usr/bin/env python3
"""
Tests for capture_session_event.py
"""
import json
import os
import sys
import tempfile
import unittest
from io import StringIO
from unittest.mock import patch, MagicMock

# Import the module under test
import capture_session_event
from claude_code_capture_utils import get_log_file_path, detect_model_lane


class TestCaptureSessionEvent(unittest.TestCase):
    """Tests for the session capture functionality."""

    def test_event_type_initialized_before_exception_handler(self):
        """Test that event_type is initialized even if exception occurs early."""
        # This ensures the exception handler won't fail with NameError
        with patch('sys.argv', ['capture_session_event.py']):
            with patch('sys.stdin', StringIO('')):
                with self.assertRaises(SystemExit):
                    capture_session_event.main()

    def test_unknown_session_id_gets_fallback(self):
        """Test that unknown session_id gets a fallback UUID to prevent file collisions."""
        input_data = {
            "session_id": "unknown",
            "transcript_path": "/tmp/test",
            "cwd": "/tmp"
        }

        with patch('sys.argv', ['capture_session_event.py', 'start']):
            with patch('sys.stdin', StringIO(json.dumps(input_data))):
                with patch.object(capture_session_event, 'get_log_file_path') as mock_get_path:
                    with patch.object(capture_session_event, 'write_log_entry_atomic') as mock_write:
                        with patch.object(capture_session_event, 'get_git_metadata', return_value=None):
                            with patch.object(capture_session_event, 'add_ab_metadata', side_effect=lambda x, y: x):
                                mock_get_path.return_value = '/tmp/test.jsonl'
                                try:
                                    capture_session_event.main()
                                except SystemExit:
                                    pass  # Expected

                                # Verify that the session_id passed to get_log_file_path starts with 'fallback_'
                                if mock_get_path.called:
                                    call_args = mock_get_path.call_args[0]
                                    session_id_used = call_args[0]
                                    self.assertTrue(session_id_used.startswith('fallback_'),
                                                    f"Expected fallback session_id, got: {session_id_used}")

    def test_write_log_entry_atomic_creates_file(self):
        """Test that write_log_entry_atomic properly writes log entries."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.jsonl', delete=False) as f:
            temp_path = f.name

        try:
            log_entry = {"type": "test", "data": "value"}
            capture_session_event.write_log_entry_atomic(temp_path, log_entry)

            with open(temp_path, 'r') as f:
                content = f.read()
                self.assertIn('"type": "test"', content)
                self.assertIn('"data": "value"', content)
        finally:
            if os.path.exists(temp_path):
                os.unlink(temp_path)

    def test_invalid_event_type_exits_with_error(self):
        """Test that invalid event type causes exit."""
        with patch('sys.argv', ['capture_session_event.py', 'invalid']):
            with self.assertRaises(SystemExit) as cm:
                capture_session_event.main()
            self.assertEqual(cm.exception.code, 1)


class TestCaptureUtils(unittest.TestCase):
    """Tests for claude_code_capture_utils.py"""

    def test_detect_model_lane_model_a(self):
        """Test detection of model_a lane."""
        result = detect_model_lane('/home/user/project/model_a/src')
        self.assertEqual(result, 'model_a')

    def test_detect_model_lane_model_b(self):
        """Test detection of model_b lane."""
        result = detect_model_lane('/home/user/project/model_b/src')
        self.assertEqual(result, 'model_b')

    def test_detect_model_lane_neither(self):
        """Test detection when neither model lane present."""
        result = detect_model_lane('/home/user/project/src')
        self.assertIsNone(result)


if __name__ == '__main__':
    unittest.main()
