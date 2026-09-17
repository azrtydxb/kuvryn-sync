# Graph supports ordering health and visualization

Status: done
Created: 2026-09-17
Epic: resource-graph-inference
Sprint: 006-phase-4-graph-health-diagnosis

## Description

Graph supports ordering health and visualization.

## Acceptance criteria

- [x] Graph output can be consumed by ordering, health propagation, diagnosis, and Kuvryn.

## Evidence

- Evidence: `internal/graph.Graph` exposes stable node/edge JSON shapes and child traversal over resource IDs, with deterministic sorting suitable for ordering, health dependency traversal, and visualization consumers.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
