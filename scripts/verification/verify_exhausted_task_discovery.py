#!/usr/bin/env python3
"""Execute the frozen native baseline, then require fresh test and receipt evidence."""
import copy
import hashlib
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
FIXTURE = ROOT / 'scripts/fixtures/exhausted-task'
TEST = 'TestExhaustedNativeQueueDiscovery'
PACKAGE = 'github.com/openexec/openexec/pkg/manager'


INVENTORY = ROOT / 'scripts/verification/exhausted-task-inventory.json'
DOCUMENT = ROOT / 'docs/exhausted-task-discovery.md'
REQUIRED_SECTIONS = (
    'Observed baseline', 'Authority', 'Candidate binding', 'Bounded admission',
    'Accepted validation', 'Receipt retention', 'Historical attempts',
    'Completed repairs', 'Stop', 'Cancellation', 'Effects', 'Queue draining',
    'Reconciliation function inventory', 'Required OpenExec checks',
    'Evidence and unresolved boundaries',
)
# Explicit initial coverage denominator: removing an entry cannot shrink scope.
REQUIRED_FUNCTIONS = {
    'internal/release/failure_repair.go:(*Manager).CreateFailureRepair',
    'internal/release/failure_repair.go:(*Manager).RecaptureEligible',
    'internal/release/failure_repair.go:(*SQLiteStore).CreateFailureRepair',
    'internal/release/failure_repair.go:runnableTasks',
    'internal/release/manager.go:(*Manager).SetTaskStatus',
    'internal/release/manager.go:(*Manager).UpdateTask',
    'internal/release/sqlite_store.go:(*SQLiteStore).CanCompleteTask',
    'pkg/db/state/task_failure.go:(*Store).RecordTaskFailureStep',
    'pkg/manager/manager.go:(*Manager).Stop',
    'pkg/manager/manager.go:(*Manager).Wait',
    'pkg/manager/manager.go:(*Manager).start',
    'pkg/manager/scheduler.go:(*Manager).ExecuteTasks',
    'pkg/manager/task_boundary.go:retainedTaskBoundary',
    'pkg/manager/task_execution_lock.go:(*Manager).lockTaskExecution',
    'pkg/manager/task_execution_lock.go:(*Manager).reconcileInterruptedTasks',
    'pkg/manager/task_execution_lock.go:entryFinished',
    'pkg/manager/task_execution_lock.go:isRepairTask',
    'pkg/manager/task_failure.go:(*Manager).persistTaskVerificationFailure',
    'pkg/manager/task_failure.go:(*Manager).repairTaskFromRetainedFailure',
    'pkg/manager/task_queue.go:(*Manager).executeTaskQueue',
    'pkg/manager/task_queue.go:(*Manager).waitTaskQueueRun',
    'pkg/manager/task_queue.go:retryWithStopReason',
    'pkg/manager/task_recapture.go:(*Manager).diagnosticFreeReceipt',
    'pkg/manager/task_recapture.go:(*Manager).recaptureTaskFailure',
    'pkg/manager/task_recapture.go:(*Manager).resolveRecaptureCommand',
    'pkg/manager/task_recapture.go:recapturePhase',
}


def validate_discovery(inventory, document):
    require(inventory['schema_version'] == 1, 'inventory version')
    rows = inventory['functions']
    identities = [row['path'] + ':' + row['symbol'] for row in rows]
    require(len(identities) == len(set(identities)), 'duplicate inventory function')
    require(REQUIRED_FUNCTIONS <= set(identities), 'missing inventory function')
    for row in rows:
        path = ROOT / row['path']
        require(path.resolve().is_relative_to(ROOT), 'inventory path outside repository')
        require(path.suffix == '.go' and not path.name.endswith('_test.go'), 'not production function')
        require(row['declaration'] in path.read_text().splitlines(), 'missing source declaration')
        match = re.match(r'func (?:\(\w+ \*(\w+)\) )?(\w+)\(', row['declaration'])
        require(match is not None, 'invalid function declaration')
        symbol = ('(*' + match[1] + ').' if match[1] else '') + match[2]
        require(symbol == row['symbol'], 'wrong source symbol')
        require('`' + row['path'] + '`: `' + symbol + '`' in document, 'undocumented inventory function')
    for section in REQUIRED_SECTIONS:
        marker = '## ' + section + '\n'
        require(document.count(marker) == 1, 'missing or duplicate discovery section: ' + section)
        body = document.split(marker)[1].split('\n## ')[0].strip()
        require(len(body) > 100, 'empty discovery section: ' + section)
    for token in ('verification/exhausted-task-discovery-result.json',
                  'make lint', 'make test', 'make type-check', 'make compat-test',
                  '.github/workflows/ci.yml', '.openexec/openexec.yaml',
                  'historical context only', 'Settings commit', 'hooks drift'):
        require(token in document, 'missing documentation evidence: ' + token)


def validate_provenance(result, provenance):
    require(result['baseline_revision'] == provenance['revision'], 'wrong baseline revision')
    require(result['baseline_archive_sha256'] == provenance['archive_sha256'], 'wrong baseline digest')
    require(result['exit_code'] == 0, 'baseline command failed')
    require(result['command'] == ['go', 'test', '-mod=readonly', '-json', './pkg/manager',
                                '-run', '^' + TEST + '$', '-count=1', '-timeout=60s'], 'wrong baseline command')
    for name in ('scenario.json', 'reproduction_test.go.txt'):
        require(result['fixture_sha256'][name] == hashlib.sha256((FIXTURE / name).read_bytes()).hexdigest(),
                'stale fixture evidence')


def require(condition, reason):
    if not condition:
        raise ValueError(reason)


def validate(events, result, scenario):
    relevant = [e for e in events if e.get('Package') == PACKAGE]
    require(not any(e.get('Action') in ('skip', 'fail') for e in relevant), 'failed or skipped reproduction')
    require(sum(e.get('Test') == TEST and e.get('Action') == 'run' for e in relevant) == 1, 'missing reproduction run')
    require(sum(e.get('Test') == TEST and e.get('Action') == 'pass' for e in relevant) == 1, 'missing reproduction pass')
    require(any(e.get('Action') == 'pass' and 'Test' not in e for e in relevant), 'missing package pass')
    for key in ('task_id', 'candidate', 'branch', 'acceptance', 'correction_authorization', 'receipt_id'):
        require(bool(scenario[key]) and result[key] == scenario[key], 'missing or mismatched ' + key)
    expected = dict(test=TEST, entry_point='Manager.ExecuteTasks(TaskOriented=true)', attempts=3,
                    max_attempts=3, status='failed', prerequisite_status='done',
                    refusals=['task repair attempt limit reached'] * 2, reopened=True,
                    candidate_corrected=True, corrected_check_exit=0, pipeline_count=0, task_count=2)
    for key, value in expected.items():
        require(result[key] == value, 'incorrect evidence: ' + key)
    receipt = result['receipt']
    payload = receipt['verification_failure_receipt']
    require(hashlib.sha256(payload.encode()).hexdigest() == receipt['verification_failure_digest'], 'receipt digest mismatch')
    require(json.loads(payload) == [dict(gate='test', exit_code=124, command='sh check.sh',
                                        output='verification timeout: deadline exceeded\n')], 'underlying timeout absent')


class VerifierControls(unittest.TestCase):
    def test_inventory_and_document_refusals(self):
        inventory = json.loads(INVENTORY.read_text())
        document = DOCUMENT.read_text()
        validate_discovery(inventory, document)
        for mutate in ('missing', 'duplicate', 'symbol', 'declaration'):
            damaged = copy.deepcopy(inventory)
            if mutate == 'missing':
                damaged['functions'].pop()
            elif mutate == 'duplicate':
                damaged['functions'].append(damaged['functions'][0])
            else:
                damaged['functions'][0][mutate] = 'invented'
            with self.subTest(mutate=mutate), self.assertRaises(ValueError):
                validate_discovery(damaged, document)
        for section in REQUIRED_SECTIONS:
            with self.subTest(section=section), self.assertRaises(ValueError):
                validate_discovery(inventory, document.replace('## ' + section + '\n', ''))

    def test_stale_baseline_refused(self):
        provenance = json.loads((FIXTURE / 'provenance.json').read_text())
        result = json.loads((ROOT / 'docs/verification/exhausted-task-discovery-result.json').read_text())
        validate_provenance(result, provenance)
        for key in ('baseline_revision', 'baseline_archive_sha256', 'exit_code', 'command', 'fixture_sha256'):
            damaged = copy.deepcopy(result)
            damaged[key] = {} if key == 'fixture_sha256' else 'wrong'
            with self.subTest(key=key), self.assertRaises((ValueError, KeyError)):
                validate_provenance(damaged, provenance)

    def test_missing_test(self):
        with self.assertRaisesRegex(ValueError, 'missing reproduction run'):
            validate([], {}, {})

    def test_skipped_test(self):
        with self.assertRaisesRegex(ValueError, 'skipped'):
            validate([dict(Package=PACKAGE, Test=TEST, Action='skip')], {}, {})

    def test_missing_evidence(self):
        events = [dict(Package=PACKAGE, Test=TEST, Action=a) for a in ('run', 'pass')]
        events.append(dict(Package=PACKAGE, Action='pass'))
        with self.assertRaises((ValueError, KeyError)):
            validate(events, {}, json.loads((FIXTURE / 'scenario.json').read_text()))


def main():
    controls = unittest.TextTestRunner().run(unittest.defaultTestLoader.loadTestsFromTestCase(VerifierControls))
    require(controls.wasSuccessful(), 'verifier controls failed')
    provenance = json.loads((FIXTURE / 'provenance.json').read_text())
    archive = FIXTURE / 'baseline.tar.gz'
    require(hashlib.sha256(archive.read_bytes()).hexdigest() == provenance['archive_sha256'], 'baseline archive changed')
    scenario = json.loads((FIXTURE / 'scenario.json').read_text())
    # A fresh temporary output prevents an old successful receipt satisfying this run.
    with tempfile.TemporaryDirectory(prefix='openexec-exhausted-') as temp:
        work = Path(temp)
        with tarfile.open(archive) as tar:
            require(set(tar.getnames()) == set(provenance['files']), 'baseline source missing')
            tar.extractall(work, filter='data')
        for name, digest in provenance['files'].items():
            require(hashlib.sha256((work / name).read_bytes()).hexdigest() == digest, 'source digest mismatch: ' + name)
        shutil.copyfile(FIXTURE / 'reproduction_test.go.txt', work / 'pkg/manager/exhausted_discovery_test.go')
        (work / 'pkg/manager/testdata').mkdir(exist_ok=True)
        shutil.copyfile(FIXTURE / 'scenario.json', work / 'pkg/manager/testdata/exhausted-task.json')
        env = os.environ.copy()
        env['GOWORK'] = 'off'
        env['GOFLAGS'] = ''
        env['GOCACHE'] = str(Path(tempfile.gettempdir()) / 'openexec-exhaustion-cache')
        env['EXHAUSTED_RESULT'] = str(work / 'result.json')
        command = ['go', 'test', '-mod=readonly', '-json', './pkg/manager', '-run', '^' + TEST + '$', '-count=1', '-timeout=60s']
        completed = subprocess.run(command, cwd=work, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        print(completed.stderr, end='')
        require(completed.returncode == 0, 'native reproduction failed:\n' + completed.stdout)
        events = [json.loads(line) for line in completed.stdout.splitlines() if line.strip()]
        require((work / 'result.json').is_file(), 'fresh fixture result absent')
        result = json.loads((work / 'result.json').read_text())
        validate(events, result, scenario)
        # Prove that missing receipt evidence is refused even with a passing test stream.
        damaged = copy.deepcopy(result)
        del damaged['receipt']
        try:
            validate(events, damaged, scenario)
        except (ValueError, KeyError):
            pass
        else:
            raise ValueError('absent receipt accepted')
        result['baseline_revision'] = provenance['revision']
        result['baseline_archive_sha256'] = provenance['archive_sha256']
        result['fixture_sha256'] = {name: hashlib.sha256((FIXTURE / name).read_bytes()).hexdigest() for name in ('scenario.json', 'reproduction_test.go.txt')}
        result['command'] = command
        result['exit_code'] = completed.returncode
        result['verifier_controls'] = ['missing_test_refused', 'skipped_test_refused', 'missing_evidence_refused', 'absent_receipt_refused']
        destination = ROOT / 'docs/verification/exhausted-task-discovery-result.json'
        validate_provenance(result, provenance)
        validate_discovery(json.loads(INVENTORY.read_text()), DOCUMENT.read_text())
        result['discovery_inventory_sha256'] = hashlib.sha256(INVENTORY.read_bytes()).hexdigest()
        result['discovery_document_sha256'] = hashlib.sha256(DOCUMENT.read_bytes()).hexdigest()
        result['verifier_controls'] += ['inventory_refusals', 'documentation_refusals', 'stale_baseline_refused']
        destination.write_text(json.dumps(result, indent=2) + '\n')
        persisted = json.loads(destination.read_text())
        validate(events, persisted, scenario)
        validate_provenance(persisted, provenance)
        require(persisted == result, 'persisted discovery evidence changed')
        print('Exhausted native discovery PASS; fresh persisted result reread:', destination.relative_to(ROOT))


if __name__ == '__main__':
    main()
