# Suspend and resume stop mutations safely

Status: done
Created: 2026-09-17
Epic: manual-approval-and-sync-policy
Sprint: 005-phase-3-ssa-apply-and-sync-policy

## Description

Suspend and resume stop mutations safely.

## Acceptance criteria

- [x] Suspended Applications do not apply/prune/self-heal.
- [x] Observation remains sufficient to report status.

## Evidence

- Evidence: `internal/syncpolicy.EnsureMutationAllowed` blocks mutation for suspended Applications and for non-mutating Revision phases, while allowing Applying/RollingBack when not suspended. Tests cover suspended and awaiting-approval gates.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
