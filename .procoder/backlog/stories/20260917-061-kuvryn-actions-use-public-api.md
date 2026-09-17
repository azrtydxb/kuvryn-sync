# Kuvryn actions use public API

Status: done
Created: 2026-09-17
Epic: kuvryn-discovery-and-actions
Sprint: 011-phase-9-integration-contracts-roadmap

## Description

Kuvryn actions use public API.

## Acceptance criteria

- [x] Plan, approve sync, rollback, suspend, and resume are expressed as Solder CRD/API operations.

## Evidence

- Evidence: `internal/contracts.ValidatePublicAction` permits only public Solder CRD action patch targets and rejects direct workload/private targets. Tests reject `deployments.apps` actions.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
