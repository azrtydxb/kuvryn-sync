# Application waits for its dependencies

Status: done 2026-09-23
Created: 2026-09-23
Epic: application-dependencies
Sprint: -

## Description

As a platform engineer, I declare `dependsOn` so my workload Application does not apply before the operator it needs is Healthy.

## Acceptance criteria

- [x] `spec.dependsOn` lists Applications by name in the same namespace only (decided 2026-09-23); a cross-namespace reference is rejected by validation.
- [x] Planning proceeds but apply waits while any dependency is not Healthy at its deployed revision, with a `DependencyNotReady` condition.
- [x] Dependency cycles are detected and reported as a condition, not a hot loop.
- [x] Dependency becoming Healthy enqueues its dependents.

## Evidence

- `spec.dependsOn` ([]LocalObjectReference, max 16): a name-only reference, so cross-namespace dependencies cannot be expressed (decided 2026-09-23).
- Gating: `waits for a dependency to be Healthy at its desired revision, then applies` covers a missing health state, a Healthy dependency still rolling out a new revision (fails without the revision check, mutation checked), and applying once ready; the plan is still built while waiting and `DependenciesReady=False/DependencyNotReady` names the pending dependency.
- Cycles: `reports a dependency cycle instead of waiting forever` asserts `DependencyCycle` with `a -> b -> a` and no requeue; disabling cycle detection makes it fail (mutation checked).
- Wake-up: the controller watches Applications and maps changes to dependents via `dependentsOf`, asserted in the gating test.
- Gates: `make test`, `make lint` 0 issues, `procoder check` clean.
