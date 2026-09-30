"""Validate local discovery/repair evidence and delivery obligations, never a merge."""
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
MANIFEST = 'docs/verification/compact-requirement-scope.json'
EVIDENCE = 'docs/verification/compact-requirement-evidence.md'
BASELINE = 'b17e136dab39bfccd4104bb9264e30595ea9118e'
# Frozen minimum; later coverage work may extend but may not narrow this scope.
REQUIRED = {
    'internal/planner/planner.go': {'Story.UnmarshalJSON', 'ProjectPlan.Validate',
        'Planner.GeneratePlan', 'Planner.GenerateCompactPlan', 'Planner.parseResponse'},
    'internal/planner/review.go': {'Planner.ReviewPlan', 'Planner.RefinePlan', 'carryGoals'},
    'pkg/manager/planner.go': {'Manager.Plan'},
    'pkg/manager/planner_replay.go': {'Manager.replayReviewedPlan'},
}
SECTIONS = ['Scope and provenance', 'Finding decisions',
    'Reusable fixtures and protected behavior', 'Executable coverage scope',
    'G-007 completion and verification', 'PR73 and delivery evidence',
    'Verification and outstanding questions']


def require(ok, message):
    if not ok:
        raise ValueError(message)


def git(*args):
    return subprocess.check_output(['git', '-C', str(ROOT), *args], text=True).strip()


def read_local(path):
    resolved = (ROOT / path).resolve()
    require(resolved.is_relative_to(ROOT), f'nonlocal source: {path}')
    return resolved.read_text()


def symbols(source):
    # Go declarations start at column zero; include receiver type in identity.
    pattern = r'^func (?:\(\w+ \*?(\w+)\) )?(\w+)\('
    return {(f'{m[1]}.' if m[1] else '') + m[2]: source[:m.start()].count('\n') + 1
            for m in re.finditer(pattern, source, re.M)}


def validate(data, evidence):
    require(data['version'] == 1 and data['task'] == 'T-US-009-001'
            and data['story'] == 'US-009', 'wrong discovery identity')
    require(data['baseline'] == BASELINE, 'baseline changed')
    require('Additive only' in data['scope_policy'], 'scope must remain additive')
    require(hashlib.sha256(data['contract'].encode()).hexdigest() == CONTRACT_HASH,
            'story completion contract changed')
    require(hashlib.sha256(data['repair_verification'].encode()).hexdigest() == VERIFY_HASH,
            'complete repair verification changed')
    for section in SECTIONS:
        require(evidence.count('## ' + section + '\n') == 1,
                f'missing/duplicate evidence section: {section}')
    for case in ['Root cause', 'Normal', 'Review', 'Refinement', 'Test gap', 'Fix', 'Prove', 'Falsify']:
        require(evidence.count(f'**{case} —') == 1, f'missing/duplicate disposition: {case}')
    require('D2 status: **unverified**' in evidence, 'discovery cannot certify delivery')
    seen = set()
    areas = set()
    for entry in data['functions']:
        path, symbol = entry['path'], entry['symbol']
        require((path, symbol) not in seen, f'duplicate scope: {symbol}')
        seen.add((path, symbol))
        require(path.endswith('.go') and not path.endswith('_test.go'), 'scope is not executable product Go')
        require(len(entry['reason'].strip()) > 20, f'missing inclusion reason: {symbol}')
        require(symbol in symbols(read_local(path)), f'absent executable function: {path}:{symbol}')
        old = symbols(git('show', f'{BASELINE}:{path}')) if path in REQUIRED else {}
        if symbol in old:
            require(entry['baseline_line'] == old[symbol], f'wrong baseline reference: {symbol}')
        areas.add(entry['area'])
    for path, names in REQUIRED.items():
        require(all((path, name) in seen for name in names), f'narrowed coverage scope: {path}')
    require({'selection', 'parsing', 'review', 'refinement'} <= areas, 'missing executable area')
    # Compare the current source to the baseline, including uncommitted new files.
    for folder in ['internal/planner', 'pkg/manager']:
        for path in sorted((ROOT / folder).glob('*.go')):
            if path.name.endswith('_test.go'):
                continue
            relative = str(path.relative_to(ROOT))
            exists = subprocess.run(['git', '-C', str(ROOT), 'cat-file', '-e',
                f'{BASELINE}:{relative}'], capture_output=True).returncode == 0
            old = symbols(git('show', f'{BASELINE}:{relative}')) if exists else {}
            for name in symbols(read_local(relative)).keys() - old.keys():
                require((relative, name) in seen, f'new helper omitted from scope: {relative}:{name}')
    fixtures = data['fixtures']
    require(len(fixtures) >= 17, 'missing reusable fixture inventory')
    fixture_keys = set()
    for fixture in fixtures:
        key = fixture['path'], fixture['symbol']
        require(key not in fixture_keys, 'duplicate fixture')
        fixture_keys.add(key)
        require(key[0].endswith('_test.go') and key[1].startswith('Test'), 'not a test fixture')
        require(key[1] in symbols(read_local(key[0])), f'absent fixture: {key}')
    notes = read_local('NOTES.md')
    now = notes.split('## Now\n', 1)[1].split('\n## ', 1)[0]
    questions = notes.split('## Questions\n', 1)[1].split('\n## ', 1)[0]
    require('[Compact planning / US-009 / T-US-009-001]' in now, 'task absent from Now')
    require('compact-requirement-evidence.md' in questions, 'questions not retained')
    baseline_notes = subprocess.check_output(
        ['git', '-C', str(ROOT), 'show', f'{BASELINE}:NOTES.md'], text=True)
    require(notes.split('## For me', 1)[1] == baseline_notes.split('## For me', 1)[1],
            'owner-reserved notes changed')
    require(git('rev-parse', '--show-toplevel') == str(ROOT), 'wrong candidate root')
    subprocess.run(['git', '-C', str(ROOT), 'merge-base', '--is-ancestor', BASELINE, 'HEAD'], check=True)
    baseline_prompt = git('show', f'{BASELINE}:internal/planner/prompt.go')
    baseline_compact = baseline_prompt.split('const CompactStoryGenerationPrompt =', 1)[1].split('INTENT DOCUMENT:', 1)[0]
    require('RequirementIdentityRule' in baseline_compact and
            '"requirement_id"' not in baseline_compact.split('in this exact shape:', 1)[1],
            'recorded baseline contradiction not present')
    subprocess.run(['git', '-C', str(ROOT), 'merge-base', '--is-ancestor',
                    'be19a5b695db31b9b2146b15bd01e613be55e4c6', BASELINE], check=True)
    prompt = read_local('internal/planner/prompt.go')
    compact = prompt.split('const CompactStoryGenerationPrompt =', 1)[1].split('INTENT DOCUMENT:', 1)[0]
    require('RequirementIdentityRule' in compact, 'current compact identity rule missing')
    shape = compact.split('in this exact shape:', 1)[1]
    return {'status': 'completed', 'mode': 'discovery', 'head': git('rev-parse', 'HEAD'),
            'functions': len(seen), 'fixtures': len(fixtures),
            'compact_output_declares_requirement_id': '"requirement_id"' in shape,
            'repair_verified': False, 'delivery_verified': False}


# Digests pin the retained story contract, not a mutable local ignored artifact.
CONTRACT_HASH = '9232f399a4e9c170e3544ed0e7a60a4d437c31ff99c452233a92009fc6b0f295'
VERIFY_HASH = '0bb289f5db59a0674355c80cd4d75b7016665b835adb3fc56d983c73398daba4'


def source_digest():
    # Bind proofs to production, tests, scripts and the frozen scope, not prose.
    paths = set()
    for folder in ('internal/planner', 'pkg/manager', 'scripts/verification'):
        paths.update(p for p in (ROOT / folder).rglob('*') if p.suffix in ('.go', '.py', '.sh'))
    paths.update((ROOT / 'scripts').glob('verify-compact-requirement-*.sh'))
    paths.add(ROOT / MANIFEST)
    digest = hashlib.sha256()
    for path in sorted(paths):
        digest.update(str(path.relative_to(ROOT)).encode() + b'\0' + path.read_bytes())
    return digest.hexdigest()


def repair(result):
    output = ROOT / '.openexec/compact-requirement-checks'
    coverage = json.loads((output / 'coverage.json').read_text())
    mutations = json.loads((output / 'mutations.json').read_text())
    require(result['compact_output_declares_requirement_id'], 'compact output field absent')
    for proof in (coverage, mutations):
        require(proof['source_sha256'] == source_digest(), 'stale repair proof')
    require(coverage['statements'] > 0 and 10 * coverage['covered'] > 9 * coverage['statements'], 'insufficient coverage')
    require(len(coverage['functions']) == result['functions'], 'missing measured functions')
    require(mutations['restored'] and mutations['restored_sha256'] == hashlib.sha256(
        (ROOT / 'internal/planner/prompt.go').read_bytes()).hexdigest(), 'source not restored')
    require({m['mutation'] for m in mutations['mutations']} == {'output-key', 'identity-rule'}
        and len(mutations['mutations']) == 2 and all(m['exit_code'] == 1 for m in mutations['mutations']), 'missing mutations')
    result.update(mode='repair', repair_verified=True, coverage=coverage, mutations=mutations)
    return result


def delivery(result, evidence):
    """Check the retained assessment's structure, not remote delivery truth."""
    section = evidence.split('## PR73 and delivery evidence\n', 1)[1].split('\n## ', 1)[0]
    require(evidence.count('D2 status:') == 1 and
            'D2 status: **unverified**.' in section, 'missing/duplicate unverified D2 status')
    obligations = {
        'D2 delivery': ('unverified; retained', 'Agent Console',
                        'authenticated candidate-matched default-branch merge receipt'),
        'G-007 repository verification': ('distinct from D2; incomplete full-story verification',
                                        'repository runner', 'host test and canonical gate'),
        'Existing candidate and PR75 delivery': ('external follow-up; pending', 'Agent Console',
            'a2c7daf0a875c10027e2d680305633d0',
            'outcome/a2c7daf0a875c10027e2d680305633d0',
            'https://github.com/openexec/openexec/pull/75',
            '308b33886fe7b51a19c503f324c7387a', 'after the task queue finishes'),
        'Later agent-console parent retry': ('external follow-up; pending', 'Agent Console',
                                           'after OpenExec delivery', 'fresh parent-run evidence'),
    }
    for label, required in obligations.items():
        rows = [line for line in section.splitlines() if line.startswith(f'| {label} |')]
        require(len(rows) == 1 and all(value in rows[0] for value in required),
                f'missing/duplicate obligation, status or external ownership: {label}')
    for limitation in (
        'Prior satisfaction\nof D2 is not established',
        'Ancestry and a PR number in a commit subject do not prove a default-branch merge',
        'Historical notes are supporting context, not delivery receipts',
        'not an OpenExec serving revision, merge receipt or deployment proof',
        'historical refusal does not prove current remote non-delivery',
        'Structural evidence validation cannot establish\nan unobserved merge',
        'not a fresh run of this stage',
    ):
        require(limitation in section, f'missing delivery limitation: {limitation}')
    references = [MANIFEST, 'docs/verification/compact-requirement-results.json',
                  'docs/verification-evidence-delivery.md', 'docs/verification/delivery-result.json']
    for path in references:
        require(Path(path).name in section and bool(read_local(path).strip()),
                f'missing local evidence reference: {path}')
    require('`6dbeb1fc`' in section and '`2026-09-30T22:28:05Z`' in section,
            'missing supplied Console observation reference')
    commits = ['be19a5b695db31b9b2146b15bd01e613be55e4c6',
               'af99d8d5156065dbe74525b9c2e27162bee0c87b',
               '65d800426c2ef16ee1a3e0d8224a3161f4982828']
    for commit in commits:
        require(commit in section and git('cat-file', '-t', commit) == 'commit',
                f'missing local commit reference: {commit}')
    historical = json.loads(read_local(references[-1]))['goal_complete_negative']
    require(historical['exit_code'] == 1 and historical['all_16_local_cases_passed']
            and historical['error'] == 'D2 incomplete: candidate-matched Console merge evidence required',
            'historical delivery assessment changed; reassess D2')
    historical_repair = json.loads(read_local(references[1]))
    require(historical_repair['repair_verified'] is True
            and historical_repair['delivery_verified'] is False
            and historical_repair['full_story_verified'] is False,
            'historical repair assessment changed; reassess G-007 distinction')
    result.update(mode='delivery', assessment_validated=True, d2_status='unverified',
                  delivery_verified=False, repair_verified=False,
                  validation_scope='structural repository evidence only; cannot establish an unobserved merge',
                  external_followups=list(obligations), evidence_references=references,
                  commit_references=commits)
    return result


def main():
    try:
        require(len(sys.argv) == 2 and sys.argv[1] in ('discovery', 'repair', 'delivery'),
                'expected discovery|repair|delivery')
        evidence = read_local(EVIDENCE)
        result = validate(json.loads(read_local(MANIFEST)), evidence)
        if sys.argv[1] == 'repair':
            result = repair(result)
        elif sys.argv[1] == 'delivery':
            result = delivery(result, evidence)
        print(json.dumps(result, sort_keys=True))
    except (ValueError, KeyError, IndexError, OSError, subprocess.CalledProcessError) as error:
        print(json.dumps({'status': 'failed', 'error': str(error)}), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
