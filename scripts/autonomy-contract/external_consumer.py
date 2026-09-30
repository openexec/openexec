"""Compile and execute an unpublished external module against this checkout."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]


def main():
    env = os.environ.copy()
    env['GOCACHE'] = '/tmp/openexec-exit1-go-cache'
    env['GOWORK'] = 'off'
    with tempfile.TemporaryDirectory(prefix='openexec-external-consumer-') as directory:
        root = Path(directory)
        (root / 'go.mod').write_text('module example.invalid/openexec-consumer\n\ngo 1.25.0\n\n'
            'require github.com/openexec/openexec v0.0.0\n\n'
            'replace github.com/openexec/openexec => ' + json.dumps(str(ROOT)) + '\n')
        shutil.copyfile(ROOT / 'scripts/autonomy-contract/external-consumer/main.go', root / 'main.go')
        shutil.copyfile(ROOT / 'go.sum', root / 'go.sum')
        subprocess.run(['go', 'build', '-mod=mod', '-buildvcs=false', '-o', str(root / 'consumer'), '.'], cwd=root, env=env, check=True)
        subprocess.run([str(root / 'consumer')], cwd=root, env=env, check=True)


if __name__ == '__main__':
    main()
