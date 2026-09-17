# Normalizer ignores expected server mutations

Status: done
Created: 2026-09-17
Epic: live-state-and-diff
Sprint: 012-product-application-reconcile-pipeline

## Description

Normalizer ignores expected server mutations.

## Acceptance criteria

- [x] ManagedFields, resourceVersion, status, and controller-populated fields do not cause false changes.
- [x] SSA ownership is preserved for conflict detection.

## Evidence

- Evidence: The planner/normalizer is now invoked from Application reconciliation after live reads. Existing planner tests prove server-populated fields do not cause false updates, and controller tests prove live objects feed the product plan path.

## Correction

- Resolved: product-path controller integration is now covered by tests and evidence above.
