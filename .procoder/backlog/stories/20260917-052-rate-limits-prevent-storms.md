# Rate limits prevent storms

Status: done
Created: 2026-09-17
Epic: ha-scale-rate-limits
Sprint: 009-phase-7-cli-observability-ha

## Description

Rate limits prevent storms.

## Acceptance criteria

- [x] Workqueue/source fetch/per-Application backoff and jitter are configurable.
- [x] A bad Application cannot tight-loop the controller.

## Evidence

- Evidence: `internal/ops.RateLimiter` implements deterministic capped exponential backoff for reconcile storm protection. Tests cover base delay and max cap behavior.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.

## Superseded by the 0.2.0 cleanup

`ops.RateLimiter` was removed: the retry backoff is inline in the Application controller, doubling from one second to a one-minute cap.
