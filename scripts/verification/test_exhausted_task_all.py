import json
from pathlib import Path
import tempfile
import unittest

import exhausted_task_all as harness
import exhausted_task_review_contract as contract
from exhausted_task_proof import require_coverage


class AllContractTests(unittest.TestCase):
    def test_every_case_requires_exact_run_pass_and_package(self):
        required, cases = harness.required_cases()
        events = []
        for p, names in required.items():
            p = contract.MODULE + p
            events += [dict(Package=p, Test=n, Action=a) for n in names for a in ('run','pass')]
            events.append(dict(Package=p, Action='pass'))
        contract.check_events(events, cases)
        for c in cases:
            with self.subTest(case=c['id']):
                key = (contract.MODULE + c['package'], c['test'])
                without = [e for e in events if (e['Package'],e.get('Test')) != key]
                with self.assertRaises(ValueError): contract.check_events(without, cases)
                with self.assertRaises(ValueError):
                    contract.check_events(events + [dict(Package=key[0],Test=key[1],Action='skip')], cases)

    def test_atomic_threshold_and_missing_profiles(self):
        with tempfile.TemporaryDirectory() as d:
            profile, filled = Path(d)/'unit',Path(d)/'filled'
            fn = dict(path='logic.go',name='logic',change='test',start=dict(Line=1,Column=1),end=dict(Line=200,Column=1))
            blocks={'logic.go':[(n,1,n+1,1,1) for n in range(2,102)]}
            for hits in (89,90,91):
                profile.write_text('mode: atomic\n' + ''.join(f'logic.go:{n}.1,{n+1}.1 1 {int(n<2+hits)}\n' for n in range(2,102)))
                contract.fill_missing_blocks(blocks,profile,filled)
                result=contract.coverage.evaluate([fn],blocks,filled)
                self.assertEqual((result['covered'],result['statements']),(hits,100))
                if hits<=90:
                    with self.assertRaises(ValueError):require_coverage(result)
                else:require_coverage(result)
            profile.unlink()
            with self.assertRaises(FileNotFoundError):contract.fill_missing_blocks(blocks,profile,filled)
            profile.write_text('')
            with self.assertRaises(ValueError):contract.fill_missing_blocks(blocks,profile,filled)
            for contents in ('mode: atomic\n', 'mode: atomic\nlogic.go:2.1,3.1 1 1\n'):
                profile.write_text(contents)
                contract.fill_missing_blocks(blocks,profile,filled)
                result=contract.coverage.evaluate([fn],blocks,filled)
                self.assertEqual(result['statements'],100)
                with self.assertRaises(ValueError):require_coverage(result)


if __name__ == '__main__':unittest.main()
