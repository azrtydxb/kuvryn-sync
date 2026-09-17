# Task 4: Implement live reader and diff engine

Status: done
Created: 2026-09-17
Plan: .procoder/plans/solder-full-product.md

## Description

SSA-aware normalization, live lookup, deterministic change classification.

## Acceptance criteria

- [x] Work is mapped to one or more backlog stories.
- [x] Implementation files and tests are identified before coding the task.
- [x] Public API, samples, docs, and generated artifacts are updated when touched.
- [x] `procoder test` passes for the resulting change.
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- Implemented live reader, normalization, deterministic planner classification, and bounded Revision plan conversion.
- `make test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` passed with 0 blocking findings.

## Correction

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
