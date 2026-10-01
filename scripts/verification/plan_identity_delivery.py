"""Read-only preparation verifier; external delivery authority stays with Console."""
import argparse
from datetime import datetime, timezone, timedelta
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[2]
RECORD = 'docs/verification/plan-identity-delivery.json'
MANIFEST = 'docs/verification/plan-identity-content.sha256'
INVENTORY = 'scripts/verification/plan-identity-discovery.json'
FEATURE = '6fbdb8c642f3b58c1788c74e84fa0860'
REVIEW = 'f320866fc267dc32777060cc8c16c646'
PR = 'https://github.com/openexec/openexec/pull/78'
CHECKS = ('targeted', 'coverage', 'mutations', 'compat-test', 'restored')
VERIFIER_FILES = {
    'scripts/verify-plan-identity-delivery-evidence.sh',
    'scripts/verification/plan_identity_delivery.py',
    'scripts/verification/plan_identity_delivery_test.py',
    *('scripts/verification/fixtures/plan-identity-delivery/' + name + '.json'
      for name in ('complete', 'incomplete', 'stale')),
}
MUTATIONS = {
    'story SQL NULL reader': 'converting NULL to string is unsupported',
    'task SQL NULL reader': 'converting NULL to string is unsupported',
    'content allocation': 'reviewed stories US-001 conflicts with retained content',
    'atomic conflict refusal': 'genuine conflict not refused',
    'native replay': 'identical native replay moved US-001',
    'native JSON writer': 'got null want []',
    'legacy comparator': 'exact legacy replay changed plan identities/content',
    'legacy BLOB predicate': 'reviewed stories US-001 conflicts with retained content',
    'SQL predicate': 'reviewed stories US-001 conflicts with retained content',
}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def local(root, name):
    require(isinstance(name, str) and name and not Path(name).is_absolute(), 'nonlocal path')
    path = (root / name).resolve()
    require(path.is_relative_to(root.resolve()) and path.is_file(), 'missing/nonlocal file: ' + name)
    return path


def pairs(items):
    result = {}
    for key, value in items:
        require(key not in result, 'duplicate JSON key: ' + key)
        result[key] = value
    return result


def read(path):
    return json.loads(path.read_text(), object_pairs_hook=pairs)


def binding(root, record):
    manifest = local(root, MANIFEST)
    require(record['content_sha256'] == sha(manifest), 'stale manifest digest')
    names = []
    for line in manifest.read_text().splitlines():
        match = re.fullmatch(r'([0-9a-f]{64})  (.+)', line)
        require(match is not None, 'malformed content manifest')
        digest, name = match.groups()
        require(sha(local(root, name)) == digest, 'stale content digest: ' + name)
        names.append(name)
    require(names and names == sorted(set(names)), 'empty/duplicate/unsorted content manifest')
    # The stored inventory cannot omit changed production, test or fixture files.
    tracked = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others',
                                       '--exclude-standard'], cwd=root, text=True).split('\0')
    require(set(record['verifier_files']) == VERIFIER_FILES, 'missing verifier content binding')
    for name, digest in record['verifier_files'].items():
        require(sha(local(root, name)) == digest, 'stale verifier content: ' + name)
    protected = {n for n in tracked if n.startswith((
        'internal/planner/', 'internal/release/', 'pkg/manager/', 'scripts/verification/'))}
    protected.update(n for n in tracked if n.startswith('scripts/verify-plan-identity-') or
                     n == 'scripts/verify-reviewed-plan-identity.sh')
    protected.update((INVENTORY, 'CLAUDE.md', 'Makefile', 'go.mod', 'go.sum'))
    require(protected <= set(names) | VERIFIER_FILES, 'content manifest omits protected candidate files')
    return set(names)


def validate(root, record, merge=None, goal_complete=False):
    require(record['schema'] == 1 and record['feature'] == FEATURE and
            record['review'] == REVIEW and record['pull_request'] == PR and
            record['branch'] == 'outcome/' + FEATURE, 'wrong candidate provenance')
    bound = binding(root, record)
    findings = record['findings']
    require(len(findings) == 1 and findings[0]['index'] == 0, 'missing/duplicate finding disposition')
    finding = findings[0]
    require(finding['decision'] in ('accepted', 'rejected') and finding['reason'].strip(),
            'missing finding disposition/reason')
    require(finding['evidence'] and all(p in bound for p in finding['evidence']), 'missing repair evidence')
    inventory = read(local(root, INVENTORY))
    require(set(record['checks']) == set(CHECKS), 'absent required checks or unknown outcomes')
    logs = {}
    for name in CHECKS:
        check = record['checks'][name]
        require(check['command'] == 'bash scripts/verify-plan-identity-compat.sh ' + name,
                'wrong check command: ' + name)
        require(type(check['exit_code']) is int and check['exit_code'] == 0 and check['passed'] is True,
                'contradictory check outcome: ' + name)
        require(check['content_sha256'] == record['content_sha256'], 'stale check content: ' + name)
        log = local(root, check['log'])
        require(sha(log) == check['sha256'], 'stale verification log: ' + name)
        text = log.read_text()
        require(text.splitlines().count('PASS identity compatibility: ' + name) == 1,
                'missing/duplicate check completion: ' + name)
        require(not re.search(r'(?m)^\s*(FAIL\b|FAILED\b|SKIP\b|Traceback|--- FAIL|--- SKIP)', text),
                'contradictory log outcome: ' + name)
        logs[name] = text
    for name in ('targeted', 'coverage', 'restored'):
        executed = re.findall(r'^PASS ((?:pkg|internal)/\S+)$', logs[name], re.M)
        require(len(executed) == len(set(executed)) and set(executed) == set(inventory['existing_tests']),
                'missing mandatory test execution: ' + name)
    for test in ('ExistingProjects_StatusCLI', 'LegacyProjectConfigFallback', 'LegacyTasksJSONFallback'):
        require('PASS internal/validation:TestCompatibility_' + test in logs['compat-test'],
                'missing compatibility journey: ' + test)
    measured = {}
    for function, covered, statements, percent in re.findall(
            r'^(\S+): (\d+)/(\d+) \(([\d.]+)%\)$', logs['coverage'], re.M):
        require(function not in measured, 'duplicate coverage function')
        covered, statements = int(covered), int(statements)
        require(0 < covered <= statements and covered * 100 > statements * 90,
                'coverage must exceed 90%: ' + function)
        require(abs(float(percent) - covered * 100 / statements) <= .0051,
                'contradictory coverage percentage')
        measured[function] = [covered, statements]
    require(set(measured) == set(inventory['functions']), 'missing required function coverage')
    require(record['coverage'] == measured, 'contradictory stored coverage')
    expected = ['PASS mutation: ' + name + ' compiled; expected assertion: ' + assertion
                for name, assertion in MUTATIONS.items()]
    actual = [line for line in logs['mutations'].splitlines() if line.startswith('PASS mutation: ')]
    require(sorted(actual) == sorted(expected) and 'PASS mutation copies cleaned' in logs['mutations'],
            'missing expected mutation failures or cleanup')
    require(record['preparation'] == 'ready', 'contradictory preparation outcome')
    require(record['d2'] in ('pending', 'merged'), 'unknown D2 outcome')
    require(record['goal_complete'] is (record['d2'] == 'merged'), 'contradictory Goal/D2 outcomes')
    result = dict(preparation='ready', d2='pending', goal_complete=False, passed=True)
    if merge is not None or record['d2'] == 'merged' or goal_complete:
        # Reuse the repository's delivery receipt contract. The separate argument
        # is a trusted coordinator export, never a self-attestation in this record.
        from delivery import check_merge, candidate_files
        require(merge is not None, 'current coordinator-supplied merge evidence required')
        require(merge['issuer'] == 'agent-console' and merge['feature'] == FEATURE and
                merge['pull_request'] == PR, 'wrong coordinator merge provenance')
        observed = datetime.fromisoformat(merge['observed_at'].replace('Z', '+00:00'))
        require(observed.tzinfo is not None and timedelta(0) <= datetime.now(timezone.utc) - observed <= timedelta(hours=24),
                'stale coordinator observation')
        revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()
        require(root.resolve() == ROOT.resolve(), 'merge verification requires actual candidate checkout')
        check_merge(merge, revision, candidate_files())
        require(merge['default_branch_merger'].strip(), 'missing default-branch merger')
        result.update(d2='verified_external_merge', goal_complete=True)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--record', default=RECORD)
    parser.add_argument('--merge-evidence', type=Path)
    parser.add_argument('--goal-complete', action='store_true')
    args = parser.parse_args()
    result = dict(passed=False, preparation='incomplete', d2='unverified', goal_complete=False)
    try:
        record = read(local(ROOT, args.record))
        # Report preparation independently even when external merger proof fails.
        preparation = dict(record, d2='pending', goal_complete=False)
        validate(ROOT, preparation)
        result['preparation'] = 'ready'
        result.update(validate(ROOT, record, read(args.merge_evidence) if args.merge_evidence else None,
                               args.goal_complete))
    except (ValueError, OSError, KeyError, TypeError, AttributeError, subprocess.SubprocessError) as error:
        result['error'] = str(error)
    print(json.dumps(result, sort_keys=True))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
