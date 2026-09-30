#!/usr/bin/env python3
"""Validate prerequisite study only; never certify product repairs or D1/D2."""
import hashlib
import json
from pathlib import Path
import re
import sys

from discovery import verify as discovery

ROOT = Path(__file__).resolve().parents[2]
OWNERS = {f'REQ-{i:03}': f'US-{i+11:03}' for i in range(1, 5)}
BASELINE = '09ac4feb2c7325b42f20a4663b9aa111d43fe386'
CONTRACT_SHA256 = '91f304bb8924b72896c6158ecb8851e23bf6f265d48331063542fe1fd86b9bf8'
# US-013 includes recapturePhase plus isRepairTask, attemptDescription and
# retryWithStopReason (T-US-013-003); expanded full-body denominator, no exclusions.
SCOPE_SHA256 = '000e6ef1b66b741dbf7687d0f817936f77692b3c963de23aa3009032ca144117'
DOC = 'docs/verification/repair-study.md'
JSON_FILES = ('repair-study-contract.json', 'repair-study-provenance.json',
              'repair-requirements.json', 'repair-coverage-scopes.json')
SECTIONS = ('Scope and provenance', 'Source and dependency assessment',
            'F1 — Legacy incident recapture', 'F2 — Public admitted evidence attachment',
            'F3 — Private evidence enters source commits', 'F4 — D2 delivery',
            'Coverage contract', 'Resources, authority and delivery', 'Study verification')
CASES = ('F1-empty-lint', 'F1-empty-test', 'F1-matching', 'F1-admitted', 'F1-native',
         'F1-success', 'F1-restart', 'F1-boundaries', 'F2-lint-test', 'F2-silent',
         'F2-tail', 'F2-results', 'F2-boundaries', 'F2-adoption', 'F3-fresh',
         'F3-existing', 'F3-console', 'F3-gates', 'F3-deterministic', 'F3-staging', 'F3-legacy')
SOURCES = {
    'internal/blueprint/engine.go': ('Execute', 'ExecuteStage'),
    'internal/pipeline/admitted_executor.go': ('Execute',),
    'internal/pipeline/pipeline.go': ('terminalEvidence',),
    'internal/execution/gates/failure.go': ('NewCommandFailure', 'CommandFailureWithEvidence', 'VerificationFailureArtifacts'),
    'internal/execution/evidence/capture.go': ('Write', 'Read', 'Public', 'PublicStream'),
    'pkg/runtime/execution.go': ('VerificationCommandFailure',),
    'pkg/runtime/evidence.go': ('RetainCommandEvidence', 'ReadCommandEvidence'),
    'pkg/manager/task_recapture.go': ('diagnosticFreeReceipt', 'resolveRecaptureCommand', 'recaptureTaskFailure'),
    'pkg/manager/task_failure.go': ('persistTaskVerificationFailure', 'repairTaskFromRetainedFailure'),
    'pkg/db/state/task_failure.go': ('RecordTaskFailureStep',),
    'internal/release/failure_repair.go': ('CreateFailureRepair', 'RecaptureEligible'),
    'internal/cli/init.go': ('ensureGitignore',),
    'internal/tools/safe_commit_tool.go': ('Execute',),
}


def read_json(root, name):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f'duplicate JSON key: {key}')
            result[key] = value
        return result
    return json.loads((root / 'docs/verification' / name).read_text(), object_pairs_hook=unique)


def verify(root):
    errors, count = discovery(root)
    def require(condition, message):
        if not condition:
            errors.append(message)
    doc = root / DOC
    body = doc.read_text()
    for title in SECTIONS:
        require(body.count(f'## {title}\n') == 1, f'missing/duplicate study section: {title}')
    for finding in ('F1', 'F2', 'F3', 'F4'):
        sections = re.findall(rf'^## {finding} — .*?\n(.*?)(?=^## |\Z)', body, re.S | re.M)
        require(len(sections) == 1, f'missing finding: {finding}')
        for section in sections:
            require('Disposition: accepted' in section or 'Disposition: rejected' in section,
                    f'missing disposition: {finding}')
            for item in ('Root cause', 'Cases', 'Fix', 'Prove', 'Falsify'):
                require(len(re.findall(rf'^- \*\*{item}:\*\* \S', section, re.M)) == 1,
                        f'missing/duplicate checklist: {finding}/{item}')
    for case in CASES:
        require(len(re.findall(r'\b' + re.escape(case) + r'\b', body)) == 1,
                f'missing/duplicate review case: {case}')
    for path in (doc, root / 'docs/ARCHITECTURE.md'):
        for link in re.findall(r'\]\(([^)]+)\)', path.read_text()):
            if '://' not in link:
                require((path.parent / link.split('#')[0]).is_file(), f'broken documentation link: {link}')
    for token in ('pending US-012', 'pending US-013', 'D2',
                  'safe_commit usage:', 'ordinary', 'Console retains', 'mode=ro',
                  'not independently', 'outside supplied', 'No measured'):
        require(token in body, f'missing evidence/authority limitation: {token}')
    require('pending US-014' in body or 'repaired by T-US-014-001' in body,
            'missing storage repair status: US-014')
    contract = read_json(root, JSON_FILES[0])
    provenance = read_json(root, JSON_FILES[1])
    requirements = read_json(root, JSON_FILES[2])
    scopes = read_json(root, JSON_FILES[3])
    digest = hashlib.sha256((root / 'docs/verification' / JSON_FILES[0]).read_bytes()).hexdigest()
    require(digest == CONTRACT_SHA256 == provenance['contract_sha256'], 'accepted contract snapshot changed')
    require(contract['goal']['id'] == 'G-007', 'wrong selected goal')
    sources = {f"stories/{s['id']}/contract": s['contract'] for s in contract['stories']}
    require(len(requirements) == len(OWNERS) and {r['id'] for r in requirements} == set(OWNERS),
            'missing/duplicate requirement')
    for row in requirements:
        require(row['id'] in OWNERS and row['owner'] == OWNERS.get(row['id']), 'wrong requirement owner')
        require(row['source'] == f"stories/{row['owner']}/contract" and
                row['exact_clause'] == sources.get(row['source']), 'changed exact accepted clause')
    require(provenance['feature'] == 'a8afdf3e98e69cffb4703cb41e39b86d' and
            provenance['review'] == '3942433c4242399f83195ad01d0a6748' and
            provenance['pull_request'] == 66 and provenance['task'] == 'T-US-010-001' and
            provenance['goal'] == 'G-007' and provenance['story'] == 'US-010', 'wrong retained candidate identity')
    candidate = provenance['candidate']
    require(candidate['revision'] == BASELINE and candidate['initial_status'] == '' and
            candidate['branch'] == 'outcome/' + provenance['feature'] and
            Path(candidate['path']).name == provenance['feature'] and
            re.fullmatch('[0-9a-f]{40}', candidate['tree']), 'invalid candidate provenance')
    console = provenance['console']
    require(console['revision'] == 'b5071d896d59b26f79cb9344dcb6b710f175a8c1' and
            console['status'] == '' and console['replace'] is False and
            console['dependency'] == 'v0.13.2-0.20260929072823-e5d026ab6196' and
            console['deployment'] == 'unverified', 'invalid Console evidence')
    require(set(console['files']) == {'go.mod', 'go.sum', 'internal/server/openexec_checks.go',
                                     'internal/server/openexec_check_conventions.go'} and
            all(re.fullmatch('[0-9a-f]{64}', h) for h in console['files'].values()),
            'missing Console source/dependency evidence')
    require(scopes['baseline'] == BASELINE and set(scopes['scopes']) == {'US-012', 'US-013', 'US-014'},
            'missing scope or changed baseline')
    scope_digest = hashlib.sha256((root / 'docs/verification' / JSON_FILES[3]).read_bytes()).hexdigest()
    require(scope_digest == SCOPE_SHA256, 'declared scope inventory/denominator changed; review required')
    declarations = {p: set(symbols) for p, symbols in SOURCES.items()}
    for owner, scope in scopes['scopes'].items():
        require(scope['minimum_statement_percent_exclusive'] == 90, f'coverage threshold changed: {owner}')
        require(bool(scope['functions']) and len(scope['functions']) == len(set(scope['functions'])),
                f'empty/duplicate scope: {owner}')
        for function in scope['functions']:
            path, symbol = function.split(':', 1)
            require(not Path(path).is_absolute() and '..' not in Path(path).parts,
                    f'unsafe scoped path: {path}')
            declarations.setdefault(path, set()).add(symbol.rsplit('.', 1)[-1])
    for path, symbols in declarations.items():
        file = root / path
        if not file.is_file():
            errors.append(f'missing scoped source: {path}')
            continue
        text = file.read_text()
        for symbol in symbols:
            require(re.search(rf'^func\s+(?:\([^\n]+?\)\s+)?{re.escape(symbol)}\s*\(', text, re.M),
                    f'missing scoped declaration: {path}:{symbol}')
    notes = (root / 'NOTES.md').read_text()
    require('T-US-010-001' in notes.split('## Now\n')[1].split('## Questions\n')[0], 'missing current task memory')
    require('US-010' in notes.split('## Questions\n')[1].split('## For me\n')[0], 'missing study questions')
    return errors, count + sum(len(s) for s in declarations.values())


def main():
    try:
        errors, count = verify(ROOT)
    except (OSError, ValueError, KeyError, TypeError, IndexError) as error:
        print(f'study failed: {error}', file=sys.stderr)
        return 1
    if errors:
        print('\n'.join(errors), file=sys.stderr)
        return 1
    print(f'study passed: {count} declaration checks; exact clauses, ownership, all review checklists, '
          'coverage scopes, provenance, docs and NOTES. Product repairs/D1/D2 not certified.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
