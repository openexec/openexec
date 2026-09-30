#!/usr/bin/env python3
"""Run the private storage journey and path/refusal regression checks.

Only test identities and dispositions are published, never subprocess fixtures.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
JOURNEYS = {
    'TestPrivateStorageRepositoryJourney/' + layout + '/' + kind
    for layout in ('fresh', 'initialized', 'whole-tree')
    for kind in ('gate', 'deterministic')
}
REQUIRED = JOURNEYS | {
    'TestPrivateStorageLegacyRefusal',
    'TestPrivateStorageNestedPathRefusals',
    'TestPrivateStorageReferenceReload/protected',
    'TestPrivateStorageReferenceReload/legacy-refused',
    'TestRecaptureUnitResolutionAndEvidence/legacy-path',
    'TestRetentionUnitPrivateRoundTripAndRefusals',
    'TestRetentionBoundariesGateCapture',
    'TestRetentionUnitNativeCallbacksAndStorageRefusal',
    'TestLegacyRecaptureFailureReloadRepair',
}


# A compile failure or a path-only mismatch is not evidence of source exposure.
EXPOSURE = ("private artifact not ignored", "capture dirtied repository",
            "capture entered index", "private artifact tracked")


def negative_result(result):
    events = [json.loads(line) for line in result.stdout.splitlines() if line.startswith('{')]
    failed = {e.get('Test') for e in events if e.get('Action') == 'fail'}
    output = {name: ''.join(e.get('Output', '') for e in events if e.get('Test') == name)
              for name in JOURNEYS}
    detected = sorted(name for name in JOURNEYS if name in failed
                      and all(marker in output[name] for marker in EXPOSURE))
    skipped = any(e.get('Action') == 'skip' for e in events)
    return {'status': 'passed' if result.returncode == 1 and not skipped
            and len(detected) == len(JOURNEYS) else 'failed',
            'exit_code': result.returncode, 'source_and_staging_detected': detected}


def negative_control():
    source = ROOT / 'internal/execution/evidence/capture.go'
    original = source.read_text()
    mutated = original
    for before, after in (
        ('const directory = ".openexec/data/verification"',
         'const directory = ".openexec-verification"'),
        ('[]string{".openexec", "data", "verification"}', '[]string{".openexec-verification"}'),
        ('name == "verification"', 'name == ".openexec-verification"'),
    ):
        if mutated.count(before) != 1:
            raise ValueError('old-directory mutation anchor changed')
        mutated = mutated.replace(before, after)
    with tempfile.TemporaryDirectory(prefix='private-storage-control-') as tmp:
        tmp = Path(tmp)
        replacement = tmp / 'capture.go'
        replacement.write_text(mutated)
        overlay = tmp / 'overlay.json'
        overlay.write_text(json.dumps({'Replace': {str(source): str(replacement)}}))
        result = subprocess.run([
            'go', 'test', '-overlay', str(overlay), './internal/cli', '-json',
            '-count=1', '-timeout=60s', '-run', '^TestPrivateStorageRepositoryJourney$',
        ], cwd=ROOT, env=dict(os.environ, GOCACHE='/tmp/openexec-retention-go-cache'),
            capture_output=True, text=True, timeout=120)
    if source.read_text() != original:
        raise ValueError('negative control changed candidate source')
    return negative_result(result)


def main():
    subprocess.run([sys.executable, '-m', 'unittest', 'discover', '-s',
                    'scripts/verification', '-p', 'test_private_storage.py'],
                   cwd=ROOT, check=True)
    result = subprocess.run([
        'go', 'test', './internal/execution/evidence', './internal/execution/gates',
        './internal/blueprint', './internal/cli', './pkg/manager', '-json',
        '-count=1', '-timeout=60s', '-run',
        'TestPrivateStorage|TestRetention.*(Capture|PrivateRoundTrip|StorageRefusal)'
        '|TestRecaptureUnitResolution|TestLegacyRecaptureFailureReloadRepair',
    ], cwd=ROOT, env=dict(os.environ, GOCACHE='/tmp/openexec-retention-go-cache'),
        capture_output=True, text=True, timeout=120)
    events = [json.loads(line) for line in result.stdout.splitlines() if line.startswith('{')]
    passed = {e.get('Test') for e in events if e.get('Action') == 'pass'}
    failed = [e.get('Test', e.get('Package')) for e in events
              if e.get('Action') in ('fail', 'skip')]
    missing = sorted(REQUIRED - passed)
    ok = result.returncode == 0 and not failed and not missing
    negative = negative_control() if ok else {'status': 'not-run'}
    ok = ok and negative['status'] == 'passed'
    print(json.dumps({'case': 'private-storage', 'status': 'passed' if ok else 'failed',
                      'exit_code': result.returncode, 'required_passed': len(REQUIRED & passed),
                      'missing': missing, 'failed_or_skipped': failed,
                      'old_directory_control': negative}, sort_keys=True))
    return 0 if ok else 1


if __name__ == '__main__':
    sys.exit(main())
