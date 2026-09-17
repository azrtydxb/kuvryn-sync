# Status transitions track deployment lifecycle

Status: done
Created: 2026-09-17
Epic: ssa-sync-engine
Sprint: 012-product-application-reconcile-pipeline

## Description

Status transitions track deployment lifecycle.

## Acceptance criteria

- [x] Application and Revision statuses move through Planning, Applying, Observing, Healthy/Failed.
- [x] Observed generation and current Revision are recorded.

## Evidence

- Evidence: Application reconciliation now moves Application/Revision through Planning, AwaitingApproval, Applying, Observing/Healthy, and Failed paths. Controller tests cover planning, exact manual approval, automatic apply, health status, validation failure, and render failure states.

## Correction

- Resolved: product-path controller integration is now covered by tests and evidence above.
