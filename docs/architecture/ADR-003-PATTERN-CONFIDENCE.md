# ADR-003: Pattern Confidence and Metadata Evaluation Framework

Status: **Deferred** (framework documented for future implementation)
Date: 2026-04-10

## Problem

How do you evaluate whether a code pattern is modern, stale, or stable? Neither code age nor recency is a reliable proxy. "Modern" does not mean "good" — Linux io_uring is modern and still finding bugs. "Old" does not mean "bad" — Ceph's CRUSH algorithm is 20 years old and still best-in-class.

We need a framework that evaluates patterns based on observable outcomes, not aesthetic judgments about age.

## Core Insight

Evaluate the metadata AROUND code, not the code itself. The signals that matter are behavioral: how code survives production, how often it changes, and whether people are investing in it or abandoning it.

## Metadata Signals Table

| Signal | Source | Interpretation |
|--------|--------|----------------|
| Survival time under load | Production metrics, incident logs | Long survival without incidents = battle-tested |
| Churn rate | `git log` per file/function | High = unstable or evolving; Low = stable or abandoned |
| Issue/bug density | Issue trackers, `fix:` commit messages | High density = fragile pattern |
| Dependency health | Deprecation notices, security advisories | Unhealthy deps = risky pattern regardless of code quality |
| Test coverage trajectory | CI history | Increasing = active investment; Flat = maintenance; Decreasing = abandoning |
| Fork/adoption curve | Package download stats over time | Rising = gaining trust; Plateau = mature; Declining = being replaced |

## What OpenExec Already Captures

Several existing subsystems produce data that feeds into this framework:

- **engram `learning_log.json`** — records task outcomes (success/failure per approach)
- **Predictive loader `file_access_patterns`** — records which files are used per task type
- **`symbol_predictions` table** — tracks successful symbol lookups
- **Symbol indexer (Layer 1)** — captures function signatures and locations

## What Needs to Be Built

1. **Pattern outcome recording** — "used pattern X for task Y, passed/failed tests"
2. **Churn signals from git history** — per-symbol modification frequency
3. **Stability scores** — symbols with unchanged signatures across multiple indexing runs = stable
4. **Confidence annotations on blueprints** — "use this pattern (stability: high, 47 successful runs)"

## Moat Against External Services

External search tools (e.g. GitHits) can approximate stable/stale/modern from public GitHub metadata — stars, forks, commit frequency. This is breadth.

Only OpenExec's execution feedback loop can tell you what actually worked at YOUR scale, with YOUR constraints, in YOUR codebase. This is depth. The moat is the closed loop: pattern selected, pattern executed, outcome recorded, confidence updated.

## Cross-Project Discovery (Layer 4)

Future concept: walk all projects in a user-level knowledge store, match new project structure against known blueprint patterns, share learned confidence scores across projects. A pattern proven stable in three of your projects carries that confidence into the fourth automatically.
