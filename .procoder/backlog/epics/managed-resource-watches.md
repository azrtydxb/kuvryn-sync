# managed-resource-watches

Status: done
Created: 2026-09-17
Milestone: phase-5-drift-self-heal
Spec: solder-full-product

## Description

Watch and index managed resources without expensive full-cluster scans or reconcile storms.

## Evidence

- Historical correction: child story product-integration evidence was required before closure; the all-gap closure pass records controller/runtime verification evidence below.

## Correction

- Historical correction: this epic was previously held for product-integration acceptance; the closure evidence below records the resolved product path.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
