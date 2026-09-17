# Kustomize renderer normalizes output

Status: done
Created: 2026-09-17
Epic: renderers-normalization-validation
Sprint: 012-product-application-reconcile-pipeline

## Description

Kustomize renderer normalizes output.

## Acceptance criteria

- [x] Kustomize paths render reproducibly.
- [x] Renderer errors are surfaced as RenderFailure.

## Evidence

- Evidence: Application reconciliation now selects `internal/renderer/exec.KustomizeRenderer` for `RenderTypeKustomize`; render failures are recorded as `Revision.status.failure.reason=RenderFailure`. Existing fake-binary tests cover safe kustomize invocation and output decoding.

## Correction

- Resolved: product-path controller integration is now covered by tests and evidence above.
