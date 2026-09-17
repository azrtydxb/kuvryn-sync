# Task 16: Package and document release artifacts

Status: done
Created: 2026-09-17
Plan: .procoder/plans/solder-full-product.md

## Description

Helm/raw manifests, API docs, operation docs, Kind E2E, upgrade tests.

## Acceptance criteria

- [x] Work is mapped to one or more backlog stories.
- [x] Implementation files and tests are identified before coding the task.
- [x] Public API, samples, docs, and generated artifacts are updated when touched.
- [x] `procoder test` passes for the resulting change.
- [x] `procoder check` has no blocking findings for the resulting change.

## Evidence

- Added alpha Helm chart with controller and leader-election RBAC, install/API/operations/upgrade docs, KW BuildKit image build validation, and KW server dry-run validation for CRDs and chart manifests. Gates passed: helm lint/template, KW server dry-runs, make fmt test, procoder test/lint/security/check. Local Docker was not used.

## Correction

- Historical correction: previous closure covered implementation building blocks; later all-gap closure added product integration evidence.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
