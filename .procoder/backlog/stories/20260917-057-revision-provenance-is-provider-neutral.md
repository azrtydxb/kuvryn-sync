# Revision provenance is provider-neutral

Status: done
Created: 2026-09-17
Epic: provenance-and-events-contract
Sprint: 011-phase-9-integration-contracts-roadmap

## Description

Revision provenance is provider-neutral.

## Acceptance criteria

- [x] Source, artifact, and pipeline provenance support Dhole, GitHub Actions, Jenkins, GitLab, and others.

## Evidence

- Evidence: Revision API already stores source/artifact/pipeline provenance without provider-specific coupling, and `internal/contracts.ValidateProvenance` validates provider-neutral provenance shapes. Tests cover valid neutral provenance and invalid empty evidence.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
