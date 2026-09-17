# Task 6: Implement SSA apply and sync policy

Status: done
Created: 2026-09-17
Plan: .procoder/plans/solder-full-product.md

## Description

Manual/automatic sync, exact Revision approval, status transitions, conflict handling.

## Acceptance criteria

- [x] Work is mapped to one or more backlog stories.
- [x] Implementation files and tests are identified before coding the task.
- [x] Public API, samples, docs, and generated artifacts are updated when touched.
- [x] `procoder test` passes for the resulting change.
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- Implemented SSA applier, conflict-policy defaulting/rejection, exact revision approval checks, suspend/phase mutation gates, and lifecycle status helpers. `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` passed with 0 blocking findings.

## Correction

- Historical correction: previous closure covered implementation building blocks; later all-gap closure added product integration evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
