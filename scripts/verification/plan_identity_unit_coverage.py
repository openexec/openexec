"""Measure the discovery inventory, preserving the existing public-path suite."""
import json

from plan_identity_discovery import ROOT, MANIFEST, validate, changed_functions
from reviewed_plan_identity import main as measure


def main():
    inventory = json.loads((ROOT / MANIFEST).read_text())

    def check(helper):
        validate(ROOT, inventory, helper)
        changed_functions(inventory, helper)

    measure(dict(inventory, tests=inventory['existing_tests']), check)
    print('PASS T-US-011-002: discovery-scoped identity unit coverage')


if __name__ == '__main__':
    main()
