# YAML renderer decodes plain manifests

Status: done
Created: 2026-09-17
Epic: renderers-normalization-validation
Sprint: 012-product-application-reconcile-pipeline

## Description

YAML renderer decodes plain manifests.

## Acceptance criteria

- [x] Multi-document YAML renders to unstructured objects.
- [x] Malformed YAML and duplicate identities fail validation.

## Evidence

- Evidence: Application reconciliation now calls the renderer selected by `spec.source.render.type` through the product path before validation, live read, planning, and Revision persistence. Controller tests cover rendered objects producing a Revision plan; renderer unit tests still cover multi-document YAML and malformed input.

## Correction

- Resolved: product-path controller integration is now covered by tests and evidence above.
