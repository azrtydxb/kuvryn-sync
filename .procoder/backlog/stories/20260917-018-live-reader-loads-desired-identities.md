# Live reader loads desired identities

Status: done
Created: 2026-09-17
Epic: live-state-and-diff
Sprint: 012-product-application-reconcile-pipeline

## Description

Live reader loads desired identities.

## Acceptance criteria

- [x] Planner reads live objects matching desired and previously managed resources.
- [x] Cluster-scoped and namespaced resources are handled.

## Evidence

- Evidence: Application reconciliation now reads live objects for rendered desired identities and, when pruning is enabled, inventories previously managed resources by Solder ownership label. Controller tests cover live update planning and stale managed resource prune planning.

## Correction

- Resolved: product-path controller integration is now covered by tests and evidence above.
