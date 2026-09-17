# Task 18: Implement Dhole and Kuvryn contracts

Status: done
Created: 2026-09-17
Plan: .procoder/plans/solder-full-product.md

## Description

Dhole observer workflow, Kuvryn CRD discovery/actions, source-to-Pod lineage.

## Acceptance criteria

- [x] Work is mapped to one or more backlog stories.
- [x] Implementation files and tests are identified before coding the task.
- [x] Public API, samples, docs, and generated artifacts are updated when touched.
- [x] `procoder test` passes for the resulting change.
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- Implemented Dhole observation contract, Kuvryn discovery/action validation, and source-to-pod lineage annotation helpers using public CRDs/status/actions only, with tests. Gates passed: make fmt test, procoder test/lint/security/check.

## Correction

- Historical correction: previous closure covered implementation building blocks; later all-gap closure added product integration evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
