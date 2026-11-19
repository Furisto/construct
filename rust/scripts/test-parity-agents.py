#!/usr/bin/env python3
"""Guard against overbroad parity normalization and allowlisting."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('parity', Path(__file__).with_name('parity-agents.py'))
parity = importlib.util.module_from_spec(spec)
spec.loader.exec_module(parity)


class Comparisons(unittest.TestCase):
    def test_empty(self):
        self.assertEqual(parity.normalize('', Path('/run')), '')
        self.assertEqual(parity.normalize('exit=0\n[]\n', Path('/run')), 'exit=0\n[]\n')

    def test_normalization_preserves_json_and_status(self):
        value = 'exit=0\n{"id":"11111111-1111-4111-8111-111111111111","time":"2026-11-19T15:00:00Z","path":"/run/file"}'
        normalized = parity.normalize(value, Path('/run'))
        self.assertIn('"<TIME>"', normalized)
        self.assertIn('"<ID>"', normalized)
        self.assertIn('"<ROOT>/file"', normalized)
        self.assertTrue(normalized.startswith('exit=0\n'))

    def test_error_allowlist_retains_actual_error(self):
        left, _ = parity.allowed_comparison('agent-missing', 'exit=1\nError: missing\nUsage:\nhelp')
        same, _ = parity.allowed_comparison('agent-missing', 'exit=1\nError: missing\n')
        other, _ = parity.allowed_comparison('agent-missing', 'exit=1\nError: broken\n')
        self.assertEqual(left, same)
        self.assertNotEqual(left, other)

    def test_list_allowlist_preserves_payload_and_multiplicity(self):
        left, _ = parity.allowed_comparison('agent-list-json', 'exit=0\n[{"name":"a"},{"name":"b"}]')
        same, _ = parity.allowed_comparison('agent-list-json', 'exit=0\n[{"name":"b"},{"name":"a"}]')
        different, _ = parity.allowed_comparison('agent-list-json', 'exit=0\n[{"name":"a"},{"name":"a"}]')
        self.assertEqual(left, same)
        self.assertNotEqual(left, different)

    def test_unlisted_output_is_untouched(self):
        value = 'exit=1\nError: broken\nUsage:\nhelp'
        self.assertEqual(parity.allowed_comparison('wire-agent-get', value), (value, ''))


if __name__ == '__main__':
    unittest.main()
