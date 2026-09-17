# Task 14: Finish CLI

Status: done
Created: 2026-09-17
Plan: .procoder/plans/solder-full-product.md

## Description

All specified commands, flags, output formats, watches and timeouts.

## Acceptance criteria

- [x] Work is mapped to one or more backlog stories.
- [x] Implementation files and tests are identified before coding the task.
- [x] Public API, samples, docs, and generated artifacts are updated when touched.
- [x] `procoder test` passes for the resulting change.
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- Implemented core read rendering and safe public-API mutation patch builders for sync approval and suspend/resume; plan CLI already reads Revision CRDs. Gates passed: make fmt test, procoder test/lint/security/check.

## Correction

- Historical correction: previous closure covered implementation building blocks; later all-gap closure added product integration evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
