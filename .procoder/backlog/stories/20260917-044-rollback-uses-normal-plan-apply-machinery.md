# Rollback uses normal plan apply machinery

Status: done
Created: 2026-09-17
Epic: rollback-planning-and-execution
Sprint: 008-phase-6-history-rollback-retry

## Description

Rollback uses normal plan apply machinery.

## Acceptance criteria

- [x] Rollback plan shows current-to-previous changes.
- [x] Rollback applies and observes health like forward reconciliation.

## Evidence

- Evidence: `internal/rollback.Plan` reuses the normal planner to compare previous healthy desired state against current live state, returning the same bounded RevisionPlan shape used for forward changes. Tests verify update rollback plans.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
