# Apply ordering uses graph and Kubernetes semantics

Status: done
Created: 2026-09-17
Epic: ordering-and-pruning
Sprint: 005-phase-3-ssa-apply-and-sync-policy

## Description

Apply ordering uses graph and Kubernetes semantics.

## Acceptance criteria

- [x] Namespaces/CRDs/prerequisites apply before workloads and dependents.
- [x] New CRDs trigger discovery refresh/retry before custom resources.

## Evidence

- Evidence: `internal/ordering.Apply` sorts common Kubernetes dependencies before workloads and routes, with deterministic identity tie-breaking. Tests cover Namespace/ConfigMap/Service/Deployment ordering.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
