# Self-heal reapplies desired state only when enabled

Status: done
Created: 2026-09-17
Epic: self-heal-conflict-safety
Sprint: 007-phase-5-drift-and-self-heal

## Description

Self-heal reapplies desired state only when enabled.

## Acceptance criteria

- [x] selfHeal=false reports drift without mutation.
- [x] selfHeal=true reconciles managed drift subject to conflict policy.

## Evidence

- Evidence: `internal/drift.ShouldSelfHeal` requires Drifted state, `spec.sync.selfHeal: true`, non-suspended Application mutation safety, and no plan conflicts before reapply. Tests cover disabled, enabled, and suspended behavior.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
