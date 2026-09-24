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

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.

## Superseded: Solder is universal

Removed with `internal/contracts`: Solder is not built for a specific product integration. Actions go through the public API (`solder` CLI or annotations) for every client.
