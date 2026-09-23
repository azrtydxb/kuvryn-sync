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

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.

## Superseded by the 0.2.0 cleanup

`internal/drift` was removed as unused: self-heal is the drift branch of `ApplicationReconciler.Reconcile`, which reapplies only when `spec.sync.selfHeal` is set.
