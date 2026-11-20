# PR #206 state and head recheck

Fresh retrieval only. The original description and review record stay in
[openexec-ci-pr206-original-record.md](openexec-ci-pr206-original-record.md);
this file adds the current state, head and commit comparison.

## Retrieval

- Source: GitHub API through the `gh` CLI, repository `openexec/openexec`.
- Command: `gh pr view 206 --repo openexec/openexec --json state,headRefOid,commits,updatedAt`
- Retrieved: 2026-11-20T02:47:21Z (UTC, from `date -u` in the same session).

Raw output:

```json
{"commits":[{"authoredDate":"2026-11-04T15:42:52Z","committedDate":"2026-11-04T15:42:52Z","messageHeadline":"docs: investigate openexec-ci US-1000 T-US-1000-001","oid":"89c8311497cc7d15da7c2fb8e7fc33173dec84ad"}],"headRefOid":"89c8311497cc7d15da7c2fb8e7fc33173dec84ad","state":"OPEN","updatedAt":"2026-11-04T15:54:12Z"}
```

(Author fields omitted from this copy; every other field is verbatim.)

## Retrieved values

| Item | Retrieved value |
| --- | --- |
| State | `OPEN` |
| Head (full SHA) | `89c8311497cc7d15da7c2fb8e7fc33173dec84ad` |
| updatedAt | 2026-11-04T15:54:12Z |

## Comparison with the expected head

Expected head: `89c8311497cc7d15da7c2fb8e7fc33173dec84ad`.
Retrieved head: `89c8311497cc7d15da7c2fb8e7fc33173dec84ad`.
Result: the retrieved head matches the expected head, and the state is `OPEN`.

## Commits on the PR

| SHA | Committed (UTC) |
| --- | --- |
| `89c8311497cc7d15da7c2fb8e7fc33173dec84ad` | 2026-11-04T15:42:52Z |

The retrieved list holds one commit. No commit in it is newer than
`89c8311497cc7d15da7c2fb8e7fc33173dec84ad`; that commit is the head itself.
This rests on the commit list above, not on the head comparison alone.

## Unverified items

None. Every value above came from the retrieval named in this record. The
Goal Stop decision and accounting rows and review 6116e3bd are out of scope
here.
