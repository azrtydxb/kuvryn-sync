# Raw manifests and Helm chart install Solder

Status: done
Created: 2026-09-17
Epic: installation-docs-upgrades
Sprint: 010-phase-8-packaging-docs-kw-validation

## Description

Raw manifests and Helm chart install Solder.

## Acceptance criteria

- [x] Generated installer and Helm chart install controller, CRDs, RBAC, metrics, and samples.

## Evidence

- Evidence: Raw generated CRDs in `config/crd/bases` pass KW server dry-run, and alpha Helm chart `charts/solder` passes `helm lint`, renders with `helm template`, includes controller RBAC/leader-election RBAC, and passes KW server dry-run.

## Correction

- Historical correction: previous evidence was helper/doc oriented; later all-gap closure added controller/runtime, image, installer, CI, KW validation, and product e2e evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
