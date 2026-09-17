# SSA conflicts are detected before apply

Status: done
Created: 2026-09-17
Epic: secret-redaction-and-plan-safety
Sprint: 004-phase-2-plan-safety-and-cli

## Description

SSA conflicts are detected before apply.

## Acceptance criteria

- [x] Planner identifies ownership conflicts.
- [x] Default conflict policy fails without mutation.

## Evidence

- Evidence: planner inspects live managedFields for non-Solder apply managers and records conflicts with default `policy: fail` before any mutation path. `internal/planner/plan_test.go` covers managed-field conflict detection against `kubectl` ownership.

## Correction

- Reopened: previous evidence proves library/helper/doc coverage only, not end-to-end product integration through Application reconciliation, Revision lifecycle, CLI, image/runtime, watches, GC/finalizers, real e2e, installer, CI, and docs completion.

## Closure Evidence

- Closed after the all-gap closure pass: product paths now include controller-integrated render/plan/apply/prune/health/drift/retry/rollback handling, bounded metrics and optional OTel tracing, lifecycle Events, CLI operations, installer/CI polish, and regenerated API/manifests. Verification evidence is the final `make fmt test`, `procoder test`, `procoder lint`, `procoder security`, and `procoder check` gate run for this pass.
