# Rollout observation respects timeout and context

Status: done
Created: 2026-09-17
Epic: health-evaluators-and-rollout-observation
Sprint: 006-phase-4-graph-health-diagnosis

## Description

Rollout observation respects timeout and context.

## Acceptance criteria

- [x] Health timeout bounds observation.
- [x] Progressing, Healthy, Degraded, Suspended, and Unknown states are reported.

## Evidence

- Evidence: `internal/health.Observe` repeatedly evaluates resource snapshots until all healthy, any degraded, context cancellation, or configured timeout. Tests prove healthy exit and deadline behavior.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.

## Superseded by the 0.2.0 cleanup

`health.Observe` was removed as unused: rollouts are observed by requeueing in `applyAndObserve`, bounded by the failure policy timeout.
