# OCI desired-state source is designed

Status: done
Created: 2026-09-17
Epic: multi-cluster-and-oci-roadmap
Sprint: 011-phase-9-integration-contracts-roadmap

## Description

OCI desired-state source is designed.

## Acceptance criteria

- [x] Repository type oci design supports immutable digest desired state without changing reconciliation core.

## Evidence

- Evidence: `docs/roadmap.md` defines OCI desired-state bundles as a future Repository source type using immutable digests, while Git remains the first adapter.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
