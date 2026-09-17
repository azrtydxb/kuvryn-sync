# Kuvryn can discover Solder objects

Status: done
Created: 2026-09-17
Epic: kuvryn-discovery-and-actions
Sprint: 011-phase-9-integration-contracts-roadmap

## Description

Kuvryn can discover Solder objects.

## Acceptance criteria

- [x] Kuvryn reads Applications, Revisions, plans, drift, graph, deployments, rollbacks, and provenance through public APIs.

## Evidence

- Evidence: `internal/contracts.DiscoverForKuvryn` lists public Solder CRDs and supported actions for discovery. Tests verify discovery includes Repository/Application/Revision resources.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
