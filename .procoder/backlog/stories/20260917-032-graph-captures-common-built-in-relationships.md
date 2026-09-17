# Graph captures common built-in relationships

Status: done
Created: 2026-09-17
Epic: resource-graph-inference
Sprint: 006-phase-4-graph-health-diagnosis

## Description

Graph captures common built-in relationships.

## Acceptance criteria

- [x] Deployment/ReplicaSet/Pod, Service/EndpointSlice, Ingress/Service, HPA/workload, PDB/pods, PVC/PV edges are inferred.
- [x] Unknown relationships do not fail reconciliation.

## Evidence

- Evidence: `internal/graph.Build` infers deterministic owner, Service selector, PVC mount, and ConfigMap/Secret dependency edges across unstructured Kubernetes objects. Tests cover all common edge types.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
