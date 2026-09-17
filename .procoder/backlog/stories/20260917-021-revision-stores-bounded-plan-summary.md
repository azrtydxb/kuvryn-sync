# Revision stores bounded plan summary

Status: done
Created: 2026-09-17
Epic: change-plan-model-and-cli
Sprint: 012-product-application-reconcile-pipeline

## Description

Revision stores bounded plan summary.

## Acceptance criteria

- [x] Revision status includes create/update/delete/unchanged counts.
- [x] Detailed resources are bounded and set `truncated` when necessary.

## Evidence

- Evidence: Application reconciliation now persists `plan.RevisionPlan(limit)` onto deterministic Revision status. Controller tests verify bounded/redacted plan resources and summary counts are written through the product path.

## Correction

- Resolved: product-path controller integration is now covered by tests and evidence above.
