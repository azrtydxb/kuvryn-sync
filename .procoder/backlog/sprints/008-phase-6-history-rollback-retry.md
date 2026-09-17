# Phase 6 history rollback retry

Status: closed 2026-09-17
Created: 2026-09-17
Milestones: phase-6-revision-rollback
Spec: solder-full-product

## Goal

Finish bounded Revision history, safe source-cache cleanup, rollback target/plan primitives, and retry-loop protection for failed desired revisions.

## Committed stories

- [x] `20260917-041-revision-retention-honors-history-limit.md` — bounded Revision retention
- [x] `20260917-042-source-cache-and-temp-files-are-safely-garbage-collected.md` — source cache GC
- [x] `20260917-043-rollback-resolves-last-healthy-revision.md` — last healthy rollback target
- [x] `20260917-044-rollback-uses-normal-plan-apply-machinery.md` — rollback planning through normal planner
- [x] `20260917-045-failed-desired-revisions-do-not-loop-forever.md` — retry-loop protection

## Supporting todo tasks

- [x] `.procoder/todo/20260917-task-12-implement-bounded-history-and-rollback.md`
- [x] `.procoder/todo/20260917-task-13-implement-retry-loop-protection.md`

## Execution plan

- [x] Add bounded history selection helpers.
- [x] Add Git cache stale-directory GC helper.
- [x] Add rollback target and plan helpers.
- [x] Add retry policy/backoff attempts helper.
- [x] Add tests and run gates.

## Sprint acceptance

- [x] History retention keeps newest audit entries and marks old ones for GC.
- [x] Source cache GC never removes active cache directories.
- [x] Rollback resolves previous healthy revision or a clear error.
- [x] Rollback planning reuses normal planner machinery.
- [x] Retry protection blocks loops and reports desired/deployed honestly.
- [x] `procoder test` and `procoder check` pass with no blockers.

## Retro

Rollback stayed small by resolving target selection separately from plan/apply/observe; rollback planning now consumes the same planner shape as forward reconciliation.

Next sprint should expand CLI and observability on top of these primitives without introducing private APIs.

Keep GC conservative: mark/select first, delete only old inactive cache or revision records.

## Result

committed: 5
done: 5 (20260917-041-revision-retention-honors-history-limit, 20260917-042-source-cache-and-temp-files-are-safely-garbage-collected, 20260917-043-rollback-resolves-last-healthy-revision, 20260917-044-rollback-uses-normal-plan-apply-machinery, 20260917-045-failed-desired-revisions-do-not-loop-forever)
carried: 0

## Retro

<!-- What slowed us down this sprint. -->

<!-- What we change next sprint because of it. -->

<!-- One adaptation from this sprint worth keeping. -->
