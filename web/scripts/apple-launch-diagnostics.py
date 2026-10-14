#!/usr/bin/env python3
"""Bounded failure evidence for a simulator app; never decides verifier success."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys


def capture(command, path):
    try:
        with path.open('w') as output:
            result = subprocess.run(command, stdout=output, stderr=subprocess.STDOUT, timeout=20, check=False)
        return {'command': command, 'exit_code': result.returncode}
    except (OSError, subprocess.TimeoutExpired) as error:
        return {'command': command, 'error': str(error)}


def collect(artifacts, app, scheme, bundle, simulator, started, home):
    destination = artifacts / 'launch-diagnostics'
    destination.mkdir(parents=True, exist_ok=True)
    report = {'bundle_id': bundle, 'simulator': simulator, 'started_at': started,
              'files': [], 'crash_reports': [], 'commands': []}
    predicate = f"process == '{scheme}' OR eventMessage CONTAINS[c] '{bundle}' OR eventMessage CONTAINS[c] '{scheme}'"
    report['commands'].append(capture(
        ['xcrun', 'simctl', 'spawn', simulator, 'log', 'show', '--last', '5m',
         '--style', 'compact', '--predicate', predicate], destination / 'simulator.log'))
    if app.is_dir():
        # App-local identity only: no simulator data containers, credentials, or host env.
        for path in sorted(app.rglob('*')):
            if not path.is_file() or path.is_symlink():
                continue
            relative = path.relative_to(app)
            if path.name == 'Info.plist' or os.access(path, os.X_OK):
                digest = file_hash(path)
                report['files'].append({'path': str(relative), 'sha256': digest, 'size': path.stat().st_size})
        binary = app / scheme
        if binary.is_file():
            for name, command in [('uuid', ['xcrun', 'dwarfdump', '--uuid']),
                                  ('dependencies', ['xcrun', 'otool', '-L']),
                                  ('load-commands', ['xcrun', 'otool', '-l']),
                                  ('signature', ['codesign', '-d', '--verbose=4'])]:
                report['commands'].append(capture(command + [str(binary)], destination / f'{name}.log'))
    roots = [home / 'Library/Logs/DiagnosticReports',
             home / 'Library/Developer/CoreSimulator/Devices' / simulator / 'data/Library/Logs/CrashReporter']
    matches = []
    for root in roots:
        if root.is_dir():
            matches.extend(path for path in root.rglob(f'{scheme}*')
                           if path.is_file() and not path.is_symlink() and path.suffix in ('.ips', '.crash')
                           and path.stat().st_mtime >= started)
    for number, path in enumerate(sorted(matches, key=lambda p: p.stat().st_mtime, reverse=True)):
        if number >= 20 or path.stat().st_size > 10 * 1024 * 1024:
            report['crash_reports'].append({'name': path.name, 'retained': False, 'reason': 'diagnostic size/count bound'})
            continue
        target = destination / f'{number}-{path.name}'
        shutil.copyfile(path, target)
        report['crash_reports'].append({'name': target.name, 'retained': True})
    (destination / 'identity.json').write_text(json.dumps(report, indent=2) + '\n')
    return report


def file_hash(path):
    digest = hashlib.sha256()
    with path.open('rb') as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b''):
            digest.update(chunk)
    return digest.hexdigest()


if __name__ == '__main__':
    collect(Path(sys.argv[1]), Path(sys.argv[2]), sys.argv[3], sys.argv[4],
            sys.argv[5], float(sys.argv[6]), Path.home())
