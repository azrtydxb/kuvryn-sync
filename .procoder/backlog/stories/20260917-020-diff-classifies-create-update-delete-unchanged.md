# Diff classifies create update delete unchanged

Status: done
Created: 2026-09-17
Epic: live-state-and-diff
Sprint: 012-product-application-reconcile-pipeline

## Description

Diff classifies create update delete unchanged.

## Acceptance criteria

- [x] Classification is deterministic and sorted.
- [x] Tests cover empty, create-only, update-only, delete-only, and mixed plans.

## Evidence

- Evidence: Application reconciliation now calls `planner.Build` with desired, live, and prune-candidate objects and writes the result to Revision status. Controller tests cover create, update, and delete/prune plan actions through the controller path; planner unit tests cover deterministic mixed classification.

## Correction

- Resolved: product-path controller integration is now covered by tests and evidence above.
