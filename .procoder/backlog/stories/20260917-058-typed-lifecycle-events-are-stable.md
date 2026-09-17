# Typed lifecycle events are stable

Status: done
Created: 2026-09-17
Epic: provenance-and-events-contract
Sprint: 011-phase-9-integration-contracts-roadmap

## Description

Typed lifecycle events are stable.

## Acceptance criteria

- [x] Internal typed events map to CRD status, Kubernetes Events, and future event sinks.

## Evidence

- Evidence: `internal/contracts` defines stable typed lifecycle event reasons (`SolderPlanned`, `SolderApproved`, `SolderApplied`, `SolderHealthy`, `SolderFailed`, `SolderRolledBack`) with deterministic listing tests.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
