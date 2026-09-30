#!/usr/bin/env python3
"""Run the private storage journey and path/refusal regression checks.

Only test identities and dispositions are published, never subprocess fixtures.
"""
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
REQUIRED = {
    'TestPrivateStorageRepositoryJourney/' + layout + '/' + kind
    for layout in ('fresh', 'initialized', 'whole-tree')
    for kind in ('gate', 'deterministic')
} | {
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


def main():
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
    print(json.dumps({'case': 'private-storage', 'status': 'passed' if ok else 'failed',
                      'exit_code': result.returncode, 'required_passed': len(REQUIRED & passed),
                      'missing': missing, 'failed_or_skipped': failed}, sort_keys=True))
    return 0 if ok else 1


if __name__ == '__main__':
    sys.exit(main())
