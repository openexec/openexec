# Refusal: long-history scenario under load on 8416c7856f9624666c480795df816566fb5a0e20

Date: 2026-11-17. No pass is claimed. The scenario was not run and no timing,
failure output, loader log or hash manifest exists, because the required head
is not available.

## What was missing

- **Head.** `git cat-file -t 8416c7856f9624666c480795df816566fb5a0e20` in this
  task checkout printed `fatal: git cat-file: could not get object info`.
  `git branch -a --contains 8416c78` printed `error: malformed object name
  8416c78` (the abbreviation does not resolve). No ref in this checkout
  mentions PR 1842. This checkout's `HEAD` is
  `fe39baf234c2891d83adf3842fee574592991954` (origin/main), and
  `git status --porcelain` is empty, so the checkout under test cannot be
  `8416c78`.
- A fetch that can reach the retained PR 1842 head is needed. This sandbox
  cannot fetch from the remote.
- **Sibling store not checked.** A direct `git cat-file` against
  `/mnt/data1/projects/openexec` was refused by the sandbox as outside the
  task checkout. A prior review reported the object also absent there
  (its HEAD was `186646ee`); that is a reviewer's report, not re-verified here.
- **Loader and load samples.** Reading `/proc/loadavg` was refused by the
  sandbox as outside the checkout, so host load before/during/after could not
  be recorded in this pass either. No full-suite loader was started, since
  there is no head to run the scenario on.

## Criteria status

1. Head verified: not met (commit absent).
2. Loader and loadavg: not met.
3. Scenario run and raw output: not met.
4. Attachment and SHA-256 manifest: not met (nothing to attach).
5. Diff against `8416c78`: not evaluable; the only change on this branch is
   this document. No test, threshold, timeout or source file changed.
6. Refusal naming what is missing: this document.

## To unblock

Make `8416c7856f9624666c480795df816566fb5a0e20` available in the object store
of a checkout the task may use (fetch the retained PR 1842 head), and permit
reading `/proc/loadavg`.
