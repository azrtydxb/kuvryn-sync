# Phase 3 SSA apply and sync policy

Status: closed 2026-09-17
Created: 2026-09-17
Milestones: phase-3-apply
Spec: solder-full-product

## Goal

Add the core mutation safety primitives: idempotent server-side apply, lifecycle status helpers, dependency-aware ordering, safe prune filtering, exact manual approval checks, and suspend/resume mutation gates.

## Committed stories

- [x] `20260917-026-ssa-applier-applies-desired-objects-idempotently.md` — SSA applier
- [x] `20260917-027-status-transitions-track-deployment-lifecycle.md` — lifecycle status transitions
- [x] `20260917-028-apply-ordering-uses-graph-and-kubernetes-semantics.md` — apply ordering
- [x] `20260917-029-prune-deletes-only-eligible-managed-resources.md` — safe prune
- [x] `20260917-030-manual-approval-targets-exact-revision.md` — exact approval
- [x] `20260917-031-suspend-and-resume-stop-mutations-safely.md` — suspend/resume mutation gates

## Execution plan

- [x] Implement SSA applier with fail-conflict default and ownership labels/annotations.
- [x] Implement Revision/Application lifecycle status helper functions.
- [x] Implement deterministic apply and prune ordering semantics.
- [x] Implement prune eligibility that requires prior Solder ownership and honors opt-outs.
- [x] Implement exact Revision approval and suspend gates as reusable policy checks.
- [x] Add tests and run gates.

## Sprint acceptance

- [x] Apply uses SSA and is idempotent in tests.
- [x] Conflict policy defaults to fail and never force-conflicts silently.
- [x] Status transition helpers keep sync state separate from health state.
- [x] Prune deletes only eligible managed resources and rejects high-risk/opt-out candidates.
- [x] Exact approval and suspend gates are tested.
- [x] `procoder test` and `procoder check` pass with no blockers.

## Retro

The apply slice stayed safe by keeping mutation policy (`internal/syncpolicy`), ordering (`internal/ordering`), prune eligibility (`internal/prune`), and SSA execution (`internal/applier`) as separate units with focused tests.

Next sprint should compose these primitives into controller reconciliation instead of expanding each primitive independently.

Keep fail-closed defaults: empty conflict policy means fail, missing approval means reject, missing ownership means do not prune.

## Result

committed: 6
done: 6 (20260917-026-ssa-applier-applies-desired-objects-idempotently, 20260917-027-status-transitions-track-deployment-lifecycle, 20260917-028-apply-ordering-uses-graph-and-kubernetes-semantics, 20260917-029-prune-deletes-only-eligible-managed-resources, 20260917-030-manual-approval-targets-exact-revision, 20260917-031-suspend-and-resume-stop-mutations-safely)
carried: 0

## Retro

<!-- What slowed us down this sprint. -->

<!-- What we change next sprint because of it. -->

<!-- One adaptation from this sprint worth keeping. -->
