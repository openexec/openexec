"""Execute the retained story commands, keeping exit codes and source bindings."""
import argparse
import json
import subprocess

import compact_requirement_evidence as evidence


def commands():
    script = json.loads(evidence.read_local(evidence.MANIFEST))['repair_verification']
    return script.splitlines()[1:]


def main():
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument('--step', type=int, choices=range(1, 9))
    args = parser.parse_args()
    output = evidence.ROOT / '.openexec/compact-requirement-checks'
    output.mkdir(parents=True, exist_ok=True)
    failed = False
    for index, command in enumerate(commands(), 1):
        if args.step is not None and index != args.step:
            continue
        receipt = output / f'story-{index}.json'
        digest = evidence.source_digest()
        receipt.write_text(json.dumps(dict(command=command, status='started', exit_code=None,
            source_sha256=digest), indent=2) + '\n')
        # Stream to disk: even an externally terminated sandbox leaves evidence.
        with (output / f'story-{index}.log').open('w') as log:
            result = subprocess.run(['bash', '-c', command], cwd=evidence.ROOT, text=True,
                                    stdout=log, stderr=subprocess.STDOUT)
        if digest != evidence.source_digest():
            raise ValueError('source changed during story check')
        receipt.write_text(json.dumps(dict(command=command, status='finished', exit_code=result.returncode,
            source_sha256=digest), indent=2) + '\n')
        print(json.dumps(dict(step=index, command=command, exit_code=result.returncode)), flush=True)
        failed |= result.returncode != 0
    return int(failed)


if __name__ == '__main__':
    raise SystemExit(main())
