"""Behavioral negative controls in a disposable copy; never mutate the candidate."""
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import tempfile

from plan_identity_discovery import ROOT

PACKAGE = './pkg/manager'


def require_assertion(result, test, assertion):
    if result.returncode != 1:
        raise ValueError('mutation must fail behaviorally with exit 1: ' + result.stdout + result.stderr)
    events = [json.loads(line) for line in result.stdout.splitlines()]
    failures = {e['Test'] for e in events if e.get('Action') == 'fail' and 'Test' in e}
    output = ''.join(e.get('Output', '') for e in events if e.get('Test', '').split('/')[0] == test.split('/')[0])
    top = test.split('/')[0]
    if test not in failures or any(f.split('/')[0] != top for f in failures) or assertion not in output:
        raise ValueError('missing expected behavioral assertion or unrelated failure: ' + result.stdout)
    leaves = {f for f in failures if not any(other.startswith(f + '/') for other in failures)}
    for leaf in leaves:
        messages = ''.join(e.get('Output', '') for e in events if e.get('Test') == leaf)
        if assertion not in messages:
            raise ValueError('unrelated failing subtest: ' + leaf + ': ' + messages)
    if any(e.get('Action') == 'skip' for e in events):
        raise ValueError('mutation skipped a test')
    if '[build failed]' in result.stdout or 'panic:' in result.stdout:
        raise ValueError('unrelated build/panic failure')


def execute(root, test):
    return subprocess.run(['go', 'test', PACKAGE, '-count=1', '-timeout=60s',
                           '-run', '/'.join('^' + re.escape(part) + '$' for part in test.split('/')), '-json'],
                          cwd=root, text=True, capture_output=True, timeout=120)


def replace_once(source, old, new):
    if source.count(old) != 1:
        raise ValueError('missing or ambiguous mutation anchor: ' + old)
    return source.replace(old, new, 1)


def main():
    # Also isolate direct Python invocations from an enclosing checkout workspace.
    os.environ.update(GOWORK='off', GOENV='off', GOFLAGS='')
    os.environ.setdefault('GOCACHE', '/tmp/openexec-identity-go-cache')
    writer = 'pkg/manager/planner.go'
    comparator = 'internal/release/reviewed_identity.go'
    sql = 'internal/release/reviewed_plan_import.go'
    reader = 'internal/release/sqlite_store.go'
    originals = {p: (ROOT / p).read_text() for p in (writer, comparator, sql, reader)}
    changed_writer = originals[writer]
    for target, indent in [('st.AcceptanceCriteria', '\t\t\t'), ('st.DependsOn', '\t\t\t'), ('task.DependsOn', '\t\t\t\t')]:
        block = f'{indent}if {target} == nil {{\n{indent}\t{target} = []string{{}}\n{indent}}}\n'
        changed_writer = replace_once(changed_writer, block, '')
    changed_comparator = replace_once(originals[comparator], '\tif old == nil {\n\t\told = []string{}\n\t}\n', '')
    changed_sql = replace_once(originals[sql],
        '"json(CASE WHEN "+column+" IS NULL OR json("+column+")=\'null\' THEN \'[]\' ELSE "+column+" END)=json(?)"',
        '"json("+column+")=json(?)"')
    text_only_sql = replace_once(originals[sql], 'OR json("+column+")=', 'OR "+column+"=')
    story_reader = replace_once(originals[reader], '&acceptanceCriteriaJSON, &story.VerificationScript',
                                '&acceptanceCriteriaJSON.String, &story.VerificationScript')
    task_reader = replace_once(originals[reader], '&task.VerificationScript, &dependsOnJSON,',
                               '&task.VerificationScript, &dependsOnJSON.String,')
    allocation = replace_once(originals[writer],
        '\t\tConflicts: func(candidate *planner.ProjectPlan) map[string]bool {\n\t\t\treturn reviewedIdentityConflicts(rel, candidate)\n\t\t},\n', '')
    refusal = replace_once(originals[sql],
        '\t\t\tif n != 1 {\n\t\t\t\treturn fmt.Errorf("reviewed %s %s conflicts with retained content", table, id)\n\t\t\t}\n', '')
    controls = [
        ('story SQL NULL reader', {reader: story_reader},
         'TestReviewedIdentityLegacyEmptyLists', 'converting NULL to string is unsupported'),
        ('task SQL NULL reader', {reader: task_reader},
         'TestReviewedIdentityLegacyEmptyLists', 'converting NULL to string is unsupported'),
        ('content allocation', {writer: allocation},
         'TestReviewedIdentityLifecycle', 'reviewed stories US-001 conflicts with retained content'),
        ('atomic conflict refusal', {sql: refusal},
         'TestReviewedIdentityPostReviewConflictAtomicRetry', 'genuine conflict not refused'),

        ('native replay', {writer: changed_writer, comparator: changed_comparator},
         'TestNativeIdenticalReimportPreservesIdentities', 'identical native replay moved US-001'),
        ('native JSON writer', {writer: changed_writer},
         'TestNativeIdenticalReimportCanonicalStorage', 'got null want []'),
        ('legacy comparator', {comparator: changed_comparator},
         'TestReviewedIdentityLegacyEmptyLists', 'exact legacy replay changed plan identities/content'),
        ('legacy BLOB predicate', {sql: text_only_sql},
         "TestReviewedIdentityLegacyEmptyLists/stories/depends_on/CAST('null'_AS_BLOB)/reviewed",
         'reviewed stories US-001 conflicts with retained content'),
        ('SQL predicate', {sql: changed_sql},
         "TestReviewedIdentityLegacyEmptyLists/stories/depends_on/'null'/reviewed",
         'reviewed stories US-001 conflicts with retained content'),
    ]
    with tempfile.TemporaryDirectory(prefix='.identity-mutations-', dir=ROOT) as directory:
        root = Path(directory)
        paths = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard'], cwd=ROOT).decode().split('\0')
        for name in set(paths) - {''}:
            source = ROOT / name
            if source.is_file() and not source.is_relative_to(root):
                destination = root / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, destination)
        for test in sorted({c[2] for c in controls}):
            result = execute(root, test)
            if result.returncode:
                raise ValueError('positive control failed: ' + result.stdout + result.stderr)
            events = [json.loads(line) for line in result.stdout.splitlines()]
            if not any(e.get('Test') == test and e.get('Action') == 'pass' for e in events):
                raise ValueError('positive control did not execute: ' + test)
            if any(e.get('Action') in ('skip', 'fail') for e in events):
                raise ValueError('positive control skipped or failed: ' + test)
        for label, replacements, test, assertion in controls:
            try:
                for path, content in replacements.items():
                    (root / path).write_text(content)
                # Compilation is a separate required success, never a negative control.
                subprocess.run(['go', 'test', '-c', '-o', str(root / 'identity.test'), PACKAGE],
                               cwd=root, check=True, capture_output=True, text=True, timeout=120)
                require_assertion(execute(root, test), test, assertion)
                print('PASS mutation: ' + label + ' compiled; expected assertion: ' + assertion, flush=True)
            finally:
                for path in replacements:
                    (root / path).write_text(originals[path])
    if root.exists():
        raise ValueError('disposable mutation copy was not cleaned')
    print('PASS mutation copies cleaned', flush=True)


if __name__ == '__main__':
    main()
