# Application waits for its dependencies

Status: open
Created: 2026-09-23
Epic: application-dependencies
Sprint: -

## Description

As a platform engineer, I declare `dependsOn` so my workload Application does not apply before the operator it needs is Healthy.

## Acceptance criteria

- [ ] `spec.dependsOn` lists Applications by name in the same namespace only (decided 2026-09-23); a cross-namespace reference is rejected by validation.
- [ ] Planning proceeds but apply waits while any dependency is not Healthy at its deployed revision, with a `DependencyNotReady` condition.
- [ ] Dependency cycles are detected and reported as a condition, not a hot loop.
- [ ] Dependency becoming Healthy enqueues its dependents.

## Evidence

