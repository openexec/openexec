"""Validate checkout-bound obligation documentation, not delivery or authority."""
import re
import sys
from pathlib import Path

import architecture_contracts

# Accepted obligation names, owners and minimum evidence vocabulary. These are
# documentation checks, deliberately not substitutes for runtime verification.
OBLIGATIONS = {
    'acceptedContract.goal': ('OpenExec engine', ('trusted', 'persisted', 'exported verification', 'native repair', 'unauthorized')),
    'D1 engine reproduction and recovery': ('OpenExec engine', ('pre-fix', 'post-fix', 'admitted executor', 'exit 1', 'reload', 'task/stage/attempt/source', 'repair execution', 'resumption', 'queue continuation', 'receipts')),
    'D1 engine refusal and compatibility': ('OpenExec engine', ('exit 0', '1 through 125', 'replay/restart', 'worker-forged', 'cancellation', 'timeout', 'launch/transport', 'mixed errors', 'reopened', 'exec.ExitError', '.openexec/.uaos/tasks.json')),
    'D1 engine validation': ('OpenExec engine', ('greater than 90%', 'unit statement coverage', 'modified production functions', 'external consumer', 'make test', 'make compat-test', 'make type-check')),
    'D1 downstream producer/projection': ('Agent Console', ('producer/projection', 'terminal exit', 'task/stage/attempt/source', 'trusted persistence', 'reload', 'provenance')),
    'D1 downstream production wiring and policy matching': ('Agent Console', ('engine revision', 'production wiring', 'execution policy', 'policy matching')),
    'D1 downstream verification': ('Agent Console', ('unchanged launch-and-policy', 'native-stage', 'real-HTTP', 'coverage', 'repository gates', 'candidate')),
    'D2 default-branch merge': ('Agent Console delivery, with owner decision', ('pull request', 'canonical gate', 'independent review', 'candidate-bound owner merge decision', 'default-branch merge evidence')),
}
BOUNDARY = {
    'Task': '`T-US-010-002`',
    'Mode': '`hitl`',
    'Depends on': '`T-US-010-001`',
    'Decision reason': "The owner must make the exact merge decision after Agent Console attaches the published pull request and required review evidence; accepted repair scope does not supply that candidate-specific decision.",
    'Decision reference': 'absent; no authentic candidate-bound owner decision was supplied.',
}


def section(content, title):
    matches = re.findall(r'^## ' + re.escape(title) + r'\n(.*?)(?=^## |\Z)', content, re.M | re.S)
    if len(matches) != 1 or not matches[0].strip():
        raise ValueError(f'missing, empty or duplicate section: {title}')
    return matches[0]


def require(text, terms, label):
    for term in terms:
        if term.lower() not in text.lower():
            raise ValueError(f'{label}: missing {term}')


def verify(root):
    count = architecture_contracts.verify(root)
    content = (root / 'docs/ARCHITECTURE.md').read_text()
    trace = section(content, 'Accepted obligation traceability')
    rows = {}
    for line in trace.splitlines():
        if not line.startswith('|'):
            continue
        cells = [cell.strip() for cell in line.strip('|').split('|')]
        if cells[0] == 'Obligation' or cells[0].startswith('---'):
            continue
        if len(cells) != 4 or cells[0] in rows:
            raise ValueError('malformed or duplicate obligation row')
        rows[cells[0]] = cells[1:]
    if rows.keys() != OBLIGATIONS.keys():
        raise ValueError('missing or unexpected obligation rows')
    for name, (owner, terms) in OBLIGATIONS.items():
        actual_owner, evidence, finding = rows[name]
        if actual_owner != owner:
            raise ValueError(f'{name}: incorrect responsible party')
        require(evidence, terms, name)
        expected = 'Unverified:' if owner == 'OpenExec engine' else 'Pending:'
        if not finding.startswith(expected) or not finding[len(expected):].strip():
            raise ValueError(f'{name}: unsupported completion claim')
    boundary = section(content, 'Promotion controls and retained owner boundary')
    for key, value in BOUNDARY.items():
        if re.findall(r'^- ' + re.escape(key) + r': (.*)$', boundary, re.M) != [value]:
            raise ValueError(f'retained boundary changed: {key}')
    require(' '.join(boundary.split()), (
        'canonical gate', 'independent review', 'do not publish',
        'merge or deploy', 'filterAutoDispatchable', 'single boundary',
        'unauthorized or candidate/PR-mismatched', 'synthetic fixtures never constitute acceptance',
        'No grant or decision reference is invented',
    ), 'promotion controls')
    history = section(content, 'Historical claims and external evidence')
    require(' '.join(history.split()), ('historical, unverified', 'Checkout notes',
        'independently verified', 'neither', 'deployed revision'), 'historical evidence')
    section(content, 'Traceability verification')
    return count


if __name__ == '__main__':
    try:
        count = verify(Path(sys.argv[1]).resolve())
    except (OSError, ValueError, IndexError) as error:
        print(f'architecture-traceability: FAIL: {error}', file=sys.stderr)
        sys.exit(1)
    print(f'architecture-traceability: PASS ({len(OBLIGATIONS)} obligations; '
          f'{count} source declarations; documentation only)')
