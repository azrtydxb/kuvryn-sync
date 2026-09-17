# Lineage links running Pods to source commit

Status: done
Created: 2026-09-17
Epic: source-to-pod-lineage
Sprint: 011-phase-9-integration-contracts-roadmap

## Description

Lineage links running Pods to source commit.

## Acceptance criteria

- [x] Pod -> ReplicaSet -> Deployment -> Solder Revision -> desired Git -> artifact digest -> pipeline run -> source commit is queryable.

## Evidence

- Evidence: `internal/contracts.LineageAnnotations` defines stable `solder.io/source-repository` and `solder.io/source-revision` annotations, with `solder.io/application` label constant for workload linkage. Tests verify annotation output.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
