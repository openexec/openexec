"""Persisted fixture controls for the preparation contract."""
import copy
import json
import subprocess
import tempfile
import unittest
from pathlib import Path

import exhausted_task_delivery as delivery


class DeliveryTests(unittest.TestCase):
    def test_persisted_package_and_refusals(self):
        record = delivery.read(delivery.ROOT / delivery.RECORD)
        delivery.validate(delivery.ROOT, record)
        fixtures = delivery.read(delivery.ROOT / 'scripts/verification/fixtures/exhausted-task-delivery/refusals.json')
        for fixture in fixtures:
            with self.subTest(fixture['name']):
                changed = copy.deepcopy(record)
                parent = changed
                for key in fixture['path'][:-1]:
                    parent = parent[key]
                key = fixture['path'][-1]
                if fixture.get('delete'):
                    del parent[key]
                else:
                    parent[key] = fixture['value']
                # Write/re-read every fixture through the real CLI in an excluded
                # evidence directory, so failure is semantic, not source drift.
                with tempfile.NamedTemporaryFile(mode='w', suffix='.json', dir=delivery.ROOT / delivery.RESULTS) as output:
                    json.dump(changed, output)
                    output.flush()
                    result = subprocess.run(['scripts/verify-exhausted-task-delivery-evidence.sh', '--phase',
                                             'preparation', '--record', str(Path(output.name).relative_to(delivery.ROOT))],
                                            cwd=delivery.ROOT, text=True, capture_output=True)
                    self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                    self.assertIn(fixture['error'], result.stderr)

    def test_manifest_includes_uncommitted_bytes_modes_and_deletions(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            subprocess.run(['git', 'init', '-q', directory], check=True)
            source = root / 'implementation.go'
            source.write_text('first')
            subprocess.run(['git', 'add', '.'], cwd=root, check=True)
            first = delivery.content(root)
            source.write_text('unstaged')
            self.assertNotEqual(first, delivery.content(root))
            source.write_text('first')
            source.chmod(0o755)
            self.assertNotEqual(first, delivery.content(root))
            (root / 'new_test.go').write_text('untracked')
            self.assertEqual(len(delivery.content(root)[0]), 2)
            source.unlink()
            self.assertEqual([r['path'] for r in delivery.content(root)[0]], ['new_test.go'])

    def test_nonlocal_reference_refused(self):
        with self.assertRaisesRegex(ValueError, 'nonlocal'):
            delivery.local(delivery.ROOT, '../outside')


if __name__ == '__main__':
    unittest.main()
