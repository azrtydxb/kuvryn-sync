# Phase 5 drift and self-heal

Status: closed 2026-09-17
Created: 2026-09-17
Milestones: phase-5-drift-self-heal
Spec: solder-full-product

## Goal

Add managed-resource watch mapping, drift classification that ignores normal mutations, independent drift status, and self-heal gating that only reapplies desired state when enabled and safe.

## Committed stories

- [x] `20260917-037-managed-resource-watches-enqueue-owning-applications.md` — managed resource watch mapping
- [x] `20260917-038-drift-distinguishes-real-changes-from-normal-mutations.md` — drift calculation
- [x] `20260917-039-drift-state-remains-independent-from-health.md` — independent drift status
- [x] `20260917-040-self-heal-reapplies-desired-state-only-when-enabled.md` — self-heal gating

## Execution plan

- [x] Add managed-resource-to-Application mapping from Solder labels.
- [x] Add drift classifier using normalized planner comparisons.
- [x] Add status helper preserving sync/health separation for drift.
- [x] Add self-heal decision policy with suspend/conflict safety.
- [x] Add tests and run gates.

## Sprint acceptance

- [x] Managed resources enqueue their owning Application deterministically.
- [x] Drift ignores expected server mutations and identifies meaningful differences.
- [x] Drift updates sync state without changing health.
- [x] Self-heal requires explicit enablement and mutation safety.
- [x] `procoder test` and `procoder check` pass with no blockers.

## Retro

Reusing planner normalization for drift avoided a second diff implementation and kept server-mutation behavior consistent.

Next sprint should connect retention and rollback to the same Revision/plan/apply primitives rather than special-casing rollback state.

Keep self-heal fail-closed: disabled, suspended, unowned, or conflicting resources do not mutate.

## Result

committed: 4
done: 4 (20260917-037-managed-resource-watches-enqueue-owning-applications, 20260917-038-drift-distinguishes-real-changes-from-normal-mutations, 20260917-039-drift-state-remains-independent-from-health, 20260917-040-self-heal-reapplies-desired-state-only-when-enabled)
carried: 0

## Retro

<!-- What slowed us down this sprint. -->

<!-- What we change next sprint because of it. -->

<!-- One adaptation from this sprint worth keeping. -->
