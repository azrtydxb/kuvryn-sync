# Managed resource watches enqueue owning Applications

Status: done
Created: 2026-09-17
Epic: managed-resource-watches
Sprint: 007-phase-5-drift-and-self-heal

## Description

Managed resource watches enqueue owning Applications.

## Acceptance criteria

- [x] Labels/annotations/indexes find managed resources efficiently.
- [x] Burst events are deduplicated and rate-limited.

## Evidence

- Evidence: `internal/drift.EnqueueOwner` maps Solder-managed resource events back to owning Application namespace/name through the `solder.io/application` label and ignores unmanaged resources. Tests cover managed and unmanaged cases.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
