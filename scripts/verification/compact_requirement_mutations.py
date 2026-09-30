"""Separate reviewer controls; require named Go assertion failures, restore bytes."""
import hashlib
import json
import signal
import subprocess

import compact_requirement_evidence as evidence

ROOT = evidence.ROOT
OUTPUT = ROOT / '.openexec/compact-requirement-checks'
TEST = 'TestAllPlanningPathsRespectSchemaAndCommitOwnership/compact'


def intended_failure(result, reason):
    events = [json.loads(line) for line in result.stdout.splitlines()]
    failures = {e.get('Test') for e in events if e.get('Action') == 'fail' and e.get('Test')}
    return (result.returncode == 1 and TEST in failures
            and failures <= {TEST, TEST.split('/')[0]}
            and any(e.get('Test') == TEST and reason in e.get('Output', '') for e in events))


def interrupted(signum, frame):
    raise RuntimeError(f'interrupted by signal {signum}')


def main():
    OUTPUT.mkdir(parents=True, exist_ok=True)
    report = OUTPUT / 'mutations.json'
    report.unlink(missing_ok=True)
    path = ROOT / 'internal/planner/prompt.go'
    original = path.read_bytes()
    source = original.decode()
    start = source.index('const CompactStoryGenerationPrompt =')
    end = source.index('INTENT DOCUMENT:', start)
    compact = source[start:end]
    cases = [('output-key', ',"requirement_id":"REQ-001"', '', 'output format omits scalar requirement_id'),
             ('identity-rule', '` + RequirementIdentityRule + `\n', '', 'provider prompt omits canonical empty requirement semantics')]
    results = []
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, interrupted)
    try:
        for name, before, after, reason in cases:
            if compact.count(before) != 1:
                raise ValueError(f'mutation target is not unique: {name}')
            try:
                path.write_text(source[:start] + compact.replace(before, after) + source[end:])
                command = ['go', 'test', './internal/planner', '-run', '^' + TEST + '$', '-count=1', '-json']
                result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True)
                (OUTPUT / (name + '.jsonl')).write_text(result.stdout + result.stderr)
                if not intended_failure(result, reason):
                    raise ValueError(f'{name} did not fail intended assertion: {result.stdout}{result.stderr}')
                results.append(dict(mutation=name, assertion=TEST, diagnostic=reason, exit_code=result.returncode))
            finally:
                path.write_bytes(original)
                if path.read_bytes() != original:
                    raise ValueError('source restoration mismatch')
    finally:
        path.write_bytes(original)
    report.write_text(json.dumps(dict(mutations=results, restored=True,
        restored_sha256=hashlib.sha256(path.read_bytes()).hexdigest(), source_sha256=evidence.source_digest()), indent=2) + '\n')
    print(report.read_text())


if __name__ == '__main__':
    main()
