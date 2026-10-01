#!/usr/bin/env python3
"""Validate the exhausted-task records without claiming unperformed delivery."""
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent / 'verification'))
from exhausted_task_records import main

if __name__ == '__main__':
    raise SystemExit(main())
