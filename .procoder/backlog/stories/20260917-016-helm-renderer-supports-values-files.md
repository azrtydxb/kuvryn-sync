# Helm renderer supports values files

Status: done
Created: 2026-09-17
Epic: renderers-normalization-validation
Sprint: 012-product-application-reconcile-pipeline

## Description

Helm renderer supports values files.

## Acceptance criteria

- [x] Helm releaseName and valuesFiles render to normalized objects.
- [x] Solder does not reimplement Helm behavior beyond safe invocation/library use.

## Evidence

- Evidence: Application reconciliation now selects `HelmRenderer` and passes `releaseName` and `valuesFiles` from the Application spec. `application_controller_test.go` verifies Helm render options are propagated through the controller path.

## Correction

- Resolved: product-path controller integration is now covered by tests and evidence above.
