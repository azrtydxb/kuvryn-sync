# Dhole observes deployment result without deploying

Status: done
Created: 2026-09-17
Epic: dhole-observer-contract
Sprint: 011-phase-9-integration-contracts-roadmap

## Description

Dhole observes deployment result without deploying.

## Acceptance criteria

- [x] Dhole can wait for Solder Revision plan/apply/rollout/health/stable states.
- [x] No Dhole command pushes workload changes directly to the cluster when Solder is used.

## Evidence

- Evidence: `internal/contracts.ObserveForDhole` exposes Application/Revision/phase/health/failure data from public CRD status only, allowing Dhole to observe deployment results without deploying. Tests verify observation shape.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
