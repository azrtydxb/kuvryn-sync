# Validation rejects unsafe desired state early

Status: done
Created: 2026-09-17
Epic: renderers-normalization-validation
Sprint: 012-product-application-reconcile-pipeline

## Description

Validation rejects unsafe desired state early.

## Acceptance criteria

- [x] Destination namespace restrictions, duplicate identities, missing APIs, and malformed metadata fail before mutation.
- [x] Server-side dry-run is used where practical.

## Evidence

- Evidence: Application reconciliation validates rendered desired state before live reads and before any SSA mutation. Controller tests verify invalid rendered resources create a failed Revision with `ValidationFailure`; validation unit tests cover missing metadata, duplicate identities, and destination namespace violations.

## Correction

- Resolved: product-path controller integration is now covered by tests and evidence above.
