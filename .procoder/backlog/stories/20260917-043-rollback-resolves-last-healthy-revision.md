# Rollback resolves last healthy Revision

Status: done
Created: 2026-09-17
Epic: rollback-planning-and-execution
Sprint: 008-phase-6-history-rollback-retry

## Description

Rollback resolves last healthy Revision.

## Acceptance criteria

- [x] Failed deployment finds the previous healthy Revision and its desired source state.
- [x] Missing previous healthy state produces clear failure.

## Evidence

- Evidence: `internal/rollback.Target` finds the most recent previous Healthy Revision for the same Application and returns a clear error when none exists. Tests cover happy and missing-target paths.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
