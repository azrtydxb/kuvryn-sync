# Product Application reconcile pipeline

Status: done
Created: 2026-09-17
Milestones: phase-1-source-render, phase-2-plan, phase-3-apply
Spec: solder-full-product

## Goal

Wire the first real product path through `Application` reconciliation: resolve Repository source, render desired state, validate it, read live/managed state, build a redacted bounded plan, create/update a `Revision`, enforce exact approval, apply/prune approved changes, and drive honest lifecycle/status transitions without claiming rollback, drift watches, metrics, or full e2e are complete.

## Committed stories

- [x] `20260917-014-yaml-renderer-decodes-plain-manifests.md` — integrate YAML renderer in Application reconcile
- [x] `20260917-015-kustomize-renderer-normalizes-output.md` — integrate Kustomize renderer in Application reconcile
- [x] `20260917-016-helm-renderer-supports-values-files.md` — integrate Helm renderer in Application reconcile
- [x] `20260917-017-validation-rejects-unsafe-desired-state-early.md` — enforce validation in product path
- [x] `20260917-018-live-reader-loads-desired-identities.md` — use live reader in product path
- [x] `20260917-019-normalizer-ignores-expected-server-mutations.md` — use normalizer through planner in product path
- [x] `20260917-020-diff-classifies-create-update-delete-unchanged.md` — build product change plan
- [x] `20260917-021-revision-stores-bounded-plan-summary.md` — persist product Revision plan/status
- [x] `20260917-027-status-transitions-track-deployment-lifecycle.md` — drive honest planning lifecycle states

## Execution plan

- [x] Replace Application controller placeholder status with a real reconciliation path.
- [x] Load Repository, resolve source, and render the configured Application source path.
- [x] Validate rendered desired state before live read, status plan persistence, or mutation.
- [x] Read live resources for desired/managed identities and build a bounded, redacted Revision plan.
- [x] Create or update a deterministic Revision record for the resolved desired revision/path/render tuple.
- [x] Update Application sync/status honestly as Planning/AwaitingApproval/Applying/Observing/Healthy/Failed.
- [x] Add envtest/integration coverage through the controller path, not only pure functions.
- [x] Run gates before closing anything.

## Sprint acceptance

- [x] A sample Application reconcile creates/updates a Revision with a bounded plan.
- [x] Renderer/validation/planner failures surface as Revision/Application status failures without leaking secrets.
- [x] Product-path tests would fail if render/validate/live-read/plan/Revision wiring is removed.
- [x] No story is re-closed based solely on pure-function coverage.
- [x] `procoder test` and `procoder check` pass with no blockers.

## Retro

<!-- What slowed us down this sprint. -->

<!-- What we change next sprint because of it. -->

<!-- One adaptation from this sprint worth keeping. -->

## Evidence

- Application reconcile product path now resolves source, renders desired state, validates before mutation, reads live and managed state, builds deterministic plans, persists bounded/redacted Revision plan status, enforces exact manual approval, applies automatic/approved syncs, prunes stale managed resources when enabled, records health/status transitions, and has controller-path envtest coverage.
