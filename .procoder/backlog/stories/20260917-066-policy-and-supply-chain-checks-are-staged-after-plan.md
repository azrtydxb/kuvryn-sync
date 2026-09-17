# Policy and supply-chain checks are staged after plan

Status: done
Created: 2026-09-17
Epic: policy-supply-chain-notifications-ai-roadmap
Sprint: 011-phase-9-integration-contracts-roadmap

## Description

Policy and supply-chain checks are staged after plan.

## Acceptance criteria

- [x] Future policy stage can check latest images, limits, privilege, signatures, attestations, SBOM, namespaces, and replicas.

## Evidence

- Evidence: `docs/roadmap.md` stages policy/signature/SBOM/provenance checks after render/plan and before apply, consuming redacted plan/provenance data.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
