"""Prove the approved persisted journey fails when schema correction is disabled."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]


def main():
    source = ROOT / 'internal/planner/review.go'
    original = source.read_bytes()
    marker = b'if errors.As(err, &decode) {'
    if original.count(marker) != 1:
        raise ValueError('schema correction mutation anchor is not unique')
    env = dict(os.environ, GOWORK='off', GOENV='off', GOFLAGS='',
               GOCACHE='/tmp/openexec-compact-go-cache')
    command = ['go', 'test', './pkg/manager', '-count=1', '-timeout=60s',
               '-run', '^TestReviewedPlanSchemaCorrection$/^approved$', '-json']
    output = ROOT / '.openexec/planner-schema-checks'
    output.mkdir(parents=True, exist_ok=True)
    receipt = output / 'control.json'
    receipt.unlink(missing_ok=True)
    with tempfile.TemporaryDirectory(prefix='planner-schema-control-') as directory:
        temp = Path(directory)
        mutated = temp / 'review.go'
        mutated.write_bytes(original.replace(marker, b'if false && errors.As(err, &decode) {'))
        overlay = temp / 'overlay.json'
        overlay.write_text(json.dumps({'Replace': {str(source): str(mutated)}}))
        negative_command = command + ['-overlay', str(overlay)]
        negative = subprocess.run(negative_command, cwd=ROOT, env=env, text=True, capture_output=True)
        (output / 'control-disabled.jsonl').write_text(negative.stdout)
        events = [json.loads(line) for line in negative.stdout.splitlines()]
        expected = 'json: cannot unmarshal array into Go struct field Story.stories.requirement_id of type string'
        if (negative.returncode != 1 or expected not in negative.stdout
                or 'correction failed:' not in negative.stdout
                or not any(e.get('Action') == 'fail' and e.get('Test') ==
                           'TestReviewedPlanSchemaCorrection/approved' for e in events)):
            raise RuntimeError('control did not fail for the expected regression:\n' + negative.stdout + negative.stderr)
    if source.read_bytes() != original:
        raise ValueError('control modified production source')
    positive = subprocess.run(command, cwd=ROOT, env=env, text=True, capture_output=True)
    (output / 'control-restored.jsonl').write_text(positive.stdout)
    events = [json.loads(line) for line in positive.stdout.splitlines()]
    if (positive.returncode or any(e.get('Action') in ('skip', 'fail') for e in events)
            or not any(e.get('Action') == 'pass' and e.get('Test') ==
                       'TestReviewedPlanSchemaCorrection/approved' for e in events)):
        raise RuntimeError('restored journey failed:\n' + positive.stdout + positive.stderr)
    result = dict(disabled_command=negative_command, disabled_exit=negative.returncode,
                  expected_failure=expected, restored_command=command,
                  restored_exit=positive.returncode,
                  source_sha256=hashlib.sha256(original).hexdigest(), source_unchanged=True)
    receipt.write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(result, indent=2))


if __name__ == '__main__':
    main()
