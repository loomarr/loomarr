"""Network-free checks for retained failure evidence and bounded command failures."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('diagnostics', Path(__file__).with_name('apple-launch-diagnostics.py'))
diagnostics = importlib.util.module_from_spec(spec)
spec.loader.exec_module(diagnostics)


class DiagnosticsTest(unittest.TestCase):
    def test_collects_current_matching_crashes_and_artifact_identity(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            app = root / 'Fixture.app'
            app.mkdir()
            binary = app / 'Fixture'
            binary.write_bytes(b'fixture executable')
            binary.chmod(0o755)
            (app / 'Info.plist').write_bytes(b'fixture plist')
            unrelated = root / 'home/Library/Logs/DiagnosticReports'
            unrelated.mkdir(parents=True)
            old = unrelated / 'Fixture-old.ips'
            old.write_text('old crash')
            os.utime(old, (1, 1))
            (unrelated / 'AnotherApp.ips').write_text('unrelated private crash')
            (unrelated / 'Fixture-now.ips').write_text('Termination Reason: DYLD, Library not loaded')
            (unrelated / 'Fixture-link.ips').symlink_to(unrelated / 'AnotherApp.ips')
            with patch.object(diagnostics.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0)) as run:
                report = diagnostics.collect(root / 'artifacts', app, 'Fixture', 'media.fixture', 'SIM', 10, root / 'home')
            self.assertEqual([entry['name'] for entry in report['crash_reports']], ['0-Fixture-now.ips'])
            retained = root / 'artifacts/launch-diagnostics'
            self.assertIn('Library not loaded', (retained / '0-Fixture-now.ips').read_text())
            self.assertEqual({entry['path'] for entry in report['files']}, {'Fixture', 'Info.plist'})
            self.assertTrue(all(len(entry['sha256']) == 64 for entry in report['files']))
            self.assertEqual(json.loads((retained / 'identity.json').read_text()), report)
            self.assertTrue(all(call.kwargs['timeout'] == 20 for call in run.call_args_list))

    def test_failed_commands_still_leave_diagnostic_record(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch.object(diagnostics.subprocess, 'run', side_effect=subprocess.TimeoutExpired('xcrun', 20)):
                report = diagnostics.collect(root, root / 'missing.app', 'Fixture', 'media.fixture', 'SIM', 0, root)
            self.assertIn('error', report['commands'][0])
            self.assertEqual(report['files'], [])
            self.assertTrue((root / 'launch-diagnostics/identity.json').is_file())


if __name__ == '__main__':
    unittest.main()
