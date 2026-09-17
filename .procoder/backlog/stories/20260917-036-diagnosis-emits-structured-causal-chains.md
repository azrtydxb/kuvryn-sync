# Diagnosis emits structured causal chains

Status: done
Created: 2026-09-17
Epic: deterministic-diagnosis
Sprint: 006-phase-4-graph-health-diagnosis

## Description

Diagnosis emits structured causal chains.

## Acceptance criteria

- [x] Common failures such as missing Secret produce resource-linked chains.
- [x] Same evidence powers status, CLI, Events, Kuvryn, and optional AI.

## Evidence

- Evidence: `internal/diagnosis.Build` combines graph edges and health results into deterministic structured causes with resource IDs, reason/message, dependencies, and symptoms. Tests cover causal output for an unhealthy workload.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
